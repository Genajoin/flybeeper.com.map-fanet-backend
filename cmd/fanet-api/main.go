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
	"github.com/flybeeper/fanet-backend/internal/models"
	"github.com/flybeeper/fanet-backend/internal/mqtt"
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
	logger := utils.NewLogger("info", "text")
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
		logger.WithField("error", err).Fatal("Failed to connect to Redis")
	}
	logger.Info("Connected to Redis")

	// Инициализируем MySQL репозиторий (опционально)
	var mysqlRepo *repository.MySQLRepository
	if cfg.MySQL.DSN != "" {
		mysqlRepo, err = repository.NewMySQLRepository(&cfg.MySQL, logger)
		if err != nil {
			logger.WithField("error", err).Warn("Failed to initialize MySQL repository")
		} else {
			defer mysqlRepo.Close()
			if err := mysqlRepo.Ping(ctx); err != nil {
				logger.WithField("error", err).Warn("Failed to connect to MySQL")
			} else {
				logger.Info("Connected to MySQL")
			}
		}
	}

	// Создаем HTTP сервер
	server := handler.NewServer(cfg, redisRepo, logger)

	// Получаем WebSocket handler для интеграции с MQTT
	wsHandler := server.GetWebSocketHandler()

	// Определяем messageHandler с поддержкой WebSocket трансляции
	messageHandler := func(msg *mqtt.FANETMessage) error {
		// Конвертируем FANET сообщение в модели и сохраняем в Redis
		switch msg.Type {
		case 1: // Air tracking
			if pilot := convertFANETToPilot(msg); pilot != nil {
				// Сохраняем в Redis
				if err := redisRepo.SavePilot(ctx, pilot); err != nil {
					return err
				}
				// Транслируем через WebSocket
				pbPilot := convertPilotToProtobuf(pilot)
				wsHandler.BroadcastUpdate(pb.UpdateType_UPDATE_TYPE_PILOT, pb.Action_ACTION_UPDATE, pbPilot)
			}
		case 9: // Thermal
			if thermal := convertFANETToThermal(msg); thermal != nil {
				// Сохраняем в Redis
				if err := redisRepo.SaveThermal(ctx, thermal); err != nil {
					return err
				}
				// Транслируем через WebSocket
				pbThermal := convertThermalToProtobuf(thermal)
				wsHandler.BroadcastUpdate(pb.UpdateType_UPDATE_TYPE_THERMAL, pb.Action_ACTION_ADD, pbThermal)
			}
		case 4: // Weather/Station
			if station := convertFANETToStation(msg); station != nil {
				// Сохраняем в Redis
				if err := redisRepo.SaveStation(ctx, station); err != nil {
					return err
				}
				// Транслируем через WebSocket
				pbStation := convertStationToProtobuf(station)
				wsHandler.BroadcastUpdate(pb.UpdateType_UPDATE_TYPE_STATION, pb.Action_ACTION_UPDATE, pbStation)
			}
		}
		return nil
	}

	// Инициализируем MQTT клиент с готовым messageHandler
	mqttClient, err := mqtt.NewClient(&cfg.MQTT, logger, messageHandler)
	if err != nil {
		logger.WithField("error", err).Fatal("Failed to initialize MQTT client")
	}
	defer mqttClient.Disconnect()

	// Подключаемся к MQTT
	if err := mqttClient.Connect(); err != nil {
		logger.WithField("error", err).Fatal("Failed to connect to MQTT broker")
	}
	logger.Info("Connected to MQTT broker")

	// Запускаем HTTP сервер в горутине
	go func() {
		logger.WithField("address", cfg.Server.Address).Info("Starting HTTP/2 server")
		if err := server.Start(); err != nil {
			logger.WithField("error", err).Fatal("Failed to start HTTP server")
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
		AircraftType: airData.AircraftType,
		Position: models.GeoPoint{
			Latitude:  airData.Latitude,
			Longitude: airData.Longitude,
			Altitude:  int16(airData.Altitude),
		},
		Speed:       airData.Speed,
		ClimbRate:   airData.ClimbRate,
		Heading:     airData.Heading,
		TrackOnline: true, // Факт получения сообщения означает онлайн
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
		Center: models.GeoPoint{
			Latitude:  thermalData.Latitude,
			Longitude: thermalData.Longitude,
		},
		Altitude:      thermalData.Altitude,
		Quality:       uint8(thermalData.Strength / 20), // Конвертируем 0-100 в 0-5
		ClimbRate:     thermalData.ClimbRate,
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

	weatherData, ok := serviceData.Data.(*mqtt.WeatherData)
	if !ok {
		return nil
	}

	return &models.Station{
		ID:   msg.DeviceID,
		Name: "", // Имя приходит в отдельном сообщении Type 2
		Position: models.GeoPoint{
			Latitude:  0, // Координаты станции не в FANET Weather
			Longitude: 0, // Координаты станции не в FANET Weather
		},
		Temperature:   weatherData.Temperature,
		WindSpeed:     weatherData.WindSpeed,
		WindDirection: weatherData.WindDirection,
		WindGusts:     0, // Нет в FANET Weather
		Humidity:      weatherData.Humidity,
		Pressure:      weatherData.Pressure,
		Battery:       100, // Нет в FANET Weather
		LastUpdate:    msg.Timestamp,
	}
}

// Конвертеры для Protobuf

func convertPilotToProtobuf(pilot *models.Pilot) *pb.Pilot {
	return &pb.Pilot{
		Addr: 0, // TODO: конвертировать DeviceID в uint32
		Name: pilot.Name,
		Type: pb.PilotType(pilot.AircraftType),
		Position: &pb.GeoPoint{
			Latitude:  pilot.Position.Latitude,
			Longitude: pilot.Position.Longitude,
		},
		Altitude:   int32(pilot.Position.Altitude),
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
			Latitude:  thermal.Center.Latitude,
			Longitude: thermal.Center.Longitude,
		},
		Altitude:    int32(thermal.Altitude),
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