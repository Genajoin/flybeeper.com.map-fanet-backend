package models

import (
	"fmt"
	"math"
	"time"

	"github.com/mmcloughlin/geohash"
)

// GeoPoint представляет географическую точку
type GeoPoint struct {
	Latitude  float64 `json:"lat"`
	Longitude float64 `json:"lon"`
	Altitude  int32   `json:"alt"`
}

// Validate проверяет корректность координат
func (p GeoPoint) Validate() error {
	if p.Latitude < -90 || p.Latitude > 90 {
		return fmt.Errorf("invalid latitude: %f", p.Latitude)
	}
	if p.Longitude < -180 || p.Longitude > 180 {
		return fmt.Errorf("invalid longitude: %f", p.Longitude)
	}
	return nil
}

// DistanceTo вычисляет расстояние до другой точки в километрах (формула Haversine)
func (p GeoPoint) DistanceTo(other GeoPoint) float64 {
	const earthRadius = 6371 // км

	lat1Rad := p.Latitude * math.Pi / 180
	lat2Rad := other.Latitude * math.Pi / 180
	deltaLat := (other.Latitude - p.Latitude) * math.Pi / 180
	deltaLon := (other.Longitude - p.Longitude) * math.Pi / 180

	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(deltaLon/2)*math.Sin(deltaLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadius * c
}

// Geohash возвращает geohash для точки с заданной точностью
func (p GeoPoint) Geohash(precision int) string {
	return geohash.EncodeWithPrecision(p.Latitude, p.Longitude, uint(precision))
}

// IsInBounds проверяет, находится ли точка в границах
func (p GeoPoint) IsInBounds(sw, ne GeoPoint) bool {
	return p.Latitude >= sw.Latitude && p.Latitude <= ne.Latitude &&
		p.Longitude >= sw.Longitude && p.Longitude <= ne.Longitude
}

// Bounds представляет географические границы
type Bounds struct {
	Southwest GeoPoint `json:"sw"`
	Northeast GeoPoint `json:"ne"`
}

// Validate проверяет корректность границ
func (b Bounds) Validate() error {
	if err := b.Southwest.Validate(); err != nil {
		return fmt.Errorf("southwest: %w", err)
	}
	if err := b.Northeast.Validate(); err != nil {
		return fmt.Errorf("northeast: %w", err)
	}
	if b.Southwest.Latitude > b.Northeast.Latitude {
		return fmt.Errorf("southwest latitude must be less than northeast latitude")
	}
	if b.Southwest.Longitude > b.Northeast.Longitude {
		return fmt.Errorf("southwest longitude must be less than northeast longitude")
	}
	return nil
}

// Contains проверяет, содержится ли точка в границах
func (b Bounds) Contains(point GeoPoint) bool {
	return point.IsInBounds(b.Southwest, b.Northeast)
}

// Center возвращает центральную точку границ
func (b Bounds) Center() GeoPoint {
	return GeoPoint{
		Latitude:  (b.Southwest.Latitude + b.Northeast.Latitude) / 2,
		Longitude: (b.Southwest.Longitude + b.Northeast.Longitude) / 2,
	}
}

// Expand расширяет границы на заданное расстояние в километрах
func (b Bounds) Expand(km float64) Bounds {
	// Приблизительные градусы на километр
	latDegPerKm := 1.0 / 111.0
	lonDegPerKm := 1.0 / (111.0 * math.Cos(b.Center().Latitude*math.Pi/180))

	latExpansion := km * latDegPerKm
	lonExpansion := km * lonDegPerKm

	return Bounds{
		Southwest: GeoPoint{
			Latitude:  b.Southwest.Latitude - latExpansion,
			Longitude: b.Southwest.Longitude - lonExpansion,
		},
		Northeast: GeoPoint{
			Latitude:  b.Northeast.Latitude + latExpansion,
			Longitude: b.Northeast.Longitude + lonExpansion,
		},
	}
}

// GeohashCover возвращает список geohash префиксов для покрытия границ
func (b Bounds) GeohashCover(precision int) []string {
	// Получаем все geohash в прямоугольнике
	hashes := make(map[string]bool)
	
	// Шаг в градусах для данной точности geohash
	step := geohashStepSize(precision)
	
	for lat := b.Southwest.Latitude; lat <= b.Northeast.Latitude; lat += step {
		for lon := b.Southwest.Longitude; lon <= b.Northeast.Longitude; lon += step {
			hash := geohash.EncodeWithPrecision(lat, lon, uint(precision))
			hashes[hash] = true
		}
	}
	
	// Конвертируем в слайс
	result := make([]string, 0, len(hashes))
	for hash := range hashes {
		result = append(result, hash)
	}
	
	return result
}

// DiagonalKm возвращает диагональ границ в километрах
func (b Bounds) DiagonalKm() float64 {
	return b.Southwest.DistanceTo(b.Northeast)
}

// MinLat возвращает минимальную широту
func (b Bounds) MinLat() float64 {
	return b.Southwest.Latitude
}

// MinLon возвращает минимальную долготу
func (b Bounds) MinLon() float64 {
	return b.Southwest.Longitude
}

// MaxLat возвращает максимальную широту
func (b Bounds) MaxLat() float64 {
	return b.Northeast.Latitude
}

// MaxLon возвращает максимальную долготу
func (b Bounds) MaxLon() float64 {
	return b.Northeast.Longitude
}

// TrackGeoPoint представляет географическую точку с временной меткой для треков
type TrackGeoPoint struct {
	GeoPoint
	Timestamp time.Time `json:"timestamp"`
}

// geohashStepSize возвращает приблизительный размер шага в градусах для заданной точности
func geohashStepSize(precision int) float64 {
	// Приблизительные размеры ячеек geohash
	sizes := map[int]float64{
		1: 5000.0,   // ±2500 км
		2: 1250.0,   // ±625 км
		3: 156.0,    // ±78 км
		4: 39.0,     // ±19.5 км
		5: 4.9,      // ±2.45 км
		6: 1.2,      // ±0.6 км
		7: 0.15,     // ±0.075 км
		8: 0.038,    // ±0.019 км
	}
	
	if size, ok := sizes[precision]; ok {
		// Конвертируем км в градусы (приблизительно)
		return size / 111.0
	}
	
	// По умолчанию для precision 5
	return 4.9 / 111.0
}