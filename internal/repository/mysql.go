package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/flybeeper/fanet-backend/internal/config"
	"github.com/flybeeper/fanet-backend/internal/models"
	"github.com/flybeeper/fanet-backend/pkg/utils"
)

// MySQLRepository репозиторий для работы с MySQL (fallback и исторические данные)
type MySQLRepository struct {
	db     *sql.DB
	logger *utils.Logger
	config *config.MySQLConfig
}

// NewMySQLRepository создает новый MySQL репозиторий
func NewMySQLRepository(cfg *config.MySQLConfig, logger *utils.Logger) (*MySQLRepository, error) {
	if cfg == nil {
		return nil, fmt.Errorf("mysql config cannot be nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}
	if cfg.DSN == "" {
		return nil, fmt.Errorf("mysql DSN is required")
	}

	db, err := sql.Open("mysql", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("failed to open MySQL connection: %w", err)
	}

	// Настройки connection pool
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetConnMaxLifetime(1 * time.Hour)

	repo := &MySQLRepository{
		db:     db,
		logger: logger,
		config: cfg,
	}

	return repo, nil
}

// Ping проверяет соединение с MySQL
func (r *MySQLRepository) Ping(ctx context.Context) error {
	return r.db.PingContext(ctx)
}

// Close закрывает соединение с MySQL
func (r *MySQLRepository) Close() error {
	return r.db.Close()
}

// LoadInitialPilots загружает начальные данные пилотов из MySQL
func (r *MySQLRepository) LoadInitialPilots(ctx context.Context, limit int) ([]*models.Pilot, error) {
	query := `
		SELECT 
			t.addr,
			COALESCE(n.name, '') as name,
			t.ufo_type,
			t.latitude,
			t.longitude,
			COALESCE(t.altitude_gps, 0) as altitude,
			COALESCE(t.speed, 0) as speed,
			COALESCE(t.climb, 0) as climb,
			COALESCE(t.course, 0) as course,
			COALESCE(t.track_online, 0) as track_online,
			t.datestamp
		FROM ufo_track t
		INNER JOIN ufo u ON t.addr = u.addr AND t.id = u.last_position
		LEFT JOIN name n ON t.addr = n.addr
		WHERE t.datestamp > DATE_SUB(NOW(), INTERVAL 12 HOUR)
		ORDER BY t.datestamp DESC
		LIMIT ?
	`

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query initial pilots: %w", err)
	}
	defer rows.Close()

	var pilots []*models.Pilot
	for rows.Next() {
		var (
			addr        int
			name        string
			aircraftType int
			lat, lon    float64
			altitude    int
			speed       int
			climb       int
			course      int
			trackOnline int
			timestamp   time.Time
		)

		err := rows.Scan(
			&addr, &name, &aircraftType, &lat, &lon, &altitude,
			&speed, &climb, &course, &trackOnline, &timestamp,
		)
		if err != nil {
			r.logger.WithField("error", err).Warn("Failed to scan pilot row")
			continue
		}

		pilot := &models.Pilot{
			DeviceID:     fmt.Sprintf("%06X", addr),
			Name:         name,
			AircraftType: uint8(aircraftType),
			Position: models.GeoPoint{
				Latitude:  lat,
				Longitude: lon,
				Altitude:  int16(altitude),
			},
			Speed:       uint16(speed),
			ClimbRate:   int16(climb),
			Heading:     uint16(course),
			TrackOnline: trackOnline == 1,
			LastUpdate:  timestamp,
			Battery:     100, // Неизвестно в legacy схеме
		}

		pilots = append(pilots, pilot)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating pilot rows: %w", err)
	}

	r.logger.WithField("count", len(pilots)).Info("Loaded initial pilots from MySQL")
	return pilots, nil
}

// LoadInitialThermals загружает начальные данные термиков
func (r *MySQLRepository) LoadInitialThermals(ctx context.Context, limit int) ([]*models.Thermal, error) {
	query := `
		SELECT 
			id,
			addr,
			latitude,
			longitude,
			COALESCE(altitude, 0) as altitude,
			COALESCE(quality, 0) as quality,
			COALESCE(climb, 0) as climb,
			COALESCE(wind_speed, 0) as wind_speed,
			COALESCE(wind_heading, 0) as wind_heading
		FROM thermal
		ORDER BY id DESC
		LIMIT ?
	`

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query initial thermals: %w", err)
	}
	defer rows.Close()

	var thermals []*models.Thermal
	for rows.Next() {
		var (
			id           int
			addr         int
			lat, lon     float64
			altitude     int
			quality      int
			climb        int
			windSpeed    int
			windHeading  int
		)

		err := rows.Scan(&id, &addr, &lat, &lon, &altitude, &quality, &climb, &windSpeed, &windHeading)
		if err != nil {
			r.logger.WithField("error", err).Warn("Failed to scan thermal row")
			continue
		}

		thermal := &models.Thermal{
			ID:         strconv.Itoa(id),
			ReportedBy: fmt.Sprintf("%06X", addr),
			Center: models.GeoPoint{
				Latitude:  lat,
				Longitude: lon,
			},
			Altitude:      int32(altitude),
			Quality:       uint8(quality),
			ClimbRate:     int16(climb),
			WindSpeed:     uint8(float64(windSpeed) / 10 * 3.6), // м/с*10 -> км/ч
			WindDirection: uint16(windHeading),
			Timestamp:     time.Now(), // Неизвестно в legacy схеме
		}

		thermals = append(thermals, thermal)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating thermal rows: %w", err)
	}

	r.logger.WithField("count", len(thermals)).Info("Loaded initial thermals from MySQL")
	return thermals, nil
}

