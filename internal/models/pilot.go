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
	Addr uint32    `json:"addr"`           // FANET адрес (уникальный ID)
	Name string    `json:"name,omitempty"` // Имя пилота
	Type PilotType `json:"type"`           // Тип летательного аппарата

	// Позиция
	Position GeoPoint `json:"position"` // Текущие координаты
	Altitude int32    `json:"altitude"` // Высота GPS (м)

	// Движение
	Speed  float32 `json:"speed"`  // Скорость (км/ч)
	Climb  float32 `json:"climb"`  // Вертикальная скорость (м/с)
	Course float32 `json:"course"` // Курс (градусы)

	// Статус
	LastUpdate  time.Time `json:"last_update"`            // Время последнего обновления
	TrackOnline bool      `json:"track_online,omitempty"` // Онлайн трекинг
	Battery     uint8     `json:"battery,omitempty"`      // Заряд батареи (%)
}

// Validate проверяет корректность данных пилота
func (p *Pilot) Validate() error {
	if p.Addr == 0 {
		return fmt.Errorf("addr is required")
	}

	if err := p.Position.Validate(); err != nil {
		return fmt.Errorf("position: %w", err)
	}

	// Проверка высоты
	if p.Altitude < -1000 || p.Altitude > 15000 {
		return fmt.Errorf("invalid altitude: %d", p.Altitude)
	}

	// Проверка скорости
	if p.Speed < 0 || p.Speed > 400 {
		return fmt.Errorf("invalid speed: %f", p.Speed)
	}

	// Проверка вариометра (реалистичные значения)
	if p.Climb < -30 || p.Climb > 30 {
		return fmt.Errorf("invalid climb rate: %f", p.Climb)
	}

	// Проверка курса
	if p.Course < 0 || p.Course >= 360 {
		return fmt.Errorf("invalid course: %f", p.Course)
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

// IsGroundSpeed проверяет, является ли скорость наземной
func (p *Pilot) IsGroundSpeed() bool {
	// Для наземных объектов Type будет >= 128
	return p.Type >= 128
}

// GetColor возвращает цвет для отображения на карте
func (p *Pilot) GetColor() string {
	// Простой алгоритм генерации цвета на основе адреса
	hash := p.Addr
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

// ToRedisHash конвертирует пилота в map для Redis HSET
func (p *Pilot) ToRedisHash() map[string]interface{} {
	hash := map[string]interface{}{
		"name":        p.Name,
		"type":        int(p.Type),
		"lat":         p.Position.Latitude,
		"lon":         p.Position.Longitude,
		"altitude":    p.Altitude,
		"speed":       p.Speed,
		"climb":       p.Climb,
		"course":      p.Course,
		"last_update": p.LastUpdate.Unix(),
		"track_online": p.TrackOnline,
	}
	
	if p.Battery > 0 {
		hash["battery"] = p.Battery
	}
	
	return hash
}

// FromRedisHash восстанавливает пилота из Redis hash
func (p *Pilot) FromRedisHash(addr uint32, data map[string]string) error {
	p.Addr = addr
	
	// Парсим данные с обработкой ошибок
	if name, ok := data["name"]; ok {
		p.Name = name
	}
	
	if typeStr, ok := data["type"]; ok {
		var pilotType int
		fmt.Sscanf(typeStr, "%d", &pilotType)
		p.Type = PilotType(pilotType)
	}
	
	if lat, ok := data["lat"]; ok {
		fmt.Sscanf(lat, "%f", &p.Position.Latitude)
	}
	
	if lon, ok := data["lon"]; ok {
		fmt.Sscanf(lon, "%f", &p.Position.Longitude)
	}
	
	if alt, ok := data["altitude"]; ok {
		fmt.Sscanf(alt, "%d", &p.Altitude)
	}
	
	if speed, ok := data["speed"]; ok {
		fmt.Sscanf(speed, "%f", &p.Speed)
	}
	
	if climb, ok := data["climb"]; ok {
		fmt.Sscanf(climb, "%f", &p.Climb)
	}
	
	if course, ok := data["course"]; ok {
		fmt.Sscanf(course, "%f", &p.Course)
	}
	
	if lastUpdate, ok := data["last_update"]; ok {
		var timestamp int64
		fmt.Sscanf(lastUpdate, "%d", &timestamp)
		p.LastUpdate = time.Unix(timestamp, 0)
	}
	
	if trackOnline, ok := data["track_online"]; ok {
		p.TrackOnline = trackOnline == "1" || trackOnline == "true"
	}
	
	if battery, ok := data["battery"]; ok {
		var bat int
		fmt.Sscanf(battery, "%d", &bat)
		p.Battery = uint8(bat)
	}
	
	return p.Validate()
}