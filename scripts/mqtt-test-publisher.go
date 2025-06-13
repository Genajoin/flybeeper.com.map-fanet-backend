package main

import (
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// Конфигурация тестовых данных
type TestConfig struct {
	BrokerURL     string
	ChipIDs       []string
	PacketTypes   []int
	PublishRate   time.Duration
	MaxMessages   int
	ClientID      string
	RandomSeed    int64
	StartLat      float64
	StartLon      float64
	MovementSpeed float64 // км/ч для симуляции движения
}

// TestPublisher публикует тестовые MQTT сообщения
type TestPublisher struct {
	client mqtt.Client
	config *TestConfig
	rand   *rand.Rand
	pilots map[string]*PilotState // Состояние симулированных пилотов
}

// PilotState состояние симулированного пилота для реалистичного движения
type PilotState struct {
	DeviceID    string
	Latitude    float64
	Longitude   float64
	Altitude    int32
	Speed       uint16
	Heading     uint16
	ClimbRate   int16
	AircraftType uint8
	Name        string
	LastUpdate  time.Time
}

func main() {
	// Параметры командной строки
	var (
		brokerURL     = flag.String("broker", "tcp://localhost:1883", "MQTT broker URL")
		chipIDsStr    = flag.String("chips", "8896672,7048812,2462966788", "Chip IDs (comma-separated)")
		packetTypesStr = flag.String("types", "1,2,4,7,9", "Packet types to publish (comma-separated)")
		rate          = flag.Duration("rate", 2*time.Second, "Publish rate per pilot")
		maxMessages   = flag.Int("max", 0, "Max messages (0 = unlimited)")
		clientID      = flag.String("client", "fanet-test-publisher", "MQTT client ID")
		seed          = flag.Int64("seed", time.Now().UnixNano(), "Random seed")
		lat           = flag.Float64("lat", 46.0, "Start latitude")
		lon           = flag.Float64("lon", 13.0, "Start longitude")
		speed         = flag.Float64("speed", 50.0, "Movement speed km/h")
	)
	flag.Parse()

	// Парсинг chip IDs
	chipIDs := parseStringSlice(*chipIDsStr)
	packetTypes := parseIntSlice(*packetTypesStr)

	config := &TestConfig{
		BrokerURL:     *brokerURL,
		ChipIDs:       chipIDs,
		PacketTypes:   packetTypes,
		PublishRate:   *rate,
		MaxMessages:   *maxMessages,
		ClientID:      *clientID,
		RandomSeed:    *seed,
		StartLat:      *lat,
		StartLon:      *lon,
		MovementSpeed: *speed,
	}

	// Создание и запуск тестового издателя
	publisher, err := NewTestPublisher(config)
	if err != nil {
		log.Fatalf("Ошибка создания издателя: %v", err)
	}

	fmt.Printf("🚀 Начинаем публикацию тестовых MQTT сообщений\n")
	fmt.Printf("📡 Брокер: %s\n", config.BrokerURL)
	fmt.Printf("📟 Базовые станции: %v\n", config.ChipIDs)
	fmt.Printf("📦 Типы пакетов: %v\n", config.PacketTypes)
	fmt.Printf("⏱️  Частота: %v на пилота\n", config.PublishRate)
	fmt.Printf("🌍 Стартовая позиция: %.4f, %.4f\n", config.StartLat, config.StartLon)
	if config.MaxMessages > 0 {
		fmt.Printf("🔢 Максимум сообщений: %d\n", config.MaxMessages)
	}
	fmt.Println()

	// Обработка сигналов для graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Запуск издателя
	done := make(chan bool)
	go func() {
		publisher.Start()
		done <- true
	}()

	select {
	case <-sigChan:
		fmt.Println("\n⏹️  Получен сигнал завершения...")
		publisher.Stop()
	case <-done:
		fmt.Println("\n✅ Публикация завершена")
	}

	fmt.Println("👋 До свидания!")
}

// NewTestPublisher создает новый тестовый издатель
func NewTestPublisher(config *TestConfig) (*TestPublisher, error) {
	// Создание MQTT клиента
	opts := mqtt.NewClientOptions()
	opts.AddBroker(config.BrokerURL)
	opts.SetClientID(config.ClientID)
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(true)

	client := mqtt.NewClient(opts)

	// Подключение к брокеру
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return nil, fmt.Errorf("ошибка подключения к MQTT брокеру: %w", token.Error())
	}

	fmt.Println("✅ Подключен к MQTT брокеру")

	// Инициализация состояния пилотов
	rng := rand.New(rand.NewSource(config.RandomSeed))
	pilots := make(map[string]*PilotState)

	for i, chipID := range config.ChipIDs {
		// Создаем несколько пилотов для каждой базовой станции
		for pilotNum := 1; pilotNum <= 3; pilotNum++ {
			deviceID := fmt.Sprintf("%06X", 0x100000+i*1000+pilotNum)
			pilots[deviceID] = &PilotState{
				DeviceID:     deviceID,
				Latitude:     config.StartLat + rng.Float64()*0.5 - 0.25, // ±0.25 градуса
				Longitude:    config.StartLon + rng.Float64()*0.5 - 0.25,
				Altitude:     int32(1000 + rng.Intn(2000)), // 1000-3000м
				Speed:        uint16(30 + rng.Intn(70)),     // 30-100 км/ч
				Heading:      uint16(rng.Intn(360)),         // 0-359 градусов
				ClimbRate:    int16(rng.Intn(60) - 30),      // ±3 м/с * 10
				AircraftType: uint8(1 + rng.Intn(4)),        // 1-4 (параплан, дельтаплан, шар, планер)
				Name:         fmt.Sprintf("TestPilot_%s_%d", chipID, pilotNum),
				LastUpdate:   time.Now(),
			}
		}
	}

	return &TestPublisher{
		client: client,
		config: config,
		rand:   rng,
		pilots: pilots,
	}, nil
}

