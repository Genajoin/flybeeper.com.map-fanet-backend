package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/flybeeper/fanet-backend/internal/config"
	"github.com/flybeeper/fanet-backend/internal/repository"
	"github.com/flybeeper/fanet-backend/pkg/utils"
	"golang.org/x/time/rate"
)

// Server HTTP/2 сервер
type Server struct {
	router     *gin.Engine
	httpServer *http.Server
	logger     *utils.Logger
	config     *config.Config
	restHandler *RESTHandler
}

// NewServer создает новый HTTP сервер
func NewServer(cfg *config.Config, repo repository.Repository, logger *utils.Logger) *Server {
	// Production mode для Gin
	if cfg.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	// Middleware
	router.Use(LoggerMiddleware(logger))
	router.Use(gin.Recovery())
	router.Use(CORSMiddleware())
	router.Use(RateLimitMiddleware())
	router.Use(CompressionMiddleware())
	router.Use(SecurityHeadersMiddleware())

	// REST handler
	restHandler := NewRESTHandler(repo, logger)

	server := &Server{
		router:      router,
		logger:      logger,
		config:      cfg,
		restHandler: restHandler,
	}

	// Настройка HTTP сервера с HTTP/2
	server.httpServer = &http.Server{
		Addr:         cfg.Server.Address,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Регистрация маршрутов
	server.setupRoutes()

	return server
}

// setupRoutes настраивает маршруты согласно OpenAPI спецификации
func (s *Server) setupRoutes() {
	// Health check
	s.router.GET("/health", s.healthCheck)

	// API v1 группа
	v1 := s.router.Group("/api/v1")
	{
		// REST endpoints согласно rest-api.yaml
		v1.GET("/snapshot", s.restHandler.GetSnapshot)
		v1.GET("/pilots", s.restHandler.GetPilots)
		v1.GET("/thermals", s.restHandler.GetThermals)
		v1.GET("/stations", s.restHandler.GetStations)
		v1.GET("/track/:addr", s.restHandler.GetTrack)

		// Protected endpoint (требует Bearer token)
		protected := v1.Group("/")
		protected.Use(AuthMiddleware(s.logger))
		{
			protected.POST("/position", s.restHandler.PostPosition)
		}
	}

	// WebSocket endpoint (будет реализован позже)
	s.router.GET("/ws/v1/updates", s.websocketHandler)

	// Метрики (для мониторинга)
	s.router.GET("/metrics", s.metricsHandler)
}

// Start запускает HTTP сервер
func (s *Server) Start() error {
	s.logger.WithFields(map[string]interface{}{
		"address": s.config.Server.Address,
		"mode":    gin.Mode(),
	}).Info("Starting HTTP/2 server")

	// Поддержка HTTP/2
	return s.httpServer.ListenAndServe()
}

// Shutdown корректное завершение сервера
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down HTTP server")
	return s.httpServer.Shutdown(ctx)
}

// Health check endpoint
func (s *Server) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"timestamp": time.Now().Unix(),
		"version":   "1.0.0",
	})
}

// WebSocket handler (заглушка, будет реализован позже)
func (s *Server) websocketHandler(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"code":    "not_implemented",
		"message": "WebSocket endpoint not yet implemented",
	})
}

// Metrics handler (простая реализация)
func (s *Server) metricsHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"http_requests_total": 0, // TODO: Реальные метрики
		"active_connections":  0,
		"uptime_seconds":      time.Now().Unix(),
	})
}

// ==================== Middleware ====================

// LoggerMiddleware логирование запросов
func LoggerMiddleware(logger *utils.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Обработка запроса
		c.Next()

		// Логирование
		latency := time.Since(start)
		status := c.Writer.Status()
		method := c.Request.Method
		path := c.Request.URL.Path
		clientIP := c.ClientIP()
		userAgent := c.Request.UserAgent()

		logger.WithFields(map[string]interface{}{
			"method":     method,
			"path":       path,
			"status":     status,
			"latency_ms": latency.Milliseconds(),
			"client_ip":  clientIP,
			"user_agent": userAgent,
		}).Info("HTTP request completed")
	}
}

// CORSMiddleware настройка CORS
func CORSMiddleware() gin.HandlerFunc {
	return cors.New(cors.Config{
		AllowOrigins:     []string{"*"}, // В production указать конкретные домены
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"*"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	})
}

// RateLimitMiddleware ограничение частоты запросов
func RateLimitMiddleware() gin.HandlerFunc {
	limiter := rate.NewLimiter(rate.Limit(100), 200) // 100 req/sec, burst 200

	return func(c *gin.Context) {
		if !limiter.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"code":    "rate_limit_exceeded",
				"message": "Too many requests",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

// CompressionMiddleware компрессия ответов (Gin имеет встроенную поддержку)
func CompressionMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Gin автоматически обрабатывает gzip если установлен соответствующий middleware
		// Здесь можно добавить дополнительную логику компрессии
		c.Next()
	}
}

// SecurityHeadersMiddleware заголовки безопасности
func SecurityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Content-Security-Policy", "default-src 'self'")
		c.Next()
	}
}

// AuthMiddleware проверка Bearer token
func AuthMiddleware(logger *utils.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    "missing_authorization",
				"message": "Authorization header is required",
			})
			c.Abort()
			return
		}

		// Проверяем формат Bearer token
		if len(authHeader) < 7 || authHeader[:7] != "Bearer " {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    "invalid_token_format",
				"message": "Invalid authorization format",
			})
			c.Abort()
			return
		}

		token := authHeader[7:]
		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    "missing_token",
				"message": "Bearer token is required",
			})
			c.Abort()
			return
		}

		// TODO: Валидация токена через Laravel API
		// Это будет реализовано в auth пакете
		
		// Пока что принимаем любой не пустой токен
		logger.WithField("token_length", len(token)).Debug("Token validation (stub)")

		// Сохраняем информацию о пользователе в контексте
		c.Set("user_token", token)
		c.Set("user_authenticated", true)

		c.Next()
	}
}