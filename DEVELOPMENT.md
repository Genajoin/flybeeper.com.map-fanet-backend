# DEVELOPMENT.md

Руководство по разработке FANET Backend API

## 🚀 Быстрый старт

### Предварительные требования
- **Docker** и Docker Compose
- **Go 1.23+** 
- **Make**
- **Git**

### Установка и запуск

```bash
# 1. Клонирование репозитория
git clone <repository-url>
cd flybeeper.com.map-fanet-backend

# 2. Установка зависимостей и генерация Protobuf
make deps
make proto

# 3. Запуск инфраструктуры (Redis, MQTT, MySQL)
make dev-env

# 4. Запуск API с hot reload
make dev
```

**Результат:** API доступен на http://localhost:8090

## 🔧 Сервисы и порты

| Сервис | URL | Описание |
|--------|-----|----------|
| **FANET API** | http://localhost:8090 | Основное API |
| **Redis** | localhost:6379 | Кеш и геопространственные данные |
| **MQTT** | localhost:1883 | FANET сообщения от устройств |
| **MySQL** | localhost:3306 | Резервная БД (user: root, pass: password) |
| **Redis Commander** | http://localhost:8081 | Веб-интерфейс для Redis |
| **Adminer** | http://localhost:8082 | Веб-интерфейс для MySQL |
| **Metrics** | http://localhost:8090/metrics | Метрики WebSocket |

## 📋 Основные команды

```bash
# Разработка
make dev          # Запуск с hot reload
make build        # Сборка бинарника
make test         # Тесты
make lint         # Линтер

# Инфраструктура  
make dev-env      # Поднять Redis/MQTT/MySQL
make dev-env-down # Остановить инфраструктуру

# Protobuf
make proto        # Генерация .pb.go файлов

# Docker
make docker-build # Сборка Docker образа
```

## 🛠️ Структура проекта

```
/cmd/fanet-api/     # Точка входа приложения
/internal/
  ├── auth/         # Аутентификация через Laravel API
  ├── config/       # Конфигурация из environment
  ├── geo/          # Геопространственные операции
  ├── handler/      # HTTP и WebSocket обработчики
  ├── models/       # Pilot, Thermal, Station модели
  ├── mqtt/         # MQTT клиент и FANET парсер
  ├── repository/   # Redis/MySQL слой данных
  └── service/      # Бизнес-логика
/pkg/pb/            # Сгенерированный Protobuf код
/ai-spec/           # Спецификации протоколов
/deployments/       # Docker конфигурации
```

## 🔍 Отладка и тестирование

### REST API endpoints

```bash
# Health check
curl http://localhost:8090/health

# Снимок данных в регионе
curl "http://localhost:8090/api/v1/snapshot?lat=46.0&lon=8.0&radius=50"

# Пилоты в области
curl "http://localhost:8090/api/v1/pilots?north=47&south=45&east=9&west=7"

# Термики в радиусе
curl "http://localhost:8090/api/v1/thermals?lat=46.0&lon=8.0&radius=50"

# WebSocket статистика
curl http://localhost:8090/metrics
```

### WebSocket тестирование

```javascript
// Подключение к WebSocket
const ws = new WebSocket('ws://localhost:8090/ws/v1/updates?lat=46.0&lon=8.0&radius=50');

ws.onopen = () => console.log('WebSocket connected');
ws.onmessage = (event) => {
    // Данные приходят в бинарном формате (Protobuf)
    console.log('Binary message received:', event.data);
};
```

### MQTT тестирование

```bash
# Подключение к MQTT брокеру
mosquitto_sub -h localhost -p 1883 -t "fb/b/+/f" -v

# Отправка тестового FANET сообщения
mosquitto_pub -h localhost -p 1883 -t "fb/b/test-station/f" \
  -m "$(echo -en '\x12\x34\x56\x78\x01\x23\x45\x01\x02\x03\x04\x05\x06\x07\x08')"
```

## 🔧 Веб-интерфейсы

