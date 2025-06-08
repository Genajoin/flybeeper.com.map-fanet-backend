package models

import (
	"fmt"
	"time"
)

// Station представляет метеостанцию
type Station struct {
	// Идентификация
	ID   string `json:"id"`             // Device ID станции
	Name string `json:"name,omitempty"` // Название станции

	// Позиция
	Position GeoPoint `json:"position"` // Координаты станции

	// Погодные данные
	Temperature   int8   `json:"temperature"`            // Температура (°C)
	WindSpeed     uint8  `json:"wind_speed"`             // Скорость ветра (км/ч)
	WindDirection uint16 `json:"wind_direction"`         // Направление ветра (градусы)
	WindGusts     uint8  `json:"wind_gusts,omitempty"`   // Порывы ветра (км/ч)
	Humidity      uint8  `json:"humidity,omitempty"`     // Влажность (%)
	Pressure      uint16 `json:"pressure,omitempty"`     // Давление (гПа)

	// Статус
	Battery    uint8     `json:"battery,omitempty"` // Заряд батареи (%)
	LastUpdate time.Time `json:"last_update"`       // Время последнего обновления
}

// Validate проверяет корректность данных станции
func (s *Station) Validate() error {
	if s.ID == "" {
		return fmt.Errorf("id is required")
	}

	if err := s.Position.Validate(); err != nil {
		return fmt.Errorf("position: %w", err)
	}

	// Проверка температуры (реалистичные значения)
	if s.Temperature < -60 || s.Temperature > 60 {
		return fmt.Errorf("invalid temperature: %d", s.Temperature)
	}

	// Проверка скорости ветра
	if s.WindSpeed < 0 || s.WindSpeed > 100 {
		return fmt.Errorf("invalid wind speed: %d", s.WindSpeed)
	}

	// Проверка направления ветра
	if s.WindDirection >= 360 {
		return fmt.Errorf("invalid wind direction: %d", s.WindDirection)
	}

	// Проверка порывов ветра
	if s.WindGusts < 0 || s.WindGusts > 150 {
		return fmt.Errorf("invalid wind gusts: %d", s.WindGusts)
	}

	// Проверка влажности
	if s.Humidity > 100 {
		return fmt.Errorf("invalid humidity: %d", s.Humidity)
	}

	// Проверка давления (реалистичные значения на уровне моря)
	if s.Pressure > 0 && (s.Pressure < 900 || s.Pressure > 1100) {
		return fmt.Errorf("invalid pressure: %d", s.Pressure)
	}

	// Проверка батареи
	if s.Battery > 100 {
		return fmt.Errorf("invalid battery level: %d", s.Battery)
	}

	return nil
}

// IsStale проверяет, устарели ли данные станции
func (s *Station) IsStale(maxAge time.Duration) bool {
	return time.Since(s.LastUpdate) > maxAge
}

// GetWindDescription возвращает описание силы ветра по шкале Бофорта
func (s *Station) GetWindDescription() string {
	switch {
	case s.WindSpeed == 0:
		return "calm"
	case s.WindSpeed <= 1:
		return "light air"
	case s.WindSpeed <= 3:
		return "light breeze"
	case s.WindSpeed <= 5:
		return "gentle breeze"
	case s.WindSpeed < 8:
		return "moderate breeze"
	case s.WindSpeed <= 10:
		return "fresh breeze"
	case s.WindSpeed <= 13:
		return "strong breeze"
	case s.WindSpeed < 17:
		return "near gale"
	case s.WindSpeed < 21:
		return "gale"
	case s.WindSpeed < 25:
		return "strong gale"
	case s.WindSpeed < 29:
		return "storm"
	case s.WindSpeed < 33:
		return "violent storm"
	default:
		return "hurricane"
	}
}

// IsFlyable проверяет, подходит ли погода для полетов
func (s *Station) IsFlyable() bool {
	// Базовые критерии для парапланов
	if s.WindSpeed > 10 { // > 10 м/с слишком сильный ветер
		return false
	}
	if s.WindGusts > 15 { // Сильные порывы опасны
		return false
	}
	// Дождь определяется по влажности и температуре (упрощенно)
	if s.Humidity > 95 && s.Temperature > 0 {
		return false
	}
	return true
}

// GetWindDirection возвращает направление ветра в виде компаса
func (s *Station) GetWindDirection() string {
	directions := []string{"N", "NNE", "NE", "ENE", "E", "ESE", "SE", "SSE",
		"S", "SSW", "SW", "WSW", "W", "WNW", "NW", "NNW"}
	
	index := int((float64(s.WindDirection) + 11.25) / 22.5)
	if index >= len(directions) {
		index = 0
	}
	
	return directions[index]
}

