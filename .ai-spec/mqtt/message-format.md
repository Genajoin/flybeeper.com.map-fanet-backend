# FANET Message Format Specification

## Обзор протокола FANET

FANET (Flying Ad-hoc Network) - протокол для обмена данными между летательными аппаратами и наземными станциями. Оптимизирован для низкого энергопотребления и дальней связи через LoRa.

## Структура MQTT сообщения

### Обертка от базовой станции

```
┌─────────────┬──────┬──────┬────────────────┐
│  Timestamp  │ RSSI │ SNR  │  FANET Packet  │
│   4 bytes   │  2B  │  2B  │    N bytes     │
└─────────────┴──────┴──────┴────────────────┘
```

- **Timestamp**: Unix timestamp (little-endian)
- **RSSI**: Received Signal Strength Indicator (dBm)
- **SNR**: Signal-to-Noise Ratio (dB)
- **FANET Packet**: Сырой FANET пакет

## FANET Packet Structure

### Header (1 byte)

```
Bit 7  6  5  4  3  2  1  0
    │  │  └──┬──┘  └──┬──┘
    │  │     │        └── Packet Type (0-7)
    │  │     └────────── Address Type (0-7)
    │  └──────────────── Tracking Flag
    └─────────────────── Forward Flag
```

### Source Address (3 bytes)

24-bit уникальный адрес устройства (little-endian)

### Payload (variable)

Зависит от типа пакета

## Детальные форматы по типам

### Type 1: Air Position (воздушное судно)

```c
struct AirPosition {
    uint8_t  header;           // Заголовок
    uint24_t source_addr;      // Адрес источника
    int24_t  latitude;         // Широта * 93206.04
    int24_t  longitude;        // Долгота * 46603.02
    uint16_t altitude;         // (Высота - 1000) метров
    uint8_t  speed;            // Скорость * 2 (км/ч)
    uint8_t  climb;            // (Вариометр * 10) + 128
    uint8_t  heading;          // Курс * 256 / 360
    uint8_t  aircraft_type;    // Тип летательного аппарата
} __attribute__((packed));
```

**Декодирование:**
```go
latitude := float64(lat_raw) / 93206.04
longitude := float64(lon_raw) / 46603.02
altitude := uint16_raw + 1000
speed := float32(speed_raw) / 2.0
climb := (float32(climb_raw) - 128.0) / 10.0
heading := float32(heading_raw) * 360.0 / 256.0
```

**Типы летательных аппаратов:**
- 0: Unknown
- 1: Paraglider (параплан)
- 2: Hangglider (дельтаплан)
- 3: Balloon (воздушный шар)
- 4: Glider (планер)
- 5: Powered aircraft (мотопараплан)
- 6: Helicopter (вертолет)
- 7: UAV (дрон)

### Type 2: Name Beacon

```c
struct NameBeacon {
    uint8_t  header;
    uint24_t source_addr;
    char     name[1-20];       // UTF-8, переменная длина
} __attribute__((packed));
```

### Type 4: Service/Weather

```c
struct ServicePacket {
    uint8_t  header;
    uint24_t source_addr;
    uint8_t  service_type;     // Подтип сервиса
    uint8_t  payload[];        // Данные сервиса
} __attribute__((packed));
```

**Service Type 0: Weather Station**
```c
struct WeatherData {
    uint16_t wind_heading;     // Направление * 182
    uint16_t wind_speed;       // Скорость * 100 (м/с)
    uint16_t wind_gusts;       // Порывы * 100 (м/с)
    int16_t  temperature;      // Температура * 100 (°C)
    uint8_t  humidity;         // Влажность (%)
    uint16_t pressure;         // (Давление - 1000) (гПа)
    uint8_t  battery;          // Заряд батареи (%)
} __attribute__((packed));
```

**Service Type 1: Simple Status**
```c
struct SimpleStatus {
    uint8_t battery;           // Заряд батареи (%)
} __attribute__((packed));
```

### Type 7: Ground Position (наземный объект)

Формат идентичен Type 1, но для наземных объектов:
- Автомобили сопровождения
- Велосипедисты
- Пешеходы

### Type 9: Thermal Report

```c
struct ThermalReport {
    uint8_t  header;
    uint24_t source_addr;
    int24_t  latitude;         // Широта * 93206.04
    int24_t  longitude;        // Долгота * 46603.02
    uint16_t altitude;         // Высота (м)
    uint8_t  quality;          // Качество термика (0-5)
    int16_t  avg_climb;        // Средний подъем * 100 (м/с)
    uint16_t wind_speed;       // Скорость ветра * 100 (м/с)
    uint16_t wind_heading;     // Направление * 182
} __attribute__((packed));
```

## Примеры декодирования

### Go implementation

```go
func DecodeFANETPacket(data []byte) (interface{}, error) {
    if len(data) < 4 {
        return nil, errors.New("packet too short")
    }
    
    header := data[0]
    packetType := header & 0x07
    sourceAddr := uint32(data[1]) | uint32(data[2])<<8 | uint32(data[3])<<16
    
    payload := data[4:]
    
    switch packetType {
    case 1: // Air Position
        return decodeAirPosition(sourceAddr, payload)
    case 2: // Name
        return decodeName(sourceAddr, payload)
    case 4: // Service
        return decodeService(sourceAddr, payload)
    case 7: // Ground Position
        return decodeGroundPosition(sourceAddr, payload)
    case 9: // Thermal
        return decodeThermal(sourceAddr, payload)
    default:
        return nil, fmt.Errorf("unknown packet type: %d", packetType)
    }
}

func decodeAirPosition(addr uint32, data []byte) (*AirPosition, error) {
    if len(data) < 11 {
        return nil, errors.New("air position packet too short")
    }
    
    // Decode coordinates
    latRaw := int32(data[0]) | int32(data[1])<<8 | int32(data[2])<<16
    if latRaw&0x800000 != 0 { // Sign extend
        latRaw |= 0xFF000000
    }
    
    lonRaw := int32(data[3]) | int32(data[4])<<8 | int32(data[5])<<16
    if lonRaw&0x800000 != 0 {
        lonRaw |= 0xFF000000
    }
    
    return &AirPosition{
        Addr:      addr,
        Latitude:  float64(latRaw) / 93206.04,
        Longitude: float64(lonRaw) / 46603.02,
        Altitude:  int32(uint16(data[6])|uint16(data[7])<<8) + 1000,
        Speed:     float32(data[8]) / 2.0,
        Climb:     (float32(data[9]) - 128.0) / 10.0,
        Course:    float32(data[10]) * 360.0 / 256.0,
        Type:      PilotType(data[11]),
    }, nil
}
```

## Валидация данных

### Проверки координат
- Широта: -90° до +90°
- Долгота: -180° до +180°

### Проверки высоты
- Минимум: -1000м (для Death Valley)
- Максимум: 15000м (для планеров)

### Проверки скорости
- Максимум: 400 км/ч (для мотопарапланов)
- Для термиков: -10 до +20 м/с

## Обработка ошибок

1. **Битые пакеты**: проверка размера и CRC (если есть)
2. **Старые данные**: отбрасывать пакеты старше 1 часа
3. **Дубликаты**: проверка по addr + timestamp
4. **Невалидные координаты**: логирование и отбрасывание