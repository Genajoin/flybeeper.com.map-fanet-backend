package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/flybeeper/fanet-backend/internal/models"
	"github.com/flybeeper/fanet-backend/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// MockRepository для тестирования
type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) GetPilotsInRadius(ctx context.Context, center models.GeoPoint, radiusKm int, types []models.PilotType) ([]*models.Pilot, error) {
	args := m.Called(ctx, center, radiusKm, types)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Pilot), args.Error(1)
}

func (m *MockRepository) GetThermalsInRadius(ctx context.Context, center models.GeoPoint, radiusKm int) ([]*models.Thermal, error) {
	args := m.Called(ctx, center, radiusKm)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Thermal), args.Error(1)
}

func (m *MockRepository) GetStationsInRadius(ctx context.Context, center models.GeoPoint, radiusKm int) ([]*models.Station, error) {
	args := m.Called(ctx, center, radiusKm)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Station), args.Error(1)
}

func (m *MockRepository) GetGroundObjectsInRadius(ctx context.Context, center models.GeoPoint, radiusKm int, types []models.GroundType) ([]*models.GroundObject, error) {
	args := m.Called(ctx, center, radiusKm, types)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.GroundObject), args.Error(1)
}

func (m *MockRepository) StorePilot(ctx context.Context, pilot *models.Pilot) error {
	args := m.Called(ctx, pilot)
	return args.Error(0)
}

func (m *MockRepository) StoreThermal(ctx context.Context, thermal *models.Thermal) error {
	args := m.Called(ctx, thermal)
	return args.Error(0)
}

func (m *MockRepository) StoreStation(ctx context.Context, station *models.Station) error {
	args := m.Called(ctx, station)
	return args.Error(0)
}

func (m *MockRepository) StoreGroundObject(ctx context.Context, ground *models.GroundObject) error {
	args := m.Called(ctx, ground)
	return args.Error(0)
}

func (m *MockRepository) GetPilot(ctx context.Context, deviceID string) (*models.Pilot, error) {
	args := m.Called(ctx, deviceID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Pilot), args.Error(1)
}

func (m *MockRepository) DeletePilot(ctx context.Context, deviceID string) error {
	args := m.Called(ctx, deviceID)
	return args.Error(0)
}

func (m *MockRepository) DeleteThermal(ctx context.Context, deviceID string) error {
	args := m.Called(ctx, deviceID)
	return args.Error(0)
}

func (m *MockRepository) DeleteStation(ctx context.Context, deviceID string) error {
	args := m.Called(ctx, deviceID)
	return args.Error(0)
}

func (m *MockRepository) DeleteGroundObject(ctx context.Context, deviceID string) error {
	args := m.Called(ctx, deviceID)
	return args.Error(0)
}

func (m *MockRepository) GetStats(ctx context.Context) (map[string]interface{}, error) {
	args := m.Called(ctx)
	return args.Get(0).(map[string]interface{}), args.Error(1)
}

func (m *MockRepository) Cleanup(ctx context.Context, maxAge time.Duration) (int, error) {
	args := m.Called(ctx, maxAge)
	return args.Int(0), args.Error(1)
}

// MockHistoryRepository для тестирования
type MockHistoryRepository struct {
	mock.Mock
}

func (m *MockHistoryRepository) GetTrack(ctx context.Context, deviceID string, startTime, endTime time.Time) ([]*models.TrackPoint, error) {
	args := m.Called(ctx, deviceID, startTime, endTime)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.TrackPoint), args.Error(1)
}

func (m *MockHistoryRepository) StoreTrackPoint(ctx context.Context, point *models.TrackPoint) error {
	args := m.Called(ctx, point)
	return args.Error(0)
}

// Создаем тестовую Gin engine без middleware
func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

