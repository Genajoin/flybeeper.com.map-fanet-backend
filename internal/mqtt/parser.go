package mqtt

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/flybeeper/fanet-backend/pkg/utils"
)

// FANETMessage представляет распарсенное FANET сообщение
type FANETMessage struct {
	Type        uint8               `json:"type"`         // Тип сообщения (0=ACK, 1=Air tracking, 2=Name, 4=Service, 7=Ground tracking, 8=HW Info, 9=Thermal)
	DeviceID    string              `json:"device_id"`    // ID устройства (24-bit адрес)
	ChipID      string              `json:"chip_id"`      // ID базовой станции (из топика)
	PacketType  string              `json:"packet_type"`  // Тип пакета из топика для дополнительной валидации
	Timestamp   time.Time           `json:"timestamp"`    // Время от базовой станции
	RSSI        int16               `json:"rssi"`         // Уровень сигнала (dBm)
	SNR         int16               `json:"snr"`          // Signal-to-Noise Ratio (dB)
	RawPayload  []byte              `json:"raw_payload"`  // Исходные FANET данные
	Data        interface{}         `json:"data"`         // Распарсенные данные зависящие от типа
}

// AirTrackingData данные отслеживания в воздухе (Type 1)
type AirTrackingData struct {
	Latitude    float64 `json:"latitude"`     // Широта
	Longitude   float64 `json:"longitude"`    // Долгота
	Altitude    int32   `json:"altitude"`     // Высота в метрах
	Speed       uint16  `json:"speed"`        // Скорость в км/ч
	Heading     uint16  `json:"heading"`      // Направление в градусах
	ClimbRate   int16   `json:"climb_rate"`   // Скорость набора высоты в м/с * 10
	TurnRate    int16   `json:"turn_rate"`    // Скорость поворота в град/с * 10
	AircraftType uint8  `json:"aircraft_type"` // Тип ВС (1=Paraglider, 2=Hangglider, etc)
	OnlineTracking bool  `json:"online_tracking"` // Онлайн трекинг (bit 15 в alt_status)
}

// NameData данные имени (Type 2)
type NameData struct {
	Name string `json:"name"` // Имя пилота/устройства (UTF-8, max 64 символа)
}

// ServiceData данные сервиса/погоды (Type 4)
type ServiceData struct {
	ServiceHeader uint8       `json:"service_header"` // Битовые флаги сервиса
	Latitude      float64     `json:"latitude"`       // Широта станции
	Longitude     float64     `json:"longitude"`      // Долгота станции
	Data          interface{} `json:"data"`           // Дополнительные данные согласно флагам
}

// WeatherData погодные данные (согласно битовым флагам Type 4)
type WeatherData struct {
	Temperature   float32 `json:"temperature"`    // Температура в °C (если флаг bit 6)
	WindSpeed     float32 `json:"wind_speed"`     // Скорость ветра в км/ч (если флаг bit 5)
	WindDirection uint16  `json:"wind_direction"` // Направление ветра в градусах (если флаг bit 5)
	WindGusts     float32 `json:"wind_gusts"`     // Порывы ветра в км/ч (если флаг bit 5)
	Humidity      uint8   `json:"humidity"`       // Влажность в % (если флаг bit 4)
	Pressure      float32 `json:"pressure"`       // Давление в hPa (если флаг bit 3)
	Battery       uint8   `json:"battery"`        // Заряд батареи в % (если флаг bit 1)
}

// GroundTrackingData данные наземного отслеживания (Type 7)
type GroundTrackingData struct {
	Latitude  float64 `json:"latitude"`  // Широта
	Longitude float64 `json:"longitude"` // Долгота
	Altitude  int32   `json:"altitude"`  // Высота в метрах
	Speed     uint16  `json:"speed"`     // Скорость в км/ч
	Heading   uint16  `json:"heading"`   // Направление в градусах
}

// ThermalData данные термика (Type 9)
type ThermalData struct {
	Latitude    float64 `json:"latitude"`     // Широта центра термика
	Longitude   float64 `json:"longitude"`    // Долгота центра термика
	Altitude    int32   `json:"altitude"`     // Высота термика в метрах
	ClimbRate   int16   `json:"climb_rate"`   // Скорость подъема в м/с * 10
	Strength    uint8   `json:"strength"`     // Сила термика (0-100)
	Radius      uint16  `json:"radius"`       // Радиус термика в метрах
}

// HWInfoData данные информации об устройстве (Type 8, deprecated)
type HWInfoData struct {
	Manufacturer uint8  `json:"manufacturer"` // Производитель устройства
	DeviceType   uint8  `json:"device_type"`  // Тип устройства
}

