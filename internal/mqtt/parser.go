package mqtt

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/flybeeper/fanet-backend/pkg/utils"
)

// FANETMessage представляет распарсенное FANET сообщение
type FANETMessage struct {
	Type        uint8               `json:"type"`         // Тип сообщения (1=Air tracking, 4=Service, 7=Ground tracking, 9=Thermal)
	DeviceID    string              `json:"device_id"`    // ID устройства (из топика)
	Region      string              `json:"region"`       // Регион (из топика)
	Timestamp   time.Time           `json:"timestamp"`    // Время получения
	RawPayload  []byte              `json:"raw_payload"`  // Исходные данные
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
	// Извлекаем информацию из топика: fb/b/+/f -> fb/b/{region}/{device_id}/f
	parts := strings.Split(topic, "/")
	if len(parts) != 4 || parts[0] != "fb" || parts[1] != "b" || parts[3] != "f" {
		return nil, fmt.Errorf("invalid topic format: %s", topic)
	}
	
	region := parts[2]
	
	// Payload должен быть в hex формате
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty payload")
	}
	
	// Декодируем hex payload
	hexStr := string(payload)
	rawData, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode hex payload: %w", err)
	}
	
	if len(rawData) < 4 {
		return nil, fmt.Errorf("payload too short: %d bytes", len(rawData))
	}
	
	// Извлекаем заголовок FANET
	header := binary.LittleEndian.Uint32(rawData[:4])
	
	// Извлекаем поля заголовка
	deviceAddr := (header >> 8) & 0xFFFFFF  // 24 бита адреса устройства
	msgType := uint8(header & 0xFF)         // 8 бит типа сообщения
	
	deviceID := fmt.Sprintf("%06X", deviceAddr)
	
	msg := &FANETMessage{
		Type:       msgType,
		DeviceID:   deviceID,
		Region:     region,
		Timestamp:  time.Now().UTC(),
		RawPayload: rawData,
	}
	
	// Парсим данные в зависимости от типа сообщения
	if len(rawData) > 4 {
		data := rawData[4:] // Пропускаем заголовок
		
		switch msgType {
		case 1: // Air tracking
			if parsed, err := p.parseAirTracking(data); err == nil {
				msg.Data = parsed
			} else {
				p.logger.Warn("Failed to parse air tracking data", "error", err, "device_id", deviceID)
			}
			
		case 4: // Service/Weather
			if parsed, err := p.parseService(data); err == nil {
				msg.Data = parsed
			} else {
				p.logger.Warn("Failed to parse service data", "error", err, "device_id", deviceID)
			}
			
		case 7: // Ground tracking
			if parsed, err := p.parseGroundTracking(data); err == nil {
				msg.Data = parsed
			} else {
				p.logger.Warn("Failed to parse ground tracking data", "error", err, "device_id", deviceID)
			}
			
		case 9: // Thermal
			if parsed, err := p.parseThermal(data); err == nil {
				msg.Data = parsed
			} else {
				p.logger.Warn("Failed to parse thermal data", "error", err, "device_id", deviceID)
			}
			
		default:
			p.logger.Debug("Unsupported FANET message type", "type", msgType, "device_id", deviceID)
			return nil, nil // Не ошибка, просто неподдерживаемый тип
		}
	}
	
	return msg, nil
}

// parseAirTracking парсит данные отслеживания в воздухе (Type 1)
func (p *Parser) parseAirTracking(data []byte) (*AirTrackingData, error) {
	if len(data) < 11 {
		return nil, fmt.Errorf("air tracking data too short: %d bytes", len(data))
	}
	
	// Координаты (6 байт)
	latRaw := int32(binary.LittleEndian.Uint32(data[0:3]) << 8) >> 8  // 24-битное число со знаком
	lonRaw := int32(binary.LittleEndian.Uint32(data[3:6]) << 8) >> 8  // 24-битное число со знаком
	
	latitude := float64(latRaw) / 93206.0   // Преобразование в градусы
	longitude := float64(lonRaw) / 46603.0  // Преобразование в градусы
	
	// Высота (2 байта)
	altitude := int32(int16(binary.LittleEndian.Uint16(data[6:8]))) // Высота в метрах со знаком
	
	// Скорость и направление (2 байта)
	speedHeading := binary.LittleEndian.Uint16(data[8:10])
	speed := uint16((speedHeading >> 6) & 0x3FF)  // 10 бит скорости
	heading := uint16(speedHeading & 0x3F) * 6    // 6 бит направления * 6 градусов
	
	tracking := &AirTrackingData{
		Latitude:  latitude,
		Longitude: longitude,
		Altitude:  altitude,
		Speed:     speed,
		Heading:   heading,
	}
	
	// Дополнительные поля если есть
	if len(data) >= 13 {
		tracking.ClimbRate = int16(binary.LittleEndian.Uint16(data[10:12]))
	}
	if len(data) >= 15 {
		tracking.TurnRate = int16(binary.LittleEndian.Uint16(data[12:14]))
	}
	if len(data) >= 16 {
		tracking.AircraftType = data[14]
	}
	
	return tracking, nil
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

// parseWeather парсит погодные данные
func (p *Parser) parseWeather(data []byte) (*WeatherData, error) {
	if len(data) < 7 {
		return nil, fmt.Errorf("weather data too short: %d bytes", len(data))
	}
	
	weather := &WeatherData{
		WindSpeed:     data[0],                                          // км/ч
		WindDirection: binary.LittleEndian.Uint16(data[1:3]),           // градусы
		Temperature:   int8(data[3]),                                    // °C
		Humidity:      data[4],                                          // %
		Pressure:      binary.LittleEndian.Uint16(data[5:7]) + 850,     // hPa (offset 850)
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

// parseThermal парсит данные термика (Type 9)
func (p *Parser) parseThermal(data []byte) (*ThermalData, error) {
	if len(data) < 10 {
		return nil, fmt.Errorf("thermal data too short: %d bytes", len(data))
	}
	
	// Координаты центра термика
	latRaw := int32(binary.LittleEndian.Uint32(data[0:3]) << 8) >> 8
	lonRaw := int32(binary.LittleEndian.Uint32(data[3:6]) << 8) >> 8
	
	latitude := float64(latRaw) / 93206.0
	longitude := float64(lonRaw) / 46603.0
	
	// Высота термика
	altitude := int32(int16(binary.LittleEndian.Uint16(data[6:8])))
	
	thermal := &ThermalData{
		Latitude:  latitude,
		Longitude: longitude,
		Altitude:  altitude,
	}
	
	// Дополнительные параметры термика
	if len(data) >= 12 {
		thermal.ClimbRate = int16(binary.LittleEndian.Uint16(data[8:10]))
	}
	if len(data) >= 13 {
		thermal.Strength = data[10]
	}
	if len(data) >= 15 {
		thermal.Radius = binary.LittleEndian.Uint16(data[11:13])
	}
	
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