func TestRESTHandler_GetSnapshot_ValidParams(t *testing.T) {
	mockRepo := &MockRepository{}
	mockHistoryRepo := &MockHistoryRepository{}
	logger := utils.NewLogger("info", "text")
	handler := NewRESTHandler(mockRepo, mockHistoryRepo, logger)

	// Настраиваем моки
	center := models.GeoPoint{Latitude: 46.0, Longitude: 8.0}
	
	mockPilots := []*models.Pilot{
		{
			DeviceID: "ABC123",
			Type:     models.PilotTypeParaglider,
			Position: &center,
			Name:     "Test Pilot",
			LastUpdate: time.Now(),
		},
	}
	
	mockThermals := []*models.Thermal{
		{
			DeviceID: "THERM01",
			Position: &center,
			Quality:  3,
			LastUpdate: time.Now(),
		},
	}
	
	mockStations := []*models.Station{
		{
			DeviceID: "STAT01",
			Position: &center,
			LastUpdate: time.Now(),
		},
	}

	mockRepo.On("GetPilotsInRadius", mock.Anything, center, 50, mock.AnythingOfType("[]models.PilotType")).Return(mockPilots, nil)
	mockRepo.On("GetThermalsInRadius", mock.Anything, center, 50).Return(mockThermals, nil)
	mockRepo.On("GetStationsInRadius", mock.Anything, center, 50).Return(mockStations, nil)
	mockRepo.On("GetGroundObjectsInRadius", mock.Anything, center, 50, mock.AnythingOfType("[]models.GroundType")).Return([]*models.GroundObject{}, nil)

	router := setupTestRouter()
	router.GET("/api/v1/snapshot", handler.GetSnapshot)

	// Тестируем валидный запрос
	req := httptest.NewRequest("GET", "/api/v1/snapshot?lat=46.0&lon=8.0&radius=50", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Contains(t, response, "pilots")
	assert.Contains(t, response, "thermals")
	assert.Contains(t, response, "stations")
	assert.Contains(t, response, "ground")

	pilots := response["pilots"].([]interface{})
	assert.Len(t, pilots, 1)

	mockRepo.AssertExpectations(t)
}

func TestRESTHandler_GetSnapshot_InvalidParams(t *testing.T) {
	mockRepo := &MockRepository{}
	mockHistoryRepo := &MockHistoryRepository{}
	logger := utils.NewLogger("info", "text")
	handler := NewRESTHandler(mockRepo, mockHistoryRepo, logger)

	router := setupTestRouter()
	router.GET("/api/v1/snapshot", handler.GetSnapshot)

	tests := []struct {
		name           string
		queryParams    string
		expectedStatus int
		expectedCode   string
	}{
		{
			name:           "Missing latitude",
			queryParams:    "lon=8.0&radius=50",
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "invalid_latitude",
		},
		{
			name:           "Invalid latitude - too high",
			queryParams:    "lat=91.0&lon=8.0&radius=50",
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "invalid_latitude",
		},
		{
			name:           "Invalid latitude - too low",
			queryParams:    "lat=-91.0&lon=8.0&radius=50",
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "invalid_latitude",
		},
		{
			name:           "Missing longitude",
			queryParams:    "lat=46.0&radius=50",
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "invalid_longitude",
		},
		{
			name:           "Invalid longitude - too high",
			queryParams:    "lat=46.0&lon=181.0&radius=50",
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "invalid_longitude",
		},
		{
			name:           "Invalid longitude - too low",
			queryParams:    "lat=46.0&lon=-181.0&radius=50",
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "invalid_longitude",
		},
		{
			name:           "Missing radius",
			queryParams:    "lat=46.0&lon=8.0",
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "invalid_radius",
		},
		{
			name:           "Invalid radius - too small",
			queryParams:    "lat=46.0&lon=8.0&radius=0",
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "invalid_radius",
		},
		{
			name:           "Invalid radius - too large",
			queryParams:    "lat=46.0&lon=8.0&radius=201",
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "invalid_radius",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/snapshot?"+tt.queryParams, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedCode, response["code"])
			assert.Contains(t, response, "message")
		})
	}
}

