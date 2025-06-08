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
	Type        uint8               `json:"type"`         // Тип сообщения (1=Air tracking, 2=Name, 4=Service, 7=Ground tracking, 9=Thermal)
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
}

// NameData данные имени (Type 2)
type NameData struct {
	Name string `json:"name"` // Имя пилота/устройства (UTF-8, max 20 символов)
}

// ServiceData данные сервиса/погоды (Type 4)
type ServiceData struct {
	ServiceType uint8       `json:"service_type"` // Тип сервиса
	Data        interface{} `json:"data"`         // Данные зависящие от типа сервиса
}

// WeatherData погодные данные (Service Type 1)
type WeatherData struct {
	WindSpeed     uint8   `json:"wind_speed"`     // Скорость ветра в км/ч
	WindDirection uint16  `json:"wind_direction"` // Направление ветра в градусах
	Temperature   int8    `json:"temperature"`    // Температура в °C
	Humidity      uint8   `json:"humidity"`       // Влажность в %
	Pressure      uint16  `json:"pressure"`       // Давление в hPa
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
	msgType := header & 0x07  // Биты 0-2: тип пакета
	
	// Извлекаем адрес источника (3 байта, little-endian)
	deviceAddr := uint32(fanetData[1]) | uint32(fanetData[2])<<8 | uint32(fanetData[3])<<16
	deviceID := fmt.Sprintf("%06X", deviceAddr)
	
	// Валидация соответствия packet_type из топика и FANET заголовка
	if expectedType := fmt.Sprintf("%d", msgType); packetType != expectedType {
		return nil, fmt.Errorf("packet type mismatch: topic has %s, FANET header has %d", packetType, msgType)
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
			
		case 9: // Thermal
			if parsed, err := p.parseThermal(data); err == nil {
				msg.Data = parsed
			} else {
				p.logger.WithField("error", err).WithField("device_id", deviceID).Warn("Failed to parse thermal data")
			}
			
		default:
			p.logger.WithField("type", msgType).WithField("device_id", deviceID).Debug("Unsupported FANET message type")
			return nil, nil // Не ошибка, просто неподдерживаемый тип
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
		latRaw |= ^0xFFFFFF
	}
	latitude := float64(latRaw) / 93206.04
	
	// Longitude: deg * 46603.02, signed 24-bit
	lonRaw := int32(data[3]) | int32(data[4])<<8 | int32(data[5])<<16
	if lonRaw&0x800000 != 0 { // Знаковое расширение для 24-bit
		lonRaw |= ^0xFFFFFF
	}
	longitude := float64(lonRaw) / 46603.02
	
	// Высота (2 байта): (altitude - 1000) метров
	altitudeRaw := binary.LittleEndian.Uint16(data[6:8])
	altitude := int32(altitudeRaw) + 1000
	
	// Скорость (1 байт): km/h * 2
	speedRaw := data[8]
	speed := uint16(speedRaw) / 2
	
	// Вертикальная скорость (1 байт): (climb * 10) + 128
	climbRaw := data[9]
	climbRate := int16(climbRaw) - 128 // м/с * 10
	
	// Курс (1 байт): degrees * 256 / 360
	headingRaw := data[10]
	heading := uint16(float32(headingRaw) * 360.0 / 256.0)
	
	tracking := &AirTrackingData{
		Latitude:  latitude,
		Longitude: longitude,
		Altitude:  altitude,
		Speed:     speed,
		Heading:   heading,
		ClimbRate: climbRate,
	}
	
	// Тип летательного аппарата (опционально)
	if len(data) >= 12 {
		tracking.AircraftType = data[11]
	}
	
	return tracking, nil
}

// parseName парсит данные имени (Type 2)
func (p *Parser) parseName(data []byte) (*NameData, error) {
	if len(data) == 0 || len(data) > 20 {
		return nil, fmt.Errorf("invalid name length: %d bytes", len(data))
	}
	
	// Имя в UTF-8 кодировке
	name := string(data)
	
	return &NameData{
		Name: name,
	}, nil
}

// parseService парсит сервисные данные (Type 4)
func (p *Parser) parseService(data []byte) (*ServiceData, error) {
	if len(data) < 1 {
		return nil, fmt.Errorf("service data too short: %d bytes", len(data))
	}
	
	serviceType := data[0]
	service := &ServiceData{
		ServiceType: serviceType,
	}
	
	switch serviceType {
	case 1: // Weather data
		if weather, err := p.parseWeather(data[1:]); err == nil {
			service.Data = weather
		} else {
			return nil, fmt.Errorf("failed to parse weather data: %w", err)
		}
	default:
		// Неизвестный тип сервиса, сохраняем как есть
		service.Data = data[1:]
	}
	
	return service, nil
}

// parseWeather парсит погодные данные согласно спецификации (Service Type 0)
func (p *Parser) parseWeather(data []byte) (*WeatherData, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("weather data too short: %d bytes", len(data))
	}
	
	// Согласно спецификации Service Type 0: Weather Station
	windHeading := binary.LittleEndian.Uint16(data[0:2])      // * 182
	windSpeed := binary.LittleEndian.Uint16(data[2:4])        // * 100 (м/с)
	_ = binary.LittleEndian.Uint16(data[4:6])        // * 100 (м/с) - не используется
	temperature := int16(binary.LittleEndian.Uint16(data[6:8])) // * 100 (°C)
	humidity := data[8]                                        // %
	pressure := binary.LittleEndian.Uint16(data[9:11])        // - 1000 (гПа)
	_ = uint8(0)                                        // По умолчанию - не используется
	
	// Удаляем неиспользуемую переменную battery
	
	weather := &WeatherData{
		WindDirection: uint16(float64(windHeading) / 182.0),           // градусы
		WindSpeed:     uint8(float64(windSpeed) / 100.0 * 3.6),       // км/ч (м/с -> км/ч)
		Temperature:   int8(float64(temperature) / 100.0),            // °C
		Humidity:      humidity,                                       // %
		Pressure:      pressure + 1000,                               // гПа
	}
	
	return weather, nil
}

// parseGroundTracking парсит данные наземного отслеживания (Type 7)
func (p *Parser) parseGroundTracking(data []byte) (*GroundTrackingData, error) {
	if len(data) < 10 {
		return nil, fmt.Errorf("ground tracking data too short: %d bytes", len(data))
	}
	
	// Аналогично Air tracking, но без climb rate и turn rate
	latRaw := int32(binary.LittleEndian.Uint32(data[0:3]) << 8) >> 8
	lonRaw := int32(binary.LittleEndian.Uint32(data[3:6]) << 8) >> 8
	
	latitude := float64(latRaw) / 93206.0
	longitude := float64(lonRaw) / 46603.0
	
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