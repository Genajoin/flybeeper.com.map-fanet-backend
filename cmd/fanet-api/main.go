package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/flybeeper/fanet-backend/internal/config"
	"github.com/flybeeper/fanet-backend/internal/handler"
	"github.com/flybeeper/fanet-backend/internal/metrics"
	"github.com/flybeeper/fanet-backend/internal/models"
	"github.com/flybeeper/fanet-backend/internal/mqtt"
	"github.com/flybeeper/fanet-backend/internal/service"
	"github.com/flybeeper/fanet-backend/internal/repository"
	"github.com/flybeeper/fanet-backend/pkg/pb"
	"github.com/flybeeper/fanet-backend/pkg/utils"
)

var (
	// Version будет установлен при сборке через ldflags
	Version = "dev"
)

func main() {
	// Загружаем конфигурацию
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Инициализируем логирование
	logger := utils.NewLogger(config.LogLevel(), config.LogFormat())
	logger.WithField("version", Version).Info("Starting FANET Backend")

	// Создаем контекст приложения
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Инициализируем Redis репозиторий
	redisRepo, err := repository.NewRedisRepository(&cfg.Redis, logger)
	if err != nil {
		logger.WithField("error", err).Fatal("Failed to initialize Redis repository")
	}
	defer redisRepo.Close()

	// Проверяем соединение с Redis
	if err := redisRepo.Ping(ctx); err != nil {
		metrics.RedisConnectionStatus.Set(0)
		logger.WithField("error", err).Fatal("Failed to connect to Redis")
	}
	metrics.RedisConnectionStatus.Set(1)
	logger.Info("Connected to Redis")

	// Инициализируем MySQL репозиторий с retry логикой
	var mysqlRepo *repository.MySQLRepository
	var batchWriter *service.BatchWriter
	if cfg.MySQL.DSN != "" {
		mysqlRepo, batchWriter = initializeMySQLWithRetry(ctx, &cfg.MySQL, logger)
		if mysqlRepo != nil {
			defer mysqlRepo.Close()
		}
		if batchWriter != nil {
			defer batchWriter.Stop()
		}
	}

	// Создаем сервис валидации входящих данных
	// Система валидации предотвращает сохранение недостоверных данных от "фантомных" пилотов
	// Алгоритм: первый пакет ждет валидации, второй проверяется по скорости движения
	validationService := service.NewValidationService(logger, nil)
	
	// Запускаем периодическую очистку старых состояний валидации
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				removed := validationService.CleanupOldStates(2 * time.Hour)
				if removed > 0 {
					logger.WithField("removed", removed).Debug("Cleaned up old validation states")
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Создаем сервис отслеживания границ OGN
	ognCenter := models.GeoPoint{
		Latitude:  cfg.Geo.OGNCenterLat,
		Longitude: cfg.Geo.OGNCenterLon,
	}
	boundaryTracker := service.NewBoundaryTracker(
		logger,
		ognCenter,
		cfg.Geo.OGNRadiusKM,
		cfg.Geo.TrackingRadiusPercent,
		cfg.Geo.BoundaryGracePeriod,
		cfg.Geo.MinMovementDistance,
	)

	// Создаем HTTP сервер с Redis клиентом для auth кеширования, сервисом валидации и boundary tracker
	server := handler.NewServer(cfg, redisRepo, mysqlRepo, redisRepo.GetClient(), logger, validationService, boundaryTracker)

	// Получаем WebSocket handler для интеграции с MQTT
	wsHandler := server.GetWebSocketHandler()

	// Определяем messageHandler с поддержкой WebSocket трансляции и асинхронного MySQL
	messageHandler := func(msg *mqtt.FANETMessage) error {
		// Конвертируем FANET сообщение в модели и сохраняем в Redis + MySQL
		switch msg.Type {
		case 1: // Air tracking
			if pilot := convertFANETToPilot(msg); pilot != nil {
				// Получаем предыдущую позицию из Redis для определения движения
				existingPilot, err := redisRepo.GetPilot(ctx, pilot.DeviceID)
				var lastPosition *models.GeoPoint
				if err == nil && existingPilot != nil && existingPilot.Position != nil {
					lastPosition = existingPilot.Position
				}
				
				// Определяем статус объекта относительно границ всех центров отслеживания
				status := boundaryTracker.GetObjectStatus(*pilot.Position, lastPosition, pilot.LastUpdate)
				
				// Обновляем поля модели на основе статуса
				pilot.TrackingDistance = status.Distance
				pilot.VisibilityStatus = status.VisibilityStatus
				if status.LastMovement != pilot.LastUpdate {
					pilot.LastMovement = &status.LastMovement
				}
				
				// Логируем начало обработки пилота
				logger.WithFields(map[string]interface{}{
					"device_id": pilot.DeviceID,
					"latitude": pilot.Position.Latitude,
					"longitude": pilot.Position.Longitude,
					"altitude": pilot.Position.Altitude,
					"aircraft_type": pilot.Type,
					"online": pilot.TrackOnline,
					"visibility_status": pilot.VisibilityStatus,
					"tracking_distance": pilot.TrackingDistance,
				}).Debug("Processing pilot data")
				
				// Валидируем данные пилота с новой логикой скоринга
				isValid, shouldStore, err := validationService.ValidatePilot(pilot)
				if err != nil {
					logger.WithField("error", err).WithField("device_id", pilot.DeviceID).
						Error("Failed to validate pilot")
					return err
				}
				
				// Проверяем состояние валидации для принятия решений
				state, stateExists := validationService.GetValidationState(pilot.DeviceID)
				
				if shouldStore {
					// Счет достаточен для сохранения в Redis
					if err := redisRepo.SavePilot(ctx, pilot); err != nil {
						logger.WithField("error", err).WithField("device_id", pilot.DeviceID).
							Error("Failed to save pilot to Redis")
						return err
					}
					
					logger.WithFields(map[string]interface{}{
						"device_id": pilot.DeviceID,
						"is_valid": isValid,
						"validation_score": func() int {
							if stateExists {
								return state.ValidationScore
							}
							return -1
						}(),
					}).Debug("Successfully saved pilot to Redis")
					
					// Асинхронно добавляем в MySQL batch queue
					if batchWriter != nil {
						if err := batchWriter.QueuePilot(pilot); err != nil {
							logger.WithField("error", err).WithField("device_id", pilot.DeviceID).
								Warn("Failed to queue pilot for MySQL batch")
						} else {
							logger.WithField("device_id", pilot.DeviceID).Debug("Queued pilot for MySQL batch")
						}
					}
					
					// Транслируем через WebSocket только если пилот должен быть видим
					pbPilot := convertPilotToProtobuf(pilot)
					wsHandler.BroadcastUpdate(pb.UpdateType_UPDATE_TYPE_PILOT, pb.Action_ACTION_UPDATE, pbPilot)
					logger.WithField("device_id", pilot.DeviceID).Debug("Broadcasted pilot update via WebSocket")
				} else {
					// Счет недостаточен - удаляем из Redis если был там
					if stateExists && state.IsValidated {
						// Пилот ранее был валидным но теперь упал ниже порога
						if err := redisRepo.RemovePilot(ctx, pilot.DeviceID); err != nil {
							logger.WithField("error", err).WithField("device_id", pilot.DeviceID).
								Warn("Failed to remove pilot from Redis")
						} else {
							logger.WithFields(map[string]interface{}{
								"device_id": pilot.DeviceID,
								"validation_score": state.ValidationScore,
							}).Info("Removed pilot from Redis due to low validation score")
							
							// Отправляем сигнал удаления через WebSocket
							pbPilot := convertPilotToProtobuf(pilot)
							wsHandler.BroadcastUpdate(pb.UpdateType_UPDATE_TYPE_PILOT, pb.Action_ACTION_REMOVE, pbPilot)
						}
					}
					
					logger.WithFields(map[string]interface{}{
						"device_id": pilot.DeviceID,
						"is_valid": isValid,
						"validation_score": func() int {
							if stateExists {
								return state.ValidationScore
							}
							return -1
						}(),
					}).Debug("Pilot validation score insufficient for Redis storage")
				}
			} else {
				logger.WithField("fanet_type", msg.Type).Warn("Failed to convert FANET message to pilot model")
			}
		case 2: // Name update
			if nameUpdate := convertFANETToNameUpdate(msg); nameUpdate != nil {
				logger.WithFields(map[string]interface{}{
					"device_id": nameUpdate.DeviceID,
					"name": nameUpdate.Name,
				}).Debug("Processing name update")
				
				// Обновляем имя пилота в Redis
				if err := redisRepo.UpdatePilotName(ctx, nameUpdate.DeviceID, nameUpdate.Name); err != nil {
					logger.WithField("error", err).WithField("device_id", nameUpdate.DeviceID).
						Error("Failed to update pilot name in Redis")
				} else {
					logger.WithField("device_id", nameUpdate.DeviceID).Debug("Successfully updated pilot name in Redis")
				}
				
				// Асинхронно обновляем в MySQL через batch
				if batchWriter != nil {
					pilot := &models.Pilot{
						DeviceID: nameUpdate.DeviceID,
						Name:     nameUpdate.Name,
						LastUpdate: time.Now(),
					}
					if err := batchWriter.QueuePilot(pilot); err != nil {
						logger.WithField("error", err).WithField("device_id", nameUpdate.DeviceID).
							Warn("Failed to queue name update for MySQL batch")
					} else {
						logger.WithField("device_id", nameUpdate.DeviceID).Debug("Queued name update for MySQL batch")
					}
				}
			} else {
				logger.WithField("fanet_type", msg.Type).Warn("Failed to convert FANET message to name update")
			}
		case 9: // Thermal
			if thermal := convertFANETToThermal(msg); thermal != nil {
				logger.WithFields(map[string]interface{}{
					"thermal_id": thermal.ID,
					"reported_by": thermal.ReportedBy,
					"latitude": thermal.Position.Latitude,
					"longitude": thermal.Position.Longitude,
					"quality": thermal.Quality,
				}).Debug("Processing thermal data")
				
				// Сохраняем в Redis
				if err := redisRepo.SaveThermal(ctx, thermal); err != nil {
					logger.WithField("error", err).WithField("thermal_id", thermal.ID).
						Error("Failed to save thermal to Redis")
					return err
				}
				
				logger.WithField("thermal_id", thermal.ID).Debug("Successfully saved thermal to Redis")
				
				// Асинхронно добавляем в MySQL batch queue
				if batchWriter != nil {
					if err := batchWriter.QueueThermal(thermal); err != nil {
						logger.WithField("error", err).WithField("thermal_id", thermal.ID).
							Warn("Failed to queue thermal for MySQL batch")
					} else {
						logger.WithField("thermal_id", thermal.ID).Debug("Queued thermal for MySQL batch")
					}
				}
				
				// Транслируем через WebSocket
				pbThermal := convertThermalToProtobuf(thermal)
				wsHandler.BroadcastUpdate(pb.UpdateType_UPDATE_TYPE_THERMAL, pb.Action_ACTION_ADD, pbThermal)
				logger.WithField("thermal_id", thermal.ID).Debug("Broadcasted thermal update via WebSocket")
			} else {
				logger.WithField("fanet_type", msg.Type).Warn("Failed to convert FANET message to thermal model")
			}
		case 4: // Weather/Station
			if station := convertFANETToStation(msg); station != nil {
				logger.WithFields(map[string]interface{}{
					"station_id": station.ID,
					"latitude": station.Position.Latitude,
					"longitude": station.Position.Longitude,
					"temperature": station.Temperature,
					"pressure": station.Pressure,
				}).Debug("Processing station data")
				
				// Сохраняем в Redis
				if err := redisRepo.SaveStation(ctx, station); err != nil {
					logger.WithField("error", err).WithField("station_id", station.ID).
						Error("Failed to save station to Redis")
					return err
				}
				
				logger.WithField("station_id", station.ID).Debug("Successfully saved station to Redis")
				
				// Асинхронно добавляем в MySQL batch queue
				if batchWriter != nil {
					if err := batchWriter.QueueStation(station); err != nil {
						logger.WithField("error", err).WithField("station_id", station.ID).
							Warn("Failed to queue station for MySQL batch")
					} else {
						logger.WithField("station_id", station.ID).Debug("Queued station for MySQL batch")
					}
				}
				
				// Транслируем через WebSocket
				pbStation := convertStationToProtobuf(station)
				wsHandler.BroadcastUpdate(pb.UpdateType_UPDATE_TYPE_STATION, pb.Action_ACTION_UPDATE, pbStation)
				logger.WithField("station_id", station.ID).Debug("Broadcasted station update via WebSocket")
			} else {
				logger.WithField("fanet_type", msg.Type).Warn("Failed to convert FANET message to station model")
			}
		default:
			logger.WithField("fanet_type", msg.Type).Debug("Unhandled FANET message type")
		}
		return nil
	}

	// Запускаем HTTP сервер в горутине
	go func() {
		logger.WithField("address", cfg.Server.Address).Info("Starting HTTP/2 server")
		if err := server.Start(); err != nil {
			logger.WithField("error", err).Fatal("Failed to start HTTP server")
		}
	}()

	// Даем серверу время на запуск
	time.Sleep(1 * time.Second)

	// Инициализируем MQTT клиент с готовым messageHandler
	mqttClient, err := mqtt.NewClient(&cfg.MQTT, logger, messageHandler)
	if err != nil {
		logger.WithField("error", err).Fatal("Failed to initialize MQTT client")
	}
	defer mqttClient.Disconnect()

	// Подключаемся к MQTT в горутине (неблокирующе)
	go func() {
		logger.WithField("broker", cfg.MQTT.URL).Info("Connecting to MQTT broker")
		if err := mqttClient.Connect(); err != nil {
			logger.WithField("error", err).Error("Failed to connect to MQTT broker")
		} else {
			logger.Info("Connected to MQTT broker")
		}
	}()

	// Загружаем начальные данные из MySQL (если доступен)
	if mysqlRepo != nil {
		go func() {
			loadInitialData(ctx, mysqlRepo, redisRepo, logger)
		}()
	}

	// Ждем сигнала остановки
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan

	logger.WithField("signal", sig).Info("Received shutdown signal")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Отменяем контекст приложения
	cancel()

	// Останавливаем HTTP сервер
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.WithField("error", err).Error("HTTP server shutdown error")
	}

	logger.Info("Server stopped gracefully")
}

