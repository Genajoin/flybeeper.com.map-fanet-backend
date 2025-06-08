# FlyBeeper FANET Backend

Высокопроизводительный Go backend для real-time отслеживания FANET устройств с минимальным энергопотреблением.

## Ключевые особенности

- **Real-time данные**: Прямая подписка на MQTT без задержек
- **Энергоэффективность**: HTTP/2 + Protobuf = -90% трафика
- **Масштабируемость**: 10000+ concurrent connections
- **Региональная фильтрация**: Автоматическая подписка на радиус 200км
- **Дифференциальные обновления**: Только изменения после начального снимка
- **Низкая латентность**: < 50ms для региональных запросов

## Архитектура

```
Frontend ←→ Go API Server ←→ Redis Cache
                ↑              ↑
         HTTP/2 + Protobuf     │
                ↑              │
         Bearer Token Auth     │
                               ↓
                         MQTT Broker ←→ FANET Devices
                               ↑
                        MqttToDb (legacy)
                               ↓
                          MySQL (backup)
```

## Быстрый старт

```bash
# Установка зависимостей и запуск среды разработки
make deps && make proto
make dev-env  # Запуск Redis, MQTT, MySQL
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