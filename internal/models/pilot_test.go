package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/flybeeper/fanet-backend/pkg/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPilotType_String(t *testing.T) {
	tests := []struct {
		pilotType PilotType
		expected  string
	}{
		{PilotTypeUnknown, "unknown"},
		{PilotTypeParaglider, "paraglider"},
		{PilotTypeHangglider, "hangglider"},
		{PilotTypeBalloon, "balloon"},
		{PilotTypeGlider, "glider"},
		{PilotTypePowered, "powered"},
		{PilotTypeHelicopter, "helicopter"},
		{PilotTypeUAV, "uav"},
		{PilotType(99), "unknown"}, // Invalid type
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.pilotType.String())
		})
	}
}

func TestPilotType_MarshalBinary(t *testing.T) {
	tests := []struct {
		name      string
		pilotType PilotType
		expected  []byte
	}{
		{"Unknown", PilotTypeUnknown, []byte{0}},
		{"Paraglider", PilotTypeParaglider, []byte{1}},
		{"Hangglider", PilotTypeHangglider, []byte{2}},
		{"Glider", PilotTypeGlider, []byte{4}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := tt.pilotType.MarshalBinary()
			require.NoError(t, err)
			assert.Equal(t, tt.expected, data)
		})
	}
}

func TestPilotType_UnmarshalBinary(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected PilotType
		hasError bool
	}{
		{"Valid - Paraglider", []byte{1}, PilotTypeParaglider, false},
		{"Valid - Glider", []byte{4}, PilotTypeGlider, false},
		{"Empty data", []byte{}, PilotTypeUnknown, true},
		{"Invalid data length", []byte{1, 2}, PilotTypeUnknown, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var pilotType PilotType
			err := pilotType.UnmarshalBinary(tt.data)
			
			if tt.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, pilotType)
			}
		})
	}
}