// loadInitialData загружает начальные данные из MySQL в Redis
func loadInitialData(ctx context.Context, mysqlRepo *repository.MySQLRepository, redisRepo *repository.RedisRepository, logger *utils.Logger) {
	logger.Info("Loading initial data from MySQL")

	// Загружаем пилотов
	pilots, err := mysqlRepo.LoadInitialPilots(ctx, 1000)
	if err != nil {
		logger.WithField("error", err).Error("Failed to load initial pilots")
	} else {
		for _, pilot := range pilots {
			if err := redisRepo.SavePilot(ctx, pilot); err != nil {
				logger.WithField("error", err).WithField("device_id", pilot.DeviceID).Warn("Failed to save pilot to Redis")
			}
		}
		logger.WithField("count", len(pilots)).Info("Loaded initial pilots")
	}

	// Загружаем термики
	thermals, err := mysqlRepo.LoadInitialThermals(ctx, 500)
	if err != nil {
		logger.WithField("error", err).Error("Failed to load initial thermals")
	} else {
		for _, thermal := range thermals {
			if err := redisRepo.SaveThermal(ctx, thermal); err != nil {
				logger.WithField("error", err).WithField("thermal_id", thermal.ID).Warn("Failed to save thermal to Redis")
			}
		}
		logger.WithField("count", len(thermals)).Info("Loaded initial thermals")
	}

	// Загружаем станции
	stations, err := mysqlRepo.LoadInitialStations(ctx, 100)
	if err != nil {
		logger.WithField("error", err).Error("Failed to load initial stations")
	} else {
		for _, station := range stations {
			if err := redisRepo.SaveStation(ctx, station); err != nil {
				logger.WithField("error", err).WithField("station_id", station.ID).Warn("Failed to save station to Redis")
			}
		}
		logger.WithField("count", len(stations)).Info("Loaded initial stations")
	}

	logger.Info("Initial data loading completed")
}

