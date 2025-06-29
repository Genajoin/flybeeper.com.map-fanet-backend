# FlyBeeper FANET Backend

Высокопроизводительный Go backend для real-time отслеживания FANET устройств с асинхронным MySQL batch writer для обработки до 10,000 сообщений в секунду.

## Ключевые особенности

- **Real-time данные**: Прямая подписка на MQTT без задержек
- **Система валидации**: Автоматическая фильтрация недостоверных данных
- **Высокая производительность**: MySQL batch writer до 10,000 msg/sec
- **SSO аутентификация**: Laravel Passport интеграция с Redis кешированием
- **Энергоэффективность**: HTTP/2 + Protobuf = -90% трафика
- **Масштабируемость**: 10000+ concurrent connections
- **Региональная фильтрация**: Автоматическая подписка на радиус 200км
- **Дифференциальные обновления**: Только изменения после начального снимка
- **Низкая латентность**: < 50ms для региональных запросов
- **Асинхронная архитектура**: Неблокирующие MySQL операции
- **🔥 Boundary Tracking**: Решение проблемы зависания ЛА на границе OGN области

## Архитектура

```
Frontend ←→ Go API Server ←→ Redis Cache (real-time)
                ↑              ↑
         HTTP/2 + Protobuf     │
                ↑              │
         Bearer Token Auth     │
                               ↓
                         MQTT Broker ←→ FANET Devices
                               ↑
                    Batch Writer (async 10k msg/sec)
                               ↓
                          MySQL (storage)
```

## Быстрый старт

```bash
# Установка зависимостей и запуск среды разработки
make deps && make proto
make dev-env  # Запуск Redis, MQTT, MySQL

# Запуск API с MySQL batch writer (рекомендуется)
MYSQL_DSN="root:password@tcp(localhost:3306)/fanet?parseTime=true" make dev

# Альтернативно: только Redis (без MySQL)
make dev      # API с hot reload на localhost:8090
```

**Для разработчиков**: См. [DEVELOPMENT.md](DEVELOPMENT.md) для подробных инструкций.

**Frontend интеграция**: См. [FRONTEND_INTEGRATION.md](FRONTEND_INTEGRATION.md) для подключения к maps.flybeeper.com.

**Система валидации**: См. [VALIDATION_USAGE.md](VALIDATION_USAGE.md) для работы с фильтрацией данных.

**Production**: См. [deployments/](deployments/) для Docker/Kubernetes.

## API

### REST Endpoints (HTTP/2)

```bash
GET  /api/v1/snapshot?lat=46.5&lon=15.6&radius=200   # Начальный снимок
GET  /api/v1/pilots?bounds=45.5,15.0,47.5,16.2       # Пилоты в регионе  
GET  /api/v1/thermals?bounds=45.5,15.0,47.5,16.2     # Термики
GET  /api/v1/stations?bounds=45.5,15.0,47.5,16.2     # Метеостанции
GET  /api/v1/track/{addr}                            # Трек пилота
POST /api/v1/position                                # Отправка позиции (🔒 auth)

# Система валидации
GET  /api/v1/validation/metrics                      # Метрики валидации
GET  /api/v1/validation/{device_id}                  # Состояние устройства
POST /api/v1/invalidate/{device_id}                  # Инвалидация устройства
```

### WebSocket Real-time

```bash
/ws/v1/updates?lat=46.5&lon=15.6&radius=200         # Real-time обновления
```

### Аутентификация

```bash
# 1. Логин через Laravel API  
POST https://api.flybeeper.com/api/v4/login

# 2. Использование Bearer token
Authorization: Bearer {token}
```

**Подробная документация**: [ai-spec/auth-integration.md](ai-spec/auth-integration.md)

## Производительность

- **Латентность**: 5ms (было 800ms в PHP)
- **Трафик**: 30KB (было 300KB)
- **CPU**: 20% (было 80%)
- **Concurrent**: 10000 (было 100)

## 📚 Документация

### 🔥 Новые функции:
- **[Boundary Tracking](ai-spec/BOUNDARY_TRACKING.md)** - Решение проблемы зависания ЛА на границе OGN области
- **[Changelog v2.4.0](ai-spec/CHANGELOG_BOUNDARY_TRACKING.md)** - Подробности реализации boundary tracking

### 📖 Основная документация:
- **[🔐 Аутентификация](ai-spec/auth-integration.md)** - Laravel Passport интеграция и SSO
- **[📊 Система валидации](ai-spec/VALIDATION_SYSTEM.md)** - Фильтрация недостоверных данных
- **[🏗 Архитектура](ai-spec/architecture/)** - Обзор системы, производительность, deployment
- **[📡 MQTT протокол](ai-spec/mqtt/)** - Форматы сообщений, типы пакетов, топики
- **[🌐 API документация](ai-spec/api/)** - REST API, WebSocket протокол, Protobuf схемы
- **[💾 База данных](ai-spec/database/)** - Модели данных, Redis схема, MySQL legacy
- **[📈 Мониторинг](ai-spec/monitoring-recommendations.md)** - Метрики, алерты, дашборды

## Требования

- Go 1.23+
- Docker и Docker Compose
- Make

## Лицензия

MIT