# FlyBeeper FANET Backend

Высокопроизводительный Go backend для real-time отслеживания FANET устройств с асинхронным MySQL batch writer для обработки до 10,000 сообщений в секунду.

## Ключевые особенности

- **Real-time данные**: Прямая подписка на MQTT без задержек
- **Высокая производительность**: MySQL batch writer до 10,000 msg/sec
- **Энергоэффективность**: HTTP/2 + Protobuf = -90% трафика
- **Масштабируемость**: 10000+ concurrent connections
- **Региональная фильтрация**: Автоматическая подписка на радиус 200км
- **Дифференциальные обновления**: Только изменения после начального снимка
- **Низкая латентность**: < 50ms для региональных запросов
- **Асинхронная архитектура**: Неблокирующие MySQL операции

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

**Production**: См. [deployments/](deployments/) для Docker/Kubernetes.

## API

### REST Endpoints (HTTP/2)

```
GET  /api/v1/snapshot?lat=46.5&lon=15.6&radius=200   # Начальный снимок
GET  /api/v1/pilots?bounds=45.5,15.0,47.5,16.2       # Пилоты в регионе
GET  /api/v1/thermals?bounds=45.5,15.0,47.5,16.2     # Термики
GET  /api/v1/stations?bounds=45.5,15.0,47.5,16.2     # Метеостанции
GET  /api/v1/track/{addr}                            # Трек пилота
POST /api/v1/position                                # Отправка позиции (auth)
```

### WebSocket

```
/ws/v1/updates?lat=46.5&lon=15.6&radius=200         # Real-time обновления
```

## Производительность

- **Латентность**: 5ms (было 800ms в PHP)
- **Трафик**: 30KB (было 300KB)
- **CPU**: 20% (было 80%)
- **Concurrent**: 10000 (было 100)

## Требования

- Go 1.23+
- Docker и Docker Compose
- Make

## Лицензия

MIT