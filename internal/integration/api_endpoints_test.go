package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/flybeeper/fanet-backend/internal/config"
	"github.com/flybeeper/fanet-backend/internal/handler"
	"github.com/flybeeper/fanet-backend/internal/models"
	"github.com/flybeeper/fanet-backend/internal/repository"
	"github.com/flybeeper/fanet-backend/pkg/utils"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// APIEndpointsTestSuite тестирует полные API endpoints с реальными зависимостями
type APIEndpointsTestSuite struct {
	suite.Suite
	router      *gin.Engine
	redisRepo   *repository.RedisRepository
	redisClient *redis.Client
	restHandler *handler.RESTHandler
	ctx         context.Context
}

func (suite *APIEndpointsTestSuite) SetupSuite() {
	suite.ctx = context.Background()
	gin.SetMode(gin.TestMode)

	// Настройка Redis
	redisConfig := &config.RedisConfig{
		URL:          "redis://localhost:6379",
		Password:     "",
		DB:           13, // Отдельная DB для API интеграционных тестов
		PoolSize:     10,
		MinIdleConns: 5,
	}

	logger := utils.NewLogger("info", "text")

	var err error
	suite.redisRepo, err = repository.NewRedisRepository(redisConfig, logger)
	require.NoError(suite.T(), err)

	suite.redisClient = suite.redisRepo.GetClient()

	// Проверяем подключение к Redis
	err = suite.redisClient.Ping(suite.ctx).Err()
	if err != nil {
		suite.T().Skip("Redis not available for integration testing: " + err.Error())
	}

	// Создаем mock history repository
	mockHistoryRepo := &MockHistoryRepository{}

	// Создаем REST handler
	suite.restHandler = handler.NewRESTHandler(suite.redisRepo, mockHistoryRepo, logger)

	// Настраиваем Gin router
	suite.router = gin.New()
	suite.router.Use(gin.Recovery())

	// Регистрируем маршруты
	api := suite.router.Group("/api/v1")
	{
		api.GET("/snapshot", suite.restHandler.GetSnapshot)
		api.GET("/pilots", suite.restHandler.GetPilots)
		api.GET("/thermals", suite.restHandler.GetThermals)
		api.GET("/stations", suite.restHandler.GetStations)
		api.GET("/track/:device_id", suite.restHandler.GetTrack)
	}

	suite.router.GET("/health", suite.restHandler.HealthCheck)
}

// MockHistoryRepository для интеграционных тестов
type MockHistoryRepository struct{}

func (m *MockHistoryRepository) GetTrack(ctx context.Context, deviceID string, startTime, endTime time.Time) ([]*models.TrackPoint, error) {
	// Возвращаем тестовый трек
	return []*models.TrackPoint{
		{
			DeviceID:  deviceID,
			Position:  &models.GeoPoint{Latitude: 46.0, Longitude: 8.0},
			Altitude:  1000,
			Timestamp: startTime.Add(5 * time.Minute),
		},
		{
			DeviceID:  deviceID,
			Position:  &models.GeoPoint{Latitude: 46.01, Longitude: 8.01},
			Altitude:  1100,
			Timestamp: startTime.Add(10 * time.Minute),
		},
	}, nil
}

func (m *MockHistoryRepository) StoreTrackPoint(ctx context.Context, point *models.TrackPoint) error {
	return nil
}

func (suite *APIEndpointsTestSuite) SetupTest() {
	// Очищаем Redis перед каждым тестом
	err := suite.redisClient.FlushDB(suite.ctx).Err()
	require.NoError(suite.T(), err)
}

func (suite *APIEndpointsTestSuite) TearDownSuite() {
	if suite.redisClient != nil {
		suite.redisClient.FlushDB(suite.ctx)
		suite.redisClient.Close()
	}
}

func (suite *APIEndpointsTestSuite) TestHealthCheckEndpoint() {
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(suite.T(), err)

	assert.Equal(suite.T(), "healthy", response["status"])
	assert.Contains(suite.T(), response, "timestamp")
}

