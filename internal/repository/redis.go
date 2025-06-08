package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/flybeeper/fanet-backend/internal/config"
	"github.com/flybeeper/fanet-backend/internal/models"
	"github.com/flybeeper/fanet-backend/pkg/utils"
)

const (
	// Ключи для геопространственных индексов
	PilotsGeoKey   = "pilots:geo"     // GEO индекс для пилотов
	ThermalsGeoKey = "thermals:geo"   // GEO индекс для термиков
	StationsGeoKey = "stations:geo"   // GEO индекс для метеостанций
	
	// Префиксы для хешей с детальными данными
	PilotPrefix   = "pilot:"         // pilot:{device_id}
	ThermalPrefix = "thermal:"       // thermal:{thermal_id}
	StationPrefix = "station:"       // station:{station_id}
	
	// TTL для данных
	PilotTTL   = 24 * time.Hour      // Пилоты удаляются через 24 часа
	ThermalTTL = 6 * time.Hour       // Термики удаляются через 6 часов
	StationTTL = 7 * 24 * time.Hour  // Станции обновляются раз в неделю
	
	// Настройки батчинга
	MaxBatchSize = 100
)

// RedisRepository репозиторий для работы с Redis
type RedisRepository struct {
	client *redis.Client
	logger *utils.Logger
	config *config.RedisConfig
}

// NewRedisRepository создает новый Redis репозиторий
func NewRedisRepository(cfg *config.RedisConfig, logger *utils.Logger) (*RedisRepository, error) {
	if cfg == nil {
		return nil, fmt.Errorf("redis config cannot be nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	// Парсим Redis URL
	opt, err := redis.ParseURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
	}

	// Дополнительные настройки
	opt.Password = cfg.Password
	opt.DB = cfg.DB
	opt.PoolSize = cfg.PoolSize
	opt.MinIdleConns = cfg.MinIdleConns
	opt.ConnMaxIdleTime = 30 * time.Minute
	opt.DialTimeout = 10 * time.Second
	opt.ReadTimeout = 3 * time.Second
	opt.WriteTimeout = 3 * time.Second

	client := redis.NewClient(opt)

	repo := &RedisRepository{
		client: client,
		logger: logger,
		config: cfg,
	}

	return repo, nil
}

// Ping проверяет соединение с Redis
func (r *RedisRepository) Ping(ctx context.Context) error {
	_, err := r.client.Ping(ctx).Result()
	if err != nil {
		return fmt.Errorf("redis ping failed: %w", err)
	}
	return nil
}

// Close закрывает соединение с Redis
func (r *RedisRepository) Close() error {
	return r.client.Close()
}

// SavePilot сохраняет данные пилота
func (r *RedisRepository) SavePilot(ctx context.Context, pilot *models.Pilot) error {
	if pilot == nil {
		return fmt.Errorf("pilot cannot be nil")
	}

	pipe := r.client.Pipeline()

	// Сохраняем в геопространственный индекс
	pipe.GeoAdd(ctx, PilotsGeoKey, &redis.GeoLocation{
		Name:      pilot.DeviceID,
		Latitude:  pilot.Position.Latitude,
		Longitude: pilot.Position.Longitude,
	})

	// Сохраняем детальные данные в хеш
	pilotKey := PilotPrefix + pilot.DeviceID
	pilotData, err := json.Marshal(pilot)
	if err != nil {
		return fmt.Errorf("failed to marshal pilot data: %w", err)
	}

	pipe.Set(ctx, pilotKey, pilotData, PilotTTL)

	// Устанавливаем TTL для геопространственного индекса
	pipe.Expire(ctx, PilotsGeoKey, PilotTTL)

	// Выполняем все операции в батче
	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to save pilot: %w", err)
	}

	r.logger.Debug("Saved pilot to Redis", 
		"device_id", pilot.DeviceID,
		"lat", pilot.Position.Latitude,
		"lon", pilot.Position.Longitude)

	return nil
}

