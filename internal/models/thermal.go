package models

import (
	"fmt"
	"time"
)

// Thermal представляет термический поток
type Thermal struct {
	// Идентификация
	ID   uint64 `json:"id"`   // Уникальный ID
	Addr uint32 `json:"addr"` // Кто обнаружил

	// Позиция
	Position GeoPoint `json:"position"` // Координаты центра
	Altitude int32    `json:"altitude"` // Высота термика (м)

	// Характеристики
	Quality uint8   `json:"quality"` // Качество 0-5
	Climb   float32 `json:"climb"`   // Средняя скороподъемность (м/с)

	// Ветер на высоте
	WindSpeed   float32 `json:"wind_speed"`   // Скорость ветра (м/с)
	WindHeading float32 `json:"wind_heading"` // Направление ветра (градусы)

	// Метаданные
	Timestamp time.Time `json:"timestamp"` // Время создания
}

// Validate проверяет корректность данных термика
func (t *Thermal) Validate() error {
	if t.ID == 0 {
		return fmt.Errorf("id is required")
	}

	if t.Addr == 0 {
		return fmt.Errorf("addr is required")
	}

	if err := t.Position.Validate(); err != nil {
		return fmt.Errorf("position: %w", err)
	}

	// Проверка высоты
	if t.Altitude < 0 || t.Altitude > 10000 {
		return fmt.Errorf("invalid altitude: %d", t.Altitude)
	}

	// Проверка качества
	if t.Quality > 5 {
		return fmt.Errorf("invalid quality: %d", t.Quality)
	}

	// Проверка скороподъемности (реалистичные значения)
	if t.Climb < -5 || t.Climb > 20 {
		return fmt.Errorf("invalid climb rate: %f", t.Climb)
	}

	// Проверка скорости ветра
	if t.WindSpeed < 0 || t.WindSpeed > 100 {
		return fmt.Errorf("invalid wind speed: %f", t.WindSpeed)
	}

	// Проверка направления ветра
	if t.WindHeading < 0 || t.WindHeading >= 360 {
		return fmt.Errorf("invalid wind heading: %f", t.WindHeading)
	}

	return nil
}

// IsStale проверяет, устарели ли данные термика
func (t *Thermal) IsStale(maxAge time.Duration) bool {
	return time.Since(t.Timestamp) > maxAge
}

// IsStrong проверяет, является ли термик сильным
func (t *Thermal) IsStrong() bool {
	return t.Quality >= 4 || t.Climb >= 3.0
}

// GetQualityDescription возвращает описание качества термика
func (t *Thermal) GetQualityDescription() string {
	switch t.Quality {
	case 0:
		return "very weak"
	case 1:
		return "weak"
	case 2:
		return "moderate"
	case 3:
		return "good"
	case 4:
		return "strong"
	case 5:
		return "excellent"
	default:
		return "unknown"
	}
}

// ToRedisHash конвертирует термик в map для Redis HSET
func (t *Thermal) ToRedisHash() map[string]interface{} {
	return map[string]interface{}{
		"addr":         t.Addr,
		"lat":          t.Position.Latitude,
		"lon":          t.Position.Longitude,
		"altitude":     t.Altitude,
		"quality":      t.Quality,
		"climb":        t.Climb,
		"wind_speed":   t.WindSpeed,
		"wind_heading": t.WindHeading,
		"timestamp":    t.Timestamp.Unix(),
	}
}

// FromRedisHash восстанавливает термик из Redis hash
func (t *Thermal) FromRedisHash(id uint64, data map[string]string) error {
	t.ID = id

	// Парсим данные
	if addr, ok := data["addr"]; ok {
		fmt.Sscanf(addr, "%d", &t.Addr)
	}

	if lat, ok := data["lat"]; ok {
		fmt.Sscanf(lat, "%f", &t.Position.Latitude)
	}

	if lon, ok := data["lon"]; ok {
		fmt.Sscanf(lon, "%f", &t.Position.Longitude)
	}

	if alt, ok := data["altitude"]; ok {
		fmt.Sscanf(alt, "%d", &t.Altitude)
	}

	if quality, ok := data["quality"]; ok {
		var q int
		fmt.Sscanf(quality, "%d", &q)
		t.Quality = uint8(q)
	}

	if climb, ok := data["climb"]; ok {
		fmt.Sscanf(climb, "%f", &t.Climb)
	}

	if windSpeed, ok := data["wind_speed"]; ok {
		fmt.Sscanf(windSpeed, "%f", &t.WindSpeed)
	}

	if windHeading, ok := data["wind_heading"]; ok {
		fmt.Sscanf(windHeading, "%f", &t.WindHeading)
	}

	if timestamp, ok := data["timestamp"]; ok {
		var ts int64
		fmt.Sscanf(timestamp, "%d", &ts)
		t.Timestamp = time.Unix(ts, 0)
	}

	return t.Validate()
}

// GenerateID генерирует уникальный ID для термика на основе позиции и времени
func GenerateThermalID(pos GeoPoint, timestamp time.Time) uint64 {
	// Простой алгоритм: комбинация geohash и timestamp
	geohash := pos.Geohash(7) // Точность ~150м
	
	// Берем первые 8 байт geohash
	var hash uint64
	for i := 0; i < len(geohash) && i < 8; i++ {
		hash = (hash << 8) | uint64(geohash[i])
	}
	
	// Добавляем временную компоненту (минуты с начала дня)
	minutes := uint64(timestamp.Hour()*60 + timestamp.Minute())
	
	return (hash << 16) | minutes
}

// MergeThermals объединяет близкие термики в один
func MergeThermals(thermals []Thermal, mergeRadius float64) []Thermal {
	if len(thermals) <= 1 {
		return thermals
	}

	merged := make([]Thermal, 0, len(thermals))
	used := make(map[int]bool)

	for i := 0; i < len(thermals); i++ {
		if used[i] {
			continue
		}

		// Начинаем с текущего термика
		result := thermals[i]
		count := 1.0
		sumClimb := result.Climb
		sumQuality := float32(result.Quality)

		// Ищем близкие термики
		for j := i + 1; j < len(thermals); j++ {
			if used[j] {
				continue
			}

			distance := result.Position.DistanceTo(thermals[j].Position)
			if distance <= mergeRadius {
				// Объединяем
				used[j] = true
				count++
				sumClimb += thermals[j].Climb
				sumQuality += float32(thermals[j].Quality)

				// Обновляем позицию как среднее
				result.Position.Latitude = (result.Position.Latitude + thermals[j].Position.Latitude) / 2
				result.Position.Longitude = (result.Position.Longitude + thermals[j].Position.Longitude) / 2
				
				// Берем максимальную высоту
				if thermals[j].Altitude > result.Altitude {
					result.Altitude = thermals[j].Altitude
				}

				// Обновляем ветер (берем от более качественного термика)
				if thermals[j].Quality > result.Quality {
					result.WindSpeed = thermals[j].WindSpeed
					result.WindHeading = thermals[j].WindHeading
				}

				// Обновляем время на более свежее
				if thermals[j].Timestamp.After(result.Timestamp) {
					result.Timestamp = thermals[j].Timestamp
				}
			}
		}

		// Усредняем характеристики
		result.Climb = sumClimb / float32(count)
		result.Quality = uint8(sumQuality / float32(count))

		merged = append(merged, result)
	}

	return merged
}