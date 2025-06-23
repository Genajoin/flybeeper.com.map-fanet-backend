package models

import (
	"time"
)

// PilotValidationState представляет состояние валидации пилота
type PilotValidationState struct {
	DeviceID                   string    `json:"device_id"`
	FirstSeen                  time.Time `json:"first_seen"`
	LastPosition               *GeoPoint `json:"last_position"`
	LastUpdate                 time.Time `json:"last_update"` 
	IsValidated                bool      `json:"is_validated"`
	PacketCount                int       `json:"packet_count"`
	AircraftType               PilotType `json:"aircraft_type"`
	ValidationScore            int       `json:"validation_score"`              // Счет валидации (0-100)
	ConsecutiveInvalidPackets  int       `json:"consecutive_invalid_packets"`    // Последовательные невалидные пакеты
}

// MaxSpeedByType возвращает максимальную скорость для типа ЛА (км/ч)
func MaxSpeedByType(pilotType PilotType) float64 {
	switch pilotType {
	case PilotTypeParaglider:
		return 80.0
	case PilotTypeHangglider:
		return 120.0
	case PilotTypePowered: // Мотопараплан
		return 1000.0
	case PilotTypeGlider:
		return 300.0
	case PilotTypeBalloon:
		return 60.0
	case PilotTypeHelicopter:
		return 300.0
	case PilotTypeUAV:
		return 100.0
	default:
		return 100.0 // Консервативное значение для неизвестных типов
	}
}

// ValidationConfig конфигурация для валидации
type ValidationConfig struct {
	MaxTimeDelta         time.Duration // Максимальное время между точками для валидации
	SpeedMultiplier      float64       // Множитель для максимальной скорости (например, 1.2 = +20%)
	MinPacketsToValid    int           // Минимальное количество пакетов для валидации (2)
	InitialScore         int           // Начальный счет валидации
	ValidPacketBonus     int           // Бонус за валидный пакет
	InvalidPacketPenalty int           // Штраф за невалидный пакет
	MinScoreToAdd        int           // Минимальный счет для появления в Redis (высокий порог гистерезиса)
	MaxScoreToRemove     int           // Максимальный счет для удаления из Redis (низкий порог гистерезиса)
	MaxScore             int           // Максимальный счет валидации
}

// DefaultValidationConfig возвращает конфигурацию по умолчанию
func DefaultValidationConfig() *ValidationConfig {
	return &ValidationConfig{
		MaxTimeDelta:         30 * time.Minute,
		SpeedMultiplier:      1.2, // +20% к максимальной скорости
		MinPacketsToValid:    2,
		InitialScore:         50,  // Начинаем с середины
		ValidPacketBonus:     15,  // Быстро растем при валидных пакетах (было 3)
		InvalidPacketPenalty: 25,  // Быстро падаем при невалидных (было 10)
		MinScoreToAdd:        70,  // Высокий порог для появления в API (гистерезис)
		MaxScoreToRemove:     30,  // Низкий порог для удаления из API (гистерезис)
		MaxScore:             100, // Максимальный счет
	}
}