// NewHWInfoData данные информации об устройстве (Type 10, новая версия)
type NewHWInfoData struct {
	Manufacturer uint8  `json:"manufacturer"` // Производитель устройства
	DeviceType   uint8  `json:"device_type"`  // Тип устройства
}

// Parser парсер FANET сообщений
type Parser struct {
	logger *utils.Logger
}

// NewParser создает новый парсер FANET сообщений
func NewParser(logger *utils.Logger) *Parser {
	return &Parser{
		logger: logger,
	}
}

// Parse парсит MQTT сообщение и извлекает FANET данные
func (p *Parser) Parse(topic string, payload []byte) (*FANETMessage, error) {
	// Извлекаем информацию из топика: fb/b/{chip_id}/f/{packet_type}
	parts := strings.Split(topic, "/")
	if len(parts) != 5 || parts[0] != "fb" || parts[1] != "b" || parts[3] != "f" {
		return nil, fmt.Errorf("invalid topic format: %s", topic)
	}
	
	chipID := parts[2]
	packetType := parts[4] // packet_type из топика
	
	// Проверяем минимальный размер (обертка базовой станции + заголовок FANET)
	if len(payload) < 12 {
		return nil, fmt.Errorf("payload too short: %d bytes", len(payload))
	}
	
	// Декодируем обертку базовой станции согласно спецификации
	timestamp := int64(binary.LittleEndian.Uint32(payload[0:4]))
	rssi := int16(binary.LittleEndian.Uint16(payload[4:6]))
	snr := int16(binary.LittleEndian.Uint16(payload[6:8]))
	fanetData := payload[8:]
	
	// Проверяем минимальный размер FANET пакета (заголовок + адрес)
	if len(fanetData) < 4 {
		return nil, fmt.Errorf("FANET packet too short: %d bytes", len(fanetData))
	}
	
	// Извлекаем заголовок FANET (1 байт)
	header := fanetData[0]
	msgType := header & 0x3F  // Биты 5-0: тип пакета (6 бит)
	
	// Извлекаем адрес источника (3 байта, little-endian)
	deviceAddr := uint32(fanetData[1]) | uint32(fanetData[2])<<8 | uint32(fanetData[3])<<16
	deviceID := fmt.Sprintf("%06X", deviceAddr)
	
	// Логирование типов для отладки и мягкая валидация
	expectedType := fmt.Sprintf("%d", msgType)
	if packetType != expectedType {
		// Не блокируем обработку, но логируем несоответствие для анализа
		p.logger.WithField("topic_type", packetType).WithField("fanet_type", msgType).WithField("device_id", deviceID).
			Info("Packet type mismatch between topic and FANET header (this is normal for some packet types)")
	} else {
		p.logger.WithField("topic_type", packetType).WithField("fanet_type", msgType).WithField("device_id", deviceID).
			Debug("Processing FANET packet")
	}

	msg := &FANETMessage{
		Type:       msgType,
		DeviceID:   deviceID,
		ChipID:     chipID,
		PacketType: packetType,
		Timestamp:  time.Unix(timestamp, 0).UTC(),
		RSSI:       rssi,
		SNR:        snr,
		RawPayload: fanetData,
	}
	
	// Парсим данные в зависимости от типа сообщения
	if len(fanetData) > 4 {
		data := fanetData[4:] // Пропускаем заголовок (1 байт) + адрес (3 байта)
		
		switch msgType {
		case 0: // ACK - подтверждение получения пакета
			p.logger.WithField("device_id", deviceID).Debug("Received ACK packet")
			// ACK пакеты не содержат полезных данных для клиентов, но логируем их
			
		case 1: // Air tracking
			if parsed, err := p.parseAirTracking(data); err == nil {
				msg.Data = parsed
			} else {
				p.logger.WithField("error", err).WithField("device_id", deviceID).Warn("Failed to parse air tracking data")
			}
			
		case 2: // Name
			if parsed, err := p.parseName(data); err == nil {
				msg.Data = parsed
			} else {
				p.logger.WithField("error", err).WithField("device_id", deviceID).Warn("Failed to parse name data")
			}
			
		case 4: // Service/Weather
			if parsed, err := p.parseService(data); err == nil {
				msg.Data = parsed
			} else {
				p.logger.WithField("error", err).WithField("device_id", deviceID).Warn("Failed to parse service data")
			}
			
		case 7: // Ground tracking
			if parsed, err := p.parseGroundTracking(data); err == nil {
				msg.Data = parsed
			} else {
				p.logger.WithField("error", err).WithField("device_id", deviceID).Warn("Failed to parse ground tracking data")
			}
			
		case 8: // HW Info (deprecated)
			if parsed, err := p.parseHWInfo(data); err == nil {
				msg.Data = parsed
				p.logger.WithField("device_id", deviceID).Debug("Received deprecated HW Info packet")
			} else {
				p.logger.WithField("error", err).WithField("device_id", deviceID).Warn("Failed to parse HW info data")
			}
			
		case 9: // Thermal
			if parsed, err := p.parseThermal(data); err == nil {
				msg.Data = parsed
			} else {
				p.logger.WithField("error", err).WithField("device_id", deviceID).Warn("Failed to parse thermal data")
			}
			
		case 10: // New HW Info (0xA)
			if parsed, err := p.parseNewHWInfo(data); err == nil {
				msg.Data = parsed
				p.logger.WithField("device_id", deviceID).Debug("Received new HW Info packet")
			} else {
				p.logger.WithField("error", err).WithField("device_id", deviceID).Warn("Failed to parse new HW info data")
			}
			
		default:
			p.logger.WithField("type", msgType).WithField("device_id", deviceID).Debug("Unsupported FANET message type")
			// Не возвращаем nil для неподдерживаемых типов - позволяем им проходить дальше
		}
	}
	
	return msg, nil
}

