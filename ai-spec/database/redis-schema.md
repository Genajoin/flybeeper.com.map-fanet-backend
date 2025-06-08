# Redis Schema для FANET Backend

## Обзор

Redis используется как основное хранилище для real-time данных с геопространственными индексами и быстрым доступом.

## Структура ключей

### 1. Пилоты/UFO

```redis
# Геопространственный индекс
GEOADD pilots:geo <longitude> <latitude> pilot:<addr>

# Данные пилота
HSET pilot:<addr>
  name <string>           # Имя пилота
  type <int>              # Тип летательного аппарата
  altitude <int>          # Высота GPS (м)
  speed <float>           # Скорость (км/ч)
  climb <float>           # Вертикальная скорость (м/с)
  course <float>          # Курс (градусы)
  last_update <timestamp> # Время последнего обновления
  track_online <bool>     # Онлайн трекинг
  battery <int>           # Заряд батареи (%)

# Трек пилота (последние 1000 точек)
LPUSH track:<addr> <protobuf_bytes>  # Сериализованная позиция
LTRIM track:<addr> 0 999             # Ограничение размера

# TTL для автоочистки
EXPIRE pilot:<addr> 43200            # 12 часов
```

### 2. Термики

```redis
# Геопространственный индекс
GEOADD thermals:geo <longitude> <latitude> thermal:<id>

# Индекс по времени
ZADD thermals:time <timestamp> thermal:<id>

# Данные термика
HSET thermal:<id>
  addr <int>              # Кто обнаружил
  altitude <int>          # Высота (м)
  quality <int>           # Качество 0-5
  climb <float>           # Скороподъемность (м/с)
  wind_speed <float>      # Скорость ветра (м/с)
  wind_heading <float>    # Направление ветра
  timestamp <timestamp>   # Время создания

# TTL для автоочистки
EXPIRE thermal:<id> 21600            # 6 часов
```

### 3. Метеостанции

```redis
# Геопространственный индекс
GEOADD stations:geo <longitude> <latitude> station:<addr>

# Данные станции
HSET station:<addr>
  name <string>           # Название станции
  temperature <float>     # Температура (°C)
  wind_speed <float>      # Скорость ветра (м/с)
  wind_heading <float>    # Направление ветра
  wind_gusts <float>      # Порывы ветра (м/с)
  humidity <int>          # Влажность (%)
  pressure <float>        # Давление (гПа)
  battery <int>           # Заряд батареи (%)
  last_update <timestamp> # Последнее обновление

# История метеоданных
LPUSH station:<addr>:history <protobuf_bytes>
LTRIM station:<addr>:history 0 287   # 24 часа с 5-мин интервалом

# TTL
EXPIRE station:<addr> 86400          # 24 часа
```

### 4. Подписки клиентов

```redis
# Регионы подписки клиента (geohash префиксы)
SADD client:<id>:regions <geohash1> <geohash2> ...

# Метаданные клиента
HSET client:<id>
  connected_at <timestamp>
  last_ping <timestamp>
  auth_token <string>     # Хэш токена
  center_lat <float>      # Центр карты
  center_lon <float>
  radius <int>            # Радиус в км

# TTL для автоочистки отключенных
EXPIRE client:<id> 300               # 5 минут после последнего ping
```

### 5. Очереди обновлений

```redis
# Очередь обновлений для WebSocket клиентов
LPUSH updates:<geohash> <protobuf_update>
LTRIM updates:<geohash> 0 99        # Максимум 100 обновлений

# Глобальный счетчик последовательности
INCR sequence:global
```

### 6. Кэш аутентификации

```redis
# Кэш проверенных Bearer токенов
SETEX auth:token:<token_hash> 3600 <user_info_json>
```

## Geohash стратегия

Используем geohash для эффективной региональной фильтрации:

- Precision 4: ~20km × 20km (для грубой фильтрации)
- Precision 5: ~5km × 5km (для точной фильтрации)
- Precision 6: ~1.2km × 1.2km (для локальных данных)

```go
// Пример: поиск в радиусе 200км
func FindInRadius(lat, lon float64, radiusKm int) []string {
    // Получаем geohash префиксы для покрытия радиуса
    prefixes := geohash.Cover(lat, lon, radiusKm)
    
    // Ищем объекты в каждом префиксе
    for _, prefix := range prefixes {
        // GEORADIUS pilots:geo lon lat radius km
    }
}
```

## Оптимизации

### 1. Pipeline для батчинга
```go
pipe := redis.Pipeline()
for _, update := range updates {
    pipe.HSet(ctx, fmt.Sprintf("pilot:%d", update.Addr), ...)
    pipe.GeoAdd(ctx, "pilots:geo", ...)
}
pipe.Exec(ctx)
```

### 2. Lua скрипты для атомарности
```lua
-- Атомарное обновление позиции с проверкой времени
local key = KEYS[1]
local timestamp = ARGV[1]
local data = ARGV[2]

local lastUpdate = redis.call('HGET', key, 'last_update')
if not lastUpdate or tonumber(timestamp) > tonumber(lastUpdate) then
    redis.call('HSET', key, 'last_update', timestamp)
    redis.call('HSET', key, 'data', data)
    return 1
end
return 0
```

### 3. Redis Streams для real-time
```redis
# Альтернатива: использование Redis Streams
XADD updates:stream * type pilot action update data <protobuf>
XREAD STREAMS updates:stream $
```

## Мониторинг

```redis
# Счетчики для метрик
INCR stats:pilots:updates
INCR stats:thermals:created
INCR stats:websocket:connections

# Размеры данных
DBSIZE
MEMORY USAGE pilot:*
```