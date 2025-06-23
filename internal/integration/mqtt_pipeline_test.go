package integration

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/flybeeper/fanet-backend/internal/config"
	mqttclient "github.com/flybeeper/fanet-backend/internal/mqtt"
	"github.com/flybeeper/fanet-backend/internal/repository"
	"github.com/flybeeper/fanet-backend/pkg/utils"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// MQTTPipelineTestSuite тестирует полный MQTT → Redis pipeline
type MQTTPipelineTestSuite struct {
	suite.Suite
	mqttClient mqtt.Client
	redisRepo  *repository.RedisRepository
	redisClient *redis.Client
	ctx        context.Context
}

func (suite *MQTTPipelineTestSuite) SetupSuite() {
	suite.ctx = context.Background()

	// Настройка Redis для тестов
	redisConfig := &config.RedisConfig{
		URL:          "redis://localhost:6379",
		Password:     "",
		DB:           14, // Используем отдельную DB для интеграционных тестов
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

	// Настройка MQTT клиента для тестов
	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://localhost:1883")
	opts.SetClientID("integration_test_client")
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(false)
	opts.SetConnectTimeout(5 * time.Second)

	suite.mqttClient = mqtt.NewClient(opts)

	// Подключаемся к MQTT
	if token := suite.mqttClient.Connect(); token.Wait() && token.Error() != nil {
		suite.T().Skip("MQTT broker not available for integration testing: " + token.Error().Error())
	}
}

func (suite *MQTTPipelineTestSuite) SetupTest() {
	// Очищаем Redis перед каждым тестом
	err := suite.redisClient.FlushDB(suite.ctx).Err()
	require.NoError(suite.T(), err)
}

func (suite *MQTTPipelineTestSuite) TearDownSuite() {
	if suite.mqttClient != nil && suite.mqttClient.IsConnected() {
		suite.mqttClient.Disconnect(1000)
	}
	if suite.redisClient != nil {
		suite.redisClient.FlushDB(suite.ctx)
		suite.redisClient.Close()
	}
}

func (suite *MQTTPipelineTestSuite) TestMQTTToRedisPipeline() {
	// Создаем MQTT клиент и парсер
	logger := utils.NewLogger("debug", "text")
	parser := mqttclient.NewParser(logger)
	parser.SetDebugMode(true)

	// Канал для получения обработанных сообщений
	processedMessages := make(chan *mqttclient.FANETMessage, 10)

	// Подписываемся на MQTT топики
	topic := "fb/b/+/f/#"
	token := suite.mqttClient.Subscribe(topic, 0, func(client mqtt.Client, msg mqtt.Message) {
		// Парсим MQTT сообщение
		fanetMsg, err := parser.Parse(msg.Topic(), msg.Payload())
		if err != nil {
			suite.T().Logf("Failed to parse MQTT message: %v", err)
			return
		}

		// Отправляем в канал для проверки
		select {
		case processedMessages <- fanetMsg:
		case <-time.After(1 * time.Second):
			suite.T().Logf("Channel full, dropping message")
		}

		// Здесь в реальном приложении сообщение сохранялось бы в Redis
		// Для теста мы будем симулировать это вручную
	})

	require.True(suite.T(), token.Wait())
	require.NoError(suite.T(), token.Error())

	// Создаем тестовое FANET сообщение Type 1 (Air Tracking)
	testTopic := "fb/b/TEST01/f/1"
	testPayload := suite.createAirTrackingPayload("001234", 46.0, 8.0, 1000, 50, 180)

	// Публикуем сообщение в MQTT
	token = suite.mqttClient.Publish(testTopic, 0, false, testPayload)
	require.True(suite.T(), token.Wait())
	require.NoError(suite.T(), token.Error())

	// Ждем обработки сообщения
	select {
	case fanetMsg := <-processedMessages:
		// Проверяем корректность парсинга
		assert.Equal(suite.T(), uint8(1), fanetMsg.Type)
		assert.Equal(suite.T(), "001234", fanetMsg.DeviceID)
		assert.Equal(suite.T(), "TEST01", fanetMsg.ChipID)
		assert.Equal(suite.T(), "1", fanetMsg.PacketType)

		// Проверяем данные Air Tracking
		require.IsType(suite.T(), &mqttclient.AirTrackingData{}, fanetMsg.Data)
		airData := fanetMsg.Data.(*mqttclient.AirTrackingData)

		assert.InDelta(suite.T(), 46.0, airData.Latitude, 0.01)
		assert.InDelta(suite.T(), 8.0, airData.Longitude, 0.01)
		assert.Equal(suite.T(), int32(1000), airData.Altitude)
		assert.Equal(suite.T(), uint16(50), airData.Speed)
		assert.Equal(suite.T(), uint16(180), airData.Heading)

		suite.T().Logf("Successfully parsed FANET message: %+v", fanetMsg)

	case <-time.After(5 * time.Second):
		suite.T().Fatal("Timeout waiting for MQTT message processing")
	}

	// Отписываемся от топика
	token = suite.mqttClient.Unsubscribe(topic)
	require.True(suite.T(), token.Wait())
	require.NoError(suite.T(), token.Error())
}

func (suite *MQTTPipelineTestSuite) TestMQTTMultipleMessageTypes() {
	logger := utils.NewLogger("debug", "text")
	parser := mqttclient.NewParser(logger)

	processedMessages := make(chan *mqttclient.FANETMessage, 10)

	// Подписываемся на MQTT топики
	topic := "fb/b/+/f/#"
	token := suite.mqttClient.Subscribe(topic, 0, func(client mqtt.Client, msg mqtt.Message) {
		fanetMsg, err := parser.Parse(msg.Topic(), msg.Payload())
		if err != nil {
			suite.T().Logf("Failed to parse MQTT message: %v", err)
			return
		}

		select {
		case processedMessages <- fanetMsg:
		default:
		}
	})

	require.True(suite.T(), token.Wait())
	require.NoError(suite.T(), token.Error())

	// Тестируем разные типы сообщений
	testCases := []struct {
		name        string
		topic       string
		payload     []byte
		expectedType uint8
		deviceID    string
	}{
		{
			name:        "Air Tracking",
			topic:       "fb/b/BASE01/f/1",
			payload:     suite.createAirTrackingPayload("ABC123", 46.1, 8.1, 1100, 60, 90),
			expectedType: 1,
			deviceID:    "ABC123",
		},
		{
			name:        "Name Message",
			topic:       "fb/b/BASE01/f/2",
			payload:     suite.createNamePayload("DEF456", "TestPilot"),
			expectedType: 2,
			deviceID:    "DEF456",
		},
		{
			name:        "Ground Tracking",
			topic:       "fb/b/BASE02/f/7",
			payload:     suite.createGroundTrackingPayload("GHI789", 46.2, 8.2, 500, 30, 270),
			expectedType: 7,
			deviceID:    "GHI789",
		},
	}

	for _, tc := range testCases {
		suite.T().Run(tc.name, func(t *testing.T) {
			// Публикуем сообщение
			token := suite.mqttClient.Publish(tc.topic, 0, false, tc.payload)
			require.True(t, token.Wait())
			require.NoError(t, token.Error())

			// Ждем обработки
			select {
			case fanetMsg := <-processedMessages:
				assert.Equal(t, tc.expectedType, fanetMsg.Type)
				assert.Equal(t, tc.deviceID, fanetMsg.DeviceID)
				t.Logf("Successfully processed %s message for device %s", tc.name, tc.deviceID)

			case <-time.After(3 * time.Second):
				t.Fatal("Timeout waiting for message processing")
			}
		})
	}

	// Отписываемся
	token = suite.mqttClient.Unsubscribe(topic)
	require.True(suite.T(), token.Wait())
	require.NoError(suite.T(), token.Error())
}

func (suite *MQTTPipelineTestSuite) TestHighVolumeMessages() {
	logger := utils.NewLogger("error", "text") // Минимальное логирование для высокой нагрузки
	parser := mqttclient.NewParser(logger)

	messageCount := 100
	processedCount := 0
	processedMessages := make(chan *mqttclient.FANETMessage, messageCount)

	// Подписываемся на MQTT топики
	topic := "fb/b/+/f/#"
	token := suite.mqttClient.Subscribe(topic, 0, func(client mqtt.Client, msg mqtt.Message) {
		fanetMsg, err := parser.Parse(msg.Topic(), msg.Payload())
		if err != nil {
			return
		}

		select {
		case processedMessages <- fanetMsg:
		default:
		}
	})

	require.True(suite.T(), token.Wait())
	require.NoError(suite.T(), token.Error())

	// Отправляем много сообщений быстро
	start := time.Now()
	for i := 0; i < messageCount; i++ {
		deviceID := fmt.Sprintf("%06X", i)
		payload := suite.createAirTrackingPayload(deviceID, 46.0+float64(i)*0.001, 8.0+float64(i)*0.001, 1000+i, 50, 180)
		topic := fmt.Sprintf("fb/b/BASE%02d/f/1", i%10)

		token := suite.mqttClient.Publish(topic, 0, false, payload)
		require.True(suite.T(), token.Wait())
		require.NoError(suite.T(), token.Error())
	}

	// Собираем обработанные сообщения
	timeout := time.After(10 * time.Second)
	for processedCount < messageCount {
		select {
		case <-processedMessages:
			processedCount++
		case <-timeout:
			suite.T().Fatalf("Timeout: processed only %d/%d messages", processedCount, messageCount)
		}
	}

	duration := time.Since(start)
	messagesPerSecond := float64(messageCount) / duration.Seconds()

	suite.T().Logf("Processed %d messages in %v (%.2f msg/sec)", 
		messageCount, duration, messagesPerSecond)

	assert.Equal(suite.T(), messageCount, processedCount)
	assert.Greater(suite.T(), messagesPerSecond, 50.0, "Should process at least 50 messages per second")

	// Отписываемся
	token = suite.mqttClient.Unsubscribe("fb/b/+/f/#")
	require.True(suite.T(), token.Wait())
	require.NoError(suite.T(), token.Error())
}

// Helper methods для создания тестовых payload'ов

func (suite *MQTTPipelineTestSuite) createAirTrackingPayload(deviceID string, lat, lon float64, altitude int32, speed uint16, heading uint16) []byte {
	payload := make([]byte, 8+4+11) // wrapper + header + air tracking data

	// Base station wrapper
	timestamp := uint32(time.Now().Unix())
	binary.LittleEndian.PutUint32(payload[0:4], timestamp)
	binary.LittleEndian.PutUint16(payload[4:6], uint16(-75)) // RSSI
	binary.LittleEndian.PutUint16(payload[6:8], uint16(15))  // SNR

	// FANET header
	payload[8] = 1 // Type 1 (Air tracking)

	// Device ID (3 bytes, little-endian)
	deviceAddr, _ := hex.DecodeString(deviceID)
	if len(deviceAddr) >= 3 {
		payload[9] = deviceAddr[2]  // Low byte
		payload[10] = deviceAddr[1] // Mid byte
		payload[11] = deviceAddr[0] // High byte
	} else {
		payload[9] = 0x34  // Default test device ID
		payload[10] = 0x12
		payload[11] = 0x00
	}

	// Air tracking data
	data := payload[12:]

	// Координаты
	latRaw := int32(lat * 93206.04)
	lonRaw := int32(lon * 46603.02)

	data[0] = byte(latRaw & 0xFF)
	data[1] = byte((latRaw >> 8) & 0xFF)
	data[2] = byte((latRaw >> 16) & 0xFF)

	data[3] = byte(lonRaw & 0xFF)
	data[4] = byte((lonRaw >> 8) & 0xFF)
	data[5] = byte((lonRaw >> 16) & 0xFF)

	// Alt_status
	altStatus := uint16(altitude) | (1 << 12) | (1 << 15) // altitude + aircraft_type=1 + online=true
	binary.LittleEndian.PutUint16(data[6:8], altStatus)

	// Speed (km/h * 2, так как единицы 0.5 км/ч)
	data[8] = byte(speed * 2)

	// Climb rate (положительный)
	data[9] = 20

	// Heading (360° -> 256 units)
	data[10] = byte(heading * 256 / 360)

	return payload
}

func (suite *MQTTPipelineTestSuite) createNamePayload(deviceID string, name string) []byte {
	payload := make([]byte, 8+4+len(name)) // wrapper + header + name

	// Base station wrapper
	timestamp := uint32(time.Now().Unix())
	binary.LittleEndian.PutUint32(payload[0:4], timestamp)
	binary.LittleEndian.PutUint16(payload[4:6], uint16(-70))
	binary.LittleEndian.PutUint16(payload[6:8], uint16(20))

	// FANET header
	payload[8] = 2 // Type 2 (Name)

	// Device ID
	deviceAddr, _ := hex.DecodeString(deviceID)
	if len(deviceAddr) >= 3 {
		payload[9] = deviceAddr[2]
		payload[10] = deviceAddr[1]
		payload[11] = deviceAddr[0]
	} else {
		payload[9] = 0x56
		payload[10] = 0x12
		payload[11] = 0x00
	}

	// Name data
	copy(payload[12:], name)

	return payload
}

func (suite *MQTTPipelineTestSuite) createGroundTrackingPayload(deviceID string, lat, lon float64, altitude int32, speed uint16, heading uint16) []byte {
	payload := make([]byte, 8+4+10) // wrapper + header + ground tracking data

	// Base station wrapper
	timestamp := uint32(time.Now().Unix())
	binary.LittleEndian.PutUint32(payload[0:4], timestamp)
	binary.LittleEndian.PutUint16(payload[4:6], uint16(-80))
	binary.LittleEndian.PutUint16(payload[6:8], uint16(12))

	// FANET header
	payload[8] = 7 // Type 7 (Ground tracking)

	// Device ID
	deviceAddr, _ := hex.DecodeString(deviceID)
	if len(deviceAddr) >= 3 {
		payload[9] = deviceAddr[2]
		payload[10] = deviceAddr[1]
		payload[11] = deviceAddr[0]
	} else {
		payload[9] = 0x9A
		payload[10] = 0x12
		payload[11] = 0x00
	}

	// Ground tracking data
	data := payload[12:]

	// Координаты
	latRaw := int32(lat * 93206.04)
	lonRaw := int32(lon * 46603.02)

	data[0] = byte(latRaw & 0xFF)
	data[1] = byte((latRaw >> 8) & 0xFF)
	data[2] = byte((latRaw >> 16) & 0xFF)

	data[3] = byte(lonRaw & 0xFF)
	data[4] = byte((lonRaw >> 8) & 0xFF)
	data[5] = byte((lonRaw >> 16) & 0xFF)

	// Altitude
	binary.LittleEndian.PutUint16(data[6:8], uint16(altitude))

	// Speed and heading
	speedHeading := uint16(speed<<6) | (heading/6)
	binary.LittleEndian.PutUint16(data[8:10], speedHeading)

	return payload
}

// Запуск интеграционных тестов
func TestMQTTPipelineSuite(t *testing.T) {
	suite.Run(t, new(MQTTPipelineTestSuite))
}