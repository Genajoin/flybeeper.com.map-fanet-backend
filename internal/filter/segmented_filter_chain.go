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
			// Слишком мало точек в сегменте - пропускаем фильтрацию
			s.logger.WithField("segment_id", segmentID).
				WithField("points", len(indices)).
				Debug("Skipping segment with too few points")
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
			// Находим временные границы сегмента
			var startTime, endTime time.Time
			for _, point := range segmentResult.Points {
				if !point.Filtered {
					if startTime.IsZero() || point.Timestamp.Before(startTime) {
						startTime = point.Timestamp
					}
					if endTime.IsZero() || point.Timestamp.After(endTime) {
						endTime = point.Timestamp
					}
				}
			}
			
			segmentInfo := SegmentInfo{
				ID:           segmentID,
				StartIndex:   indices[0],
				EndIndex:     indices[len(indices)-1],
				StartTime:    startTime,
				EndTime:      endTime,
				Duration:     endTime.Sub(startTime).Minutes(),
				PointCount:   validPoints,
				Color:        generateSegmentColor(segmentID),
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

// generateSegmentColor генерирует цвет для сегмента
func generateSegmentColor(segmentID int) string {
	colors := []string{
		"#1bb12e", "#ff6b35", "#f7931e", "#c149ad", "#00b4d8",
		"#0077b6", "#90e0ef", "#e63946", "#f77f00", "#fcbf49",
	}
	return colors[(segmentID-1)%len(colors)]
}