// Start запускает публикацию сообщений
func (p *TestPublisher) Start() {
	messageCount := 0
	ticker := time.NewTicker(p.config.PublishRate)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Публикуем сообщения для каждого пилота
			for _, pilot := range p.pilots {
				// Выбираем случайную базовую станцию и тип пакета
				chipID := p.config.ChipIDs[p.rand.Intn(len(p.config.ChipIDs))]
				packetType := p.config.PacketTypes[p.rand.Intn(len(p.config.PacketTypes))]

				// Обновляем состояние пилота для реалистичности
				p.updatePilotState(pilot)

				// Создаем и публикуем сообщение
				if err := p.publishMessage(chipID, pilot, packetType); err != nil {
					log.Printf("❌ Ошибка публикации: %v", err)
				} else {
					messageCount++
					if messageCount%10 == 0 {
						fmt.Printf("📤 Опубликовано сообщений: %d\n", messageCount)
					}
				}

				// Проверяем лимит сообщений
				if p.config.MaxMessages > 0 && messageCount >= p.config.MaxMessages {
					fmt.Printf("🏁 Достигнут лимит сообщений: %d\n", messageCount)
					return
				}
			}
		}
	}
}

// Stop останавливает издателя
func (p *TestPublisher) Stop() {
	if p.client.IsConnected() {
		p.client.Disconnect(1000)
		fmt.Println("🔌 Отключен от MQTT брокера")
	}
}

// updatePilotState обновляет состояние пилота для симуляции движения
func (p *TestPublisher) updatePilotState(pilot *PilotState) {
	now := time.Now()
	dt := now.Sub(pilot.LastUpdate).Seconds()
	pilot.LastUpdate = now

	// Симуляция движения
	speedMS := float64(pilot.Speed) / 3.6 // км/ч -> м/с
	distance := speedMS * dt              // метры

	// Обновление позиции (упрощенно, без учета кривизны Земли)
	headingRad := float64(pilot.Heading) * math.Pi / 180
	latDelta := distance * math.Cos(headingRad) / 111111.0 // ~111км на градус
	lonDelta := distance * math.Sin(headingRad) / (111111.0 * math.Cos(pilot.Latitude*math.Pi/180))

	pilot.Latitude += latDelta
	pilot.Longitude += lonDelta

	// Случайные изменения параметров
	if p.rand.Float64() < 0.1 { // 10% вероятность изменения курса
		pilot.Heading = uint16((int(pilot.Heading) + p.rand.Intn(60) - 30) % 360)
	}

	if p.rand.Float64() < 0.1 { // 10% вероятность изменения скорости
		speedChange := p.rand.Intn(20) - 10
		newSpeed := int(pilot.Speed) + speedChange
		if newSpeed < 20 {
			newSpeed = 20
		}
		if newSpeed > 150 {
			newSpeed = 150
		}
		pilot.Speed = uint16(newSpeed)
	}

	// Симуляция набора высоты
	pilot.Altitude += int32(pilot.ClimbRate/10) * int32(dt)
	if pilot.Altitude < 500 {
		pilot.Altitude = 500
	}
	if pilot.Altitude > 4000 {
		pilot.Altitude = 4000
	}

	// Случайные изменения вертикальной скорости
	if p.rand.Float64() < 0.2 {
		pilot.ClimbRate = int16(p.rand.Intn(60) - 30)
	}
}

