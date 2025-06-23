package mqtt

import (
	"encoding/binary"
	"encoding/hex"
	"testing"
	"time"

	"github.com/flybeeper/fanet-backend/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParser_Parse_ValidTopic(t *testing.T) {
	logger := utils.NewLogger("info", "text")
	parser := NewParser(logger)

	tests := []struct {
		name        string
		topic       string
		expectError bool
	}{
		{
			name:        "Valid topic format",
			topic:       "fb/b/ABC123/f/1",
			expectError: false,
		},
		{
			name:        "Invalid topic - wrong prefix",
			topic:       "fanet/b/ABC123/f/1",
			expectError: true,
		},
		{
			name:        "Invalid topic - missing parts",
			topic:       "fb/b/ABC123",
			expectError: true,
		},
		{
			name:        "Invalid topic - wrong structure",
			topic:       "fb/x/ABC123/f/1",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Минимальный валидный payload (wrapper + FANET header)
			payload := make([]byte, 12)
			binary.LittleEndian.PutUint32(payload[0:4], uint32(time.Now().Unix()))
			binary.LittleEndian.PutUint16(payload[4:6], uint16(int16(-80))) // RSSI
			binary.LittleEndian.PutUint16(payload[6:8], uint16(10))  // SNR
			payload[8] = 1                                           // FANET type
			payload[9] = 0x34                                        // Device ID low
			payload[10] = 0x12                                       // Device ID mid
			payload[11] = 0x00                                       // Device ID high

			msg, err := parser.Parse(tt.topic, payload)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, msg)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, msg)
				assert.Equal(t, "ABC123", msg.ChipID)
				assert.Equal(t, "1", msg.PacketType)
			}
		})
	}
}

func TestParser_Parse_PayloadLength(t *testing.T) {
	logger := utils.NewLogger("info", "text")
	parser := NewParser(logger)

	tests := []struct {
		name        string
		payloadSize int
		expectError bool
	}{
		{
			name:        "Payload too short - less than 12 bytes",
			payloadSize: 11,
			expectError: true,
		},
		{
			name:        "Minimum valid payload - 12 bytes",
			payloadSize: 12,
			expectError: false,
		},
		{
			name:        "Valid payload with data",
			payloadSize: 20,
			expectError: false,
		},
	}

	topic := "fb/b/TEST01/f/1"

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := make([]byte, tt.payloadSize)
			if tt.payloadSize >= 12 {
				binary.LittleEndian.PutUint32(payload[0:4], uint32(time.Now().Unix()))
				binary.LittleEndian.PutUint16(payload[4:6], uint16(int16(-80)))
				binary.LittleEndian.PutUint16(payload[6:8], uint16(10))
				payload[8] = 1     // FANET type
				payload[9] = 0x34  // Device ID
				payload[10] = 0x12
				payload[11] = 0x00
			}

			msg, err := parser.Parse(topic, payload)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, msg)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, msg)
			}
		})
	}
}