func TestRESTHandler_GetSnapshot_WithTypeFilters(t *testing.T) {
	mockRepo := &MockRepository{}
	mockHistoryRepo := &MockHistoryRepository{}
	logger := utils.NewLogger("info", "text")
	handler := NewRESTHandler(mockRepo, mockHistoryRepo, logger)

	center := models.GeoPoint{Latitude: 46.0, Longitude: 8.0}

	// Настраиваем моки для фильтрации типов
	mockRepo.On("GetPilotsInRadius", mock.Anything, center, 50, []models.PilotType{models.PilotTypeParaglider, models.PilotTypeHangglider}).Return([]*models.Pilot{}, nil)
	mockRepo.On("GetThermalsInRadius", mock.Anything, center, 50).Return([]*models.Thermal{}, nil)
	mockRepo.On("GetStationsInRadius", mock.Anything, center, 50).Return([]*models.Station{}, nil)
	mockRepo.On("GetGroundObjectsInRadius", mock.Anything, center, 50, []models.GroundType{models.GroundTypeCar, models.GroundTypeTruck}).Return([]*models.GroundObject{}, nil)

	router := setupTestRouter()
	router.GET("/api/v1/snapshot", handler.GetSnapshot)

	// Тестируем с фильтрами типов
	req := httptest.NewRequest("GET", "/api/v1/snapshot?lat=46.0&lon=8.0&radius=50&air-types=1,2&ground-types=1,2", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockRepo.AssertExpectations(t)
}

func TestRESTHandler_GetSnapshot_InvalidTypeFilters(t *testing.T) {
	mockRepo := &MockRepository{}
	mockHistoryRepo := &MockHistoryRepository{}
	logger := utils.NewLogger("info", "text")
	handler := NewRESTHandler(mockRepo, mockHistoryRepo, logger)

	router := setupTestRouter()
	router.GET("/api/v1/snapshot", handler.GetSnapshot)

	tests := []struct {
		name           string
		queryParams    string
		expectedCode   string
	}{
		{
			name:         "Invalid air-types format",
			queryParams:  "lat=46.0&lon=8.0&radius=50&air-types=invalid",
			expectedCode: "invalid_air_types",
		},
		{
			name:         "Invalid ground-types format",
			queryParams:  "lat=46.0&lon=8.0&radius=50&ground-types=abc",
			expectedCode: "invalid_ground_types",
		},
		{
			name:         "Out of range air-types",
			queryParams:  "lat=46.0&lon=8.0&radius=50&air-types=99",
			expectedCode: "invalid_air_types",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/snapshot?"+tt.queryParams, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedCode, response["code"])
		})
	}
}

func TestRESTHandler_GetSnapshot_RepositoryError(t *testing.T) {
	mockRepo := &MockRepository{}
	mockHistoryRepo := &MockHistoryRepository{}
	logger := utils.NewLogger("info", "text")
	handler := NewRESTHandler(mockRepo, mockHistoryRepo, logger)

	center := models.GeoPoint{Latitude: 46.0, Longitude: 8.0}

	// Симулируем ошибку репозитория
	mockRepo.On("GetPilotsInRadius", mock.Anything, center, 50, mock.AnythingOfType("[]models.PilotType")).Return(nil, fmt.Errorf("database error"))

	router := setupTestRouter()
	router.GET("/api/v1/snapshot", handler.GetSnapshot)

	req := httptest.NewRequest("GET", "/api/v1/snapshot?lat=46.0&lon=8.0&radius=50", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "database_error", response["code"])

	mockRepo.AssertExpectations(t)
}

func TestRESTHandler_GetSnapshot_Protobuf(t *testing.T) {
	mockRepo := &MockRepository{}
	mockHistoryRepo := &MockHistoryRepository{}
	logger := utils.NewLogger("info", "text")
	handler := NewRESTHandler(mockRepo, mockHistoryRepo, logger)

	center := models.GeoPoint{Latitude: 46.0, Longitude: 8.0}

	mockPilots := []*models.Pilot{
		{
			DeviceID: "ABC123",
			Type:     models.PilotTypeParaglider,
			Position: &center,
			Name:     "Test Pilot",
			LastUpdate: time.Now(),
		},
	}

	mockRepo.On("GetPilotsInRadius", mock.Anything, center, 50, mock.AnythingOfType("[]models.PilotType")).Return(mockPilots, nil)
	mockRepo.On("GetThermalsInRadius", mock.Anything, center, 50).Return([]*models.Thermal{}, nil)
	mockRepo.On("GetStationsInRadius", mock.Anything, center, 50).Return([]*models.Station{}, nil)
	mockRepo.On("GetGroundObjectsInRadius", mock.Anything, center, 50, mock.AnythingOfType("[]models.GroundType")).Return([]*models.GroundObject{}, nil)

	router := setupTestRouter()
	router.GET("/api/v1/snapshot", handler.GetSnapshot)

	// Тестируем с Accept: application/x-protobuf
	req := httptest.NewRequest("GET", "/api/v1/snapshot?lat=46.0&lon=8.0&radius=50", nil)
	req.Header.Set("Accept", "application/x-protobuf")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/x-protobuf", w.Header().Get("Content-Type"))

	// Проверяем, что ответ можно декодировать как protobuf
	assert.Greater(t, len(w.Body.Bytes()), 0)

	mockRepo.AssertExpectations(t)
}

