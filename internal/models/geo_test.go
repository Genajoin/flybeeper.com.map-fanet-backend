package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeoPoint_Validate(t *testing.T) {
	tests := []struct {
		name    string
		point   GeoPoint
		wantErr bool
		errMsg  string
	}{
		{
			name:    "Valid coordinates - Alps",
			point:   GeoPoint{Latitude: 46.0, Longitude: 8.0},
			wantErr: false,
		},
		{
			name:    "Valid coordinates - Equator",
			point:   GeoPoint{Latitude: 0.0, Longitude: 0.0},
			wantErr: false,
		},
		{
			name:    "Valid coordinates - North Pole",
			point:   GeoPoint{Latitude: 90.0, Longitude: 0.0},
			wantErr: false,
		},
		{
			name:    "Valid coordinates - South Pole",
			point:   GeoPoint{Latitude: -90.0, Longitude: 0.0},
			wantErr: false,
		},
		{
			name:    "Valid coordinates - Date line",
			point:   GeoPoint{Latitude: 0.0, Longitude: 180.0},
			wantErr: false,
		},
		{
			name:    "Valid coordinates - Date line negative",
			point:   GeoPoint{Latitude: 0.0, Longitude: -180.0},
			wantErr: false,
		},
		{
			name:    "Invalid latitude - too high",
			point:   GeoPoint{Latitude: 91.0, Longitude: 0.0},
			wantErr: true,
			errMsg:  "invalid latitude",
		},
		{
			name:    "Invalid latitude - too low",
			point:   GeoPoint{Latitude: -91.0, Longitude: 0.0},
			wantErr: true,
			errMsg:  "invalid latitude",
		},
		{
			name:    "Invalid longitude - too high",
			point:   GeoPoint{Latitude: 0.0, Longitude: 181.0},
			wantErr: true,
			errMsg:  "invalid longitude",
		},
		{
			name:    "Invalid longitude - too low",
			point:   GeoPoint{Latitude: 0.0, Longitude: -181.0},
			wantErr: true,
			errMsg:  "invalid longitude",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.point.Validate()
			
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGeoPoint_DistanceTo(t *testing.T) {
	tests := []struct {
		name     string
		point1   GeoPoint
		point2   GeoPoint
		expected float64
		tolerance float64
	}{
		{
			name:      "Same point",
			point1:    GeoPoint{Latitude: 46.0, Longitude: 8.0},
			point2:    GeoPoint{Latitude: 46.0, Longitude: 8.0},
			expected:  0.0,
			tolerance: 0.1,
		},
		{
			name:      "1 degree latitude difference",
			point1:    GeoPoint{Latitude: 46.0, Longitude: 8.0},
			point2:    GeoPoint{Latitude: 47.0, Longitude: 8.0},
			expected:  111.0, // ~111km
			tolerance: 5.0,
		},
		{
			name:      "1 degree longitude difference at equator",
			point1:    GeoPoint{Latitude: 0.0, Longitude: 0.0},
			point2:    GeoPoint{Latitude: 0.0, Longitude: 1.0},
			expected:  111.0, // ~111km at equator
			tolerance: 5.0,
		},
		{
			name:      "1 degree longitude difference at 60° latitude",
			point1:    GeoPoint{Latitude: 60.0, Longitude: 0.0},
			point2:    GeoPoint{Latitude: 60.0, Longitude: 1.0},
			expected:  55.5, // ~55.5km (cos(60°) ≈ 0.5)
			tolerance: 5.0,
		},
		{
			name:      "Zurich to Bern (approximate)",
			point1:    GeoPoint{Latitude: 47.3769, Longitude: 8.5417}, // Zurich
			point2:    GeoPoint{Latitude: 46.9481, Longitude: 7.4474}, // Bern
			expected:  95.0, // ~95km
			tolerance: 10.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			distance := tt.point1.DistanceTo(tt.point2)
			assert.InDelta(t, tt.expected, distance, tt.tolerance)
			
			// Проверяем симметричность
			reverseDistance := tt.point2.DistanceTo(tt.point1)
			assert.InDelta(t, distance, reverseDistance, 0.1)
		})
	}
}

func TestGeoPoint_Geohash(t *testing.T) {
	point := GeoPoint{
		Latitude:  46.123456,
		Longitude: 8.987654,
	}

	tests := []struct {
		name      string
		precision int
		minLength int
		maxLength int
	}{
		{"Precision 5", 5, 5, 5},
		{"Precision 7", 7, 7, 7},
		{"Precision 10", 10, 10, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			geohash := point.Geohash(tt.precision)
			assert.GreaterOrEqual(t, len(geohash), tt.minLength)
			assert.LessOrEqual(t, len(geohash), tt.maxLength)
			assert.NotEmpty(t, geohash)
		})
	}
}

func TestGeoPoint_IsInBounds(t *testing.T) {
	center := GeoPoint{Latitude: 46.0, Longitude: 8.0}
	sw := GeoPoint{Latitude: 45.5, Longitude: 7.5}
	ne := GeoPoint{Latitude: 46.5, Longitude: 8.5}
	
	tests := []struct {
		name     string
		point    GeoPoint
		expected bool
	}{
		{
			name:     "Point inside bounds",
			point:    center,
			expected: true,
		},
		{
			name:     "Point on southwest corner",
			point:    sw,
			expected: true,
		},
		{
			name:     "Point on northeast corner",
			point:    ne,
			expected: true,
		},
		{
			name:     "Point outside bounds - too far north",
			point:    GeoPoint{Latitude: 47.0, Longitude: 8.0},
			expected: false,
		},
		{
			name:     "Point outside bounds - too far west",
			point:    GeoPoint{Latitude: 46.0, Longitude: 7.0},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.point.IsInBounds(sw, ne)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGeoPoint_JSONSerialization(t *testing.T) {
	original := GeoPoint{
		Latitude:  46.123456789,
		Longitude: 8.987654321,
		Altitude:  1500,
	}

	// Сериализация в JSON
	jsonData, err := json.Marshal(original)
	require.NoError(t, err)
	assert.NotEmpty(t, jsonData)

	// Десериализация из JSON
	var restored GeoPoint
	err = json.Unmarshal(jsonData, &restored)
	require.NoError(t, err)

	assert.InDelta(t, original.Latitude, restored.Latitude, 0.000000001)
	assert.InDelta(t, original.Longitude, restored.Longitude, 0.000000001)
	assert.Equal(t, original.Altitude, restored.Altitude)
}

// Тестирование Bounds
func TestBounds_Validate(t *testing.T) {
	tests := []struct {
		name    string
		bounds  Bounds
		wantErr bool
		errMsg  string
	}{
		{
			name: "Valid bounds",
			bounds: Bounds{
				Southwest: GeoPoint{Latitude: 45.0, Longitude: 7.0},
				Northeast: GeoPoint{Latitude: 47.0, Longitude: 9.0},
			},
			wantErr: false,
		},
		{
			name: "Invalid bounds - latitude reversed",
			bounds: Bounds{
				Southwest: GeoPoint{Latitude: 47.0, Longitude: 7.0},
				Northeast: GeoPoint{Latitude: 45.0, Longitude: 9.0},
			},
			wantErr: true,
			errMsg:  "southwest latitude must be less than northeast latitude",
		},
		{
			name: "Invalid bounds - longitude reversed",
			bounds: Bounds{
				Southwest: GeoPoint{Latitude: 45.0, Longitude: 9.0},
				Northeast: GeoPoint{Latitude: 47.0, Longitude: 7.0},
			},
			wantErr: true,
			errMsg:  "southwest longitude must be less than northeast longitude",
		},
		{
			name: "Invalid southwest coordinates",
			bounds: Bounds{
				Southwest: GeoPoint{Latitude: 91.0, Longitude: 7.0},
				Northeast: GeoPoint{Latitude: 47.0, Longitude: 9.0},
			},
			wantErr: true,
			errMsg:  "southwest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.bounds.Validate()
			
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBounds_Contains(t *testing.T) {
	bounds := Bounds{
		Southwest: GeoPoint{Latitude: 45.0, Longitude: 7.0},
		Northeast: GeoPoint{Latitude: 47.0, Longitude: 9.0},
	}
	
	tests := []struct {
		name     string
		point    GeoPoint
		expected bool
	}{
		{
			name:     "Point inside bounds",
			point:    GeoPoint{Latitude: 46.0, Longitude: 8.0},
			expected: true,
		},
		{
			name:     "Point on corner",
			point:    GeoPoint{Latitude: 45.0, Longitude: 7.0},
			expected: true,
		},
		{
			name:     "Point outside bounds",
			point:    GeoPoint{Latitude: 48.0, Longitude: 8.0},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := bounds.Contains(tt.point)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBounds_Center(t *testing.T) {
	bounds := Bounds{
		Southwest: GeoPoint{Latitude: 45.0, Longitude: 7.0},
		Northeast: GeoPoint{Latitude: 47.0, Longitude: 9.0},
	}

	center := bounds.Center()
	expected := GeoPoint{Latitude: 46.0, Longitude: 8.0}
	
	assert.InDelta(t, expected.Latitude, center.Latitude, 0.000001)
	assert.InDelta(t, expected.Longitude, center.Longitude, 0.000001)
}

func TestBounds_Expand(t *testing.T) {
	bounds := Bounds{
		Southwest: GeoPoint{Latitude: 46.0, Longitude: 8.0},
		Northeast: GeoPoint{Latitude: 46.1, Longitude: 8.1},
	}

	expanded := bounds.Expand(10.0) // 10km
	
	// Expanded bounds should be larger
	assert.Less(t, expanded.Southwest.Latitude, bounds.Southwest.Latitude)
	assert.Less(t, expanded.Southwest.Longitude, bounds.Southwest.Longitude)
	assert.Greater(t, expanded.Northeast.Latitude, bounds.Northeast.Latitude)
	assert.Greater(t, expanded.Northeast.Longitude, bounds.Northeast.Longitude)
}

func TestBounds_DiagonalKm(t *testing.T) {
	bounds := Bounds{
		Southwest: GeoPoint{Latitude: 46.0, Longitude: 8.0},
		Northeast: GeoPoint{Latitude: 47.0, Longitude: 9.0},
	}

	diagonal := bounds.DiagonalKm()
	
	// Диагональ должна быть больше, чем расстояние только по широте или долготе
	latDistance := bounds.Southwest.DistanceTo(GeoPoint{Latitude: 47.0, Longitude: 8.0})
	lonDistance := bounds.Southwest.DistanceTo(GeoPoint{Latitude: 46.0, Longitude: 9.0})
	
	assert.Greater(t, diagonal, latDistance)
	assert.Greater(t, diagonal, lonDistance)
	// Фактическая диагональ по формуле Haversine ~135км, не 157км как в теории
	assert.InDelta(t, 135.0, diagonal, 5.0) // Реальное значение Haversine
}

func TestBounds_GeohashCover(t *testing.T) {
	bounds := Bounds{
		Southwest: GeoPoint{Latitude: 46.0, Longitude: 8.0},
		Northeast: GeoPoint{Latitude: 46.1, Longitude: 8.1},
	}

	// Тестируем с небольшой точностью
	hashes := bounds.GeohashCover(5)
	
	assert.NotEmpty(t, hashes)
	assert.Greater(t, len(hashes), 0)
	
	// Все хеши должны иметь правильную длину
	for _, hash := range hashes {
		assert.Equal(t, 5, len(hash))
	}
}

// Benchmark тесты
func BenchmarkGeoPoint_DistanceTo(b *testing.B) {
	point1 := GeoPoint{Latitude: 46.0, Longitude: 8.0}
	point2 := GeoPoint{Latitude: 47.0, Longitude: 9.0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = point1.DistanceTo(point2)
	}
}

func BenchmarkGeoPoint_Geohash(b *testing.B) {
	point := GeoPoint{Latitude: 46.0, Longitude: 8.0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = point.Geohash(7)
	}
}

func BenchmarkGeoPoint_Validate(b *testing.B) {
	point := GeoPoint{Latitude: 46.0, Longitude: 8.0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = point.Validate()
	}
}