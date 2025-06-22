package filter

import (
	"github.com/flybeeper/fanet-backend/pkg/utils"
)

// ActivityType тип активности на основе скорости
type ActivityType int

const (
	ActivityTypeGround ActivityType = iota // Наземная активность (пешеход, стоянка)
	ActivityTypeFlight                     // Полетная активность
)

// ActivitySegmentationFilter фильтр для разделения сегментов по типу активности (скорости)
type ActivitySegmentationFilter struct {
	config              *FilterConfig
	logger              *utils.Logger
	speedThreshold      float64 // Пороговая скорость для разделения активностей (км/ч)
	hysteresisLower     float64 // Нижний порог для гистерезиса
	hysteresisUpper     float64 // Верхний порог для гистерезиса
	minSegmentPoints    int     // Минимальное количество точек в сегменте
	minSegmentDuration  float64 // Минимальная продолжительность сегмента в минутах
}

// NewActivitySegmentationFilter создает новый фильтр сегментации по активности
func NewActivitySegmentationFilter(config *FilterConfig, logger *utils.Logger) *ActivitySegmentationFilter {
	return &ActivitySegmentationFilter{
		config:              config,
		logger:              logger,
		speedThreshold:      8.0,  // 8 км/ч - граница пешеход/полет
		hysteresisLower:     6.0,  // 6 км/ч - нижний порог гистерезиса
		hysteresisUpper:     10.0, // 10 км/ч - верхний порог гистерезиса
		minSegmentPoints:    5,    // Минимум 5 точек
		minSegmentDuration:  2.0,  // Минимум 2 минуты
	}
}

// Filter применяет сегментацию по активности к треку
func (f *ActivitySegmentationFilter) Filter(track *TrackData) (*FilterResult, error) {
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
		WithField("speed_threshold", f.speedThreshold).
		Debug("Applying activity segmentation")

	// Вычисляем скорости если они не заданы
	points := f.calculateSpeeds(track.Points)

	// Находим существующие временные сегменты
	timeSegments := f.extractTimeSegments(points)
	
	// Разбиваем каждый временной сегмент по активности
	var allActivitySegments []SegmentInfo
	segmentID := 1

	for _, timeSegment := range timeSegments {
		// Извлекаем точки временного сегмента
		segmentPoints := points[timeSegment.StartIndex:timeSegment.EndIndex+1]
		
		// Разбиваем по активности
		activitySegments := f.segmentByActivity(segmentPoints, timeSegment.StartIndex, &segmentID)
		
		// Фильтруем слишком маленькие сегменты
		filteredSegments := f.filterSmallSegments(activitySegments, segmentPoints)
		
		allActivitySegments = append(allActivitySegments, filteredSegments...)
	}

	// Создаем новый массив точек с дублированными переходными точками
	continuousPoints := f.createContinuousPoints(points, allActivitySegments)
	
	// Обновляем SegmentID в новом массиве точек
	f.updatePointSegmentIDs(continuousPoints, allActivitySegments)

	// Логируем информацию о созданных сегментах
	for _, segment := range allActivitySegments {
		activityType := "ground"
		if segment.AvgSpeed > f.speedThreshold {
			activityType = "flight"
		}
		
		f.logger.WithField("segment_id", segment.ID).
			WithField("activity_type", activityType).
			WithField("avg_speed", segment.AvgSpeed).
			WithField("duration_min", segment.Duration).
			WithField("points", segment.PointCount).
			Debug("Activity segment created")
	}

	// Создаем статистику
	stats := FilterStats{
		Segments:      allActivitySegments,
		SegmentCount:  len(allActivitySegments),
		SegmentBreaks: len(allActivitySegments) - 1,
	}

	result := &FilterResult{
		OriginalCount: len(track.Points),
		FilteredCount: 0, // Этот фильтр не удаляет точки, только пересегментирует
		Points:        continuousPoints,
		Statistics:    stats,
	}

	f.logger.WithField("device_id", track.DeviceID).
		WithField("activity_segments_count", len(allActivitySegments)).
		Info("Activity segmentation completed")

	return result, nil
}

