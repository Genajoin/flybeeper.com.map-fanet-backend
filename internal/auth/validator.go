package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
)

// Validator проверяет токены через Laravel API
type Validator struct {
	apiEndpoint string
	httpClient  *http.Client
	cache       *Cache
	logger      *logrus.Logger
}

// NewValidator создает новый валидатор токенов
func NewValidator(apiEndpoint string, cache *Cache, logger *logrus.Logger) *Validator {
	return &Validator{
		apiEndpoint: apiEndpoint,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		cache:  cache,
		logger: logger,
	}
}

// ValidateToken проверяет токен и возвращает данные пользователя
func (v *Validator) ValidateToken(ctx context.Context, token string) (*User, error) {
	// Сначала проверяем кеш
	if user, err := v.cache.GetUser(ctx, token); err != nil {
		v.logger.WithError(err).Warn("Failed to get user from cache")
	} else if user != nil {
		v.logger.WithField("user_id", user.ID).Debug("User found in cache")
		return user, nil
	}

	// Если не в кеше, проверяем через Laravel API
	user, err := v.validateWithAPI(ctx, token)
	if err != nil {
		return nil, err
	}

	// Кешируем результат
	if err := v.cache.SetUser(ctx, token, user); err != nil {
		v.logger.WithError(err).Warn("Failed to cache user")
	}

	v.logger.WithField("user_id", user.ID).Debug("User validated and cached")
	return user, nil
}

// validateWithAPI выполняет проверку токена через Laravel API
func (v *Validator) validateWithAPI(ctx context.Context, token string) (*User, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", v.apiEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "FANET-API/1.0")

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request to Laravel API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		var user User
		if err := json.Unmarshal(body, &user); err != nil {
			return nil, fmt.Errorf("failed to parse user data: %w", err)
		}
		return &user, nil

	case http.StatusUnauthorized:
		v.logger.WithField("token_prefix", token[:min(10, len(token))]).Debug("Token validation failed")
		return nil, fmt.Errorf("invalid or expired token")

	default:
		v.logger.WithFields(logrus.Fields{
			"status_code": resp.StatusCode,
			"response":    string(body),
		}).Error("Unexpected response from Laravel API")
		return nil, fmt.Errorf("Laravel API returned status %d", resp.StatusCode)
	}
}

// InvalidateToken удаляет токен из кеша (при logout)
func (v *Validator) InvalidateToken(ctx context.Context, token string) error {
	return v.cache.DeleteUser(ctx, token)
}

