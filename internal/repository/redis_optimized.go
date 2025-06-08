package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"flybeeper.com/fanet-api/internal/geo"
	"flybeeper.com/fanet-api/internal/models"
	"flybeeper.com/fanet-api/pkg/utils"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

// OptimizedRedisRepository реализует оптимизированный репозиторий с pipeline и батчингом
type OptimizedRedisRepository struct {
	client        *redis.Client
	logger        *logrus.Entry
	spatial       *geo.SpatialIndex
	defaultRadius float64
	
	// Pipeline батчинг
	pipelineMu    sync.Mutex
	pipeline      redis.Pipeliner
	pipelineSize  int
	maxBatchSize  int
	flushInterval time.Duration
	flushTimer    *time.Timer
	
	// Метрики
	metrics       *RepositoryMetrics
}

// RepositoryMetrics содержит метрики производительности
type RepositoryMetrics struct {
	PipelineFlushes   uint64
	PipelineCommands  uint64
	AvgBatchSize      float64
	AvgFlushTimeMs    float64
}

// NewOptimizedRedisRepository создает новый оптимизированный репозиторий
func NewOptimizedRedisRepository(client *redis.Client, defaultRadius float64) *OptimizedRedisRepository {
	logger := utils.Logger.WithField("component", "redis_optimized")
	
	// Настройка пула соединений для оптимальной производительности
	opt := client.Options()
	opt.MaxRetries = 3
	opt.MinIdleConns = 10
	opt.MaxIdleConns = 100
	opt.MaxActiveConns = 500
	opt.ConnMaxIdleTime = 5 * time.Minute
	opt.ConnMaxLifetime = 30 * time.Minute
	
	// Создаем пространственный индекс
	spatial := geo.NewSpatialIndex(5*time.Minute, 1000, 30*time.Second)
	
	repo := &OptimizedRedisRepository{
		client:        client,
		logger:        logger,
		spatial:       spatial,
		defaultRadius: defaultRadius,
		maxBatchSize:  100,
		flushInterval: 100 * time.Millisecond,
		metrics:       &RepositoryMetrics{},
	}
	
	// Запускаем периодический flush
	go repo.periodicFlush()
	
	return repo
}

// SavePilot сохраняет пилота с использованием pipeline
func (r *OptimizedRedisRepository) SavePilot(ctx context.Context, pilot *models.Pilot) error {
	if err := pilot.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	
	// Обновляем пространственный индекс сразу
	r.spatial.Insert(pilot)
	
	// Добавляем в pipeline
	r.pipelineMu.Lock()
	defer r.pipelineMu.Unlock()
	
	if r.pipeline == nil {
		r.pipeline = r.client.Pipeline()
	}
	
	// GEO операция
	r.pipeline.GeoAdd(ctx, "pilots:geo", &redis.GeoLocation{
		Name:      pilot.GetID(),
		Longitude: pilot.GetLongitude(),
		Latitude:  pilot.GetLatitude(),
	})
	
	// Сохраняем детальные данные
	pilotKey := fmt.Sprintf("pilot:%s", pilot.GetID())
	data := map[string]interface{}{
		"name":         pilot.Name,
		"type":         int(pilot.Type),
		"lat":          pilot.GetLatitude(),
		"lon":          pilot.GetLongitude(),
		"alt":          pilot.Altitude,
		"speed":        pilot.Speed,
		"heading":      pilot.Heading,
		"climb_rate":   pilot.ClimbRate,
		"last_seen":    pilot.GetTimestamp().Unix(),
	}
	
	r.pipeline.HSet(ctx, pilotKey, data)
	r.pipeline.Expire(ctx, pilotKey, 12*time.Hour) // TTL 12 часов
	
	r.pipelineSize += 3
	
	// Флашим если достигли лимита
	if r.pipelineSize >= r.maxBatchSize {
		return r.flushPipeline(ctx)
	}
	
	// Сбрасываем таймер
	r.resetFlushTimer()
	
	return nil
}

