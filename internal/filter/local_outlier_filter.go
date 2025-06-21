package filter

import (
	"math"

	"github.com/flybeeper/fanet-backend/pkg/utils"
)

// LocalOutlierFilter фильтр для обнаружения выбросов внутри сегмента с учетом локального контекста
type LocalOutlierFilter struct {
	config *FilterConfig
	logger *utils.Logger
}

// NewLocalOutlierFilter создает новый локальный фильтр выбросов
func NewLocalOutlierFilter(config *FilterConfig, logger *utils.Logger) *LocalOutlierFilter {
	return &LocalOutlierFilter{
		config: config,
		logger: logger,
	}
}

// Filter применяет локальную фильтрацию выбросов
func (f *LocalOutlierFilter) Filter(track *TrackData) (*FilterResult, error) {
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
		Debug("Applying local outlier filter")

	// Группируем точки по сегментам
	segments := f.groupBySegments(track.Points)
	
	var allPoints []TrackPoint
	totalOutliers := 0
	
	// Обрабатываем каждый сегмент отдельно
	for segmentID, segmentPoints := range segments {
		f.logger.WithField("segment_id", segmentID).
			WithField("segment_points", len(segmentPoints)).
			Debug("Processing segment")
		
		// Применяем локальную фильтрацию только если в сегменте достаточно точек
		if len(segmentPoints) >= 10 { // Увеличиваем минимум до 10 точек
			outliers := f.detectLocalOutliers(segmentPoints, segmentID)
			totalOutliers += outliers
		}
		
		allPoints = append(allPoints, segmentPoints...)
	}

	// Фильтруем отмеченные выбросы
	var filteredPoints []TrackPoint
	for _, point := range allPoints {
		if !point.Filtered {
			filteredPoints = append(filteredPoints, point)
		}
	}

	result := &FilterResult{
		OriginalCount: len(track.Points),
		FilteredCount: totalOutliers,
		Points:        filteredPoints,
		Statistics: FilterStats{
			Outliers: totalOutliers,
		},
	}

	f.logger.WithField("device_id", track.DeviceID).
		WithField("original_points", len(track.Points)).
		WithField("outliers_removed", totalOutliers).
		Info("Local outlier filtering completed")

	return result, nil
}

// groupBySegments группирует точки по сегментам
func (f *LocalOutlierFilter) groupBySegments(points []TrackPoint) map[int][]TrackPoint {
	segments := make(map[int][]TrackPoint)
	
	for _, point := range points {
		segmentID := point.SegmentID
		if segmentID == 0 {
			segmentID = 1 // Если сегменты не определены, считаем все одним сегментом
		}
		segments[segmentID] = append(segments[segmentID], point)
	}
	
	return segments
}

// detectLocalOutliers обнаруживает выбросы внутри сегмента
func (f *LocalOutlierFilter) detectLocalOutliers(points []TrackPoint, segmentID int) int {
	n := len(points)
	if n < 5 {
		return 0 // Слишком мало точек для анализа
	}

	outlierCount := 0
	
	// Вычисляем локальные расстояния между точками
	distances := make([]float64, 0, n-1)
	for i := 1; i < n; i++ {
		if !points[i-1].Filtered && !points[i].Filtered {
			dist := points[i-1].Position.DistanceTo(points[i].Position)
			distances = append(distances, dist)
		}
	}
	
	if len(distances) < 3 {
		return 0
	}
	
	// Вычисляем локальную медиану и MAD
	median := f.calculateMedian(distances)
	mad := f.calculateMAD(distances, median)
	
	// Адаптивный порог с учетом характера движения в сегменте
	adaptiveThreshold := f.calculateAdaptiveThreshold(median, mad, len(points))
	
	f.logger.WithField("segment_id", segmentID).
		WithField("median_distance", median).
		WithField("mad", mad).
		WithField("adaptive_threshold", adaptiveThreshold).
		Debug("Segment statistics")
	
	// Применяем скользящее окно для обнаружения локальных аномалий
	windowSize := 5
	if n < 10 {
		windowSize = 3
	}
	
	for i := 0; i < n; i++ {
		if points[i].Filtered {
			continue
		}
		
		// Вычисляем локальный контекст
		startWindow := i - windowSize/2
		endWindow := i + windowSize/2
		
		if startWindow < 0 {
			startWindow = 0
		}
		if endWindow >= n {
			endWindow = n - 1
		}
		
		// Проверяем аномальность точки относительно локального контекста
		if f.isLocalOutlier(points, i, startWindow, endWindow, adaptiveThreshold) {
			points[i].Filtered = true
			points[i].FilterReason = "Local outlier within segment"
			outlierCount++
			
			f.logger.WithField("segment_id", segmentID).
				WithField("point_index", i).
				WithField("lat", points[i].Position.Latitude).
				WithField("lon", points[i].Position.Longitude).
				Debug("Local outlier detected")
		}
	}
	
	return outlierCount
}

