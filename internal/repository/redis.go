package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/flybeeper/fanet-backend/internal/config"
	"github.com/flybeeper/fanet-backend/internal/metrics"
	"github.com/flybeeper/fanet-backend/internal/models"
	"github.com/flybeeper/fanet-backend/pkg/utils"
)

const (
	// Ключи для геопространственных индексов
	PilotsGeoKey        = "pilots:geo"        // GEO индекс для пилотов
	ThermalsGeoKey      = "thermals:geo"      // GEO индекс для термиков
	StationsGeoKey      = "stations:geo"      // GEO индекс для метеостанций
	GroundObjectsGeoKey = "ground_objects:geo" // GEO индекс для наземных объектов
	
	// Дополнительные индексы
	ThermalsTimeKey = "thermals:time" // Z-SET индекс термиков по времени
	
	// Префиксы для хешей с детальными данными
	PilotPrefix        = "pilot:"         // pilot:{addr}
	ThermalPrefix      = "thermal:"       // thermal:{id}
	StationPrefix      = "station:"       // station:{addr}
	GroundObjectPrefix = "ground:"        // ground:{addr}
	TrackPrefix        = "track:"         // track:{addr} - список точек трека
	
	// Префиксы для клиентов и подписок
	ClientPrefix        = "client:"         // client:{id}
	ClientRegionsPrefix = "client:%s:regions" // client:{id}:regions
	UpdatesPrefix       = "updates:"        // updates:{geohash}
	
	// Кэш аутентификации
	AuthTokenPrefix = "auth:token:"   // auth:token:{token_hash}
	
	// Счетчики и статистика
	SequenceGlobal = "sequence:global"  // Глобальный счетчик
	StatsPrefix    = "stats:"          // stats:{metric}
	
	// TTL для данных (согласно спецификации)
	PilotTTL        = 12 * time.Hour     // 43200 секунд
	ThermalTTL      = 6 * time.Hour      // 21600 секунд
	StationTTL      = 24 * time.Hour     // 86400 секунд
	GroundObjectTTL = 4 * time.Hour      // 14400 секунд
	ClientTTL       = 5 * time.Minute    // 300 секунд
	AuthTokenTTL    = 1 * time.Hour      // 3600 секунд
	
	// Настройки для списков
	MaxTrackPoints    = 999          // Максимум точек в треке
	MaxStationHistory = 287          // 24 часа с 5-мин интервалом
	MaxUpdatesQueue   = 99           // Максимум обновлений в очереди
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

// GetClient возвращает Redis клиент для внешнего использования (например, для auth кеширования)
func (r *RedisRepository) GetClient() *redis.Client {
	return r.client
}

// SavePilot сохраняет данные пилота согласно Redis схеме
func (r *RedisRepository) SavePilot(ctx context.Context, pilot *models.Pilot) error {
	if pilot == nil {
		return fmt.Errorf("pilot cannot be nil")
	}

	start := time.Now()
	pipe := r.client.Pipeline()

	// Сохраняем в геопространственный индекс только если координаты валидны
	// Redis GEO ограничения: lat [-85.05112878, 85.05112878], lon [-180, 180]
	if pilot.Position != nil && 
		pilot.Position.Latitude != 0 && pilot.Position.Longitude != 0 &&
		pilot.Position.Latitude >= -85.05112878 && pilot.Position.Latitude <= 85.05112878 &&
		pilot.Position.Longitude >= -180 && pilot.Position.Longitude <= 180 &&
		!math.IsNaN(pilot.Position.Latitude) && !math.IsNaN(pilot.Position.Longitude) &&
		!math.IsInf(pilot.Position.Latitude, 0) && !math.IsInf(pilot.Position.Longitude, 0) {
		
		pipe.GeoAdd(ctx, PilotsGeoKey, &redis.GeoLocation{
			Name:      fmt.Sprintf("pilot:%s", pilot.DeviceID),
			Latitude:  pilot.Position.Latitude,
			Longitude: pilot.Position.Longitude,
		})
	} else {
		// Детальная диагностика проблем с координатами пилота
		var reason string
		if pilot.Position == nil {
			reason = "position is nil"
		} else if pilot.Position.Latitude == 0 && pilot.Position.Longitude == 0 {
			reason = "coordinates are zero"
		} else if pilot.Position.Latitude < -85.05112878 || pilot.Position.Latitude > 85.05112878 {
			reason = fmt.Sprintf("latitude out of Redis GEO range [-85.05112878, 85.05112878]: %f", pilot.Position.Latitude)
		} else if pilot.Position.Longitude < -180 || pilot.Position.Longitude > 180 {
			reason = fmt.Sprintf("longitude out of range [-180, 180]: %f", pilot.Position.Longitude)
		} else if math.IsNaN(pilot.Position.Latitude) || math.IsNaN(pilot.Position.Longitude) {
			reason = "coordinates contain NaN values"
		} else if math.IsInf(pilot.Position.Latitude, 0) || math.IsInf(pilot.Position.Longitude, 0) {
			reason = "coordinates contain Inf values"
		} else {
			reason = "unknown validation failure"
		}
		
		r.logger.WithField("device_id", pilot.DeviceID).
			WithField("lat", pilot.Position.Latitude).
			WithField("lon", pilot.Position.Longitude).
			WithField("reason", reason).
			Warn("Skipping GEO indexing for pilot with invalid coordinates")
	}

	// Сохраняем детальные данные в HSET согласно спецификации
	pilotKey := PilotPrefix + pilot.DeviceID
	pilotData := map[string]interface{}{
		"name":         pilot.Name,
		"type":         uint8(pilot.Type), // Явно конвертируем PilotType в uint8 для Redis
		"altitude":     pilot.Position.Altitude,
		"speed":        pilot.Speed,
		"climb":        pilot.ClimbRate,
		"course":       pilot.Heading,
		"last_update":  pilot.LastUpdate.Unix(),
		"track_online": pilot.TrackOnline,
		"battery":      pilot.Battery,
		"latitude":     pilot.Position.Latitude,
		"longitude":    pilot.Position.Longitude,
	}
	
	// Добавляем дополнительные поля для отслеживания границ если они есть
	if pilot.LastMovement != nil && !pilot.LastMovement.IsZero() {
		pilotData["last_movement"] = pilot.LastMovement.Unix()
	}
	if pilot.TrackingDistance >= 0 {
		pilotData["tracking_distance"] = pilot.TrackingDistance
	}
	if pilot.VisibilityStatus != "" {
		pilotData["visibility_status"] = pilot.VisibilityStatus
	}
	
	pipe.HSet(ctx, pilotKey, pilotData)

	// Устанавливаем TTL
	pipe.Expire(ctx, pilotKey, PilotTTL)

	// Сохраняем точку трека если есть
	if pilot.Position.Latitude != 0 && pilot.Position.Longitude != 0 {
		trackKey := TrackPrefix + pilot.DeviceID
		
		// Сериализуем позицию в protobuf для экономии места
		positionData, err := json.Marshal(map[string]interface{}{
			"lat": pilot.Position.Latitude,
			"lon": pilot.Position.Longitude,
			"alt": pilot.Position.Altitude,
			"ts":  pilot.LastUpdate.Unix(),
		})
		if err == nil {
			pipe.LPush(ctx, trackKey, positionData)
			pipe.LTrim(ctx, trackKey, 0, MaxTrackPoints)
			pipe.Expire(ctx, trackKey, PilotTTL)
		}
	}

	// Выполняем все операции в батче
	_, err := pipe.Exec(ctx)
	if err != nil {
		metrics.RedisOperationErrors.WithLabelValues("save_pilot").Inc()
		return fmt.Errorf("failed to save pilot: %w", err)
	}

	// Обновляем статистику
	r.client.Incr(ctx, StatsPrefix+"pilots:updates")

	r.logger.WithFields(map[string]interface{}{
		"device_id": pilot.DeviceID,
		"lat": pilot.Position.Latitude,
		"lon": pilot.Position.Longitude,
		"pilot_type": pilot.Type,
		"pilot_type_uint8": uint8(pilot.Type),
	}).Debug("Saved pilot to Redis")

	// Записываем метрики
	duration := time.Since(start).Seconds()
	metrics.RedisOperationDuration.WithLabelValues("save_pilot").Observe(duration)
	
	return nil
}

// UpdatePilotName обновляет имя пилота в Redis
func (r *RedisRepository) UpdatePilotName(ctx context.Context, deviceID string, name string) error {
	if deviceID == "" {
		return fmt.Errorf("device ID cannot be empty")
	}

	pilotKey := PilotPrefix + deviceID
	
	// Проверяем существование пилота
	exists := r.client.Exists(ctx, pilotKey)
	if exists.Val() == 0 {
		// Пилот не существует, создаем минимальную запись
		pipe := r.client.Pipeline()
		pipe.HSet(ctx, pilotKey, map[string]interface{}{
			"name":        name,
			"last_update": time.Now().Unix(),
		})
		pipe.Expire(ctx, pilotKey, PilotTTL)
		
		_, err := pipe.Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to create pilot name record: %w", err)
		}
		
		r.logger.WithField("device_id", deviceID).WithField("name", name).Debug("Created new pilot name record in Redis")
		return nil
	}

	// Обновляем существующего пилота
	result := r.client.HSet(ctx, pilotKey, "name", name)
	if result.Err() != nil {
		return fmt.Errorf("failed to update pilot name: %w", result.Err())
	}

	r.logger.WithField("device_id", deviceID).WithField("name", name).Debug("Updated pilot name in Redis")
	return nil
}