func (suite *APIEndpointsTestSuite) TestSnapshotEndpoint_EmptyData() {
	req := httptest.NewRequest("GET", "/api/v1/snapshot?lat=46.0&lon=8.0&radius=50", nil)
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(suite.T(), err)

	assert.Contains(suite.T(), response, "pilots")
	assert.Contains(suite.T(), response, "thermals")
	assert.Contains(suite.T(), response, "stations")
	assert.Contains(suite.T(), response, "ground")

	// Проверяем, что все списки пустые
	pilots := response["pilots"].([]interface{})
	thermals := response["thermals"].([]interface{})
	stations := response["stations"].([]interface{})
	ground := response["ground"].([]interface{})

	assert.Empty(suite.T(), pilots)
	assert.Empty(suite.T(), thermals)
	assert.Empty(suite.T(), stations)
	assert.Empty(suite.T(), ground)
}

func (suite *APIEndpointsTestSuite) TestSnapshotEndpoint_WithData() {
	// Добавляем тестовые данные в Redis
	center := models.GeoPoint{Latitude: 46.0, Longitude: 8.0}

	// Добавляем пилота
	pilot := &models.Pilot{
		DeviceID: "TEST001",
		Type:     models.PilotTypeParaglider,
		Position: &center,
		Name:     "Integration Test Pilot",
		Altitude: 1500,
		Speed:    45,
		Heading:  180,
		LastUpdate: time.Now(),
	}
	err := suite.redisRepo.StorePilot(suite.ctx, pilot)
	require.NoError(suite.T(), err)

	// Добавляем термик
	thermal := &models.Thermal{
		DeviceID: "THERM001",
		Position: &models.GeoPoint{Latitude: 46.01, Longitude: 8.01},
		Quality:  4,
		ClimbRate: 350,
		Radius:   150,
		LastUpdate: time.Now(),
	}
	err = suite.redisRepo.StoreThermal(suite.ctx, thermal)
	require.NoError(suite.T(), err)

	// Добавляем станцию
	station := &models.Station{
		DeviceID: "STAT001",
		Position: &models.GeoPoint{Latitude: 46.02, Longitude: 8.02},
		Name:     "Test Weather Station",
		Temperature: 22.5,
		Humidity:    65,
		LastUpdate:  time.Now(),
	}
	err = suite.redisRepo.StoreStation(suite.ctx, station)
	require.NoError(suite.T(), err)

	// Выполняем запрос
	req := httptest.NewRequest("GET", "/api/v1/snapshot?lat=46.0&lon=8.0&radius=10", nil)
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(suite.T(), err)

	// Проверяем данные пилота
	pilots := response["pilots"].([]interface{})
	assert.Len(suite.T(), pilots, 1)
	
	pilotData := pilots[0].(map[string]interface{})
	assert.Equal(suite.T(), "TEST001", pilotData["device_id"])
	assert.Equal(suite.T(), "Integration Test Pilot", pilotData["name"])
	assert.Equal(suite.T(), float64(1500), pilotData["altitude"])

	// Проверяем данные термика
	thermals := response["thermals"].([]interface{})
	assert.Len(suite.T(), thermals, 1)
	
	thermalData := thermals[0].(map[string]interface{})
	assert.Equal(suite.T(), "THERM001", thermalData["device_id"])
	assert.Equal(suite.T(), float64(4), thermalData["quality"])

	// Проверяем данные станции
	stations := response["stations"].([]interface{})
	assert.Len(suite.T(), stations, 1)
	
	stationData := stations[0].(map[string]interface{})
	assert.Equal(suite.T(), "STAT001", stationData["device_id"])
	assert.Equal(suite.T(), "Test Weather Station", stationData["name"])
}

