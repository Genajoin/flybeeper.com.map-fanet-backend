package filter

import (
	"fmt"
	"math"
	"time"

	"github.com/flybeeper/fanet-backend/internal/models"
	"github.com/flybeeper/fanet-backend/pkg/utils"
)

// DuplicateFilter фильтр для удаления дублирующихся точек
type DuplicateFilter struct {
	config *FilterConfig
	logger *utils.Logger
}

// NewDuplicateFilter создает новый фильтр дублей
func NewDuplicateFilter(config *FilterConfig, logger *utils.Logger) *DuplicateFilter {
	return &DuplicateFilter{
		config: config,
		logger: logger,
	}
}

// Filter применяет фильтр дублей к треку
func (f *DuplicateFilter) Filter(track *TrackData) (*FilterResult, error) {
	if len(track.Points) <= 1 {
		return &FilterResult{
			OriginalCount: len(track.Points),
			FilteredCount: 0,
			Points:        track.Points,
			Statistics:    FilterStats{},
		}, nil
	}

	f.logger.WithField("device_id", track.DeviceID).
		WithField("points_count", len(track.Points)).
		WithField("min_distance_meters", f.config.MinDistanceMeters).
		WithField("min_time_interval", f.config.MinTimeInterval).
		Debug("Applying duplicate filter")

	var filteredPoints []TrackPoint
	stats := FilterStats{}
	
	// Первая точка всегда проходит
	filteredPoints = append(filteredPoints, track.Points[0])

	for i := 1; i < len(track.Points); i++ {
		currentPoint := track.Points[i]
		lastValidPoint := filteredPoints[len(filteredPoints)-1]
		
		// Вычисляем расстояние между точками в метрах
		distanceKm := lastValidPoint.Position.DistanceTo(currentPoint.Position)
		distanceMeters := distanceKm * 1000
		
		// Вычисляем временную разность
		timeDiff := currentPoint.Timestamp.Sub(lastValidPoint.Timestamp)
		
		// Проверяем условия дублирования
		isDuplicate := false
		reason := ""
		
		// 1. Проверка минимального расстояния
		if distanceMeters < f.config.MinDistanceMeters {
			isDuplicate = true
			reason = fmt.Sprintf("Distance %.1fm < min %.1fm", distanceMeters, f.config.MinDistanceMeters)
		}
		
		// 2. Проверка минимального времени (если указано)
		if !isDuplicate && f.config.MinTimeInterval > 0 && timeDiff < f.config.MinTimeInterval {
			isDuplicate = true
			reason = fmt.Sprintf("Time diff %v < min %v", timeDiff, f.config.MinTimeInterval)
		}
		
		// 3. Проверка идентичных координат (с точностью до 6 знаков после запятой)
		if !isDuplicate && f.areCoordinatesIdentical(lastValidPoint.Position, currentPoint.Position) {
			isDuplicate = true
			reason = "Identical coordinates"
		}

		// 4. Проверка кластеризации точек (много точек в небольшой области)
		if !isDuplicate && f.isPointInCluster(currentPoint, filteredPoints, 5, 0.1) {
			isDuplicate = true
			reason = "Point in cluster"
		}

		if isDuplicate {
			f.logger.WithField("device_id", track.DeviceID).
				WithField("point_index", i).
				WithField("distance_meters", distanceMeters).
				WithField("time_diff", timeDiff).
				WithField("reason", reason).
				WithField("lat", currentPoint.Position.Latitude).
				WithField("lon", currentPoint.Position.Longitude).
				Debug("Point filtered as duplicate")
			
			currentPoint.Filtered = true
			currentPoint.FilterReason = reason
			stats.Duplicates++
			
			// Обновляем timestamp последней валидной точки, если новая точка новее
			if currentPoint.Timestamp.After(lastValidPoint.Timestamp) {
				filteredPoints[len(filteredPoints)-1].Timestamp = currentPoint.Timestamp
			}
			
			continue
		}

		// Точка не является дублем
		filteredPoints = append(filteredPoints, currentPoint)
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
		WithField("duplicates_removed", stats.Duplicates).
		Info("Duplicate filtering completed")

	return result, nil
}

// areCoordinatesIdentical проверяет идентичность координат с заданной точностью
func (f *DuplicateFilter) areCoordinatesIdentical(p1, p2 models.GeoPoint) bool {
	const precision = 1e-6 // Точность до 6 знаков после запятой (~1 метр)
	
	latDiff := math.Abs(p1.Latitude - p2.Latitude)
	lonDiff := math.Abs(p1.Longitude - p2.Longitude)
	
	return latDiff < precision && lonDiff < precision
}

// isPointInCluster проверяет, находится ли точка в кластере недавних точек
func (f *DuplicateFilter) isPointInCluster(point TrackPoint, recentPoints []TrackPoint, checkLast int, radiusKm float64) bool {
	if len(recentPoints) < checkLast {
		checkLast = len(recentPoints)
	}
	
	// Проверяем последние N точек
	start := len(recentPoints) - checkLast
	if start < 0 {
		start = 0
	}
	
	clusterCount := 0
	for i := start; i < len(recentPoints); i++ {
		distance := recentPoints[i].Position.DistanceTo(point.Position)
		if distance <= radiusKm {
			clusterCount++
		}
	}
	
	// Если больше половины недавних точек в радиусе - это кластер
	return clusterCount > checkLast/2
}

// RemoveTimeBasedDuplicates удаляет дубли на основе временных интервалов
func (f *DuplicateFilter) RemoveTimeBasedDuplicates(points []TrackPoint, minInterval time.Duration) []TrackPoint {
	if len(points) <= 1 {
		return points
	}
	
	var result []TrackPoint
	result = append(result, points[0])
	
	for i := 1; i < len(points); i++ {
		lastPoint := result[len(result)-1]
		currentPoint := points[i]
		
		timeDiff := currentPoint.Timestamp.Sub(lastPoint.Timestamp)
		
		if timeDiff >= minInterval {
			result = append(result, currentPoint)
		} else {
			// Обновляем timestamp последней точки, если новая точка новее
			if currentPoint.Timestamp.After(lastPoint.Timestamp) {
				result[len(result)-1].Timestamp = currentPoint.Timestamp
			}
		}
	}
	
	return result
}

// GetDuplicateStatistics возвращает статистику дублирования
func (f *DuplicateFilter) GetDuplicateStatistics(points []TrackPoint) map[string]interface{} {
	if len(points) <= 1 {
		return map[string]interface{}{}
	}

	totalPoints := len(points)
	uniqueCoordinates := make(map[string]bool)
	timeGaps := make([]time.Duration, 0)
	distances := make([]float64, 0)
	
	for i, point := range points {
		// Создаем ключ координат (с точностью до 6 знаков)
		coordKey := fmt.Sprintf("%.6f,%.6f", point.Position.Latitude, point.Position.Longitude)
		uniqueCoordinates[coordKey] = true
		
		// Вычисляем временные промежутки и расстояния
		if i > 0 {
			prevPoint := points[i-1]
			timeDiff := point.Timestamp.Sub(prevPoint.Timestamp)
			distance := prevPoint.Position.DistanceTo(point.Position)
			
			timeGaps = append(timeGaps, timeDiff)
			distances = append(distances, distance)
		}
	}
	
	// Вычисляем среднее время между точками
	var avgTimeGap time.Duration
	if len(timeGaps) > 0 {
		totalDuration := time.Duration(0)
		for _, gap := range timeGaps {
			totalDuration += gap
		}
		avgTimeGap = totalDuration / time.Duration(len(timeGaps))
	}
	
	// Вычисляем среднее расстояние между точками
	var avgDistance float64
	if len(distances) > 0 {
		totalDistance := 0.0
		for _, dist := range distances {
			totalDistance += dist
		}
		avgDistance = totalDistance / float64(len(distances))
	}

	return map[string]interface{}{
		"total_points":        totalPoints,
		"unique_coordinates":  len(uniqueCoordinates),
		"coordinate_duplicates": totalPoints - len(uniqueCoordinates),
		"avg_time_gap_sec":    avgTimeGap.Seconds(),
		"avg_distance_km":     avgDistance,
		"compression_ratio":   float64(len(uniqueCoordinates)) / float64(totalPoints),
	}
}

// Name возвращает имя фильтра
func (f *DuplicateFilter) Name() string {
	return "DuplicateFilter"
}

// Description возвращает описание фильтра
func (f *DuplicateFilter) Description() string {
	return "Removes duplicate track points based on distance, time intervals and coordinate similarity"
}