// RemovePilot удаляет пилота из всех ключей Redis
func (r *RedisRepository) RemovePilot(ctx context.Context, deviceID string) error {
	if deviceID == "" {
		return fmt.Errorf("device ID cannot be empty")
	}

	start := time.Now()
	pipe := r.client.Pipeline()

	// Удаляем из геопространственного индекса
	geoMember := fmt.Sprintf("pilot:%s", deviceID)
	pipe.ZRem(ctx, PilotsGeoKey, geoMember)

	// Удаляем детальные данные
	pilotKey := PilotPrefix + deviceID
	pipe.Del(ctx, pilotKey)

	// Удаляем данные трека
	trackKey := TrackPrefix + deviceID
	pipe.Del(ctx, trackKey)

	// Выполняем все операции
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to remove pilot from Redis: %w", err)
	}

	elapsed := time.Since(start)
	r.logger.WithField("device_id", deviceID).
		WithField("elapsed_ms", elapsed.Milliseconds()).
		Debug("Successfully removed pilot from Redis")

	return nil
}

// GetPilotsInRadius возвращает пилотов в указанном радиусе
func (r *RedisRepository) GetPilotsInRadius(ctx context.Context, center models.GeoPoint, radiusKM float64) ([]*models.Pilot, error) {
	start := time.Now()
	
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

	// Получаем детальные данные пилотов из HSET батчем
	pipe := r.client.Pipeline()
	cmds := make([]*redis.MapStringStringCmd, len(locations))
	
	for i, loc := range locations {
		// Извлекаем device_id из имени (pilot:device_id -> device_id)
		deviceID := loc.Name
		if strings.HasPrefix(loc.Name, "pilot:") {
			deviceID = strings.TrimPrefix(loc.Name, "pilot:")
		}
		
		pilotKey := PilotPrefix + deviceID
		cmds[i] = pipe.HGetAll(ctx, pilotKey)
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
			r.logger.WithFields(map[string]interface{}{
				"device_id": locations[i].Name,
				"error": cmd.Err(),
			}).Warn("Failed to get pilot data")
			continue
		}

		data := cmd.Val()
		if len(data) == 0 {
			continue // Пустые данные
		}

		// Извлекаем device_id
		deviceID := locations[i].Name
		if strings.HasPrefix(locations[i].Name, "pilot:") {
			deviceID = strings.TrimPrefix(locations[i].Name, "pilot:")
		}

		// Конвертируем HSET данные в модель пилота
		pilot, err := r.mapToPilot(deviceID, data, &locations[i])
		if err != nil {
			r.logger.WithFields(map[string]interface{}{
				"device_id": deviceID,
				"error": err,
			}).Warn("Failed to map pilot data")
			continue
		}

		pilots = append(pilots, pilot)
	}

	r.logger.WithFields(map[string]interface{}{
		"center_lat": center.Latitude,
		"center_lon": center.Longitude,
		"radius_km": radiusKM,
		"found": len(pilots),
	}).Debug("Retrieved pilots in radius")

	// Записываем метрики
	duration := time.Since(start).Seconds()
	metrics.RedisOperationDuration.WithLabelValues("get_pilots_radius").Observe(duration)
	
	return pilots, nil
}

