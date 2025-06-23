package repository

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/flybeeper/fanet-backend/internal/config"
	"github.com/flybeeper/fanet-backend/internal/models"
	"github.com/flybeeper/fanet-backend/pkg/utils"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// RedisTestSuite представляет тестовый набор для Redis repository
type RedisTestSuite struct {
	suite.Suite
	repo   *RedisRepository
	client *redis.Client
	ctx    context.Context
}

// SetupSuite запускается один раз перед всеми тестами
func (suite *RedisTestSuite) SetupSuite() {
	suite.ctx = context.Background()
	
	// Используем Redis тестовую базу данных
	cfg := &config.RedisConfig{
		URL:          "redis://localhost:6379",
		Password:     "",
		DB:           15, // Используем DB 15 для тестов
		PoolSize:     10,
		MinIdleConns: 5,
	}

	logger := utils.NewLogger("info", "text")

	var err error
	suite.repo, err = NewRedisRepository(cfg, logger)
	require.NoError(suite.T(), err)

	suite.client = suite.repo.client

	// Проверяем подключение к Redis
	err = suite.client.Ping(suite.ctx).Err()
	if err != nil {
		suite.T().Skip("Redis not available for testing: " + err.Error())
	}
}

// SetupTest запускается перед каждым тестом
func (suite *RedisTestSuite) SetupTest() {
	// Очищаем тестовую базу данных
	err := suite.client.FlushDB(suite.ctx).Err()
	require.NoError(suite.T(), err)
}

// TearDownSuite запускается один раз после всех тестов
func (suite *RedisTestSuite) TearDownSuite() {
	if suite.client != nil {
		// Очищаем тестовую базу и закрываем соединение
		suite.client.FlushDB(suite.ctx)
		suite.client.Close()
	}
}

func (suite *RedisTestSuite) TestStorePilot() {
	pilot := &models.Pilot{
		DeviceID: "ABC123",
		Type:     models.PilotTypeParaglider,
		Position: &models.GeoPoint{
			Latitude:  46.0,
			Longitude: 8.0,
		},
		Name:       "Test Pilot",
		Altitude:   1000,
		Speed:      50,
		Heading:    180,
		ClimbRate:  2,
		LastUpdate: time.Now(),
	}

	err := suite.repo.StorePilot(suite.ctx, pilot)
	require.NoError(suite.T(), err)

	// Проверяем, что пилот сохранен в геоиндексе
	geoResult := suite.client.GeoPos(suite.ctx, PilotsGeoKey, pilot.DeviceID)
	require.NoError(suite.T(), geoResult.Err())
	positions := geoResult.Val()
	require.Len(suite.T(), positions, 1)
	assert.NotNil(suite.T(), positions[0])
	assert.InDelta(suite.T(), 8.0, positions[0].Longitude, 0.001)
	assert.InDelta(suite.T(), 46.0, positions[0].Latitude, 0.001)

	// Проверяем детальные данные в хеше
	hashKey := PilotPrefix + pilot.DeviceID
	hashResult := suite.client.HGetAll(suite.ctx, hashKey)
	require.NoError(suite.T(), hashResult.Err())
	
	pilotData := hashResult.Val()
	assert.Equal(suite.T(), pilot.DeviceID, pilotData["device_id"])
	assert.Equal(suite.T(), "Test Pilot", pilotData["name"])
	assert.Equal(suite.T(), "1000", pilotData["altitude"])

	// Проверяем TTL
	ttlResult := suite.client.TTL(suite.ctx, hashKey)
	require.NoError(suite.T(), ttlResult.Err())
	assert.Greater(suite.T(), ttlResult.Val(), 10*time.Hour) // Должно быть близко к PilotTTL
}