// GetPilotsInRadius возвращает пилотов в указанном радиусе
func (r *RedisRepository) GetPilotsInRadius(ctx context.Context, center models.GeoPoint, radiusKM float64) ([]*models.Pilot, error) {
	// Поиск по геопространственному индексу
	locations, err := r.client.GeoRadius(ctx, PilotsGeoKey, center.Longitude, center.Latitude, &redis.GeoRadiusQuery{
		Radius:    radiusKM,
		Unit:      "km",
		WithCoord: true,
		WithDist:  true,
		Count:     1000, // Максимум 1000 пилотов
		Sort:      "ASC", // Сортировка по расстоянию
	}).Result()

	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to get pilots in radius: %w", err)
	}

	if len(locations) == 0 {
		return []*models.Pilot{}, nil
	}

	// Получаем детальные данные пилотов батчем
	pipe := r.client.Pipeline()
	cmds := make([]*redis.StringCmd, len(locations))
	
	for i, loc := range locations {
		pilotKey := PilotPrefix + loc.Name
		cmds[i] = pipe.Get(ctx, pilotKey)
	}

	_, err = pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to get pilot details: %w", err)
	}

	// Парсим результаты
	pilots := make([]*models.Pilot, 0, len(locations))
	for i, cmd := range cmds {
		if cmd.Err() == redis.Nil {
			continue // Пилот удален, но еще в гео-индексе
		}
		if cmd.Err() != nil {
			r.logger.Warn("Failed to get pilot data", "device_id", locations[i].Name, "error", cmd.Err())
			continue
		}

		var pilot models.Pilot
		if err := json.Unmarshal([]byte(cmd.Val()), &pilot); err != nil {
			r.logger.Warn("Failed to unmarshal pilot data", "device_id", locations[i].Name, "error", err)
			continue
		}

		// Добавляем расстояние от центра поиска
		pilot.DistanceKM = locations[i].Dist
		pilots = append(pilots, &pilot)
	}

	r.logger.Debug("Retrieved pilots in radius", 
		"center_lat", center.Latitude,
		"center_lon", center.Longitude,
		"radius_km", radiusKM,
		"found", len(pilots))

	return pilots, nil
}

// SaveThermal сохраняет данные термика
func (r *RedisRepository) SaveThermal(ctx context.Context, thermal *models.Thermal) error {
	if thermal == nil {
		return fmt.Errorf("thermal cannot be nil")
	}

	pipe := r.client.Pipeline()

	// Сохраняем в геопространственный индекс
	pipe.GeoAdd(ctx, ThermalsGeoKey, &redis.GeoLocation{
		Name:      thermal.ID,
		Latitude:  thermal.Center.Latitude,
		Longitude: thermal.Center.Longitude,
	})

	// Сохраняем детальные данные
	thermalKey := ThermalPrefix + thermal.ID
	thermalData, err := json.Marshal(thermal)
	if err != nil {
		return fmt.Errorf("failed to marshal thermal data: %w", err)
	}

	pipe.Set(ctx, thermalKey, thermalData, ThermalTTL)
	pipe.Expire(ctx, ThermalsGeoKey, ThermalTTL)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to save thermal: %w", err)
	}

	r.logger.Debug("Saved thermal to Redis", 
		"thermal_id", thermal.ID,
		"lat", thermal.Center.Latitude,
		"lon", thermal.Center.Longitude)

	return nil
}

// GetThermalsInRadius возвращает термики в указанном радиусе
func (r *RedisRepository) GetThermalsInRadius(ctx context.Context, center models.GeoPoint, radiusKM float64) ([]*models.Thermal, error) {
	locations, err := r.client.GeoRadius(ctx, ThermalsGeoKey, center.Longitude, center.Latitude, &redis.GeoRadiusQuery{
		Radius:    radiusKM,
		Unit:      "km",
		WithCoord: true,
		WithDist:  true,
		Count:     500, // Максимум 500 термиков
		Sort:      "ASC",
	}).Result()

	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to get thermals in radius: %w", err)
	}

	if len(locations) == 0 {
		return []*models.Thermal{}, nil
	}

	// Получаем детальные данные
	pipe := r.client.Pipeline()
	cmds := make([]*redis.StringCmd, len(locations))
	
	for i, loc := range locations {
		thermalKey := ThermalPrefix + loc.Name
		cmds[i] = pipe.Get(ctx, thermalKey)
	}

	_, err = pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to get thermal details: %w", err)
	}

	thermals := make([]*models.Thermal, 0, len(locations))
	for i, cmd := range cmds {
		if cmd.Err() == redis.Nil {
			continue
		}
		if cmd.Err() != nil {
			r.logger.Warn("Failed to get thermal data", "thermal_id", locations[i].Name, "error", cmd.Err())
			continue
		}

		var thermal models.Thermal
		if err := json.Unmarshal([]byte(cmd.Val()), &thermal); err != nil {
			r.logger.Warn("Failed to unmarshal thermal data", "thermal_id", locations[i].Name, "error", err)
			continue
		}

		thermal.DistanceKM = locations[i].Dist
		thermals = append(thermals, &thermal)
	}

	return thermals, nil
}