// calculateSpeeds вычисляет скорости между точками если они не заданы
func (f *ActivitySegmentationFilter) calculateSpeeds(points []TrackPoint) []TrackPoint {
	result := make([]TrackPoint, len(points))
	copy(result, points)

	for i := 1; i < len(result); i++ {
		if result[i].Speed == 0 && !result[i].Filtered && !result[i-1].Filtered {
			// Вычисляем скорость
			distance := result[i-1].Position.DistanceTo(result[i].Position)
			timeDiff := result[i].Timestamp.Sub(result[i-1].Timestamp)
			
			if timeDiff > 0 && distance > 0 {
				hours := timeDiff.Hours()
				if hours > 0 {
					result[i].Speed = distance / hours
				}
			}
		}
	}

	return result
}

// extractTimeSegments извлекает существующие временные сегменты
func (f *ActivitySegmentationFilter) extractTimeSegments(points []TrackPoint) []SegmentInfo {
	if len(points) == 0 {
		return nil
	}

	var segments []SegmentInfo
	currentSegmentID := points[0].SegmentID
	if currentSegmentID == 0 {
		currentSegmentID = 1
	}
	
	segmentStart := 0

	for i := 1; i < len(points); i++ {
		pointSegmentID := points[i].SegmentID
		if pointSegmentID == 0 {
			pointSegmentID = 1
		}

		// Если изменился SegmentID, создаем сегмент
		if pointSegmentID != currentSegmentID {
			segment := SegmentInfo{
				ID:         currentSegmentID,
				StartIndex: segmentStart,
				EndIndex:   i - 1,
				StartTime:  points[segmentStart].Timestamp,
				EndTime:    points[i-1].Timestamp,
			}
			segments = append(segments, segment)
			
			segmentStart = i
			currentSegmentID = pointSegmentID
		}
	}

	// Добавляем последний сегмент
	if segmentStart < len(points) {
		segment := SegmentInfo{
			ID:         currentSegmentID,
			StartIndex: segmentStart,
			EndIndex:   len(points) - 1,
			StartTime:  points[segmentStart].Timestamp,
			EndTime:    points[len(points)-1].Timestamp,
		}
		segments = append(segments, segment)
	}

	return segments
}

// segmentByActivity разбивает набор точек по типу активности
func (f *ActivitySegmentationFilter) segmentByActivity(points []TrackPoint, baseIndex int, segmentID *int) []SegmentInfo {
	if len(points) == 0 {
		return nil
	}

	var segments []SegmentInfo
	currentActivity := f.classifyActivity(points[0].Speed)
	segmentStart := 0

	for i := 1; i < len(points); i++ {
		newActivity := f.classifyActivityWithHysteresis(points[i].Speed, currentActivity)
		
		// Если изменился тип активности, создаем новый сегмент
		if newActivity != currentActivity {
			// Включаем переходную точку (i) в предыдущий сегмент для непрерывности
			segment := f.createActivitySegment(points[segmentStart:i+1], *segmentID, baseIndex+segmentStart)
			if segment.PointCount > 0 {
				segments = append(segments, segment)
				(*segmentID)++
			}
			
			// Следующий сегмент начинается с переходной точки (i)
			segmentStart = i
			currentActivity = newActivity
		}
	}

	// Добавляем последний сегмент
	if segmentStart < len(points) {
		segment := f.createActivitySegment(points[segmentStart:], *segmentID, baseIndex+segmentStart)
		if segment.PointCount > 0 {
			segments = append(segments, segment)
			(*segmentID)++
		}
	}

	return segments
}

// classifyActivity определяет тип активности по скорости
func (f *ActivitySegmentationFilter) classifyActivity(speed float64) ActivityType {
	if speed <= f.speedThreshold {
		return ActivityTypeGround
	}
	return ActivityTypeFlight
}

