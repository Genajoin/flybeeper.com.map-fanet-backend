package service

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/flybeeper/fanet-backend/internal/metrics"
	"github.com/flybeeper/fanet-backend/internal/models"
	"github.com/flybeeper/fanet-backend/pkg/utils"
)

// ValidationService сервис для валидации входящих данных пилотов
type ValidationService struct {
	states  map[string]*models.PilotValidationState
	mu      sync.RWMutex
	config  *models.ValidationConfig
	logger  *utils.Logger
	metrics ValidationMetrics
}

// ValidationMetrics метрики валидации
type ValidationMetrics struct {
	TotalPackets     int64
	ValidatedPackets int64
	RejectedPackets  int64
	InvalidatedIDs   int64
}

// NewValidationService создает новый сервис валидации
func NewValidationService(logger *utils.Logger, config *models.ValidationConfig) *ValidationService {
	if config == nil {
		config = models.DefaultValidationConfig()
	}

	return &ValidationService{
		states: make(map[string]*models.PilotValidationState),
		config: config,
		logger: logger,
	}
}

// ValidatePilot проверяет и валидирует данные пилота
// Возвращает (isValid, shouldStore, error) где:
// - isValid: пакет прошел валидацию
// - shouldStore: счет достаточен для сохранения в Redis
func (s *ValidationService) ValidatePilot(pilot *models.Pilot) (bool, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.metrics.TotalPackets++
	metrics.ValidationTotalPackets.Inc()

	state, exists := s.states[pilot.DeviceID]
	
	// Первый пакет от этого ID
	if !exists {
		state = &models.PilotValidationState{
			DeviceID:                  pilot.DeviceID,
			FirstSeen:                 pilot.LastUpdate,
			LastPosition:              pilot.Position,
			LastUpdate:                pilot.LastUpdate,
			IsValidated:               false,
			PacketCount:               1,
			AircraftType:              pilot.Type,
			ValidationScore:           s.config.InitialScore,
			ConsecutiveInvalidPackets: 0,
		}
		s.states[pilot.DeviceID] = state
		
		// Обновляем метрику активных состояний
		metrics.ValidationActiveStates.Set(float64(len(s.states)))
		
		s.logger.WithField("device_id", pilot.DeviceID).
			WithField("initial_score", s.config.InitialScore).
			Debug("First packet from device, waiting for validation")
		
		// Первый пакет не сохраняем в Redis пока не накопим достаточный счет
		return false, false, nil
	}

	// Обновляем счетчик пакетов
	state.PacketCount++

	// Проверка наличия координат
	if pilot.Position == nil || state.LastPosition == nil {
		// Если нет координат, обновляем состояние но считаем пакет невалидным
		state.LastUpdate = pilot.LastUpdate
		if pilot.Position != nil {
			state.LastPosition = pilot.Position
		}
		
		// Уменьшаем счет за отсутствие координат
		s.updateValidationScore(state, false)
		
		// Проверяем гистерезис после изменения счета
		s.checkHysteresis(state, pilot.DeviceID)
		
		shouldStore := state.IsValidated
		return false, shouldStore, nil
	}

	// Вычисляем параметры движения
	distance := s.calculateDistance(
		state.LastPosition.Latitude, state.LastPosition.Longitude,
		pilot.Position.Latitude, pilot.Position.Longitude,
	)
	timeDelta := pilot.LastUpdate.Sub(state.LastUpdate)

	// Проверяем временной интервал
	if timeDelta > s.config.MaxTimeDelta {
		// Слишком большой интервал, начинаем заново с позиции
		s.logger.WithFields(map[string]interface{}{
			"device_id": pilot.DeviceID,
			"time_delta": timeDelta,
			"max_delta": s.config.MaxTimeDelta,
			"validation_score": state.ValidationScore,
		}).Debug("Time delta too large, resetting position reference")
		
		state.FirstSeen = pilot.LastUpdate
		state.LastPosition = pilot.Position
		state.LastUpdate = pilot.LastUpdate
		
		// Большой интервал считаем невалидным пакетом
		s.updateValidationScore(state, false)
		
		// Проверяем гистерезис после изменения счета
		s.checkHysteresis(state, pilot.DeviceID)
		
		shouldStore := state.IsValidated
		return false, shouldStore, nil
	}

	// Проверяем нулевой/отрицательный интервал времени
	if timeDelta <= 0 {
		s.logger.WithField("device_id", pilot.DeviceID).
			Warn("Zero or negative time delta, skipping")
		
		// Невалидный пакет
		s.updateValidationScore(state, false)
		
		// Проверяем гистерезис после изменения счета
		s.checkHysteresis(state, pilot.DeviceID)
		
		shouldStore := state.IsValidated
		return false, shouldStore, nil
	}

	// Вычисляем скорость
	speedKmh := (distance / 1000.0) / timeDelta.Hours()
	maxSpeed := models.MaxSpeedByType(pilot.Type) * s.config.SpeedMultiplier
	
	// Определяем валидность на основе скорости
	isValid := speedKmh <= maxSpeed

	s.logger.WithFields(map[string]interface{}{
		"device_id": pilot.DeviceID,
		"speed_kmh": speedKmh,
		"max_speed": maxSpeed,
		"distance_m": distance,
		"time_delta": timeDelta,
		"aircraft_type": pilot.Type,
		"packet_count": state.PacketCount,
		"is_valid": isValid,
		"current_score": state.ValidationScore,
	}).Debug("Validating pilot movement")

	// Обновляем счет валидации
	s.updateValidationScore(state, isValid)

	// Обновляем позицию и время
	state.LastPosition = pilot.Position
	state.LastUpdate = pilot.LastUpdate
	
	// Проверяем гистерезис после изменения счета
	s.checkHysteresis(state, pilot.DeviceID)

	// Обновляем метрики пороговых значений
	s.updateMetrics()

	// Обновляем метрики
	if isValid {
		s.metrics.ValidatedPackets++
		metrics.ValidationValidatedPackets.Inc()
	} else {
		s.metrics.RejectedPackets++
		metrics.ValidationRejectedPackets.Inc()
		metrics.ValidationSpeedViolations.WithLabelValues(pilot.Type.String()).Inc()
		
		// Если скорость нереалистична, обновляем опорную точку
		state.FirstSeen = pilot.LastUpdate
	}

	// Определяем нужно ли сохранять в Redis на основе статуса валидации (гистерезис)
	shouldStore := state.IsValidated
	
	return isValid, shouldStore, nil
}