// SavePilotBatch сохраняет несколько пилотов одной операцией
func (r *OptimizedRedisRepository) SavePilotBatch(ctx context.Context, pilots []*models.Pilot) error {
	if len(pilots) == 0 {
		return nil
	}
	
	r.pipelineMu.Lock()
	defer r.pipelineMu.Unlock()
	
	if r.pipeline == nil {
		r.pipeline = r.client.Pipeline()
	}
	
	for _, pilot := range pilots {
		if err := pilot.Validate(); err != nil {
			r.logger.WithError(err).WithField("pilot_id", pilot.GetID()).Warn("Skipping invalid pilot")
			continue
		}
		
		// Обновляем пространственный индекс
		r.spatial.Insert(pilot)
		
		// GEO операция
		r.pipeline.GeoAdd(ctx, "pilots:geo", &redis.GeoLocation{
			Name:      pilot.GetID(),
			Longitude: pilot.GetLongitude(),
			Latitude:  pilot.GetLatitude(),
		})
		
		// Детальные данные
		pilotKey := fmt.Sprintf("pilot:%s", pilot.GetID())
		data := map[string]interface{}{
			"name":         pilot.Name,
			"type":         int(pilot.Type),
			"lat":          pilot.GetLatitude(),
			"lon":          pilot.GetLongitude(),
			"alt":          pilot.Altitude,
			"speed":        pilot.Speed,
			"heading":      pilot.Heading,
			"climb_rate":   pilot.ClimbRate,
			"last_seen":    pilot.GetTimestamp().Unix(),
		}
		
		r.pipeline.HSet(ctx, pilotKey, data)
		r.pipeline.Expire(ctx, pilotKey, 12*time.Hour)
		
		r.pipelineSize += 3
	}
	
	return r.flushPipeline(ctx)
}

// GetPilotsInRadius получает пилотов в радиусе с использованием пространственного индекса
func (r *OptimizedRedisRepository) GetPilotsInRadius(ctx context.Context, center models.GeoPoint, radiusKm float64) ([]*models.Pilot, error) {
	// Сначала проверяем пространственный индекс для горячих данных
	spatialResults := r.spatial.QueryRadius(center.Latitude, center.Longitude, radiusKm)
	
	pilots := make([]*models.Pilot, 0, len(spatialResults))
	for _, obj := range spatialResults {
		if pilot, ok := obj.(*models.Pilot); ok {
			pilots = append(pilots, pilot)
		}
	}
	
	// Если нашли достаточно данных в индексе, возвращаем
	if len(pilots) > 0 {
		r.logger.WithFields(logrus.Fields{
			"source": "spatial_index",
			"count":  len(pilots),
			"radius": radiusKm,
		}).Debug("Pilots retrieved from spatial index")
		return pilots, nil
	}
	
	// Иначе запрашиваем из Redis
	return r.getPilotsFromRedis(ctx, center, radiusKm)
}

// getPilotsFromRedis получает пилотов из Redis
func (r *OptimizedRedisRepository) getPilotsFromRedis(ctx context.Context, center models.GeoPoint, radiusKm float64) ([]*models.Pilot, error) {
	// Выполняем GEO запрос
	results, err := r.client.GeoRadius(ctx, "pilots:geo", 
		center.Longitude, center.Latitude, 
		&redis.GeoRadiusQuery{
			Radius:    radiusKm,
			Unit:      "km",
			WithCoord: true,
			Count:     1000,
			Sort:      "ASC",
		}).Result()
	
	if err != nil {
		return nil, fmt.Errorf("geo radius query failed: %w", err)
	}
	
	if len(results) == 0 {
		return []*models.Pilot{}, nil
	}
	
	// Используем pipeline для получения деталей
	pipe := r.client.Pipeline()
	
	cmds := make([]*redis.MapStringStringCmd, len(results))
	for i, loc := range results {
		pilotKey := fmt.Sprintf("pilot:%s", loc.Name)
		cmds[i] = pipe.HGetAll(ctx, pilotKey)
	}
	
	_, err = pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("pipeline exec failed: %w", err)
	}
	
	// Собираем результаты
	pilots := make([]*models.Pilot, 0, len(results))
	
	for i, cmd := range cmds {
		data, err := cmd.Result()
		if err != nil || len(data) == 0 {
			continue
		}
		
		pilot, err := r.mapToPilot(results[i].Name, data)
		if err != nil {
			r.logger.WithError(err).WithField("pilot_id", results[i].Name).Warn("Failed to map pilot")
			continue
		}
		
		// Добавляем координаты из GEO результата
		if pilot.Position == nil {
			pilot.Position = &models.GeoPoint{}
		}
		pilot.Position.Latitude = results[i].Latitude
		pilot.Position.Longitude = results[i].Longitude
		
		pilots = append(pilots, pilot)
		
		// Добавляем в пространственный индекс для будущих запросов
		r.spatial.Insert(pilot)
	}
	
	r.logger.WithFields(logrus.Fields{
		"source": "redis",
		"count":  len(pilots),
		"radius": radiusKm,
	}).Debug("Pilots retrieved from Redis")
	
	return pilots, nil
}