func TestPilot_Validate(t *testing.T) {
	validPosition := &GeoPoint{
		Latitude:  46.0,
		Longitude: 8.0,
	}

	tests := []struct {
		name    string
		pilot   *Pilot
		wantErr bool
		errMsg  string
	}{
		{
			name: "Valid pilot",
			pilot: &Pilot{
				DeviceID:   "ABC123",
				Type:       PilotTypeParaglider,
				Position:   validPosition,
				Name:       "Test Pilot",
				LastUpdate: time.Now(),
			},
			wantErr: false,
		},
		{
			name: "Empty device ID",
			pilot: &Pilot{
				DeviceID:   "",
				Type:       PilotTypeParaglider,
				Position:   validPosition,
				LastUpdate: time.Now(),
			},
			wantErr: true,
			errMsg:  "device_id cannot be empty",
		},
		{
			name: "Nil position",
			pilot: &Pilot{
				DeviceID:   "ABC123",
				Type:       PilotTypeParaglider,
				Position:   nil,
				LastUpdate: time.Now(),
			},
			wantErr: true,
			errMsg:  "position cannot be nil",
		},
		{
			name: "Invalid position",
			pilot: &Pilot{
				DeviceID: "ABC123",
				Type:     PilotTypeParaglider,
				Position: &GeoPoint{
					Latitude:  91.0, // Invalid latitude
					Longitude: 8.0,
				},
				LastUpdate: time.Now(),
			},
			wantErr: true,
			errMsg:  "invalid position",
		},
		{
			name: "Zero timestamp",
			pilot: &Pilot{
				DeviceID:   "ABC123",
				Type:       PilotTypeParaglider,
				Position:   validPosition,
				LastUpdate: time.Time{},
			},
			wantErr: true,
			errMsg:  "last_update cannot be zero",
		},
		{
			name: "Negative altitude",
			pilot: &Pilot{
				DeviceID:   "ABC123",
				Type:       PilotTypeParaglider,
				Position:   validPosition,
				Altitude:   -100,
				LastUpdate: time.Now(),
			},
			wantErr: true,
			errMsg:  "altitude cannot be negative",
		},
		{
			name: "Invalid speed",
			pilot: &Pilot{
				DeviceID:   "ABC123",
				Type:       PilotTypeParaglider,
				Position:   validPosition,
				Speed:      500, // 500 km/h unrealistic for paraglider
				LastUpdate: time.Now(),
			},
			wantErr: true,
			errMsg:  "speed exceeds maximum",
		},
		{
			name: "Invalid heading",
			pilot: &Pilot{
				DeviceID:   "ABC123",
				Type:       PilotTypeParaglider,
				Position:   validPosition,
				Heading:    361, // Invalid heading
				LastUpdate: time.Now(),
			},
			wantErr: true,
			errMsg:  "heading must be between 0 and 360",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.pilot.Validate()
			
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Примечание: методы IsValidSpeed и GetMaxSpeed не реализованы в текущей структуре Pilot
// Тестируем валидацию через метод Validate()
func TestPilot_SpeedValidation(t *testing.T) {
	tests := []struct {
		name      string
		speed     float32
		expectErr bool
	}{
		{"Normal speed", 50.0, false},
		{"Max acceptable speed", 400.0, false},
		{"Too fast", 450.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pilot := &Pilot{
				DeviceID: "TEST01",
				Speed:    tt.speed,
				Position: &GeoPoint{Latitude: 46.0, Longitude: 8.0},
			}
			err := pilot.Validate()
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Примечание: метод GetMaxSpeed не реализован в текущей структуре Pilot
// Тест закомментирован до реализации метода
// func TestPilot_GetMaxSpeed(t *testing.T) {
// 	tests := []struct {
// 		pilotType PilotType
// 		expected  uint16
// 	}{
// 		{PilotTypeParaglider, 100},
// 		{PilotTypeHangglider, 120},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.pilotType.String(), func(t *testing.T) {
// 			pilot := &Pilot{Type: tt.pilotType}
// 			assert.Equal(t, tt.expected, pilot.GetMaxSpeed())
// 		})
// 	}
// }

func TestPilot_ToProto(t *testing.T) {
	now := time.Now()
	pilot := &Pilot{
		DeviceID: "ABC123",
		Type:     PilotTypeParaglider,
		Position: &GeoPoint{
			Latitude:  46.0,
			Longitude: 8.0,
		},
		Name:         "Test Pilot",
		Altitude:     1000,
		Speed:        50,
		Heading:      180,
		ClimbRate:    200, // будет конвертировано в 20.0 м/с в ToProto
		TrackOnline:  true,
		LastUpdate:   now,
		Battery:      75,
	}

	protoPilot := pilot.ToProto()
	require.NotNil(t, protoPilot)

	// Проверяем основные поля
	assert.Equal(t, "Test Pilot", protoPilot.Name)
	assert.Equal(t, pb.PilotType_PILOT_TYPE_PARAGLIDER, protoPilot.Type)
	assert.Equal(t, int32(1000), protoPilot.Altitude)
	assert.Equal(t, float32(50), protoPilot.Speed)
	assert.Equal(t, float32(180), protoPilot.Course)
	assert.Equal(t, float32(20.0), protoPilot.Climb) // 200/10 = 20.0 м/с
	assert.True(t, protoPilot.TrackOnline)
	assert.Equal(t, now.Unix(), protoPilot.LastUpdate)
	assert.Equal(t, uint32(75), protoPilot.Battery)

	// Проверяем позицию
	require.NotNil(t, protoPilot.Position)
	assert.InDelta(t, 46.0, protoPilot.Position.Latitude, 0.000001)
	assert.InDelta(t, 8.0, protoPilot.Position.Longitude, 0.000001)
}

// Примечание: метод FromProto не реализован в текущей структуре Pilot
// Тест закомментирован до реализации метода
// func TestPilot_FromProto(t *testing.T) {
// 	protoPilot := &pb.Pilot{
// 		Addr: 0x123456, // Пример FANET адреса
// 		Type: pb.PilotType_PILOT_TYPE_HANGGLIDER,
// 		Position: &pb.GeoPoint{
// 			Latitude:  47.0,
// 			Longitude: 9.0,
// 		},
// 		Name:        "Proto Pilot",
// 		Altitude:    1500,
// 		Speed:       60,
// 		Course:      270,
// 		Climb:       -1.0, // -1.0 m/s (descent)
// 		TrackOnline: false,
// 		LastUpdate:  time.Now().Unix(),
// 	}

// 	pilot := &Pilot{}
// 	pilot.FromProto(protoPilot)

// 	assert.Equal(t, PilotTypeHangglider, pilot.Type)
// 	assert.Equal(t, "Proto Pilot", pilot.Name)
// 	// и т.д.
// }

func TestPilot_JSONSerialization(t *testing.T) {
	original := &Pilot{
		DeviceID: "JSON123",
		Type:     PilotTypeGlider,
		Position: &GeoPoint{
			Latitude:  45.5,
			Longitude: 7.5,
		},
		Name:        "JSON Test Pilot",
		Altitude:    2000,
		Speed:       80,
		Heading:     90,
		ClimbRate:   150,
		TrackOnline: true,
		Battery:     85,
		LastUpdate:  time.Now().Truncate(time.Second), // Убираем наносекунды для точного сравнения
	}

	// Сериализация в JSON
	jsonData, err := json.Marshal(original)
	require.NoError(t, err)
	assert.NotEmpty(t, jsonData)

	// Десериализация из JSON
	var restored Pilot
	err = json.Unmarshal(jsonData, &restored)
	require.NoError(t, err)

	// Сравниваем поля
	assert.Equal(t, original.DeviceID, restored.DeviceID)
	assert.Equal(t, original.Type, restored.Type)
	assert.Equal(t, original.Name, restored.Name)
	assert.Equal(t, original.Altitude, restored.Altitude)
	assert.Equal(t, original.Speed, restored.Speed)
	assert.Equal(t, original.Heading, restored.Heading)
	assert.Equal(t, original.ClimbRate, restored.ClimbRate)
	assert.Equal(t, original.TrackOnline, restored.TrackOnline)
	assert.Equal(t, original.Battery, restored.Battery)
	assert.Equal(t, original.LastUpdate.Unix(), restored.LastUpdate.Unix())

	// Проверяем позицию
	require.NotNil(t, restored.Position)
	assert.InDelta(t, original.Position.Latitude, restored.Position.Latitude, 0.000001)
	assert.InDelta(t, original.Position.Longitude, restored.Position.Longitude, 0.000001)
}

// Примечание: метод CalculateDistance не реализован, используем DistanceTo из GeoPoint
func TestPilot_DistanceCalculation(t *testing.T) {
	pilot1 := &Pilot{
		Position: &GeoPoint{Latitude: 46.0, Longitude: 8.0},
	}
	pilot2 := &Pilot{
		Position: &GeoPoint{Latitude: 47.0, Longitude: 8.0}, // ~111km north
	}

	distance := pilot1.Position.DistanceTo(*pilot2.Position)
	
	// Проверяем, что расстояние примерно 111км (±5%)
	expectedDistance := 111.0 // километры
	assert.InDelta(t, expectedDistance, distance, expectedDistance*0.05)
}

// Примечание: метод CalculateSpeed не реализован
// func TestPilot_CalculateSpeed(t *testing.T) {
// 	now := time.Now()
	
// 	pilot1 := &Pilot{
// 		Position:   &GeoPoint{Latitude: 46.0, Longitude: 8.0},
// 		LastUpdate: now,
// 	}
// 	pilot2 := &Pilot{
// 		Position:   &GeoPoint{Latitude: 46.01, Longitude: 8.0}, // ~1.1km north
// 		LastUpdate: now.Add(2 * time.Minute), // 2 minutes later
// 	}

// 	speed := pilot1.CalculateSpeed(pilot2)
	
// 	// Ожидаемая скорость: ~1.1km за 2 минуты = ~33 км/ч
// 	expectedSpeed := 33.0 // км/ч
// 	assert.InDelta(t, expectedSpeed, speed, 5.0) // ±5 км/ч tolerance
// }

// Примечание: метод Clone не реализован
// func TestPilot_Clone(t *testing.T) {
// 	original := &Pilot{
// 		DeviceID: "CLONE123",
// 		Type:     PilotTypeParaglider,
// 		Position: &GeoPoint{
// 			Latitude:  46.0,
// 			Longitude: 8.0,
// 		},
// 		Name:        "Original Pilot",
// 		Altitude:    1000,
// 		Speed:       50,
// 		LastUpdate:  time.Now(),
// 	}

// 	cloned := original.Clone()
// 	require.NotNil(t, cloned)

// 	// Проверяем, что все поля скопированы
// 	assert.Equal(t, original.DeviceID, cloned.DeviceID)
// 	assert.Equal(t, original.Type, cloned.Type)
// }

func TestPilot_IsStale(t *testing.T) {
	now := time.Now()
	
	tests := []struct {
		name       string
		lastUpdate time.Time
		maxAge     time.Duration
		expected   bool
	}{
		{
			name:       "Fresh pilot",
			lastUpdate: now.Add(-1 * time.Minute),
			maxAge:     5 * time.Minute,
			expected:   false,
		},
		{
			name:       "Expired pilot",
			lastUpdate: now.Add(-10 * time.Minute),
			maxAge:     5 * time.Minute,
			expected:   true,
		},
		{
			name:       "Exactly at limit",
			lastUpdate: now.Add(-5 * time.Minute),
			maxAge:     5 * time.Minute,
			expected:   false, // Should not be expired exactly at the limit
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pilot := &Pilot{
				DeviceID:   "TEST123",
				LastUpdate: tt.lastUpdate,
			}
			assert.Equal(t, tt.expected, pilot.IsStale(tt.maxAge))
		})
	}
}

// Benchmark тесты
func BenchmarkPilot_ToProto(b *testing.B) {
	pilot := &Pilot{
		DeviceID: "BENCH123",
		Type:     PilotTypeParaglider,
		Position: &GeoPoint{Latitude: 46.0, Longitude: 8.0},
		Name:     "Benchmark Pilot",
		Altitude: 1000,
		Speed:    50,
		Heading:  180,
		LastUpdate: time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pilot.ToProto()
	}
}

func BenchmarkPilot_Validate(b *testing.B) {
	pilot := &Pilot{
		DeviceID: "BENCH123",
		Type:     PilotTypeParaglider,
		Position: &GeoPoint{Latitude: 46.0, Longitude: 8.0},
		Name:     "Benchmark Pilot",
		Altitude: 1000,
		Speed:    50,
		Heading:  180,
		LastUpdate: time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pilot.Validate()
	}
}

// Примечание: метод Clone не реализован
// func BenchmarkPilot_Clone(b *testing.B) {
// 	pilot := &Pilot{
// 		DeviceID: "BENCH123",
// 		Type:     PilotTypeParaglider,
// 		Position: &GeoPoint{Latitude: 46.0, Longitude: 8.0},
// 		Name:     "Benchmark Pilot",
// 		Altitude: 1000,
// 		Speed:    50,
// 		Heading:  180,
// 		LastUpdate: time.Now(),
// 	}

// 	b.ResetTimer()
// 	for i := 0; i < b.N; i++ {
// 		_ = pilot.Clone()
// 	}
// }