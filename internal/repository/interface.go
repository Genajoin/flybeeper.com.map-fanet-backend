package repository

import (
	"context"
	"time"

	"github.com/flybeeper/fanet-backend/internal/models"
)

// Repository интерфейс для работы с данными
type Repository interface {
	// Проверка соединения
	Ping(ctx context.Context) error
	Close() error

	// Операции с пилотами
	SavePilot(ctx context.Context, pilot *models.Pilot) error
	GetPilotsInRadius(ctx context.Context, center models.GeoPoint, radiusKM float64) ([]*models.Pilot, error)
	UpdatePilotName(ctx context.Context, deviceID string, name string) error
	DeletePilot(ctx context.Context, deviceID string) error

	// Операции с термиками
	SaveThermal(ctx context.Context, thermal *models.Thermal) error
	GetThermalsInRadius(ctx context.Context, center models.GeoPoint, radiusKM float64) ([]*models.Thermal, error)

	// Операции с метеостанциями
	SaveStation(ctx context.Context, station *models.Station) error
	GetStationsInRadius(ctx context.Context, center models.GeoPoint, radiusKM float64) ([]*models.Station, error)
	GetAllStations(ctx context.Context) ([]*models.Station, error)

	// Операции с наземными объектами
	SaveGroundObject(ctx context.Context, groundObject *models.GroundObject) error
	GetGroundObjectsInRadius(ctx context.Context, center models.GeoPoint, radiusKM float64) ([]*models.GroundObject, error)
	DeleteGroundObject(ctx context.Context, deviceID string) error

	// Статистика
	GetStats(ctx context.Context) (map[string]interface{}, error)
}

// HistoryRepository интерфейс для работы с историческими данными
type HistoryRepository interface {
	// Проверка соединения
	Ping(ctx context.Context) error
	Close() error

	// Загрузка начальных данных
	LoadInitialPilots(ctx context.Context, limit int) ([]*models.Pilot, error)
	LoadInitialThermals(ctx context.Context, limit int) ([]*models.Thermal, error)
	LoadInitialStations(ctx context.Context, limit int) ([]*models.Station, error)

	// Операции с треками
	GetPilotTrack(ctx context.Context, deviceID string, limit int) ([]models.GeoPoint, error)
	GetPilotTrackWithTimestamps(ctx context.Context, deviceID string, limit int) ([]models.TrackGeoPoint, error)
	GetPilotAircraftType(ctx context.Context, deviceID string) (models.PilotType, error)

	// Сохранение для backup
	SavePilotToHistory(ctx context.Context, pilot *models.Pilot) error

	// Обслуживание
	CleanupOldTracks(ctx context.Context, olderThan time.Duration) error
	GetStats(ctx context.Context) (map[string]interface{}, error)
}

// MySQLRepositoryInterface интерфейс для MySQL репозитория с batch операциями
type MySQLRepositoryInterface interface {
	HistoryRepository

	// Batch операции для высокой производительности
	SavePilotsBatch(ctx context.Context, pilots []*models.Pilot) error
	SaveThermalsBatch(ctx context.Context, thermals []*models.Thermal) error
	SaveStationsBatch(ctx context.Context, stations []*models.Station) error
}

// Ensure implementations
var _ Repository = (*RedisRepository)(nil)
var _ HistoryRepository = (*MySQLRepository)(nil)
var _ MySQLRepositoryInterface = (*MySQLRepository)(nil)