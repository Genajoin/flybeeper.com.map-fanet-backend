package filter

import (
	"time"

	"github.com/flybeeper/fanet-backend/pkg/utils"
)

// SegmentType тип сегмента трека
type SegmentType int

const (
	SegmentTypeStationary SegmentType = iota // < 5 км/ч
	SegmentTypeHike                          // 5-15 км/ч
	SegmentTypeFlight                        // > 15 км/ч
)

// SegmentInfo информация о сегменте трека
type SegmentInfo struct {
	ID           int         `json:"id"`
	Type         SegmentType `json:"type"`
	StartIndex   int         `json:"start_index"`
	EndIndex     int         `json:"end_index"`
	StartTime    time.Time   `json:"start_time"`
	EndTime      time.Time   `json:"end_time"`
	Duration     float64     `json:"duration_minutes"`
	Distance     float64     `json:"distance_km"`
	AvgSpeed     float64     `json:"avg_speed_kmh"`
	MaxSpeed     float64     `json:"max_speed_kmh"`
	PointCount   int         `json:"point_count"`
	Color        string      `json:"color"`
}

// SegmentationFilter фильтр для разделения трека на логические сегменты
type SegmentationFilter struct {
	config *FilterConfig
	logger *utils.Logger
}

// NewSegmentationFilter создает новый фильтр сегментации
func NewSegmentationFilter(config *FilterConfig, logger *utils.Logger) *SegmentationFilter {
	return &SegmentationFilter{
		config: config,
		logger: logger,
	}
}

// Filter применяет сегментацию к треку
func (f *SegmentationFilter) Filter(track *TrackData) (*FilterResult, error) {
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
		Debug("Applying segmentation filter")

	// Определяем сегменты
	segments := f.detectSegments(track.Points)
	
	// Присваиваем SegmentID каждой точке
	for _, segment := range segments {
		for i := segment.StartIndex; i <= segment.EndIndex; i++ {
			track.Points[i].SegmentID = segment.ID
		}
	}

	// Добавляем информацию о сегментах в статистику
	stats := FilterStats{
		Segments:      segments,
		SegmentCount:  len(segments),
		SegmentBreaks: len(segments) - 1, // Количество разрывов = сегменты - 1
	}

	result := &FilterResult{
		OriginalCount: len(track.Points),
		FilteredCount: 0, // Сегментация не фильтрует точки
		Points:        track.Points,
		Statistics:    stats,
	}

	f.logger.WithField("device_id", track.DeviceID).
		WithField("segments_count", len(segments)).
		Info("Segmentation completed")

	return result, nil
}

// detectSegments определяет сегменты в треке
func (f *SegmentationFilter) detectSegments(points []TrackPoint) []SegmentInfo {
	if len(points) == 0 {
		return nil
	}

	var segments []SegmentInfo
	segmentID := 1
	segmentStart := 0

	for i := 1; i < len(points); i++ {
		prevPoint := points[i-1]
		currPoint := points[i]

		// Пропускаем отфильтрованные точки
		if currPoint.Filtered || prevPoint.Filtered {
			continue
		}

		// Вычисляем временной разрыв
		timeDiff := currPoint.Timestamp.Sub(prevPoint.Timestamp)
		
		// Вычисляем расстояние
		distance := prevPoint.Position.DistanceTo(currPoint.Position)

		// Критерии для начала нового сегмента
		shouldSplit := false
		reason := ""

		// 1. Временной разрыв более 5 минут
		if timeDiff > 5*time.Minute {
			shouldSplit = true
			reason = "time gap"
		}

		// 2. Телепортация (большое расстояние при малом времени)
		if timeDiff > 0 && distance > 10 { // более 10 км
			speed := distance / timeDiff.Hours()
			// Используем консервативный максимум для проверки телепортации
			if speed > 300 * 2 { // 600 км/ч - явная телепортация для любого типа ЛА
				shouldSplit = true
				reason = "teleportation"
			}
		}

		// Если нужно разделить, сохраняем текущий сегмент
		if shouldSplit {
			segment := f.createSegmentInfo(points[segmentStart:i], segmentID, segmentStart)
			if segment.PointCount > 0 {
				segments = append(segments, segment)
				f.logger.WithField("segment_id", segmentID).
					WithField("reason", reason).
					WithField("points", segment.PointCount).
					Debug("Segment split detected")
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

// createSegmentInfo создает информацию о сегменте
func (f *SegmentationFilter) createSegmentInfo(points []TrackPoint, id int, startIndex int) SegmentInfo {
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
	maxSpeed := 0.0

	for i := 1; i < len(validPoints); i++ {
		dist := validPoints[i-1].Position.DistanceTo(validPoints[i].Position)
		totalDistance += dist

		timeDiff := validPoints[i].Timestamp.Sub(validPoints[i-1].Timestamp).Hours()
		if timeDiff > 0 {
			speed := dist / timeDiff
			if speed > maxSpeed {
				maxSpeed = speed
			}
		}
	}

	segment.Distance = totalDistance
	segment.MaxSpeed = maxSpeed

	// Средняя скорость
	if segment.Duration > 0 {
		segment.AvgSpeed = segment.Distance / (segment.Duration / 60.0)
	}

	// Определяем тип сегмента и цвет
	segment.Type = f.classifySegment(segment.AvgSpeed)
	segment.Color = f.getSegmentColor(segment.Type, segment.AvgSpeed)

	return segment
}

// classifySegment классифицирует сегмент по средней скорости
func (f *SegmentationFilter) classifySegment(avgSpeed float64) SegmentType {
	if avgSpeed < 5 {
		return SegmentTypeStationary
	} else if avgSpeed < 15 {
		return SegmentTypeHike
	}
	return SegmentTypeFlight
}

// getSegmentColor возвращает цвет для сегмента
func (f *SegmentationFilter) getSegmentColor(segmentType SegmentType, avgSpeed float64) string {
	switch segmentType {
	case SegmentTypeStationary:
		return "#808080" // Серый
	case SegmentTypeHike:
		return "#0066CC" // Синий
	case SegmentTypeFlight:
		// Градация цветов для полета
		if avgSpeed < 30 {
			return "#00AA00" // Зеленый - медленный полет
		} else if avgSpeed < 60 {
			return "#FFAA00" // Желтый - средний полет
		}
		return "#FF0000" // Красный - быстрый полет
	default:
		return "#000000" // Черный (по умолчанию)
	}
}

// Name возвращает имя фильтра
func (f *SegmentationFilter) Name() string {
	return "SegmentationFilter"
}

// Description возвращает описание фильтра
func (f *SegmentationFilter) Description() string {
	return "Splits track into logical segments based on time gaps and movement patterns"
}