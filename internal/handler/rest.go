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
	"github.com/flybeeper/fanet-backend/internal/filter"
	"github.com/flybeeper/fanet-backend/internal/models"
	"github.com/flybeeper/fanet-backend/internal/repository"
	"github.com/flybeeper/fanet-backend/internal/service"
	"github.com/flybeeper/fanet-backend/pkg/pb"
	"github.com/flybeeper/fanet-backend/pkg/utils"
	"google.golang.org/protobuf/proto"
)

// RESTHandler обработчик REST API endpoints
type RESTHandler struct {
	repo            repository.Repository
	historyRepo     repository.HistoryRepository
	logger          *utils.Logger
	timeout         time.Duration
	boundaryTracker *service.BoundaryTracker
}

// NewRESTHandler создает новый REST handler
func NewRESTHandler(repo repository.Repository, historyRepo repository.HistoryRepository, logger *utils.Logger, boundaryTracker *service.BoundaryTracker) *RESTHandler {
	return &RESTHandler{
		repo:            repo,
		historyRepo:     historyRepo,
		logger:          logger,
		timeout:         30 * time.Second,
		boundaryTracker: boundaryTracker,
	}
}

// GetSnapshot возвращает начальный снимок всех объектов в радиусе
// GET /api/v1/snapshot?lat=46.5&lon=15.6&radius=200&air-types=1,2,5&ground-types=1,2,4&max_age=300&pilots=true&stations=true&thermals=true&ground_objects=true
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

	// Парсинг параметра max_age (опционально, в секундах)
	maxAgeDuration := 24 * time.Hour // По умолчанию 24 часа
	if maxAgeParam := c.Query("max_age"); maxAgeParam != "" {
		maxAge, err := strconv.Atoi(maxAgeParam)
		if err != nil || maxAge < 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    "invalid_max_age",
				"message": "max_age must be a positive number of seconds",
			})
			return
		}
		maxAgeDuration = time.Duration(maxAge) * time.Second
	}

	// Парсинг параметров типов объектов (по умолчанию все true)
	includePilots := c.DefaultQuery("pilots", "true") == "true"
	includeStations := c.DefaultQuery("stations", "true") == "true"
	includeThermals := c.DefaultQuery("thermals", "true") == "true"
	includeGroundObjects := c.DefaultQuery("ground_objects", "true") == "true"

	// Парсинг параметра air-types (опционально)
	var filterAirTypes []models.PilotType
	airTypesParam := c.Query("air-types")
	if airTypesParam != "" {
		filterAirTypes, err = parseAirTypes(airTypesParam)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    "invalid_air_types",
				"message": err.Error(),
			})
			return
		}
	}

	// Парсинг параметра ground-types (опционально)
	var filterGroundTypes []models.GroundType
	groundTypesParam := c.Query("ground-types")
	if groundTypesParam != "" {
		filterGroundTypes, err = parseGroundTypes(groundTypesParam)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    "invalid_ground_types",
				"message": err.Error(),
			})
			return
		}
	}

	// Получаем данные из репозитория только для запрошенных типов
	var pilots []*models.Pilot
	var thermals []*models.Thermal
	var stations []*models.Station
	var groundObjects []*models.GroundObject

	// Получаем пилотов если включены
	if includePilots {
		pilots, err = h.repo.GetPilotsInRadius(ctx, center, float64(radius))
		if err != nil {
			h.logger.WithField("error", err).Error("Failed to get pilots")
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    "internal_error",
				"message": "Failed to retrieve pilots",
			})
			return
		}

		// Фильтруем пилотов по типам, если указан параметр air-types
		if len(filterAirTypes) > 0 {
			filtered := make([]*models.Pilot, 0, len(pilots))
			typeMap := make(map[models.PilotType]bool)
			for _, t := range filterAirTypes {
				typeMap[t] = true
			}
			for _, pilot := range pilots {
				if typeMap[pilot.Type] {
					filtered = append(filtered, pilot)
				}
			}
			pilots = filtered
		}

		// Фильтруем по max_age
		if maxAgeDuration < 24*time.Hour {
			filtered := make([]*models.Pilot, 0, len(pilots))
			for _, pilot := range pilots {
				if !pilot.IsStale(maxAgeDuration) {
					filtered = append(filtered, pilot)
				}
			}
			pilots = filtered
		}
		
		// Фильтруем по границам отслеживания
		if h.boundaryTracker != nil {
			filtered := make([]*models.Pilot, 0, len(pilots))
			for _, pilot := range pilots {
				if pilot.Position != nil {
					// Используем last_movement из модели если есть, иначе last_update
					lastMovement := pilot.LastUpdate
					if pilot.LastMovement != nil && !pilot.LastMovement.IsZero() {
						lastMovement = *pilot.LastMovement
					}
					
					if h.boundaryTracker.ShouldIncludeInSnapshot(*pilot.Position, lastMovement) {
						filtered = append(filtered, pilot)
					}
				}
			}
			pilots = filtered
		}
	}

	// Получаем термики если включены
	if includeThermals {
		thermals, err = h.repo.GetThermalsInRadius(ctx, center, float64(radius))
		if err != nil {
			h.logger.WithField("error", err).Error("Failed to get thermals")
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    "internal_error",
				"message": "Failed to retrieve thermals",
			})
			return
		}

		// Фильтруем по max_age
		if maxAgeDuration < 24*time.Hour {
			filtered := make([]*models.Thermal, 0, len(thermals))
			for _, thermal := range thermals {
				if !thermal.IsStale(maxAgeDuration) {
					filtered = append(filtered, thermal)
				}
			}
			thermals = filtered
		}
	}

	// Получаем наземные объекты если включены
	if includeGroundObjects {
		groundObjects, err = h.repo.GetGroundObjectsInRadius(ctx, center, float64(radius))
		if err != nil {
			h.logger.WithField("error", err).Error("Failed to get ground objects")
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    "internal_error",
				"message": "Failed to retrieve ground objects",
			})
			return
		}

		// Фильтруем наземные объекты по типам, если указан параметр ground-types
		if len(filterGroundTypes) > 0 && len(groundObjects) > 0 {
			filtered := make([]*models.GroundObject, 0, len(groundObjects))
			typeMap := make(map[models.GroundType]bool)
			for _, t := range filterGroundTypes {
				typeMap[t] = true
			}
			for _, obj := range groundObjects {
				if typeMap[obj.Type] {
					filtered = append(filtered, obj)
				}
			}
			groundObjects = filtered
		}

		// Фильтруем по max_age
		if maxAgeDuration < 24*time.Hour {
			filtered := make([]*models.GroundObject, 0, len(groundObjects))
			for _, obj := range groundObjects {
				if !obj.IsStale(maxAgeDuration) {
					filtered = append(filtered, obj)
				}
			}
			groundObjects = filtered
		}
	}

	// Получаем станции если включены
	if includeStations {
		// Для snapshot получаем все станции, не только в радиусе
		// (станции без координат не индексируются в GEO, но сохраняются в Redis)
		stations, err = h.repo.GetAllStations(ctx)
		if err != nil {
			h.logger.WithField("error", err).Error("Failed to get stations")
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    "internal_error",
				"message": "Failed to retrieve stations",
			})
			return
		}

		// Фильтруем по max_age
		if maxAgeDuration < 24*time.Hour {
			filtered := make([]*models.Station, 0, len(stations))
			for _, station := range stations {
				if !station.IsStale(maxAgeDuration) {
					filtered = append(filtered, station)
				}
			}
			stations = filtered
		}
	}

	// Создаем Protobuf ответ
	response := &pb.SnapshotResponse{
		Pilots:        convertPilotsToProto(pilots),
		GroundObjects: convertGroundObjectsToProto(groundObjects),
		Thermals:      convertThermalsToProto(thermals),
		Stations:      convertStationsToProto(stations),
		Sequence:      uint64(time.Now().Unix()), // Простая последовательность
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

	logFields := map[string]interface{}{
		"lat":            lat,
		"lon":            lon,
		"radius":         radius,
		"pilots":         len(pilots),
		"ground_objects": len(groundObjects),
		"thermals":       len(thermals),
		"stations":       len(stations),
	}
	if maxAgeDuration < 24*time.Hour {
		logFields["max_age_seconds"] = int(maxAgeDuration.Seconds())
	}
	if !includePilots || !includeStations || !includeThermals || !includeGroundObjects {
		logFields["include_types"] = map[string]bool{
			"pilots":         includePilots,
			"stations":       includeStations,
			"thermals":       includeThermals,
			"ground_objects": includeGroundObjects,
		}
	}
	if len(filterAirTypes) > 0 {
		logFields["filter_air_types"] = filterAirTypes
	}
	if len(filterGroundTypes) > 0 {
		logFields["filter_ground_types"] = filterGroundTypes
	}
	h.logger.WithFields(logFields).Info("Snapshot request completed")
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
// GET /api/v1/track/{addr}?hours=12&format=geojson&filter-level=1
// Параметры:
//   - hours: количество часов истории (1-12, по умолчанию 12)
//   - format: формат ответа (json/geojson, по умолчанию json)
//   - filter-level: уровень фильтрации (0-3, по умолчанию 0)
//     0 - без фильтрации (raw data)
//     1 - базовая: дубли + телепортации >200км
//     2 - средняя: уровень 1 + сегментация по времени (30 мин)
//     3 - полная: двухэтапная фильтрация
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

	// Формат ответа (по умолчанию geojson)
	format := c.DefaultQuery("format", "geojson")
	
	// Валидация формата
	if format != "json" && format != "geojson" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "invalid_format",
			"message": "Invalid format parameter. Supported: json, geojson",
		})
		return
	}

	// Уровень фильтрации (по умолчанию 2 - средний)
	filterLevelStr := c.DefaultQuery("filter-level", "3")
	filterLevel := 0
	if lvl, err := strconv.Atoi(filterLevelStr); err == nil && lvl >= 0 && lvl <= 3 {
		filterLevel = lvl
	}

	// Проверяем доступность historyRepo
	if h.historyRepo == nil {
		h.logger.Error("MySQL repository not available")
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"code":    "mysql_unavailable",
			"message": "MySQL history repository not available",
		})
		return
	}

	// Получаем трек из MySQL базы данных с временными метками
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.timeout)
	defer cancel()
	
	trackWithTimestamps, err := h.historyRepo.GetPilotTrackWithTimestamps(ctx, addrStr, 1000)
	if err != nil {
		h.logger.WithField("error", err).WithField("addr", addrStr).Error("Failed to get pilot track")
		c.JSON(http.StatusNotFound, gin.H{
			"code":    "track_not_found",
			"message": "Pilot track not found",
		})
		return
	}

	if len(trackWithTimestamps) == 0 {
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

	// Применяем фильтры в зависимости от уровня
	var filterResult *filter.FilterResult
	var filteredTrack []models.GeoPoint
	
	if filterLevel > 0 && len(trackWithTimestamps) > 1 {
		h.logger.WithField("device_id", addrStr).
			WithField("original_points", len(trackWithTimestamps)).
			WithField("filter_level", filterLevel).
			Debug("Applying track filters")
		
		// Получаем тип ЛА из базы данных
		aircraftType, err := h.historyRepo.GetPilotAircraftType(ctx, addrStr)
		if err != nil {
			h.logger.WithField("error", err).WithField("device_id", addrStr).
				Warn("Failed to get aircraft type, using default")
			aircraftType = models.PilotTypeUnknown
		}
		
		filterResult, err = applyTrackFiltersWithTimestamps(trackWithTimestamps, addrStr, aircraftType, filterLevel, h.logger)
		if err != nil {
			h.logger.WithField("error", err).WithField("device_id", addrStr).
				Warn("Failed to apply track filters, using original data")
			// Конвертируем в GeoPoint без фильтрации
			filteredTrack = make([]models.GeoPoint, len(trackWithTimestamps))
			for i, pt := range trackWithTimestamps {
				filteredTrack[i] = pt.GeoPoint
			}
		} else {
			filteredTrack = convertFilteredTrackToGeoPoints(filterResult)
			h.logger.WithField("device_id", addrStr).
				WithField("original_points", filterResult.OriginalCount).
				WithField("filtered_points", filterResult.FilteredCount).
				WithField("final_points", len(filteredTrack)).
				WithField("filter_level", filterLevel).
				Info("Track filters applied successfully")
		}
	} else {
		// Конвертируем в GeoPoint без фильтрации
		filteredTrack = make([]models.GeoPoint, len(trackWithTimestamps))
		for i, pt := range trackWithTimestamps {
			filteredTrack[i] = pt.GeoPoint
		}
	}

	response := &pb.TrackResponse{
		Track: &pb.Track{
			Addr:      uint32(addr),
			Points:    convertTrackToProto(filteredTrack),
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
	} else if format == "geojson" {
		if filterLevel > 0 && filterResult != nil {
			c.JSON(http.StatusOK, convertTrackToGeoJSONWithFilter(response.Track, filterResult))
		} else {
			c.JSON(http.StatusOK, convertTrackToGeoJSON(response.Track))
		}
	} else {
		if filterLevel > 0 && filterResult != nil {
			c.JSON(http.StatusOK, convertTrackToJSONWithFilter(response.Track, filterResult))
		} else {
			c.JSON(http.StatusOK, convertTrackToJSON(response.Track))
		}
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
		Type: models.PilotTypeParaglider, // По умолчанию
		Position: &models.GeoPoint{
			Latitude:  request.Position.Latitude,
			Longitude: request.Position.Longitude,
			Altitude:  request.Altitude,
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

func parseAirTypes(typesStr string) ([]models.PilotType, error) {
	if typesStr == "" {
		return nil, nil
	}

	typeStrings := strings.Split(typesStr, ",")
	types := make([]models.PilotType, 0, len(typeStrings))

	for _, typeStr := range typeStrings {
		typeStr = strings.TrimSpace(typeStr)
		if typeStr == "" {
			continue
		}

		typeInt, err := strconv.Atoi(typeStr)
		if err != nil {
			return nil, fmt.Errorf("invalid aircraft type: %s", typeStr)
		}

		if typeInt < 0 || typeInt > 7 {
			return nil, fmt.Errorf("aircraft type must be between 0 and 7, got: %d", typeInt)
		}

		types = append(types, models.PilotType(typeInt))
	}

	if len(types) == 0 {
		return nil, fmt.Errorf("no valid aircraft types specified")
	}

	return types, nil
}

func parseGroundTypes(typesStr string) ([]models.GroundType, error) {
	if typesStr == "" {
		return nil, nil
	}

	typeStrings := strings.Split(typesStr, ",")
	types := make([]models.GroundType, 0, len(typeStrings))

	validGroundTypes := map[int]bool{
		0: true, 1: true, 2: true, 3: true, 4: true,
		8: true, 9: true, 12: true, 13: true, 14: true, 15: true,
	}

	for _, typeStr := range typeStrings {
		typeStr = strings.TrimSpace(typeStr)
		if typeStr == "" {
			continue
		}

		typeInt, err := strconv.Atoi(typeStr)
		if err != nil {
			return nil, fmt.Errorf("invalid ground type: %s", typeStr)
		}

		if !validGroundTypes[typeInt] {
			return nil, fmt.Errorf("ground type must be one of [0,1,2,3,4,8,9,12,13,14,15], got: %d", typeInt)
		}

		types = append(types, models.GroundType(typeInt))
	}

	if len(types) == 0 {
		return nil, fmt.Errorf("no valid ground types specified")
	}

	return types, nil
}