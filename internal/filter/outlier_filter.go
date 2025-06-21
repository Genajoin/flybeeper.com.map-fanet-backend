package filter

import (
	"math"
	"sort"

	"github.com/flybeeper/fanet-backend/internal/models"
	"github.com/flybeeper/fanet-backend/pkg/utils"
)

// OutlierFilter фильтр для обнаружения аномальных выбросов в треке
type OutlierFilter struct {
	config *FilterConfig
	logger *utils.Logger
}

// NewOutlierFilter создает новый фильтр выбросов
func NewOutlierFilter(config *FilterConfig, logger *utils.Logger) *OutlierFilter {
	return &OutlierFilter{
		config: config,
		logger: logger,
	}
}

// Filter применяет фильтр выбросов к треку
func (f *OutlierFilter) Filter(track *TrackData) (*FilterResult, error) {
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
		WithField("outlier_threshold_km", f.config.OutlierThresholdKm).
		Debug("Applying outlier filter")

	var filteredPoints []TrackPoint
	stats := FilterStats{}
	
	// Копируем точки для анализа
	points := make([]TrackPoint, len(track.Points))
	copy(points, track.Points)

	// Применяем различные методы обнаружения выбросов
	outlierFlags := f.detectOutliers(points)

	for i, point := range points {
		if outlierFlags[i] {
			f.logger.WithField("device_id", track.DeviceID).
				WithField("point_index", i).
				WithField("lat", point.Position.Latitude).
				WithField("lon", point.Position.Longitude).
				Debug("Point marked as outlier")
			
			point.Filtered = true
			point.FilterReason = "Detected as statistical outlier"
			stats.Outliers++
		} else {
			filteredPoints = append(filteredPoints, point)
		}
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
		WithField("outliers_removed", stats.Outliers).
		Info("Outlier filtering completed")

	return result, nil
}

// detectOutliers обнаруживает выбросы с использованием нескольких методов
func (f *OutlierFilter) detectOutliers(points []TrackPoint) []bool {
	n := len(points)
	outliers := make([]bool, n)
	
	// Метод 1: Обнаружение по большим расстояниям от соседних точек
	distanceOutliers := f.detectDistanceOutliers(points)
	
	// Метод 2: Обнаружение по отклонению от медианной позиции
	medianOutliers := f.detectMedianDeviationOutliers(points)
	
	// Метод 3: Обнаружение "мерцающих" точек (точки, которые далеко от траектории)
	flickeringOutliers := f.detectFlickeringOutliers(points)
	
	// Объединяем результаты (точка считается выбросом, если ее обнаружили 2+ методов)
	for i := 0; i < n; i++ {
		detectionCount := 0
		if distanceOutliers[i] {
			detectionCount++
		}
		if medianOutliers[i] {
			detectionCount++
		}
		if flickeringOutliers[i] {
			detectionCount++
		}
		
		// Помечаем как выброс, если обнаружено 2+ методами
		outliers[i] = detectionCount >= 2
	}
	
	return outliers
}

// detectDistanceOutliers обнаруживает точки с аномально большими расстояниями
func (f *OutlierFilter) detectDistanceOutliers(points []TrackPoint) []bool {
	n := len(points)
	outliers := make([]bool, n)
	
	if n < 3 {
		return outliers
	}
	
	// Вычисляем расстояния между соседними точками
	distances := make([]float64, n-1)
	for i := 1; i < n; i++ {
		distances[i-1] = points[i-1].Position.DistanceTo(points[i].Position)
	}
	
	// Находим медиану и MAD (Median Absolute Deviation)
	median := f.calculateMedian(distances)
	mad := f.calculateMAD(distances, median)
	
	// Устанавливаем порог (медиана + 3*MAD или минимальный порог)
	threshold := median + 3*mad
	minThreshold := 10.0 // Минимальный порог 10 км для расстояний
	if threshold < minThreshold {
		threshold = minThreshold
	}
	// Но не больше конфигурируемого максимума
	if threshold > f.config.OutlierThresholdKm {
		threshold = f.config.OutlierThresholdKm
	}
	
	// Помечаем точки с большими расстояниями
	for i := 1; i < n; i++ {
		distance := distances[i-1]
		if distance > threshold {
			outliers[i] = true
			
			f.logger.WithField("point_index", i).
				WithField("distance_km", distance).
				WithField("threshold_km", threshold).
				WithField("median_km", median).
				WithField("mad_km", mad).
				Debug("Distance outlier detected")
		}
	}
	
	return outliers
}

// detectMedianDeviationOutliers обнаруживает точки, сильно отклоняющиеся от медианной позиции
func (f *OutlierFilter) detectMedianDeviationOutliers(points []TrackPoint) []bool {
	n := len(points)
	outliers := make([]bool, n)
	
	if n < 5 {
		return outliers
	}
	
	// Вычисляем медианную позицию
	lats := make([]float64, n)
	lons := make([]float64, n)
	
	for i, point := range points {
		lats[i] = point.Position.Latitude
		lons[i] = point.Position.Longitude
	}
	
	medianLat := f.calculateMedian(lats)
	medianLon := f.calculateMedian(lons)
	medianPos := models.GeoPoint{Latitude: medianLat, Longitude: medianLon}
	
	// Вычисляем расстояния от медианной позиции
	deviations := make([]float64, n)
	for i, point := range points {
		deviations[i] = point.Position.DistanceTo(medianPos)
	}
	
	// Находим MAD для отклонений
	medianDeviation := f.calculateMedian(deviations)
	mad := f.calculateMAD(deviations, medianDeviation)
	
	// Порог: медиана + 4*MAD (более консервативный для этого метода)
	threshold := medianDeviation + 4*mad
	
	// Используем максимум между вычисленным порогом и минимальным (5 км)
	minThreshold := 5.0 // Минимальный порог 5 км
	if threshold < minThreshold {
		threshold = minThreshold
	}
	// Дополнительно учитываем конфигурируемый порог как максимум
	if threshold > f.config.OutlierThresholdKm {
		threshold = f.config.OutlierThresholdKm
	}
	
	for i, deviation := range deviations {
		if deviation > threshold {
			outliers[i] = true
			
			f.logger.WithField("point_index", i).
				WithField("deviation_km", deviation).
				WithField("threshold_km", threshold).
				WithField("median_deviation_km", medianDeviation).
				Debug("Median deviation outlier detected")
		}
	}
	
	return outliers
}

// detectFlickeringOutliers обнаруживает "мерцающие" точки
func (f *OutlierFilter) detectFlickeringOutliers(points []TrackPoint) []bool {
	n := len(points)
	outliers := make([]bool, n)
	
	if n < 5 {
		return outliers
	}
	
	// Окно для анализа соседних точек
	windowSize := 3
	
	for i := windowSize; i < n-windowSize; i++ {
		currentPoint := points[i]
		
		// Вычисляем среднюю позицию соседних точек (исключая текущую)
		var avgLat, avgLon float64
		count := 0
		
		for j := i - windowSize; j <= i + windowSize; j++ {
			if j != i {
				avgLat += points[j].Position.Latitude
				avgLon += points[j].Position.Longitude
				count++
			}
		}
		
		if count > 0 {
			avgLat /= float64(count)
			avgLon /= float64(count)
			avgPos := models.GeoPoint{Latitude: avgLat, Longitude: avgLon}
			
			// Расстояние от текущей точки до средней позиции соседей
			distanceToAvg := currentPoint.Position.DistanceTo(avgPos)
			
			// Если точка слишком далеко от соседей - это может быть "мерцание"
			if distanceToAvg > f.config.OutlierThresholdKm/2 {
				outliers[i] = true
				
				f.logger.WithField("point_index", i).
					WithField("distance_to_neighbors_km", distanceToAvg).
					WithField("threshold_km", f.config.OutlierThresholdKm/2).
					Debug("Flickering outlier detected")
			}
		}
	}
	
	return outliers
}

// calculateMedian вычисляет медиану для слайса чисел
func (f *OutlierFilter) calculateMedian(values []float64) float64 {
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

// calculateMAD вычисляет Median Absolute Deviation
func (f *OutlierFilter) calculateMAD(values []float64, median float64) float64 {
	if len(values) == 0 {
		return 0
	}
	
	deviations := make([]float64, len(values))
	for i, value := range values {
		deviations[i] = math.Abs(value - median)
	}
	
	return f.calculateMedian(deviations)
}

// GetOutlierStatistics возвращает статистику выбросов
func (f *OutlierFilter) GetOutlierStatistics(points []TrackPoint) map[string]interface{} {
	if len(points) < 3 {
		return map[string]interface{}{}
	}

	// Вычисляем различные статистики
	distances := make([]float64, len(points)-1)
	for i := 1; i < len(points); i++ {
		distances[i-1] = points[i-1].Position.DistanceTo(points[i].Position)
	}

	medianDistance := f.calculateMedian(distances)
	madDistance := f.calculateMAD(distances, medianDistance)
	
	// Максимальное расстояние
	maxDistance := 0.0
	for _, dist := range distances {
		if dist > maxDistance {
			maxDistance = dist
		}
	}

	return map[string]interface{}{
		"median_distance_km":    medianDistance,
		"mad_distance_km":       madDistance,
		"max_distance_km":       maxDistance,
		"outlier_threshold_km":  f.config.OutlierThresholdKm,
		"potential_outliers":    f.countPotentialOutliers(distances, medianDistance, madDistance),
	}
}

// countPotentialOutliers подсчитывает потенциальные выбросы
func (f *OutlierFilter) countPotentialOutliers(distances []float64, median, mad float64) int {
	threshold := median + 3*mad
	count := 0
	
	for _, distance := range distances {
		if distance > threshold {
			count++
		}
	}
	
	return count
}

// Name возвращает имя фильтра
func (f *OutlierFilter) Name() string {
	return "OutlierFilter"
}

// Description возвращает описание фильтра
func (f *OutlierFilter) Description() string {
	return "Detects and removes statistical outliers and anomalous track points using multiple detection methods"
}