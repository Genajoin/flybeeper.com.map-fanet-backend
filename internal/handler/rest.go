package handler

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/flybeeper/fanet-backend/internal/auth"
	"github.com/flybeeper/fanet-backend/internal/models"
	"github.com/flybeeper/fanet-backend/internal/repository"
	"github.com/flybeeper/fanet-backend/pkg/pb"
	"github.com/flybeeper/fanet-backend/pkg/utils"
	"google.golang.org/protobuf/proto"
)

// RESTHandler обработчик REST API endpoints
type RESTHandler struct {
	repo    repository.Repository
	logger  *utils.Logger
	timeout time.Duration
}

// NewRESTHandler создает новый REST handler
func NewRESTHandler(repo repository.Repository, logger *utils.Logger) *RESTHandler {
	return &RESTHandler{
		repo:    repo,
		logger:  logger,
		timeout: 30 * time.Second,
	}
}

// GetSnapshot возвращает начальный снимок всех объектов в радиусе
// GET /api/v1/snapshot?lat=46.5&lon=15.6&radius=200
func (h *RESTHandler) GetSnapshot(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.timeout)
	defer cancel()

	// Парсинг параметров согласно OpenAPI спецификации
	lat, err := strconv.ParseFloat(c.Query("lat"), 64)
	if err != nil || lat < -90 || lat > 90 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "invalid_latitude",
			"message": "Latitude must be between -90 and 90",
		})
		return
	}

	lon, err := strconv.ParseFloat(c.Query("lon"), 64)
	if err != nil || lon < -180 || lon > 180 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "invalid_longitude", 
			"message": "Longitude must be between -180 and 180",
		})
		return
	}

	radius, err := strconv.Atoi(c.Query("radius"))
	if err != nil || radius < 1 || radius > 200 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "invalid_radius",
			"message": "Radius must be between 1 and 200 km",
		})
		return
	}

	center := models.GeoPoint{
		Latitude:  lat,
		Longitude: lon,
	}

	// Получаем данные из репозитория
	pilots, err := h.repo.GetPilotsInRadius(ctx, center, float64(radius))
	if err != nil {
		h.logger.WithField("error", err).Error("Failed to get pilots")
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "internal_error",
			"message": "Failed to retrieve pilots",
		})
		return
	}

	thermals, err := h.repo.GetThermalsInRadius(ctx, center, float64(radius))
	if err != nil {
		h.logger.WithField("error", err).Error("Failed to get thermals")
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "internal_error",
			"message": "Failed to retrieve thermals",
		})
		return
	}

	// Для snapshot получаем все станции, не только в радиусе
	// (станции без координат не индексируются в GEO, но сохраняются в Redis)
	stations, err := h.repo.GetAllStations(ctx)
	if err != nil {
		h.logger.WithField("error", err).Error("Failed to get stations")
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "internal_error",
			"message": "Failed to retrieve stations",
		})
		return
	}

	// Создаем Protobuf ответ
	response := &pb.SnapshotResponse{
		Pilots:   convertPilotsToProto(pilots),
		Thermals: convertThermalsToProto(thermals),
		Stations: convertStationsToProto(stations),
		Sequence: uint64(time.Now().Unix()), // Простая последовательность
	}

	// Определяем формат ответа по Accept header
	contentType := c.GetHeader("Accept")
	if strings.Contains(contentType, "application/x-protobuf") {
		// Protobuf ответ
		data, err := proto.Marshal(response)
		if err != nil {
			h.logger.WithField("error", err).Error("Failed to marshal protobuf")
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    "marshal_error",
				"message": "Failed to serialize response",
			})
			return
		}

		c.Data(http.StatusOK, "application/x-protobuf", data)
	} else {
		// JSON ответ (fallback)
		c.JSON(http.StatusOK, convertSnapshotToJSON(response))
	}

	h.logger.WithFields(map[string]interface{}{
		"lat":      lat,
		"lon":      lon,
		"radius":   radius,
		"pilots":   len(pilots),
		"thermals": len(thermals),
		"stations": len(stations),
	}).Info("Snapshot request completed")
}