// GetPilot возвращает данные одного пилота по device ID
func (r *RedisRepository) GetPilot(ctx context.Context, deviceID string) (*models.Pilot, error) {
	if deviceID == "" {
		return nil, fmt.Errorf("device ID cannot be empty")
	}

	start := time.Now()
	pilotKey := PilotPrefix + deviceID

	// Получаем данные пилота из HSET
	data, err := r.client.HGetAll(ctx, pilotKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // Пилот не найден
		}
		return nil, fmt.Errorf("failed to get pilot data: %w", err)
	}

	if len(data) == 0 {
		return nil, nil // Пилот не найден
	}

	// Получаем координаты из сохраненных полей
	var lat, lon float64
	if latStr, ok := data["latitude"]; ok {
		lat, _ = strconv.ParseFloat(latStr, 64)
	}
	if lonStr, ok := data["longitude"]; ok {
		lon, _ = strconv.ParseFloat(lonStr, 64)
	}

	// Создаем GeoLocation для обратной совместимости с mapToPilot
	location := &redis.GeoLocation{
		Latitude:  lat,
		Longitude: lon,
	}

	// Конвертируем в модель пилота
	pilot, err := r.mapToPilot(deviceID, data, location)
	if err != nil {
		return nil, fmt.Errorf("failed to map pilot data: %w", err)
	}

	// Записываем метрики
	duration := time.Since(start).Seconds()
	metrics.RedisOperationDuration.WithLabelValues("get_pilot").Observe(duration)

	return pilot, nil
}