// SaveThermal сохраняет термик с использованием pipeline
func (r *OptimizedRedisRepository) SaveThermal(ctx context.Context, thermal *models.Thermal) error {
	if err := thermal.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	
	// Обновляем пространственный индекс
	r.spatial.Insert(thermal)
	
	r.pipelineMu.Lock()
	defer r.pipelineMu.Unlock()
	
	if r.pipeline == nil {
		r.pipeline = r.client.Pipeline()
	}
	
	// GEO операция
	r.pipeline.GeoAdd(ctx, "thermals:geo", &redis.GeoLocation{
		Name:      thermal.ID,
		Longitude: thermal.GetLongitude(),
		Latitude:  thermal.GetLatitude(),
	})
	
	// Детальные данные
	thermalKey := fmt.Sprintf("thermal:%s", thermal.ID)
	data := map[string]interface{}{
		"reported_by":   thermal.ReportedBy,
		"lat":          thermal.GetLatitude(),
		"lon":          thermal.GetLongitude(),
		"alt":          thermal.Altitude,
		"quality":      thermal.Quality,
		"climb_rate":   thermal.ClimbRate,
		"pilot_count":  thermal.PilotCount,
		"last_seen":    thermal.GetTimestamp().Unix(),
	}
	
	r.pipeline.HSet(ctx, thermalKey, data)
	r.pipeline.Expire(ctx, thermalKey, 6*time.Hour) // TTL 6 часов
	
	r.pipelineSize += 3
	
	if r.pipelineSize >= r.maxBatchSize {
		return r.flushPipeline(ctx)
	}
	
	r.resetFlushTimer()
	return nil
}

// SaveStation сохраняет станцию с использованием pipeline
func (r *OptimizedRedisRepository) SaveStation(ctx context.Context, station *models.Station) error {
	if err := station.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	
	// Обновляем пространственный индекс
	r.spatial.Insert(station)
	
	r.pipelineMu.Lock()
	defer r.pipelineMu.Unlock()
	
	if r.pipeline == nil {
		r.pipeline = r.client.Pipeline()
	}
	
	// GEO операция
	r.pipeline.GeoAdd(ctx, "stations:geo", &redis.GeoLocation{
		Name:      station.GetID(),
		Longitude: station.GetLongitude(),
		Latitude:  station.GetLatitude(),
	})
	
	// Детальные данные
	stationKey := fmt.Sprintf("station:%s", station.GetID())
	data := map[string]interface{}{
		"name":           station.Name,
		"lat":            station.GetLatitude(),
		"lon":            station.GetLongitude(),
		"temperature":    station.Temperature,
		"wind_speed":     station.WindSpeed,
		"wind_direction": station.WindDirection,
		"humidity":       station.Humidity,
		"pressure":       station.Pressure,
		"last_seen":      station.GetTimestamp().Unix(),
	}
	
	r.pipeline.HSet(ctx, stationKey, data)
	r.pipeline.Expire(ctx, stationKey, 24*time.Hour) // TTL 24 часа
	
	r.pipelineSize += 3
	
	if r.pipelineSize >= r.maxBatchSize {
		return r.flushPipeline(ctx)
	}
	
	r.resetFlushTimer()
	return nil
}

// flushPipeline выполняет накопленные команды
func (r *OptimizedRedisRepository) flushPipeline(ctx context.Context) error {
	if r.pipeline == nil || r.pipelineSize == 0 {
		return nil
	}
	
	start := time.Now()
	
	_, err := r.pipeline.Exec(ctx)
	if err != nil && err != redis.Nil {
		r.logger.WithError(err).Error("Pipeline execution failed")
		// Сбрасываем pipeline даже при ошибке
		r.pipeline = nil
		r.pipelineSize = 0
		return fmt.Errorf("pipeline exec failed: %w", err)
	}
	
	// Обновляем метрики
	r.updateMetrics(r.pipelineSize, time.Since(start))
	
	r.logger.WithFields(logrus.Fields{
		"commands": r.pipelineSize,
		"duration": time.Since(start),
	}).Debug("Pipeline flushed")
	
	r.pipeline = nil
	r.pipelineSize = 0
	
	return nil
}