func (suite *APIEndpointsTestSuite) TestPilotsEndpoint() {
	// Добавляем несколько пилотов
	pilots := []*models.Pilot{
		{
			DeviceID: "PILOT001",
			Type:     models.PilotTypeParaglider,
			Position: &models.GeoPoint{Latitude: 46.0, Longitude: 8.0},
			Name:     "Pilot 1",
			LastUpdate: time.Now(),
		},
		{
			DeviceID: "PILOT002",
			Type:     models.PilotTypeHangglider,
			Position: &models.GeoPoint{Latitude: 46.01, Longitude: 8.01},
			Name:     "Pilot 2",
			LastUpdate: time.Now(),
		},
		{
			DeviceID: "PILOT003",
			Type:     models.PilotTypeGlider,
			Position: &models.GeoPoint{Latitude: 47.0, Longitude: 9.0}, // Далеко
			Name:     "Pilot 3",
			LastUpdate: time.Now(),
		},
	}

	for _, pilot := range pilots {
		err := suite.redisRepo.StorePilot(suite.ctx, pilot)
		require.NoError(suite.T(), err)
	}

	// Запрашиваем пилотов в радиусе 20км
	req := httptest.NewRequest("GET", "/api/v1/pilots?lat=46.0&lon=8.0&radius=20", nil)
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)

	var response []*models.Pilot
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(suite.T(), err)

	// Должны получить первых двух пилотов (в радиусе 20км)
	assert.Len(suite.T(), response, 2)
	
	deviceIDs := make([]string, len(response))
	for i, p := range response {
		deviceIDs[i] = p.DeviceID
	}
	assert.Contains(suite.T(), deviceIDs, "PILOT001")
	assert.Contains(suite.T(), deviceIDs, "PILOT002")
	assert.NotContains(suite.T(), deviceIDs, "PILOT003")
}

func (suite *APIEndpointsTestSuite) TestPilotsEndpoint_WithTypeFilter() {
	// Добавляем пилотов разных типов
	pilots := []*models.Pilot{
		{
			DeviceID: "PARA001",
			Type:     models.PilotTypeParaglider,
			Position: &models.GeoPoint{Latitude: 46.0, Longitude: 8.0},
			Name:     "Paraglider 1",
			LastUpdate: time.Now(),
		},
		{
			DeviceID: "HANG001",
			Type:     models.PilotTypeHangglider,
			Position: &models.GeoPoint{Latitude: 46.01, Longitude: 8.01},
			Name:     "Hangglider 1",
			LastUpdate: time.Now(),
		},
		{
			DeviceID: "GLIDER001",
			Type:     models.PilotTypeGlider,
			Position: &models.GeoPoint{Latitude: 46.02, Longitude: 8.02},
			Name:     "Glider 1",
			LastUpdate: time.Now(),
		},
	}

	for _, pilot := range pilots {
		err := suite.redisRepo.StorePilot(suite.ctx, pilot)
		require.NoError(suite.T(), err)
	}

	// Запрашиваем только параглайдеры
	req := httptest.NewRequest("GET", "/api/v1/pilots?lat=46.0&lon=8.0&radius=10&types=1", nil)
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)

	var response []*models.Pilot
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(suite.T(), err)

	// Должны получить только параглайдер
	assert.Len(suite.T(), response, 1)
	assert.Equal(suite.T(), "PARA001", response[0].DeviceID)
	assert.Equal(suite.T(), models.PilotTypeParaglider, response[0].Type)
}

func (suite *APIEndpointsTestSuite) TestThermalsEndpoint() {
	// Добавляем термики
	thermals := []*models.Thermal{
		{
			DeviceID: "THERM001",
			Position: &models.GeoPoint{Latitude: 46.0, Longitude: 8.0},
			Quality:  3,
			ClimbRate: 250,
			LastUpdate: time.Now(),
		},
		{
			DeviceID: "THERM002",
			Position: &models.GeoPoint{Latitude: 46.01, Longitude: 8.01},
			Quality:  5,
			ClimbRate: 400,
			LastUpdate: time.Now(),
		},
	}

	for _, thermal := range thermals {
		err := suite.redisRepo.StoreThermal(suite.ctx, thermal)
		require.NoError(suite.T(), err)
	}

	req := httptest.NewRequest("GET", "/api/v1/thermals?lat=46.0&lon=8.0&radius=5", nil)
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)

	var response []*models.Thermal
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(suite.T(), err)

	assert.Len(suite.T(), response, 2)
	
	deviceIDs := make([]string, len(response))
	for i, t := range response {
		deviceIDs[i] = t.DeviceID
	}
	assert.Contains(suite.T(), deviceIDs, "THERM001")
	assert.Contains(suite.T(), deviceIDs, "THERM002")
}

