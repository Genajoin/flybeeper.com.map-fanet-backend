# Boundary Tracking System - Решение проблемы зависания ЛА на границе OGN

## Проблема

Когда летательный аппарат (ЛА) выходит за границу области отслеживания OGN, обновления его позиции прекращаются. Однако, из-за TTL в Redis (12 часов для пилотов), старые позиции продолжают отображаться в снимках, создавая визуальный эффект "зависания" объектов на границе отслеживаемой области.

## Реализованное решение

### 1. Статический центр OGN с двухуровневой системой радиусов

Система использует один статический центр OGN с двумя радиусами:

- **OGN Radius** - внешний радиус, из которого поступают данные OGN (например, 200км)
- **Tracking Radius** - внутренний радиус для включения в snapshot (по умолчанию 90% от OGN radius)

Объекты между этими радиусами находятся в "пограничной зоне" (boundary zone).

### 2. Компоненты решения

#### BoundaryTracker Service (`internal/service/boundary_tracker.go`)

Основной сервис для отслеживания границ OGN:

```go
type BoundaryTracker struct {
    ognCenter          models.GeoPoint
    ognRadiusKM        float64       // Внешний радиус OGN
    trackingRadiusKM   float64       // Внутренний радиус для snapshot
    gracePeriod        time.Duration // Время показа после выхода за границу
    minMovementDist    float64       // Минимальное расстояние движения
}
```

Основные методы:
- `GetObjectStatus()` - определяет статус объекта относительно границ OGN
- `ShouldIncludeInSnapshot()` - решает, включать ли объект в снимок
- `CalculateVisibilityScore()` - вычисляет коэффициент видимости (0.0-1.0)
- `GetOGNInfo()` - возвращает информацию о конфигурации OGN

#### Расширенная модель Pilot

В модель `Pilot` добавлены поля для отслеживания границ:

```go
type Pilot struct {
    // ... существующие поля ...
    
    // Поля для отслеживания границ
    LastMovement     *time.Time // Время последнего значимого движения
    TrackingDistance float64    // Расстояние от центра OGN в км
    VisibilityStatus string     // "visible", "boundary", "outside"
}
```

### 3. Логика обработки

#### При получении данных MQTT:

1. Получаем предыдущую позицию пилота из Redis
2. Определяем статус объекта через `BoundaryTracker`
3. Обновляем поля `TrackingDistance`, `VisibilityStatus`, `LastMovement`
4. Сохраняем в Redis с дополнительными полями

#### При запросе снимка (snapshot):

1. Получаем пилотов из Redis в запрошенном радиусе
2. Фильтруем через `ShouldIncludeInSnapshot()`:
   - Включаем объекты внутри tracking radius OGN
   - Включаем объекты в boundary zone, если не истек grace period
   - Исключаем остальные

### 4. Интеграция с моделями данных

Boundary tracking интегрирован в модель `Pilot` с дополнительными полями:

```go
// В internal/models/pilot.go добавлены поля:
LastMovement     *time.Time `json:"last_movement,omitempty"`
TrackingDistance float64    `json:"tracking_distance,omitempty"`  
VisibilityStatus string     `json:"visibility_status,omitempty"`
```

Эти поля:
- Сохраняются в Redis через `SavePilot()` 
- Загружаются через `GetPilotsInRadius()`
- Используются в REST API для фильтрации snapshot

### 5. Конфигурация

Новые параметры в `GeoConfig`:

```go
type GeoConfig struct {
    // ... существующие поля ...
    
    // OGN центр отслеживания
    OGNCenterLat           float64       // Широта центра OGN
    OGNCenterLon           float64       // Долгота центра OGN  
    OGNRadiusKM            float64       // Радиус отслеживания OGN в км
    
    TrackingRadiusPercent  float64       // Процент от OGN радиуса (default: 0.9)
    BoundaryGracePeriod    time.Duration // Grace period (default: 5m)
    MinMovementDistance    float64       // Мин. движение в метрах (default: 100)
}
```

