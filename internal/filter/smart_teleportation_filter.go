package filter

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/flybeeper/fanet-backend/internal/models"
	"github.com/flybeeper/fanet-backend/pkg/utils"
)

// SmartTeleportationFilter умный фильтр для обнаружения различных типов GPS аномалий
type SmartTeleportationFilter struct {
	config                *FilterConfig
	logger                *utils.Logger
	maxReasonableSpeed    float64 // Максимальная разумная скорость для типа ЛА (км/ч)
	duplicateThreshold    int     // Количество одинаковых точек подряд для считания дублем
	pingPongThreshold     int     // Количество переключений для детекции пинг-понга
	slidingWindowSize     int     // Размер окна для скользящей медианы
	speedMultiplierThresh float64 // Множитель медианы для определения аномальной скорости
}

// NewSmartTeleportationFilter создает новый умный фильтр телепортаций
func NewSmartTeleportationFilter(config *FilterConfig, logger *utils.Logger) *SmartTeleportationFilter {
	return &SmartTeleportationFilter{
		config:                config,
		logger:                logger,
		maxReasonableSpeed:    300.0, // 300 км/ч - максимум для параглайдера в экстремальных условиях
		duplicateThreshold:    5,     // 5+ одинаковых точек = массовый дубль
		pingPongThreshold:     3,     // 3+ переключения = пинг-понг
		slidingWindowSize:     15,    // Окно для медианы
		speedMultiplierThresh: 10.0,  // Превышение медианы в 10x = аномалия
	}
}

// PingPongDetector детектирует повторяющиеся переключения между точками
type PingPongDetector struct {
	positions []models.GeoPoint
	counts    []int
	threshold int
}

func NewPingPongDetector(threshold int) *PingPongDetector {
	return &PingPongDetector{
		positions: make([]models.GeoPoint, 0, 10),
		counts:    make([]int, 0, 10),
		threshold: threshold,
	}
}

func (ppd *PingPongDetector) AddPoint(point models.GeoPoint) bool {
	// Ищем эту позицию в уже известных (с точностью до 10м)
	for i, pos := range ppd.positions {
		if pos.DistanceTo(point) < 0.01 { // 10 метров
			ppd.counts[i]++
			return ppd.counts[i] >= ppd.threshold
		}
	}
	
	// Новая позиция
	ppd.positions = append(ppd.positions, point)
	ppd.counts = append(ppd.counts, 1)
	
	// Ограничиваем размер детектора
	if len(ppd.positions) > 10 {
		ppd.positions = ppd.positions[1:]
		ppd.counts = ppd.counts[1:]
	}
	
	return false
}

// calculateSlidingMedianSpeed вычисляет скользящую медиану скорости
func (f *SmartTeleportationFilter) calculateSlidingMedianSpeed(points []TrackPoint, index int) float64 {
	if index < f.slidingWindowSize {
		return 0 // Недостаточно данных для медианы
	}
	
	// Собираем скорости в окне
	speeds := make([]float64, 0, f.slidingWindowSize)
	start := index - f.slidingWindowSize
	if start < 1 {
		start = 1
	}
	
	for i := start; i < index; i++ {
		if points[i].Speed > 0 {
			speeds = append(speeds, points[i].Speed)
		}
	}
	
	if len(speeds) < 3 {
		return 0 // Недостаточно данных
	}
	
	// Вычисляем медиану
	sort.Float64s(speeds)
	mid := len(speeds) / 2
	if len(speeds)%2 == 0 {
		return (speeds[mid-1] + speeds[mid]) / 2
	}
	return speeds[mid]
}

// calculateMAD вычисляет Median Absolute Deviation
func (f *SmartTeleportationFilter) calculateMAD(speeds []float64, median float64) float64 {
	if len(speeds) < 3 {
		return 0
	}
	
	deviations := make([]float64, len(speeds))
	for i, speed := range speeds {
		deviations[i] = math.Abs(speed - median)
	}
	
	sort.Float64s(deviations)
	mid := len(deviations) / 2
	if len(deviations)%2 == 0 {
		return (deviations[mid-1] + deviations[mid]) / 2
	}
	return deviations[mid]
}

// isDuplicate проверяет, является ли точка дублем предыдущих
func (f *SmartTeleportationFilter) isDuplicate(points []TrackPoint, index int) bool {
	if index < f.duplicateThreshold {
		return false
	}
	
	current := points[index].Position
	duplicateCount := 1
	
	// Проверяем назад
	for i := index - 1; i >= 0 && duplicateCount < f.duplicateThreshold; i-- {
		if current.DistanceTo(points[i].Position) < 0.001 { // 1 метр точность
			duplicateCount++
		} else {
			break
		}
	}
	
	return duplicateCount >= f.duplicateThreshold
}

