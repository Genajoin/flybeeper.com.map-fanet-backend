package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/flybeeper/fanet-backend/internal/config"
)

var (
	// Version будет установлен при сборке через ldflags
	Version = "dev"
)

func main() {
	// Загружаем конфигурацию
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Инициализируем логирование
	logger := setupLogger(cfg)
	logger.Printf("Starting FANET Backend %s", Version)

	// Создаем контекст с отменой
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// TODO: Инициализируем Redis
	// redisClient := redis.NewClient(&redis.Options{...})

	// TODO: Инициализируем MQTT клиент
	// mqttClient := mqtt.NewClient(...)

	// TODO: Инициализируем сервисы
	// pilotService := service.NewPilotService(redisClient)
	// thermalService := service.NewThermalService(redisClient)
	// stationService := service.NewStationService(redisClient)

	// TODO: Инициализируем handlers
	// restHandler := handler.NewRESTHandler(...)
	// wsHandler := handler.NewWebSocketHandler(...)

	// Создаем HTTP сервер
	mux := http.NewServeMux()
	
	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Ready check endpoint
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		// TODO: Проверить готовность всех компонентов
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Ready"))
	})

	// TODO: Регистрируем API endpoints
	// mux.HandleFunc("/api/v1/snapshot", restHandler.GetSnapshot)
	// mux.HandleFunc("/api/v1/pilots", restHandler.GetPilots)
	// mux.HandleFunc("/api/v1/thermals", restHandler.GetThermals)
	// mux.HandleFunc("/api/v1/stations", restHandler.GetStations)
	// mux.HandleFunc("/api/v1/track/", restHandler.GetTrack)
	// mux.HandleFunc("/api/v1/position", restHandler.PostPosition)
	// mux.HandleFunc("/ws/v1/updates", wsHandler.Handle)

	// Временный тестовый endpoint
	mux.HandleFunc("/api/v1/test", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"version":"%s","status":"ok"}`, Version)
	})

	// Создаем HTTP сервер
	srv := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      mux,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// Запускаем сервер в горутине
	go func() {
		logger.Printf("HTTP server listening on :%s", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("Failed to start server: %v", err)
		}
	}()

	// TODO: Запускаем метрики сервер, если включен
	if cfg.Monitoring.MetricsEnabled {
		go func() {
			metricsServer := &http.Server{
				Addr:    ":" + cfg.Monitoring.MetricsPort,
				Handler: http.DefaultServeMux, // Prometheus регистрирует свои handlers глобально
			}
			logger.Printf("Metrics server listening on :%s", cfg.Monitoring.MetricsPort)
			if err := metricsServer.ListenAndServe(); err != nil {
				logger.Printf("Metrics server error: %v", err)
			}
		}()
	}

	// TODO: Запускаем MQTT обработчик
	// go mqttHandler.Start(ctx)

	// TODO: Запускаем фоновые задачи
	// go backgroundTasks(ctx)

	// Ждем сигнала остановки
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan

	logger.Printf("Received signal %v, shutting down...", sig)

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Останавливаем HTTP сервер
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Printf("HTTP server shutdown error: %v", err)
	}

	// TODO: Закрываем соединения
	// redisClient.Close()
	// mqttClient.Disconnect(250)

	logger.Println("Server stopped")
}

func setupLogger(cfg *config.Config) *log.Logger {
	// TODO: Настроить структурированное логирование (например, zerolog или zap)
	return log.New(os.Stdout, "[FANET] ", log.LstdFlags|log.Lshortfile)
}