// isLocalOutlier проверяет, является ли точка выбросом в локальном контексте
func (f *LocalOutlierFilter) isLocalOutlier(points []TrackPoint, idx, startWindow, endWindow int, threshold float64) bool {
	// Если точка на краю окна, проверяем только с одной стороны
	if idx == 0 || idx == len(points)-1 {
		return false // Не фильтруем граничные точки
	}
	
	// Вычисляем расстояния до соседних точек в окне
	var distances []float64
	
	for i := startWindow; i <= endWindow; i++ {
		if i != idx && !points[i].Filtered {
			dist := points[idx].Position.DistanceTo(points[i].Position)
			distances = append(distances, dist)
		}
	}
	
	if len(distances) < 2 {
		return false
	}
	
	// Вычисляем среднее расстояние до соседей
	avgDistance := 0.0
	for _, d := range distances {
		avgDistance += d
	}
	avgDistance /= float64(len(distances))
	
	// Проверяем прямое расстояние между предыдущей и следующей точкой
	if idx > 0 && idx < len(points)-1 {
		directDistance := points[idx-1].Position.DistanceTo(points[idx+1].Position)
		detourDistance := points[idx-1].Position.DistanceTo(points[idx].Position) + 
						 points[idx].Position.DistanceTo(points[idx+1].Position)
		
		// Если обход через текущую точку намного длиннее прямого пути - это выброс
		if detourDistance > directDistance*2 && detourDistance-directDistance > threshold {
			return true
		}
	}
	
	// Если точка слишком далека от всех соседей - это выброс
	return avgDistance > threshold
}

// calculateAdaptiveThreshold вычисляет адаптивный порог для сегмента
func (f *LocalOutlierFilter) calculateAdaptiveThreshold(median, mad float64, segmentSize int) float64 {
	// Базовый порог
	baseThreshold := median + 3*mad
	
	// Минимальный порог зависит от размера сегмента
	minThreshold := 2.0 // 2 км для малых сегментов
	if segmentSize > 50 {
		minThreshold = 10.0 // 10 км для больших сегментов
	}
	
	// Применяем минимальный порог
	if baseThreshold < minThreshold {
		baseThreshold = minThreshold
	}
	
	// Но не больше глобального максимума
	if baseThreshold > f.config.OutlierThresholdKm {
		baseThreshold = f.config.OutlierThresholdKm
	}
	
	return baseThreshold
}

// calculateMedian вычисляет медиану
func (f *LocalOutlierFilter) calculateMedian(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	
	// Копируем и сортируем
	sorted := make([]float64, len(values))
	copy(sorted, values)
	
	// Простая сортировка пузырьком для небольших массивов
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	
	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[mid-1] + sorted[mid]) / 2
	}
	return sorted[mid]
}

// calculateMAD вычисляет Median Absolute Deviation
func (f *LocalOutlierFilter) calculateMAD(values []float64, median float64) float64 {
	if len(values) == 0 {
		return 0
	}
	
	deviations := make([]float64, len(values))
	for i, v := range values {
		deviations[i] = math.Abs(v - median)
	}
	
	return f.calculateMedian(deviations)
}

// Name возвращает имя фильтра
func (f *LocalOutlierFilter) Name() string {
	return "LocalOutlierFilter"
}

// Description возвращает описание фильтра
func (f *LocalOutlierFilter) Description() string {
	return "Detects outliers within segment context using local statistics"
}