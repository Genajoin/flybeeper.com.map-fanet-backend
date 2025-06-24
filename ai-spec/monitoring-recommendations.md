# Рекомендации по мониторингу FANET API

## Обзор

Этот документ содержит рекомендации по настройке мониторинга и алертов для FANET API сервиса.

## Ключевые метрики для мониторинга

### 1. MySQL Batch Writer

**Метрики производительности:**
- `fanet_mysql_batch_size` - размер батчей (histogram)
- `fanet_mysql_batch_duration_seconds` - время выполнения батчей
- `fanet_mysql_queue_size` - размер очередей по типам (pilots, thermals, stations)
- `fanet_mysql_batch_flushes_total` - количество flush операций по триггерам

**Метрики состояния:**
- `fanet_mysql_batch_writer_status{metric="pilots_queued"}` - всего добавлено в очередь
- `fanet_mysql_batch_writer_status{metric="pilots_processed"}` - успешно обработано
- `fanet_mysql_batch_writer_status{metric="pilots_errors"}` - количество ошибок
- `fanet_mysql_batch_writer_status{metric="last_batch_size"}` - размер последнего батча

**Рекомендуемые алерты:**
```promql
# Очередь переполнена
fanet_mysql_queue_size{entity_type="pilots"} > 5000
  
# Высокий процент ошибок
rate(fanet_mysql_write_errors_total[5m]) > 0.1

# Batch writer не работает (нет flush операций)
rate(fanet_mysql_batch_flushes_total[5m]) == 0
```

### 2. Подключения к базам данных

**Метрики:**
- `fanet_mysql_connection_status` - статус подключения к MySQL (1/0)
- `fanet_redis_connection_status` - статус подключения к Redis (1/0)
- `fanet_mqtt_connection_status` - статус подключения к MQTT (1/0)

**Рекомендуемые алерты:**
```promql
# MySQL недоступен
fanet_mysql_connection_status == 0

# Redis недоступен
fanet_redis_connection_status == 0
```

### 3. HTTP API производительность

**Метрики:**
- `fanet_http_request_duration_seconds` - длительность запросов
- `fanet_http_requests_total` - общее количество запросов

**Рекомендуемые алерты:**
```promql
# Высокая латентность API
histogram_quantile(0.95, rate(fanet_http_request_duration_seconds_bucket[5m])) > 1.0

# Высокий процент ошибок
rate(fanet_http_requests_total{status=~"5.."}[5m]) > 0.05
```

### 4. Валидация данных

**Метрики:**
- `fanet_validation_total_packets` - всего пакетов
- `fanet_validation_validated_packets` - валидных пакетов
- `fanet_validation_rejected_packets` - отклоненных пакетов
- `fanet_validation_invalidated_devices` - инвалидированных устройств

**Рекомендуемые алерты:**
```promql
# Высокий процент отклоненных пакетов
rate(fanet_validation_rejected_packets[5m]) / rate(fanet_validation_total_packets[5m]) > 0.3
```

## Grafana дашборды

### Dashboard 1: Batch Writer Performance

**Панели:**
1. Queue Sizes (график) - размеры очередей по времени
2. Batch Sizes Distribution (histogram) - распределение размеров батчей
3. Processing Rate (график) - скорость обработки записей/сек
4. Error Rate (график) - процент ошибок
5. Flush Triggers (pie chart) - распределение по триггерам flush

### Dashboard 2: System Health

**Панели:**
1. Connection Status (single stat) - статус всех подключений
2. Active Entities (gauge) - количество активных пилотов/термиков/станций
3. MQTT Message Rate (график) - скорость поступления MQTT сообщений
4. API Request Rate (график) - RPS по endpoints

### Dashboard 3: Data Validation

**Панели:**
1. Validation Score Distribution (histogram) - распределение score устройств
2. Rejection Reasons (pie chart) - причины отклонения
3. Device States (table) - таблица состояний устройств
4. Speed Violations by Aircraft Type (bar chart)

## Логирование

### Ключевые события для логирования:

1. **Batch Writer:**
   - Каждый flush с размером батча и длительностью (INFO)
   - Ошибки записи в MySQL (ERROR)
   - Статус каждую минуту (INFO)

2. **Подключения:**
   - Успешное подключение к сервисам (INFO)
   - Потеря соединения (ERROR)
   - Retry попытки (WARN)

3. **Валидация:**
   - Достижение порога валидации устройством (INFO)
   - Инвалидация устройства (WARN)

### Структурированное логирование:

Все логи должны включать контекстные поля:
- `device_id` - для операций с устройствами
- `batch_size` - для batch операций
- `duration` - для измерения производительности
- `error` - детали ошибок

## Рекомендации по настройке

### 1. Prometheus scrape interval
```yaml
scrape_interval: 15s  # Базовый интервал
evaluation_interval: 15s

scrape_configs:
  - job_name: 'fanet-api'
    static_configs:
      - targets: ['fanet-api:8090']
    metrics_path: '/metrics'
```

### 2. Хранение метрик
- Retention: минимум 15 дней для анализа трендов
- Downsampling: агрегация старых данных для экономии места

### 3. Алерты приоритеты
- **P1 (Critical)**: MySQL/Redis недоступны, API не отвечает
- **P2 (Warning)**: Высокий error rate, переполнение очередей
- **P3 (Info)**: Высокая латентность, много отклоненных пакетов

## Инструменты отладки

### 1. Проверка состояния batch writer:
```bash
curl http://localhost:8090/metrics | grep fanet_mysql_batch_writer_status
```

### 2. Проверка очередей:
```bash
curl http://localhost:8090/metrics | grep fanet_mysql_queue_size
```

### 3. Debug логирование:
```bash
export LOG_LEVEL=debug
export MQTT_DEBUG=true
./fanet-api
```

## Тестирование мониторинга

### 1. Симуляция нагрузки:
```bash
make mqtt-test  # Генерация MQTT трафика
```

### 2. Проверка алертов:
- Остановить MySQL контейнер - должен сработать алерт о недоступности
- Заполнить очередь большим количеством сообщений
- Отправить невалидные данные для проверки rejection алертов

## Интеграции

### PagerDuty/Opsgenie
Настроить webhook для критических алертов:
- MySQL/Redis connection lost
- API health check failed
- High error rate sustained > 5 min

### Slack
Информационные алерты:
- Batch writer status summary (ежечасно)
- Validation statistics (ежедневно)