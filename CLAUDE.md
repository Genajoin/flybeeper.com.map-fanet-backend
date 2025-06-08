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

### WebSocket протокол ✅ ЗАВЕРШЕН
- Бинарные сообщения (Protobuf)
- Heartbeat каждые 30 секунд
- Graceful reconnect с сохранением состояния
- Endpoint: /ws/v1/updates
- Geohash фильтрация по регионам (precision 5)
- Real-time трансляция MQTT → WebSocket

### Аутентификация (частично)
- Bearer token middleware с базовой валидацией
- TODO: Валидация через Laravel API
- TODO: Кеширование валидных токенов на 5 минут
- Rate limiting: 100 req/sec per IP (burst 200)

## Конфигурация окружения

Основные переменные:
- `SERVER_PORT` - порт API (по умолчанию 8090)
- `REDIS_URL` - подключение к Redis (redis://localhost:6379)
- `MQTT_URL` - подключение к MQTT broker (tcp://localhost:1883)
- `MYSQL_DSN` - MySQL для fallback данных
- `AUTH_ENDPOINT` - Laravel API для проверки токенов
- `DEFAULT_RADIUS_KM` - радиус фильтрации (200км)
- `LOG_LEVEL` - уровень логирования (debug/info/warn/error)

## Текущий статус

**Этапы 1-3 ПОЛНОСТЬЮ ЗАВЕРШЕНЫ** - Функциональная система готова к production:

### ✅ Завершено (100%):
- **MQTT клиент и парсер FANET** - полное соответствие спецификации ai-spec/mqtt/
- **Redis репозиторий** - геопространственные запросы, TTL, mapper функции
- **MySQL fallback** - загрузка начальных данных, синхронизация с Redis  
- **HTTP/2 сервер** - Gin + middleware + Protobuf/JSON поддержка
- **REST API endpoints** - все endpoints согласно OpenAPI спецификации
- **WebSocket handler** - real-time обновления с geohash фильтрацией
- **MQTT-WebSocket интеграция** - полный pipeline MQTT→Redis→WebSocket
- **Структурированное логирование** - WithField/WithFields pattern во всех компонентах
- **Среда разработки** - Docker Compose + hot reload + отладочные интерфейсы
- **Документация разработчика** - DEVELOPMENT.md с полными инструкциями

### ⚠️ Частично завершено:
- **Аутентификация** - базовая проверка Bearer token (требует Laravel API интеграция)

### 🔄 Следующие приоритеты:
- **Полная аутентификация** - интеграция с Laravel API
- **Оптимизации производительности** - профилирование, кеширование
- **Production deployment** - Kubernetes манифесты

### 🚀 Среда разработки готова:
```bash
# Быстрый старт разработки
make deps && make proto  # Установка и генерация
make dev-env             # Поднять Redis, MQTT, MySQL  
make dev                 # API с hot reload на localhost:8090

# Отладочные интерфейсы
# Redis Commander: http://localhost:8081
# Adminer (MySQL): http://localhost:8082
# API Health: http://localhost:8090/health
# WebSocket: ws://localhost:8090/ws/v1/updates?lat=46&lon=8&radius=50
```

## Дополнительные директивы

- По-возможности используй спецификации @ai-spec/
- При изменении .proto файлов всегда запускай `make proto`
- Для тестирования используй среду разработки через `make dev-env && make dev`
- Порт API изменен с 8080 на 8090 для избежания конфликтов
- Полная документация разработчика в DEVELOPMENT.md
- При проблемах с MQTT проверь docker logs docker-mqtt-1

## Команды для быстрого тестирования

```bash
# Проверка API
curl http://localhost:8090/health
curl "http://localhost:8090/api/v1/snapshot?lat=46.0&lon=8.0&radius=50"

# Проверка WebSocket (в браузере console)
const ws = new WebSocket('ws://localhost:8090/ws/v1/updates?lat=46&lon=8&radius=50');

# Проверка MQTT
mosquitto_pub -h localhost -p 1883 -t "fb/b/test/f" -m "test-message"
```