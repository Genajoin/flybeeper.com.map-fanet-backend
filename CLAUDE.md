# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Проект: FANET API Backend

Высокопроизводительный Go backend для real-time отслеживания FANET устройств (парапланы, дельтапланы). Заменяет PHP/Laravel решение, обеспечивая 94% снижение латентности и 90% экономию трафика.

## Команды разработки

```bash
# Установка зависимостей
make deps

# Генерация Protobuf (после изменения .proto файлов)
make proto

# Локальная разработка с hot reload
make dev

# Запуск тестов
make test

# Поднять окружение разработки (Redis, MQTT, MySQL)
make dev-env

# Docker сборка
make docker-build
```

## Архитектура

### Технологический стек
- **Go 1.23+** с HTTP/2 и Protobuf
- **Gin** HTTP framework с middleware
- **Redis** для кеширования и геопространственных запросов
- **MQTT** для получения данных от устройств
- **WebSocket** для real-time обновлений клиентов

### Структура кода
```
/cmd/fanet-api/     - Точка входа
/internal/
  ├── auth/         - Аутентификация через Laravel API  
  ├── config/       - Конфигурация из environment
  ├── geo/          - Геопространственные операции (Geohash)
  ├── handler/      - HTTP и WebSocket обработчики
  ├── models/       - Pilot, Thermal, Station
  ├── mqtt/         - MQTT клиент и FANET парсер
  ├── repository/   - Redis/MySQL слой
  └── service/      - Бизнес-логика
/pkg/pb/            - Сгенерированный Protobuf код
```

### Ключевые архитектурные решения

1. **Геофильтрация**: Redis GEO команды для поиска объектов в радиусе 200км
2. **Дифференциальные обновления**: Полный снимок при подключении, затем только изменения
3. **Энергоэффективность**: Protobuf вместо JSON, батчинг обновлений, адаптивные интервалы
4. **Stateless архитектура** для горизонтального масштабирования

## Важные детали реализации

### MQTT интеграция
- Topic pattern: `fb/b/+/f` (базовая_станция/chip_id/fanet)
- Обертка базовой станции: timestamp (4) + RSSI (2) + SNR (2) + FANET пакет
- FANET заголовок: 1 байт (тип в битах 0-2) + 3 байта адрес источника
- Поддерживаемые типы: 1 (Air), 2 (Name), 4 (Service), 7 (Ground), 9 (Thermal)
- Координаты: lat * 93206.04, lon * 46603.02 (24-bit signed)
- Парсинг FANET протокола в `internal/mqtt/parser.go`
- Автореконнект и обработка ошибок

### Redis использование
- Геопространственные индексы для pilots/thermals/stations
- TTL 24 часа для автоочистки (pilots: 12h, thermals: 6h, stations: 24h)
- Pipeline для батчевых операций
- HSET для детальных данных с маппингом

### REST API
- HTTP/2 с Gin framework
- Protobuf и JSON поддержка (по Accept header)
- OpenAPI 3.0 совместимость
- Middleware: logging, CORS, rate limiting, security headers
- Endpoints: /snapshot, /pilots, /thermals, /stations, /track, /position

### WebSocket протокол (TODO)
- Бинарные сообщения (Protobuf)
- Heartbeat каждые 30 секунд
- Graceful reconnect с сохранением состояния
- Endpoint: /ws/v1/updates

### Аутентификация (частично)
- Bearer token middleware с базовой валидацией
- TODO: Валидация через Laravel API
- TODO: Кеширование валидных токенов на 5 минут
- Rate limiting: 100 req/sec per IP (burst 200)

## Конфигурация окружения

Основные переменные:
- `REDIS_URL` - подключение к Redis
- `MQTT_URL` - подключение к MQTT broker  
- `AUTH_ENDPOINT` - Laravel API для проверки токенов
- `DEFAULT_RADIUS_KM` - радиус фильтрации (200км)

## Текущий статус

**Этап 3 ЗАВЕРШЕН** - HTTP/2 сервер и REST API полностью реализованы:

### ✅ Завершено:
- **MQTT клиент и парсер FANET** - полное соответствие спецификации ai-spec/mqtt/
- **Redis репозиторий** - геопространственные запросы, TTL, mapper функции
- **MySQL fallback** - загрузка начальных данных, синхронизация с Redis  
- **HTTP/2 сервер** - Gin + middleware + Protobuf/JSON поддержка
- **REST API endpoints** - все endpoints согласно OpenAPI спецификации
- **Структурированное логирование** - WithField/WithFields pattern
- **Конвертеры** - между FANET сообщениями, внутренними моделями и Protobuf

### 🔄 В процессе:
- **WebSocket handler** - real-time обновления (следующий приоритет)
- **Полная аутентификация** - интеграция с Laravel API
- **Оптимизации производительности** - профилирование, кеширование

### 📦 Готов к запуску:
```bash
# Генерация Protobuf
bash scripts/proto-gen.sh

# Компиляция
go build ./cmd/fanet-api

# Запуск (требует Redis и MQTT broker)
./fanet-api
```

## Дополнительные директивы

- По-возможности используй спецификации @ai-spec/