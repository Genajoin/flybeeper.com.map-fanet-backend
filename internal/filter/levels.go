package filter

import (
	"github.com/flybeeper/fanet-backend/pkg/utils"
)

// NewLevel1FilterChain создает базовую цепочку фильтров (уровень 1)
// Удаляет только дубли и явные телепортации (>200км)
func NewLevel1FilterChain(config *FilterConfig, logger *utils.Logger) *FilterChain {
	chain := &FilterChain{
		filters: make([]TrackFilter, 0),
		config:  config,
		logger:  logger,
	}
	
	// 1. Удаляем дубли координат
	chain.AddFilter(NewDuplicateFilter(config, logger))
	
	// 2. Умная фильтрация телепортаций, пинг-понга и массовых дублей
	chain.AddFilter(NewSmartTeleportationFilter(config, logger))
	
	return chain
}

// NewLevel2FilterChain создает среднюю цепочку фильтров (уровень 2)
// Сначала очищает граничные выбросы, затем сегментирует по времени и применяет Level 1 фильтры к каждому сегменту
func NewLevel2FilterChain(config *FilterConfig, logger *utils.Logger) *FilterChain {
	chain := &FilterChain{
		filters: make([]TrackFilter, 0),
		config:  config,
		logger:  logger,
	}
	
	// 1. Предварительная очистка граничных выбросов
	chain.AddFilter(NewPreCleanupFilter(config, logger))
	
	// 2. Сегментируем по временным разрывам (30 минут)
	chain.AddFilter(NewTimeGapSegmentationFilter(config, logger, 30))
	
	// 3. Применяем Level 1 фильтры к каждому сегменту независимо
	chain.AddFilter(NewSegmentedFilterChain(config, logger))
	
	return chain
}

// NewLevel3FilterChain создает полную цепочку фильтров (уровень 3)
// Применяет Level 2 фильтрацию и дополнительно сегментирует по активности (скорости)
func NewLevel3FilterChain(config *FilterConfig, logger *utils.Logger) TrackFilter {
	chain := &FilterChain{
		filters: make([]TrackFilter, 0),
		config:  config,
		logger:  logger,
	}
	
	// 1. Сначала применяем уровень 2 (предочистка + временная сегментация + level 1 к сегментам)
	level2Chain := NewLevel2FilterChain(config, logger)
	
	// Добавляем все фильтры из уровня 2
	for _, filter := range level2Chain.filters {
		chain.AddFilter(filter)
	}
	
	// 2. Дополнительно разбиваем по активности (пешеход vs полет)
	chain.AddFilter(NewActivitySegmentationFilter(config, logger))
	
	return chain
}