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
- **Go 1.21+** с HTTP/2 и Protobuf
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
- Topic pattern: `fanet/+/+` (region/device_id)
- Парсинг FANET протокола в `internal/mqtt/parser.go`
- Автореконнект и обработка ошибок

### Redis использование
- Геопространственные индексы для pilots/thermals
- TTL 24 часа для автоочистки
- Pipeline для батчевых операций

### WebSocket протокол
- Бинарные сообщения (Protobuf)
- Heartbeat каждые 30 секунд
- Graceful reconnect с сохранением состояния

### Аутентификация
- Bearer token проверяется через Laravel API
- Кеширование валидных токенов на 5 минут
- Rate limiting: 100 req/min per IP

## Конфигурация окружения

Основные переменные:
- `REDIS_URL` - подключение к Redis
- `MQTT_URL` - подключение к MQTT broker  
- `AUTH_ENDPOINT` - Laravel API для проверки токенов
- `DEFAULT_RADIUS_KM` - радиус фильтрации (200км)

## Текущий статус

Проект на ранней стадии. Завершена базовая инфраструктура, в процессе реализация:
- MQTT клиент и парсер FANET
- Redis репозиторий с геозапросами
- REST API endpoints
- WebSocket handler
- Оптимизации производительности