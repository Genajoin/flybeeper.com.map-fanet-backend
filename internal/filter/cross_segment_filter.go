package filter

import (
	"github.com/flybeeper/fanet-backend/pkg/utils"
)

// CrossSegmentFilter фильтр для удаления телепортаций между сегментами
type CrossSegmentFilter struct {
	config *FilterConfig
	logger *utils.Logger
}

// NewCrossSegmentFilter создает новый фильтр межсегментных телепортаций
func NewCrossSegmentFilter(config *FilterConfig, logger *utils.Logger) *CrossSegmentFilter {
	return &CrossSegmentFilter{
		config: config,
		logger: logger,
	}
}

// Filter применяет фильтрацию телепортаций между сегментами
func (f *CrossSegmentFilter) Filter(track *TrackData) (*FilterResult, error) {
	if len(track.Points) < 2 {
		return &FilterResult{
			OriginalCount: len(track.Points),
			FilteredCount: 0,
			Points:        track.Points,
			Statistics:    FilterStats{},
		}, nil
	}

	f.logger.WithField("device_id", track.DeviceID).
		WithField("points_count", len(track.Points)).
		Debug("Applying cross-segment teleportation filter")

	// Группируем точки по сегментам
	segments := f.groupBySegments(track.Points)
	
	// Проверяем переходы между сегментами
	var allPoints []TrackPoint
	teleportationsRemoved := 0
	
	// Получаем отсортированные ID сегментов
	segmentIDs := f.getSortedSegmentIDs(segments)
	
	for i, segmentID := range segmentIDs {
		segmentPoints := segments[segmentID]
		
		// Проверяем переход к следующему сегменту
		if i < len(segmentIDs)-1 {
			nextSegmentID := segmentIDs[i+1]
			nextSegmentPoints := segments[nextSegmentID]
			
			// Проверяем телепортацию между последней точкой текущего сегмента
			// и первой точкой следующего сегмента
			if len(segmentPoints) > 0 && len(nextSegmentPoints) > 0 {
				lastPoint := segmentPoints[len(segmentPoints)-1]
				firstNextPoint := nextSegmentPoints[0]
				
				if f.isTeleportation(lastPoint, firstNextPoint) {
					// Вместо удаления целого сегмента, помечаем только граничные точки
					// если телепортация слишком большая
					distance := lastPoint.Position.DistanceTo(firstNextPoint.Position)
					
					if distance > 100 { // Только для очень больших телепортаций (> 100 км)
						// Решаем, какой сегмент меньше и вероятно является выбросом
						if len(segmentPoints) <= 3 && len(nextSegmentPoints) > 10 {
							// Маленький сегмент перед большим - возможно выброс
							f.logger.WithField("segment_id", segmentID).
								WithField("segment_size", len(segmentPoints)).
								WithField("distance_km", distance).
								Info("Small segment before teleportation marked as outlier")
							
							for j := range segmentPoints {
								segmentPoints[j].Filtered = true
								segmentPoints[j].FilterReason = "Small segment with large teleportation"
								teleportationsRemoved++
							}
						} else if len(nextSegmentPoints) <= 3 && len(segmentPoints) > 10 {
							// Маленький сегмент после большого - возможно выброс
							f.logger.WithField("segment_id", nextSegmentID).
								WithField("segment_size", len(nextSegmentPoints)).
								WithField("distance_km", distance).
								Info("Small segment after teleportation marked as outlier")
							
							for j := range nextSegmentPoints {
								nextSegmentPoints[j].Filtered = true
								nextSegmentPoints[j].FilterReason = "Small segment with large teleportation"
								teleportationsRemoved++
							}
						} else {
							// Оба сегмента достаточно большие - просто логируем
							f.logger.WithField("segment_id", segmentID).
								WithField("next_segment_id", nextSegmentID).
								WithField("distance_km", distance).
								Info("Large teleportation between segments detected but both segments kept")
						}
					}
				}
			}
		}
		
		allPoints = append(allPoints, segmentPoints...)
	}

	// Дополнительная проверка изолированных точек между сегментами
	allPoints = f.checkIsolatedPoints(allPoints, &teleportationsRemoved)

	// Фильтруем отмеченные точки
	var filteredPoints []TrackPoint
	for _, point := range allPoints {
		if !point.Filtered {
			filteredPoints = append(filteredPoints, point)
		}
	}

	result := &FilterResult{
		OriginalCount: len(track.Points),
		FilteredCount: teleportationsRemoved,
		Points:        filteredPoints,
		Statistics: FilterStats{
			Outliers: teleportationsRemoved,
		},
	}

	f.logger.WithField("device_id", track.DeviceID).
		WithField("original_points", len(track.Points)).
		WithField("teleportations_removed", teleportationsRemoved).
		Info("Cross-segment filtering completed")

	return result, nil
}