func TestParser_ParseAirTracking(t *testing.T) {
	logger := utils.NewLogger("info", "text")
	parser := NewParser(logger)

	// Создаем тестовый Air Tracking пакет (Type 1)
	payload := make([]byte, 8+4+11) // wrapper + header + air tracking data

	// Base station wrapper
	timestamp := uint32(time.Now().Unix())
	binary.LittleEndian.PutUint32(payload[0:4], timestamp)
	binary.LittleEndian.PutUint16(payload[4:6], uint16(int16(-80))) // RSSI
	binary.LittleEndian.PutUint16(payload[6:8], uint16(10))  // SNR

	// FANET header
	payload[8] = 1     // Type 1 (Air tracking)
	payload[9] = 0x34  // Device ID: 0x001234
	payload[10] = 0x12
	payload[11] = 0x00

	// Air tracking data (11 bytes)
	data := payload[12:]

	// Координаты: 46.0, 8.0 (Alps region)
	latFloat := 46.0 * 93206.04
	lonFloat := 8.0 * 46603.02
	lat := int32(latFloat) & 0xFFFFFF  // 24-bit
	lon := int32(lonFloat) & 0xFFFFFF  // 24-bit

	data[0] = byte(lat & 0xFF)
	data[1] = byte((lat >> 8) & 0xFF)
	data[2] = byte((lat >> 16) & 0xFF)

	data[3] = byte(lon & 0xFF)
	data[4] = byte((lon >> 8) & 0xFF)
	data[5] = byte((lon >> 16) & 0xFF)

	// Alt_status: altitude=1000м, aircraft_type=1 (paraglider), online=true
	altStatus := uint16(1000) | (1 << 12) | (1 << 15) // altitude + aircraft_type + online_tracking
	binary.LittleEndian.PutUint16(data[6:8], altStatus)

	// Speed: 50 km/h (100 * 0.5)
	data[8] = 100

	// Climb rate: +2 m/s (20 * 0.1)
	data[9] = 20

	// Heading: 90° (64 * 360/256)
	data[10] = 64

	msg, err := parser.Parse("fb/b/TEST01/f/1", payload)
	require.NoError(t, err)
	require.NotNil(t, msg)

	assert.Equal(t, uint8(1), msg.Type)
	assert.Equal(t, "001234", msg.DeviceID)
	assert.Equal(t, "TEST01", msg.ChipID)
	assert.Equal(t, int16(-80), msg.RSSI)
	assert.Equal(t, int16(10), msg.SNR)

	// Проверяем распарсенные данные
	require.IsType(t, &AirTrackingData{}, msg.Data)
	airData := msg.Data.(*AirTrackingData)

	assert.InDelta(t, 46.0, airData.Latitude, 0.001)
	assert.InDelta(t, 8.0, airData.Longitude, 0.001)
	assert.Equal(t, int32(1000), airData.Altitude)
	assert.Equal(t, uint16(50), airData.Speed)
	assert.Equal(t, uint16(90), airData.Heading)
	assert.Equal(t, int16(20), airData.ClimbRate)
	assert.Equal(t, uint8(1), airData.AircraftType)
	assert.True(t, airData.OnlineTracking)
}

func TestParser_ParseName(t *testing.T) {
	logger := utils.NewLogger("info", "text")
	parser := NewParser(logger)

	// Создаем тестовый Name пакет (Type 2)
	testName := "TestPilot"
	payload := make([]byte, 8+4+len(testName))

	// Base station wrapper
	binary.LittleEndian.PutUint32(payload[0:4], uint32(time.Now().Unix()))
	binary.LittleEndian.PutUint16(payload[4:6], uint16(int16(-75)))
	binary.LittleEndian.PutUint16(payload[6:8], uint16(15))

	// FANET header
	payload[8] = 2     // Type 2 (Name)
	payload[9] = 0x56  // Device ID: 0x001256
	payload[10] = 0x12
	payload[11] = 0x00

	// Name data
	copy(payload[12:], testName)

	msg, err := parser.Parse("fb/b/BASE01/f/2", payload)
	require.NoError(t, err)
	require.NotNil(t, msg)

	assert.Equal(t, uint8(2), msg.Type)
	assert.Equal(t, "001256", msg.DeviceID)

	require.IsType(t, &NameData{}, msg.Data)
	nameData := msg.Data.(*NameData)
	assert.Equal(t, testName, nameData.Name)
}

func TestParser_ParseService(t *testing.T) {
	logger := utils.NewLogger("info", "text")
	parser := NewParser(logger)

	// Создаем тестовый Service пакет (Type 4) с температурой
	payload := make([]byte, 8+4+7+1) // wrapper + header + coordinates + temp

	// Base station wrapper
	binary.LittleEndian.PutUint32(payload[0:4], uint32(time.Now().Unix()))
	binary.LittleEndian.PutUint16(payload[4:6], uint16(int16(-70)))
	binary.LittleEndian.PutUint16(payload[6:8], uint16(20))

	// FANET header
	payload[8] = 4     // Type 4 (Service)
	payload[9] = 0x78  // Device ID: 0x001278
	payload[10] = 0x12
	payload[11] = 0x00

	// Service data
	data := payload[12:]

	// Service header с temperature flag (bit 6)
	data[0] = 0x40 // Bit 6 set = temperature data

	// Координаты станции: 47.0, 8.5
	lat := int32(47.0 * 93206.04) & 0xFFFFFF  // 24-bit
	lon := int32(8.5 * 46603.02) & 0xFFFFFF   // 24-bit

	data[1] = byte(lat & 0xFF)
	data[2] = byte((lat >> 8) & 0xFF)
	data[3] = byte((lat >> 16) & 0xFF)

	data[4] = byte(lon & 0xFF)
	data[5] = byte((lon >> 8) & 0xFF)
	data[6] = byte((lon >> 16) & 0xFF)

	// Temperature: 20°C (40 * 0.5)
	data[7] = 40

	msg, err := parser.Parse("fb/b/WEATHER/f/4", payload)
	require.NoError(t, err)
	require.NotNil(t, msg)

	assert.Equal(t, uint8(4), msg.Type)
	assert.Equal(t, "001278", msg.DeviceID)

	require.IsType(t, &ServiceData{}, msg.Data)
	serviceData := msg.Data.(*ServiceData)

	assert.Equal(t, uint8(0x40), serviceData.ServiceHeader)
	assert.InDelta(t, 47.0, serviceData.Latitude, 0.001)
	assert.InDelta(t, 8.5, serviceData.Longitude, 0.001)

	require.IsType(t, &WeatherData{}, serviceData.Data)
	weatherData := serviceData.Data.(*WeatherData)
	assert.InDelta(t, 20.0, weatherData.Temperature, 0.1)
}

