package filter

import (
	"fmt"

	"github.com/flybeeper/fanet-backend/pkg/utils"
)

// SpeedBasedFilter фильтр на основе анализа скоростей
type SpeedBasedFilter struct {
	config *FilterConfig
	logger *utils.Logger
}

// NewSpeedBasedFilter создает новый фильтр скоростей
func NewSpeedBasedFilter(config *FilterConfig, logger *utils.Logger) *SpeedBasedFilter {
	return &SpeedBasedFilter{
		config: config,
		logger: logger,
	}
}

// Filter применяет фильтр скоростей к треку
func (f *SpeedBasedFilter) Filter(track *TrackData) (*FilterResult, error) {
	if len(track.Points) < 2 {
		return &FilterResult{
			OriginalCount: len(track.Points),
			FilteredCount: 0,
			Points:        track.Points,
			Statistics:    FilterStats{},
		}, nil
	}

	f.logger.WithField("device_id", track.DeviceID).
		WithField("aircraft_type", track.AircraftType).
		WithField("points_count", len(track.Points)).
		Debug("Applying speed-based filter")

	// Получаем максимальную скорость для типа ЛА
	maxSpeed := f.config.GetMaxSpeed(track.AircraftType)
	
	f.logger.WithField("max_speed_kmh", maxSpeed).
		WithField("aircraft_type", track.AircraftType).
		Debug("Using max speed for aircraft type")

	// Вычисляем статистики трека
	points := CalculateTrackStatistics(track.Points)
	
	var filteredPoints []TrackPoint
	stats := FilterStats{}
	
	// Первая точка всегда проходит
	if len(points) > 0 {
		filteredPoints = append(filteredPoints, points[0])
	}

	// Проверяем каждую последующую точку
	for i := 1; i < len(points); i++ {
		point := points[i]
		prevPoint := filteredPoints[len(filteredPoints)-1] // Берем последнюю валидную точку
		
		// Пересчитываем скорость относительно последней валидной точки
		distance := prevPoint.Position.DistanceTo(point.Position)
		timeDiff := point.Timestamp.Sub(prevPoint.Timestamp)
		
		var speed float64
		if timeDiff.Hours() > 0 && distance > 0 {
			speed = distance / timeDiff.Hours()
			point.Speed = speed
			point.Distance = distance
		}

		// Обновляем максимальную скорость в статистике
		if speed > stats.MaxSpeedDetected {
			stats.MaxSpeedDetected = speed
		}
		
		// Обновляем максимальный прыжок по расстоянию
		if distance > stats.MaxDistanceJump {
			stats.MaxDistanceJump = distance
		}

		// Проверяем превышение скорости
		if speed > maxSpeed {
			f.logger.WithField("device_id", track.DeviceID).
				WithField("point_index", i).
				WithField("calculated_speed", speed).
				WithField("max_speed", maxSpeed).
				WithField("distance_km", distance).
				WithField("time_diff_sec", timeDiff.Seconds()).
				WithField("lat", point.Position.Latitude).
				WithField("lon", point.Position.Longitude).
				Warn("Point filtered due to speed violation")
			
			point.Filtered = true
			point.FilterReason = fmt.Sprintf("Speed %.1f km/h exceeds max %.1f km/h", speed, maxSpeed)
			stats.SpeedViolations++
			
			// Не добавляем точку в результат
			continue
		}

		// Дополнительная проверка: слишком большие прыжки по расстоянию
		// Даже если скорость кажется нормальной, прыжок >100км подозрителен
		if distance > 100 {
			f.logger.WithField("device_id", track.DeviceID).
				WithField("point_index", i).
				WithField("distance_km", distance).
				WithField("time_diff_sec", timeDiff.Seconds()).
				WithField("lat", point.Position.Latitude).
				WithField("lon", point.Position.Longitude).
				Warn("Point filtered due to large distance jump")
			
			point.Filtered = true
			point.FilterReason = fmt.Sprintf("Distance jump %.1f km is too large", distance)
			stats.SpeedViolations++
			
			continue
		}

		// Точка прошла все проверки
		filteredPoints = append(filteredPoints, point)
	}

	result := &FilterResult{
		OriginalCount: len(track.Points),
		FilteredCount: len(track.Points) - len(filteredPoints),
		Points:        filteredPoints,
		Statistics:    stats,
	}

	f.logger.WithField("device_id", track.DeviceID).
		WithField("original_points", len(track.Points)).
		WithField("filtered_points", len(filteredPoints)).
		WithField("speed_violations", stats.SpeedViolations).
		WithField("max_speed_detected", stats.MaxSpeedDetected).
		WithField("max_distance_jump", stats.MaxDistanceJump).
		Info("Speed-based filtering completed")

	return result, nil
}

// Name возвращает имя фильтра
func (f *SpeedBasedFilter) Name() string {
	return "SpeedBasedFilter"
}

// Description возвращает описание фильтра
func (f *SpeedBasedFilter) Description() string {
	return "Filters track points based on realistic speed limits for aircraft types"
}

// ValidateSpeed проверяет, является ли скорость реалистичной для данного типа ЛА
func (f *SpeedBasedFilter) ValidateSpeed(speed float64, aircraftType string) bool {
	// Можно добавить дополнительную логику валидации
	return true
}

// GetSpeedStatistics возвращает статистику скоростей для трека
func (f *SpeedBasedFilter) GetSpeedStatistics(points []TrackPoint) map[string]float64 {
	if len(points) < 2 {
		return map[string]float64{}
	}

	speeds := make([]float64, 0, len(points))
	totalSpeed := 0.0
	maxSpeed := 0.0
	minSpeed := float64(^uint(0) >> 1) // Максимальное значение float64

	for _, point := range points {
		if point.Speed > 0 {
			speeds = append(speeds, point.Speed)
			totalSpeed += point.Speed
			
			if point.Speed > maxSpeed {
				maxSpeed = point.Speed
			}
			
			if point.Speed < minSpeed {
				minSpeed = point.Speed
			}
		}
	}

	if len(speeds) == 0 {
		return map[string]float64{}
	}

	avgSpeed := totalSpeed / float64(len(speeds))

	return map[string]float64{
		"avg_speed": avgSpeed,
		"max_speed": maxSpeed,
		"min_speed": minSpeed,
		"count":     float64(len(speeds)),
	}
}