// GetPilots возвращает пилотов в указанных границах
// GET /api/v1/pilots?bounds=45.5,15.0,47.5,16.2
func (h *RESTHandler) GetPilots(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.timeout)
	defer cancel()

	bounds, err := parseBounds(c.Query("bounds"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "invalid_bounds",
			"message": "Bounds must be: sw_lat,sw_lon,ne_lat,ne_lon",
		})
		return
	}

	// Вычисляем центр и радиус из bounds
	center, radius := boundsToRadiusQuery(bounds)

	pilots, err := h.repo.GetPilotsInRadius(ctx, center, radius)
	if err != nil {
		h.logger.WithField("error", err).Error("Failed to get pilots")
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "internal_error",
			"message": "Failed to retrieve pilots",
		})
		return
	}

	response := &pb.PilotsResponse{
		Pilots: convertPilotsToProto(pilots),
	}

	if strings.Contains(c.GetHeader("Accept"), "application/x-protobuf") {
		data, err := proto.Marshal(response)
		if err != nil {
			h.logger.WithField("error", err).Error("Failed to marshal protobuf")
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    "marshal_error",
				"message": "Failed to serialize response",
			})
			return
		}
		c.Data(http.StatusOK, "application/x-protobuf", data)
	} else {
		c.JSON(http.StatusOK, map[string]interface{}{
			"pilots": convertPilotsToJSONArray(pilots),
		})
	}
}

// GetThermals возвращает термики в указанных границах
// GET /api/v1/thermals?bounds=45.5,15.0,47.5,16.2&min_quality=3
func (h *RESTHandler) GetThermals(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.timeout)
	defer cancel()

	bounds, err := parseBounds(c.Query("bounds"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "invalid_bounds",
			"message": "Bounds must be: sw_lat,sw_lon,ne_lat,ne_lon",
		})
		return
	}

	// Минимальное качество (опционально)
	minQuality := 0
	if q := c.Query("min_quality"); q != "" {
		if mq, err := strconv.Atoi(q); err == nil && mq >= 0 && mq <= 5 {
			minQuality = mq
		}
	}

	center, radius := boundsToRadiusQuery(bounds)

	thermals, err := h.repo.GetThermalsInRadius(ctx, center, radius)
	if err != nil {
		h.logger.WithField("error", err).Error("Failed to get thermals")
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "internal_error",
			"message": "Failed to retrieve thermals",
		})
		return
	}

	// Фильтруем по минимальному качеству
	if minQuality > 0 {
		filtered := make([]*models.Thermal, 0, len(thermals))
		for _, thermal := range thermals {
			if int(thermal.Quality) >= minQuality {
				filtered = append(filtered, thermal)
			}
		}
		thermals = filtered
	}

	response := &pb.ThermalsResponse{
		Thermals: convertThermalsToProto(thermals),
	}

	if strings.Contains(c.GetHeader("Accept"), "application/x-protobuf") {
		data, err := proto.Marshal(response)
		if err != nil {
			h.logger.WithField("error", err).Error("Failed to marshal protobuf")
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    "marshal_error",
				"message": "Failed to serialize response",
			})
			return
		}
		c.Data(http.StatusOK, "application/x-protobuf", data)
	} else {
		c.JSON(http.StatusOK, map[string]interface{}{
			"thermals": convertThermalsToJSONArray(thermals),
		})
	}
}

