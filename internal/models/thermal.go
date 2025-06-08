package models

import (
	"fmt"
	"time"
	
	"github.com/flybeeper/fanet-backend/pkg/pb"
)

// Thermal представляет термический поток
type Thermal struct {
	// Идентификация
	ID         string `json:"id"`          // Уникальный ID
	ReportedBy string `json:"reported_by"` // Device ID кто обнаружил

	// Позиция
	Center     GeoPoint  `json:"center"`     // Координаты центра
	Position   *GeoPoint `json:"position"`   // Координаты центра (для совместимости)
	Altitude   int32     `json:"altitude"`   // Высота термика (м)

	// Характеристики
	Quality    int32   `json:"quality"`      // Качество 0-5
	ClimbRate  float32 `json:"climb_rate"`   // Средняя скороподъемность (м/с)
	PilotCount int32   `json:"pilot_count"`  // Количество пилотов в термике

	// Ветер на высоте
	WindSpeed     uint8  `json:"wind_speed"`     // Скорость ветра (км/ч)
	WindDirection uint16 `json:"wind_direction"` // Направление ветра (градусы)

	// Метаданные
	Timestamp time.Time `json:"timestamp"` // Время создания
	LastSeen  time.Time `json:"last_seen"` // Время последнего обновления
}

// GetID возвращает уникальный идентификатор для geo.Object
func (t *Thermal) GetID() string {
	return t.ID
}

// GetLatitude возвращает широту для geo.Object
func (t *Thermal) GetLatitude() float64 {
	if t.Position != nil {
		return t.Position.Latitude
	}
	return t.Center.Latitude
}

// GetLongitude возвращает долготу для geo.Object
func (t *Thermal) GetLongitude() float64 {
	if t.Position != nil {
		return t.Position.Longitude
	}
	return t.Center.Longitude
}

// GetTimestamp возвращает время последнего обновления для geo.Object
func (t *Thermal) GetTimestamp() time.Time {
	if !t.LastSeen.IsZero() {
		return t.LastSeen
	}
	return t.Timestamp
}

// Validate проверяет корректность данных термика
func (t *Thermal) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("id is required")
	}

	if t.ReportedBy == "" {
		return fmt.Errorf("reported_by is required")
	}

	if err := t.Center.Validate(); err != nil {
		return fmt.Errorf("center: %w", err)
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
	if t.ClimbRate < -500 || t.ClimbRate > 2000 {
		return fmt.Errorf("invalid climb rate: %f", t.ClimbRate)
	}

	// Проверка скорости ветра
	if t.WindSpeed > 100 {
		return fmt.Errorf("invalid wind speed: %d", t.WindSpeed)
	}

	// Проверка направления ветра
	if t.WindDirection >= 360 {
		return fmt.Errorf("invalid wind direction: %d", t.WindDirection)
	}

	return nil
}

// IsStale проверяет, устарели ли данные термика
func (t *Thermal) IsStale(maxAge time.Duration) bool {
	return time.Since(t.Timestamp) > maxAge
}

// IsStrong проверяет, является ли термик сильным
func (t *Thermal) IsStrong() bool {
	return t.Quality >= 4 || t.ClimbRate >= 300
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
		"reported_by":     t.ReportedBy,
		"lat":             t.Center.Latitude,
		"lon":             t.Center.Longitude,
		"altitude":        t.Altitude,
		"quality":         t.Quality,
		"climb_rate":      t.ClimbRate,
		"wind_speed":      t.WindSpeed,
		"wind_direction":  t.WindDirection,
		"timestamp":       t.Timestamp.Unix(),
	}
}

