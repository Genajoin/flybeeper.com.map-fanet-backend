# FANET API - Операционная документация

## Мониторинг и наблюдаемость

### 🚀 Быстрый старт мониторинга

```bash
# Запуск мониторинг стека
cd deployments/monitoring
docker-compose up -d

# Доступ к интерфейсам
# Grafana: http://localhost:3000 (admin/fanet_monitor_2024)
# Prometheus: http://localhost:9090
# AlertManager: http://localhost:9093
```

### 📊 Prometheus метрики

API экспортирует следующие метрики на `/metrics`:

#### HTTP метрики
- `fanet_http_request_duration_seconds` - Длительность HTTP запросов
- `fanet_http_requests_total` - Общее количество HTTP запросов (по методу, endpoint, статусу)

#### WebSocket метрики
- `fanet_websocket_connections_active` - Активные WebSocket соединения
- `fanet_websocket_messages_out_total` - Отправленные WebSocket сообщения (по типу)
- `fanet_websocket_errors_total` - Ошибки WebSocket

#### MQTT метрики
- `fanet_mqtt_messages_received_total` - Получено MQTT сообщений (по типу пакета)
- `fanet_mqtt_parse_errors_total` - Ошибки парсинга MQTT
- `fanet_mqtt_connection_status` - Статус MQTT соединения (1 = подключен, 0 = отключен)

#### Redis метрики
- `fanet_redis_operation_duration_seconds` - Длительность Redis операций
- `fanet_redis_operation_errors_total` - Ошибки Redis операций

#### MySQL метрики
- `fanet_mysql_batch_size` - Размер MySQL батчей
- `fanet_mysql_batch_duration_seconds` - Длительность MySQL батчей
- `fanet_mysql_queue_size` - Размер очередей MySQL writer (по типу)
- `fanet_mysql_write_errors_total` - Ошибки записи в MySQL

#### Системные метрики
- `fanet_active_pilots_total` - Активные пилоты в системе
- `fanet_active_thermals_total` - Активные термики в системе
- `fanet_active_stations_total` - Активные станции в системе
- `fanet_app_info` - Информация о версии приложения

### 📈 Grafana дашборды

Включены готовые дашборды:

1. **System Overview** (`fanet-system-overview`)
   - Состояние системы (goroutines, память)
   - Активные объекты (пилоты, термики, станции)
   - Статус MQTT подключения
   - WebSocket соединения

2. **API Performance** (`fanet-api-performance`)
   - RPS по endpoint'ам
   - Процентили времени ответа (p50, p95, p99)
   - Процент ошибок HTTP
   - Производительность Redis операций

3. **MQTT Pipeline** (`fanet-mqtt-pipeline`)
   - Трафик MQTT по типам сообщений
   - Ошибки парсинга
   - MySQL очереди и батчи
   - Распределение типов FANET сообщений

4. **WebSocket Real-time** (`fanet-websocket-realtime`)
   - Активные соединения
   - Статистика исходящих сообщений
   - Ошибки WebSocket

### 🚨 Алерты

Настроены критические алерты:

- **FANETAPIDown** - API недоступен > 1 минуты
- **HighErrorRate** - HTTP ошибки > 5% в течение 5 минут  
- **HighResponseTime** - p95 время ответа > 1 секунды
- **MQTTDisconnected** - MQTT отключен > 2 минут
- **HighMemoryUsage** - Использование памяти > 80%
- **TooManyGoroutines** - Goroutines > 1000
- **MySQLQueueFull** - Очередь MySQL > 8000 элементов
- **WebSocketConnectionDrop** - Резкое падение соединений

### 📋 Операционные процедуры

#### Проверка состояния системы

```bash
# Health check
curl http://localhost:8090/health

# Метрики Prometheus  
curl http://localhost:8090/metrics

# Статистика MQTT
curl http://localhost:8090/debug/pprof/ # В debug режиме
```

#### Мониторинг производительности

```bash
# Профилирование (debug режим)
go tool pprof http://localhost:8090/debug/pprof/profile
go tool pprof http://localhost:8090/debug/pprof/heap

# Мониторинг goroutines
go tool pprof http://localhost:8090/debug/pprof/goroutine
```

#### Диагностика проблем

1. **Высокая латентность**:
   - Проверить Redis операции в Grafana
   - Профилировать CPU и memory
   - Проверить MySQL batch writer

2. **MQTT проблемы**:
   - Проверить `fanet_mqtt_connection_status` 
   - Анализировать `fanet_mqtt_parse_errors_total`
   - Логи MQTT клиента

3. **WebSocket проблемы**:
   - Мониторить `fanet_websocket_errors_total`
   - Проверить активные соединения
   - Анализировать логи WebSocket handler

4. **MySQL проблемы**:
   - Размер очередей `fanet_mysql_queue_size`
   - Ошибки записи `fanet_mysql_write_errors_total`
   - Время батчей `fanet_mysql_batch_duration_seconds`

### 🔧 Настройка мониторинга

#### Prometheus конфигурация

Для production обновить `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'fanet-api'
    static_configs:
      - targets: ['your-api-host:8090']
```

#### Grafana настройка

1. Импортировать дашборды из `deployments/monitoring/dashboards/`
2. Настроить data source: Prometheus URL
3. Настроить уведомления (Slack, email)

#### AlertManager настройка

Обновить `alertmanager.yml`:

```yaml
receivers:
  - name: 'production-alerts'
    slack_configs:
      - api_url: 'YOUR_SLACK_WEBHOOK'
        channel: '#production-alerts'
```

### 📊 Ключевые SLA метрики

- **Доступность**: > 99.9% (fanet_http_requests_total)
- **Латентность**: p95 < 100ms (fanet_http_request_duration_seconds)
- **Пропускная способность**: > 10k msg/sec MQTT
- **Ошибки**: < 0.1% HTTP 5xx errors

### 🎯 Production checklist

- [ ] Prometheus scraping настроен
- [ ] Grafana дашборды импортированы
- [ ] AlertManager notifications настроены
- [ ] SLA метрики мониторятся
- [ ] Log aggregation настроен (ELK/Loki)
- [ ] Backup мониторинга данных
- [ ] Документированы runbooks для алертов