// periodicFlush периодически сбрасывает pipeline
func (r *OptimizedRedisRepository) periodicFlush() {
	ticker := time.NewTicker(r.flushInterval)
	defer ticker.Stop()
	
	for range ticker.C {
		r.pipelineMu.Lock()
		if r.pipelineSize > 0 {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := r.flushPipeline(ctx); err != nil {
				r.logger.WithError(err).Error("Periodic flush failed")
			}
			cancel()
		}
		r.pipelineMu.Unlock()
	}
}

// resetFlushTimer сбрасывает таймер автоматического flush
func (r *OptimizedRedisRepository) resetFlushTimer() {
	if r.flushTimer != nil {
		r.flushTimer.Stop()
	}
	
	r.flushTimer = time.AfterFunc(r.flushInterval, func() {
		r.pipelineMu.Lock()
		defer r.pipelineMu.Unlock()
		
		if r.pipelineSize > 0 {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := r.flushPipeline(ctx); err != nil {
				r.logger.WithError(err).Error("Timer flush failed")
			}
			cancel()
		}
	})
}

// updateMetrics обновляет метрики производительности
func (r *OptimizedRedisRepository) updateMetrics(batchSize int, duration time.Duration) {
	r.metrics.PipelineFlushes++
	r.metrics.PipelineCommands += uint64(batchSize)
	
	// Экспоненциальное скользящее среднее
	alpha := 0.1
	r.metrics.AvgBatchSize = r.metrics.AvgBatchSize*(1-alpha) + float64(batchSize)*alpha
	
	ms := float64(duration.Microseconds()) / 1000.0
	r.metrics.AvgFlushTimeMs = r.metrics.AvgFlushTimeMs*(1-alpha) + ms*alpha
}

// GetMetrics возвращает метрики производительности
func (r *OptimizedRedisRepository) GetMetrics() RepositoryMetrics {
	return *r.metrics
}

// Close закрывает репозиторий и сбрасывает оставшиеся команды
func (r *OptimizedRedisRepository) Close() error {
	r.pipelineMu.Lock()
	defer r.pipelineMu.Unlock()
	
	if r.flushTimer != nil {
		r.flushTimer.Stop()
	}
	
	// Финальный flush
	if r.pipelineSize > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := r.flushPipeline(ctx); err != nil {
			return err
		}
	}
	
	return r.client.Close()
}

// Вспомогательные методы для маппинга данных

func (r *OptimizedRedisRepository) mapToPilot(id string, data map[string]string) (*models.Pilot, error) {
	pilot := &models.Pilot{
		Address:  id,
		DeviceID: id,
		Position: &models.GeoPoint{},
	}
	
	// Парсим поля
	if name, ok := data["name"]; ok {
		pilot.Name = name
	}
	
	if typeStr, ok := data["type"]; ok {
		var pilotType int
		if _, err := fmt.Sscanf(typeStr, "%d", &pilotType); err == nil {
			pilot.Type = models.PilotType(pilotType)
		}
	}
	
	if lat, ok := data["lat"]; ok {
		fmt.Sscanf(lat, "%f", &pilot.Position.Latitude)
	}
	
	if lon, ok := data["lon"]; ok {
		fmt.Sscanf(lon, "%f", &pilot.Position.Longitude)
	}
	
	if alt, ok := data["alt"]; ok {
		fmt.Sscanf(alt, "%d", &pilot.Altitude)
	}
	
	if speed, ok := data["speed"]; ok {
		fmt.Sscanf(speed, "%f", &pilot.Speed)
	}
	
	if heading, ok := data["heading"]; ok {
		fmt.Sscanf(heading, "%f", &pilot.Heading)
	}
	
	if climbRate, ok := data["climb_rate"]; ok {
		fmt.Sscanf(climbRate, "%d", &pilot.ClimbRate)
	}
	
	if lastSeen, ok := data["last_seen"]; ok {
		var ts int64
		if _, err := fmt.Sscanf(lastSeen, "%d", &ts); err == nil {
			pilot.LastSeen = time.Unix(ts, 0)
			pilot.LastUpdate = pilot.LastSeen
		}
	}
	
	return pilot, nil
}

