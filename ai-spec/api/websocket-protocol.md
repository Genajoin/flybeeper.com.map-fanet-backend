# WebSocket Protocol Specification

## Обзор

WebSocket используется для real-time доставки дифференциальных обновлений клиентам. Все сообщения передаются в Protobuf формате для минимизации трафика.

## Endpoint

```
wss://api.flybeeper.com/ws/v1/updates
ws://localhost:8090/ws/v1/updates (dev)
```

## Параметры подключения

```
/ws/v1/updates?lat=46.5&lon=15.6&radius=200&token=<bearer_token>
```

- `lat` - широта центра карты (обязательно)
- `lon` - долгота центра карты (обязательно)
- `radius` - радиус в км, max 200 (обязательно)
- `token` - Bearer token для авторизованных пользователей (опционально)

## Протокол сообщений

### 1. Handshake

После установки соединения сервер отправляет приветственное сообщение:

```protobuf
message Welcome {
  uint64 server_time = 1;     // Время сервера для синхронизации
  uint64 sequence = 2;        // Текущий номер последовательности
  string server_version = 3;  // Версия сервера
}
```

### 2. Подписка на регион

Клиент может изменить регион подписки:

```protobuf
message SubscribeRequest {
  GeoPoint center = 1;     // Новый центр карты
  int32 radius = 2;        // Новый радиус
  uint64 last_sequence = 3; // Последняя полученная последовательность
}
```

Сервер отвечает:

```protobuf
message SubscribeResponse {
  bool success = 1;
  string error = 2;         // Если success = false
  repeated string geohashes = 3; // Подписанные geohash регионы
}
```

### 3. Дифференциальные обновления

Сервер отправляет батчи обновлений каждые 5 секунд (или чаще при критических изменениях):

```protobuf
message UpdateBatch {
  repeated Update updates = 1;  // Список обновлений
  int64 timestamp = 2;         // Время батча
}

message Update {
  UpdateType type = 1;     // PILOT, THERMAL, STATION
  Action action = 2;       // ADD, UPDATE, REMOVE
  bytes data = 3;          // Protobuf данные соответствующего типа
  uint64 sequence = 4;     // Номер последовательности
}
```

### 4. Heartbeat

Для поддержания соединения используется ping/pong:

```protobuf
message Ping {
  int64 timestamp = 1;
}

message Pong {
  int64 timestamp = 1;     // Echo от клиента
  int64 server_time = 2;   // Текущее время сервера
}
```

Интервал: каждые 30 секунд. Если pong не получен в течение 60 секунд, соединение закрывается.

### 5. Отписка

При отключении клиент может отправить:

```protobuf
message UnsubscribeRequest {
  string reason = 1;       // Причина отключения
}
```

## Обработка обновлений

### Типы обновлений

1. **ADD** - новый объект появился в регионе
2. **UPDATE** - существующий объект изменил позицию/данные
3. **REMOVE** - объект покинул регион или offline

### Пример обработки на клиенте

```javascript
ws.onmessage = (event) => {
  const batch = UpdateBatch.decode(new Uint8Array(event.data));
  
  for (const update of batch.updates) {
    switch (update.type) {
      case UpdateType.PILOT:
        const pilot = Pilot.decode(update.data);
        handlePilotUpdate(update.action, pilot);
        break;
      case UpdateType.THERMAL:
        const thermal = Thermal.decode(update.data);
        handleThermalUpdate(update.action, thermal);
        break;
      case UpdateType.STATION:
        const station = Station.decode(update.data);
        handleStationUpdate(update.action, station);
        break;
    }
    
    // Сохраняем последнюю sequence
    lastSequence = update.sequence;
  }
};
```

## Восстановление соединения

При разрыве соединения клиент должен:

1. Переподключиться с экспоненциальным backoff (1s, 2s, 4s, 8s, max 30s)
2. Отправить `SubscribeRequest` с `last_sequence`
3. Получить пропущенные обновления

## Оптимизации

### 1. Региональная фильтрация

Сервер отправляет только обновления для geohash регионов, на которые подписан клиент:

```
Precision 5 geohash = ~5km × 5km
Для радиуса 200км = ~1600 geohash ячеек
```

### 2. Батчинг

Обновления группируются и отправляются батчами:
- Обычный интервал: 5 секунд
- Критические обновления (SOS, collision): немедленно
- Максимум 100 обновлений в батче

### 3. Компрессия

WebSocket поддерживает per-message deflate:
```
Sec-WebSocket-Extensions: permessage-deflate
```

### 4. Адаптивные интервалы

При низкой активности интервал увеличивается до 10 секунд.
При высокой активности уменьшается до 1 секунды.

## Безопасность

1. **Rate limiting**: максимум 10 подписок в минуту
2. **Размер региона**: максимум 200км радиус
3. **Аутентификация**: Bearer token для записи позиции
4. **Валидация**: все входные данные проверяются

## Метрики

Сервер отслеживает:
- Количество активных соединений
- Среднее количество обновлений/сек
- Размер батчей
- Latency доставки
- Количество переподключений