// classifyActivityWithHysteresis определяет тип активности с учетом гистерезиса
func (f *ActivitySegmentationFilter) classifyActivityWithHysteresis(speed float64, currentActivity ActivityType) ActivityType {
	switch currentActivity {
	case ActivityTypeGround:
		// Для перехода к полету нужно превысить верхний порог
		if speed > f.hysteresisUpper {
			return ActivityTypeFlight
		}
		return ActivityTypeGround
	case ActivityTypeFlight:
		// Для перехода к земле нужно опуститься ниже нижнего порога
		if speed < f.hysteresisLower {
			return ActivityTypeGround
		}
		return ActivityTypeFlight
	default:
		return f.classifyActivity(speed)
	}
}

// createActivitySegment создает информацию о сегменте активности
func (f *ActivitySegmentationFilter) createActivitySegment(points []TrackPoint, id int, startIndex int) SegmentInfo {
	if len(points) == 0 {
		return SegmentInfo{}
	}

	// Фильтруем только неотфильтрованные точки
	validPoints := make([]TrackPoint, 0, len(points))
	for _, p := range points {
		if !p.Filtered {
			validPoints = append(validPoints, p)
		}
	}

	if len(validPoints) == 0 {
		return SegmentInfo{}
	}

	segment := SegmentInfo{
		ID:         id,
		StartIndex: startIndex,
		EndIndex:   startIndex + len(points) - 1,
		StartTime:  validPoints[0].Timestamp,
		EndTime:    validPoints[len(validPoints)-1].Timestamp,
		PointCount: len(validPoints),
	}

	// Вычисляем продолжительность
	segment.Duration = segment.EndTime.Sub(segment.StartTime).Minutes()

	// Вычисляем общее расстояние и скорости
	totalDistance := 0.0
	totalSpeed := 0.0
	maxSpeed := 0.0
	speedCount := 0

	for i := 1; i < len(validPoints); i++ {
		dist := validPoints[i-1].Position.DistanceTo(validPoints[i].Position)
		totalDistance += dist

		if validPoints[i].Speed > 0 {
			totalSpeed += validPoints[i].Speed
			speedCount++
			if validPoints[i].Speed > maxSpeed {
				maxSpeed = validPoints[i].Speed
			}
		}
	}

	segment.Distance = totalDistance
	segment.MaxSpeed = maxSpeed

	// Средняя скорость
	if speedCount > 0 {
		segment.AvgSpeed = totalSpeed / float64(speedCount)
	} else if segment.Duration > 0 {
		segment.AvgSpeed = segment.Distance / (segment.Duration / 60.0)
	}

	// Цвет НЕ устанавливаем - он будет определен при выводе на основе AvgSpeed

	return segment
}

// filterSmallSegments фильтрует слишком маленькие сегменты
func (f *ActivitySegmentationFilter) filterSmallSegments(segments []SegmentInfo, allPoints []TrackPoint) []SegmentInfo {
	if len(segments) <= 1 {
		return segments
	}

	var filtered []SegmentInfo
	
	for i, segment := range segments {
		// Проверяем размер сегмента
		isSmall := segment.PointCount < f.minSegmentPoints || segment.Duration < f.minSegmentDuration
		
		if isSmall {
			// Пытаемся объединить с соседним сегментом
			if len(filtered) > 0 {
				// Объединяем с предыдущим сегментом
				lastIdx := len(filtered) - 1
				filtered[lastIdx] = f.mergeSegments(filtered[lastIdx], segment, allPoints)
				
				f.logger.WithField("small_segment_id", segment.ID).
					WithField("merged_with", filtered[lastIdx].ID).
					WithField("points", segment.PointCount).
					WithField("duration", segment.Duration).
					Debug("Small segment merged with previous")
			} else if i+1 < len(segments) {
				// Объединяем со следующим сегментом
				nextSegment := segments[i+1]
				merged := f.mergeSegments(segment, nextSegment, allPoints)
				filtered = append(filtered, merged)
				
				// Пропускаем следующий сегмент, так как он уже объединен
				i++
				
				f.logger.WithField("small_segment_id", segment.ID).
					WithField("merged_with", nextSegment.ID).
					WithField("points", segment.PointCount).
					WithField("duration", segment.Duration).
					Debug("Small segment merged with next")
			} else {
				// Если это единственный сегмент, оставляем как есть
				filtered = append(filtered, segment)
			}
		} else {
			filtered = append(filtered, segment)
		}
	}

	return filtered
}