func (suite *RedisTestSuite) TestGetPilotsInRadius() {
	// Создаем несколько пилотов в разных локациях
	pilots := []*models.Pilot{
		{
			DeviceID: "PILOT1",
			Type:     models.PilotTypeParaglider,
			Position: &models.GeoPoint{Latitude: 46.0, Longitude: 8.0},
			Name:     "Pilot 1",
			LastUpdate: time.Now(),
		},
		{
			DeviceID: "PILOT2",
			Type:     models.PilotTypeHangglider,
			Position: &models.GeoPoint{Latitude: 46.1, Longitude: 8.1}, // ~15km от первого
			Name:     "Pilot 2",
			LastUpdate: time.Now(),
		},
		{
			DeviceID: "PILOT3",
			Type:     models.PilotTypeParaglider,
			Position: &models.GeoPoint{Latitude: 47.0, Longitude: 9.0}, // ~150km от первого
			Name:     "Pilot 3",
			LastUpdate: time.Now(),
		},
	}

	// Сохраняем всех пилотов
	for _, pilot := range pilots {
		err := suite.repo.StorePilot(suite.ctx, pilot)
		require.NoError(suite.T(), err)
	}

	// Тестируем поиск в радиусе 50км от точки (46.0, 8.0)
	center := models.GeoPoint{Latitude: 46.0, Longitude: 8.0}
	foundPilots, err := suite.repo.GetPilotsInRadius(suite.ctx, center, 50, nil)
	require.NoError(suite.T(), err)

	// Должны найти первых двух пилотов (в радиусе 50км)
	assert.Len(suite.T(), foundPilots, 2)
	
	foundIDs := make([]string, len(foundPilots))
	for i, p := range foundPilots {
		foundIDs[i] = p.DeviceID
	}
	assert.Contains(suite.T(), foundIDs, "PILOT1")
	assert.Contains(suite.T(), foundIDs, "PILOT2")
	assert.NotContains(suite.T(), foundIDs, "PILOT3")
}

func (suite *RedisTestSuite) TestGetPilotsInRadius_WithTypeFilter() {
	// Создаем пилотов разных типов
	pilots := []*models.Pilot{
		{
			DeviceID: "PARA1",
			Type:     models.PilotTypeParaglider,
			Position: &models.GeoPoint{Latitude: 46.0, Longitude: 8.0},
			Name:     "Paraglider 1",
			LastUpdate: time.Now(),
		},
		{
			DeviceID: "HANG1",
			Type:     models.PilotTypeHangglider,
			Position: &models.GeoPoint{Latitude: 46.01, Longitude: 8.01},
			Name:     "Hangglider 1",
			LastUpdate: time.Now(),
		},
		{
			DeviceID: "GLIDER1",
			Type:     models.PilotTypeGlider,
			Position: &models.GeoPoint{Latitude: 46.02, Longitude: 8.02},
			Name:     "Glider 1",
			LastUpdate: time.Now(),
		},
	}

	for _, pilot := range pilots {
		err := suite.repo.StorePilot(suite.ctx, pilot)
		require.NoError(suite.T(), err)
	}

	// Тестируем фильтр только по параглайдерам
	center := models.GeoPoint{Latitude: 46.0, Longitude: 8.0}
	types := []models.PilotType{models.PilotTypeParaglider}
	foundPilots, err := suite.repo.GetPilotsInRadius(suite.ctx, center, 10, types)
	require.NoError(suite.T(), err)

	assert.Len(suite.T(), foundPilots, 1)
	assert.Equal(suite.T(), "PARA1", foundPilots[0].DeviceID)
	assert.Equal(suite.T(), models.PilotTypeParaglider, foundPilots[0].Type)
}

func (suite *RedisTestSuite) TestStoreThermal() {
	thermal := &models.Thermal{
		DeviceID: "THERM123",
		Position: &models.GeoPoint{
			Latitude:  46.5,
			Longitude: 8.5,
		},
		Quality:    4,
		ClimbRate:  300, // 3.0 m/s
		Radius:     150,
		LastUpdate: time.Now(),
	}

	err := suite.repo.StoreThermal(suite.ctx, thermal)
	require.NoError(suite.T(), err)

	// Проверяем геоиндекс
	geoResult := suite.client.GeoPos(suite.ctx, ThermalsGeoKey, thermal.DeviceID)
	require.NoError(suite.T(), geoResult.Err())
	positions := geoResult.Val()
	require.Len(suite.T(), positions, 1)
	assert.InDelta(suite.T(), 8.5, positions[0].Longitude, 0.001)
	assert.InDelta(suite.T(), 46.5, positions[0].Latitude, 0.001)

	// Проверяем временной индекс
	timeScore := suite.client.ZScore(suite.ctx, ThermalsTimeKey, thermal.DeviceID)
	require.NoError(suite.T(), timeScore.Err())
	assert.Greater(suite.T(), timeScore.Val(), float64(0))

	// Проверяем детальные данные
	hashKey := ThermalPrefix + thermal.DeviceID
	hashResult := suite.client.HGetAll(suite.ctx, hashKey)
	require.NoError(suite.T(), hashResult.Err())
	
	thermalData := hashResult.Val()
	assert.Equal(suite.T(), thermal.DeviceID, thermalData["device_id"])
	assert.Equal(suite.T(), "4", thermalData["quality"])
	assert.Equal(suite.T(), "300", thermalData["climb_rate"])
}