func TestRESTHandler_GetPilots(t *testing.T) {
	mockRepo := &MockRepository{}
	mockHistoryRepo := &MockHistoryRepository{}
	logger := utils.NewLogger("info", "text")
	handler := NewRESTHandler(mockRepo, mockHistoryRepo, logger)

	center := models.GeoPoint{Latitude: 46.0, Longitude: 8.0}

	mockPilots := []*models.Pilot{
		{
			DeviceID: "ABC123",
			Type:     models.PilotTypeParaglider,
			Position: &center,
			Name:     "Test Pilot",
			LastUpdate: time.Now(),
		},
	}

	mockRepo.On("GetPilotsInRadius", mock.Anything, center, 100, mock.AnythingOfType("[]models.PilotType")).Return(mockPilots, nil)

	router := setupTestRouter()
	router.GET("/api/v1/pilots", handler.GetPilots)

	req := httptest.NewRequest("GET", "/api/v1/pilots?lat=46.0&lon=8.0&radius=100", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var pilots []*models.Pilot
	err := json.Unmarshal(w.Body.Bytes(), &pilots)
	require.NoError(t, err)

	assert.Len(t, pilots, 1)
	assert.Equal(t, "ABC123", pilots[0].DeviceID)

	mockRepo.AssertExpectations(t)
}

func TestRESTHandler_GetThermals(t *testing.T) {
	mockRepo := &MockRepository{}
	mockHistoryRepo := &MockHistoryRepository{}
	logger := utils.NewLogger("info", "text")
	handler := NewRESTHandler(mockRepo, mockHistoryRepo, logger)

	center := models.GeoPoint{Latitude: 46.0, Longitude: 8.0}

	mockThermals := []*models.Thermal{
		{
			DeviceID: "THERM01",
			Position: &center,
			Quality:  4,
			LastUpdate: time.Now(),
		},
	}

	mockRepo.On("GetThermalsInRadius", mock.Anything, center, 75, mock.AnythingOfType("[]models.ThermalType")).Return(mockThermals, nil)

	router := setupTestRouter()
	router.GET("/api/v1/thermals", handler.GetThermals)

	req := httptest.NewRequest("GET", "/api/v1/thermals?lat=46.0&lon=8.0&radius=75", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var thermals []*models.Thermal
	err := json.Unmarshal(w.Body.Bytes(), &thermals)
	require.NoError(t, err)

	assert.Len(t, thermals, 1)
	assert.Equal(t, "THERM01", thermals[0].DeviceID)

	mockRepo.AssertExpectations(t)
}

func TestRESTHandler_GetTrack(t *testing.T) {
	mockRepo := &MockRepository{}
	mockHistoryRepo := &MockHistoryRepository{}
	logger := utils.NewLogger("info", "text")
	handler := NewRESTHandler(mockRepo, mockHistoryRepo, logger)

	deviceID := "ABC123"
	now := time.Now()
	startTime := now.Add(-1 * time.Hour)
	endTime := now

	mockTrackPoints := []*models.TrackPoint{
		{
			DeviceID:  deviceID,
			Position:  &models.GeoPoint{Latitude: 46.0, Longitude: 8.0},
			Altitude:  1000,
			Timestamp: startTime.Add(10 * time.Minute),
		},
		{
			DeviceID:  deviceID,
			Position:  &models.GeoPoint{Latitude: 46.01, Longitude: 8.01},
			Altitude:  1100,
			Timestamp: startTime.Add(20 * time.Minute),
		},
	}

	mockHistoryRepo.On("GetTrack", mock.Anything, deviceID, mock.AnythingOfType("time.Time"), mock.AnythingOfType("time.Time")).Return(mockTrackPoints, nil)

	router := setupTestRouter()
	router.GET("/api/v1/track/:device_id", handler.GetTrack)

	// Тестируем с временным диапазоном
	reqURL := fmt.Sprintf("/api/v1/track/%s?start=%d&end=%d", 
		deviceID, 
		startTime.Unix(), 
		endTime.Unix())
	req := httptest.NewRequest("GET", reqURL, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var track []*models.TrackPoint
	err := json.Unmarshal(w.Body.Bytes(), &track)
	require.NoError(t, err)

	assert.Len(t, track, 2)
	assert.Equal(t, deviceID, track[0].DeviceID)

	mockHistoryRepo.AssertExpectations(t)
}

func TestRESTHandler_PostPosition_RequiresAuth(t *testing.T) {
	mockRepo := &MockRepository{}
	mockHistoryRepo := &MockHistoryRepository{}
	logger := utils.NewLogger("info", "text")
	handler := NewRESTHandler(mockRepo, mockHistoryRepo, logger)

	router := setupTestRouter()
	router.POST("/api/v1/position", handler.PostPosition)

	// Тестируем без аутентификации
	body := strings.NewReader(`{"position":{"latitude":46.0,"longitude":8.0},"altitude":1000}`)
	req := httptest.NewRequest("POST", "/api/v1/position", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Ожидаем 401 без аутентификации
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRESTHandler_HealthCheck(t *testing.T) {
	mockRepo := &MockRepository{}
	mockHistoryRepo := &MockHistoryRepository{}
	logger := utils.NewLogger("info", "text")
	handler := NewRESTHandler(mockRepo, mockHistoryRepo, logger)

	router := setupTestRouter()
	router.GET("/health", handler.HealthCheck)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "healthy", response["status"])
	assert.Contains(t, response, "timestamp")
}

// Benchmark тесты
func BenchmarkRESTHandler_GetSnapshot(b *testing.B) {
	mockRepo := &MockRepository{}
	mockHistoryRepo := &MockHistoryRepository{}
	logger := utils.NewLogger("error", "text") // Минимальное логирование
	handler := NewRESTHandler(mockRepo, mockHistoryRepo, logger)

	center := models.GeoPoint{Latitude: 46.0, Longitude: 8.0}

	// Создаем большой набор тестовых данных
	pilots := make([]*models.Pilot, 1000)
	for i := 0; i < 1000; i++ {
		pilots[i] = &models.Pilot{
			DeviceID: fmt.Sprintf("PILOT%04d", i),
			Type:     models.PilotTypeParaglider,
			Position: &models.GeoPoint{
				Latitude:  46.0 + float64(i)*0.001,
				Longitude: 8.0 + float64(i)*0.001,
			},
			Name:       fmt.Sprintf("Pilot %d", i),
			LastUpdate: time.Now(),
		}
	}

	mockRepo.On("GetPilotsInRadius", mock.Anything, center, 50, mock.AnythingOfType("[]models.PilotType")).Return(pilots, nil)
	mockRepo.On("GetThermalsInRadius", mock.Anything, center, 50).Return([]*models.Thermal{}, nil)
	mockRepo.On("GetStationsInRadius", mock.Anything, center, 50).Return([]*models.Station{}, nil)
	mockRepo.On("GetGroundObjectsInRadius", mock.Anything, center, 50, mock.AnythingOfType("[]models.GroundType")).Return([]*models.GroundObject{}, nil)

	router := setupTestRouter()
	router.GET("/api/v1/snapshot", handler.GetSnapshot)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/api/v1/snapshot?lat=46.0&lon=8.0&radius=50", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		
		if w.Code != http.StatusOK {
			b.Fatalf("Expected status 200, got %d", w.Code)
		}
	}
}