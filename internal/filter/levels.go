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
// Использует двухэтапную фильтрацию
func NewLevel3FilterChain(config *FilterConfig, logger *utils.Logger) TrackFilter {
	// Используем существующую двухэтапную фильтрацию
	return NewImprovedFilterChain(config, logger)
}