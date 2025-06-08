# Спецификация интеграции аутентификации FANET API с Laravel Passport

## Обзор

FANET API использует существующую систему аутентификации Laravel Passport из проекта flybeeper.com.api для обеспечения единой точки входа (SSO) между всеми сервисами FlyBeeper.

## Архитектура

### Компоненты системы

1. **Laravel API** (flybeeper.com.api) - центральный сервис аутентификации
   - OAuth2 сервер на базе Laravel Passport
   - Выдача и управление Bearer токенами
   - Хранение пользовательских данных

2. **FANET Backend** (этот проект) - потребитель токенов
   - Валидация токенов через Laravel API
   - Кеширование результатов валидации
   - Авторизация доступа к endpoints

3. **Web приложение** (maps.flybeeper.com) - клиент
   - Получение токена через Laravel API
   - Использование токена для доступа к FANET API

### Поток аутентификации

```mermaid
sequenceDiagram
    participant User
    participant WebApp as maps.flybeeper.com
    participant LaravelAPI as Laravel API
    participant FANETAPI as FANET API
    participant Redis as Redis Cache

    User->>WebApp: Вход в систему
    WebApp->>LaravelAPI: POST /api/v4/login
    LaravelAPI-->>WebApp: Bearer token
    WebApp->>WebApp: Сохранить token
    
    User->>WebApp: Запрос данных карты
    WebApp->>FANETAPI: GET /api/v1/snapshot<br/>Authorization: Bearer {token}
    
    FANETAPI->>Redis: Проверить кеш токена
    alt Токен в кеше
        Redis-->>FANETAPI: User data
    else Токен не в кеше
        FANETAPI->>LaravelAPI: GET /api/v4/user<br/>Authorization: Bearer {token}
        LaravelAPI-->>FANETAPI: User data или 401
        FANETAPI->>Redis: Кешировать на 5 мин
    end
    
    FANETAPI-->>WebApp: Данные или 401
```

## Спецификация API

### Endpoint валидации токена

**Laravel API endpoint для проверки токена:**

```
GET https://api.flybeeper.com/api/v4/user
Authorization: Bearer {token}
```

**Успешный ответ (200):**
```json
{
    "id": 1234,
    "name": "Pilot Name",
    "email": "pilot@example.com",
    "email_verified_at": "2024-01-01T00:00:00Z",
    "role": "user",
    "settings": {
        "units": "metric",
        "language": "en"
    }
}
```

**Ошибка аутентификации (401):**
```json
{
    "message": "Unauthenticated."
}
```

### Защищенные endpoints FANET API

Следующие endpoints требуют аутентификации:

1. **POST /api/v1/position** - отправка собственной позиции
   - Требует валидный Bearer token
   - User ID берется из валидированного токена

### Формат токена

- **Тип**: Bearer token (JWT signed by Laravel Passport)
- **Передача**: 
  - Предпочтительно: `Authorization: Bearer {token}` header
  - Альтернативно: `?token={token}` query parameter
  - Cookie: `token={token}`

## Реализация в FANET Backend

### Конфигурация

```yaml
# Environment variables
AUTH_ENDPOINT: "https://api.flybeeper.com/api/v4/user"
AUTH_CACHE_TTL: 300  # 5 минут в секундах
AUTH_TIMEOUT: 5s     # Таймаут запроса к Laravel API
```

### Middleware аутентификации

```go
// internal/auth/middleware.go
type AuthMiddleware struct {
    laravelAPI   string
    cache        *redis.Client
    cacheTTL     time.Duration
    httpClient   *http.Client
}

func (m *AuthMiddleware) Authenticate() gin.HandlerFunc {
    return func(c *gin.Context) {
        token := m.extractToken(c)
        if token == "" {
            c.JSON(401, gin.H{"error": "Missing authentication token"})
            c.Abort()
            return
        }
        
        user, err := m.validateToken(c.Request.Context(), token)
        if err != nil {
            c.JSON(401, gin.H{"error": "Invalid or expired token"})
            c.Abort()
            return
        }
        
        c.Set("user", user)
        c.Set("user_id", user.ID)
        c.Next()
    }
}
```

