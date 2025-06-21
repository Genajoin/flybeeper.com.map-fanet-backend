package filter

import (
	"time"

	"github.com/flybeeper/fanet-backend/internal/models"
)

// TrackPoint представляет точку трека с дополнительной информацией для фильтрации
type TrackPoint struct {
	Position     models.GeoPoint `json:"position"`
	Timestamp    time.Time       `json:"timestamp"`
	Speed        float64         `json:"speed,omitempty"`        // Вычисленная скорость км/ч
	Distance     float64         `json:"distance,omitempty"`     // Расстояние от предыдущей точки в км
	Filtered     bool            `json:"filtered,omitempty"`     // Была ли точка отфильтрована
	FilterReason string          `json:"filter_reason,omitempty"` // Причина фильтрации
	SegmentID    int             `json:"segment_id,omitempty"`    // ID сегмента (для разрывов во времени)
}

// TrackData содержит информацию о треке для фильтрации
type TrackData struct {
	DeviceID     string       `json:"device_id"`
	AircraftType models.PilotType `json:"aircraft_type"`
	Points       []TrackPoint `json:"points"`
}

// FilterResult результат фильтрации
type FilterResult struct {
	OriginalCount int          `json:"original_count"`
	FilteredCount int          `json:"filtered_count"`
	Points        []TrackPoint `json:"points"`
	Statistics    FilterStats  `json:"statistics"`
}

// FilterStats статистика фильтрации
type FilterStats struct {
	SpeedViolations   int           `json:"speed_violations"`
	Duplicates        int           `json:"duplicates"`
	Outliers          int           `json:"outliers"`
	Teleportations    int           `json:"teleportations,omitempty"` // Количество телепортаций
	MaxSpeedDetected  float64       `json:"max_speed_detected"`
	AvgSpeed          float64       `json:"avg_speed"`
	MaxDistanceJump   float64       `json:"max_distance_jump"`
	SegmentCount      int           `json:"segment_count,omitempty"`   // Количество сегментов
	SegmentBreaks     int           `json:"segment_breaks,omitempty"`  // Количество разрывов
	Segments          []SegmentInfo `json:"segments,omitempty"`        // Информация о сегментах
}

// TrackFilter интерфейс для фильтров треков
type TrackFilter interface {
	// Filter применяет фильтр к трeku
	Filter(track *TrackData) (*FilterResult, error)
	
	// Name возвращает имя фильтра
	Name() string
	
	// Description возвращает описание фильтра
	Description() string
}

// FilterConfig конфигурация фильтров
type FilterConfig struct {
	// Максимальные скорости для типов ЛА (км/ч)
	MaxSpeeds map[models.PilotType]float64 `json:"max_speeds"`
	
	// Буферный коэффициент для максимальной скорости (например, 1.5 = +50%)
	SpeedBuffer float64 `json:"speed_buffer"`
	
	// Минимальное расстояние между точками для удаления дублей (м)
	MinDistanceMeters float64 `json:"min_distance_meters"`
	
	// Минимальный интервал времени между точками (секунды)
	MinTimeInterval time.Duration `json:"min_time_interval"`
	
	// Максимальное отклонение для определения выбросов (км)
	OutlierThresholdKm float64 `json:"outlier_threshold_km"`
	
	// Включить/выключить отдельные фильтры
	EnableSpeedFilter     bool `json:"enable_speed_filter"`
	EnableDuplicateFilter bool `json:"enable_duplicate_filter"`
	EnableOutlierFilter   bool `json:"enable_outlier_filter"`
}

// DefaultFilterConfig возвращает конфигурацию по умолчанию
func DefaultFilterConfig() *FilterConfig {
	return &FilterConfig{
		MaxSpeeds: map[models.PilotType]float64{
			models.PilotTypeUnknown:    150, // Консервативный подход для неизвестных
			models.PilotTypeParaglider: 80,  // Параплан
			models.PilotTypeHangglider: 120, // Дельтаплан  
			models.PilotTypeBalloon:    50,  // Воздушный шар
			models.PilotTypeGlider:     200, // Планер
			models.PilotTypePowered:    300, // Мотопараплан/самолет
			models.PilotTypeHelicopter: 250, // Вертолет
			models.PilotTypeUAV:        100, // Дрон
		},
		SpeedBuffer:           1.5,                // +50% буфер
		MinDistanceMeters:     10,                 // 10 метров минимум между точками
		MinTimeInterval:       5 * time.Second,    // 5 секунд минимум между точками
		OutlierThresholdKm:    50,                 // 50км максимальный "прыжок"
		EnableSpeedFilter:     true,
		EnableDuplicateFilter: true,
		EnableOutlierFilter:   true,
	}
}

// GetMaxSpeed возвращает максимальную скорость для типа ЛА
func (c *FilterConfig) GetMaxSpeed(aircraftType models.PilotType) float64 {
	if speed, ok := c.MaxSpeeds[aircraftType]; ok {
		return speed * c.SpeedBuffer
	}
	// Fallback для неизвестных типов
	return c.MaxSpeeds[models.PilotTypeUnknown] * c.SpeedBuffer
}