// FromRedisHash восстанавливает термик из Redis hash
func (t *Thermal) FromRedisHash(id string, data map[string]string) error {
	t.ID = id

	// Парсим данные
	if reportedBy, ok := data["reported_by"]; ok {
		t.ReportedBy = reportedBy
	}

	if lat, ok := data["lat"]; ok {
		fmt.Sscanf(lat, "%f", &t.Center.Latitude)
	}

	if lon, ok := data["lon"]; ok {
		fmt.Sscanf(lon, "%f", &t.Center.Longitude)
	}

	if alt, ok := data["altitude"]; ok {
		fmt.Sscanf(alt, "%d", &t.Altitude)
	}

	if quality, ok := data["quality"]; ok {
		var q int
		fmt.Sscanf(quality, "%d", &q)
		t.Quality = int32(q)
	}

	if climb, ok := data["climb_rate"]; ok {
		var c int
		fmt.Sscanf(climb, "%d", &c)
		t.ClimbRate = float32(c)
	}

	if windSpeed, ok := data["wind_speed"]; ok {
		var ws int
		fmt.Sscanf(windSpeed, "%d", &ws)
		t.WindSpeed = uint8(ws)
	}

	if windDirection, ok := data["wind_direction"]; ok {
		var wd int
		fmt.Sscanf(windDirection, "%d", &wd)
		t.WindDirection = uint16(wd)
	}

	if timestamp, ok := data["timestamp"]; ok {
		var ts int64
		fmt.Sscanf(timestamp, "%d", &ts)
		t.Timestamp = time.Unix(ts, 0)
	}

	return t.Validate()
}

// GenerateID генерирует уникальный ID для термика на основе позиции и времени
func GenerateThermalID(pos GeoPoint, timestamp time.Time) string {
	// Простой алгоритм: комбинация geohash и timestamp
	geohash := pos.Geohash(7) // Точность ~150м
	
	// Добавляем временную компоненту (минуты с начала дня)
	minutes := timestamp.Hour()*60 + timestamp.Minute()
	
	return fmt.Sprintf("%s_%04d", geohash, minutes)
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
		sumClimb := float32(result.ClimbRate)
		sumQuality := float32(result.Quality)

		// Ищем близкие термики
		for j := i + 1; j < len(thermals); j++ {
			if used[j] {
				continue
			}

			distance := result.Center.DistanceTo(thermals[j].Center)
			if distance <= mergeRadius {
				// Объединяем
				used[j] = true
				count++
				sumClimb += float32(thermals[j].ClimbRate)
				sumQuality += float32(thermals[j].Quality)

				// Обновляем позицию как среднее
				result.Center.Latitude = (result.Center.Latitude + thermals[j].Center.Latitude) / 2
				result.Center.Longitude = (result.Center.Longitude + thermals[j].Center.Longitude) / 2
				
				// Берем максимальную высоту
				if thermals[j].Altitude > result.Altitude {
					result.Altitude = thermals[j].Altitude
				}

				// Обновляем ветер (берем от более качественного термика)
				if thermals[j].Quality > result.Quality {
					result.WindSpeed = thermals[j].WindSpeed
					result.WindDirection = thermals[j].WindDirection
				}

				// Обновляем время на более свежее
				if thermals[j].Timestamp.After(result.Timestamp) {
					result.Timestamp = thermals[j].Timestamp
				}
			}
		}

		// Усредняем характеристики
		result.ClimbRate = sumClimb / float32(count)
		result.Quality = int32(sumQuality / float32(count))

		merged = append(merged, result)
	}

	return merged
}

// ToProto конвертирует Thermal в protobuf
func (t *Thermal) ToProto() *pb.Thermal {
	thermal := &pb.Thermal{
		Id:       0, // TODO: конвертировать ID в uint64
		Addr:     0, // TODO: конвертировать ReportedBy в uint32
		Altitude: t.Altitude,
		Quality:  uint32(t.Quality),
		Climb:    t.ClimbRate,
		WindSpeed:   float32(t.WindSpeed),
		WindHeading: float32(t.WindDirection),
		Timestamp:   t.Timestamp.Unix(),
	}
	
	if t.Position != nil {
		thermal.Position = &pb.GeoPoint{
			Latitude:  t.Position.Latitude,
			Longitude: t.Position.Longitude,
		}
	} else {
		thermal.Position = &pb.GeoPoint{
			Latitude:  t.Center.Latitude,
			Longitude: t.Center.Longitude,
		}
	}
	
	return thermal
}