// parseAirTracking парсит данные отслеживания в воздухе (Type 1) согласно спецификации
func (p *Parser) parseAirTracking(data []byte) (*AirTrackingData, error) {
	if len(data) < 11 {
		return nil, fmt.Errorf("air tracking data too short: %d bytes", len(data))
	}
	
	// Координаты (3 + 3 байта) - согласно спецификации
	// Latitude: deg * 93206.04, signed 24-bit
	latRaw := int32(data[0]) | int32(data[1])<<8 | int32(data[2])<<16
	if latRaw&0x800000 != 0 { // Знаковое расширение для 24-bit
		latRaw |= -16777216  // 0xFF000000 как отрицательное int32
	}
	latitude := float64(latRaw) / 93206.04
	
	// Longitude: deg * 46603.02, signed 24-bit
	lonRaw := int32(data[3]) | int32(data[4])<<8 | int32(data[5])<<16
	if lonRaw&0x800000 != 0 { // Знаковое расширение для 24-bit
		lonRaw |= -16777216  // 0xFF000000 как отрицательное int32
	}
	longitude := float64(lonRaw) / 46603.02
	
	// Alt_status (2 байта) - биты 6-7
	altStatus := binary.LittleEndian.Uint16(data[6:8])
	
	// Извлекаем поля согласно спецификации
	onlineTracking := (altStatus & 0x8000) != 0  // Bit 15: Online Tracking flag
	aircraftType := uint8((altStatus >> 12) & 0x07)  // Bits 14-12: Aircraft Type
	altScale := (altStatus & 0x0800) != 0  // Bit 11: altitude scaling
	altRaw := int32(altStatus & 0x07FF)  // Bits 10-0: altitude в метрах
	
	// Применяем масштабирование высоты
	var altitude int32
	if altScale {
		altitude = altRaw * 4
	} else {
		altitude = altRaw
	}
	
	// Скорость (1 байт) - байт 8
	speedRaw := data[8]
	speedScale := (speedRaw & 0x80) != 0  // Bit 7: speed scaling
	speedVal := float32(speedRaw & 0x7F)  // Bits 6-0
	
	var speed uint16
	if speedScale {
		speed = uint16(speedVal * 5 * 0.5)  // 5x scaling, единицы 0.5 км/ч
	} else {
		speed = uint16(speedVal * 0.5)
	}
	
	// Вертикальная скорость (1 байт) - байт 9
	climbRaw := data[9]
	climbScale := (climbRaw & 0x80) != 0  // Bit 7: climb scaling
	climbVal := int8(climbRaw & 0x7F)  // Bits 6-0 (signed 7-bit)
	
	// Знаковое расширение для 7-битного signed значения
	if climbVal&0x40 != 0 {
		climbVal |= -128  // 0x80 как отрицательное int8
	}
	
	var climbRate int16
	if climbScale {
		climbRate = int16(climbVal) * 5  // 5x scaling, единицы 0.1 м/с -> результат в 0.1 м/с
	} else {
		climbRate = int16(climbVal)  // единицы 0.1 м/с
	}
	
	// Курс (1 байт) - байт 10
	headingRaw := data[10]
	heading := uint16(float32(headingRaw) * 360.0 / 256.0)
	
	tracking := &AirTrackingData{
		Latitude:       latitude,
		Longitude:      longitude,
		Altitude:       altitude,
		Speed:          speed,
		Heading:        heading,
		ClimbRate:      climbRate,
		AircraftType:   aircraftType,   // Извлечено из alt_status bits 14-12
		OnlineTracking: onlineTracking, // Извлечено из alt_status bit 15
	}
	
	return tracking, nil
}