// publishMessage публикует MQTT сообщение согласно FANET протоколу
func (p *TestPublisher) publishMessage(chipID string, pilot *PilotState, packetType int) error {
	// Создание топика в новом формате
	topic := fmt.Sprintf("fb/b/%s/f/%d", chipID, packetType)

	// Создание payload согласно спецификации
	payload, err := p.createPayload(pilot, packetType)
	if err != nil {
		return fmt.Errorf("ошибка создания payload: %w", err)
	}

	// Публикация сообщения
	token := p.client.Publish(topic, 0, false, payload)
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("ошибка публикации в топик %s: %w", topic, token.Error())
	}

	// Логирование для отладки
	fmt.Printf("📡 %s -> %s: %s (pilot %s)\n", 
		chipID, topic, hex.EncodeToString(payload[:min(16, len(payload))]), pilot.DeviceID)

	return nil
}

// createPayload создает FANET payload согласно спецификации
func (p *TestPublisher) createPayload(pilot *PilotState, packetType int) ([]byte, error) {
	now := time.Now()

	// Обертка базовой станции (8 байт)
	wrapper := make([]byte, 8)
	binary.LittleEndian.PutUint32(wrapper[0:4], uint32(now.Unix()))
	binary.LittleEndian.PutUint16(wrapper[4:6], uint16(p.rand.Intn(100)-120)) // RSSI: -120 to -20 dBm
	binary.LittleEndian.PutUint16(wrapper[6:8], uint16(p.rand.Intn(20)-5))    // SNR: -5 to +15 dB

	// FANET пакет
	var fanetData []byte

	// Заголовок (1 байт) + адрес источника (3 байта)
	header := uint8(packetType) // Тип в битах 0-2
	deviceAddr, _ := strconv.ParseUint(pilot.DeviceID, 16, 32)

	fanetData = append(fanetData, header)
	fanetData = append(fanetData, byte(deviceAddr&0xFF))
	fanetData = append(fanetData, byte((deviceAddr>>8)&0xFF))
	fanetData = append(fanetData, byte((deviceAddr>>16)&0xFF))

	// Payload зависит от типа пакета
	switch packetType {
	case 1: // Air Tracking
		payload := p.createAirTrackingPayload(pilot)
		fanetData = append(fanetData, payload...)

	case 2: // Name
		payload := p.createNamePayload(pilot)
		fanetData = append(fanetData, payload...)

	case 4: // Service/Weather
		payload := p.createServicePayload()
		fanetData = append(fanetData, payload...)

	case 7: // Ground Tracking
		payload := p.createGroundTrackingPayload(pilot)
		fanetData = append(fanetData, payload...)

	case 9: // Thermal
		payload := p.createThermalPayload(pilot)
		fanetData = append(fanetData, payload...)

	default:
		return nil, fmt.Errorf("неподдерживаемый тип пакета: %d", packetType)
	}

	// Объединяем обертку и FANET данные
	result := append(wrapper, fanetData...)
	return result, nil
}