// InvalidateDevice сбрасывает валидацию для устройства
func (s *ValidationService) InvalidateDevice(deviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if state, exists := s.states[deviceID]; exists {
		state.IsValidated = false
		state.PacketCount = 0
		s.metrics.InvalidatedIDs++
		metrics.ValidationInvalidatedDevices.Inc()
		
		s.logger.WithField("device_id", deviceID).
			Info("Device ID invalidated")
		
		return nil
	}

	return fmt.Errorf("device ID not found: %s", deviceID)
}

// GetValidationState возвращает состояние валидации для устройства
func (s *ValidationService) GetValidationState(deviceID string) (*models.PilotValidationState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state, exists := s.states[deviceID]
	if !exists {
		return nil, false
	}

	// Возвращаем копию для безопасности
	stateCopy := *state
	return &stateCopy, true
}

// GetMetrics возвращает метрики валидации
func (s *ValidationService) GetMetrics() ValidationMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	return s.metrics
}

// CleanupOldStates удаляет старые состояния валидации
func (s *ValidationService) CleanupOldStates(maxAge time.Duration) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	removed := 0

	for id, state := range s.states {
		if now.Sub(state.LastUpdate) > maxAge {
			delete(s.states, id)
			removed++
		}
	}

	if removed > 0 {
		s.logger.WithField("removed", removed).
			Debug("Cleaned up old validation states")
		
		// Обновляем метрики
		s.updateMetrics()
	}

	return removed
}

