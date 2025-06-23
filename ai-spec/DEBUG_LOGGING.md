# Debug Logging для MQTT отладки

## Настройка детального логирования

Для отладки MQTT пакетов и их обработки используйте следующие environment переменные:

### 1. Базовое логирование
```bash
# Установить уровень логирования 
export LOG_LEVEL=debug        # debug, info, warn, error
export LOG_FORMAT=json        # json, text

# Запустить API
make dev
```

### 2. Максимальная детализация (для отладки MQTT)
```bash
# Включить детальный режим MQTT отладки
export LOG_LEVEL=debug
export LOG_FORMAT=json
export MQTT_DEBUG=true

# Запустить API
make dev
```

## Что логируется в debug режиме

### MQTT Client (internal/mqtt/client.go):
- ✅ **Hex dump входящих пакетов** - полные сырые данные
- ✅ **Статус обработки** - успех/ошибка для каждого пакета 
- ✅ **RSSI/SNR значения** - качество сигнала
- ✅ **Детали топика** - chip_id, packet_type

### MQTT Parser (internal/mqtt/parser.go):
- ✅ **Raw data hex** - сырые байты для парсинга
- ✅ **Парсинг координат** - lat_raw, lon_raw, lat_bytes, lon_bytes 
- ✅ **Декодирование полей** - altitude, speed, climb, heading
- ✅ **Валидация координат** - проверка на корректность
- ✅ **Битовые поля** - aircraft_type, online_tracking, scaling

### Redis Operations (cmd/fanet-api/main.go):
- ✅ **Статус сохранения** - успех/ошибка для каждой записи
- ✅ **Данные объектов** - координаты, высота, тип устройства
- ✅ **MySQL batch queue** - статус добавления в очередь
- ✅ **WebSocket трансляция** - статус рассылки обновлений

## Анализ вашего пакета

Для пакета `9B8B5068BAFF0A0001070A20E55D42A20A089496000000` в топике `fb/b/ogn/f/1`:

```bash
# 1. Запустите с debug логированием
export LOG_LEVEL=debug MQTT_DEBUG=true
make dev

# 2. Отправьте пакет в MQTT
mosquitto_pub -h localhost -p 1883 -t "fb/b/ogn/f/1" -m "$(echo '9B8B5068BAFF0A0001070A20E55D42A20A089496000000' | xxd -r -p)"

# 3. Проверьте логи - должны появиться:
# - "Received MQTT message (DEBUG MODE)" с payload_hex
# - "Parsing Air Tracking data (DEBUG)" с raw_data_hex  
# - "Parsed coordinates (DEBUG)" с lat/lon значениями
# - "Valid Air Tracking data parsed successfully (DEBUG)"
# - "Processing pilot data" с координатами
#
# - "Successfully saved pilot to Redis"
```

## ✅ Исправлен маппинг типов летательных аппаратов

**Проблема**: FANET тип 5 (Powered aircraft) неправильно отображался как "HELICOPTER" в API.

**Решение**: Добавлена функция `fanetAircraftTypeToProtobuf()` для корректного маппинга:

| FANET Type | Описание | Protobuf Enum |
|------------|----------|---------------|
| 0 | Other | PILOT_TYPE_UNKNOWN |
| 1 | Paraglider | PILOT_TYPE_PARAGLIDER |
| 2 | Hangglider | PILOT_TYPE_HANGGLIDER |
| 3 | Balloon | PILOT_TYPE_BALLOON |
| 4 | Glider | PILOT_TYPE_GLIDER |
| 5 | **Powered aircraft** | **PILOT_TYPE_POWERED** ← исправлено |
| 6 | Helicopter | PILOT_TYPE_HELICOPTER |
| 7 | UAV | PILOT_TYPE_UAV |

Теперь самолеты (тип 5) корректно отображаются как "POWERED", а не "HELICOPTER".

## Поиск проблем

### Если пакет не появляется в snapshot:

1. **Проверьте MQTT логи**:
   ```bash
   grep "payload_hex.*9b8b5068" logs.json
   ```

2. **Проверьте парсинг координат**:
   ```bash
   grep "Invalid coordinates detected" logs.json
   ```

3. **Проверьте Redis сохранение**:
   ```bash
   grep "Failed to save pilot to Redis" logs.json
   ```

4. **Проверьте TTL в Redis**:
   ```bash
   redis-cli TTL "pilots:200A07"
   ```

### Типичные проблемы:

- **Неверные координаты** - lat/lon вне диапазона ±90/±180
- **Ошибка Redis** - проблемы с подключением или памятью
- **TTL истек** - данные автоматически удалены (pilots: 12h, thermals: 6h)
- **Ошибка парсинга** - неверный формат пакета или размер

## Выключение debug режима

```bash
# Вернуться к обычному логированию
unset MQTT_DEBUG
export LOG_LEVEL=info
```

## Производительность

⚠️ **Внимание**: `MQTT_DEBUG=true` создает много логов и снижает производительность.
Используйте только для отладки, не в production!