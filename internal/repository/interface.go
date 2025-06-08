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
	DeletePilot(ctx context.Context, deviceID string) error

	// Операции с термиками
	SaveThermal(ctx context.Context, thermal *models.Thermal) error
	GetThermalsInRadius(ctx context.Context, center models.GeoPoint, radiusKM float64) ([]*models.Thermal, error)

	// Операции с метеостанциями
	SaveStation(ctx context.Context, station *models.Station) error
	GetStationsInRadius(ctx context.Context, center models.GeoPoint, radiusKM float64) ([]*models.Station, error)

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

	// Сохранение для backup
	SavePilotToHistory(ctx context.Context, pilot *models.Pilot) error

	// Обслуживание
	CleanupOldTracks(ctx context.Context, olderThan time.Duration) error
	GetStats(ctx context.Context) (map[string]interface{}, error)
}

// Ensure implementations
var _ Repository = (*RedisRepository)(nil)
var _ HistoryRepository = (*MySQLRepository)(nil)