// GetThermalsInRadius получает термики в радиусе
func (r *OptimizedRedisRepository) GetThermalsInRadius(ctx context.Context, center models.GeoPoint, radiusKm float64) ([]*models.Thermal, error) {
	// Сначала проверяем пространственный индекс
	spatialResults := r.spatial.QueryRadius(center.Latitude, center.Longitude, radiusKm)
	
	thermals := make([]*models.Thermal, 0)
	for _, obj := range spatialResults {
		if thermal, ok := obj.(*models.Thermal); ok {
			thermals = append(thermals, thermal)
		}
	}
	
	if len(thermals) > 0 {
		return thermals, nil
	}
	
	// Запрашиваем из Redis
	results, err := r.client.GeoRadius(ctx, "thermals:geo",
		center.Longitude, center.Latitude,
		&redis.GeoRadiusQuery{
			Radius:    radiusKm,
			Unit:      "km",
			WithCoord: true,
			Count:     100,
			Sort:      "ASC",
		}).Result()
	
	if err != nil {
		return nil, fmt.Errorf("geo radius query failed: %w", err)
	}
	
	// Используем pipeline для получения деталей
	if len(results) > 0 {
		pipe := r.client.Pipeline()
		cmds := make([]*redis.MapStringStringCmd, len(results))
		
		for i, loc := range results {
			thermalKey := fmt.Sprintf("thermal:%s", loc.Name)
			cmds[i] = pipe.HGetAll(ctx, thermalKey)
		}
		
		_, err = pipe.Exec(ctx)
		if err != nil && err != redis.Nil {
			return nil, fmt.Errorf("pipeline exec failed: %w", err)
		}
		
		for i, cmd := range cmds {
			data, err := cmd.Result()
			if err != nil || len(data) == 0 {
				continue
			}
			
			thermal := r.mapToThermal(results[i].Name, data)
			thermal.Position = &models.GeoPoint{
				Latitude:  results[i].Latitude,
				Longitude: results[i].Longitude,
			}
			
			thermals = append(thermals, thermal)
			r.spatial.Insert(thermal)
		}
	}
	
	return thermals, nil
}

// GetStationsInBounds получает станции в границах
func (r *OptimizedRedisRepository) GetStationsInBounds(ctx context.Context, bounds models.Bounds) ([]*models.Station, error) {
	// Используем центр и радиус для GEO запроса
	center := bounds.Center()
	radiusKm := bounds.DiagonalKm() / 2
	
	// Проверяем пространственный индекс
	spatialBounds := geo.Bounds{
		MinLat: bounds.MinLat,
		MinLon: bounds.MinLon,
		MaxLat: bounds.MaxLat,
		MaxLon: bounds.MaxLon,
	}
	
	spatialResults := r.spatial.QueryBounds(spatialBounds)
	
	stations := make([]*models.Station, 0)
	for _, obj := range spatialResults {
		if station, ok := obj.(*models.Station); ok {
			stations = append(stations, station)
		}
	}
	
	if len(stations) > 0 {
		return stations, nil
	}
	
	// Запрашиваем из Redis
	results, err := r.client.GeoRadius(ctx, "stations:geo",
		center.Longitude, center.Latitude,
		&redis.GeoRadiusQuery{
			Radius:    radiusKm,
			Unit:      "km",
			WithCoord: true,
			Count:     100,
			Sort:      "ASC",
		}).Result()
	
	if err != nil {
		return nil, fmt.Errorf("geo radius query failed: %w", err)
	}
	
	// Фильтруем по точным границам и получаем детали
	if len(results) > 0 {
		pipe := r.client.Pipeline()
		validIndices := make([]int, 0, len(results))
		cmds := make([]*redis.MapStringStringCmd, 0, len(results))
		
		for i, loc := range results {
			if bounds.Contains(models.GeoPoint{Latitude: loc.Latitude, Longitude: loc.Longitude}) {
				stationKey := fmt.Sprintf("station:%s", loc.Name)
				cmd := pipe.HGetAll(ctx, stationKey)
				cmds = append(cmds, cmd)
				validIndices = append(validIndices, i)
			}
		}
		
		_, err = pipe.Exec(ctx)
		if err != nil && err != redis.Nil {
			return nil, fmt.Errorf("pipeline exec failed: %w", err)
		}
		
		for i, cmd := range cmds {
			data, err := cmd.Result()
			if err != nil || len(data) == 0 {
				continue
			}
			
			idx := validIndices[i]
			station := r.mapToStation(results[idx].Name, data)
			station.Position = &models.GeoPoint{
				Latitude:  results[idx].Latitude,
				Longitude: results[idx].Longitude,
			}
			
			stations = append(stations, station)
			r.spatial.Insert(station)
		}
	}
	
	return stations, nil
}