// GetStations возвращает метеостанции в указанных границах
// GET /api/v1/stations?bounds=45.5,15.0,47.5,16.2
func (h *RESTHandler) GetStations(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.timeout)
	defer cancel()

	bounds, err := parseBounds(c.Query("bounds"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "invalid_bounds",
			"message": "Bounds must be: sw_lat,sw_lon,ne_lat,ne_lon",
		})
		return
	}

	center, radius := boundsToRadiusQuery(bounds)

	stations, err := h.repo.GetStationsInRadius(ctx, center, radius)
	if err != nil {
		h.logger.WithField("error", err).Error("Failed to get stations")
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "internal_error",
			"message": "Failed to retrieve stations",
		})
		return
	}

	response := &pb.StationsResponse{
		Stations: convertStationsToProto(stations),
	}

	if strings.Contains(c.GetHeader("Accept"), "application/x-protobuf") {
		data, err := proto.Marshal(response)
		if err != nil {
			h.logger.WithField("error", err).Error("Failed to marshal protobuf")
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    "marshal_error",
				"message": "Failed to serialize response",
			})
			return
		}
		c.Data(http.StatusOK, "application/x-protobuf", data)
	} else {
		c.JSON(http.StatusOK, map[string]interface{}{
			"stations": convertStationsToJSONArray(stations),
		})
	}
}

// GetTrack возвращает трек полета пилота
// GET /api/v1/track/{addr}?hours=12
func (h *RESTHandler) GetTrack(c *gin.Context) {
	_, cancel := context.WithTimeout(c.Request.Context(), h.timeout)
	defer cancel()

	// Парсинг FANET адреса
	addrStr := c.Param("addr")
	if addrStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "missing_addr",
			"message": "FANET address is required",
		})
		return
	}

	// Часы истории (по умолчанию 12)
	hours := 12
	if h := c.Query("hours"); h != "" {
		if hrs, err := strconv.Atoi(h); err == nil && hrs >= 1 && hrs <= 12 {
			hours = hrs
		}
	}

	// Для получения трека нужен доступ к MySQL репозиторию
	// Пока что возвращаем заглушку
	track := []models.GeoPoint{
		{
			Latitude:  46.5,
			Longitude: 15.6,
			Altitude:  1000,
		},
	}

	// TODO: Реализовать получение трека через service layer
	err := error(nil)
	if err != nil {
		h.logger.WithField("error", err).WithField("addr", addrStr).Error("Failed to get pilot track")
		c.JSON(http.StatusNotFound, gin.H{
			"code":    "track_not_found",
			"message": "Pilot track not found",
		})
		return
	}

	if len(track) == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    "track_empty",
			"message": "No track data available",
		})
		return
	}

	// Конвертируем адрес в uint32
	addr, err := strconv.ParseUint(addrStr, 16, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "invalid_addr_format",
			"message": "Invalid FANET address format",
		})
		return
	}

	response := &pb.TrackResponse{
		Track: &pb.Track{
			Addr:      uint32(addr),
			Points:    convertTrackToProto(track),
			StartTime: time.Now().Add(-time.Duration(hours) * time.Hour).Unix(),
			EndTime:   time.Now().Unix(),
		},
	}

	if strings.Contains(c.GetHeader("Accept"), "application/x-protobuf") {
		data, err := proto.Marshal(response)
		if err != nil {
			h.logger.WithField("error", err).Error("Failed to marshal protobuf")
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    "marshal_error",
				"message": "Failed to serialize response",
			})
			return
		}
		c.Data(http.StatusOK, "application/x-protobuf", data)
	} else {
		c.JSON(http.StatusOK, convertTrackToJSON(response.Track))
	}
}