// Конвертеры FANET сообщений в модели данных

func convertFANETToPilot(msg *mqtt.FANETMessage) *models.Pilot {
	// Получаем данные для Air tracking (Type 1)
	airData, ok := msg.Data.(*mqtt.AirTrackingData)
	if !ok {
		return nil
	}

	return &models.Pilot{
		DeviceID:     msg.DeviceID,
		Name:         "", // Имя приходит в отдельном сообщении Type 2
		Type:         models.PilotType(airData.AircraftType),
		Position: &models.GeoPoint{
			Latitude:  airData.Latitude,
			Longitude: airData.Longitude,
			Altitude:  airData.Altitude,
		},
		Speed:       float32(airData.Speed),
		ClimbRate:   airData.ClimbRate,
		Heading:     float32(airData.Heading),
		TrackOnline: airData.OnlineTracking, // Из FANET alt_status bit 15
		Battery:     100,  // Нет в FANET Type 1
		LastUpdate:  msg.Timestamp,
	}
}

func convertFANETToThermal(msg *mqtt.FANETMessage) *models.Thermal {
	// Получаем данные для Thermal (Type 9) 
	thermalData, ok := msg.Data.(*mqtt.ThermalData)
	if !ok {
		return nil
	}

	return &models.Thermal{
		ID:         fmt.Sprintf("%s_%d", msg.DeviceID, msg.Timestamp.Unix()),
		ReportedBy: msg.DeviceID,
		Position: &models.GeoPoint{
			Latitude:  thermalData.Latitude,
			Longitude: thermalData.Longitude,
			Altitude:  thermalData.Altitude,
		},
		Quality:       int32(thermalData.Strength / 20), // Конвертируем 0-100 в 0-5
		ClimbRate:     float32(thermalData.ClimbRate),
		WindSpeed:     0,  // Нет в FANET Thermal
		WindDirection: 0,  // Нет в FANET Thermal
		Timestamp:     msg.Timestamp,
	}
}

