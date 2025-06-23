package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/flybeeper/fanet-backend/internal/service"
)

// ValidationHandler обработчик для валидации устройств
type ValidationHandler struct {
	validationService *service.ValidationService
}

// NewValidationHandler создает новый обработчик валидации
func NewValidationHandler(validationService *service.ValidationService) *ValidationHandler {
	return &ValidationHandler{
		validationService: validationService,
	}
}

// InvalidateDevice обрабатывает запрос на инвалидацию устройства
// POST /api/v1/invalidate/:device_id
func (h *ValidationHandler) InvalidateDevice(c *gin.Context) {
	deviceID := c.Param("device_id")
	if deviceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "device_id is required",
		})
		return
	}

	if err := h.validationService.InvalidateDevice(deviceID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Device invalidated successfully",
		"device_id": deviceID,
	})
}

// GetValidationState возвращает состояние валидации устройства
// GET /api/v1/validation/:device_id
func (h *ValidationHandler) GetValidationState(c *gin.Context) {
	deviceID := c.Param("device_id")
	if deviceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "device_id is required",
		})
		return
	}

	state, exists := h.validationService.GetValidationState(deviceID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Device not found",
		})
		return
	}

	c.JSON(http.StatusOK, state)
}

// GetValidationMetrics возвращает метрики валидации
// GET /api/v1/validation/metrics
func (h *ValidationHandler) GetValidationMetrics(c *gin.Context) {
	metrics := h.validationService.GetMetrics()
	
	c.JSON(http.StatusOK, gin.H{
		"total_packets": metrics.TotalPackets,
		"validated_packets": metrics.ValidatedPackets,
		"rejected_packets": metrics.RejectedPackets,
		"invalidated_ids": metrics.InvalidatedIDs,
		"validation_rate": float64(metrics.ValidatedPackets) / float64(metrics.TotalPackets) * 100,
	})
}