// SaveStation сохраняет данные метеостанции
func (r *RedisRepository) SaveStation(ctx context.Context, station *models.Station) error {
	if station == nil {
		return fmt.Errorf("station cannot be nil")
	}

	pipe := r.client.Pipeline()

	pipe.GeoAdd(ctx, StationsGeoKey, &redis.GeoLocation{
		Name:      station.ID,
		Latitude:  station.Position.Latitude,
		Longitude: station.Position.Longitude,
	})

	stationKey := StationPrefix + station.ID
	stationData, err := json.Marshal(station)
	if err != nil {
		return fmt.Errorf("failed to marshal station data: %w", err)
	}

	pipe.Set(ctx, stationKey, stationData, StationTTL)
	pipe.Expire(ctx, StationsGeoKey, StationTTL)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to save station: %w", err)
	}

	return nil
}

// GetStationsInRadius возвращает метеостанции в указанном радиусе
func (r *RedisRepository) GetStationsInRadius(ctx context.Context, center models.GeoPoint, radiusKM float64) ([]*models.Station, error) {
	locations, err := r.client.GeoRadius(ctx, StationsGeoKey, center.Longitude, center.Latitude, &redis.GeoRadiusQuery{
		Radius:    radiusKM,
		Unit:      "km",
		WithCoord: true,
		WithDist:  true,
		Count:     100, // Максимум 100 станций
		Sort:      "ASC",
	}).Result()

	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to get stations in radius: %w", err)
	}

	if len(locations) == 0 {
		return []*models.Station{}, nil
	}

	pipe := r.client.Pipeline()
	cmds := make([]*redis.StringCmd, len(locations))
	
	for i, loc := range locations {
		stationKey := StationPrefix + loc.Name
		cmds[i] = pipe.Get(ctx, stationKey)
	}

	_, err = pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to get station details: %w", err)
	}

	stations := make([]*models.Station, 0, len(locations))
	for i, cmd := range cmds {
		if cmd.Err() == redis.Nil {
			continue
		}
		if cmd.Err() != nil {
			r.logger.Warn("Failed to get station data", "station_id", locations[i].Name, "error", cmd.Err())
			continue
		}

		var station models.Station
		if err := json.Unmarshal([]byte(cmd.Val()), &station); err != nil {
			r.logger.Warn("Failed to unmarshal station data", "station_id", locations[i].Name, "error", err)
			continue
		}

		station.DistanceKM = locations[i].Dist
		stations = append(stations, &station)
	}

	return stations, nil
}

// DeletePilot удаляет пилота
func (r *RedisRepository) DeletePilot(ctx context.Context, deviceID string) error {
	pipe := r.client.Pipeline()
	
	// Удаляем из геопространственного индекса
	pipe.ZRem(ctx, PilotsGeoKey, deviceID)
	
	// Удаляем детальные данные
	pilotKey := PilotPrefix + deviceID
	pipe.Del(ctx, pilotKey)
	
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete pilot: %w", err)
	}
	
	return nil
}

// GetStats возвращает статистику Redis
func (r *RedisRepository) GetStats(ctx context.Context) (map[string]interface{}, error) {
	pipe := r.client.Pipeline()
	
	pilotsCountCmd := pipe.ZCard(ctx, PilotsGeoKey)
	thermalsCountCmd := pipe.ZCard(ctx, ThermalsGeoKey)
	stationsCountCmd := pipe.ZCard(ctx, StationsGeoKey)
	infoCmd := pipe.Info(ctx, "memory")
	
	_, err := pipe.Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get Redis stats: %w", err)
	}
	
	stats := map[string]interface{}{
		"pilots_count":   pilotsCountCmd.Val(),
		"thermals_count": thermalsCountCmd.Val(),
		"stations_count": stationsCountCmd.Val(),
		"memory_info":    infoCmd.Val(),
	}
	
	return stats, nil
}

// CleanupExpired удаляет устаревшие записи из геопространственных индексов
func (r *RedisRepository) CleanupExpired(ctx context.Context) error {
	// Эта операция выполняется периодически для удаления записей
	// из гео-индексов, когда основные ключи уже истекли
	
	// Получаем все записи из каждого гео-индекса и проверяем существование основных ключей
	
	pilots, err := r.client.ZRange(ctx, PilotsGeoKey, 0, -1).Result()
	if err != nil && err != redis.Nil {
		return fmt.Errorf("failed to get pilots for cleanup: %w", err)
	}
	
	pipe := r.client.Pipeline()
	cleanupCount := 0
	
	for _, deviceID := range pilots {
		pilotKey := PilotPrefix + deviceID
		exists := r.client.Exists(ctx, pilotKey)
		if exists.Val() == 0 {
			pipe.ZRem(ctx, PilotsGeoKey, deviceID)
			cleanupCount++
		}
	}
	
	if cleanupCount > 0 {
		_, err = pipe.Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to cleanup expired pilots: %w", err)
		}
		
		r.logger.Info("Cleaned up expired pilots", "count", cleanupCount)
	}
	
	// Аналогично для термиков и станций...
	
	return nil
}