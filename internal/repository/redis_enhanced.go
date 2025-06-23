package repository

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/flybeeper/fanet-backend/internal/config"
	"github.com/flybeeper/fanet-backend/internal/models"
	"github.com/flybeeper/fanet-backend/pkg/utils"
	"github.com/redis/go-redis/v9"
)

// EnhancedRedisRepository расширяет базовый Redis репозиторий с оптимизированными pipeline операциями
type EnhancedRedisRepository struct {
	*RedisRepository // Встраиваем базовый репозиторий
	
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

// NewEnhancedRedisRepository создает новый оптимизированный репозиторий
func NewEnhancedRedisRepository(cfg *config.RedisConfig, logger *utils.Logger) *EnhancedRedisRepository {
	baseRepo, err := NewRedisRepository(cfg, logger)
	if err != nil {
		logger.Errorf("Failed to create base repository: %v", err)
		return nil
	}
	
	repo := &EnhancedRedisRepository{
		RedisRepository: baseRepo,
		maxBatchSize:    100,
		flushInterval:   100 * time.Millisecond,
		metrics:         &RepositoryMetrics{},
	}
	
	// Запускаем периодический flush
	go repo.periodicFlush()
	
	return repo
}

// SavePilotBatch сохраняет несколько пилотов одной операцией
func (r *EnhancedRedisRepository) SavePilotBatch(ctx context.Context, pilots []*models.Pilot) error {
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
			r.logger.Warnf("Skipping invalid pilot %s: %v", pilot.GetID(), err)
			continue
		}
		
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
			"alt":          pilot.Position.Altitude,
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

// EnhancedSavePilot сохраняет пилота с использованием pipeline
func (r *EnhancedRedisRepository) EnhancedSavePilot(ctx context.Context, pilot *models.Pilot) error {
	if err := pilot.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	
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
	
	// Детальные данные
	pilotKey := fmt.Sprintf("pilot:%s", pilot.GetID())
	data := map[string]interface{}{
		"name":         pilot.Name,
		"type":         int(pilot.Type),
		"lat":          pilot.GetLatitude(),
		"lon":          pilot.GetLongitude(),
		"alt":          pilot.Position.Altitude,
		"speed":        pilot.Speed,
		"heading":      pilot.Heading,
		"climb_rate":   pilot.ClimbRate,
		"last_seen":    pilot.GetTimestamp().Unix(),
	}
	
	r.pipeline.HSet(ctx, pilotKey, data)
	r.pipeline.Expire(ctx, pilotKey, 12*time.Hour)
	
	r.pipelineSize += 3
	
	// Флашим если достигли лимита
	if r.pipelineSize >= r.maxBatchSize {
		return r.flushPipeline(ctx)
	}
	
	// Сбрасываем таймер
	r.resetFlushTimer()
	
	return nil
}

// EnhancedSaveThermal сохраняет термик с использованием pipeline
func (r *EnhancedRedisRepository) EnhancedSaveThermal(ctx context.Context, thermal *models.Thermal) error {
	if err := thermal.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	
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
		"alt":          thermal.Position.Altitude,
		"quality":      thermal.Quality,
		"climb_rate":   thermal.ClimbRate,
		"last_seen":    thermal.GetTimestamp().Unix(),
	}
	
	r.pipeline.HSet(ctx, thermalKey, data)
	r.pipeline.Expire(ctx, thermalKey, 6*time.Hour)
	
	r.pipelineSize += 3
	
	if r.pipelineSize >= r.maxBatchSize {
		return r.flushPipeline(ctx)
	}
	
	r.resetFlushTimer()
	return nil
}

// EnhancedSaveStation сохраняет станцию с использованием pipeline
func (r *EnhancedRedisRepository) EnhancedSaveStation(ctx context.Context, station *models.Station) error {
	if err := station.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	
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
	r.pipeline.Expire(ctx, stationKey, 24*time.Hour)
	
	r.pipelineSize += 3
	
	if r.pipelineSize >= r.maxBatchSize {
		return r.flushPipeline(ctx)
	}
	
	r.resetFlushTimer()
	return nil
}

// flushPipeline выполняет накопленные команды
func (r *EnhancedRedisRepository) flushPipeline(ctx context.Context) error {
	if r.pipeline == nil || r.pipelineSize == 0 {
		return nil
	}
	
	start := time.Now()
	
	_, err := r.pipeline.Exec(ctx)
	if err != nil && err != redis.Nil {
		r.logger.Errorf("Pipeline execution failed: %v", err)
		// Сбрасываем pipeline даже при ошибке
		r.pipeline = nil
		r.pipelineSize = 0
		return fmt.Errorf("pipeline exec failed: %w", err)
	}
	
	// Обновляем метрики
	r.updateMetrics(r.pipelineSize, time.Since(start))
	
	r.logger.Debugf("Enhanced pipeline flushed: %d commands in %v", r.pipelineSize, time.Since(start))
	
	r.pipeline = nil
	r.pipelineSize = 0
	
	return nil
}

// ForcedFlush принудительно сбрасывает pipeline
func (r *EnhancedRedisRepository) ForcedFlush(ctx context.Context) error {
	r.pipelineMu.Lock()
	defer r.pipelineMu.Unlock()
	return r.flushPipeline(ctx)
}

// periodicFlush периодически сбрасывает pipeline
func (r *EnhancedRedisRepository) periodicFlush() {
	ticker := time.NewTicker(r.flushInterval)
	defer ticker.Stop()
	
	for range ticker.C {
		r.pipelineMu.Lock()
		if r.pipelineSize > 0 {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := r.flushPipeline(ctx); err != nil {
				r.logger.Errorf("Periodic flush failed: %v", err)
			}
			cancel()
		}
		r.pipelineMu.Unlock()
	}
}

// resetFlushTimer сбрасывает таймер автоматического flush
func (r *EnhancedRedisRepository) resetFlushTimer() {
	if r.flushTimer != nil {
		r.flushTimer.Stop()
	}
	
	r.flushTimer = time.AfterFunc(r.flushInterval, func() {
		r.pipelineMu.Lock()
		defer r.pipelineMu.Unlock()
		
		if r.pipelineSize > 0 {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := r.flushPipeline(ctx); err != nil {
				r.logger.Errorf("Timer flush failed: %v", err)
			}
			cancel()
		}
	})
}

// updateMetrics обновляет метрики производительности
func (r *EnhancedRedisRepository) updateMetrics(batchSize int, duration time.Duration) {
	r.metrics.PipelineFlushes++
	r.metrics.PipelineCommands += uint64(batchSize)
	
	// Экспоненциальное скользящее среднее
	alpha := 0.1
	r.metrics.AvgBatchSize = r.metrics.AvgBatchSize*(1-alpha) + float64(batchSize)*alpha
	
	ms := float64(duration.Microseconds()) / 1000.0
	r.metrics.AvgFlushTimeMs = r.metrics.AvgFlushTimeMs*(1-alpha) + ms*alpha
}

// GetMetrics возвращает метрики производительности
func (r *EnhancedRedisRepository) GetMetrics() RepositoryMetrics {
	return *r.metrics
}

// Close закрывает репозиторий и сбрасывает оставшиеся команды
func (r *EnhancedRedisRepository) Close() error {
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
	
	return r.RedisRepository.Close()
}