# Модели данных FANET Backend

## Основные сущности

### 1. Pilot (Пилот/UFO)

Представляет летающий объект (параплан, дельтаплан, планер и т.д.)

```go
type Pilot struct {
    // Идентификация
    Addr     uint32    `json:"addr" protobuf:"1"`     // FANET адрес (уникальный ID)
    Name     string    `json:"name" protobuf:"2"`     // Имя пилота
    Type     PilotType `json:"type" protobuf:"3"`     // Тип летательного аппарата
    
    // Позиция
    Position GeoPoint  `json:"position" protobuf:"4"` // Текущие координаты
    Altitude int32     `json:"altitude" protobuf:"5"` // Высота GPS (м)
    
    // Движение
    Speed    float32   `json:"speed" protobuf:"6"`    // Скорость (км/ч)
    Climb    float32   `json:"climb" protobuf:"7"`    // Вертикальная скорость (м/с)
    Course   float32   `json:"course" protobuf:"8"`   // Курс (градусы)
    
    // Статус
    LastUpdate  int64  `json:"last_update" protobuf:"9"`  // Unix timestamp
    TrackOnline bool   `json:"track_online" protobuf:"10"` // Онлайн трекинг
    Battery     uint8  `json:"battery" protobuf:"11"`      // Заряд батареи (%)
}

type PilotType int32
const (
    PilotType_UNKNOWN    PilotType = 0
    PilotType_PARAGLIDER PilotType = 1  // Параплан
    PilotType_HANGGLIDER PilotType = 2  // Дельтаплан
    PilotType_GLIDER     PilotType = 3  // Планер
    PilotType_POWERED    PilotType = 4  // Мотопараплан
    PilotType_HELICOPTER PilotType = 5  // Вертолет
    PilotType_UAV        PilotType = 6  // Дрон
    PilotType_BALLOON    PilotType = 7  // Воздушный шар
)
```

### 2. Thermal (Термический поток)

Информация о восходящих потоках воздуха

```go
type Thermal struct {
    // Идентификация
    ID       uint64    `json:"id" protobuf:"1"`       // Уникальный ID
    Addr     uint32    `json:"addr" protobuf:"2"`     // Кто обнаружил
    
    // Позиция
    Position GeoPoint  `json:"position" protobuf:"3"` // Координаты центра
    Altitude int32     `json:"altitude" protobuf:"4"` // Высота термика (м)
    
    // Характеристики
    Quality  uint8     `json:"quality" protobuf:"5"`  // Качество 0-5
    Climb    float32   `json:"climb" protobuf:"6"`    // Средняя скороподъемность (м/с)
    
    // Ветер на высоте
    WindSpeed   float32 `json:"wind_speed" protobuf:"7"`   // Скорость ветра (м/с)
    WindHeading float32 `json:"wind_heading" protobuf:"8"` // Направление ветра (градусы)
    
    // Метаданные
    Timestamp int64    `json:"timestamp" protobuf:"9"` // Unix timestamp создания
}
```

### 3. Station (Метеостанция)

Наземная метеостанция с погодными данными

```go
type Station struct {
    // Идентификация
    Addr     uint32    `json:"addr" protobuf:"1"`     // FANET адрес станции
    Name     string    `json:"name" protobuf:"2"`     // Название станции
    
    // Позиция
    Position GeoPoint  `json:"position" protobuf:"3"` // Координаты станции
    
    // Погодные данные
    Temperature float32 `json:"temperature" protobuf:"4"` // Температура (°C)
    WindSpeed   float32 `json:"wind_speed" protobuf:"5"`   // Скорость ветра (м/с)
    WindHeading float32 `json:"wind_heading" protobuf:"6"` // Направление ветра (градусы)
    WindGusts   float32 `json:"wind_gusts" protobuf:"7"`   // Порывы ветра (м/с)
    Humidity    uint8   `json:"humidity" protobuf:"8"`     // Влажность (%)
    Pressure    float32 `json:"pressure" protobuf:"9"`     // Давление (гПа)
    
    // Статус
    Battery     uint8   `json:"battery" protobuf:"10"`     // Заряд батареи (%)
    LastUpdate  int64   `json:"last_update" protobuf:"11"` // Unix timestamp
}
```

### 4. Track (Трек полета)

История позиций пилота