### Модель пользователя

```go
// internal/auth/user.go
type User struct {
    ID              int                    `json:"id"`
    Name            string                 `json:"name"`
    Email           string                 `json:"email"`
    EmailVerifiedAt *time.Time            `json:"email_verified_at"`
    Role            string                 `json:"role"`
    Settings        map[string]interface{} `json:"settings"`
}
```

### Кеширование

```go
// Ключ кеша: "auth:token:{token_hash}"
// Значение: сериализованный User объект
// TTL: 5 минут

func (m *AuthMiddleware) getCachedUser(token string) (*User, error) {
    key := fmt.Sprintf("auth:token:%s", hashToken(token))
    data, err := m.cache.Get(ctx, key).Bytes()
    if err == redis.Nil {
        return nil, nil
    }
    // Десериализация user
}

func (m *AuthMiddleware) cacheUser(token string, user *User) error {
    key := fmt.Sprintf("auth:token:%s", hashToken(token))
    data, _ := json.Marshal(user)
    return m.cache.Set(ctx, key, data, m.cacheTTL).Err()
}
```

## Сценарии использования

### 1. Просмотр карты (без аутентификации)

```bash
# Получение данных без токена - разрешено
GET /api/v1/snapshot?lat=46&lon=8&radius=50
GET /ws/v1/updates?lat=46&lon=8&radius=50
```

### 2. Отправка собственной позиции (требует аутентификацию)

```bash
# Сначала логин через Laravel API
POST https://api.flybeeper.com/api/v4/login
Content-Type: application/json
{
    "email": "pilot@example.com",
    "password": "password"
}

# Ответ
{
    "message": "ok",
    "token": "eyJ0eXAiOiJKV1QiLCJhbGc...",
    "data": { "user": {...} }
}

# Использование токена для отправки позиции
POST /api/v1/position
Authorization: Bearer eyJ0eXAiOiJKV1QiLCJhbGc...
Content-Type: application/x-protobuf

{binary protobuf data}
```

### 3. WebSocket с аутентификацией (будущее)

```javascript
// Подключение с токеном для персонализированных данных
const ws = new WebSocket('wss://api.flybeeper.com/ws/v1/updates?token=' + token);
```

## Безопасность

### Валидация токенов

1. **Проверка формата**: JWT с правильной структурой
2. **Проверка через Laravel API**: единственный источник правды
3. **Кеширование**: снижение нагрузки на Laravel API
4. **Таймауты**: защита от медленных ответов

### Rate Limiting

- Общий rate limit: 100 req/sec per IP
- Аутентифицированные пользователи: 200 req/sec per user
- Endpoint /api/v1/position: 1 req/sec per user

### Обработка ошибок

```go
// 401 Unauthorized - токен недействителен или отсутствует
// 403 Forbidden - токен валиден, но нет прав на действие
// 429 Too Many Requests - превышен rate limit
```

## Миграция и совместимость

### Этапы внедрения

1. **Фаза 1** (текущая): Базовая проверка Bearer token
2. **Фаза 2**: Полная интеграция с Laravel API
3. **Фаза 3**: Поддержка ролей и расширенных прав

### Обратная совместимость

- Публичные endpoints остаются доступными без аутентификации
- Поддержка токенов в разных форматах (header, query, cookie)

## Мониторинг

### Метрики

- Количество успешных/неуспешных валидаций
- Cache hit rate для токенов
- Латентность запросов к Laravel API
- Количество активных аутентифицированных сессий

### Логирование

```go
log.WithFields(log.Fields{
    "user_id": user.ID,
    "endpoint": c.Request.URL.Path,
    "method": c.Request.Method,
    "ip": c.ClientIP(),
}).Info("Authenticated request")
```

## Будущие улучшения

1. **WebSocket аутентификация** для персонализированных обновлений
2. **Роли и права** (admin, moderator, pilot)
3. **API ключи** для автоматизированных систем
4. **OAuth2 flow** для сторонних приложений
5. **Refresh tokens** для долгоживущих сессий