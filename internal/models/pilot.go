package models

import (
	"fmt"
	"time"
	
	"github.com/flybeeper/fanet-backend/pkg/pb"
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
	Address      string    `json:"address"`            // FANET адрес (алиас для DeviceID)
	Name         string    `json:"name,omitempty"`     // Имя пилота
	Type         PilotType `json:"type"`               // Тип летательного аппарата
	AircraftType uint8     `json:"aircraft_type"`      // Тип летательного аппарата (legacy)

	// Позиция
	Position *GeoPoint `json:"position"` // Текущие координаты
	Altitude int32    `json:"altitude"` // Высота в метрах

	// Движение
	Speed     float32 `json:"speed"`      // Скорость (км/ч)
	ClimbRate int16  `json:"climb_rate"` // Вертикальная скорость (м/с * 10)
	Heading   float32 `json:"heading"`    // Курс (градусы)

	// Статус
	LastUpdate  time.Time `json:"last_update"`            // Время последнего обновления
	LastSeen    time.Time `json:"last_seen"`              // Время последнего обновления (алиас)
	TrackOnline bool      `json:"track_online,omitempty"` // Онлайн трекинг
	Battery     uint8     `json:"battery,omitempty"`      // Заряд батареи (%)
}

// GetID возвращает уникальный идентификатор для geo.Object
func (p *Pilot) GetID() string {
	if p.Address != "" {
		return p.Address
	}
	return p.DeviceID
}

// GetLatitude возвращает широту для geo.Object
func (p *Pilot) GetLatitude() float64 {
	if p.Position != nil {
		return p.Position.Latitude
	}
	return 0
}

// GetLongitude возвращает долготу для geo.Object
func (p *Pilot) GetLongitude() float64 {
	if p.Position != nil {
		return p.Position.Longitude
	}
	return 0
}

// GetTimestamp возвращает время последнего обновления для geo.Object
func (p *Pilot) GetTimestamp() time.Time {
	if !p.LastSeen.IsZero() {
		return p.LastSeen
	}
	return p.LastUpdate
}

// Validate проверяет корректность данных пилота
func (p *Pilot) Validate() error {
	if p.DeviceID == "" && p.Address == "" {
		return fmt.Errorf("device_id or address is required")
	}

	if p.Position != nil {
		if err := p.Position.Validate(); err != nil {
			return fmt.Errorf("position: %w", err)
		}
	}

	// Проверка высоты
	if p.Altitude < -1000 || p.Altitude > 15000 {
		return fmt.Errorf("invalid altitude: %d", p.Altitude)
	}

	// Проверка скорости
	if p.Speed > 400 {
		return fmt.Errorf("invalid speed: %f", p.Speed)
	}

	// Проверка вариометра (реалистичные значения: -30 до +30 м/с)
	climbMS := float32(p.ClimbRate) / 10.0
	if climbMS < -30 || climbMS > 30 {
		return fmt.Errorf("invalid climb rate: %f m/s", climbMS)
	}

	// Проверка курса
	if p.Heading >= 360 {
		return fmt.Errorf("invalid heading: %f", p.Heading)
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

// ToProto конвертирует Pilot в protobuf представление
func (p *Pilot) ToProto() *pb.Pilot {
	pilot := &pb.Pilot{
		Addr:        0, // TODO: конвертировать DeviceID в uint32
		Name:        p.Name,
		Type:        pb.PilotType(p.Type),
		Altitude:    int32(p.Position.Altitude),
		Speed:       p.Speed,
		Climb:       float32(p.ClimbRate) / 10.0,
		Course:      p.Heading,
		LastUpdate:  p.LastUpdate.Unix(),
		TrackOnline: p.TrackOnline,
		Battery:     uint32(p.Battery),
	}
	
	if p.Position != nil {
		pilot.Position = &pb.GeoPoint{
			Latitude:  p.Position.Latitude,
			Longitude: p.Position.Longitude,
		}
	}
	
	return pilot
}

