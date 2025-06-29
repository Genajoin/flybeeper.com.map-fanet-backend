package handler

import (
	"context"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/flybeeper/fanet-backend/internal/auth"
	"github.com/flybeeper/fanet-backend/internal/config"
	"github.com/flybeeper/fanet-backend/internal/metrics"
	"github.com/flybeeper/fanet-backend/internal/repository"
	"github.com/flybeeper/fanet-backend/internal/service"
	"github.com/flybeeper/fanet-backend/pkg/utils"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
)

// Server HTTP/2 сервер
type Server struct {
	router           *gin.Engine
	httpServer       *http.Server
	logger           *utils.Logger
	config           *config.Config
	restHandler      *RESTHandler
	wsHandler        *WebSocketHandler
	authMW           *auth.Middleware
	validationHandler *ValidationHandler
	boundaryTracker   *service.BoundaryTracker
}

// NewServer создает новый HTTP сервер
func NewServer(cfg *config.Config, repo repository.Repository, historyRepo repository.HistoryRepository, redisClient *redis.Client, logger *utils.Logger, validationService *service.ValidationService, boundaryTracker *service.BoundaryTracker) *Server {
	// Production mode для Gin
	if cfg.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	// Middleware
	router.Use(LoggerMiddleware(logger))
	router.Use(gin.Recovery())
	router.Use(CORSMiddleware(cfg.CORS))
	router.Use(RateLimitMiddleware())
	router.Use(CompressionMiddleware())
	router.Use(SecurityHeadersMiddleware())
	router.Use(metrics.HTTPMetricsMiddleware())

	// REST handler с boundary tracker
	restHandler := NewRESTHandler(repo, historyRepo, logger, boundaryTracker)
	
	// WebSocket handler
	wsHandler := NewWebSocketHandler(repo, logger)
	
	// Validation handler
	var validationHandler *ValidationHandler
	if validationService != nil {
		validationHandler = NewValidationHandler(validationService)
	}

	// Auth middleware - создаем logrus.Logger для совместимости
	logrusLogger := logrus.New()
	logrusLogger.SetLevel(logrus.InfoLevel)
	
	authCache := auth.NewCache(redisClient, cfg.Auth.CacheTTL)
	authValidator := auth.NewValidator(cfg.Auth.Endpoint, authCache, logrusLogger)
	authMW := auth.NewMiddleware(authValidator, logrusLogger)

	server := &Server{
		router:           router,
		logger:           logger,
		config:           cfg,
		restHandler:      restHandler,
		wsHandler:        wsHandler,
		authMW:           authMW,
		validationHandler: validationHandler,
		boundaryTracker:   boundaryTracker,
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

// GetWebSocketHandler возвращает WebSocket handler для интеграции с MQTT
func (s *Server) GetWebSocketHandler() *WebSocketHandler {
	return s.wsHandler
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
		protected.Use(s.authMW.Authenticate())
		{
			protected.POST("/position", s.restHandler.PostPosition)
		}

		// Validation endpoints (если validationHandler доступен)
		if s.validationHandler != nil {
			v1.POST("/invalidate/:device_id", s.validationHandler.InvalidateDevice)
			v1.GET("/validation/:device_id", s.validationHandler.GetValidationState)
			v1.GET("/validation/metrics", s.validationHandler.GetValidationMetrics)
		}
	}

	// WebSocket endpoint (будет реализован позже)
	s.router.GET("/ws/v1/updates", s.websocketHandler)

	// Prometheus метрики
	s.router.GET("/metrics", gin.WrapH(promhttp.Handler()))
	
	// pprof endpoints для профилирования (только в development)
	if s.config.Environment == "development" {
		pprofGroup := s.router.Group("/debug/pprof")
		{
			pprofGroup.GET("/", gin.WrapF(pprof.Index))
			pprofGroup.GET("/cmdline", gin.WrapF(pprof.Cmdline))
			pprofGroup.GET("/profile", gin.WrapF(pprof.Profile))
			pprofGroup.GET("/symbol", gin.WrapF(pprof.Symbol))
			pprofGroup.GET("/trace", gin.WrapF(pprof.Trace))
			pprofGroup.GET("/heap", gin.WrapH(pprof.Handler("heap")))
			pprofGroup.GET("/goroutine", gin.WrapH(pprof.Handler("goroutine")))
			pprofGroup.GET("/block", gin.WrapH(pprof.Handler("block")))
			pprofGroup.GET("/threadcreate", gin.WrapH(pprof.Handler("threadcreate")))
			pprofGroup.GET("/mutex", gin.WrapH(pprof.Handler("mutex")))
			pprofGroup.GET("/allocs", gin.WrapH(pprof.Handler("allocs")))
		}
		s.logger.Info("pprof profiling endpoints enabled at /debug/pprof/")
	}
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

// WebSocket handler 
func (s *Server) websocketHandler(c *gin.Context) {
	s.wsHandler.HandleWebSocket(c)
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
func CORSMiddleware(corsConfig config.CORSConfig) gin.HandlerFunc {
	return cors.New(cors.Config{
		AllowOrigins:     corsConfig.AllowedOrigins,
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

// Старый AuthMiddleware удален - теперь используется auth.Middleware