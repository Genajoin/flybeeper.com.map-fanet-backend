package filter

import (
	"fmt"
	"time"

	"github.com/flybeeper/fanet-backend/pkg/utils"
)

// TeleportationFilter фильтр для удаления явных телепортаций
type TeleportationFilter struct {
	config      *FilterConfig
	logger      *utils.Logger
	maxDistance float64 // Максимальное расстояние в км
}

// NewTeleportationFilter создает новый фильтр телепортаций
func NewTeleportationFilter(config *FilterConfig, logger *utils.Logger, maxDistanceKm float64) *TeleportationFilter {
	return &TeleportationFilter{
		config:      config,
		logger:      logger,
		maxDistance: maxDistanceKm,
	}
}

// Filter применяет фильтр телепортаций
func (f *TeleportationFilter) Filter(track *TrackData) (*FilterResult, error) {
	if len(track.Points) < 2 {
		return &FilterResult{
			OriginalCount: len(track.Points),
			FilteredCount: 0,
			Points:        track.Points,
			Statistics:    FilterStats{},
		}, nil
	}

	f.logger.WithField("device_id", track.DeviceID).
		WithField("max_distance_km", f.maxDistance).
		Debug("Applying teleportation filter")

	start := time.Now()
	originalCount := len(track.Points)
	
	// Вычисляем расстояния и скорости
	track.Points = CalculateTrackStatistics(track.Points)
	
	// Результирующие точки
	result := make([]TrackPoint, 0, len(track.Points))
	teleportations := 0
	maxJump := 0.0
	
	// Первая точка всегда остается
	result = append(result, track.Points[0])
	
	for i := 1; i < len(track.Points); i++ {
		prevPoint := result[len(result)-1]
		currPoint := track.Points[i]
		
		// Вычисляем расстояние от последней принятой точки
		distance := prevPoint.Position.DistanceTo(currPoint.Position)
		
		// Проверяем на телепортацию
		if distance > f.maxDistance {
			teleportations++
			if distance > maxJump {
				maxJump = distance
			}
			
			// Помечаем точку как отфильтрованную
			currPoint.Filtered = true
			currPoint.FilterReason = fmt.Sprintf("Teleportation: %.1f km jump", distance)
			
			f.logger.WithFields(map[string]interface{}{
				"index":    i,
				"distance": distance,
				"from_lat": prevPoint.Position.Latitude,
				"from_lon": prevPoint.Position.Longitude,
				"to_lat":   currPoint.Position.Latitude,
				"to_lon":   currPoint.Position.Longitude,
			}).Debug("Teleportation detected")
		}
		
		result = append(result, currPoint)
	}

	duration := time.Since(start)
	filteredCount := teleportations

	f.logger.WithFields(map[string]interface{}{
		"device_id":      track.DeviceID,
		"original_count": originalCount,
		"filtered_count": filteredCount,
		"teleportations": teleportations,
		"max_jump_km":    maxJump,
		"duration_ms":    duration.Milliseconds(),
	}).Info("Teleportation filter completed")

	return &FilterResult{
		OriginalCount: originalCount,
		FilteredCount: filteredCount,
		Points:        result,
		Statistics: FilterStats{
			Teleportations:  teleportations,
			MaxDistanceJump: maxJump,
		},
	}, nil
}

// Name возвращает имя фильтра
func (f *TeleportationFilter) Name() string {
	return "TeleportationFilter"
}

// Description возвращает описание фильтра
func (f *TeleportationFilter) Description() string {
	return fmt.Sprintf("Removes teleportations > %.0f km", f.maxDistance)
}