// ToRedisHash конвертирует станцию в map для Redis HSET
func (s *Station) ToRedisHash() map[string]interface{} {
	hash := map[string]interface{}{
		"name":         s.Name,
		"lat":          s.Position.Latitude,
		"lon":          s.Position.Longitude,
		"temperature":  s.Temperature,
		"wind_speed":   s.WindSpeed,
		"wind_direction": s.WindDirection,
		"last_update":  s.LastUpdate.Unix(),
	}

	// Опциональные поля
	if s.WindGusts > 0 {
		hash["wind_gusts"] = s.WindGusts
	}
	if s.Humidity > 0 {
		hash["humidity"] = s.Humidity
	}
	if s.Pressure > 0 {
		hash["pressure"] = s.Pressure
	}
	if s.Battery > 0 {
		hash["battery"] = s.Battery
	}

	return hash
}

// FromRedisHash восстанавливает станцию из Redis hash
func (s *Station) FromRedisHash(id string, data map[string]string) error {
	s.ID = id

	// Парсим данные
	if name, ok := data["name"]; ok {
		s.Name = name
	}

	if lat, ok := data["lat"]; ok {
		fmt.Sscanf(lat, "%f", &s.Position.Latitude)
	}

	if lon, ok := data["lon"]; ok {
		fmt.Sscanf(lon, "%f", &s.Position.Longitude)
	}

	if temp, ok := data["temperature"]; ok {
		var t int
		fmt.Sscanf(temp, "%d", &t)
		s.Temperature = int8(t)
	}

	if windSpeed, ok := data["wind_speed"]; ok {
		var ws int
		fmt.Sscanf(windSpeed, "%d", &ws)
		s.WindSpeed = uint8(ws)
	}

	if windDirection, ok := data["wind_direction"]; ok {
		var dir int
		fmt.Sscanf(windDirection, "%d", &dir)
		s.WindDirection = uint16(dir)
	}

	if windGusts, ok := data["wind_gusts"]; ok {
		var wg int
		fmt.Sscanf(windGusts, "%d", &wg)
		s.WindGusts = uint8(wg)
	}

	if humidity, ok := data["humidity"]; ok {
		var h int
		fmt.Sscanf(humidity, "%d", &h)
		s.Humidity = uint8(h)
	}

	if pressure, ok := data["pressure"]; ok {
		var p int
		fmt.Sscanf(pressure, "%d", &p)
		s.Pressure = uint16(p)
	}

	if battery, ok := data["battery"]; ok {
		var b int
		fmt.Sscanf(battery, "%d", &b)
		s.Battery = uint8(b)
	}

	if lastUpdate, ok := data["last_update"]; ok {
		var timestamp int64
		fmt.Sscanf(lastUpdate, "%d", &timestamp)
		s.LastUpdate = time.Unix(timestamp, 0)
	}

	return s.Validate()
}

// WeatherHistory представляет историческую запись погоды
type WeatherHistory struct {
	Timestamp   time.Time `json:"timestamp"`
	Temperature float32   `json:"temperature"`
	WindSpeed   float32   `json:"wind_speed"`
	WindHeading float32   `json:"wind_heading"`
	Humidity    uint8     `json:"humidity,omitempty"`
	Pressure    float32   `json:"pressure,omitempty"`
}

// GetTrend возвращает тренд изменения погоды
func GetWeatherTrend(history []WeatherHistory) map[string]string {
	if len(history) < 2 {
		return map[string]string{
			"temperature": "stable",
			"wind":        "stable",
			"pressure":    "stable",
		}
	}

	trends := make(map[string]string)
	
	// Анализ температуры
	tempDiff := history[len(history)-1].Temperature - history[0].Temperature
	switch {
	case tempDiff > 2:
		trends["temperature"] = "rising"
	case tempDiff < -2:
		trends["temperature"] = "falling"
	default:
		trends["temperature"] = "stable"
	}

	// Анализ ветра
	windDiff := history[len(history)-1].WindSpeed - history[0].WindSpeed
	switch {
	case windDiff > 2:
		trends["wind"] = "increasing"
	case windDiff < -2:
		trends["wind"] = "decreasing"
	default:
		trends["wind"] = "stable"
	}

	// Анализ давления
	if history[0].Pressure > 0 && history[len(history)-1].Pressure > 0 {
		pressureDiff := history[len(history)-1].Pressure - history[0].Pressure
		switch {
		case pressureDiff > 2:
			trends["pressure"] = "rising"
		case pressureDiff < -2:
			trends["pressure"] = "falling"
		default:
			trends["pressure"] = "stable"
		}
	}

	return trends
}