func convertFANETToStation(msg *mqtt.FANETMessage) *models.Station {
	// Получаем данные для Weather service (Type 4)
	serviceData, ok := msg.Data.(*mqtt.ServiceData)
	if !ok {
		return nil
	}

	// Создаем базовую станцию с координатами из ServiceData
	station := &models.Station{
		ID:   msg.DeviceID,
		Name: "", // Имя приходит в отдельном сообщении Type 2
		Position: &models.GeoPoint{
			Latitude:  serviceData.Latitude,  // Координаты станции из FANET Type 4
			Longitude: serviceData.Longitude, // Координаты станции из FANET Type 4
		},
		LastUpdate: msg.Timestamp,
	}

	// Если есть погодные данные, добавляем их
	if weatherData, ok := serviceData.Data.(*mqtt.WeatherData); ok {
		station.Temperature = int8(weatherData.Temperature)
		station.WindSpeed = uint8(weatherData.WindSpeed)
		station.WindDirection = weatherData.WindDirection
		station.WindGusts = uint8(weatherData.WindGusts)
		station.Humidity = weatherData.Humidity
		station.Pressure = uint16(weatherData.Pressure)
		station.Battery = weatherData.Battery
	}

	return station
}

// NameUpdate структура для обновления имени пилота
type NameUpdate struct {
	DeviceID string
	Name     string
}