// isTeleportation проверяет, является ли переход телепортацией
func (f *CrossSegmentFilter) isTeleportation(point1, point2 TrackPoint) bool {
	// Вычисляем расстояние
	distance := point1.Position.DistanceTo(point2.Position)
	
	// Вычисляем время между точками
	timeDiff := point2.Timestamp.Sub(point1.Timestamp)
	
	// Проверка 1: Огромное расстояние (явная телепортация)
	if distance > f.config.OutlierThresholdKm {
		f.logger.WithField("distance_km", distance).
			WithField("threshold_km", f.config.OutlierThresholdKm).
			Debug("Teleportation detected by distance")
		return true
	}
	
	// Проверка 2: Невозможная скорость
	if timeDiff.Hours() > 0 {
		speed := distance / timeDiff.Hours()
		maxSpeed := 500.0 // Консервативный максимум для любого типа ЛА
		
		if speed > maxSpeed {
			f.logger.WithField("speed_kmh", speed).
				WithField("max_speed_kmh", maxSpeed).
				WithField("distance_km", distance).
				WithField("time_hours", timeDiff.Hours()).
				Debug("Teleportation detected by speed")
			return true
		}
	}
	
	return false
}

// checkIsolatedPoints проверяет изолированные точки между сегментами
func (f *CrossSegmentFilter) checkIsolatedPoints(points []TrackPoint, teleportCount *int) []TrackPoint {
	if len(points) < 3 {
		return points
	}
	
	for i := 1; i < len(points)-1; i++ {
		if points[i].Filtered {
			continue
		}
		
		// Проверяем, если точка слишком далека от обеих соседних точек
		prevDist := points[i-1].Position.DistanceTo(points[i].Position)
		nextDist := points[i].Position.DistanceTo(points[i+1].Position)
		directDist := points[i-1].Position.DistanceTo(points[i+1].Position)
		
		// Если обход через точку намного длиннее прямого пути
		detourDistance := prevDist + nextDist
		if detourDistance > directDist*3 && prevDist > 50 && nextDist > 50 {
			points[i].Filtered = true
			points[i].FilterReason = "Isolated point between segments"
			*teleportCount++
			
			f.logger.WithField("point_index", i).
				WithField("prev_dist", prevDist).
				WithField("next_dist", nextDist).
				WithField("direct_dist", directDist).
				Debug("Isolated point removed")
		}
	}
	
	return points
}

// groupBySegments группирует точки по сегментам
func (f *CrossSegmentFilter) groupBySegments(points []TrackPoint) map[int][]TrackPoint {
	segments := make(map[int][]TrackPoint)
	
	for i, point := range points {
		segmentID := point.SegmentID
		if segmentID == 0 {
			segmentID = 1
		}
		segments[segmentID] = append(segments[segmentID], points[i])
	}
	
	return segments
}

// getSortedSegmentIDs возвращает отсортированные ID сегментов
func (f *CrossSegmentFilter) getSortedSegmentIDs(segments map[int][]TrackPoint) []int {
	ids := make([]int, 0, len(segments))
	for id := range segments {
		ids = append(ids, id)
	}
	
	// Простая сортировка
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			if ids[i] > ids[j] {
				ids[i], ids[j] = ids[j], ids[i]
			}
		}
	}
	
	return ids
}

// Name возвращает имя фильтра
func (f *CrossSegmentFilter) Name() string {
	return "CrossSegmentFilter"
}

// Description возвращает описание фильтра
func (f *CrossSegmentFilter) Description() string {
	return "Removes teleportations between track segments"
}