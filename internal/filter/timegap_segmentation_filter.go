package filter

import (
	"time"

	"github.com/flybeeper/fanet-backend/pkg/utils"
)

// TimeGapSegmentationFilter фильтр для разделения трека по большим временным разрывам
type TimeGapSegmentationFilter struct {
	config     *FilterConfig
	logger     *utils.Logger
	gapMinutes int // Минимальный разрыв в минутах для создания нового сегмента
}

// NewTimeGapSegmentationFilter создает новый фильтр сегментации по времени
func NewTimeGapSegmentationFilter(config *FilterConfig, logger *utils.Logger, gapMinutes int) *TimeGapSegmentationFilter {
	if gapMinutes <= 0 {
		gapMinutes = 30 // По умолчанию 30 минут
	}
	return &TimeGapSegmentationFilter{
		config:     config,
		logger:     logger,
		gapMinutes: gapMinutes,
	}
}

// Filter применяет сегментацию по временным разрывам
func (f *TimeGapSegmentationFilter) Filter(track *TrackData) (*FilterResult, error) {
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
		WithField("gap_minutes", f.gapMinutes).
		Debug("Applying time gap segmentation")

	// Определяем сегменты по временным разрывам
	segments := f.detectTimeGapSegments(track.Points)
	
	// Присваиваем SegmentID каждой точке
	for _, segment := range segments {
		for i := segment.StartIndex; i <= segment.EndIndex; i++ {
			track.Points[i].SegmentID = segment.ID
		}
	}

	// Логируем информацию о сегментах
	for _, segment := range segments {
		f.logger.WithField("segment_id", segment.ID).
			WithField("start_time", segment.StartTime).
			WithField("end_time", segment.EndTime).
			WithField("duration_min", segment.Duration).
			WithField("points", segment.PointCount).
			Debug("Time segment detected")
	}

	// Добавляем информацию о сегментах в статистику
	stats := FilterStats{
		Segments:      segments,
		SegmentCount:  len(segments),
		SegmentBreaks: len(segments) - 1,
	}

	result := &FilterResult{
		OriginalCount: len(track.Points),
		FilteredCount: 0, // Этот фильтр не удаляет точки
		Points:        track.Points,
		Statistics:    stats,
	}

	f.logger.WithField("device_id", track.DeviceID).
		WithField("segments_count", len(segments)).
		WithField("gap_threshold_min", f.gapMinutes).
		Info("Time gap segmentation completed")

	return result, nil
}

// detectTimeGapSegments определяет сегменты на основе временных разрывов
func (f *TimeGapSegmentationFilter) detectTimeGapSegments(points []TrackPoint) []SegmentInfo {
	if len(points) == 0 {
		return nil
	}

	var segments []SegmentInfo
	segmentID := 1
	segmentStart := 0
	gapThreshold := time.Duration(f.gapMinutes) * time.Minute

	for i := 1; i < len(points); i++ {
		prevPoint := points[i-1]
		currPoint := points[i]

		// Вычисляем временной разрыв
		timeDiff := currPoint.Timestamp.Sub(prevPoint.Timestamp)

		// Если разрыв больше порога, создаем новый сегмент
		if timeDiff > gapThreshold {
			segment := f.createSegmentInfo(points[segmentStart:i], segmentID, segmentStart)
			if segment.PointCount > 0 {
				segments = append(segments, segment)
				
				f.logger.WithField("segment_id", segmentID).
					WithField("time_gap_minutes", timeDiff.Minutes()).
					WithField("points", segment.PointCount).
					Debug("Large time gap detected, creating new segment")
			}
			segmentID++
			segmentStart = i
		}
	}

	// Добавляем последний сегмент
	if segmentStart < len(points) {
		segment := f.createSegmentInfo(points[segmentStart:], segmentID, segmentStart)
		if segment.PointCount > 0 {
			segments = append(segments, segment)
		}
	}

	return segments
}

// createSegmentInfo создает базовую информацию о сегменте
func (f *TimeGapSegmentationFilter) createSegmentInfo(points []TrackPoint, id int, startIndex int) SegmentInfo {
	if len(points) == 0 {
		return SegmentInfo{}
	}

	// Фильтруем только неотфильтрованные точки для статистики
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

	// Базовый цвет для временных сегментов (будет переопределен позже)
	segment.Color = "#999999" // Серый по умолчанию

	return segment
}

// Name возвращает имя фильтра
func (f *TimeGapSegmentationFilter) Name() string {
	return "TimeGapSegmentationFilter"
}

// Description возвращает описание фильтра
func (f *TimeGapSegmentationFilter) Description() string {
	return "Splits track into segments based on large time gaps between points"
}