// LoadInitialStations загружает начальные данные метеостанций
func (r *MySQLRepository) LoadInitialStations(ctx context.Context, limit int) ([]*models.Station, error) {
	query := `
		SELECT 
			addr,
			COALESCE(name, '') as name,
			COALESCE(latitude, 0) as latitude,
			COALESCE(longitude, 0) as longitude,
			COALESCE(temperature, 0) as temperature,
			COALESCE(wind_heading, 0) as wind_heading,
			COALESCE(wind_speed, 0) as wind_speed,
			COALESCE(wind_gusts, 0) as wind_gusts,
			COALESCE(humidity, 0) as humidity,
			COALESCE(pressure, 0) as pressure,
			COALESCE(battery, 0) as battery,
			datestamp
		FROM station
		WHERE latitude IS NOT NULL AND longitude IS NOT NULL
		  AND latitude != 0 AND longitude != 0
		ORDER BY datestamp DESC
		LIMIT ?
	`

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query initial stations: %w", err)
	}
	defer rows.Close()

	var stations []*models.Station
	for rows.Next() {
		var (
			addr                          int
			name                          string
			lat, lon                      float64
			temperature                   float64
			windHeading                   int
			windSpeed, windGusts         float64
			humidity, pressure, battery   int
			timestamp                     time.Time
		)

		err := rows.Scan(
			&addr, &name, &lat, &lon, &temperature, &windHeading,
			&windSpeed, &windGusts, &humidity, &pressure, &battery, &timestamp,
		)
		if err != nil {
			r.logger.WithField("error", err).Warn("Failed to scan station row")
			continue
		}

		station := &models.Station{
			ID:   fmt.Sprintf("%06X", addr),
			Name: name,
			Position: models.GeoPoint{
				Latitude:  lat,
				Longitude: lon,
			},
			Temperature:   int8(temperature),
			WindDirection: uint16(windHeading),
			WindSpeed:     uint8(windSpeed * 3.6), // м/с -> км/ч
			WindGusts:     uint8(windGusts * 3.6), // м/с -> км/ч
			Humidity:      uint8(humidity),
			Pressure:      uint16(pressure),
			Battery:       uint8(battery),
			LastUpdate:    timestamp,
		}

		stations = append(stations, station)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating station rows: %w", err)
	}

	r.logger.WithField("count", len(stations)).Info("Loaded initial stations from MySQL")
	return stations, nil
}

// GetPilotTrack получает историю трека пилота
func (r *MySQLRepository) GetPilotTrack(ctx context.Context, deviceID string, limit int) ([]models.GeoPoint, error) {
	// Конвертируем hex device ID в int
	addr, err := strconv.ParseInt(deviceID, 16, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid device ID format: %s", deviceID)
	}

	query := `
		SELECT latitude, longitude, altitude_gps, datestamp
		FROM ufo_track
		WHERE addr = ? AND datestamp > DATE_SUB(NOW(), INTERVAL 24 HOUR)
		ORDER BY datestamp DESC
		LIMIT ?
	`

	rows, err := r.db.QueryContext(ctx, query, addr, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query pilot track: %w", err)
	}
	defer rows.Close()

	var track []models.GeoPoint
	for rows.Next() {
		var (
			lat, lon  float64
			altitude  sql.NullFloat64
			timestamp time.Time
		)

		err := rows.Scan(&lat, &lon, &altitude, &timestamp)
		if err != nil {
			r.logger.WithField("error", err).Warn("Failed to scan track point")
			continue
		}

		altInt := int16(0)
		if altitude.Valid {
			altInt = int16(altitude.Float64)
		}

		point := models.GeoPoint{
			Latitude:  lat,
			Longitude: lon,
			Altitude:  altInt,
		}

		track = append(track, point)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating track points: %w", err)
	}

	return track, nil
}