// PostPosition принимает позицию от пилота (требует аутентификации)
// POST /api/v1/position
func (h *RESTHandler) PostPosition(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.timeout)
	defer cancel()

	// Получаем пользователя из контекста (установлен middleware)
	user, exists := auth.GetUser(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    "auth_required",
			"message": "Authentication required",
		})
		return
	}

	userID, _ := auth.GetUserID(c)

	var request pb.PositionRequest

	// Парсинг Protobuf или JSON
	contentType := c.GetHeader("Content-Type")
	if strings.Contains(contentType, "application/x-protobuf") {
		data, err := c.GetRawData()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    "read_error",
				"message": "Failed to read request body",
			})
			return
		}

		if err := proto.Unmarshal(data, &request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    "unmarshal_error",
				"message": "Failed to parse protobuf data",
			})
			return
		}
	} else {
		// JSON fallback
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    "json_error",
				"message": "Invalid JSON format",
			})
			return
		}
	}

	// Валидация данных
	if request.Position == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "missing_position",
			"message": "Position is required",
		})
		return
	}

	if request.Position.Latitude < -90 || request.Position.Latitude > 90 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "invalid_latitude",
			"message": "Invalid latitude",
		})
		return
	}

	if request.Position.Longitude < -180 || request.Position.Longitude > 180 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "invalid_longitude",
			"message": "Invalid longitude",
		})
		return
	}

	// Создаем pilot модель с данными пользователя
	pilot := &models.Pilot{
		DeviceID:     fmt.Sprintf("user_%d", userID), // Используем user ID как FANET адрес
		Name:         user.Name,
		AircraftType: uint8(models.PilotTypeParaglider), // По умолчанию
		Position: &models.GeoPoint{
			Latitude:  request.Position.Latitude,
			Longitude: request.Position.Longitude,
			Altitude:  int16(request.Altitude),
		},
		Speed:      float32(request.Speed),
		ClimbRate:  int16(request.Climb * 10), // Конвертируем м/с в дециметры/с
		Heading:    float32(request.Course),
		LastUpdate: time.Unix(request.Timestamp, 0),
	}

	// Сохраняем позицию через репозиторий
	if err := h.repo.SavePilot(ctx, pilot); err != nil {
		h.logger.WithFields(map[string]interface{}{
			"user_id": userID,
			"error":   err,
		}).Error("Failed to save pilot position")
		
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "save_error",
			"message": "Failed to save position",
		})
		return
	}

	response := &pb.PositionResponse{
		Success: true,
	}

	if strings.Contains(c.GetHeader("Accept"), "application/x-protobuf") {
		data, err := proto.Marshal(response)
		if err != nil {
			h.logger.WithField("error", err).Error("Failed to marshal protobuf")
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    "marshal_error",
				"message": "Failed to serialize response",
			})
			return
		}
		c.Data(http.StatusOK, "application/x-protobuf", data)
	} else {
		c.JSON(http.StatusOK, map[string]interface{}{
			"success": true,
		})
	}

	h.logger.WithFields(map[string]interface{}{
		"user_id": userID,
		"user":    user.Email,
		"lat":     request.Position.Latitude,
		"lon":     request.Position.Longitude,
		"alt":     request.Altitude,
	}).Info("Position update received and saved")
}

// Вспомогательные функции
func parseBounds(boundsStr string) (*models.Bounds, error) {
	if boundsStr == "" {
		return nil, fmt.Errorf("bounds parameter is required")
	}

	parts := strings.Split(boundsStr, ",")
	if len(parts) != 4 {
		return nil, fmt.Errorf("bounds must have 4 values")
	}

	coords := make([]float64, 4)
	for i, part := range parts {
		coord, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			return nil, fmt.Errorf("invalid coordinate: %s", part)
		}
		coords[i] = coord
	}

	return &models.Bounds{
		Southwest: models.GeoPoint{
			Latitude:  coords[0],
			Longitude: coords[1],
		},
		Northeast: models.GeoPoint{
			Latitude:  coords[2],
			Longitude: coords[3],
		},
	}, nil
}

func boundsToRadiusQuery(bounds *models.Bounds) (models.GeoPoint, float64) {
	// Вычисляем центр
	center := models.GeoPoint{
		Latitude:  (bounds.Southwest.Latitude + bounds.Northeast.Latitude) / 2,
		Longitude: (bounds.Southwest.Longitude + bounds.Northeast.Longitude) / 2,
	}

	// Вычисляем радиус как расстояние от центра до угла
	radius := center.DistanceTo(bounds.Northeast)

	return center, radius
}