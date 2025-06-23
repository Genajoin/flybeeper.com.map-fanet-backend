package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestThermal_Validate(t *testing.T) {
	validPosition := &GeoPoint{
		Latitude:  46.0,
		Longitude: 8.0,
	}

	tests := []struct {
		name    string
		thermal *Thermal
		wantErr bool
		errMsg  string
	}{
		{
			name: "Valid thermal",
			thermal: &Thermal{
				ID:         "THERM001",
				ReportedBy: "PILOT001",
				Position:   validPosition,
				Quality:    4,
				ClimbRate:  3.0, // 3.0 m/s
				Timestamp:  time.Now(),
			},
			wantErr: false,
		},
		{
			name: "Empty ID",
			thermal: &Thermal{
				ID:         "",
				ReportedBy: "PILOT001",
				Position:   validPosition,
				Quality:    3,
				Timestamp:  time.Now(),
			},
			wantErr: true,
			errMsg:  "id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.thermal.Validate()
			
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestThermal_JSONSerialization(t *testing.T) {
	original := &Thermal{
		ID:         "JSON_THERM",
		ReportedBy: "PILOT123",
		Position: &GeoPoint{
			Latitude:  46.123,
			Longitude: 8.456,
		},
		Quality:   3,
		ClimbRate: 2.5,
		Timestamp: time.Now().Truncate(time.Second),
	}

	// Сериализация в JSON
	jsonData, err := json.Marshal(original)
	require.NoError(t, err)
	assert.NotEmpty(t, jsonData)

	// Десериализация из JSON
	var restored Thermal
	err = json.Unmarshal(jsonData, &restored)
	require.NoError(t, err)

	// Сравниваем поля
	assert.Equal(t, original.ID, restored.ID)
	assert.Equal(t, original.ReportedBy, restored.ReportedBy)
	assert.Equal(t, original.Quality, restored.Quality)
	assert.Equal(t, original.ClimbRate, restored.ClimbRate)
	assert.Equal(t, original.Timestamp.Unix(), restored.Timestamp.Unix())

	// Проверяем позицию
	require.NotNil(t, restored.Position)
	assert.InDelta(t, original.Position.Latitude, restored.Position.Latitude, 0.000001)
	assert.InDelta(t, original.Position.Longitude, restored.Position.Longitude, 0.000001)
}

// Примечание: ToProto тест закомментирован до уточнения protobuf структуры
// func TestThermal_ToProto(t *testing.T) {
// 	thermal := &Thermal{
// 		ID:         "PROTO_THERM",
// 		ReportedBy: "PILOT456",
// 		Position: &GeoPoint{
// 			Latitude:  47.0,
// 			Longitude: 9.0,
// 		},
// 		Quality:   5,
// 		ClimbRate: 4.0,
// 		Timestamp: time.Now(),
// 	}

// 	protoThermal := thermal.ToProto()
// 	require.NotNil(t, protoThermal)
// }

// Benchmark тесты
func BenchmarkThermal_Validate(b *testing.B) {
	thermal := &Thermal{
		ID:         "BENCH_THERM",
		ReportedBy: "PILOT001",
		Position:   &GeoPoint{Latitude: 46.0, Longitude: 8.0},
		Quality:    3,
		ClimbRate:  2.0,
		Timestamp:  time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = thermal.Validate()
	}
}

func BenchmarkThermal_ToProto(b *testing.B) {
	thermal := &Thermal{
		ID:         "BENCH_THERM",
		ReportedBy: "PILOT001",
		Position:   &GeoPoint{Latitude: 46.0, Longitude: 8.0},
		Quality:    3,
		ClimbRate:  2.0,
		Timestamp:  time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = thermal.ToProto()
	}
}