// SaveThermal сохраняет данные термика
func (r *RedisRepository) SaveThermal(ctx context.Context, thermal *models.Thermal) error {
	if thermal == nil {
		return fmt.Errorf("thermal cannot be nil")
	}

	start := time.Now()
	pipe := r.client.Pipeline()

	// Сохраняем в геопространственный индекс только если координаты валидны
	// Redis GEO ограничения: lat [-85.05112878, 85.05112878], lon [-180, 180]
	if thermal.Position.Latitude != 0 && thermal.Position.Longitude != 0 &&
		thermal.Position.Latitude >= -85.05112878 && thermal.Position.Latitude <= 85.05112878 &&
		thermal.Position.Longitude >= -180 && thermal.Position.Longitude <= 180 &&
		!math.IsNaN(thermal.Position.Latitude) && !math.IsNaN(thermal.Position.Longitude) &&
		!math.IsInf(thermal.Position.Latitude, 0) && !math.IsInf(thermal.Position.Longitude, 0) {
		
		pipe.GeoAdd(ctx, ThermalsGeoKey, &redis.GeoLocation{
			Name:      thermal.ID,
			Latitude:  thermal.Position.Latitude,
			Longitude: thermal.Position.Longitude,
		})
	} else {
		// Детальная диагностика проблем с координатами термика
		var reason string
		if thermal.Position.Latitude == 0 && thermal.Position.Longitude == 0 {
			reason = "coordinates are zero"
		} else if thermal.Position.Latitude < -85.05112878 || thermal.Position.Latitude > 85.05112878 {
			reason = fmt.Sprintf("latitude out of Redis GEO range [-85.05112878, 85.05112878]: %f", thermal.Position.Latitude)
		} else if thermal.Position.Longitude < -180 || thermal.Position.Longitude > 180 {
			reason = fmt.Sprintf("longitude out of range [-180, 180]: %f", thermal.Position.Longitude)
		} else if math.IsNaN(thermal.Position.Latitude) || math.IsNaN(thermal.Position.Longitude) {
			reason = "coordinates contain NaN values"
		} else if math.IsInf(thermal.Position.Latitude, 0) || math.IsInf(thermal.Position.Longitude, 0) {
			reason = "coordinates contain Inf values"
		} else {
			reason = "unknown validation failure"
		}
		
		r.logger.WithField("thermal_id", thermal.ID).
			WithField("lat", thermal.Position.Latitude).
			WithField("lon", thermal.Position.Longitude).
			WithField("reason", reason).
			Warn("Skipping GEO indexing for thermal with invalid coordinates")
	}

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

	r.logger.WithField("thermal_id", thermal.ID).WithField("lat", thermal.Position.Latitude).WithField("lon", thermal.Position.Longitude).Debug("Saved thermal to Redis")

	// Записываем метрики
	duration := time.Since(start).Seconds()
	metrics.RedisOperationDuration.WithLabelValues("save_thermal").Observe(duration)
	
	return nil
}

// GetThermalsInRadius возвращает термики в указанном радиусе
func (r *RedisRepository) GetThermalsInRadius(ctx context.Context, center models.GeoPoint, radiusKM float64) ([]*models.Thermal, error) {
	start := time.Now()
	
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
			r.logger.WithFields(map[string]interface{}{
				"thermal_id": locations[i].Name,
				"error": cmd.Err(),
			}).Warn("Failed to get thermal data")
			continue
		}

		var thermal models.Thermal
		if err := json.Unmarshal([]byte(cmd.Val()), &thermal); err != nil {
			r.logger.WithFields(map[string]interface{}{
				"thermal_id": locations[i].Name,
				"error": err,
			}).Warn("Failed to unmarshal thermal data")
			continue
		}

		thermals = append(thermals, &thermal)
	}

	// Записываем метрики
	duration := time.Since(start).Seconds()
	metrics.RedisOperationDuration.WithLabelValues("get_thermals_radius").Observe(duration)
	
	return thermals, nil
}

// SaveStation сохраняет данные метеостанции
func (r *RedisRepository) SaveStation(ctx context.Context, station *models.Station) error {
	if station == nil {
		return fmt.Errorf("station cannot be nil")
	}

	pipe := r.client.Pipeline()

	// Добавляем в GEO индекс только если координаты валидны
	// Redis GEO ограничения: lat [-85.05112878, 85.05112878], lon [-180, 180]
	if station.Position != nil && 
		station.Position.Latitude != 0 && station.Position.Longitude != 0 &&
		station.Position.Latitude >= -85.05112878 && station.Position.Latitude <= 85.05112878 &&
		station.Position.Longitude >= -180 && station.Position.Longitude <= 180 &&
		!math.IsNaN(station.Position.Latitude) && !math.IsNaN(station.Position.Longitude) &&
		!math.IsInf(station.Position.Latitude, 0) && !math.IsInf(station.Position.Longitude, 0) {
		
		pipe.GeoAdd(ctx, StationsGeoKey, &redis.GeoLocation{
			Name:      station.ID,
			Latitude:  station.Position.Latitude,
			Longitude: station.Position.Longitude,
		})
		pipe.Expire(ctx, StationsGeoKey, StationTTL)
	} else {
		// Детальная диагностика проблем с координатами
		var reason string
		if station.Position == nil {
			reason = "position is nil"
		} else if station.Position.Latitude == 0 && station.Position.Longitude == 0 {
			reason = "coordinates are zero"
		} else if station.Position.Latitude < -85.05112878 || station.Position.Latitude > 85.05112878 {
			reason = fmt.Sprintf("latitude out of Redis GEO range [-85.05112878, 85.05112878]: %f", station.Position.Latitude)
		} else if station.Position.Longitude < -180 || station.Position.Longitude > 180 {
			reason = fmt.Sprintf("longitude out of range [-180, 180]: %f", station.Position.Longitude)
		} else if math.IsNaN(station.Position.Latitude) || math.IsNaN(station.Position.Longitude) {
			reason = "coordinates contain NaN values"
		} else if math.IsInf(station.Position.Latitude, 0) || math.IsInf(station.Position.Longitude, 0) {
			reason = "coordinates contain Inf values"
		} else {
			reason = "unknown validation failure"
		}
		
		r.logger.WithField("station_id", station.ID).
			WithField("lat", station.Position.Latitude).
			WithField("lon", station.Position.Longitude).
			WithField("reason", reason).
			Warn("Skipping GEO indexing for station with invalid coordinates")
	}

	stationKey := StationPrefix + station.ID
	stationData, err := json.Marshal(station)
	if err != nil {
		return fmt.Errorf("failed to marshal station data: %w", err)
	}

	pipe.Set(ctx, stationKey, stationData, StationTTL)

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
			r.logger.WithFields(map[string]interface{}{
				"station_id": locations[i].Name,
				"error": cmd.Err(),
			}).Warn("Failed to get station data")
			continue
		}

		var station models.Station
		if err := json.Unmarshal([]byte(cmd.Val()), &station); err != nil {
			r.logger.WithFields(map[string]interface{}{
				"station_id": locations[i].Name,
				"error": err,
			}).Warn("Failed to unmarshal station data")
			continue
		}

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