// createAirTrackingPayload создает payload для Type 1 (Air Tracking)
func (p *TestPublisher) createAirTrackingPayload(pilot *PilotState) []byte {
	payload := make([]byte, 11) // 11 байт: 6(координаты) + 2(alt_status) + 1(speed) + 1(climb) + 1(heading)

	// Координаты (3 + 3 байта)
	latRaw := int32(pilot.Latitude * 93206.04)
	lonRaw := int32(pilot.Longitude * 46603.02)

	payload[0] = byte(latRaw & 0xFF)
	payload[1] = byte((latRaw >> 8) & 0xFF)
	payload[2] = byte((latRaw >> 16) & 0xFF)

	payload[3] = byte(lonRaw & 0xFF)
	payload[4] = byte((lonRaw >> 8) & 0xFF)
	payload[5] = byte((lonRaw >> 16) & 0xFF)

	// Alt_status (2 байта) - согласно FANET спецификации
	// Bit 15: Online Tracking (1=онлайн, 0=replay)
	// Bits 14-12: Aircraft Type (0-7)
	// Bit 11: Altitude scaling (0=1x, 1=4x)
	// Bits 10-0: Altitude в метрах
	
	var altStatus uint16
	altStatus |= 0x8000 // Bit 15: Online tracking = 1
	altStatus |= uint16(pilot.AircraftType&0x07) << 12 // Bits 14-12: Aircraft type
	
	// Определяем нужно ли 4x scaling для высоты
	altRaw := pilot.Altitude
	if altRaw > 2047 { // Максимум для 11 бит = 2047
		altStatus |= 0x0800 // Bit 11: 4x scaling
		altRaw = altRaw / 4
		if altRaw > 2047 {
			altRaw = 2047 // Ограничиваем максимум
		}
	}
	altStatus |= uint16(altRaw & 0x07FF) // Bits 10-0: высота
	
	binary.LittleEndian.PutUint16(payload[6:8], altStatus)
	
	// Скорость (1 байт) - Byte 8
	// Bit 7: Speed scaling (0=1x, 1=5x)
	// Bits 6-0: Speed в 0.5 км/ч
	var speedByte uint8
	speedVal := pilot.Speed
	if speedVal > 63 { // Максимум для 7 бит в единицах 0.5 км/ч = 31.5 км/ч
		speedByte |= 0x80 // Bit 7: 5x scaling
		speedVal = speedVal / 5
		if speedVal > 63 {
			speedVal = 63
		}
	}
	// Преобразуем км/ч в единицы 0.5 км/ч
	speedByte |= uint8((speedVal * 2) & 0x7F) // Bits 6-0
	payload[8] = speedByte
	
	// Вертикальная скорость (1 байт) - Byte 9
	// Bit 7: Climb scaling (0=1x, 1=5x)
	// Bits 6-0: Climb rate в 0.1 м/с (signed 7-bit)
	var climbByte uint8
	climbVal := pilot.ClimbRate // уже в единицах 0.1 м/с
	if climbVal > 63 || climbVal < -64 { // 7-bit signed range: -64 до +63
		climbByte |= 0x80 // Bit 7: 5x scaling
		climbVal = climbVal / 5
		if climbVal > 63 {
			climbVal = 63
		} else if climbVal < -64 {
			climbVal = -64
		}
	}
	// 7-bit signed: преобразуем в unsigned для хранения
	climbByte |= uint8(climbVal & 0x7F) // Bits 6-0
	payload[9] = climbByte
	
	// Курс (1 байт) - Byte 10
	// 0-255 представляет 0-360°
	payload[10] = byte(float32(pilot.Heading) * 256.0 / 360.0)
	
	// Опциональные поля (не включаем AircraftType отдельно)
	// Тип ВС уже в alt_status

	return payload
}

// createNamePayload создает payload для Type 2 (Name)
func (p *TestPublisher) createNamePayload(pilot *PilotState) []byte {
	name := pilot.Name
	if len(name) > 20 {
		name = name[:20]
	}
	return []byte(name)
}

