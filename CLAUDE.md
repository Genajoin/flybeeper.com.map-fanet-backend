# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Проект: FANET API Backend

Высокопроизводительный Go backend для real-time отслеживания FANET устройств (парапланы, дельтапланы).

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

# MQTT тестирование
make mqtt-test        # Запустить тестовый издатель MQTT
make mqtt-test-quick  # Быстрый тест (50 сообщений)

# Debug логирование MQTT (NEW!)
./test-mqtt-debug.sh  # Настройка детального логирования для отладки
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
5. **Высокопроизводительная архитектура**: Асинхронный MySQL batch writer для 10k msg/sec

## Важные детали реализации

### MQTT интеграция ✅ ОБНОВЛЕНО
- Topic pattern: `fb/b/{chip_id}/f/{packet_type}` (новый формат с packet_type)
- Подписка: `fb/b/+/f/#` для всех базовых станций и типов
- Обертка базовой станции: timestamp (4) + RSSI (2) + SNR (2) + FANET пакет
- FANET заголовок: 1 байт (тип в битах 0-2) + 3 байта адрес источника
- Поддерживаемые типы: 1 (Air), 2 (Name), 4 (Service), 7 (Ground), 9 (Thermal)
- Координаты: lat * 93206.04, lon * 46603.02 (24-bit signed, исправлены коэффициенты для Type 4)
- Парсинг FANET протокола в `internal/mqtt/parser.go`
- Валидация соответствия packet_type из топика и FANET заголовка
- Автореконнект и обработка ошибок
- **Тестирование**: `make mqtt-test` для публикации тестовых данных

### Redis использование
- Геопространственные индексы для pilots/thermals/stations
- TTL 24 часа для автоочистки (pilots: 12h, thermals: 6h, stations: 24h)
- Pipeline для батчевых операций
- HSET для детальных данных с маппингом

### MySQL batch writer ✅ НОВОЕ
- **Асинхронные очереди** для высокопроизводительной записи (до 10k msg/sec)
- **Batch INSERT** операции: 1000 записей в батче или 5-секундный flush
- **Worker pool** архитектура с retry логикой и graceful shutdown
- **Неблокирующие операции** - MQTT обработка не ждет MySQL
- **Type 2 (Name) поддержка** для обновления имен пилотов
- **Метрики производительности**: batch size, queue depth, latency
- **Транзакционная безопасность** с rollback при ошибках

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

### Аутентификация ✅ ЗАВЕРШЕН
- **Laravel Passport интеграция** - полная валидация через API `/api/v4/user`
- **SSO архитектура** - единая точка входа для всех сервисов FlyBeeper
- **Redis кеширование** - валидные токены кешируются на 5 минут
- **Middleware аутентификации** - Bearer token в header/query/cookie
- **User context** - данные пользователя доступны в handlers
- **POST /position защищен** - требует аутентификацию для отправки координат
- **Спецификация**: см. `ai-spec/auth-integration.md`
- Rate limiting: 100 req/sec per IP (burst 200), 200 req/sec для аутентифицированных

## Конфигурация окружения

### Основные переменные:
- `SERVER_PORT` - порт API (по умолчанию 8090)
- `REDIS_URL` - подключение к Redis (redis://localhost:6379)
- `MQTT_URL` - подключение к MQTT broker (tcp://localhost:1883)
- `MYSQL_DSN` - MySQL для высокопроизводительной записи и backup (требуется для batch writer)
- `AUTH_ENDPOINT` - Laravel API для проверки токенов (по умолчанию: https://api.flybeeper.com/api/v4/user)
- `AUTH_CACHE_TTL` - время кеширования токенов (по умолчанию: 5m)
- `DEFAULT_RADIUS_KM` - радиус фильтрации (200км)

### 🔥 **Boundary Tracking переменные (НОВОЕ):**
- `OGN_CENTER_LAT` - широта центра OGN (по умолчанию: 46.5)
- `OGN_CENTER_LON` - долгота центра OGN (по умолчанию: 14.2)
- `OGN_RADIUS_KM` - радиус отслеживания OGN в км (по умолчанию: 200)
- `TRACKING_RADIUS_PERCENT` - процент от OGN радиуса для внутренней зоны (по умолчанию: 0.9)
- `BOUNDARY_GRACE_PERIOD` - время показа объектов после выхода за границу (по умолчанию: 5m)
- `MIN_MOVEMENT_DISTANCE` - минимальное движение в метрах для обновления (по умолчанию: 100)

### 🔧 Логирование и отладка (NEW!)
- `LOG_LEVEL` - уровень логирования (debug/info/warn/error)
- `LOG_FORMAT` - формат логов (json/text)
- `MQTT_DEBUG` - детальное логирование MQTT пакетов (true/false)

**Для отладки MQTT пакетов:**
```bash
export LOG_LEVEL=debug LOG_FORMAT=json MQTT_DEBUG=true
make dev
# Или используйте: ./test-mqtt-debug.sh
```

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
# Prometheus метрики: http://localhost:8090/metrics
```

## Дополнительные директивы

- По-возможности используй спецификации @ai-spec/
- Новую документацию пиши в @ai-spec/
- При изменении .proto файлов всегда запускай `make proto`
- Среда разработки развернута в контенерах уровнем выше. api запущен с air для hot reload. имя контейнера flybeeper-fanet-api-1 при этом используются переменные из ./.env
- Полная документация разработчика в DEVELOPMENT.md
- При проблемах с MQTT проверь docker logs docker-mqtt-1
- используй unix форматирование окончания строк

## Команды для быстрого тестирования

```bash
# Проверка API
curl http://localhost:8090/health
curl "http://localhost:8090/api/v1/snapshot?lat=46.0&lon=13.0&radius=200"

# Проверка WebSocket (в браузере console)
const ws = new WebSocket('ws://localhost:8090/ws/v1/updates?lat=46&lon=13&radius=200');

# Тестирование аутентификации
curl -H "Authorization: Bearer YOUR_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{"position":{"latitude":46.0,"longitude":13.0},"altitude":1000,"timestamp":1640995200}' \
     http://localhost:8090/api/v1/position

# Тестирование MQTT с реальными FANET данными
make mqtt-test-quick  # Быстрый тест (50 сообщений)
make mqtt-test        # Полноценное тестирование

# Ручная публикация тестовых данных
scripts/mqtt-test.sh -r 1s -m 10 -t 1,2  # Только tracking и name

# Тестирование MySQL batch writer
MYSQL_DSN="root:password@tcp(localhost:3306)/fanet?parseTime=true" make dev
make mqtt-test-quick  # После теста проверить данные в MySQL через Adminer

# Мониторинг и метрики
curl http://localhost:8090/metrics  # Prometheus метрики
cd deployments/monitoring && docker-compose up -d  # Запуск Grafana + Prometheus
```

## 🌐 Frontend Integration

Для интеграции с frontend приложением (maps.flybeeper.com) см. полную спецификацию:
**📖 [ai-spec/auth-integration.md](ai-spec/auth-integration.md)**

### Краткий workflow:
1. **Логин через Laravel API**: `POST https://api.flybeeper.com/api/v4/login`
2. **Получение Bearer token** из ответа Laravel API  
3. **Использование токена для FANET API**: `Authorization: Bearer {token}`
4. **Отправка позиций**: `POST /api/v1/position` с аутентификацией
5. **WebSocket подключение**: `ws://api.flybeeper.com/ws/v1/updates?token={token}` (будущее)