// mapToPilot конвертирует HSET данные в модель пилота
func (r *RedisRepository) mapToPilot(deviceID string, data map[string]string, location *redis.GeoLocation) (*models.Pilot, error) {
	pilot := &models.Pilot{
		DeviceID: deviceID,
		Position: &models.GeoPoint{
			Latitude:  location.Latitude,
			Longitude: location.Longitude,
		},
	}

	// Парсим строковые значения из Redis HSET
	if name, ok := data["name"]; ok {
		pilot.Name = name
	}

	if typeStr, ok := data["type"]; ok {
		if t, err := r.parseRedisUint8(typeStr, "pilot_type", deviceID); err == nil {
			pilot.Type = models.PilotType(t)
			r.logger.WithFields(map[string]interface{}{
				"device_id": deviceID,
				"type_string": typeStr,
				"type_hex": fmt.Sprintf("%x", []byte(typeStr)),
				"type_parsed": t,
				"pilot_type": pilot.Type,
			}).Debug("Parsed pilot type from Redis")
		} else {
			r.logger.WithFields(map[string]interface{}{
				"device_id": deviceID,
				"type_string": typeStr,
				"type_hex": fmt.Sprintf("%x", []byte(typeStr)),
				"parse_error": err,
			}).Warn("Failed to parse pilot type from Redis")
		}
	}

	if altStr, ok := data["altitude"]; ok {
		if alt, err := strconv.Atoi(altStr); err == nil {
			pilot.Position.Altitude = int32(alt)
		}
	}

	if speedStr, ok := data["speed"]; ok {
		if speed, err := strconv.ParseFloat(speedStr, 64); err == nil {
			pilot.Speed = float32(speed)
		}
	}

	if climbStr, ok := data["climb"]; ok {
		if climb, err := strconv.ParseFloat(climbStr, 64); err == nil {
			pilot.ClimbRate = int16(climb)
		}
	}

	if courseStr, ok := data["course"]; ok {
		if course, err := strconv.ParseFloat(courseStr, 64); err == nil {
			pilot.Heading = float32(course)
		}
	}

	if updateStr, ok := data["last_update"]; ok {
		if timestamp, err := strconv.ParseInt(updateStr, 10, 64); err == nil {
			pilot.LastUpdate = time.Unix(timestamp, 0)
		}
	}

	if onlineStr, ok := data["track_online"]; ok {
		pilot.TrackOnline = onlineStr == "1" || onlineStr == "true"
	}

	if batteryStr, ok := data["battery"]; ok {
		if battery, err := r.parseRedisUint8(batteryStr, "battery", deviceID); err == nil {
			pilot.Battery = battery
		} else {
			r.logger.WithFields(map[string]interface{}{
				"device_id": deviceID,
				"battery_string": batteryStr,
				"parse_error": err,
			}).Debug("Failed to parse battery from Redis")
		}
	}

	// Парсим поля для отслеживания границ
	if lastMovementStr, ok := data["last_movement"]; ok {
		if timestamp, err := strconv.ParseInt(lastMovementStr, 10, 64); err == nil {
			t := time.Unix(timestamp, 0)
			pilot.LastMovement = &t
		}
	}

	if trackingDistStr, ok := data["tracking_distance"]; ok {
		if dist, err := strconv.ParseFloat(trackingDistStr, 64); err == nil {
			pilot.TrackingDistance = dist
		}
	}

	if visStatus, ok := data["visibility_status"]; ok {
		pilot.VisibilityStatus = visStatus
	}

	return pilot, nil
}

