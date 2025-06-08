# MySQL Batch Writer Architecture

## Обзор

MySQL Batch Writer - это высокопроизводительный асинхронный компонент для массовой записи MQTT данных в MySQL. Разработан для обработки до 10,000 сообщений в секунду без блокировки основного MQTT pipeline.

## Архитектура

### Компоненты

```
MQTT Message → Queue (10k buffer) → Worker Pool (10 workers) → Batch INSERT → MySQL
```

#### 1. BatchWriter (`internal/service/batch_writer.go`)
- **Асинхронные очереди** для каждого типа данных (pilots, thermals, stations)
- **Worker pool** с configurable количеством workers
- **Graceful shutdown** с flush всех pending данных
- **Метрики производительности** в реальном времени

#### 2. MySQL Repository (`internal/repository/mysql.go`)
- **Batch INSERT методы**: `SavePilotsBatch`, `SaveThermalsBatch`, `SaveStationsBatch`  
- **Транзакционная безопасность** с rollback при ошибках
- **Placeholder generation** для эффективных массовых вставок

#### 3. MQTT Integration (`cmd/fanet-api/main.go`)
- **Неблокирующая отправка** в batch queues
- **Type 2 (Name) поддержка** для обновления имен пилотов
- **Error handling** с logging всех ошибок

## Конфигурация

### BatchConfig

```go
type BatchConfig struct {
    BatchSize       int           // Размер батча (по умолчанию 1000)
    FlushInterval   time.Duration // Интервал flush (по умолчанию 5s)
    ChannelBuffer   int           // Размер буфера канала (по умолчанию 10000)
    WorkerCount     int           // Количество workers (по умолчанию 10)
    MaxRetries      int           // Максимум повторов (по умолчанию 3)
    RetryDelay      time.Duration // Задержка между повторами (по умолчанию 100ms)
}
```

### Переменные окружения

```bash
# Обязательная переменная для активации batch writer
MYSQL_DSN="root:password@tcp(localhost:3306)/fanet?parseTime=true"

# Опциональные (используются значения по умолчанию)
BATCH_SIZE=1000
FLUSH_INTERVAL=5s
WORKER_COUNT=10
```

## Производительность

### Целевые показатели

| Метрика | Значение | Описание |
|---------|----------|----------|
| Throughput | 10,000 msg/sec | Максимальная пропускная способность |
| Batch Size | 1,000 записей | Записей в одном батче |
| Flush Timeout | 5 секунд | Максимальная задержка записи |
| Queue Buffer | 10,000 записей | Размер буфера для каждого типа |
| Worker Count | 10 workers | Количество параллельных workers |

### Алгоритм батчинга

1. **Накопление**: Сообщения накапливаются в буферах до достижения `BatchSize` или истечения `FlushInterval`
2. **Flush**: Worker забирает батч и выполняет batch INSERT в транзакции
3. **Retry**: При ошибках применяется exponential backoff с `MaxRetries` попытками
4. **Graceful shutdown**: При остановке все pending батчи записываются

## Типы данных

### 1. Pilots (Type 1 + Type 2)

**Type 1 (Air Tracking)**:
```sql
INSERT INTO ufo_track (addr, ufo_type, latitude, longitude, altitude_gps, 
                       speed, climb, course, track_online, datestamp)
```

**Type 2 (Name)**:
```sql
INSERT INTO name (addr, name) VALUES (?, ?)
ON DUPLICATE KEY UPDATE name = VALUES(name)
```

### 2. Thermals (Type 9)

```sql
INSERT INTO thermal (addr, latitude, longitude, altitude, quality, climb,
                     wind_speed, wind_heading)
```

### 3. Stations (Type 4)

```sql
INSERT INTO station (addr, name, latitude, longitude, temperature, wind_heading,
                     wind_speed, wind_gusts, humidity, pressure, battery, datestamp)
ON DUPLICATE KEY UPDATE name = VALUES(name), ...
```

## Метрики

### BatchMetrics

```go
type BatchMetrics struct {
    // Счетчики по типам
    PilotsQueued      int64 // Добавлено в очередь
    PilotsBatched     int64 // Обработанные батчи
    PilotsProcessed   int64 // Записанные записи
    PilotsErrors      int64 // Ошибки

    // Производительность
    QueueDepthPilots   int64         // Текущая глубина очереди
    LastFlushDuration  time.Duration // Время последнего flush
    LastBatchSize      int           // Размер последнего батча
}
```

### Prometheus метрики