func convertFANETToNameUpdate(msg *mqtt.FANETMessage) *NameUpdate {
	// Получаем данные для Name (Type 2)
	nameData, ok := msg.Data.(*mqtt.NameData)
	if !ok {
		return nil
	}

	return &NameUpdate{
		DeviceID: msg.DeviceID,
		Name:     nameData.Name,
	}
}

// Конвертеры для Protobuf

func convertPilotToProtobuf(pilot *models.Pilot) *pb.Pilot {
	return &pb.Pilot{
		Addr: 0, // TODO: конвертировать DeviceID в uint32
		Name: pilot.Name,
		Type: pb.PilotType(pilot.Type),
		Position: &pb.GeoPoint{
			Latitude:  pilot.Position.Latitude,
			Longitude: pilot.Position.Longitude,
			Altitude:  pilot.Position.Altitude,
		},
		Speed:      float32(pilot.Speed),
		Climb:      float32(pilot.ClimbRate) / 10.0, // Конвертируем обратно в м/с
		Course:     float32(pilot.Heading),
		LastUpdate: pilot.LastUpdate.Unix(),
		TrackOnline: pilot.TrackOnline,
		Battery:    uint32(pilot.Battery),
	}
}


func convertThermalToProtobuf(thermal *models.Thermal) *pb.Thermal {
	return &pb.Thermal{
		Id:   0, // TODO: конвертировать ID в uint64
		Addr: 0, // TODO: конвертировать ReportedBy в uint32
		Position: &pb.GeoPoint{
			Latitude:  thermal.Position.Latitude,
			Longitude: thermal.Position.Longitude,
			Altitude:  thermal.Position.Altitude,
		},
		Quality:     uint32(thermal.Quality),
		Climb:       float32(thermal.ClimbRate),
		WindSpeed:   float32(thermal.WindSpeed),
		WindHeading: float32(thermal.WindDirection),
		Timestamp:   thermal.Timestamp.Unix(),
	}
}

