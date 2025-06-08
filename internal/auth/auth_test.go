package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockRedisClient для тестирования
type MockRedisClient struct {
	mock.Mock
}

func (m *MockRedisClient) Get(ctx context.Context, key string) *redis.StringCmd {
	args := m.Called(ctx, key)
	cmd := redis.NewStringCmd(ctx)
	if err := args.Error(1); err != nil {
		cmd.SetErr(err)
	} else {
		cmd.SetVal(args.String(0))
	}
	return cmd
}

func (m *MockRedisClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
	args := m.Called(ctx, key, value, expiration)
	cmd := redis.NewStatusCmd(ctx)
	if err := args.Error(0); err != nil {
		cmd.SetErr(err)
	}
	return cmd
}

func (m *MockRedisClient) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	args := m.Called(ctx, keys)
	cmd := redis.NewIntCmd(ctx)
	if err := args.Error(1); err != nil {
		cmd.SetErr(err)
	} else {
		cmd.SetVal(int64(args.Int(0)))
	}
	return cmd
}

func (m *MockRedisClient) Keys(ctx context.Context, pattern string) *redis.StringSliceCmd {
	args := m.Called(ctx, pattern)
	cmd := redis.NewStringSliceCmd(ctx)
	if err := args.Error(1); err != nil {
		cmd.SetErr(err)
	} else {
		cmd.SetVal(args.Get(0).([]string))
	}
	return cmd
}

func TestUser_ToJSON(t *testing.T) {
	user := &User{
		ID:    123,
		Name:  "Test User",
		Email: "test@example.com",
		Role:  "user",
		Settings: map[string]interface{}{
			"theme": "dark",
			"lang":  "en",
		},
	}

	data, err := user.ToJSON()
	assert.NoError(t, err)
	assert.NotEmpty(t, data)

	// Проверяем, что можем десериализовать обратно
	restored, err := UserFromJSON(data)
	assert.NoError(t, err)
	assert.Equal(t, user.ID, restored.ID)
	assert.Equal(t, user.Name, restored.Name)
	assert.Equal(t, user.Email, restored.Email)
	assert.Equal(t, user.Role, restored.Role)
}

func TestUser_IsAdmin(t *testing.T) {
	adminUser := &User{Role: "admin"}
	regularUser := &User{Role: "user"}

	assert.True(t, adminUser.IsAdmin())
	assert.False(t, regularUser.IsAdmin())
}

func TestCache_SetAndGetUser(t *testing.T) {
	mockClient := &MockRedisClient{}
	cache := NewCache(mockClient, 5*time.Minute)

	user := &User{
		ID:    123,
		Name:  "Test User",
		Email: "test@example.com",
	}

	token := "test-token-123"
	ctx := context.Background()

	// Настраиваем mock для Set
	userData, _ := user.ToJSON()
	mockClient.On("Set", ctx, mock.AnythingOfType("string"), userData, 5*time.Minute).Return(nil)

	// Тестируем SetUser
	err := cache.SetUser(ctx, token, user)
	assert.NoError(t, err)

	// Настраиваем mock для Get
	mockClient.On("Get", ctx, mock.AnythingOfType("string")).Return(string(userData), nil)

	// Тестируем GetUser
	retrievedUser, err := cache.GetUser(ctx, token)
	assert.NoError(t, err)
	assert.Equal(t, user.ID, retrievedUser.ID)
	assert.Equal(t, user.Name, retrievedUser.Name)
	assert.Equal(t, user.Email, retrievedUser.Email)

	mockClient.AssertExpectations(t)
}

func TestCache_GetUser_NotFound(t *testing.T) {
	mockClient := &MockRedisClient{}
	cache := NewCache(mockClient, 5*time.Minute)

	token := "non-existent-token"
	ctx := context.Background()

	// Настраиваем mock для возврата redis.Nil
	mockClient.On("Get", ctx, mock.AnythingOfType("string")).Return("", redis.Nil)

	// Тестируем GetUser с несуществующим токеном
	user, err := cache.GetUser(ctx, token)
	assert.NoError(t, err)
	assert.Nil(t, user)

	mockClient.AssertExpectations(t)
}