```go
type Track struct {
    Addr      uint32        `json:"addr" protobuf:"1"`      // FANET адрес пилота
    Points    []TrackPoint  `json:"points" protobuf:"2"`    // Точки трека
    StartTime int64         `json:"start_time" protobuf:"3"` // Начало трека
    EndTime   int64         `json:"end_time" protobuf:"4"`   // Конец трека
}

type TrackPoint struct {
    Position  GeoPoint `json:"position" protobuf:"1"`  // Координаты
    Altitude  int32    `json:"altitude" protobuf:"2"`  // Высота (м)
    Speed     float32  `json:"speed" protobuf:"3"`     // Скорость (км/ч)
    Climb     float32  `json:"climb" protobuf:"4"`     // Вариометр (м/с)
    Timestamp int64    `json:"timestamp" protobuf:"5"` // Unix timestamp
}
```

## Вспомогательные типы

### GeoPoint (Географическая точка)

```go
type GeoPoint struct {
    Latitude  float64 `json:"lat" protobuf:"1"`  // Широта
    Longitude float64 `json:"lon" protobuf:"2"`  // Долгота
}

// Методы
func (p GeoPoint) DistanceTo(other GeoPoint) float64
func (p GeoPoint) Geohash(precision int) string
func (p GeoPoint) IsInBounds(sw, ne GeoPoint) bool
```

### Bounds (Географические границы)

```go
type Bounds struct {
    Southwest GeoPoint `json:"sw" protobuf:"1"` // Юго-западный угол
    Northeast GeoPoint `json:"ne" protobuf:"2"` // Северо-восточный угол
}

// Методы
func (b Bounds) Contains(point GeoPoint) bool
func (b Bounds) Expand(km float64) Bounds
func (b Bounds) Center() GeoPoint
```

## API типы

### Snapshot (Начальный снимок)

```go
type SnapshotRequest struct {
    Center   GeoPoint `json:"center" protobuf:"1"`   // Центр карты
    Radius   int32    `json:"radius" protobuf:"2"`   // Радиус в км (max 200)
}

type SnapshotResponse struct {
    Pilots   []Pilot   `json:"pilots" protobuf:"1"`   // Пилоты в регионе
    Thermals []Thermal `json:"thermals" protobuf:"2"` // Термики
    Stations []Station `json:"stations" protobuf:"3"` // Метеостанции
    Sequence uint64    `json:"sequence" protobuf:"4"` // Номер последовательности
}
```

### Update (Дифференциальное обновление)

```go
type Update struct {
    Type     UpdateType `json:"type" protobuf:"1"`     // Тип обновления
    Action   Action     `json:"action" protobuf:"2"`   // Действие
    Data     []byte     `json:"data" protobuf:"3"`     // Protobuf данные
    Sequence uint64     `json:"sequence" protobuf:"4"` // Номер последовательности
}

type UpdateType int32
const (
    UpdateType_PILOT   UpdateType = 0
    UpdateType_THERMAL UpdateType = 1
    UpdateType_STATION UpdateType = 2
)

type Action int32
const (
    Action_ADD    Action = 0
    Action_UPDATE Action = 1
    Action_REMOVE Action = 2
)
```

### Position (Отправка позиции)

```go
type PositionRequest struct {
    Position  GeoPoint `json:"position" protobuf:"1"`  // Координаты
    Altitude  int32    `json:"altitude" protobuf:"2"`  // Высота (м)
    Speed     float32  `json:"speed" protobuf:"3"`     // Скорость (км/ч)
    Climb     float32  `json:"climb" protobuf:"4"`     // Вариометр (м/с)
    Course    float32  `json:"course" protobuf:"5"`    // Курс (градусы)
    Timestamp int64    `json:"timestamp" protobuf:"6"` // Unix timestamp
}

type PositionResponse struct {
    Success bool   `json:"success" protobuf:"1"`
    Error   string `json:"error" protobuf:"2"`
}
```

## Валидация

```go
// Пример валидации
func (p *Pilot) Validate() error {
    if p.Addr == 0 {
        return errors.New("addr is required")
    }
    if p.Position.Latitude < -90 || p.Position.Latitude > 90 {
        return errors.New("invalid latitude")
    }
    if p.Position.Longitude < -180 || p.Position.Longitude > 180 {
        return errors.New("invalid longitude")
    }
    if p.Speed < 0 || p.Speed > 400 {
        return errors.New("invalid speed")
    }
    return nil
}
```