func (suite *RedisTestSuite) TestGetThermalsInRadius() {
	thermals := []*models.Thermal{
		{
			DeviceID: "THERM1",
			Position: &models.GeoPoint{Latitude: 46.0, Longitude: 8.0},
			Quality:  3,
			LastUpdate: time.Now(),
		},
		{
			DeviceID: "THERM2",
			Position: &models.GeoPoint{Latitude: 46.01, Longitude: 8.01},
			Quality:  5,
			LastUpdate: time.Now(),
		},
		{
			DeviceID: "THERM3",
			Position: &models.GeoPoint{Latitude: 47.0, Longitude: 9.0}, // Далеко
			Quality:  2,
			LastUpdate: time.Now(),
		},
	}

	for _, thermal := range thermals {
		err := suite.repo.StoreThermal(suite.ctx, thermal)
		require.NoError(suite.T(), err)
	}

	// Поиск термиков в радиусе 10км
	center := models.GeoPoint{Latitude: 46.0, Longitude: 8.0}
	foundThermals, err := suite.repo.GetThermalsInRadius(suite.ctx, center, 10)
	require.NoError(suite.T(), err)

	assert.Len(suite.T(), foundThermals, 2)
	
	foundIDs := make([]string, len(foundThermals))
	for i, t := range foundThermals {
		foundIDs[i] = t.DeviceID
	}
	assert.Contains(suite.T(), foundIDs, "THERM1")
	assert.Contains(suite.T(), foundIDs, "THERM2")
	assert.NotContains(suite.T(), foundIDs, "THERM3")
}

func (suite *RedisTestSuite) TestStoreStation() {
	station := &models.Station{
		DeviceID: "STAT001",
		Position: &models.GeoPoint{
			Latitude:  47.0,
			Longitude: 7.0,
		},
		Name:        "Test Weather Station",
		Temperature: 22.5,
		Humidity:    65,
		Pressure:    1013.25,
		WindSpeed:   10.5,
		WindDirection: 270,
		LastUpdate:  time.Now(),
	}

	err := suite.repo.StoreStation(suite.ctx, station)
	require.NoError(suite.T(), err)

	// Проверяем геоиндекс
	geoResult := suite.client.GeoPos(suite.ctx, StationsGeoKey, station.DeviceID)
	require.NoError(suite.T(), geoResult.Err())
	positions := geoResult.Val()
	require.Len(suite.T(), positions, 1)
	assert.InDelta(suite.T(), 7.0, positions[0].Longitude, 0.001)
	assert.InDelta(suite.T(), 47.0, positions[0].Latitude, 0.001)

	// Проверяем детальные данные
	hashKey := StationPrefix + station.DeviceID
	hashResult := suite.client.HGetAll(suite.ctx, hashKey)
	require.NoError(suite.T(), hashResult.Err())
	
	stationData := hashResult.Val()
	assert.Equal(suite.T(), station.DeviceID, stationData["device_id"])
	assert.Equal(suite.T(), "Test Weather Station", stationData["name"])
	assert.Equal(suite.T(), "22.5", stationData["temperature"])
	assert.Equal(suite.T(), "65", stationData["humidity"])
}

func (suite *RedisTestSuite) TestGetStationsInRadius() {
	stations := []*models.Station{
		{
			DeviceID: "STATION1",
			Position: &models.GeoPoint{Latitude: 46.0, Longitude: 8.0},
			Name:     "Station 1",
			LastUpdate: time.Now(),
		},
		{
			DeviceID: "STATION2",
			Position: &models.GeoPoint{Latitude: 46.05, Longitude: 8.05},
			Name:     "Station 2",
			LastUpdate: time.Now(),
		},
	}

	for _, station := range stations {
		err := suite.repo.StoreStation(suite.ctx, station)
		require.NoError(suite.T(), err)
	}

	// Поиск станций в радиусе 20км
	center := models.GeoPoint{Latitude: 46.0, Longitude: 8.0}
	foundStations, err := suite.repo.GetStationsInRadius(suite.ctx, center, 20)
	require.NoError(suite.T(), err)

	assert.Len(suite.T(), foundStations, 2)
	
	foundIDs := make([]string, len(foundStations))
	for i, s := range foundStations {
		foundIDs[i] = s.DeviceID
	}
	assert.Contains(suite.T(), foundIDs, "STATION1")
	assert.Contains(suite.T(), foundIDs, "STATION2")
}

