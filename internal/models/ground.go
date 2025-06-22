package models

import (
	"fmt"
	"time"

	"github.com/flybeeper/fanet-backend/pkg/pb"
)

// GroundType тип наземного объекта (согласно FANET спецификации)
type GroundType uint8

const (
	GroundTypeOther                  GroundType = 0  // Другое
	GroundTypeWalking                GroundType = 1  // Пешеход
	GroundTypeVehicle                GroundType = 2  // Транспортное средство
	GroundTypeBike                   GroundType = 3  // Велосипед
	GroundTypeBoot                   GroundType = 4  // Лодка
	GroundTypeNeedRide               GroundType = 8  // Нужна помощь с транспортом
	GroundTypeLandedWell             GroundType = 9  // Успешная посадка
	GroundTypeNeedTechnicalSupport   GroundType = 12 // Нужна техническая помощь
	GroundTypeNeedMedicalHelp        GroundType = 13 // Нужна медицинская помощь
	GroundTypeDistressCall           GroundType = 14 // Сигнал бедствия
	GroundTypeDistressCallAuto       GroundType = 15 // Автоматический сигнал бедствия
)

// String возвращает строковое представление типа
func (t GroundType) String() string {
	switch t {
	case GroundTypeWalking:
		return "walking"
	case GroundTypeVehicle:
		return "vehicle"
	case GroundTypeBike:
		return "bike"
	case GroundTypeBoot:
		return "boot"
	case GroundTypeNeedRide:
		return "need_ride"
	case GroundTypeLandedWell:
		return "landed_well"
	case GroundTypeNeedTechnicalSupport:
		return "need_technical_support"
	case GroundTypeNeedMedicalHelp:
		return "need_medical_help"
	case GroundTypeDistressCall:
		return "distress_call"
	case GroundTypeDistressCallAuto:
		return "distress_call_auto"
	default:
		return "other"
	}
}

// IsEmergency проверяет, является ли тип экстренным
func (t GroundType) IsEmergency() bool {
	switch t {
	case GroundTypeNeedTechnicalSupport, GroundTypeNeedMedicalHelp, 
		 GroundTypeDistressCall, GroundTypeDistressCallAuto:
		return true
	default:
		return false
	}
}

// MarshalBinary реализует encoding.BinaryMarshaler для Redis
func (t GroundType) MarshalBinary() ([]byte, error) {
	return []byte{uint8(t)}, nil
}

// UnmarshalBinary реализует encoding.BinaryUnmarshaler для Redis
func (t *GroundType) UnmarshalBinary(data []byte) error {
	if len(data) != 1 {
		return fmt.Errorf("invalid data length for GroundType: %d", len(data))
	}
	*t = GroundType(data[0])
	return nil
}

// GroundObject представляет наземный объект (FANET Type 7)
type GroundObject struct {
	// Идентификация
	DeviceID     string     `json:"device_id"`          // FANET адрес в hex формате
	Address      string     `json:"address"`            // FANET адрес (алиас для DeviceID)
	Name         string     `json:"name,omitempty"`     // Название объекта
	Type         GroundType `json:"type"`               // Тип наземного объекта

	// Позиция
	Position *GeoPoint `json:"position"` // Текущие координаты

	// Статус
	TrackOnline bool      `json:"track_online,omitempty"` // Онлайн трекинг
	LastUpdate  time.Time `json:"last_update"`            // Время последнего обновления
	LastSeen    time.Time `json:"last_seen"`              // Время последнего обновления (алиас)
}

// GetID возвращает уникальный идентификатор для geo.Object
func (g *GroundObject) GetID() string {
	if g.Address != "" {
		return g.Address
	}
	return g.DeviceID
}

// GetLatitude возвращает широту для geo.Object
func (g *GroundObject) GetLatitude() float64 {
	if g.Position != nil {
		return g.Position.Latitude
	}
	return 0
}

// GetLongitude возвращает долготу для geo.Object
func (g *GroundObject) GetLongitude() float64 {
	if g.Position != nil {
		return g.Position.Longitude
	}
	return 0
}

// GetTimestamp возвращает время последнего обновления для geo.Object
func (g *GroundObject) GetTimestamp() time.Time {
	if !g.LastSeen.IsZero() {
		return g.LastSeen
	}
	return g.LastUpdate
}

// Validate проверяет корректность данных наземного объекта
func (g *GroundObject) Validate() error {
	if g.DeviceID == "" && g.Address == "" {
		return fmt.Errorf("device_id or address is required")
	}

	if g.Position != nil {
		if err := g.Position.Validate(); err != nil {
			return fmt.Errorf("position: %w", err)
		}
	}

	return nil
}

// IsStale проверяет, устарели ли данные
func (g *GroundObject) IsStale(maxAge time.Duration) bool {
	return time.Since(g.LastUpdate) > maxAge
}

// GetColor возвращает цвет для отображения на карте (на основе типа)
func (g *GroundObject) GetColor() string {
	switch g.Type {
	case GroundTypeWalking:
		return "#2E7D32" // Зеленый
	case GroundTypeVehicle:
		return "#1976D2" // Синий
	case GroundTypeBike:
		return "#FF9800" // Оранжевый
	case GroundTypeBoot:
		return "#00BCD4" // Голубой
	case GroundTypeNeedRide:
		return "#FFC107" // Желтый
	case GroundTypeLandedWell:
		return "#4CAF50" // Светло-зеленый
	case GroundTypeNeedTechnicalSupport:
		return "#FF5722" // Красно-оранжевый
	case GroundTypeNeedMedicalHelp:
		return "#F44336" // Красный
	case GroundTypeDistressCall, GroundTypeDistressCallAuto:
		return "#B71C1C" // Темно-красный
	default:
		return "#757575" // Серый
	}
}

// ToProto конвертирует GroundObject в protobuf
func (g *GroundObject) ToProto() *pb.GroundObject {
	groundObject := &pb.GroundObject{
		Addr:        0, // TODO: конвертировать DeviceID в uint32
		Name:        g.Name,
		Type:        pb.GroundType(g.Type),
		TrackOnline: g.TrackOnline,
		LastUpdate:  g.LastUpdate.Unix(),
	}

	if g.Position != nil {
		groundObject.Position = &pb.GeoPoint{
			Latitude:  g.Position.Latitude,
			Longitude: g.Position.Longitude,
		}
	}

	return groundObject
}