// mapToThermal конвертирует HSET данные в модель термика
func (r *RedisRepository) mapToThermal(thermalID string, data map[string]string, location *redis.GeoLocation) (*models.Thermal, error) {
	thermal := &models.Thermal{
		ID: thermalID,
		Position: &models.GeoPoint{
			Latitude:  location.Latitude,
			Longitude: location.Longitude,
		},
	}

	if addrStr, ok := data["addr"]; ok {
		if addr, err := strconv.Atoi(addrStr); err == nil {
			thermal.ReportedBy = fmt.Sprintf("%06X", addr)
		}
	}

	if altStr, ok := data["altitude"]; ok {
		if alt, err := strconv.Atoi(altStr); err == nil {
			thermal.Position.Altitude = int32(alt)
		}
	}

	if qualityStr, ok := data["quality"]; ok {
		if quality, err := strconv.Atoi(qualityStr); err == nil {
			thermal.Quality = int32(quality)
		}
	}

	if climbStr, ok := data["climb"]; ok {
		if climb, err := strconv.ParseFloat(climbStr, 64); err == nil {
			thermal.ClimbRate = float32(climb) // м/с -> м/с * 10
		}
	}

	if windSpeedStr, ok := data["wind_speed"]; ok {
		if windSpeed, err := strconv.ParseFloat(windSpeedStr, 64); err == nil {
			thermal.WindSpeed = uint8(windSpeed * 3.6) // м/с -> км/ч
		}
	}

	if windHeadingStr, ok := data["wind_heading"]; ok {
		if windHeading, err := strconv.ParseFloat(windHeadingStr, 64); err == nil {
			thermal.WindDirection = uint16(windHeading)
		}
	}

	if timestampStr, ok := data["timestamp"]; ok {
		if timestamp, err := strconv.ParseInt(timestampStr, 10, 64); err == nil {
			thermal.Timestamp = time.Unix(timestamp, 0)
		}
	}

	return thermal, nil
}

// mapToStation конвертирует HSET данные в модель станции
func (r *RedisRepository) mapToStation(stationID string, data map[string]string, location *redis.GeoLocation) (*models.Station, error) {
	station := &models.Station{
		ID: stationID,
		Position: &models.GeoPoint{
			Latitude:  location.Latitude,
			Longitude: location.Longitude,
		},
	}

	if name, ok := data["name"]; ok {
		station.Name = name
	}

	if tempStr, ok := data["temperature"]; ok {
		if temp, err := strconv.ParseFloat(tempStr, 64); err == nil {
			station.Temperature = int8(temp)
		}
	}

	if windSpeedStr, ok := data["wind_speed"]; ok {
		if windSpeed, err := strconv.ParseFloat(windSpeedStr, 64); err == nil {
			station.WindSpeed = uint8(windSpeed * 3.6) // м/с -> км/ч
		}
	}

	if windHeadingStr, ok := data["wind_heading"]; ok {
		if windHeading, err := strconv.ParseFloat(windHeadingStr, 64); err == nil {
			station.WindDirection = uint16(windHeading)
		}
	}

	if windGustsStr, ok := data["wind_gusts"]; ok {
		if windGusts, err := strconv.ParseFloat(windGustsStr, 64); err == nil {
			station.WindGusts = uint8(windGusts * 3.6) // м/с -> км/ч
		}
	}

	if humidityStr, ok := data["humidity"]; ok {
		if humidity, err := r.parseRedisUint8(humidityStr, "humidity", stationID); err == nil {
			station.Humidity = humidity
		}
	}

	if pressureStr, ok := data["pressure"]; ok {
		if pressure, err := strconv.ParseFloat(pressureStr, 64); err == nil {
			station.Pressure = uint16(pressure)
		}
	}

	if batteryStr, ok := data["battery"]; ok {
		if battery, err := r.parseRedisUint8(batteryStr, "battery", stationID); err == nil {
			station.Battery = battery
		}
	}

	if updateStr, ok := data["last_update"]; ok {
		if timestamp, err := strconv.ParseInt(updateStr, 10, 64); err == nil {
			station.LastUpdate = time.Unix(timestamp, 0)
		}
	}

	return station, nil
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
		
		r.logger.WithField("count", cleanupCount).Info("Cleaned up expired pilots")
	}
	
	// Аналогично для термиков и станций...
	
	return nil
}

// GetAllStations возвращает все станции (для snapshot без географической фильтрации)
func (r *RedisRepository) GetAllStations(ctx context.Context) ([]*models.Station, error) {
	start := time.Now()
	defer func() {
		duration := time.Since(start).Seconds()
		metrics.RedisOperationDuration.WithLabelValues("get_all_stations").Observe(duration)
	}()

	// Получаем все ключи станций
	pattern := StationPrefix + "*"
	keys, err := r.client.Keys(ctx, pattern).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get station keys: %w", err)
	}

	if len(keys) == 0 {
		return []*models.Station{}, nil
	}

	// Получаем данные всех станций batch запросом
	pipe := r.client.Pipeline()
	cmds := make([]*redis.StringCmd, len(keys))
	
	for i, key := range keys {
		cmds[i] = pipe.Get(ctx, key)
	}

	_, err = pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to get station details: %w", err)
	}

	stations := make([]*models.Station, 0, len(keys))
	for _, cmd := range cmds {
		if cmd.Err() == redis.Nil {
			continue
		}
		if cmd.Err() != nil {
			continue // Пропускаем ошибочные записи
		}

		var station models.Station
		if err := json.Unmarshal([]byte(cmd.Val()), &station); err != nil {
			r.logger.WithField("error", err).Warn("Failed to unmarshal station data")
			continue
		}

		stations = append(stations, &station)
	}

	r.logger.WithField("count", len(stations)).Debug("Retrieved all stations")
	return stations, nil
}