// createServicePayload создает payload для Type 4 (Service/Weather)
func (p *TestPublisher) createServicePayload() []byte {
	payload := make([]byte, 13)

	// Service Type 0: Weather
	payload[0] = 0

	// Погодные данные согласно спецификации
	windHeading := uint16(p.rand.Intn(360) * 182)
	windSpeed := uint16(p.rand.Intn(15) * 100)    // 0-15 м/с
	windGusts := uint16(windSpeed + uint16(p.rand.Intn(5)*100))
	temperature := int16((p.rand.Intn(40) - 10) * 100) // -10 to +30°C
	humidity := uint8(30 + p.rand.Intn(70))             // 30-100%
	pressure := uint16(p.rand.Intn(100))                // 1000-1100 hPa (offset)
	battery := uint8(20 + p.rand.Intn(80))              // 20-100%

	binary.LittleEndian.PutUint16(payload[1:3], windHeading)
	binary.LittleEndian.PutUint16(payload[3:5], windSpeed)
	binary.LittleEndian.PutUint16(payload[5:7], windGusts)
	binary.LittleEndian.PutUint16(payload[7:9], uint16(temperature))
	payload[9] = humidity
	binary.LittleEndian.PutUint16(payload[10:12], pressure)
	payload[12] = battery

	return payload
}

// createGroundTrackingPayload создает payload для Type 7 (Ground Tracking)
func (p *TestPublisher) createGroundTrackingPayload(pilot *PilotState) []byte {
	// Упрощенная версия Air Tracking без climb rate
	payload := make([]byte, 11)

	// Координаты (аналогично Type 1)
	latRaw := int32(pilot.Latitude * 93206.04)
	lonRaw := int32(pilot.Longitude * 46603.02)

	payload[0] = byte(latRaw & 0xFF)
	payload[1] = byte((latRaw >> 8) & 0xFF)
	payload[2] = byte((latRaw >> 16) & 0xFF)

	payload[3] = byte(lonRaw & 0xFF)
	payload[4] = byte((lonRaw >> 8) & 0xFF)
	payload[5] = byte((lonRaw >> 16) & 0xFF)

	// Высота (2 байта)
	altRaw := uint16(pilot.Altitude - 1000)
	binary.LittleEndian.PutUint16(payload[6:8], altRaw)

	// Скорость (1 байт)
	payload[8] = byte(pilot.Speed * 2)

	// Курс (1 байт)
	payload[9] = byte(float32(pilot.Heading) * 256.0 / 360.0)

	// Тип объекта (1 байт) - 0 для наземного
	payload[10] = 0

	return payload
}

// createThermalPayload создает payload для Type 9 (Thermal)
func (p *TestPublisher) createThermalPayload(pilot *PilotState) []byte {
	payload := make([]byte, 13)

	// Координаты центра термика (аналогично Type 1)
	latRaw := int32(pilot.Latitude * 93206.04)
	lonRaw := int32(pilot.Longitude * 46603.02)

	payload[0] = byte(latRaw & 0xFF)
	payload[1] = byte((latRaw >> 8) & 0xFF)
	payload[2] = byte((latRaw >> 16) & 0xFF)

	payload[3] = byte(lonRaw & 0xFF)
	payload[4] = byte((lonRaw >> 8) & 0xFF)
	payload[5] = byte((lonRaw >> 16) & 0xFF)

	// Высота термика (2 байта) - без offset
	binary.LittleEndian.PutUint16(payload[6:8], uint16(pilot.Altitude))

	// Качество термика (1 байт): 0-5
	payload[8] = uint8(p.rand.Intn(6))

	// Средний подъем (2 байта): м/с * 100
	avgClimb := int16(100 + p.rand.Intn(400)) // 1-5 м/с
	binary.LittleEndian.PutUint16(payload[9:11], uint16(avgClimb))

	// Ветер (4 байта) - не входит в ThermalData согласно спецификации
	// Но добавляем для полноты пакета
	windSpeed := uint16(p.rand.Intn(10) * 100)  // 0-10 м/с
	windHeading := uint16(p.rand.Intn(360) * 182)
	binary.LittleEndian.PutUint16(payload[11:13], windSpeed)
	// Сокращаем до 13 байт, так как windHeading не помещается
	_ = windHeading

	return payload
}

// Вспомогательные функции

func parseStringSlice(s string) []string {
	if s == "" {
		return []string{}
	}
	return strings.Split(s, ",")
}

func parseIntSlice(s string) []int {
	if s == "" {
		return []int{}
	}
	strs := strings.Split(s, ",")
	ints := make([]int, len(strs))
	for i, str := range strs {
		val, err := strconv.Atoi(strings.TrimSpace(str))
		if err != nil {
			log.Fatalf("Ошибка парсинга числа '%s': %v", str, err)
		}
		ints[i] = val
	}
	return ints
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

