package filter

import (
	"time"
	"github.com/flybeeper/fanet-backend/pkg/utils"
)

// SegmentedFilterChain применяет фильтры к каждому сегменту трека независимо
type SegmentedFilterChain struct {
	config *FilterConfig
	logger *utils.Logger
}

// NewSegmentedFilterChain создает новую цепочку для сегментированных треков
func NewSegmentedFilterChain(config *FilterConfig, logger *utils.Logger) *SegmentedFilterChain {
	return &SegmentedFilterChain{
		config: config,
		logger: logger,
	}
}

// Filter применяет Level 1 фильтры к каждому сегменту независимо
func (s *SegmentedFilterChain) Filter(track *TrackData) (*FilterResult, error) {
	if len(track.Points) == 0 {
		return &FilterResult{
			OriginalCount: 0,
			FilteredCount: 0,
			Points:        []TrackPoint{},
			Statistics:    FilterStats{},
		}, nil
	}

	s.logger.WithField("device_id", track.DeviceID).
		WithField("total_points", len(track.Points)).
		Info("🔧 STARTING SegmentedFilterChain")

	// Группируем точки по сегментам
	segmentMap := make(map[int][]int) // SegmentID -> indices
	for i, point := range track.Points {
		segmentID := point.SegmentID
		if segmentID == 0 {
			segmentID = 1 // Default segment
		}
		segmentMap[segmentID] = append(segmentMap[segmentID], i)
	}

	// Создаем результирующий трек с теми же точками
	// ВАЖНО: Инициализируем все точки как отфильтрованные по умолчанию
	resultPoints := make([]TrackPoint, len(track.Points))
	copy(resultPoints, track.Points)
	
	// Помечаем все точки как отфильтрованные изначально
	// Только точки, прошедшие фильтрацию в сегментах, будут помечены как неотфильтрованные
	for i := range resultPoints {
		resultPoints[i].Filtered = true
	}

	totalFilteredCount := 0
	combinedStats := FilterStats{}
	var allSegments []SegmentInfo

	// Применяем Level 1 фильтры к каждому сегменту
	for segmentID, indices := range segmentMap {
		if len(indices) < 2 {
			// Слишком мало точек в сегменте - помечаем все точки как отфильтрованные
			s.logger.WithField("segment_id", segmentID).
				WithField("points", len(indices)).
				Debug("Filtering out segment with too few points")
			
			// Помечаем единственную точку как отфильтрованную
			for _, idx := range indices {
				resultPoints[idx].Filtered = true
				resultPoints[idx].FilterReason = "Isolated segment point"
			}
			totalFilteredCount += len(indices)
			continue
		}

		// Создаем под-трек для этого сегмента
		segmentPoints := make([]TrackPoint, len(indices))
		for i, idx := range indices {
			segmentPoints[i] = track.Points[idx]
			segmentPoints[i].SegmentID = segmentID // Убеждаемся что SegmentID установлен
		}

		segmentTrack := &TrackData{
			DeviceID:     track.DeviceID,
			AircraftType: track.AircraftType,
			Points:       segmentPoints,
		}

		// Создаем Level 1 цепочку для этого сегмента
		level1Chain := NewLevel1FilterChain(s.config, s.logger)
		
		segmentResult, err := level1Chain.Filter(segmentTrack)
		if err != nil {
			s.logger.WithField("segment_id", segmentID).
				WithField("error", err).
				Warn("Failed to filter segment, keeping original points")
			continue
		}

		s.logger.WithField("segment_id", segmentID).
			WithField("original_points", len(indices)).
			WithField("filtered_points", segmentResult.FilteredCount).
			Debug("Segment filtering completed")

		// Применяем результаты фильтрации к основному треку
		for i, originalIdx := range indices {
			if i < len(segmentResult.Points) {
				resultPoints[originalIdx] = segmentResult.Points[i]
				resultPoints[originalIdx].SegmentID = segmentID // Сохраняем SegmentID
			}
		}

		// Аккумулируем статистику
		totalFilteredCount += segmentResult.FilteredCount
		combinedStats.SpeedViolations += segmentResult.Statistics.SpeedViolations
		combinedStats.Duplicates += segmentResult.Statistics.Duplicates
		combinedStats.Outliers += segmentResult.Statistics.Outliers
		combinedStats.Teleportations += segmentResult.Statistics.Teleportations

		if segmentResult.Statistics.MaxSpeedDetected > combinedStats.MaxSpeedDetected {
			combinedStats.MaxSpeedDetected = segmentResult.Statistics.MaxSpeedDetected
		}
		if segmentResult.Statistics.MaxDistanceJump > combinedStats.MaxDistanceJump {
			combinedStats.MaxDistanceJump = segmentResult.Statistics.MaxDistanceJump
		}

		// Создаем SegmentInfo для каждого обработанного сегмента
		validPoints := 0
		for _, point := range segmentResult.Points {
			if !point.Filtered {
				validPoints++
			}
		}
		
		if validPoints > 1 {
			// Находим временные границы сегмента и вычисляем среднюю скорость
			var startTime, endTime time.Time
			totalDistance := 0.0
			segmentSpeed := 0.0
			speedCount := 0
			
			for i, point := range segmentResult.Points {
				if !point.Filtered {
					if startTime.IsZero() || point.Timestamp.Before(startTime) {
						startTime = point.Timestamp
					}
					if endTime.IsZero() || point.Timestamp.After(endTime) {
						endTime = point.Timestamp
					}
					
					// Вычисляем расстояние и скорость
					if i > 0 && !segmentResult.Points[i-1].Filtered {
						dist := segmentResult.Points[i-1].Position.DistanceTo(point.Position)
						totalDistance += dist
					}
					
					if point.Speed > 0 {
						segmentSpeed += point.Speed
						speedCount++
					}
				}
			}
			
			// Средняя скорость сегмента
			avgSpeed := 0.0
			if speedCount > 0 {
				avgSpeed = segmentSpeed / float64(speedCount)
			} else if endTime.Sub(startTime).Hours() > 0 {
				avgSpeed = totalDistance / endTime.Sub(startTime).Hours()
			}
			
			segmentInfo := SegmentInfo{
				ID:           segmentID,
				StartIndex:   indices[0],
				EndIndex:     indices[len(indices)-1],
				StartTime:    startTime,
				EndTime:      endTime,
				Duration:     endTime.Sub(startTime).Minutes(),
				Distance:     totalDistance,
				AvgSpeed:     avgSpeed,
				PointCount:   validPoints,
				Color:        generateSegmentColor(avgSpeed),
			}
			
			allSegments = append(allSegments, segmentInfo)
		}
	}

	// Вычисляем среднюю скорость
	validPointCount := 0
	totalSpeed := 0.0
	for _, point := range resultPoints {
		if !point.Filtered && point.Speed > 0 {
			totalSpeed += point.Speed
			validPointCount++
		}
	}
	if validPointCount > 0 {
		combinedStats.AvgSpeed = totalSpeed / float64(validPointCount)
	}

	// Устанавливаем информацию о сегментах
	combinedStats.Segments = allSegments
	combinedStats.SegmentCount = len(segmentMap)
	combinedStats.SegmentBreaks = len(segmentMap) - 1

	// Пересчитываем общее количество отфильтрованных точек
	// так как теперь все точки вне сегментов тоже считаются отфильтрованными
	actualFilteredCount := 0
	for _, point := range resultPoints {
		if point.Filtered {
			actualFilteredCount++
		}
	}

	result := &FilterResult{
		OriginalCount: len(track.Points),
		FilteredCount: actualFilteredCount,
		Points:        resultPoints,
		Statistics:    combinedStats,
	}

	s.logger.WithField("device_id", track.DeviceID).
		WithField("segments_processed", len(segmentMap)).
		WithField("total_filtered", actualFilteredCount).
		WithField("final_points", len(resultPoints)-actualFilteredCount).
		Info("Segmented filtering completed")

	return result, nil
}

// Name возвращает имя фильтра
func (s *SegmentedFilterChain) Name() string {
	return "SegmentedFilterChain"
}

// Description возвращает описание фильтра
func (s *SegmentedFilterChain) Description() string {
	return "Applies Level 1 filters to each track segment independently"
}

// generateSegmentColor генерирует цвет для сегмента на основе средней скорости
func generateSegmentColor(avgSpeed float64) string {
	return GenerateColorBySpeed(avgSpeed)
}