func convertStationToProtobuf(station *models.Station) *pb.Station {
	return &pb.Station{
		Addr: 0, // TODO: конвертировать ID в uint32
		Name: station.Name,
		Position: &pb.GeoPoint{
			Latitude:  station.Position.Latitude,
			Longitude: station.Position.Longitude,
		},
		Temperature: float32(station.Temperature),
		WindSpeed:   float32(station.WindSpeed),
		WindHeading: float32(station.WindDirection),
		WindGusts:   float32(station.WindGusts),
		Humidity:    uint32(station.Humidity),
		Pressure:    float32(station.Pressure),
		Battery:     uint32(station.Battery),
		LastUpdate:  station.LastUpdate.Unix(),
	}
}

// initializeMySQLWithRetry инициализирует MySQL соединение с retry логикой
func initializeMySQLWithRetry(ctx context.Context, cfg *config.MySQLConfig, logger *utils.Logger) (*repository.MySQLRepository, *service.BatchWriter) {
	maxRetries := 5
	retryDelay := 2 * time.Second
	
	var mysqlRepo *repository.MySQLRepository
	var err error
	
	// Попытки создать репозиторий
	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			logger.WithField("attempt", i+1).WithField("max_retries", maxRetries).
				Info("Retrying MySQL connection...")
			time.Sleep(retryDelay)
		}
		
		mysqlRepo, err = repository.NewMySQLRepository(cfg, logger)
		if err != nil {
			logger.WithField("error", err).WithField("attempt", i+1).
				Warn("Failed to initialize MySQL repository")
			continue
		}
		
		// Проверяем соединение
		if err := mysqlRepo.Ping(ctx); err != nil {
			metrics.MySQLConnectionStatus.Set(0)
			logger.WithField("error", err).WithField("attempt", i+1).
				Warn("Failed to ping MySQL")
			mysqlRepo.Close()
			mysqlRepo = nil
			continue
		}
		
		// Успешное подключение
		metrics.MySQLConnectionStatus.Set(1)
		logger.Info("Connected to MySQL")
		
		// Инициализируем batch writer
		batchWriter := service.NewBatchWriter(mysqlRepo, logger, nil)
		
		logger.WithField("batch_size", 1000).
			WithField("flush_interval", "5s").
			WithField("worker_count", 10).
			Info("Started MySQL batch writer")
		
		return mysqlRepo, batchWriter
	}
	
	// Все попытки исчерпаны
	logger.WithField("max_retries", maxRetries).
		Error("Failed to connect to MySQL after all retries")
	metrics.MySQLConnectionStatus.Set(0)
	
	return nil, nil
}