func TestParser_ParseGroundTracking(t *testing.T) {
	logger := utils.NewLogger("info", "text")
	parser := NewParser(logger)

	// Создаем тестовый Ground Tracking пакет (Type 7)
	payload := make([]byte, 8+4+10) // wrapper + header + ground tracking data

	// Base station wrapper
	binary.LittleEndian.PutUint32(payload[0:4], uint32(time.Now().Unix()))
	binary.LittleEndian.PutUint16(payload[4:6], uint16(int16(-85)))
	binary.LittleEndian.PutUint16(payload[6:8], uint16(5))

	// FANET header
	payload[8] = 7     // Type 7 (Ground tracking)
	payload[9] = 0x9A  // Device ID: 0x00129A
	payload[10] = 0x12
	payload[11] = 0x00

	// Ground tracking data
	data := payload[12:]

	// Координаты: 46.5, 7.5
	lat := int32(46.5 * 93206.04) & 0xFFFFFF  // 24-bit
	lon := int32(7.5 * 46603.02) & 0xFFFFFF   // 24-bit

	data[0] = byte(lat & 0xFF)
	data[1] = byte((lat >> 8) & 0xFF)
	data[2] = byte((lat >> 16) & 0xFF)

	data[3] = byte(lon & 0xFF)
	data[4] = byte((lon >> 8) & 0xFF)
	data[5] = byte((lon >> 16) & 0xFF)

	// Altitude: 500m
	binary.LittleEndian.PutUint16(data[6:8], 500)

	// Speed and heading combined: speed=30km/h, heading=180°
	speedHeading := uint16(30<<6) | (180/6)
	binary.LittleEndian.PutUint16(data[8:10], speedHeading)

	msg, err := parser.Parse("fb/b/GROUND/f/7", payload)
	require.NoError(t, err)
	require.NotNil(t, msg)

	assert.Equal(t, uint8(7), msg.Type)
	assert.Equal(t, "00129A", msg.DeviceID)

	require.IsType(t, &GroundTrackingData{}, msg.Data)
	groundData := msg.Data.(*GroundTrackingData)

	assert.InDelta(t, 46.5, groundData.Latitude, 0.001)
	assert.InDelta(t, 7.5, groundData.Longitude, 0.001)
	assert.Equal(t, int32(500), groundData.Altitude)
	assert.Equal(t, uint16(30), groundData.Speed)
	assert.Equal(t, uint16(180), groundData.Heading)
}

func TestParser_ParseThermal(t *testing.T) {
	logger := utils.NewLogger("info", "text")
	parser := NewParser(logger)

	// Создаем тестовый Thermal пакет (Type 9)
	payload := make([]byte, 8+4+11) // wrapper + header + thermal data

	// Base station wrapper
	binary.LittleEndian.PutUint32(payload[0:4], uint32(time.Now().Unix()))
	binary.LittleEndian.PutUint16(payload[4:6], uint16(int16(-70)))
	binary.LittleEndian.PutUint16(payload[6:8], uint16(25))

	// FANET header
	payload[8] = 9     // Type 9 (Thermal)
	payload[9] = 0xBC  // Device ID: 0x0012BC
	payload[10] = 0x12
	payload[11] = 0x00

	// Thermal data
	data := payload[12:]

	// Координаты центра термика: 46.2, 8.1
	lat := int32(46.2 * 93206.04) & 0xFFFFFF  // 24-bit
	lon := int32(8.1 * 46603.02) & 0xFFFFFF   // 24-bit

	data[0] = byte(lat & 0xFF)
	data[1] = byte((lat >> 8) & 0xFF)
	data[2] = byte((lat >> 16) & 0xFF)

	data[3] = byte(lon & 0xFF)
	data[4] = byte((lon >> 8) & 0xFF)
	data[5] = byte((lon >> 16) & 0xFF)

	// Altitude: 1200m
	binary.LittleEndian.PutUint16(data[6:8], 1200)

	// Quality: 4 (good thermal)
	data[8] = 4

	// Average climb rate: 3.5 m/s = 350 (cm/s)
	binary.LittleEndian.PutUint16(data[9:11], 350)

	msg, err := parser.Parse("fb/b/THERMAL/f/9", payload)
	require.NoError(t, err)
	require.NotNil(t, msg)

	assert.Equal(t, uint8(9), msg.Type)
	assert.Equal(t, "0012BC", msg.DeviceID)

	require.IsType(t, &ThermalData{}, msg.Data)
	thermalData := msg.Data.(*ThermalData)

	assert.InDelta(t, 46.2, thermalData.Latitude, 0.001)
	assert.InDelta(t, 8.1, thermalData.Longitude, 0.001)
	assert.Equal(t, int32(1200), thermalData.Altitude)
	assert.Equal(t, uint8(4), thermalData.Strength)
	assert.Equal(t, int16(350), thermalData.ClimbRate)
}