func TestValidator_ValidateToken_Success(t *testing.T) {
	// Создаем mock HTTP сервер для Laravel API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Проверяем заголовки
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Accept"))

		// Возвращаем успешный ответ
		user := User{
			ID:    123,
			Name:  "Test User",
			Email: "test@example.com",
			Role:  "user",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(user)
	}))
	defer server.Close()

	mockClient := &MockRedisClient{}
	cache := NewCache(mockClient, 5*time.Minute)
	logger := logrus.New()

	validator := NewValidator(server.URL, cache, logger)

	ctx := context.Background()
	token := "test-token"

	// Настраиваем mock для кеша (токен не найден)
	mockClient.On("Get", ctx, mock.AnythingOfType("string")).Return("", redis.Nil)

	// Настраиваем mock для сохранения в кеш
	mockClient.On("Set", ctx, mock.AnythingOfType("string"), mock.AnythingOfType("[]uint8"), 5*time.Minute).Return(nil)

	// Тестируем валидацию
	user, err := validator.ValidateToken(ctx, token)
	assert.NoError(t, err)
	assert.NotNil(t, user)
	assert.Equal(t, 123, user.ID)
	assert.Equal(t, "Test User", user.Name)
	assert.Equal(t, "test@example.com", user.Email)

	mockClient.AssertExpectations(t)
}

func TestValidator_ValidateToken_Unauthorized(t *testing.T) {
	// Создаем mock HTTP сервер, который возвращает 401
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"message": "Unauthenticated."})
	}))
	defer server.Close()

	mockClient := &MockRedisClient{}
	cache := NewCache(mockClient, 5*time.Minute)
	logger := logrus.New()

	validator := NewValidator(server.URL, cache, logger)

	ctx := context.Background()
	token := "invalid-token"

	// Настраиваем mock для кеша (токен не найден)
	mockClient.On("Get", ctx, mock.AnythingOfType("string")).Return("", redis.Nil)

	// Тестируем валидацию с неверным токеном
	user, err := validator.ValidateToken(ctx, token)
	assert.Error(t, err)
	assert.Nil(t, user)
	assert.Contains(t, err.Error(), "invalid or expired token")

	mockClient.AssertExpectations(t)
}

func TestValidator_ValidateToken_CacheHit(t *testing.T) {
	mockClient := &MockRedisClient{}
	cache := NewCache(mockClient, 5*time.Minute)
	logger := logrus.New()

	// Создаем validator с фиктивным URL (не должен использоваться при cache hit)
	validator := NewValidator("http://localhost:12345", cache, logger)

	user := &User{
		ID:    123,
		Name:  "Cached User",
		Email: "cached@example.com",
	}

	ctx := context.Background()
	token := "cached-token"

	// Настраиваем mock для возврата пользователя из кеша
	userData, _ := user.ToJSON()
	mockClient.On("Get", ctx, mock.AnythingOfType("string")).Return(string(userData), nil)

	// Тестируем валидацию с кешем
	retrievedUser, err := validator.ValidateToken(ctx, token)
	assert.NoError(t, err)
	assert.NotNil(t, retrievedUser)
	assert.Equal(t, user.ID, retrievedUser.ID)
	assert.Equal(t, user.Name, retrievedUser.Name)

	mockClient.AssertExpectations(t)
}

// Бенчмарк для проверки производительности кеширования
func BenchmarkCache_SetUser(b *testing.B) {
	mockClient := &MockRedisClient{}
	cache := NewCache(mockClient, 5*time.Minute)

	user := &User{
		ID:    123,
		Name:  "Test User",
		Email: "test@example.com",
	}

	ctx := context.Background()

	// Настраиваем mock
	mockClient.On("Set", ctx, mock.AnythingOfType("string"), mock.AnythingOfType("[]uint8"), 5*time.Minute).Return(nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.SetUser(ctx, "token", user)
	}
}