func (suite *RedisTestSuite) TestDeletePilot() {
	pilot := &models.Pilot{
		DeviceID: "DELETE_TEST",
		Type:     models.PilotTypeParaglider,
		Position: &models.GeoPoint{Latitude: 46.0, Longitude: 8.0},
		Name:     "Delete Test",
		LastUpdate: time.Now(),
	}

	// Сначала сохраняем пилота
	err := suite.repo.StorePilot(suite.ctx, pilot)
	require.NoError(suite.T(), err)

	// Проверяем, что пилот существует
	geoResult := suite.client.GeoPos(suite.ctx, PilotsGeoKey, pilot.DeviceID)
	require.NoError(suite.T(), geoResult.Err())
	assert.Len(suite.T(), geoResult.Val(), 1)

	// Удаляем пилота
	err = suite.repo.DeletePilot(suite.ctx, pilot.DeviceID)
	require.NoError(suite.T(), err)

	// Проверяем, что пилот удален из геоиндекса
	geoResult = suite.client.GeoPos(suite.ctx, PilotsGeoKey, pilot.DeviceID)
	require.NoError(suite.T(), geoResult.Err())
	positions := geoResult.Val()
	assert.Len(suite.T(), positions, 1)
	assert.Nil(suite.T(), positions[0]) // Nil означает, что элемент не найден

	// Проверяем, что хеш удален
	hashKey := PilotPrefix + pilot.DeviceID
	exists := suite.client.Exists(suite.ctx, hashKey)
	require.NoError(suite.T(), exists.Err())
	assert.Equal(suite.T(), int64(0), exists.Val())
}

func (suite *RedisTestSuite) TestCleanup() {
	now := time.Now()
	
	// Создаем старых и новых пилотов
	oldPilot := &models.Pilot{
		DeviceID: "OLD_PILOT",
		Type:     models.PilotTypeParaglider,
		Position: &models.GeoPoint{Latitude: 46.0, Longitude: 8.0},
		Name:     "Old Pilot",
		LastUpdate: now.Add(-25 * time.Hour), // Старше 24 часов
	}

	newPilot := &models.Pilot{
		DeviceID: "NEW_PILOT",
		Type:     models.PilotTypeParaglider,
		Position: &models.GeoPoint{Latitude: 46.0, Longitude: 8.0},
		Name:     "New Pilot",
		LastUpdate: now.Add(-1 * time.Hour), // Свежий
	}

	// Сохраняем пилотов
	err := suite.repo.StorePilot(suite.ctx, oldPilot)
	require.NoError(suite.T(), err)
	err = suite.repo.StorePilot(suite.ctx, newPilot)
	require.NoError(suite.T(), err)

	// Вручную устанавливаем старое время для тестирования
	hashKey := PilotPrefix + oldPilot.DeviceID
	suite.client.HSet(suite.ctx, hashKey, "last_update", oldPilot.LastUpdate.Unix())

	// Выполняем очистку данных старше 24 часов
	removedCount, err := suite.repo.Cleanup(suite.ctx, 24*time.Hour)
	require.NoError(suite.T(), err)

	// Должен быть удален 1 старый пилот
	assert.Equal(suite.T(), 1, removedCount)

	// Проверяем, что старый пилот удален, а новый остался
	center := models.GeoPoint{Latitude: 46.0, Longitude: 8.0}
	pilots, err := suite.repo.GetPilotsInRadius(suite.ctx, center, 10, nil)
	require.NoError(suite.T(), err)

	assert.Len(suite.T(), pilots, 1)
	assert.Equal(suite.T(), "NEW_PILOT", pilots[0].DeviceID)
}

func (suite *RedisTestSuite) TestGetStats() {
	// Добавляем несколько объектов для статистики
	pilot := &models.Pilot{
		DeviceID: "STAT_PILOT",
		Type:     models.PilotTypeParaglider,
		Position: &models.GeoPoint{Latitude: 46.0, Longitude: 8.0},
		LastUpdate: time.Now(),
	}

	thermal := &models.Thermal{
		DeviceID: "STAT_THERMAL",
		Position: &models.GeoPoint{Latitude: 46.0, Longitude: 8.0},
		Quality:  3,
		LastUpdate: time.Now(),
	}

	station := &models.Station{
		DeviceID: "STAT_STATION",
		Position: &models.GeoPoint{Latitude: 46.0, Longitude: 8.0},
		LastUpdate: time.Now(),
	}

	err := suite.repo.StorePilot(suite.ctx, pilot)
	require.NoError(suite.T(), err)
	err = suite.repo.StoreThermal(suite.ctx, thermal)
	require.NoError(suite.T(), err)
	err = suite.repo.StoreStation(suite.ctx, station)
	require.NoError(suite.T(), err)

	// Получаем статистику
	stats, err := suite.repo.GetStats(suite.ctx)
	require.NoError(suite.T(), err)

	// Проверяем основные счетчики
	assert.Contains(suite.T(), stats, "pilots_count")
	assert.Contains(suite.T(), stats, "thermals_count")
	assert.Contains(suite.T(), stats, "stations_count")

	assert.Equal(suite.T(), int64(1), stats["pilots_count"])
	assert.Equal(suite.T(), int64(1), stats["thermals_count"])
	assert.Equal(suite.T(), int64(1), stats["stations_count"])
}