// parseRedisUint8 безопасно парсит uint8 значение из Redis, которое может быть сохранено как строка или байт
func (r *RedisRepository) parseRedisUint8(value string, fieldName string, deviceID string) (uint8, error) {
	// Сначала пробуем парсить как обычную строку с числом
	if t, err := strconv.Atoi(value); err == nil {
		if t >= 0 && t <= 255 {
			return uint8(t), nil
		}
		return 0, fmt.Errorf("value %d out of uint8 range [0, 255]", t)
	}
	
	// Если обычный парсинг не сработал, проверяем на байтовое представление
	if len(value) == 1 {
		byteVal := uint8(value[0])
		r.logger.WithFields(map[string]interface{}{
			"device_id": deviceID,
			"field": fieldName,
			"byte_value": byteVal,
			"hex_value": fmt.Sprintf("0x%02x", byteVal),
		}).Debug("Parsed Redis uint8 from byte representation")
		return byteVal, nil
	}
	
	// Если это многобайтовая строка, логируем для отладки
	r.logger.WithFields(map[string]interface{}{
		"device_id": deviceID,
		"field": fieldName,
		"value_length": len(value),
		"value_bytes": fmt.Sprintf("%x", []byte(value)),
		"first_byte": fmt.Sprintf("0x%02x", value[0]),
	}).Debug("Redis uint8 parsing debug info")
	
	return 0, fmt.Errorf("unable to parse '%s' as uint8: not a valid number string or single byte", value)
}

// SaveGroundObject сохраняет данные наземного объекта в Redis
func (r *RedisRepository) SaveGroundObject(ctx context.Context, groundObject *models.GroundObject) error {
	if groundObject == nil {
		return fmt.Errorf("ground object cannot be nil")
	}

	start := time.Now()
	pipe := r.client.Pipeline()

	// Сохраняем в геопространственный индекс только если координаты валидны
	if groundObject.Position != nil && 
		groundObject.Position.Latitude != 0 && groundObject.Position.Longitude != 0 &&
		groundObject.Position.Latitude >= -85.05112878 && groundObject.Position.Latitude <= 85.05112878 &&
		groundObject.Position.Longitude >= -180 && groundObject.Position.Longitude <= 180 &&
		!math.IsNaN(groundObject.Position.Latitude) && !math.IsNaN(groundObject.Position.Longitude) &&
		!math.IsInf(groundObject.Position.Latitude, 0) && !math.IsInf(groundObject.Position.Longitude, 0) {
		
		pipe.GeoAdd(ctx, GroundObjectsGeoKey, &redis.GeoLocation{
			Name:      fmt.Sprintf("ground:%s", groundObject.DeviceID),
			Latitude:  groundObject.Position.Latitude,
			Longitude: groundObject.Position.Longitude,
		})
	} else {
		r.logger.WithField("device_id", groundObject.DeviceID).
			Warn("Skipping GEO indexing for ground object with invalid coordinates")
	}

	// Сохраняем детальные данные в HSET
	groundKey := GroundObjectPrefix + groundObject.DeviceID
	pipe.HSet(ctx, groundKey, map[string]interface{}{
		"name":         groundObject.Name,
		"type":         uint8(groundObject.Type),
		"track_online": groundObject.TrackOnline,
		"last_update":  groundObject.LastUpdate.Unix(),
	})

	// Устанавливаем TTL (4 часа для наземных объектов)
	pipe.Expire(ctx, groundKey, GroundObjectTTL)

	// Выполняем пайплайн
	_, err := pipe.Exec(ctx)
	if err != nil {
		metrics.RedisOperationErrors.WithLabelValues("save_ground_object").Inc()
		return fmt.Errorf("failed to save ground object: %w", err)
	}

	duration := time.Since(start).Seconds()
	metrics.RedisOperationDuration.WithLabelValues("save_ground_object").Observe(duration)
	
	r.logger.WithFields(map[string]interface{}{
		"device_id": groundObject.DeviceID,
		"name": groundObject.Name,
		"type": groundObject.Type,
		"lat": groundObject.Position.Latitude,
		"lon": groundObject.Position.Longitude,
		"duration_ms": duration * 1000,
	}).Debug("Ground object saved to Redis")

	return nil
}

