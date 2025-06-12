# FANET Message Format Specification

## Обзор протокола FANET

FANET (Flying Ad-hoc Network) - протокол для обмена данными между летательными аппаратами и наземными станциями. Оптимизирован для низкого энергопотребления и дальней связи через LoRa.

**Исходная базовая спецификация**: [https://github.com/3s1d/fanet-stm32/blob/master/Src/fanet/radio/protocol.txt](https://github.com/3s1d/fanet-stm32/blob/master/Src/fanet/radio/protocol.txt)

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
    │  │  └─────────────┬─┘
    │  │                └── Packet Type (0-63)
    │  └──────────────────── Forward Flag
    └─────────────────────── Extended Header Flag
```

**Структура заголовка:**
- **Bit 7**: Extended Header flag
- **Bit 6**: Forward flag  
- **Bits 5-0**: Packet Type (6 бит, 0-63)

**Существующие типы пакетов:**
- 0: ACK, 1: Tracking, 2: Name, 3: Message, 4: Service
- 5: Landmarks, 6: Remote Configuration, 7: Ground Tracking  
- 8: HW Info (deprecated), 9: Thermal, 10(0xA): New HW Info

### Source Address (3 bytes)

24-bit уникальный адрес устройства (little-endian)

### Payload (variable)

Зависит от типа пакета

## Детальные форматы по типам

### Type 0: ACK (Acknowledgement)
**Интервал**: По необходимости  
**Payload**: Нет (только заголовок + адрес)  
**Назначение**: Подтверждение получения unicast сообщений

### Type 1: Tracking (Air/Ground Position)
**Интервал**: 1-5 секунд  

**Payload:**
```c
struct TrackingData {
    uint8_t  header;           // Заголовок
    uint24_t source_addr;      // Адрес источника (little-endian)
    int24_t  latitude;         // Широта (little-endian, 2-complement)
    int24_t  longitude;        // Долгота (little-endian, 2-complement)
    uint16_t alt_status;       // Высота + статус + тип ВС
    uint8_t  speed;            // Скорость
    uint8_t  climb;            // Вертикальная скорость
    uint8_t  heading;          // Курс
    // Опционально:
    uint8_t  turn_rate;        // Скорость поворота
    uint8_t  qne_offset;       // QNE offset
} __attribute__((packed));
```

**Декодирование координат:**
```go
// Координаты в градусах
latitude := float64(lat_raw) / 93206.0
longitude := float64(lon_raw) / 46603.0
```

**Bytes 6-7 (alt_status) битовые поля:**
- Bit 15: Online Tracking flag (1=online, 0=replay)
- Bits 14-12: Aircraft Type (0-7)
- Bit 11: Altitude scaling (0=1x, 1=4x)
- Bits 10-0: Altitude в метрах

**Byte 8 (speed) битовые поля:**
- Bit 7: Speed scaling (0=1x, 1=5x)
- Bits 6-0: Speed в 0.5 км/ч

**Byte 9 (climb) битовые поля:**
- Bit 7: Climb scaling (0=1x, 1=5x)  
- Bits 6-0: Climb rate в 0.1 м/с (signed)

**Byte 10 (heading):**
- 0-255 представляет 0-360°

**Типы летательных аппаратов:**
- 0: Other
- 1: Paraglider
- 2: Hangglider  
- 3: Balloon
- 4: Glider
- 5: Powered aircraft
- 6: Helicopter
- 7: UAV

### Type 2: Name Beacon
**Интервал**: 5 минут или при изменении  

**Payload:**
```c
struct NameBeacon {
    uint8_t  header;
    uint24_t source_addr;
    char     name[];           // UTF-8, переменная длина, без null-терминатора
} __attribute__((packed));
```

### Type 3: Message  
**Интервал**: По необходимости  

**Payload:**
```c
struct Message {
    uint8_t  header;
    uint24_t source_addr;
    uint8_t  subheader;        // Подтип сообщения
    char     message[];        // UTF-8 текст
} __attribute__((packed));
```

### Type 4: Service (Weather Station)
**Интервал**: 40 секунд  

**Заголовок службы (Byte 0 после адреса):**
- Bit 7: Internet Gateway
- Bit 6: Temperature (+1 байт: °C * 2, 2-complement)
- Bit 5: Wind (+3 байта: направление, скорость, порывы)
- Bit 4: Humidity (+1 байт: %RH * 4)
- Bit 3: Barometric pressure (+2 байта: (hPa-430)*10, little-endian)
- Bit 2: Remote Configuration Support
- Bit 1: State of Charge (+1 байт: 0x0=0%, 0x1=6.67%, ..., 0xF=100%)
- Bit 0: Extended Header (+1 байт)

**Обязательные координаты (если есть данные):**
```c
struct ServiceData {
    uint8_t  header;
    uint24_t source_addr;
    uint8_t  service_header;   // Битовые флаги выше
    int24_t  latitude;         // Координаты станции (little-endian)
    int24_t  longitude;        // Координаты станции (little-endian)
    // + дополнительные данные согласно флагам в service_header
} __attribute__((packed));
```

**Формат дополнительных данных:**
- **Temperature**: 1 байт, °C * 2 (signed)
- **Wind**: 3 байта
  - Byte 0: Направление (0-255 = 0-360°)
  - Byte 1: Скорость и порывы, биты 7-0
  - Byte 2: Скорость и порывы, биты 15-8
- **Humidity**: 1 байт, %RH * 4
- **Pressure**: 2 байта, (hPa - 430) * 10, little-endian  
- **Battery**: 1 байт, младшие 4 бита (0x0-0xF)

### Type 5: Landmarks
**Интервал**: Редко  

**Payload:** Сложная структура с подтипами для точек, линий, областей

### Type 6: Remote Configuration
**Интервал**: По запросу  

**Payload:** Команды конфигурации устройства

### Type 7: Ground Tracking  
**Интервал**: 1-10 секунд  

**Payload:** Идентичен Type 1, но для наземного транспорта

### Type 8: Hardware Info (deprecated)
**Интервал**: При запуске  
**Статус**: Устаревший, заменен на Type 10

**Payload:**
```c
struct HWInfo {
    uint8_t  header;
    uint24_t source_addr;
    uint8_t  hw_type;          // Тип устройства/производитель
    // Дополнительная информация об устройстве
} __attribute__((packed));
```

### Type 10 (0xA): New Hardware Info  
**Интервал**: При запуске  
**Статус**: Заменяет Type 8

**Payload:**
```c
struct NewHWInfo {
    uint8_t  header;
    uint24_t source_addr;
    uint8_t  manufacturer;     // Производитель (1 байт)
    uint8_t  hw_type;          // Тип устройства
    // Расширенная информация об устройстве
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
    packetType := header & 0x3F  // Биты 5-0
    sourceAddr := uint32(data[1]) | uint32(data[2])<<8 | uint32(data[3])<<16
    
    payload := data[4:]
    
    switch packetType {
    case 0: // ACK
        return &ACKPacket{Addr: sourceAddr}, nil
    case 1: // Tracking
        return decodeTracking(sourceAddr, payload)
    case 2: // Name
        return decodeName(sourceAddr, payload)
    case 3: // Message
        return decodeMessage(sourceAddr, payload)
    case 4: // Service
        return decodeService(sourceAddr, payload)
    case 5: // Landmarks
        return decodeLandmarks(sourceAddr, payload)
    case 6: // Remote Config
        return decodeRemoteConfig(sourceAddr, payload)
    case 7: // Ground Tracking
        return decodeTracking(sourceAddr, payload) // Идентичен Type 1
    case 8: // HW Info (deprecated)
        return decodeHWInfo(sourceAddr, payload)
    case 9: // Thermal
        return decodeThermal(sourceAddr, payload)
    case 10: // New HW Info
        return decodeNewHWInfo(sourceAddr, payload)
    default:
        return nil, fmt.Errorf("unknown packet type: %d", packetType)
    }
}

func decodeTracking(addr uint32, data []byte) (*TrackingData, error) {
    if len(data) < 7 {
        return nil, errors.New("tracking packet too short")
    }
    
    // Декодирование координат (3+3 байта)
    latRaw := int32(data[0]) | int32(data[1])<<8 | int32(data[2])<<16
    if latRaw&0x800000 != 0 { // Знаковое расширение для 24-bit
        latRaw |= 0xFF000000
    }
    
    lonRaw := int32(data[3]) | int32(data[4])<<8 | int32(data[5])<<16
    if lonRaw&0x800000 != 0 {
        lonRaw |= 0xFF000000
    }
    
    latitude := float64(latRaw) / 93206.0
    longitude := float64(lonRaw) / 46603.0
    
    // Alt_status (2 байта) если есть
    var altitude int32
    var aircraftType uint8
    var onlineTracking bool
    
    if len(data) >= 8 {
        altStatus := uint16(data[6]) | uint16(data[7])<<8
        onlineTracking = (altStatus & 0x8000) != 0
        aircraftType = uint8((altStatus >> 12) & 0x07)
        altScale := (altStatus & 0x0800) != 0
        altRaw := int32(altStatus & 0x07FF)
        
        if altScale {
            altitude = altRaw * 4
        } else {
            altitude = altRaw
        }
    }
    
    // Speed (1 байт) если есть
    var speed float32
    if len(data) >= 9 {
        speedRaw := data[8]
        speedScale := (speedRaw & 0x80) != 0
        speedVal := float32(speedRaw & 0x7F)
        
        if speedScale {
            speed = speedVal * 5 * 0.5 // 5x scaling, 0.5 km/h units
        } else {
            speed = speedVal * 0.5
        }
    }
    
    // Climb (1 байт) если есть
    var climbRate float32
    if len(data) >= 10 {
        climbRaw := data[9]
        climbScale := (climbRaw & 0x80) != 0
        climbVal := float32(int8(climbRaw & 0x7F)) // Signed 7-bit
        
        if climbScale {
            climbRate = climbVal * 5 * 0.1 // 5x scaling, 0.1 m/s units
        } else {
            climbRate = climbVal * 0.1
        }
    }
    
    // Heading (1 байт) если есть
    var heading float32
    if len(data) >= 11 {
        heading = float32(data[10]) * 360.0 / 256.0
    }
    
    return &TrackingData{
        Addr:           sourceAddr,
        Latitude:       latitude,
        Longitude:      longitude,
        Altitude:       altitude,
        Speed:          speed,
        ClimbRate:      climbRate,
        Heading:        heading,
        AircraftType:   aircraftType,
        OnlineTracking: onlineTracking,
    }, nil
}

func decodeName(addr uint32, data []byte) (*NameData, error) {
    if len(data) == 0 {
        return nil, errors.New("name packet is empty")
    }
    
    return &NameData{
        Addr: addr,
        Name: string(data), // UTF-8, без null-терминатора
    }, nil
}

func decodeService(addr uint32, data []byte) (*ServiceData, error) {
    if len(data) < 7 { // Минимум: service_header + координаты
        return nil, errors.New("service packet too short")
    }
    
    serviceHeader := data[0]
    
    // Координаты станции (обязательные для Type 4)
    latRaw := int32(data[1]) | int32(data[2])<<8 | int32(data[3])<<16
    if latRaw&0x800000 != 0 {
        latRaw |= 0xFF000000
    }
    
    lonRaw := int32(data[4]) | int32(data[5])<<8 | int32(data[6])<<16
    if lonRaw&0x800000 != 0 {
        lonRaw |= 0xFF000000
    }
    
    latitude := float64(latRaw) / 93206.0
    longitude := float64(lonRaw) / 46603.0
    
    service := &ServiceData{
        Addr:      addr,
        Latitude:  latitude,
        Longitude: longitude,
        Header:    serviceHeader,
    }
    
    // Декодирование дополнительных данных согласно флагам
    offset := 7
    
    if serviceHeader&0x40 != 0 { // Temperature
        if offset < len(data) {
            service.Temperature = int8(data[offset]) / 2.0 // °C
            offset++
        }
    }
    
    if serviceHeader&0x20 != 0 { // Wind
        if offset+2 < len(data) {
            service.WindDirection = uint16(data[offset]) * 360 / 256
            // Wind speed/gusts декодирование более сложное
            offset += 3
        }
    }
    
    if serviceHeader&0x10 != 0 { // Humidity
        if offset < len(data) {
            service.Humidity = data[offset] / 4 // %RH
            offset++
        }
    }
    
    if serviceHeader&0x08 != 0 { // Pressure
        if offset+1 < len(data) {
            pressureRaw := uint16(data[offset]) | uint16(data[offset+1])<<8
            service.Pressure = (float32(pressureRaw) / 10.0) + 430.0 // hPa
            offset += 2
        }
    }
    
    if serviceHeader&0x02 != 0 { // Battery
        if offset < len(data) {
            battery := data[offset] & 0x0F
            service.Battery = uint8(battery) * 100 / 15 // 0x0-0xF -> 0-100%
            offset++
        }
    }
    
    return service, nil
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