package service

import (
	"testing"
	"time"

	"github.com/flybeeper/fanet-backend/internal/models"
	"github.com/flybeeper/fanet-backend/pkg/utils"
	"github.com/stretchr/testify/assert"
)

func TestBoundaryTracker(t *testing.T) {
	logger := utils.NewLogger("debug", "text")
	
	// OGN центр в Словении (Любляна)
	ognCenter := models.GeoPoint{
		Latitude:  46.0569,
		Longitude: 14.5058,
	}
	ognRadiusKM := 200.0
	trackingRadiusPercent := 0.9  // 180km внутренний радиус
	gracePeriod := 5 * time.Minute
	minMovementDist := 100.0

	tracker := NewBoundaryTracker(logger, ognCenter, ognRadiusKM, trackingRadiusPercent, gracePeriod, minMovementDist)

	t.Run("object inside tracking zone", func(t *testing.T) {
		// Позиция внутри tracking radius (180km)
		// Примерно 50км к северу от центра
		position := models.GeoPoint{
			Latitude:  46.5069,  // ~50km от центра
			Longitude: 14.5058,
		}
		
		status := tracker.GetObjectStatus(position, nil, time.Now())
		
		assert.True(t, status.IsInTrackingZone)
		assert.True(t, status.IsInMonitoringZone)
		assert.Equal(t, "visible", status.VisibilityStatus)
		assert.True(t, tracker.ShouldIncludeInSnapshot(position, time.Now()))
		assert.Equal(t, 1.0, tracker.CalculateVisibilityScore(status))
	})

	t.Run("object in boundary zone", func(t *testing.T) {
		// Позиция между tracking (180km) и OGN (200km) радиусом
		// Примерно 185км к северу от центра (1 градус широты ~ 111км)
		position := models.GeoPoint{
			Latitude:  47.7269,  // ~185km от центра
			Longitude: 14.5058,
		}
		
		status := tracker.GetObjectStatus(position, nil, time.Now())
		
		assert.False(t, status.IsInTrackingZone)
		assert.True(t, status.IsInMonitoringZone)
		assert.Equal(t, "boundary", status.VisibilityStatus)
		assert.True(t, tracker.ShouldIncludeInSnapshot(position, time.Now()))
	})

	t.Run("object outside OGN zone", func(t *testing.T) {
		// Позиция за пределами OGN радиуса (>200km)
		// Примерно 220км к северу от центра
		position := models.GeoPoint{
			Latitude:  48.0369,  // ~220km от центра
			Longitude: 14.5058,
		}
		
		status := tracker.GetObjectStatus(position, nil, time.Now())
		
		assert.False(t, status.IsInTrackingZone)
		assert.False(t, status.IsInMonitoringZone)
		assert.Equal(t, "outside", status.VisibilityStatus)
		assert.False(t, tracker.ShouldIncludeInSnapshot(position, time.Now()))
		assert.Equal(t, 0.0, tracker.CalculateVisibilityScore(status))
	})

	t.Run("object in boundary zone with expired grace period", func(t *testing.T) {
		// Позиция на границе
		position := models.GeoPoint{
			Latitude:  47.7269,  // ~185km от центра
			Longitude: 14.5058,
		}
		
		oldMovementTime := time.Now().Add(-10 * time.Minute) // Прошло больше grace period
		status := tracker.GetObjectStatus(position, nil, oldMovementTime)
		
		assert.False(t, status.IsInTrackingZone)
		assert.True(t, status.IsInMonitoringZone)
		assert.Equal(t, "outside", status.VisibilityStatus) // Изменился из-за истечения grace period
		assert.False(t, tracker.ShouldIncludeInSnapshot(position, oldMovementTime))
	})

	t.Run("movement detection", func(t *testing.T) {
		oldPosition := models.GeoPoint{
			Latitude:  46.0,
			Longitude: 14.0,
		}
		
		// Новая позиция с движением > 100м
		// Примерно 200м движение
		newPosition := models.GeoPoint{
			Latitude:  46.0018,  // ~200м к северу
			Longitude: 14.0,
		}
		
		oldTime := time.Now().Add(-1 * time.Minute)
		status := tracker.GetObjectStatus(newPosition, &oldPosition, oldTime)
		
		// LastMovement должен обновиться, так как движение больше минимального
		assert.True(t, status.LastMovement.After(oldTime))
	})

	t.Run("visibility score calculation", func(t *testing.T) {
		// Позиция на границе
		position := models.GeoPoint{
			Latitude:  47.7269,
			Longitude: 14.5058,
		}
		
		// Проверяем плавное уменьшение видимости в течение grace period
		for i := 0; i <= 5; i++ {
			movementTime := time.Now().Add(-time.Duration(i) * time.Minute)
			status := tracker.GetObjectStatus(position, nil, movementTime)
			status.LastMovement = movementTime
			
			score := tracker.CalculateVisibilityScore(status)
			
			if i == 0 {
				assert.InDelta(t, 1.0, score, 0.000001) // Полная видимость в начале
			} else if i < 5 {
				assert.True(t, score > 0.3 && score < 1.0) // Плавное уменьшение
			} else {
				assert.Equal(t, 0.0, score) // Нулевая видимость после grace period
			}
		}
	})
	
	t.Run("GetOGNInfo returns correct configuration", func(t *testing.T) {
		center, ognRadius, trackingRadius := tracker.GetOGNInfo()
		
		assert.Equal(t, ognCenter.Latitude, center.Latitude)
		assert.Equal(t, ognCenter.Longitude, center.Longitude)
		assert.Equal(t, 200.0, ognRadius)
		assert.Equal(t, 180.0, trackingRadius) // 90% от 200km
	})
}