Переменные окружения:
- `OGN_CENTER_LAT` - широта центра OGN (default: 46.5)
- `OGN_CENTER_LON` - долгота центра OGN (default: 14.2)
- `OGN_RADIUS_KM` - радиус OGN в километрах (default: 200)
- `TRACKING_RADIUS_PERCENT` - процент от OGN радиуса (0.9 = 90%)
- `BOUNDARY_GRACE_PERIOD` - время показа после выхода за границу (5m)
- `MIN_MOVEMENT_DISTANCE` - минимальное движение для обновления (100м)

### 6. Статусы видимости

- **"visible"** - объект внутри tracking radius OGN, полностью видим
- **"boundary"** - объект между tracking и OGN radius
- **"outside"** - объект за пределами OGN radius или истек grace period

### 7. Grace Period

Объекты в boundary zone продолжают отображаться в течение grace period (5 минут по умолчанию) после последнего значимого движения. Это позволяет избежать "мигания" объектов на границе.

### 8. Visibility Score

Для плавного исчезновения объектов на границе вычисляется visibility score:
- 1.0 - полная видимость (visible)
- 0.3-1.0 - плавное уменьшение в течение grace period (boundary)
- 0.0 - невидим (outside)

Frontend может использовать этот коэффициент для отображения объектов с разной прозрачностью.

## Преимущества решения

1. **Устраняет зависание** - объекты за пределами tracking radius не включаются в снимок
2. **Единый центр OGN** - простая и понятная конфигурация
3. **Плавные переходы** - grace period предотвращает резкое исчезновение
4. **Гибкая настройка** - все параметры конфигурируемые
5. **Минимальное влияние на производительность** - простая проверка расстояния
6. **Обратная совместимость** - старые клиенты продолжают работать

## Тестирование

Реализованы unit-тесты в `boundary_tracker_test.go`:
- Тестирование различных позиций относительно границ OGN
- Проверка grace period
- Тестирование отслеживания движения
- Проверка visibility score для плавного исчезновения

## Настройка переменных окружения

Добавьте в `.env` файл:

```bash
# OGN центр отслеживания (Словения/Австрия)
OGN_CENTER_LAT=46.5
OGN_CENTER_LON=14.2
OGN_RADIUS_KM=200

# Boundary tracking настройки
TRACKING_RADIUS_PERCENT=0.9
BOUNDARY_GRACE_PERIOD=5m
MIN_MOVEMENT_DISTANCE=100
```

## Примеры тестирования

```bash
# Запуск API с boundary tracking
make dev

# Тестирование разных зон
# 1. Объект внутри tracking radius (180км от центра) - включается в snapshot
curl "http://localhost:8090/api/v1/snapshot?lat=46.5&lon=14.2&radius=200"

# 2. Объект в boundary zone (185км от центра) - включается с grace period
curl "http://localhost:8090/api/v1/snapshot?lat=47.7&lon=14.2&radius=200"

# 3. Объект за пределами OGN (220км от центра) - исключается из snapshot
curl "http://localhost:8090/api/v1/snapshot?lat=48.1&lon=14.2&radius=200"

# Запуск unit-тестов
go test ./internal/service/ -v -run TestBoundaryTracker
```

## Мониторинг и отладка

```bash
# Проверка логов boundary tracker
make dev LOG_LEVEL=debug

# Метрики (в будущих версиях)
curl http://localhost:8090/metrics | grep boundary
```

## Будущие улучшения

1. **Интеграция с WebSocket** - передача visibility status клиентам в реальном времени
2. **Множественные OGN центры** - поддержка нескольких OGN станций
3. **Предиктивная фильтрация** - предсказание выхода за границу на основе вектора движения
4. **Адаптивный tracking radius** - автоматическая настройка на основе плотности трафика