// GetPilotTrack получает трек пилота (заглушка)
func (r *OptimizedRedisRepository) GetPilotTrack(ctx context.Context, pilotID string, duration time.Duration) ([]*models.TrackPoint, error) {
	// TODO: Реализовать получение трека когда будет готова схема хранения
	return []*models.TrackPoint{}, nil
}

// LoadInitialData загружает начальные данные (заглушка)
func (r *OptimizedRedisRepository) LoadInitialData(ctx context.Context) error {
	// Данные загружаются по мере поступления через MQTT
	r.logger.Info("Initial data load skipped - data loaded via MQTT")
	return nil
}

// mapToThermal конвертирует данные Redis в модель Thermal
func (r *OptimizedRedisRepository) mapToThermal(id string, data map[string]string) *models.Thermal {
	thermal := &models.Thermal{
		ID:       id,
		Position: &models.GeoPoint{},
	}
	
	if reportedBy, ok := data["reported_by"]; ok {
		thermal.ReportedBy = reportedBy
	}
	
	if lat, ok := data["lat"]; ok {
		fmt.Sscanf(lat, "%f", &thermal.Center.Latitude)
	}
	
	if lon, ok := data["lon"]; ok {
		fmt.Sscanf(lon, "%f", &thermal.Center.Longitude)
	}
	
	if alt, ok := data["alt"]; ok {
		fmt.Sscanf(alt, "%d", &thermal.Altitude)
	}
	
	if quality, ok := data["quality"]; ok {
		fmt.Sscanf(quality, "%d", &thermal.Quality)
	}
	
	if climbRate, ok := data["climb_rate"]; ok {
		fmt.Sscanf(climbRate, "%f", &thermal.ClimbRate)
	}
	
	if pilotCount, ok := data["pilot_count"]; ok {
		fmt.Sscanf(pilotCount, "%d", &thermal.PilotCount)
	}
	
	if lastSeen, ok := data["last_seen"]; ok {
		var ts int64
		if _, err := fmt.Sscanf(lastSeen, "%d", &ts); err == nil {
			thermal.LastSeen = time.Unix(ts, 0)
			thermal.Timestamp = thermal.LastSeen
		}
	}
	
	return thermal
}

// mapToStation конвертирует данные Redis в модель Station
func (r *OptimizedRedisRepository) mapToStation(id string, data map[string]string) *models.Station {
	station := &models.Station{
		ID:       id,
		ChipID:   id,
		Position: &models.GeoPoint{},
	}
	
	if name, ok := data["name"]; ok {
		station.Name = name
	}
	
	if lat, ok := data["lat"]; ok {
		fmt.Sscanf(lat, "%f", &station.Position.Latitude)
	}
	
	if lon, ok := data["lon"]; ok {
		fmt.Sscanf(lon, "%f", &station.Position.Longitude)
	}
	
	if temp, ok := data["temperature"]; ok {
		fmt.Sscanf(temp, "%d", &station.Temperature)
	}
	
	if windSpeed, ok := data["wind_speed"]; ok {
		fmt.Sscanf(windSpeed, "%d", &station.WindSpeed)
	}
	
	if windDir, ok := data["wind_direction"]; ok {
		fmt.Sscanf(windDir, "%d", &station.WindDirection)
	}
	
	if humidity, ok := data["humidity"]; ok {
		fmt.Sscanf(humidity, "%d", &station.Humidity)
	}
	
	if pressure, ok := data["pressure"]; ok {
		fmt.Sscanf(pressure, "%d", &station.Pressure)
	}
	
	if lastSeen, ok := data["last_seen"]; ok {
		var ts int64
		if _, err := fmt.Sscanf(lastSeen, "%d", &ts); err == nil {
			station.LastSeen = time.Unix(ts, 0)
			station.LastUpdate = station.LastSeen
		}
	}
	
	return station
}