func (suite *RedisTestSuite) TestRepositoryConfiguration() {
	// Тестируем создание репозитория с неправильной конфигурацией
	_, err := NewRedisRepository(nil, utils.NewLogger("info", "text"))
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "redis config cannot be nil")

	// Тестируем с неправильным logger
	cfg := &config.RedisConfig{URL: "redis://localhost:6379"}
	_, err = NewRedisRepository(cfg, nil)
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "logger cannot be nil")

	// Тестируем с неправильным URL
	invalidCfg := &config.RedisConfig{URL: "invalid-url"}
	_, err = NewRedisRepository(invalidCfg, utils.NewLogger("info", "text"))
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "failed to parse Redis URL")
}

// Benchmark тесты для производительности
func (suite *RedisTestSuite) TestBenchmarkStorePilot() {
	pilot := &models.Pilot{
		DeviceID: "BENCH_PILOT",
		Type:     models.PilotTypeParaglider,
		Position: &models.GeoPoint{Latitude: 46.0, Longitude: 8.0},
		Name:     "Benchmark Pilot",
		LastUpdate: time.Now(),
	}

	// Простое измерение времени выполнения
	start := time.Now()
	for i := 0; i < 100; i++ {
		pilot.DeviceID = fmt.Sprintf("BENCH_PILOT_%d", i)
		err := suite.repo.StorePilot(suite.ctx, pilot)
		require.NoError(suite.T(), err)
	}
	duration := time.Since(start)

	suite.T().Logf("100 StorePilot operations took %v (avg: %v per operation)", 
		duration, duration/100)

	// Проверяем, что все пилоты сохранены
	center := models.GeoPoint{Latitude: 46.0, Longitude: 8.0}
	pilots, err := suite.repo.GetPilotsInRadius(suite.ctx, center, 10, nil)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 100, len(pilots))
}

func (suite *RedisTestSuite) TestConcurrentOperations() {
	// Тестируем конкурентные операции
	const numWorkers = 10
	const operationsPerWorker = 10

	done := make(chan error, numWorkers)

	// Запускаем несколько горутин, выполняющих операции параллельно
	for i := 0; i < numWorkers; i++ {
		go func(workerID int) {
			for j := 0; j < operationsPerWorker; j++ {
				pilot := &models.Pilot{
					DeviceID: fmt.Sprintf("WORKER_%d_PILOT_%d", workerID, j),
					Type:     models.PilotTypeParaglider,
					Position: &models.GeoPoint{
						Latitude:  46.0 + float64(workerID)*0.01,
						Longitude: 8.0 + float64(j)*0.01,
					},
					Name:       fmt.Sprintf("Worker %d Pilot %d", workerID, j),
					LastUpdate: time.Now(),
				}

				if err := suite.repo.StorePilot(suite.ctx, pilot); err != nil {
					done <- err
					return
				}
			}
			done <- nil
		}(i)
	}

	// Ждем завершения всех горутин
	for i := 0; i < numWorkers; i++ {
		err := <-done
		require.NoError(suite.T(), err)
	}

	// Проверяем, что все данные сохранены корректно
	center := models.GeoPoint{Latitude: 46.05, Longitude: 8.05}
	pilots, err := suite.repo.GetPilotsInRadius(suite.ctx, center, 50, nil)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), numWorkers*operationsPerWorker, len(pilots))
}

// Запускаем тестовый набор
func TestRedisRepositorySuite(t *testing.T) {
	suite.Run(t, new(RedisTestSuite))
}

// Дополнительные unit тесты, не требующие Redis подключения
func TestRedisConstants(t *testing.T) {
	// Тестируем константы и TTL значения
	assert.Equal(t, "pilots:geo", PilotsGeoKey)
	assert.Equal(t, "thermals:geo", ThermalsGeoKey)
	assert.Equal(t, "stations:geo", StationsGeoKey)

	assert.Equal(t, 12*time.Hour, PilotTTL)
	assert.Equal(t, 6*time.Hour, ThermalTTL)
	assert.Equal(t, 24*time.Hour, StationTTL)

	assert.Equal(t, 999, MaxTrackPoints)
	assert.Equal(t, 287, MaxStationHistory)
}