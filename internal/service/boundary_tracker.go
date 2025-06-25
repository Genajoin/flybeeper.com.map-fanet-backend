package service

import (
	"sync"
	"time"

	"github.com/flybeeper/fanet-backend/internal/models"
	"github.com/flybeeper/fanet-backend/pkg/utils"
)

// BoundaryTracker отслеживает границы OGN и определяет, какие объекты включать в snapshot
type BoundaryTracker struct {
	mu                 sync.RWMutex
	ognCenter          models.GeoPoint
	ognRadiusKM        float64       // Внешний радиус OGN (откуда приходят данные)
	trackingRadiusKM   float64       // Внутренний радиус для включения в snapshot
	gracePeriod        time.Duration
	minMovementDist    float64       // Минимальное расстояние для считывания движения (в метрах)
	logger             *utils.Logger
}

// ObjectStatus статус объекта относительно границ OGN
type ObjectStatus struct {
	IsInTrackingZone   bool      // Внутри зоны отслеживания (включается в снимок)
	IsInMonitoringZone bool      // Внутри зоны мониторинга OGN (данные поступают)
	Distance           float64   // Расстояние от центра OGN в км
	LastMovement       time.Time // Время последнего существенного движения
	VisibilityStatus   string    // "visible", "boundary", "outside"
}

// NewBoundaryTracker создает новый трекер границ с одним статическим центром OGN
func NewBoundaryTracker(logger *utils.Logger, ognCenter models.GeoPoint, ognRadiusKM float64, trackingRadiusPercent float64, gracePeriod time.Duration, minMovementDist float64) *BoundaryTracker {
	if trackingRadiusPercent <= 0 || trackingRadiusPercent > 1 {
		trackingRadiusPercent = 0.9 // По умолчанию 90%
	}
	
	trackingRadiusKM := ognRadiusKM * trackingRadiusPercent
	
	bt := &BoundaryTracker{
		ognCenter:        ognCenter,
		ognRadiusKM:      ognRadiusKM,
		trackingRadiusKM: trackingRadiusKM,
		gracePeriod:      gracePeriod,
		minMovementDist:  minMovementDist,
		logger:           logger,
	}
	
	logger.WithFields(map[string]interface{}{
		"ogn_center_lat":    ognCenter.Latitude,
		"ogn_center_lon":    ognCenter.Longitude,
		"ogn_radius_km":     ognRadiusKM,
		"tracking_radius_km": trackingRadiusKM,
		"grace_period":      gracePeriod,
		"min_movement_dist": minMovementDist,
	}).Info("Initialized OGN boundary tracker")
	
	return bt
}

// GetObjectStatus определяет статус объекта относительно границ OGN
func (bt *BoundaryTracker) GetObjectStatus(position models.GeoPoint, lastPosition *models.GeoPoint, lastUpdate time.Time) ObjectStatus {
	bt.mu.RLock()
	defer bt.mu.RUnlock()

	// Вычисляем расстояние от центра OGN в километрах
	distanceKM := position.DistanceTo(bt.ognCenter)
	
	status := ObjectStatus{
		IsInTrackingZone:   false,
		IsInMonitoringZone: false,
		Distance:           distanceKM,
		LastMovement:       lastUpdate,
		VisibilityStatus:   "outside",
	}

	// Проверяем движение если есть предыдущая позиция
	if lastPosition != nil {
		movementDistKM := position.DistanceTo(*lastPosition)
		movementDistM := movementDistKM * 1000 // конвертируем в метры
		if movementDistM >= bt.minMovementDist {
			status.LastMovement = time.Now()
		}
	}

	// Определяем статус относительно зон
	if distanceKM <= bt.trackingRadiusKM {
		// Внутри tracking radius - полностью видим
		status.IsInTrackingZone = true
		status.IsInMonitoringZone = true
		status.VisibilityStatus = "visible"
	} else if distanceKM <= bt.ognRadiusKM {
		// Между tracking и OGN радиусом - на границе
		status.IsInMonitoringZone = true
		status.VisibilityStatus = "boundary"
		
		// Применяем grace period для объектов на границе
		if time.Since(status.LastMovement) > bt.gracePeriod {
			status.VisibilityStatus = "outside"
		}
	}
	// else - за пределами OGN радиуса, статус уже "outside"

	return status
}

// ShouldIncludeInSnapshot определяет, должен ли объект быть включен в снимок
func (bt *BoundaryTracker) ShouldIncludeInSnapshot(position models.GeoPoint, lastMovement time.Time) bool {
	status := bt.GetObjectStatus(position, nil, lastMovement)
	
	// Включаем в снимок если:
	// 1. Объект внутри tracking zone
	// 2. Объект на границе, но в пределах grace period
	return status.IsInTrackingZone || 
		(status.VisibilityStatus == "boundary" && time.Since(lastMovement) <= bt.gracePeriod)
}

// CalculateVisibilityScore вычисляет коэффициент видимости объекта (0.0 - 1.0)
// Используется для плавного исчезновения объектов на границе
func (bt *BoundaryTracker) CalculateVisibilityScore(status ObjectStatus) float64 {
	if status.VisibilityStatus == "visible" {
		return 1.0
	}

	if status.VisibilityStatus == "outside" {
		return 0.0
	}

	// Для объектов на границе - плавное уменьшение видимости
	timeSinceMovement := time.Since(status.LastMovement)
	if timeSinceMovement >= bt.gracePeriod {
		return 0.0
	}

	// Линейное уменьшение от 1.0 до 0.3 в течение grace period
	fadeRatio := float64(timeSinceMovement) / float64(bt.gracePeriod)
	return 1.0 - (fadeRatio * 0.7) // Минимум 0.3 для видимости
}

// GetOGNInfo возвращает информацию о конфигурации OGN центра
func (bt *BoundaryTracker) GetOGNInfo() (center models.GeoPoint, ognRadiusKM, trackingRadiusKM float64) {
	bt.mu.RLock()
	defer bt.mu.RUnlock()
	
	return bt.ognCenter, bt.ognRadiusKM, bt.trackingRadiusKM
}