// SavePilotToHistory сохраняет данные пилота в историю (для backup)
func (r *MySQLRepository) SavePilotToHistory(ctx context.Context, pilot *models.Pilot) error {
	// Конвертируем hex device ID в int
	addr, err := strconv.ParseInt(pilot.DeviceID, 16, 32)
	if err != nil {
		return fmt.Errorf("invalid device ID format: %s", pilot.DeviceID)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Вставляем запись в ufo_track
	insertTrackQuery := `
		INSERT INTO ufo_track (
			addr, ufo_type, latitude, longitude, altitude_gps,
			speed, climb, course, track_online, datestamp
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := tx.ExecContext(ctx, insertTrackQuery,
		addr, pilot.AircraftType, pilot.Position.Latitude, pilot.Position.Longitude,
		pilot.Position.Altitude, pilot.Speed, pilot.ClimbRate, pilot.Heading,
		pilot.TrackOnline, pilot.LastUpdate)
	if err != nil {
		return fmt.Errorf("failed to insert track record: %w", err)
	}

	trackID, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get track ID: %w", err)
	}

	// Обновляем last_position в таблице ufo
	updateUFOQuery := `
		INSERT INTO ufo (addr, last_position) VALUES (?, ?)
		ON DUPLICATE KEY UPDATE last_position = VALUES(last_position)
	`

	_, err = tx.ExecContext(ctx, updateUFOQuery, addr, trackID)
	if err != nil {
		return fmt.Errorf("failed to update UFO record: %w", err)
	}

	// Сохраняем имя если есть
	if pilot.Name != "" {
		nameQuery := `
			INSERT INTO name (addr, name) VALUES (?, ?)
			ON DUPLICATE KEY UPDATE name = VALUES(name)
		`
		_, err = tx.ExecContext(ctx, nameQuery, addr, pilot.Name)
		if err != nil {
			r.logger.WithField("device_id", pilot.DeviceID).WithField("error", err).Warn("Failed to save pilot name")
		}
	}

	return tx.Commit()
}

// GetStats возвращает статистику MySQL
func (r *MySQLRepository) GetStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Количество записей в таблицах
	queries := map[string]string{
		"pilots_count":   "SELECT COUNT(*) FROM ufo",
		"tracks_count":   "SELECT COUNT(*) FROM ufo_track WHERE datestamp > DATE_SUB(NOW(), INTERVAL 24 HOUR)",
		"thermals_count": "SELECT COUNT(*) FROM thermal",
		"stations_count": "SELECT COUNT(*) FROM station",
	}

	for key, query := range queries {
		var count int
		err := r.db.QueryRowContext(ctx, query).Scan(&count)
		if err != nil {
			r.logger.WithField("key", key).WithField("error", err).Warn("Failed to get MySQL stat")
			stats[key] = 0
		} else {
			stats[key] = count
		}
	}

	// Статистика соединений
	dbStats := r.db.Stats()
	stats["open_connections"] = dbStats.OpenConnections
	stats["in_use"] = dbStats.InUse
	stats["idle"] = dbStats.Idle

	return stats, nil
}

// CleanupOldTracks удаляет старые треки
func (r *MySQLRepository) CleanupOldTracks(ctx context.Context, olderThan time.Duration) error {
	query := `DELETE FROM ufo_track WHERE datestamp < DATE_SUB(NOW(), INTERVAL ? HOUR)`
	
	result, err := r.db.ExecContext(ctx, query, int(olderThan.Hours()))
	if err != nil {
		return fmt.Errorf("failed to cleanup old tracks: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected > 0 {
		r.logger.WithField("count", affected).WithField("older_than_hours", olderThan.Hours()).Info("Cleaned up old tracks")
	}

	return nil
}