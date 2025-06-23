package service

import (
	"testing"
	"time"

	"github.com/flybeeper/fanet-backend/internal/models"
	"github.com/flybeeper/fanet-backend/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidationService_ValidatePilot(t *testing.T) {
	logger := utils.NewLogger("info", "text")
	service := NewValidationService(logger, nil)

	// Тестовый пилот с реалистичными данными
	createPilot := func(deviceID string, lat, lon float64, timestamp time.Time, pilotType models.PilotType) *models.Pilot {
		return &models.Pilot{
			DeviceID: deviceID,
			Type:     pilotType,
			Position: &models.GeoPoint{
				Latitude:  lat,
				Longitude: lon,
			},
			LastUpdate: timestamp,
		}
	}

	t.Run("FirstPacketNotValidated", func(t *testing.T) {
		pilot := createPilot("ABC123", 46.0, 8.0, time.Now(), models.PilotTypeParaglider)
		
		valid, err := service.ValidatePilot(pilot)
		require.NoError(t, err)
		assert.False(t, valid, "First packet should not be validated")
		
		// Проверяем, что состояние создано
		state, exists := service.GetValidationState("ABC123")
		require.True(t, exists)
		assert.False(t, state.IsValidated)
		assert.Equal(t, 1, state.PacketCount)
	})

	t.Run("RealisticSpeedValidation", func(t *testing.T) {
		deviceID := "ABC124"
		baseTime := time.Now()
		
		// Первый пакет
		pilot1 := createPilot(deviceID, 46.0, 8.0, baseTime, models.PilotTypeParaglider)
		valid, err := service.ValidatePilot(pilot1)
		require.NoError(t, err)
		assert.False(t, valid)
		
		// Второй пакет через 5 минут, 2 км дальше (скорость ~24 км/ч - реалистично для параплана)
		pilot2 := createPilot(deviceID, 46.018, 8.0, baseTime.Add(5*time.Minute), models.PilotTypeParaglider)
		valid, err = service.ValidatePilot(pilot2)
		require.NoError(t, err)
		assert.True(t, valid, "Realistic speed should validate")
		
		// Проверяем состояние
		state, exists := service.GetValidationState(deviceID)
		require.True(t, exists)
		assert.True(t, state.IsValidated)
	})

	t.Run("UnrealisticSpeedRejection", func(t *testing.T) {
		deviceID := "ABC125"
		baseTime := time.Now()
		
		// Первый пакет
		pilot1 := createPilot(deviceID, 46.0, 8.0, baseTime, models.PilotTypeParaglider)
		valid, err := service.ValidatePilot(pilot1)
		require.NoError(t, err)
		assert.False(t, valid)
		
		// Второй пакет через 1 минуту, 50 км дальше (скорость 3000 км/ч - нереально)
		pilot2 := createPilot(deviceID, 46.5, 8.0, baseTime.Add(1*time.Minute), models.PilotTypeParaglider)
		valid, err = service.ValidatePilot(pilot2)
		require.NoError(t, err)
		assert.False(t, valid, "Unrealistic speed should not validate")
		
		// Проверяем, что состояние сброшено
		state, exists := service.GetValidationState(deviceID)
		require.True(t, exists)
		assert.False(t, state.IsValidated)
		assert.Equal(t, 1, state.PacketCount) // Счетчик сброшен
	})

	t.Run("DifferentAircraftTypes", func(t *testing.T) {
		baseTime := time.Now()
		
		// Тест для планера (высокая максимальная скорость)
		deviceID := "GLIDER1"
		pilot1 := createPilot(deviceID, 46.0, 8.0, baseTime, models.PilotTypeGlider)
		service.ValidatePilot(pilot1)
		
		// 150 км/ч для планера - должно пройти валидацию
		pilot2 := createPilot(deviceID, 46.1, 8.0, baseTime.Add(5*time.Minute), models.PilotTypeGlider)
		valid, err := service.ValidatePilot(pilot2)
		require.NoError(t, err)
		assert.True(t, valid, "High speed should be valid for glider")
	})

	t.Run("InvalidateDevice", func(t *testing.T) {
		deviceID := "ABC126"
		pilot := createPilot(deviceID, 46.0, 8.0, time.Now(), models.PilotTypeParaglider)
		
		// Создаем состояние
		service.ValidatePilot(pilot)
		
		// Инвалидируем
		err := service.InvalidateDevice(deviceID)
		require.NoError(t, err)
		
		// Проверяем, что состояние сброшено
		state, exists := service.GetValidationState(deviceID)
		require.True(t, exists)
		assert.False(t, state.IsValidated)
		assert.Equal(t, 0, state.PacketCount)
	})

	t.Run("InvalidateNonExistentDevice", func(t *testing.T) {
		err := service.InvalidateDevice("NONEXISTENT")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "device ID not found")
	})

	t.Run("CleanupOldStates", func(t *testing.T) {
		deviceID := "ABC127"
		oldTime := time.Now().Add(-25 * time.Hour) // Старше 24 часов
		
		// Создаем состояние с старым временем
		pilot := createPilot(deviceID, 46.0, 8.0, oldTime, models.PilotTypeParaglider)
		service.ValidatePilot(pilot)
		
		// Вручную устанавливаем старое время
		service.mu.Lock()
		service.states[deviceID].LastUpdate = oldTime
		service.mu.Unlock()
		
		// Выполняем очистку
		removed := service.CleanupOldStates(24 * time.Hour)
		assert.Equal(t, 1, removed)
		
		// Проверяем, что состояние удалено
		_, exists := service.GetValidationState(deviceID)
		assert.False(t, exists)
	})
}

func TestValidationService_CalculateDistance(t *testing.T) {
	logger := utils.NewLogger("info", "text")
	service := NewValidationService(logger, nil)

	// Известное расстояние между двумя точками (примерно 111 км)
	lat1, lon1 := 46.0, 8.0
	lat2, lon2 := 47.0, 8.0 // 1 градус широты ≈ 111 км

	distance := service.calculateDistance(lat1, lon1, lat2, lon2)
	
	// Проверяем, что расстояние близко к ожидаемому (±5%)
	expected := 111000.0 // метры
	assert.InDelta(t, expected, distance, expected*0.05, "Distance calculation should be accurate")
}

func TestValidationService_Metrics(t *testing.T) {
	logger := utils.NewLogger("info", "text")
	service := NewValidationService(logger, nil)

	// Проверяем начальные метрики
	metrics := service.GetMetrics()
	assert.Equal(t, int64(0), metrics.TotalPackets)
	assert.Equal(t, int64(0), metrics.ValidatedPackets)
	assert.Equal(t, int64(0), metrics.RejectedPackets)

	// Обрабатываем несколько пакетов
	pilot := &models.Pilot{
		DeviceID: "TEST123",
		Type:     models.PilotTypeParaglider,
		Position: &models.GeoPoint{Latitude: 46.0, Longitude: 8.0},
		LastUpdate: time.Now(),
	}

	service.ValidatePilot(pilot)
	
	// Проверяем обновленные метрики
	metrics = service.GetMetrics()
	assert.Equal(t, int64(1), metrics.TotalPackets)
	assert.Equal(t, int64(0), metrics.ValidatedPackets) // Первый пакет не валидируется
}