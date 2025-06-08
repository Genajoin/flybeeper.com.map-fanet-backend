package models

import (
	"fmt"
	"time"
)

// PilotType тип летательного аппарата
type PilotType uint8

const (
	PilotTypeUnknown    PilotType = 0
	PilotTypeParaglider PilotType = 1 // Параплан
	PilotTypeHangglider PilotType = 2 // Дельтаплан
	PilotTypeBalloon    PilotType = 3 // Воздушный шар
	PilotTypeGlider     PilotType = 4 // Планер
	PilotTypePowered    PilotType = 5 // Мотопараплан
	PilotTypeHelicopter PilotType = 6 // Вертолет
	PilotTypeUAV        PilotType = 7 // Дрон
)

// String возвращает строковое представление типа
func (t PilotType) String() string {
	switch t {
	case PilotTypeParaglider:
		return "paraglider"
	case PilotTypeHangglider:
		return "hangglider"
	case PilotTypeBalloon:
		return "balloon"
	case PilotTypeGlider:
		return "glider"
	case PilotTypePowered:
		return "powered"
	case PilotTypeHelicopter:
		return "helicopter"
	case PilotTypeUAV:
		return "uav"
	default:
		return "unknown"
	}
}

// Pilot представляет летающий объект
type Pilot struct {
	// Идентификация
	DeviceID     string    `json:"device_id"`          // FANET адрес в hex формате
	Name         string    `json:"name,omitempty"`     // Имя пилота
	AircraftType uint8     `json:"aircraft_type"`      // Тип летательного аппарата

	// Позиция
	Position GeoPoint `json:"position"` // Текущие координаты

	// Движение
	Speed     uint16 `json:"speed"`      // Скорость (км/ч)
	ClimbRate int16  `json:"climb_rate"` // Вертикальная скорость (м/с * 10)
	Heading   uint16 `json:"heading"`    // Курс (градусы)

	// Статус
	LastUpdate  time.Time `json:"last_update"`            // Время последнего обновления
	TrackOnline bool      `json:"track_online,omitempty"` // Онлайн трекинг
	Battery     uint8     `json:"battery,omitempty"`      // Заряд батареи (%)
}

// Validate проверяет корректность данных пилота
func (p *Pilot) Validate() error {
	if p.DeviceID == "" {
		return fmt.Errorf("device_id is required")
	}

	if err := p.Position.Validate(); err != nil {
		return fmt.Errorf("position: %w", err)
	}

	// Проверка высоты
	if p.Position.Altitude < -1000 || p.Position.Altitude > 15000 {
		return fmt.Errorf("invalid altitude: %d", p.Position.Altitude)
	}

	// Проверка скорости
	if p.Speed > 400 {
		return fmt.Errorf("invalid speed: %d", p.Speed)
	}

	// Проверка вариометра (реалистичные значения: -30 до +30 м/с)
	climbMS := float32(p.ClimbRate) / 10.0
	if climbMS < -30 || climbMS > 30 {
		return fmt.Errorf("invalid climb rate: %f m/s", climbMS)
	}

	// Проверка курса
	if p.Heading >= 360 {
		return fmt.Errorf("invalid heading: %d", p.Heading)
	}

	// Проверка батареи
	if p.Battery > 100 {
		return fmt.Errorf("invalid battery level: %d", p.Battery)
	}

	return nil
}

// IsStale проверяет, устарели ли данные
func (p *Pilot) IsStale(maxAge time.Duration) bool {
	return time.Since(p.LastUpdate) > maxAge
}


// GetColor возвращает цвет для отображения на карте
func (p *Pilot) GetColor() string {
	// Простой алгоритм генерации цвета на основе device ID
	hash := uint32(0)
	for _, b := range p.DeviceID {
		hash = hash*31 + uint32(b)
	}
	
	r := uint8(hash & 0xFF)
	g := uint8((hash >> 8) & 0xFF)
	b := uint8((hash >> 16) & 0xFF)
	
	// Обеспечиваем минимальную яркость
	if r < 64 && g < 64 && b < 64 {
		r += 64
		g += 64
		b += 64
	}
	
	return fmt.Sprintf("#%02X%02X%02X", r, g, b)
}