// mergeSegments объединяет два сегмента
func (f *ActivitySegmentationFilter) mergeSegments(seg1, seg2 SegmentInfo, allPoints []TrackPoint) SegmentInfo {
	merged := SegmentInfo{
		ID:         seg1.ID, // Используем ID первого сегмента
		StartIndex: seg1.StartIndex,
		EndIndex:   seg2.EndIndex,
		StartTime:  seg1.StartTime,
		EndTime:    seg2.EndTime,
		PointCount: seg1.PointCount + seg2.PointCount,
		Distance:   seg1.Distance + seg2.Distance,
	}

	merged.Duration = merged.EndTime.Sub(merged.StartTime).Minutes()

	// Пересчитываем среднюю и максимальную скорость
	totalSpeed := 0.0
	speedCount := 0
	maxSpeed := 0.0

	for i := merged.StartIndex; i <= merged.EndIndex && i < len(allPoints); i++ {
		if !allPoints[i].Filtered && allPoints[i].Speed > 0 {
			totalSpeed += allPoints[i].Speed
			speedCount++
			if allPoints[i].Speed > maxSpeed {
				maxSpeed = allPoints[i].Speed
			}
		}
	}

	if speedCount > 0 {
		merged.AvgSpeed = totalSpeed / float64(speedCount)
	} else if merged.Duration > 0 {
		merged.AvgSpeed = merged.Distance / (merged.Duration / 60.0)
	}

	merged.MaxSpeed = maxSpeed

	return merged
}

// createContinuousPoints создает новый массив точек с дублированными переходными точками
func (f *ActivitySegmentationFilter) createContinuousPoints(originalPoints []TrackPoint, segments []SegmentInfo) []TrackPoint {
	if len(segments) <= 1 {
		// Если один сегмент или меньше, дублирование не нужно
		result := make([]TrackPoint, len(originalPoints))
		copy(result, originalPoints)
		return result
	}
	
	var continuousPoints []TrackPoint
	processedIndices := make(map[int]bool)
	
	// Проходим по сегментам в порядке их ID
	for i := 0; i < len(segments); i++ {
		currentSegment := segments[i]
		
		// Добавляем все точки текущего сегмента
		for idx := currentSegment.StartIndex; idx <= currentSegment.EndIndex && idx < len(originalPoints); idx++ {
			if !processedIndices[idx] {
				point := originalPoints[idx]
				point.SegmentID = currentSegment.ID
				continuousPoints = append(continuousPoints, point)
				processedIndices[idx] = true
			}
		}
		
		// Если есть следующий сегмент, дублируем переходную точку
		if i < len(segments)-1 {
			nextSegment := segments[i+1]
			
			// Переходная точка - это первая точка следующего сегмента
			transitionIdx := nextSegment.StartIndex
			if transitionIdx < len(originalPoints) {
				// Дублируем точку с SegmentID следующего сегмента
				transitionPoint := originalPoints[transitionIdx]
				transitionPoint.SegmentID = nextSegment.ID
				continuousPoints = append(continuousPoints, transitionPoint)
			}
		}
	}
	
	return continuousPoints
}

// updatePointSegmentIDs обновляет SegmentID в точках согласно новым сегментам
func (f *ActivitySegmentationFilter) updatePointSegmentIDs(points []TrackPoint, segments []SegmentInfo) {
	for _, segment := range segments {
		for i := segment.StartIndex; i <= segment.EndIndex && i < len(points); i++ {
			// Переходные точки принадлежат более позднему сегменту (с большим ID)
			if points[i].SegmentID == 0 || segment.ID > points[i].SegmentID {
				points[i].SegmentID = segment.ID
			}
		}
	}
}

// Name возвращает имя фильтра
func (f *ActivitySegmentationFilter) Name() string {
	return "ActivitySegmentationFilter"
}

// Description возвращает описание фильтра
func (f *ActivitySegmentationFilter) Description() string {
	return "Splits track segments by activity type based on speed (ground vs flight activity)"
}