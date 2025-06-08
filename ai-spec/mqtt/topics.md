# MQTT Topics Structure

## Основные топики

### 1. FANET данные от базовых станций

```
fb/b/{chip_id}/f
```

- `fb` - FlyBeeper namespace
- `b` - Base station (базовая станция)
- `{chip_id}` - Уникальный ID базовой станции (например: 2462966788)
- `f` - FANET data

Пример: `fb/b/2462966788/f`

### 2. Hardware информация от станций

```
fb/b/{chip_id}/hw
```

- Содержит информацию о самой базовой станции
- Статус, версия прошивки, уровень сигнала

### 3. Служебные топики (будущее)

```
fb/s/{service}/status  - Статус сервисов
fb/s/{service}/metrics - Метрики
```

## Формат сообщений

### FANET пакет (fb/b/+/f)

Бинарный формат:

```
Offset | Size | Description
-------|------|-------------
0      | 4    | Timestamp (Unix time, little-endian)
4      | 2    | RSSI (уровень сигнала, signed)
6      | 2    | SNR (Signal-to-Noise Ratio, signed)
8      | N    | Raw FANET packet data
```

### FANET packet structure

```
Byte 0: Header
  Bit 7: Forward flag
  Bit 6: Tracking flag  
  Bit 5-3: Address type
  Bit 2-0: Packet type

Byte 1-3: Source address (24 bit)
Byte 4+: Type-specific payload
```

## Типы FANET пакетов

### Type 1: Air Tracking (воздушное судно)

```
Payload:
  3 bytes: Latitude (signed, deg * 93206.04)
  3 bytes: Longitude (signed, deg * 46603.02)
  2 bytes: Altitude (unsigned, m - 1000)
  1 byte:  Speed (unsigned, km/h * 2)
  1 byte:  Climb (signed, m/s * 10 + 128)
  1 byte:  Heading (unsigned, deg * 256/360)
  1 byte:  Aircraft type
```

Aircraft types:
- 0: Unknown
- 1: Paraglider
- 2: Hangglider
- 3: Balloon
- 4: Glider
- 5: Powered aircraft
- 6: Helicopter
- 7: UAV

### Type 2: Name

```
Payload:
  N bytes: UTF-8 encoded name (max 20 chars)
```

### Type 4: Service (метеостанция)

```
Payload varies by subtype:

Subtype 0: Weather
  2 bytes: Wind heading (deg * 182)
  2 bytes: Wind speed (m/s * 100)
  2 bytes: Wind gusts (m/s * 100)
  2 bytes: Temperature (°C * 100)
  1 byte:  Humidity (%)
  2 bytes: Pressure (hPa - 1000)
  1 byte:  Battery (%)

Subtype 1: Battery only
  1 byte: Battery (%)
```

### Type 7: Ground Tracking (наземный объект)

```
Payload: Same as Type 1 but for ground vehicles
```

### Type 9: Thermal

```
Payload:
  3 bytes: Latitude
  3 bytes: Longitude  
  2 bytes: Altitude (m)
  1 byte:  Quality (0-5)
  2 bytes: Average climb (m/s * 100)
  2 bytes: Wind speed (m/s * 100)
  2 bytes: Wind heading (deg * 182)
```

## Подписка и фильтрация

### Подписка на все базовые станции

```javascript
mqtt.subscribe('fb/b/+/f', (err) => {
  if (!err) {
    console.log('Subscribed to all FANET data');
  }
});
```

### Обработка сообщений

```javascript
mqtt.on('message', (topic, message) => {
  const parts = topic.split('/');
  const chipId = parseInt(parts[2]);
  
  // Парсинг бинарных данных
  const timestamp = message.readUInt32LE(0);
  const rssi = message.readInt16LE(4);
  const snr = message.readInt16LE(6);
  const raw = message.slice(8);
  
  // Обработка FANET пакета
  processFANETPacket(chipId, timestamp, rssi, snr, raw);
});
```

## Quality of Service (QoS)

- **QoS 0** - для tracking данных (допустима потеря)
- **QoS 1** - для критических данных (SOS, collision warning)
- **QoS 2** - не используется (избыточно)

## Производительность

### Ожидаемая нагрузка

- Активные базовые станции: ~50-100
- Сообщений от станции: ~10-50/сек (в пик)
- Общий поток: ~1000-5000 msg/sec
- Размер сообщения: ~50-100 bytes

### Оптимизации

1. **Фильтрация по времени**: игнорировать пакеты старше 1 часа
2. **Дедупликация**: по source address + timestamp
3. **Батчинг**: группировка обновлений для Redis
4. **Приоритизация**: SOS и collision warning обрабатываются первыми