package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config содержит конфигурацию приложения
type Config struct {
	Environment string
	Server      ServerConfig
	Redis       RedisConfig
	MQTT        MQTTConfig
	MySQL       MySQLConfig
	Auth        AuthConfig
	Geo         GeoConfig
	Performance PerformanceConfig
	Monitoring  MonitoringConfig
	Features    FeaturesConfig
}

// ServerConfig конфигурация HTTP сервера
type ServerConfig struct {
	Address      string
	Port         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

// RedisConfig конфигурация Redis
type RedisConfig struct {
	URL          string
	Password     string
	DB           int
	PoolSize     int
	MinIdleConns int
}

// MQTTConfig конфигурация MQTT
type MQTTConfig struct {
	URL          string
	ClientID     string
	Username     string
	Password     string
	CleanSession bool
	OrderMatters bool
	TopicPrefix  string
}

// MySQLConfig конфигурация MySQL (backup)
type MySQLConfig struct {
	DSN          string
	MaxIdleConns int
	MaxOpenConns int
}

// AuthConfig конфигурация аутентификации
type AuthConfig struct {
	Endpoint string
	CacheTTL time.Duration
}

// GeoConfig конфигурация геопространственных настроек
type GeoConfig struct {
	DefaultRadiusKM int
	MaxRadiusKM     int
	GeohashPrecision int
}

// PerformanceConfig конфигурация производительности
type PerformanceConfig struct {
	WorkerPoolSize      int
	MaxBatchSize        int
	BatchTimeout        time.Duration
	WebSocketPingInterval time.Duration
	WebSocketPongTimeout  time.Duration
}

// MonitoringConfig конфигурация мониторинга
type MonitoringConfig struct {
	MetricsEnabled bool
	MetricsPort    string
}

// FeaturesConfig флаги функций
type FeaturesConfig struct {
	EnableMySQLFallback bool
	EnableProfiling     bool
}

// Load загружает конфигурацию из переменных окружения
func Load() (*Config, error) {
	cfg := &Config{
		Environment: getEnv("ENVIRONMENT", "development"),
		Server: ServerConfig{
			Address:      getEnv("SERVER_ADDRESS", ":8090"),
			Port:         getEnv("SERVER_PORT", "8090"),
			ReadTimeout:  getDuration("SERVER_READ_TIMEOUT", 10*time.Second),
			WriteTimeout: getDuration("SERVER_WRITE_TIMEOUT", 10*time.Second),
			IdleTimeout:  getDuration("SERVER_IDLE_TIMEOUT", 120*time.Second),
		},
		Redis: RedisConfig{
			URL:          getEnv("REDIS_URL", "redis://localhost:6379"),
			Password:     getEnv("REDIS_PASSWORD", ""),
			DB:           getInt("REDIS_DB", 0),
			PoolSize:     getInt("REDIS_POOL_SIZE", 100),
			MinIdleConns: getInt("REDIS_MIN_IDLE_CONNS", 10),
		},
		MQTT: MQTTConfig{
			URL:          getEnv("MQTT_URL", "tcp://localhost:1883"),
			ClientID:     getEnv("MQTT_CLIENT_ID", "fanet-api"),
			Username:     getEnv("MQTT_USERNAME", ""),
			Password:     getEnv("MQTT_PASSWORD", ""),
			CleanSession: getBool("MQTT_CLEAN_SESSION", false),
			OrderMatters: getBool("MQTT_ORDER_MATTERS", false),
			TopicPrefix:  getEnv("MQTT_TOPIC_PREFIX", "fb/b/+/f"),
		},
		MySQL: MySQLConfig{
			DSN:          getEnv("MYSQL_DSN", ""),
			MaxIdleConns: getInt("MYSQL_MAX_IDLE_CONNS", 10),
			MaxOpenConns: getInt("MYSQL_MAX_OPEN_CONNS", 100),
		},
		Auth: AuthConfig{
			Endpoint: getEnv("AUTH_ENDPOINT", "https://api.flybeeper.com/api/v3/auth/verify"),
			CacheTTL: getDuration("AUTH_CACHE_TTL", 5*time.Minute),
		},
		Geo: GeoConfig{
			DefaultRadiusKM:  getInt("DEFAULT_RADIUS_KM", 200),
			MaxRadiusKM:      getInt("MAX_RADIUS_KM", 200),
			GeohashPrecision: getInt("GEOHASH_PRECISION", 5),
		},
		Performance: PerformanceConfig{
			WorkerPoolSize:        getInt("WORKER_POOL_SIZE", 100),
			MaxBatchSize:          getInt("MAX_BATCH_SIZE", 100),
			BatchTimeout:          getDuration("BATCH_TIMEOUT", 5*time.Second),
			WebSocketPingInterval: getDuration("WEBSOCKET_PING_INTERVAL", 30*time.Second),
			WebSocketPongTimeout:  getDuration("WEBSOCKET_PONG_TIMEOUT", 60*time.Second),
		},
		Monitoring: MonitoringConfig{
			MetricsEnabled: getBool("METRICS_ENABLED", true),
			MetricsPort:    getEnv("METRICS_PORT", "9090"),
		},
		Features: FeaturesConfig{
			EnableMySQLFallback: getBool("ENABLE_MYSQL_FALLBACK", true),
			EnableProfiling:     getBool("ENABLE_PROFILING", false),
		},
	}

	// Валидация
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// Validate проверяет корректность конфигурации
func (c *Config) Validate() error {
	// Проверка портов
	if c.Server.Port == "" {
		return fmt.Errorf("SERVER_PORT is required")
	}

	// Проверка Redis URL
	if c.Redis.URL == "" {
		return fmt.Errorf("REDIS_URL is required")
	}

	// Проверка MQTT URL
	if c.MQTT.URL == "" {
		return fmt.Errorf("MQTT_URL is required")
	}

	// Проверка geo настроек
	if c.Geo.MaxRadiusKM <= 0 {
		return fmt.Errorf("MAX_RADIUS_KM must be positive")
	}

	if c.Geo.GeohashPrecision < 1 || c.Geo.GeohashPrecision > 12 {
		return fmt.Errorf("GEOHASH_PRECISION must be between 1 and 12")
	}

	// Проверка производительности
	if c.Performance.WorkerPoolSize <= 0 {
		return fmt.Errorf("WORKER_POOL_SIZE must be positive")
	}

	if c.Performance.MaxBatchSize <= 0 {
		return fmt.Errorf("MAX_BATCH_SIZE must be positive")
	}

	return nil
}

// Helper функции для чтения переменных окружения

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func getDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

// LogLevel возвращает уровень логирования
func LogLevel() string {
	return getEnv("LOG_LEVEL", "info")
}

// LogFormat возвращает формат логирования
func LogFormat() string {
	return getEnv("LOG_FORMAT", "json")
}

// IsDevelopment проверяет, запущено ли приложение в режиме разработки
func IsDevelopment() bool {
	return getEnv("APP_ENV", "production") == "development"
}

// IsProduction проверяет, запущено ли приложение в production
func IsProduction() bool {
	return getEnv("APP_ENV", "production") == "production"
}