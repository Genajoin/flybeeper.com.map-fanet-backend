package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
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
	
	// Дополнительные индексы
	ThermalsTimeKey = "thermals:time" // Z-SET индекс термиков по времени
	
	// Префиксы для хешей с детальными данными
	PilotPrefix   = "pilot:"         // pilot:{addr}
	ThermalPrefix = "thermal:"       // thermal:{id}
	StationPrefix = "station:"       // station:{addr}
	TrackPrefix   = "track:"         // track:{addr} - список точек трека
	
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
	PilotTTL     = 12 * time.Hour     // 43200 секунд
	ThermalTTL   = 6 * time.Hour      // 21600 секунд
	StationTTL   = 24 * time.Hour     // 86400 секунд
	ClientTTL    = 5 * time.Minute    // 300 секунд
	AuthTokenTTL = 1 * time.Hour      // 3600 секунд
	
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

	pipe := r.client.Pipeline()

	// Сохраняем в геопространственный индекс
	pipe.GeoAdd(ctx, PilotsGeoKey, &redis.GeoLocation{
		Name:      fmt.Sprintf("pilot:%s", pilot.DeviceID),
		Latitude:  pilot.Position.Latitude,
		Longitude: pilot.Position.Longitude,
	})

	// Сохраняем детальные данные в HSET согласно спецификации
	pilotKey := PilotPrefix + pilot.DeviceID
	pipe.HSet(ctx, pilotKey, map[string]interface{}{
		"name":         pilot.Name,
		"type":         pilot.AircraftType,
		"altitude":     pilot.Position.Altitude,
		"speed":        pilot.Speed,
		"climb":        pilot.ClimbRate,
		"course":       pilot.Heading,
		"last_update":  pilot.LastUpdate.Unix(),
		"track_online": pilot.TrackOnline,
		"battery":      pilot.Battery,
	})

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
		return fmt.Errorf("failed to save pilot: %w", err)
	}

	// Обновляем статистику
	r.client.Incr(ctx, StatsPrefix+"pilots:updates")

	r.logger.WithField("device_id", pilot.DeviceID).WithField("lat", pilot.Position.Latitude).WithField("lon", pilot.Position.Longitude).Debug("Saved pilot to Redis")

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

	r.logger.WithField("thermal_id", thermal.ID).WithField("lat", thermal.Center.Latitude).WithField("lon", thermal.Center.Longitude).Debug("Saved thermal to Redis")

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
		if t, err := strconv.Atoi(typeStr); err == nil {
			pilot.AircraftType = uint8(t)
		}
	}

	if altStr, ok := data["altitude"]; ok {
		if alt, err := strconv.Atoi(altStr); err == nil {
			pilot.Position.Altitude = int16(alt)
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
		if battery, err := strconv.Atoi(batteryStr); err == nil {
			pilot.Battery = uint8(battery)
		}
	}

	return pilot, nil
}

// mapToThermal конвертирует HSET данные в модель термика
func (r *RedisRepository) mapToThermal(thermalID string, data map[string]string, location *redis.GeoLocation) (*models.Thermal, error) {
	thermal := &models.Thermal{
		ID: thermalID,
		Center: models.GeoPoint{
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
			thermal.Altitude = int32(alt)
		}
	}

	if qualityStr, ok := data["quality"]; ok {
		if quality, err := strconv.Atoi(qualityStr); err == nil {
			thermal.Quality = int32(quality)
		}
	}

	if climbStr, ok := data["climb"]; ok {
		if climb, err := strconv.ParseFloat(climbStr, 64); err == nil {
			thermal.ClimbRate = float32(climb) // уже в правильных единицах
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
		if humidity, err := strconv.Atoi(humidityStr); err == nil {
			station.Humidity = uint8(humidity)
		}
	}

	if pressureStr, ok := data["pressure"]; ok {
		if pressure, err := strconv.ParseFloat(pressureStr, 64); err == nil {
			station.Pressure = uint16(pressure)
		}
	}

	if batteryStr, ok := data["battery"]; ok {
		if battery, err := strconv.Atoi(batteryStr); err == nil {
			station.Battery = uint8(battery)
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