func (suite *APIEndpointsTestSuite) TestTrackEndpoint() {
	deviceID := "TRACK001"
	now := time.Now()
	startTime := now.Add(-1 * time.Hour)
	endTime := now

	reqURL := fmt.Sprintf("/api/v1/track/%s?start=%d&end=%d", 
		deviceID, 
		startTime.Unix(), 
		endTime.Unix())
	
	req := httptest.NewRequest("GET", reqURL, nil)
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)

	var response []*models.TrackPoint
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(suite.T(), err)

	// MockHistoryRepository возвращает 2 точки
	assert.Len(suite.T(), response, 2)
	assert.Equal(suite.T(), deviceID, response[0].DeviceID)
}

func (suite *APIEndpointsTestSuite) TestAPIValidation() {
	// Тестируем различные сценарии валидации
	tests := []struct {
		name           string
		endpoint       string
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "Invalid latitude",
			endpoint:       "/api/v1/snapshot?lat=91&lon=8&radius=50",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "invalid_latitude",
		},
		{
			name:           "Invalid longitude",
			endpoint:       "/api/v1/snapshot?lat=46&lon=181&radius=50",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "invalid_longitude",
		},
		{
			name:           "Invalid radius",
			endpoint:       "/api/v1/snapshot?lat=46&lon=8&radius=201",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "invalid_radius",
		},
		{
			name:           "Missing parameters",
			endpoint:       "/api/v1/snapshot",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "invalid_latitude",
		},
		{
			name:           "Valid parameters",
			endpoint:       "/api/v1/snapshot?lat=46&lon=8&radius=50",
			expectedStatus: http.StatusOK,
			expectedError:  "",
		},
	}

	for _, tt := range tests {
		suite.T().Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.endpoint, nil)
			w := httptest.NewRecorder()
			suite.router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedError != "" {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.Equal(t, tt.expectedError, response["code"])
			}
		})
	}
}

func (suite *APIEndpointsTestSuite) TestAPIPerformance() {
	// Добавляем много данных для тестирования производительности
	numPilots := 1000
	for i := 0; i < numPilots; i++ {
		pilot := &models.Pilot{
			DeviceID: fmt.Sprintf("PERF%04d", i),
			Type:     models.PilotTypeParaglider,
			Position: &models.GeoPoint{
				Latitude:  46.0 + float64(i)*0.001,
				Longitude: 8.0 + float64(i)*0.001,
			},
			Name:       fmt.Sprintf("Performance Pilot %d", i),
			LastUpdate: time.Now(),
		}
		err := suite.redisRepo.StorePilot(suite.ctx, pilot)
		require.NoError(suite.T(), err)
	}

	// Измеряем время ответа
	start := time.Now()
	req := httptest.NewRequest("GET", "/api/v1/snapshot?lat=46.0&lon=8.0&radius=100", nil)
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)
	duration := time.Since(start)

	assert.Equal(suite.T(), http.StatusOK, w.Code)
	assert.Less(suite.T(), duration, 1*time.Second, "API should respond within 1 second even with 1000 pilots")

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(suite.T(), err)

	pilots := response["pilots"].([]interface{})
	assert.Greater(suite.T(), len(pilots), 500, "Should return substantial number of pilots")

	suite.T().Logf("API responded in %v with %d pilots", duration, len(pilots))
}

// Запуск интеграционных тестов для API
func TestAPIEndpointsSuite(t *testing.T) {
	suite.Run(t, new(APIEndpointsTestSuite))
}