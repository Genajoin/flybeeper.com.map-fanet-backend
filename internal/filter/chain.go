package filter

import (
	"fmt"
	"time"

	"github.com/flybeeper/fanet-backend/internal/models"
	"github.com/flybeeper/fanet-backend/pkg/utils"
)

// FilterChain цепочка фильтров для последовательного применения
type FilterChain struct {
	filters []TrackFilter
	config  *FilterConfig
	logger  *utils.Logger
}

// NewFilterChain создает новую цепочку фильтров
func NewFilterChain(config *FilterConfig, logger *utils.Logger) *FilterChain {
	chain := &FilterChain{
		filters: make([]TrackFilter, 0),
		config:  config,
		logger:  logger,
	}

	// Добавляем фильтры в зависимости от конфигурации
	if config.EnableDuplicateFilter {
		chain.AddFilter(NewDuplicateFilter(config, logger))
	}
	
	if config.EnableSpeedFilter {
		chain.AddFilter(NewSpeedBasedFilter(config, logger))
	}
	
	if config.EnableOutlierFilter {
		chain.AddFilter(NewOutlierFilter(config, logger))
	}

	return chain
}

// AddFilter добавляет фильтр в цепочку
func (fc *FilterChain) AddFilter(filter TrackFilter) {
	fc.filters = append(fc.filters, filter)
}

// Filter применяет все фильтры в цепочке
func (fc *FilterChain) Filter(track *TrackData) (*FilterResult, error) {
	if len(track.Points) == 0 {
		return &FilterResult{
			OriginalCount: 0,
			FilteredCount: 0,
			Points:        []TrackPoint{},
			Statistics:    FilterStats{},
		}, nil
	}

	fc.logger.WithField("device_id", track.DeviceID).
		WithField("original_points", len(track.Points)).
		WithField("filters_count", len(fc.filters)).
		Debug("Starting track filtering")

	originalCount := len(track.Points)
	currentTrack := *track // Копируем трек
	combinedStats := FilterStats{}
	
	// Применяем каждый фильтр последовательно
	for _, filter := range fc.filters {
		start := time.Now()
		
		result, err := filter.Filter(&currentTrack)
		if err != nil {
			fc.logger.WithField("filter", filter.Name()).
				WithField("error", err).
				Error("Filter failed")
			continue
		}

		duration := time.Since(start)
		
		fc.logger.WithField("filter", filter.Name()).
			WithField("input_points", len(currentTrack.Points)).
			WithField("output_points", len(result.Points)).
			WithField("filtered_points", result.FilteredCount).
			WithField("duration_ms", duration.Milliseconds()).
			Debug("Filter applied")

		// Обновляем трек для следующего фильтра
		currentTrack.Points = result.Points
		
		// Объединяем статистику
		combinedStats.SpeedViolations += result.Statistics.SpeedViolations
		combinedStats.Duplicates += result.Statistics.Duplicates
		combinedStats.Outliers += result.Statistics.Outliers
		
		if result.Statistics.MaxSpeedDetected > combinedStats.MaxSpeedDetected {
			combinedStats.MaxSpeedDetected = result.Statistics.MaxSpeedDetected
		}
		
		if result.Statistics.MaxDistanceJump > combinedStats.MaxDistanceJump {
			combinedStats.MaxDistanceJump = result.Statistics.MaxDistanceJump
		}
	}

	// Вычисляем финальную статистику
	finalCount := len(currentTrack.Points)
	filteredCount := originalCount - finalCount
	
	// Вычисляем среднюю скорость
	if finalCount > 1 {
		totalSpeed := 0.0
		validSpeedPoints := 0
		
		for _, point := range currentTrack.Points {
			if point.Speed > 0 {
				totalSpeed += point.Speed
				validSpeedPoints++
			}
		}
		
		if validSpeedPoints > 0 {
			combinedStats.AvgSpeed = totalSpeed / float64(validSpeedPoints)
		}
	}

	result := &FilterResult{
		OriginalCount: originalCount,
		FilteredCount: filteredCount,
		Points:        currentTrack.Points,
		Statistics:    combinedStats,
	}

	fc.logger.WithField("device_id", track.DeviceID).
		WithField("original_count", originalCount).
		WithField("filtered_count", filteredCount).
		WithField("final_count", finalCount).
		WithField("avg_speed", combinedStats.AvgSpeed).
		WithField("max_speed", combinedStats.MaxSpeedDetected).
		Info("Track filtering completed")

	return result, nil
}

// Name возвращает имя цепочки фильтров
func (fc *FilterChain) Name() string {
	return "FilterChain"
}

// Description возвращает описание цепочки фильтров
func (fc *FilterChain) Description() string {
	filterNames := make([]string, len(fc.filters))
	for i, filter := range fc.filters {
		filterNames[i] = filter.Name()
	}
	return fmt.Sprintf("Chain of filters: %v", filterNames)
}

// ConvertGeoPointsToTrackData конвертирует слайс GeoPoint в TrackData для фильтрации
func ConvertGeoPointsToTrackData(points []models.GeoPoint, deviceID string, aircraftType models.PilotType) *TrackData {
	trackPoints := make([]TrackPoint, len(points))
	
	for i, point := range points {
		trackPoints[i] = TrackPoint{
			Position:  point,
			Timestamp: time.Now().Add(time.Duration(i) * time.Minute), // Простая временная метка
		}
	}
	
	return &TrackData{
		DeviceID:     deviceID,
		AircraftType: aircraftType,
		Points:       trackPoints,
	}
}

// ConvertTrackDataToGeoPoints конвертирует TrackData обратно в слайс GeoPoint
func ConvertTrackDataToGeoPoints(track *TrackData) []models.GeoPoint {
	points := make([]models.GeoPoint, 0, len(track.Points))
	
	for _, trackPoint := range track.Points {
		if !trackPoint.Filtered {
			points = append(points, trackPoint.Position)
		}
	}
	
	return points
}

// CalculateTrackStatistics вычисляет скорости и расстояния между точками
func CalculateTrackStatistics(points []TrackPoint) []TrackPoint {
	if len(points) < 2 {
		return points
	}
	
	result := make([]TrackPoint, len(points))
	copy(result, points)
	
	for i := 1; i < len(result); i++ {
		prev := result[i-1]
		curr := &result[i]
		
		// Вычисляем расстояние между точками
		distance := prev.Position.DistanceTo(curr.Position)
		curr.Distance = distance
		
		// Вычисляем скорость если есть временная разница
		timeDiff := curr.Timestamp.Sub(prev.Timestamp)
		if timeDiff > 0 && distance > 0 {
			// Скорость в км/ч = расстояние(км) / время(ч)
			hours := timeDiff.Hours()
			if hours > 0 {
				curr.Speed = distance / hours
			}
		}
	}
	
	return result
}