// GetGroundObjectsInRadius возвращает наземные объекты в радиусе от центра
func (r *RedisRepository) GetGroundObjectsInRadius(ctx context.Context, center models.GeoPoint, radiusKM float64) ([]*models.GroundObject, error) {
	start := time.Now()
	defer func() {
		duration := time.Since(start).Seconds()
		metrics.RedisOperationDuration.WithLabelValues("get_ground_objects_radius").Observe(duration)
	}()

	// Получаем объекты из геопространственного индекса
	results, err := r.client.GeoRadius(ctx, GroundObjectsGeoKey, center.Longitude, center.Latitude, &redis.GeoRadiusQuery{
		Radius:       radiusKM,
		Unit:         "km",
		WithCoord:    true,
		WithDist:     true,
		Sort:         "ASC",
		Count:        1000, // Ограничение на количество результатов
	}).Result()

	if err != nil {
		if err == redis.Nil {
			return []*models.GroundObject{}, nil
		}
		metrics.RedisOperationErrors.WithLabelValues("get_ground_objects_radius").Inc()
		return nil, fmt.Errorf("failed to get ground objects in radius: %w", err)
	}

	if len(results) == 0 {
		return []*models.GroundObject{}, nil
	}

	// Получаем детальные данные для каждого объекта
	pipe := r.client.Pipeline()
	cmds := make([]*redis.MapStringStringCmd, len(results))
	deviceIDs := make([]string, len(results))

	for i, result := range results {
		deviceID := strings.TrimPrefix(result.Name, "ground:")
		deviceIDs[i] = deviceID
		groundKey := GroundObjectPrefix + deviceID
		cmds[i] = pipe.HGetAll(ctx, groundKey)
	}

	_, err = pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		metrics.RedisOperationErrors.WithLabelValues("get_ground_objects_details").Inc()
		return nil, fmt.Errorf("failed to get ground object details: %w", err)
	}

	groundObjects := make([]*models.GroundObject, 0, len(results))
	
	for i, cmd := range cmds {
		data, err := cmd.Result()
		if err != nil {
			r.logger.WithField("device_id", deviceIDs[i]).WithField("error", err).
				Warn("Failed to get ground object details")
			continue
		}

		if len(data) == 0 {
			r.logger.WithField("device_id", deviceIDs[i]).
				Warn("Ground object exists in GEO index but has no data")
			continue
		}

		groundObject, err := r.mapToGroundObject(deviceIDs[i], data, &redis.GeoLocation{
			Latitude:  results[i].Latitude,
			Longitude: results[i].Longitude,
		})
		if err != nil {
			r.logger.WithField("device_id", deviceIDs[i]).WithField("error", err).
				Warn("Failed to map ground object data")
			continue
		}

		groundObjects = append(groundObjects, groundObject)
	}

	r.logger.WithFields(map[string]interface{}{
		"count": len(groundObjects),
		"center_lat": center.Latitude,
		"center_lon": center.Longitude,
		"radius_km": radiusKM,
		"duration_ms": time.Since(start).Milliseconds(),
	}).Debug("Retrieved ground objects in radius")

	return groundObjects, nil
}

// DeleteGroundObject удаляет наземный объект из Redis
func (r *RedisRepository) DeleteGroundObject(ctx context.Context, deviceID string) error {
	pipe := r.client.Pipeline()
	
	// Удаляем из геопространственного индекса
	pipe.ZRem(ctx, GroundObjectsGeoKey, fmt.Sprintf("ground:%s", deviceID))
	
	// Удаляем детальные данные
	groundKey := GroundObjectPrefix + deviceID
	pipe.Del(ctx, groundKey)
	
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete ground object: %w", err)
	}
	
	r.logger.WithField("device_id", deviceID).Debug("Ground object deleted from Redis")
	return nil
}

// mapToGroundObject конвертирует HSET данные в модель наземного объекта
func (r *RedisRepository) mapToGroundObject(deviceID string, data map[string]string, location *redis.GeoLocation) (*models.GroundObject, error) {
	groundObject := &models.GroundObject{
		DeviceID: deviceID,
		Position: &models.GeoPoint{
			Latitude:  location.Latitude,
			Longitude: location.Longitude,
		},
	}

	// Парсим строковые значения из Redis HSET
	if name, ok := data["name"]; ok {
		groundObject.Name = name
	}

	if typeStr, ok := data["type"]; ok {
		if t, err := r.parseRedisUint8(typeStr, "ground_type", deviceID); err == nil {
			groundObject.Type = models.GroundType(t)
		} else {
			r.logger.WithFields(map[string]interface{}{
				"device_id": deviceID,
				"type_string": typeStr,
				"parse_error": err,
			}).Warn("Failed to parse ground object type from Redis")
		}
	}

	if updateStr, ok := data["last_update"]; ok {
		if timestamp, err := strconv.ParseInt(updateStr, 10, 64); err == nil {
			groundObject.LastUpdate = time.Unix(timestamp, 0)
		}
	}

	if onlineStr, ok := data["track_online"]; ok {
		groundObject.TrackOnline = onlineStr == "1" || onlineStr == "true"
	}

	return groundObject, nil
}