// updateMetrics обновляет метрики на основе текущего состояния
func (s *ValidationService) updateMetrics() {
	totalStates := len(s.states)
	validatedDevices := 0
	
	for _, state := range s.states {
		if state.IsValidated {
			validatedDevices++
		}
	}
	
	metrics.ValidationActiveStates.Set(float64(totalStates))
	metrics.ValidationDevicesAboveThreshold.Set(float64(validatedDevices))
}

// updateValidationScore обновляет счет валидации на основе результата проверки пакета
func (s *ValidationService) updateValidationScore(state *models.PilotValidationState, isValid bool) {
	oldScore := state.ValidationScore
	
	if isValid {
		// Валидный пакет - увеличиваем счет
		state.ValidationScore += s.config.ValidPacketBonus
		if state.ValidationScore > s.config.MaxScore {
			state.ValidationScore = s.config.MaxScore
		}
		
		// Сбрасываем счетчик последовательных невалидных пакетов
		state.ConsecutiveInvalidPackets = 0
		
		// Обновляем метрики
		metrics.ValidationScoreChanges.WithLabelValues("increase").Inc()
		metrics.ValidationScoreDistribution.Observe(float64(state.ValidationScore))
		
		s.logger.WithFields(map[string]interface{}{
			"device_id": state.DeviceID,
			"old_score": oldScore,
			"new_score": state.ValidationScore,
			"change": fmt.Sprintf("+%d", s.config.ValidPacketBonus),
		}).Debug("Validation score increased for valid packet")
		
	} else {
		// Невалидный пакет - уменьшаем счет
		state.ValidationScore -= s.config.InvalidPacketPenalty
		if state.ValidationScore < 0 {
			state.ValidationScore = 0
		}
		
		// Увеличиваем счетчик последовательных невалидных пакетов
		state.ConsecutiveInvalidPackets++
		
		// Обновляем метрики
		metrics.ValidationScoreChanges.WithLabelValues("decrease").Inc()
		metrics.ValidationScoreDistribution.Observe(float64(state.ValidationScore))
		
		s.logger.WithFields(map[string]interface{}{
			"device_id": state.DeviceID,
			"old_score": oldScore,
			"new_score": state.ValidationScore,
			"change": fmt.Sprintf("-%d", s.config.InvalidPacketPenalty),
			"consecutive_invalid": state.ConsecutiveInvalidPackets,
		}).Debug("Validation score decreased for invalid packet")
	}
}

// calculateDistance вычисляет расстояние между двумя точками в метрах (формула гаверсинуса)
func (s *ValidationService) calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000 // Радиус Земли в метрах
	
	lat1Rad := lat1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	deltaLat := (lat2 - lat1) * math.Pi / 180
	deltaLon := (lon2 - lon1) * math.Pi / 180
	
	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
		math.Sin(deltaLon/2)*math.Sin(deltaLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	
	return R * c
}

// checkHysteresis проверяет и обновляет статус валидации с учетом гистерезиса
func (s *ValidationService) checkHysteresis(state *models.PilotValidationState, deviceID string) {
	// Устанавливаем флаг валидации если набрали достаточный счет (высокий порог гистерезиса)
	if !state.IsValidated && state.ValidationScore >= s.config.MinScoreToAdd {
		state.IsValidated = true
		s.logger.WithField("device_id", deviceID).
			WithField("validation_score", state.ValidationScore).
			WithField("add_threshold", s.config.MinScoreToAdd).
			Info("Device ID reached validation score for API inclusion")
	}

	// Сбрасываем валидацию если счет упал ниже низкого порога (гистерезис)
	if state.IsValidated && state.ValidationScore <= s.config.MaxScoreToRemove {
		state.IsValidated = false
		s.logger.WithField("device_id", deviceID).
			WithField("validation_score", state.ValidationScore).
			WithField("removal_threshold", s.config.MaxScoreToRemove).
			Warn("Device ID dropped below removal threshold (hysteresis)")
		
		s.metrics.InvalidatedIDs++
		metrics.ValidationInvalidatedDevices.Inc()
	}
}