// parseName парсит данные имени (Type 2)
func (p *Parser) parseName(data []byte) (*NameData, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("name data is empty")
	}
	
	// Разумное ограничение для имени (увеличено до 64 символов для совместимости)
	if len(data) > 64 {
		p.logger.WithField("length", len(data)).Warn("Name too long, truncating to 64 bytes")
		data = data[:64]
	}
	
	// Имя в UTF-8 кодировке
	name := string(data)
	
	// Убираем null-терминаторы если есть
	name = strings.TrimRight(name, "\x00")
	
	return &NameData{
		Name: name,
	}, nil
}

// parseService парсит сервисные данные (Type 4) согласно новой спецификации
func (p *Parser) parseService(data []byte) (*ServiceData, error) {
	if len(data) < 7 {
		return nil, fmt.Errorf("service data too short: %d bytes (минимум 7: header + координаты)", len(data))
	}
	
	// Извлекаем service header с битовыми флагами
	serviceHeader := data[0]
	
	// Извлекаем координаты станции (обязательные для Type 4)
	latRaw := int32(data[1]) | int32(data[2])<<8 | int32(data[3])<<16
	if latRaw&0x800000 != 0 { // Знаковое расширение для 24-bit
		latRaw |= ^0xFFFFFF
	}
	latitude := float64(latRaw) / 93206.04
	
	lonRaw := int32(data[4]) | int32(data[5])<<8 | int32(data[6])<<16
	if lonRaw&0x800000 != 0 {
		lonRaw |= ^0xFFFFFF
	}
	longitude := float64(lonRaw) / 46603.02
	
	service := &ServiceData{
		ServiceHeader: serviceHeader,
		Latitude:      latitude,
		Longitude:     longitude,
	}
	
	// Парсим дополнительные данные согласно флагам в service header
	weather := &WeatherData{}
	offset := 7
	hasWeatherData := false
	
	// Bit 6: Temperature
	if serviceHeader&0x40 != 0 && offset < len(data) {
		tempRaw := int8(data[offset])
		weather.Temperature = float32(tempRaw) / 2.0 // °C
		offset++
		hasWeatherData = true
	}
	
	// Bit 5: Wind (3 байта)
	if serviceHeader&0x20 != 0 && offset+2 < len(data) {
		windDir := data[offset]
		weather.WindDirection = uint16(windDir) * 360 / 256
		
		// Wind speed и gusts в следующих байтах (более сложная структура)
		// Упрощенная реализация
		windSpeed := data[offset+1]
		windGusts := data[offset+2]
		weather.WindSpeed = float32(windSpeed) * 0.2 // 0.2 км/ч units
		weather.WindGusts = float32(windGusts) * 0.2
		offset += 3
		hasWeatherData = true
	}
	
	// Bit 4: Humidity
	if serviceHeader&0x10 != 0 && offset < len(data) {
		humRaw := data[offset]
		weather.Humidity = humRaw / 4 // %RH * 4
		offset++
		hasWeatherData = true
	}
	
	// Bit 3: Barometric pressure (2 байта)
	if serviceHeader&0x08 != 0 && offset+1 < len(data) {
		pressureRaw := uint16(data[offset]) | uint16(data[offset+1])<<8
		weather.Pressure = (float32(pressureRaw) / 10.0) + 430.0 // hPa
		offset += 2
		hasWeatherData = true
	}
	
	// Bit 1: State of Charge (battery)
	if serviceHeader&0x02 != 0 && offset < len(data) {
		batteryRaw := data[offset] & 0x0F // Младшие 4 бита
		weather.Battery = uint8(batteryRaw) * 100 / 15 // 0x0-0xF -> 0-100%
		offset++
		hasWeatherData = true
	}
	
	if hasWeatherData {
		service.Data = weather
	}
	
	return service, nil
}


