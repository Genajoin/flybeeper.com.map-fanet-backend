package filter

import (
	"fmt"
	"time"

	"github.com/flybeeper/fanet-backend/internal/models"
	"github.com/flybeeper/fanet-backend/pkg/utils"
)

// TwoStageFilterChain двухэтапная цепочка фильтров
type TwoStageFilterChain struct {
	stage1Filters []TrackFilter // Первичная очистка и сегментация
	stage2Filters []TrackFilter // Адаптивная фильтрация внутри сегментов
	config        *FilterConfig
	logger        *utils.Logger
}

// NewTwoStageFilterChain создает новую двухэтапную цепочку фильтров
func NewTwoStageFilterChain(config *FilterConfig, logger *utils.Logger) *TwoStageFilterChain {
	chain := &TwoStageFilterChain{
		stage1Filters: make([]TrackFilter, 0),
		stage2Filters: make([]TrackFilter, 0),
		config:        config,
		logger:        logger,
	}

	// Настраиваем первую стадию - очистка и первичная сегментация
	chain.setupStage1()
	
	// Настраиваем вторую стадию - адаптивная фильтрация
	chain.setupStage2()

	return chain
}

// setupStage1 настраивает фильтры первой стадии
func (fc *TwoStageFilterChain) setupStage1() {
	// 1. Мягкое удаление дублей (только явные дубли)
	if fc.config.EnableDuplicateFilter {
		// Создаем конфигурацию для мягкого режима
		gentleConfig := &FilterConfig{
			MinDistanceMeters: 10.0,              // Только точки ближе 10м считаются дублями
			MinTimeInterval:   5 * time.Second,   // Минимальный интервал 5 секунд
		}
		fc.stage1Filters = append(fc.stage1Filters, NewDuplicateFilter(gentleConfig, fc.logger))
	}

	// 2. Сегментация по временным разрывам (30 минут)
	fc.stage1Filters = append(fc.stage1Filters, NewTimeGapSegmentationFilter(fc.config, fc.logger, 30))
	
	// 3. Удаление явных телепортаций между сегментами
	teleportConfig := &FilterConfig{
		OutlierThresholdKm:    200.0, // Явные телепортации > 200км
		EnableOutlierFilter:   true,
		EnableSpeedFilter:     true,
		EnableDuplicateFilter: false,
		MaxSpeeds: map[models.PilotType]float64{
			models.PilotTypeUnknown:    500, // Консервативный максимум для телепортаций
			models.PilotTypeParaglider: 500,
			models.PilotTypeHangglider: 500,
			models.PilotTypeBalloon:    500,
			models.PilotTypeGlider:     500,
			models.PilotTypePowered:    500,
			models.PilotTypeHelicopter: 500,
			models.PilotTypeUAV:        500,
		},
	}
	fc.stage1Filters = append(fc.stage1Filters, NewCrossSegmentFilter(teleportConfig, fc.logger))
}

// setupStage2 настраивает фильтры второй стадии
func (fc *TwoStageFilterChain) setupStage2() {
	// 1. Локальная фильтрация выбросов внутри сегментов
	if fc.config.EnableOutlierFilter {
		fc.stage2Filters = append(fc.stage2Filters, NewLocalOutlierFilter(fc.config, fc.logger))
	}

	// 2. Фильтрация по скорости с учетом типа ЛА
	if fc.config.EnableSpeedFilter {
		fc.stage2Filters = append(fc.stage2Filters, NewSpeedBasedFilter(fc.config, fc.logger))
	}

	// 3. Классификация активности и micro-segmentation
	fc.stage2Filters = append(fc.stage2Filters, NewSegmentationFilter(fc.config, fc.logger))
}

// Filter применяет двухэтапную фильтрацию
func (fc *TwoStageFilterChain) Filter(track *TrackData) (*FilterResult, error) {
	if len(track.Points) == 0 {
		return &FilterResult{
			OriginalCount: 0,
			FilteredCount: 0,
			Points:        []TrackPoint{},
			Statistics:    FilterStats{},
		}, nil
	}

	fc.logger.WithField("device_id", track.DeviceID).
		WithField("original_points", len(track.Points)).
		WithField("stage1_filters", len(fc.stage1Filters)).
		WithField("stage2_filters", len(fc.stage2Filters)).
		Info("Starting two-stage track filtering")

	originalCount := len(track.Points)
	currentTrack := *track // Копируем трек
	combinedStats := FilterStats{}

	// СТАДИЯ 1: Первичная очистка и сегментация
	fc.logger.Debug("Starting Stage 1: Primary cleanup and segmentation")
	
	stage1Result, err := fc.applyStage(fc.stage1Filters, &currentTrack, "Stage1")
	if err != nil {
		return nil, fmt.Errorf("stage 1 failed: %w", err)
	}
	
	// Обновляем трек результатами первой стадии
	currentTrack.Points = stage1Result.Points
	combinedStats = fc.mergeStats(combinedStats, stage1Result.Statistics)
	
	fc.logger.WithField("stage1_filtered", stage1Result.FilteredCount).
		WithField("stage1_remaining", len(stage1Result.Points)).
		WithField("segments_detected", stage1Result.Statistics.SegmentCount).
		Info("Stage 1 completed")

	// Если после первой стадии не осталось сегментов, возвращаем результат
	if len(stage1Result.Statistics.Segments) == 0 {
		return stage1Result, nil
	}

	// СТАДИЯ 2: Адаптивная фильтрация внутри сегментов
	fc.logger.Debug("Starting Stage 2: Adaptive filtering within segments")
	
	stage2Result, err := fc.applyStage(fc.stage2Filters, &currentTrack, "Stage2")
	if err != nil {
		return nil, fmt.Errorf("stage 2 failed: %w", err)
	}
	
	// Обновляем финальную статистику
	finalStats := fc.mergeStats(combinedStats, stage2Result.Statistics)
	
	// Вычисляем финальные метрики
	finalCount := len(stage2Result.Points)
	totalFiltered := originalCount - finalCount
	
	// Вычисляем среднюю скорость для оставшихся точек
	if finalCount > 1 {
		totalSpeed := 0.0
		validSpeedPoints := 0
		
		for _, point := range stage2Result.Points {
			if point.Speed > 0 && !point.Filtered {
				totalSpeed += point.Speed
				validSpeedPoints++
			}
		}
		
		if validSpeedPoints > 0 {
			finalStats.AvgSpeed = totalSpeed / float64(validSpeedPoints)
		}
	}

	result := &FilterResult{
		OriginalCount: originalCount,
		FilteredCount: totalFiltered,
		Points:        stage2Result.Points,
		Statistics:    finalStats,
	}

	fc.logger.WithField("device_id", track.DeviceID).
		WithField("original_count", originalCount).
		WithField("total_filtered", totalFiltered).
		WithField("final_count", finalCount).
		WithField("final_segments", len(finalStats.Segments)).
		WithField("efficiency", fmt.Sprintf("%.1f%%", float64(totalFiltered)/float64(originalCount)*100)).
		Info("Two-stage filtering completed")

	return result, nil
}