func TestParser_ValidateCoordinates(t *testing.T) {
	logger := utils.NewLogger("info", "text")
	parser := NewParser(logger)

	tests := []struct {
		name     string
		lat, lon float64
		valid    bool
	}{
		{"Valid coordinates - Alps", 46.0, 8.0, true},
		{"Valid coordinates - Equator", 0.0, 0.0, true},
		{"Valid coordinates - North Pole", 90.0, 0.0, true},
		{"Valid coordinates - South Pole", -90.0, 0.0, true},
		{"Valid coordinates - Date line", 0.0, 180.0, true},
		{"Valid coordinates - Date line negative", 0.0, -180.0, true},
		{"Invalid latitude - too high", 91.0, 0.0, false},
		{"Invalid latitude - too low", -91.0, 0.0, false},
		{"Invalid longitude - too high", 0.0, 181.0, false},
		{"Invalid longitude - too low", 0.0, -181.0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := parser.ValidateCoordinates(tt.lat, tt.lon)
			assert.Equal(t, tt.valid, valid)
		})
	}
}

func TestParser_CalculateDistance(t *testing.T) {
	logger := utils.NewLogger("info", "text")
	parser := NewParser(logger)

	// Известные расстояния для проверки
	tests := []struct {
		name                     string
		lat1, lon1, lat2, lon2   float64
		expectedDistance         float64
		tolerance                float64
	}{
		{
			name:             "Same point",
			lat1: 46.0, lon1: 8.0,
			lat2: 46.0, lon2: 8.0,
			expectedDistance: 0.0,
			tolerance:        1.0,
		},
		{
			name:             "1 degree latitude difference",
			lat1: 46.0, lon1: 8.0,
			lat2: 47.0, lon2: 8.0,
			expectedDistance: 111000.0, // ~111km
			tolerance:        5000.0,
		},
		{
			name:             "Zurich to Bern (approx)",
			lat1: 47.3769, lon1: 8.5417,
			lat2: 46.9481, lon2: 7.4474,
			expectedDistance: 95000.0, // ~95km
			tolerance:        10000.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			distance := parser.CalculateDistance(tt.lat1, tt.lon1, tt.lat2, tt.lon2)
			assert.InDelta(t, tt.expectedDistance, distance, tt.tolerance)
		})
	}
}

func TestParser_DebugMode(t *testing.T) {
	logger := utils.NewLogger("debug", "text")
	parser := NewParser(logger)

	// Тестируем переключение debug режима
	assert.False(t, parser.debugEnabled)

	parser.SetDebugMode(true)
	assert.True(t, parser.debugEnabled)

	parser.SetDebugMode(false)
	assert.False(t, parser.debugEnabled)
}

func TestParser_UnsupportedMessageType(t *testing.T) {
	logger := utils.NewLogger("info", "text")
	parser := NewParser(logger)

	// Создаем пакет с неподдерживаемым типом (Type 15)
	payload := make([]byte, 8+4+5) // wrapper + header + some data

	// Base station wrapper
	binary.LittleEndian.PutUint32(payload[0:4], uint32(time.Now().Unix()))
	binary.LittleEndian.PutUint16(payload[4:6], uint16(int16(-80)))
	binary.LittleEndian.PutUint16(payload[6:8], uint16(10))

	// FANET header с неподдерживаемым типом
	payload[8] = 15    // Type 15 (unsupported)
	payload[9] = 0x34  // Device ID
	payload[10] = 0x12
	payload[11] = 0x00

	// Добавляем некоторые данные
	payload[12] = 0x01
	payload[13] = 0x02
	payload[14] = 0x03
	payload[15] = 0x04
	payload[16] = 0x05

	msg, err := parser.Parse("fb/b/TEST/f/15", payload)
	
	// Парсер должен вернуть сообщение без ошибки, но без данных
	require.NoError(t, err)
	require.NotNil(t, msg)
	
	assert.Equal(t, uint8(15), msg.Type)
	assert.Equal(t, "001234", msg.DeviceID)
	assert.Nil(t, msg.Data) // Данные не парсятся для неподдерживаемых типов
}

