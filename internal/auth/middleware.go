package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// Middleware для аутентификации запросов
type Middleware struct {
	validator *Validator
	logger    *logrus.Logger
}

// NewMiddleware создает новый middleware аутентификации
func NewMiddleware(validator *Validator, logger *logrus.Logger) *Middleware {
	return &Middleware{
		validator: validator,
		logger:    logger,
	}
}

// Authenticate проверяет токен аутентификации
func (m *Middleware) Authenticate() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := m.extractToken(c)
		if token == "" {
			m.logger.WithField("ip", c.ClientIP()).Warn("Missing authentication token")
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Missing authentication token",
				"code":  "MISSING_TOKEN",
			})
			c.Abort()
			return
		}

		user, err := m.validator.ValidateToken(c.Request.Context(), token)
		if err != nil {
			m.logger.WithFields(logrus.Fields{
				"ip":           c.ClientIP(),
				"token_prefix": token[:min(10, len(token))],
				"error":        err.Error(),
			}).Warn("Token validation failed")
			
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid or expired token",
				"code":  "INVALID_TOKEN",
			})
			c.Abort()
			return
		}

		// Сохраняем данные пользователя в контексте
		c.Set("user", user)
		c.Set("user_id", user.ID)
		c.Set("user_email", user.Email)

		m.logger.WithFields(logrus.Fields{
			"user_id": user.ID,
			"email":   user.Email,
			"method":  c.Request.Method,
			"path":    c.Request.URL.Path,
			"ip":      c.ClientIP(),
		}).Info("Authenticated request")

		c.Next()
	}
}

// OptionalAuthenticate пытается аутентифицировать пользователя, но не требует этого
// Полезно для endpoints, которые могут работать как с авторизацией, так и без неё
func (m *Middleware) OptionalAuthenticate() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := m.extractToken(c)
		if token == "" {
			c.Next()
			return
		}

		user, err := m.validator.ValidateToken(c.Request.Context(), token)
		if err != nil {
			m.logger.WithFields(logrus.Fields{
				"ip":           c.ClientIP(),
				"token_prefix": token[:min(10, len(token))],
				"error":        err.Error(),
			}).Debug("Optional token validation failed")
			c.Next()
			return
		}

		// Сохраняем данные пользователя в контексте
		c.Set("user", user)
		c.Set("user_id", user.ID)
		c.Set("user_email", user.Email)

		m.logger.WithFields(logrus.Fields{
			"user_id": user.ID,
			"email":   user.Email,
			"method":  c.Request.Method,
			"path":    c.Request.URL.Path,
			"ip":      c.ClientIP(),
		}).Debug("Optional authentication successful")

		c.Next()
	}
}

// extractToken извлекает токен из запроса (header, query parameter или cookie)
func (m *Middleware) extractToken(c *gin.Context) string {
	// 1. Проверяем Authorization header
	authHeader := c.GetHeader("Authorization")
	if authHeader != "" {
		parts := strings.Split(authHeader, " ")
		if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
			return parts[1]
		}
	}

	// 2. Проверяем query parameter
	if token := c.Query("token"); token != "" {
		return token
	}

	// 3. Проверяем cookie
	if token, err := c.Cookie("token"); err == nil && token != "" {
		return token
	}

	return ""
}

// GetUser возвращает пользователя из контекста Gin
func GetUser(c *gin.Context) (*User, bool) {
	if user, exists := c.Get("user"); exists {
		if u, ok := user.(*User); ok {
			return u, true
		}
	}
	return nil, false
}

// GetUserID возвращает ID пользователя из контекста Gin
func GetUserID(c *gin.Context) (int, bool) {
	if userID, exists := c.Get("user_id"); exists {
		if id, ok := userID.(int); ok {
			return id, true
		}
	}
	return 0, false
}

// RequireEmailVerification проверяет, что email пользователя подтвержден
func (m *Middleware) RequireEmailVerification() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Authentication required",
				"code":  "AUTH_REQUIRED",
			})
			c.Abort()
			return
		}

		if !user.IsEmailVerified() {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Email verification required",
				"code":  "EMAIL_NOT_VERIFIED",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireAdmin проверяет, что пользователь является администратором
func (m *Middleware) RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Authentication required",
				"code":  "AUTH_REQUIRED",
			})
			c.Abort()
			return
		}

		if !user.IsAdmin() {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Admin access required",
				"code":  "INSUFFICIENT_PERMISSIONS",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// min возвращает минимальное из двух чисел
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}