// applyStage применяет список фильтров как одну стадию
func (fc *TwoStageFilterChain) applyStage(filters []TrackFilter, track *TrackData, stageName string) (*FilterResult, error) {
	if len(filters) == 0 {
		return &FilterResult{
			OriginalCount: len(track.Points),
			FilteredCount: 0,
			Points:        track.Points,
			Statistics:    FilterStats{},
		}, nil
	}

	stageTrack := *track // Копируем трек для стадии
	stageStats := FilterStats{}
	stageOriginalCount := len(track.Points)

	for _, filter := range filters {
		start := time.Now()
		
		result, err := filter.Filter(&stageTrack)
		if err != nil {
			fc.logger.WithField("filter", filter.Name()).
				WithField("stage", stageName).
				WithField("error", err).
				Error("Filter failed")
			continue
		}

		duration := time.Since(start)
		
		fc.logger.WithField("filter", filter.Name()).
			WithField("stage", stageName).
			WithField("input_points", len(stageTrack.Points)).
			WithField("output_points", len(result.Points)).
			WithField("filtered_points", result.FilteredCount).
			WithField("duration_ms", duration.Milliseconds()).
			Debug("Filter applied")

		// Обновляем трек для следующего фильтра
		stageTrack.Points = result.Points
		
		// Объединяем статистику
		stageStats = fc.mergeStats(stageStats, result.Statistics)
	}

	return &FilterResult{
		OriginalCount: stageOriginalCount,
		FilteredCount: stageOriginalCount - len(stageTrack.Points),
		Points:        stageTrack.Points,
		Statistics:    stageStats,
	}, nil
}

// mergeStats объединяет статистику из двух результатов
func (fc *TwoStageFilterChain) mergeStats(stats1, stats2 FilterStats) FilterStats {
	merged := FilterStats{
		SpeedViolations: stats1.SpeedViolations + stats2.SpeedViolations,
		Duplicates:      stats1.Duplicates + stats2.Duplicates,
		Outliers:        stats1.Outliers + stats2.Outliers,
	}

	// Сегменты берем из последнего результата (если есть)
	if len(stats2.Segments) > 0 {
		merged.Segments = stats2.Segments
		merged.SegmentCount = stats2.SegmentCount
		merged.SegmentBreaks = stats2.SegmentBreaks
	} else if len(stats1.Segments) > 0 {
		merged.Segments = stats1.Segments
		merged.SegmentCount = stats1.SegmentCount
		merged.SegmentBreaks = stats1.SegmentBreaks
	}

	// Максимальные значения
	if stats2.MaxSpeedDetected > merged.MaxSpeedDetected {
		merged.MaxSpeedDetected = stats2.MaxSpeedDetected
	}
	if stats1.MaxSpeedDetected > merged.MaxSpeedDetected {
		merged.MaxSpeedDetected = stats1.MaxSpeedDetected
	}

	if stats2.MaxDistanceJump > merged.MaxDistanceJump {
		merged.MaxDistanceJump = stats2.MaxDistanceJump
	}
	if stats1.MaxDistanceJump > merged.MaxDistanceJump {
		merged.MaxDistanceJump = stats1.MaxDistanceJump
	}

	// Средние значения (берем последнее вычисленное)
	if stats2.AvgSpeed > 0 {
		merged.AvgSpeed = stats2.AvgSpeed
	} else if stats1.AvgSpeed > 0 {
		merged.AvgSpeed = stats1.AvgSpeed
	}

	return merged
}

// Name возвращает имя цепочки фильтров
func (fc *TwoStageFilterChain) Name() string {
	return "TwoStageFilterChain"
}

// Description возвращает описание цепочки фильтров
func (fc *TwoStageFilterChain) Description() string {
	stage1Names := make([]string, len(fc.stage1Filters))
	for i, filter := range fc.stage1Filters {
		stage1Names[i] = filter.Name()
	}
	
	stage2Names := make([]string, len(fc.stage2Filters))
	for i, filter := range fc.stage2Filters {
		stage2Names[i] = filter.Name()
	}
	
	return fmt.Sprintf("Two-stage filter chain - Stage1: %v, Stage2: %v", stage1Names, stage2Names)
}