// Filter применяет умную фильтрацию телепортаций
func (f *SmartTeleportationFilter) Filter(track *TrackData) (*FilterResult, error) {
	if len(track.Points) < 3 {
		return &FilterResult{
			OriginalCount: len(track.Points),
			FilteredCount: 0,
			Points:        track.Points,
			Statistics:    FilterStats{},
		}, nil
	}

	f.logger.WithField("device_id", track.DeviceID).
		WithField("max_speed_kmh", f.maxReasonableSpeed).
		Debug("Applying smart teleportation filter")

	start := time.Now()
	originalCount := len(track.Points)
	
	// Вычисляем расстояния и скорости
	track.Points = CalculateTrackStatistics(track.Points)
	
	// Детекторы
	pingPongDetector := NewPingPongDetector(f.pingPongThreshold)
	
	// Результирующие точки
	result := make([]TrackPoint, 0, len(track.Points))
	
	// Статистика
	teleportations := 0
	massiveDuplicates := 0
	pingPongPoints := 0
	speedViolations := 0
	maxJump := 0.0
	maxSpeedDetected := 0.0
	
	// Первая точка всегда остается
	result = append(result, track.Points[0])
	pingPongDetector.AddPoint(track.Points[0].Position)
	
	for i := 1; i < len(track.Points); i++ {
		currPoint := track.Points[i]
		prevPoint := result[len(result)-1]
		
		shouldFilter := false
		filterReason := ""
		
		// 1. Проверка на массовые дубли
		if f.isDuplicate(track.Points, i) {
			shouldFilter = true
			filterReason = "Massive duplicate"
			massiveDuplicates++
		}
		
		// 2. Проверка на пинг-понг если не дубль
		if !shouldFilter && pingPongDetector.AddPoint(currPoint.Position) {
			shouldFilter = true
			filterReason = "Ping-pong pattern"
			pingPongPoints++
		}
		
		// 3. Проверка скорости
		if !shouldFilter && currPoint.Speed > 0 {
			// Абсолютное превышение скорости
			if currPoint.Speed > f.maxReasonableSpeed {
				shouldFilter = true
				filterReason = fmt.Sprintf("Speed violation: %.1f km/h", currPoint.Speed)
				speedViolations++
				if currPoint.Speed > maxSpeedDetected {
					maxSpeedDetected = currPoint.Speed
				}
			}
			
			// Проверка относительно скользящей медианы
			if !shouldFilter {
				medianSpeed := f.calculateSlidingMedianSpeed(track.Points, i)
				if medianSpeed > 0 && currPoint.Speed > medianSpeed*f.speedMultiplierThresh {
					shouldFilter = true
					filterReason = fmt.Sprintf("Speed anomaly: %.1f km/h (%.1fx median)", currPoint.Speed, currPoint.Speed/medianSpeed)
					speedViolations++
					if currPoint.Speed > maxSpeedDetected {
						maxSpeedDetected = currPoint.Speed
					}
				}
			}
		}
		
		// 4. Проверка расстояния (экстремальные телепортации)
		if !shouldFilter {
			distance := prevPoint.Position.DistanceTo(currPoint.Position)
			if distance > 200 { // Сохраняем проверку на экстремальные расстояния
				shouldFilter = true
				filterReason = fmt.Sprintf("Extreme teleportation: %.1f km", distance)
				teleportations++
				if distance > maxJump {
					maxJump = distance
				}
			}
		}
		
		// Применяем фильтрацию
		if shouldFilter {
			currPoint.Filtered = true
			currPoint.FilterReason = filterReason
			
			f.logger.WithFields(map[string]interface{}{
				"index":    i,
				"reason":   filterReason,
				"lat":      currPoint.Position.Latitude,
				"lon":      currPoint.Position.Longitude,
				"speed":    currPoint.Speed,
			}).Debug("Point filtered by smart teleportation filter")
		}
		
		result = append(result, currPoint)
	}

	duration := time.Since(start)
	totalFiltered := teleportations + massiveDuplicates + pingPongPoints + speedViolations

	f.logger.WithFields(map[string]interface{}{
		"device_id":         track.DeviceID,
		"original_count":    originalCount,
		"filtered_count":    totalFiltered,
		"teleportations":    teleportations,
		"massive_dupes":     massiveDuplicates,
		"ping_pong":         pingPongPoints,
		"speed_violations":  speedViolations,
		"max_speed_kmh":     maxSpeedDetected,
		"max_jump_km":       maxJump,
		"duration_ms":       duration.Milliseconds(),
	}).Info("Smart teleportation filter completed")

	return &FilterResult{
		OriginalCount: originalCount,
		FilteredCount: totalFiltered,
		Points:        result,
		Statistics: FilterStats{
			Teleportations:    teleportations + pingPongPoints, // Объединяем телепортации и пинг-понг
			Duplicates:        massiveDuplicates,
			SpeedViolations:   speedViolations,
			MaxSpeedDetected:  maxSpeedDetected,
			MaxDistanceJump:   maxJump,
		},
	}, nil
}

// Name возвращает имя фильтра
func (f *SmartTeleportationFilter) Name() string {
	return "SmartTeleportationFilter"
}

// Description возвращает описание фильтра
func (f *SmartTeleportationFilter) Description() string {
	return fmt.Sprintf("Smart teleportation detection: speed limit %.0f km/h, ping-pong detection, massive duplicates", f.maxReasonableSpeed)
}