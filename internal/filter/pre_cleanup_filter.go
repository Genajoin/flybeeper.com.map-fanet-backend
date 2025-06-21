package filter

import (
	"math"
	"sort"

	"github.com/flybeeper/fanet-backend/internal/models"
	"github.com/flybeeper/fanet-backend/pkg/utils"
)

// PreCleanupFilter предварительный фильтр для очистки граничных выбросов
// Определяет основной кластер точек и удаляет изолированные точки на краях трека
type PreCleanupFilter struct {
	config *FilterConfig
	logger *utils.Logger
}

// NewPreCleanupFilter создает новый фильтр предварительной очистки
func NewPreCleanupFilter(config *FilterConfig, logger *utils.Logger) *PreCleanupFilter {
	return &PreCleanupFilter{
		config: config,
		logger: logger,
	}
}

// Filter применяет предварительную очистку к треку
func (f *PreCleanupFilter) Filter(track *TrackData) (*FilterResult, error) {
	if len(track.Points) < 3 {
		return &FilterResult{
			OriginalCount: len(track.Points),
			FilteredCount: 0,
			Points:        track.Points,
			Statistics:    FilterStats{},
		}, nil
	}

	f.logger.WithField("device_id", track.DeviceID).
		WithField("points_count", len(track.Points)).
		Debug("Applying pre-cleanup filter")

	// Определяем медианный центр и основной кластер
	medianCenter := f.calculateMedianCenter(track.Points)
	clusterRadius := f.calculateClusterRadius(track.Points, medianCenter)
	
	// Статистика
	stats := FilterStats{}
	boundaryOutliers := 0
	isolatedPoints := 0
	
	// Проверяем все точки относительно основного кластера
	for i := range track.Points {
		point := &track.Points[i]
		
		// Расстояние от медианного центра
		distanceFromCenter := point.Position.DistanceTo(medianCenter)
		
		// Проверка изолированности для граничных точек
		isIsolated := false
		isBoundary := i == 0 || i == len(track.Points)-1
		
		if isBoundary {
			// Для граничных точек проверяем связность с соседями
			neighborDistance := f.getMinNeighborDistance(track.Points, i)
			
			// Граничная точка считается изолированной если:
			// 1. Она далеко от медианного центра
			// 2. Она далеко от ближайших соседей
			if distanceFromCenter > clusterRadius*2 && neighborDistance > 50 { // 50 км от соседей
				isIsolated = true
				boundaryOutliers++
			}
		} else {
			// Для внутренних точек - более строгая проверка
			if distanceFromCenter > clusterRadius*3 {
				// Проверяем, есть ли рядом другие точки
				nearbyCount := f.countNearbyPoints(track.Points, i, 10) // в радиусе 10 км
				if nearbyCount < 2 {
					isIsolated = true
					isolatedPoints++
				}
			}
		}
		
		if isIsolated {
			point.Filtered = true
			point.FilterReason = "Isolated point outside main cluster"
			
			f.logger.WithFields(map[string]interface{}{
				"index": i,
				"lat": point.Position.Latitude,
				"lon": point.Position.Longitude,
				"distance_from_center": distanceFromCenter,
				"is_boundary": isBoundary,
			}).Debug("Point filtered by pre-cleanup")
		}
	}
	
	totalFiltered := boundaryOutliers + isolatedPoints
	
	// Обновляем статистику
	stats.Outliers = totalFiltered
	
	result := &FilterResult{
		OriginalCount: len(track.Points),
		FilteredCount: totalFiltered,
		Points:        track.Points,
		Statistics:    stats,
	}

	f.logger.WithFields(map[string]interface{}{
		"device_id": track.DeviceID,
		"original_count": len(track.Points),
		"filtered_count": totalFiltered,
		"boundary_outliers": boundaryOutliers,
		"isolated_points": isolatedPoints,
		"median_center": medianCenter,
		"cluster_radius": clusterRadius,
	}).Info("Pre-cleanup filter completed")

	return result, nil
}

// calculateMedianCenter вычисляет медианный центр трека
func (f *PreCleanupFilter) calculateMedianCenter(points []TrackPoint) models.GeoPoint {
	lats := make([]float64, 0, len(points))
	lons := make([]float64, 0, len(points))
	
	for _, point := range points {
		if !point.Filtered {
			lats = append(lats, point.Position.Latitude)
			lons = append(lons, point.Position.Longitude)
		}
	}
	
	if len(lats) == 0 {
		// Если все точки отфильтрованы, используем все
		for _, point := range points {
			lats = append(lats, point.Position.Latitude)
			lons = append(lons, point.Position.Longitude)
		}
	}
	
	medianLat := f.calculateMedian(lats)
	medianLon := f.calculateMedian(lons)
	
	return models.GeoPoint{
		Latitude:  medianLat,
		Longitude: medianLon,
	}
}

// calculateClusterRadius вычисляет типичный радиус основного кластера
func (f *PreCleanupFilter) calculateClusterRadius(points []TrackPoint, center models.GeoPoint) float64 {
	distances := make([]float64, 0, len(points))
	
	for _, point := range points {
		if !point.Filtered {
			distance := point.Position.DistanceTo(center)
			distances = append(distances, distance)
		}
	}
	
	if len(distances) == 0 {
		return 100 // По умолчанию 100 км
	}
	
	// Используем 75-й перцентиль как радиус кластера
	sort.Float64s(distances)
	percentile75 := int(float64(len(distances)) * 0.75)
	if percentile75 >= len(distances) {
		percentile75 = len(distances) - 1
	}
	
	radius := distances[percentile75]
	
	// Минимальный радиус 20 км, максимальный 200 км
	if radius < 20 {
		radius = 20
	} else if radius > 200 {
		radius = 200
	}
	
	return radius
}

// getMinNeighborDistance находит минимальное расстояние до соседних точек
func (f *PreCleanupFilter) getMinNeighborDistance(points []TrackPoint, index int) float64 {
	minDistance := math.MaxFloat64
	
	// Проверяем расстояние до 5 ближайших точек с каждой стороны
	windowSize := 5
	
	for i := index - windowSize; i <= index + windowSize; i++ {
		if i >= 0 && i < len(points) && i != index && !points[i].Filtered {
			distance := points[index].Position.DistanceTo(points[i].Position)
			if distance < minDistance {
				minDistance = distance
			}
		}
	}
	
	if minDistance == math.MaxFloat64 {
		// Если не нашли соседей в окне, проверяем весь трек
		for i, point := range points {
			if i != index && !point.Filtered {
				distance := points[index].Position.DistanceTo(point.Position)
				if distance < minDistance {
					minDistance = distance
				}
			}
		}
	}
	
	return minDistance
}

// countNearbyPoints подсчитывает количество точек в заданном радиусе
func (f *PreCleanupFilter) countNearbyPoints(points []TrackPoint, index int, radiusKm float64) int {
	count := 0
	centerPoint := points[index].Position
	
	for i, point := range points {
		if i != index && !point.Filtered {
			if centerPoint.DistanceTo(point.Position) <= radiusKm {
				count++
			}
		}
	}
	
	return count
}

// calculateMedian вычисляет медиану
func (f *PreCleanupFilter) calculateMedian(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)
	
	n := len(sorted)
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return sorted[n/2]
}

// Name возвращает имя фильтра
func (f *PreCleanupFilter) Name() string {
	return "PreCleanupFilter"
}

// Description возвращает описание фильтра
func (f *PreCleanupFilter) Description() string {
	return "Removes isolated boundary points and outliers before main filtering"
}