func TestParser_RealWorldHexData(t *testing.T) {
	logger := utils.NewLogger("debug", "text")
	parser := NewParser(logger)
	parser.SetDebugMode(true)

	// Реальный пример MQTT пакета в hex формате (из логов)
	// Это Type 1 (Air Tracking) пакет
	hexData := "6b23496601F5000A007B1B8EC50000007C000000C82A"
	payload, err := hex.DecodeString(hexData)
	require.NoError(t, err)

	msg, err := parser.Parse("fb/b/40FE17/f/1", payload)
	require.NoError(t, err)
	require.NotNil(t, msg)

	assert.Equal(t, uint8(1), msg.Type) // Air tracking
	assert.Equal(t, "40FE17", msg.ChipID)

	// Проверяем, что координаты в разумных пределах для Европы
	if msg.Data != nil {
		airData, ok := msg.Data.(*AirTrackingData)
		if ok {
			assert.True(t, parser.ValidateCoordinates(airData.Latitude, airData.Longitude))
			t.Logf("Parsed coordinates: %.6f, %.6f", airData.Latitude, airData.Longitude)
			t.Logf("Altitude: %dm, Speed: %dkm/h", airData.Altitude, airData.Speed)
		}
	}
}

// Benchmark тесты для производительности
func BenchmarkParser_Parse(b *testing.B) {
	logger := utils.NewLogger("error", "text") // Минимальное логирование для бенчмарка
	parser := NewParser(logger)

	// Создаем типичный Air Tracking пакет
	payload := make([]byte, 8+4+11)
	binary.LittleEndian.PutUint32(payload[0:4], uint32(time.Now().Unix()))
	binary.LittleEndian.PutUint16(payload[4:6], uint16(int16(-80)))
	binary.LittleEndian.PutUint16(payload[6:8], uint16(10))
	payload[8] = 1
	payload[9] = 0x34
	payload[10] = 0x12
	payload[11] = 0x00

	// Добавляем реалистичные air tracking данные
	data := payload[12:]
	lat := int32(46.0 * 93206.04) & 0xFFFFFF  // 24-bit
	lon := int32(8.0 * 46603.02) & 0xFFFFFF   // 24-bit
	data[0] = byte(lat & 0xFF)
	data[1] = byte((lat >> 8) & 0xFF)
	data[2] = byte((lat >> 16) & 0xFF)
	data[3] = byte(lon & 0xFF)
	data[4] = byte((lon >> 8) & 0xFF)
	data[5] = byte((lon >> 16) & 0xFF)
	binary.LittleEndian.PutUint16(data[6:8], 1000)
	data[8] = 100  // speed
	data[9] = 20   // climb
	data[10] = 64  // heading

	topic := "fb/b/BENCH/f/1"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := parser.Parse(topic, payload)
		if err != nil {
			b.Fatalf("Parse failed: %v", err)
		}
	}
}

func BenchmarkParser_ParseAirTracking(b *testing.B) {
	logger := utils.NewLogger("error", "text")
	parser := NewParser(logger)

	// Air tracking data (11 bytes)
	data := make([]byte, 11)
	lat := int32(46.0 * 93206.04) & 0xFFFFFF  // 24-bit
	lon := int32(8.0 * 46603.02) & 0xFFFFFF   // 24-bit
	data[0] = byte(lat & 0xFF)
	data[1] = byte((lat >> 8) & 0xFF)
	data[2] = byte((lat >> 16) & 0xFF)
	data[3] = byte(lon & 0xFF)
	data[4] = byte((lon >> 8) & 0xFF)
	data[5] = byte((lon >> 16) & 0xFF)
	binary.LittleEndian.PutUint16(data[6:8], 1000)
	data[8] = 100
	data[9] = 20
	data[10] = 64

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := parser.parseAirTracking(data)
		if err != nil {
			b.Fatalf("parseAirTracking failed: %v", err)
		}
	}
}