// parseGroundTracking парсит данные наземного отслеживания (Type 7)
func (p *Parser) parseGroundTracking(data []byte) (*GroundTrackingData, error) {
	if len(data) < 10 {
		return nil, fmt.Errorf("ground tracking data too short: %d bytes", len(data))
	}
	
	// Координаты (3 + 3 байта) - аналогично Air tracking
	// Latitude: deg * 93206.04, signed 24-bit
	latRaw := int32(data[0]) | int32(data[1])<<8 | int32(data[2])<<16
	if latRaw&0x800000 != 0 { // Знаковое расширение для 24-bit
		latRaw |= ^0xFFFFFF
	}
	latitude := float64(latRaw) / 93206.04
	
	// Longitude: deg * 46603.02, signed 24-bit
	lonRaw := int32(data[3]) | int32(data[4])<<8 | int32(data[5])<<16
	if lonRaw&0x800000 != 0 { // Знаковое расширение для 24-bit
		lonRaw |= ^0xFFFFFF
	}
	longitude := float64(lonRaw) / 46603.02
	
	altitude := int32(int16(binary.LittleEndian.Uint16(data[6:8])))
	
	speedHeading := binary.LittleEndian.Uint16(data[8:10])
	speed := uint16((speedHeading >> 6) & 0x3FF)
	heading := uint16(speedHeading & 0x3F) * 6
	
	return &GroundTrackingData{
		Latitude:  latitude,
		Longitude: longitude,
		Altitude:  altitude,
		Speed:     speed,
		Heading:   heading,
	}, nil
}

// parseThermal парсит данные термика (Type 9) согласно спецификации
func (p *Parser) parseThermal(data []byte) (*ThermalData, error) {
	if len(data) < 13 {
		return nil, fmt.Errorf("thermal data too short: %d bytes", len(data))
	}
	
	// Координаты центра термика (3 + 3 байта) - аналогично Type 1
	latRaw := int32(data[0]) | int32(data[1])<<8 | int32(data[2])<<16
	if latRaw&0x800000 != 0 {
		latRaw |= ^0xFFFFFF
	}
	latitude := float64(latRaw) / 93206.04
	
	lonRaw := int32(data[3]) | int32(data[4])<<8 | int32(data[5])<<16
	if lonRaw&0x800000 != 0 {
		lonRaw |= ^0xFFFFFF
	}
	longitude := float64(lonRaw) / 46603.02
	
	// Высота (2 байта) - без offset для термиков
	altitude := int32(binary.LittleEndian.Uint16(data[6:8]))
	
	// Качество термика (1 байт): 0-5
	quality := data[8]
	
	thermal := &ThermalData{
		Latitude:  latitude,
		Longitude: longitude,
		Altitude:  altitude,
		Strength:  quality,
	}
	
	// Дополнительные параметры согласно спецификации
	if len(data) >= 11 {
		// Средний подъем (2 байта): м/с * 100
		avgClimb := int16(binary.LittleEndian.Uint16(data[9:11]))
		thermal.ClimbRate = avgClimb // уже * 100
	}
	
	// Ветер не входит в ThermalData структуру согласно спецификации
	
	return thermal, nil
}

// parseHWInfo парсит данные информации об устройстве (Type 8, deprecated)
func (p *Parser) parseHWInfo(data []byte) (*HWInfoData, error) {
	if len(data) < 1 {
		return nil, fmt.Errorf("HW info data too short: %d bytes", len(data))
	}
	
	// Согласно спецификации, Type 8 содержит информацию об устройстве
	// Byte 0: Device/Instrument Type 
	deviceType := data[0]
	manufacturer := uint8(0) // По умолчанию неизвестно
	
	// Попытка извлечь производителя из старших битов (если есть дополнительные данные)
	if len(data) >= 2 {
		manufacturer = data[1]
	}
	
	return &HWInfoData{
		Manufacturer: manufacturer,
		DeviceType:   deviceType,
	}, nil
}

// parseNewHWInfo парсит данные информации об устройстве (Type 10, новая версия)
func (p *Parser) parseNewHWInfo(data []byte) (*NewHWInfoData, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("new HW info data too short: %d bytes", len(data))
	}
	
	// Согласно спецификации Type 10
	// Byte 0: Manufacturer
	// Byte 1: Device Type
	manufacturer := data[0]
	deviceType := data[1]
	
	return &NewHWInfoData{
		Manufacturer: manufacturer,
		DeviceType:   deviceType,
	}, nil
}

// ValidateCoordinates проверяет валидность координат
func (p *Parser) ValidateCoordinates(lat, lon float64) bool {
	return lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180
}

// CalculateDistance вычисляет расстояние между двумя точками в метрах
func (p *Parser) CalculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000 // Радиус Земли в метрах
	
	lat1Rad := lat1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	deltaLat := (lat2 - lat1) * math.Pi / 180
	deltaLon := (lon2 - lon1) * math.Pi / 180
	
	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
		math.Sin(deltaLon/2)*math.Sin(deltaLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	
	return R * c
}