```go
// Размер батчей
mysql_batch_size histogram

// Глубина очередей  
mysql_queue_depth{queue_type="pilots|thermals|stations"} gauge

// Время выполнения
mysql_batch_duration_seconds histogram

// Количество ошибок
mysql_batch_errors{type="pilot|thermal|station"} counter
```

## Обработка ошибок

### Retry стратегия

```go
func (bw *BatchWriter) retryOperation(operation func() error) error {
    for attempt := 0; attempt <= bw.config.MaxRetries; attempt++ {
        if attempt > 0 {
            time.Sleep(bw.config.RetryDelay * time.Duration(attempt)) // Exponential backoff
        }
        
        if err := operation(); err == nil {
            return nil
        }
    }
    return fmt.Errorf("operation failed after %d retries", bw.config.MaxRetries)
}
```

### Типы ошибок

1. **Queue Full**: Очередь переполнена - сообщение отбрасывается с warning
2. **MySQL Connection**: Проблемы с подключением - retry с backoff
3. **SQL Error**: Ошибки в SQL - batch отбрасывается с error logging
4. **Parsing Error**: Неверный device ID - запись пропускается

## Мониторинг

### Логирование

```bash
# Успешные операции (DEBUG)
[DEBUG] Flushed pilots batch to MySQL batch_size=1000 duration=1.2s

# Ошибки (ERROR) 
[ERROR] Failed to flush pilots batch batch_size=1000 duration=5.1s error="connection refused"

# Предупреждения (WARN)
[WARN] Failed to queue pilot for MySQL batch device_id=100001 error="queue is full"
```

### Алерты

```yaml
# Глубина очереди > 5000
mysql_queue_depth > 5000

# Время flush > 10 секунд
mysql_batch_duration_seconds > 10

# Error rate > 5%
rate(mysql_batch_errors[5m]) / rate(mysql_messages_total[5m]) > 0.05

# Queue full events
increase(mysql_queue_full_total[5m]) > 10
```

## Тестирование

### Команды

```bash
# Запуск с batch writer
MYSQL_DSN="root:password@tcp(localhost:3306)/fanet?parseTime=true" make dev

# Нагрузочное тестирование
make mqtt-test-quick  # 50 сообщений за 30 секунд
make mqtt-test        # Полный тест производительности

# Проверка данных в MySQL
docker exec docker-mysql-1 mysql -uroot -ppassword -e "
  USE fanet; 
  SELECT COUNT(*) FROM ufo_track WHERE datestamp > DATE_SUB(NOW(), INTERVAL 1 HOUR);
"
```

### Ожидаемые результаты

```sql
-- После mqtt-test-quick (50 сообщений)
SELECT COUNT(*) FROM ufo_track;  -- ~15-20 записей (Type 1)
SELECT COUNT(*) FROM name;       -- ~8-10 записей (Type 2)  
SELECT COUNT(*) FROM thermal;    -- ~5-8 записей (Type 9)
SELECT COUNT(*) FROM station;    -- ~5-8 записей (Type 4)
```

## Troubleshooting

### Частые проблемы

1. **Данные не записываются в MySQL**
   - Проверьте установку `MYSQL_DSN`
   - Проверьте подключение к MySQL
   - Смотрите логи на ошибки batch writer

2. **Высокая latency**
   - Увеличьте `WORKER_COUNT`
   - Уменьшите `BATCH_SIZE`  
   - Оптимизируйте MySQL (индексы, InnoDB settings)

3. **Queue overflow**
   - Увеличьте `ChannelBuffer`
   - Уменьшите `FlushInterval`
   - Добавьте больше workers

4. **MySQL connection errors**
   - Проверьте max_connections в MySQL
   - Настройте connection pooling
   - Увеличьте timeout'ы

### Дебаггинг

```bash
# Подробные логи
LOG_LEVEL=debug make dev

# Метрики через HTTP
curl http://localhost:8090/metrics | grep mysql

# Проверка queue depth
# В логах batch writer каждые 5 секунд выводится статистика
```

## Масштабирование

### Вертикальное масштабирование

- **Увеличение workers**: Больше параллельных операций
- **Больше RAM**: Увеличение буферов каналов  
- **Faster storage**: SSD для MySQL, улучшение I/O

### Горизонтальное масштабирование

- **Шардирование MySQL**: Разделение по регионам
- **Partitioning**: Партиционирование таблиц по времени
- **Read replicas**: Разделение аналитических запросов

Batch Writer архитектура готова к production использованию с высокой нагрузкой и обеспечивает надежную, производительную запись MQTT данных в MySQL.