### Redis Commander (http://localhost:8081)
- Просмотр геопространственных индексов
- Мониторинг TTL ключей
- Анализ Redis команд

### Adminer (http://localhost:8082)
- **Server:** mysql  
- **Username:** root
- **Password:** password
- **Database:** fanet

## 📊 Мониторинг

### Логи
```bash
# Логи API (с hot reload)
tail -f logs/app.log

# Логи Docker сервисов
docker logs docker-fanet-api-1
docker logs docker-redis-1
docker logs docker-mqtt-1
docker logs docker-mysql-1
```

### Метрики
- **WebSocket**: количество подключений, sequence numbers
- **HTTP**: latency, status codes, client IPs
- **MQTT**: подключения, подписки на топики
- **Redis**: геопространственные запросы, TTL

## 🧪 Тестирование

### Unit тесты
```bash
make test
```

### Нагрузочное тестирование
```bash
# Benchmark тесты
make bench

# Профилирование CPU
make profile-cpu

# Профилирование памяти  
make profile-mem
```

## 🔀 Workflow разработки

### 1. Изменение Protobuf схем
```bash
# После изменения ai-spec/api/fanet.proto
make proto
# Автоматически перезапустится через air
```

### 2. Изменение кода
- Air автоматически пересоберет и перезапустит API
- Логи покажут результат в реальном времени
- Hot reload сохраняет состояние подключений

### 3. Тестирование изменений
```bash
# Быстрая проверка
curl http://localhost:8090/health

# Проверка WebSocket
# Открыть в браузере: ws://localhost:8090/ws/v1/updates?lat=46&lon=8&radius=50
```

## 🐛 Troubleshooting

### API не запускается
```bash
# Проверить порты
lsof -i :8090

# Проверить зависимости
make deps
make proto
```

### MQTT не подключается
```bash
# Проверить MQTT брокер
docker logs docker-mqtt-1

# Перезапустить MQTT
docker compose -f deployments/docker/docker-compose.yml restart mqtt
```

### Redis недоступен
```bash
# Проверить Redis
docker logs docker-redis-1

# Подключиться к Redis CLI
docker exec -it docker-redis-1 redis-cli
```

### WebSocket не работает
- Проверить в браузере Developer Tools → Network → WS
- Убедиться что параметры lat/lon/radius корректны
- Проверить логи API на ошибки соединения

## 🔧 Конфигурация

### Environment переменные
Основные переменные (см. `.env.example`):

```bash
# Server
SERVER_PORT=8090
SERVER_ADDRESS=:8090

# Redis  
REDIS_URL=redis://localhost:6379

# MQTT
MQTT_URL=tcp://localhost:1883
MQTT_TOPIC_PREFIX=fb/b/+/f

# MySQL (опционально)
MYSQL_DSN=root:password@tcp(localhost:3306)/fanet?parseTime=true

# Логирование
LOG_LEVEL=debug
LOG_FORMAT=text

# Производительность
DEFAULT_RADIUS_KM=200
WORKER_POOL_SIZE=100
```

### Режимы работы
```bash
# Development (по умолчанию)
ENVIRONMENT=development

# Production
ENVIRONMENT=production
GIN_MODE=release
LOG_FORMAT=json
```

## 📚 Дополнительные ресурсы

- **Архитектура**: См. `ai-spec/architecture/overview.md`
- **FANET протокол**: См. `ai-spec/mqtt/`
- **REST API**: См. `ai-spec/api/rest-api.yaml`  
- **WebSocket**: См. `ai-spec/api/websocket-protocol.md`
- **Deployment**: См. `deployments/`

## 🤝 Участие в разработке

1. Форкнуть репозиторий
2. Создать feature branch: `git checkout -b feature/amazing-feature`
3. Сделать изменения и тесты
4. Коммит: `git commit -m 'Add amazing feature'`
5. Push: `git push origin feature/amazing-feature`
6. Создать Pull Request

---

**Happy coding! 🚀**