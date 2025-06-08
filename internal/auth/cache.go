package auth

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisClient интерфейс для Redis клиента
type RedisClient interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
	Keys(ctx context.Context, pattern string) *redis.StringSliceCmd
}

// Cache управляет кешированием результатов аутентификации
type Cache struct {
	client RedisClient
	ttl    time.Duration
}

// NewCache создает новый экземпляр кеша аутентификации
func NewCache(client RedisClient, ttl time.Duration) *Cache {
	return &Cache{
		client: client,
		ttl:    ttl,
	}
}

// GetUser получает пользователя из кеша по токену
func (c *Cache) GetUser(ctx context.Context, token string) (*User, error) {
	key := c.tokenKey(token)
	data, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil // Пользователь не найден в кеше
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user from cache: %w", err)
	}

	user, err := UserFromJSON(data)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize user: %w", err)
	}

	return user, nil
}

// SetUser сохраняет пользователя в кеш
func (c *Cache) SetUser(ctx context.Context, token string, user *User) error {
	key := c.tokenKey(token)
	data, err := user.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to serialize user: %w", err)
	}

	err = c.client.Set(ctx, key, data, c.ttl).Err()
	if err != nil {
		return fmt.Errorf("failed to set user in cache: %w", err)
	}

	return nil
}

// DeleteUser удаляет пользователя из кеша (при logout)
func (c *Cache) DeleteUser(ctx context.Context, token string) error {
	key := c.tokenKey(token)
	err := c.client.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("failed to delete user from cache: %w", err)
	}
	return nil
}

// tokenKey генерирует ключ кеша для токена
func (c *Cache) tokenKey(token string) string {
	// Хешируем токен для безопасности и ограничения длины ключа
	hash := sha256.Sum256([]byte(token))
	return fmt.Sprintf("auth:token:%x", hash[:16]) // Используем первые 16 байт хеша
}

// ClearAll очищает весь кеш аутентификации (для тестирования)
func (c *Cache) ClearAll(ctx context.Context) error {
	pattern := "auth:token:*"
	keys, err := c.client.Keys(ctx, pattern).Result()
	if err != nil {
		return fmt.Errorf("failed to get cache keys: %w", err)
	}

	if len(keys) > 0 {
		err = c.client.Del(ctx, keys...).Err()
		if err != nil {
			return fmt.Errorf("failed to delete cache keys: %w", err)
		}
	}

	return nil
}