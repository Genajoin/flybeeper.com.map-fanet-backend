# Performance Targets

## Целевые показатели производительности

### Латентность

| Операция | p50 | p95 | p99 | Max |
|----------|-----|-----|-----|-----|
| REST API (snapshot) | 20ms | 50ms | 100ms | 200ms |
| REST API (region query) | 10ms | 30ms | 50ms | 100ms |
| WebSocket update | 5ms | 15ms | 30ms | 50ms |
| MQTT processing | 1ms | 5ms | 10ms | 20ms |
| Redis query | 1ms | 3ms | 5ms | 10ms |

### Пропускная способность

| Метрика | Целевое значение | Описание |
|---------|------------------|----------|
| MQTT messages/sec | 10,000 | Входящие FANET пакеты |
| HTTP requests/sec | 5,000 | REST API запросы |
| WebSocket connections | 10,000 | Одновременные подключения |
| WebSocket updates/sec | 50,000 | Исходящие обновления |
| Redis operations/sec | 100,000 | Чтение/запись |
| MySQL batch inserts/sec | 10,000 | Асинхронная высокопроизводительная запись |
| MySQL batch size | 1,000 | Записей в одном батче |
| MySQL flush timeout | 5s | Максимальная задержка записи |

### Использование ресурсов

| Ресурс | Норма | Тревога | Критично |
|--------|-------|---------|----------|
| CPU | < 50% | > 70% | > 90% |
| Memory | < 60% | > 80% | > 95% |
| Network IN | < 100 Mbps | > 500 Mbps | > 800 Mbps |
| Network OUT | < 200 Mbps | > 800 Mbps | > 1.5 Gbps |
| Redis Memory | < 70% | > 85% | > 95% |

### Размеры данных

| Тип данных | Средний размер | После сжатия |
|------------|----------------|--------------|
| Pilot (JSON) | 250 bytes | - |
| Pilot (Protobuf) | 80 bytes | 60 bytes |
| Thermal (JSON) | 180 bytes | - |
| Thermal (Protobuf) | 50 bytes | 40 bytes |
| Station (JSON) | 320 bytes | - |
| Station (Protobuf) | 100 bytes | 80 bytes |
| Snapshot (1000 pilots) | 80 KB | 25 KB |
| WebSocket update | 100 bytes | 70 bytes |

## Оптимизации для достижения целей

### 1. CPU оптимизации

#### Параллелизм
```go
// Использование всех CPU ядер
runtime.GOMAXPROCS(runtime.NumCPU())

// Worker pool для MQTT обработки
workerPool := pond.New(100, 10000)
```

#### Zero allocations
```go
// Object pooling
var pilotPool = sync.Pool{
    New: func() interface{} {
        return &Pilot{}
    },
}

// Reuse buffers
var bufferPool = sync.Pool{
    New: func() interface{} {
        return bytes.NewBuffer(make([]byte, 0, 1024))
    },
}
```

### 2. Память

#### Efficient structures
```go
// Использование packed структур
type Position struct {
    Lat int32 // вместо float64
    Lon int32 // сохраняем как int32 * 1e7
}

// String interning для повторяющихся строк
var nameCache = make(map[string]string)
```

#### GC tuning
```go
// Уменьшение давления на GC
debug.SetGCPercent(200) // Реже GC для throughput
// или
os.Setenv("GOGC", "200")
```

### 3. Сеть

#### HTTP/2 оптимизации
```go
server := &http.Server{
    Handler: handler,
    TLSConfig: &tls.Config{
        MinVersion: tls.VersionTLS12,
        CurvePreferences: []tls.CurveID{
            tls.X25519, // Быстрая кривая
        },
    },
    // Настройки HTTP/2
    MaxConcurrentStreams: 1000,
    ReadTimeout:          10 * time.Second,
    WriteTimeout:         10 * time.Second,
    IdleTimeout:          120 * time.Second,
}
```

#### TCP tuning
```go
// Увеличение буферов
listener, _ := net.Listen("tcp", ":8080")
tcpListener := listener.(*net.TCPListener)
tcpListener.SetDeadline(time.Now().Add(60 * time.Second))

// SO_REUSEPORT для нескольких listeners
config := &net.ListenConfig{
    Control: func(network, address string, c syscall.RawConn) error {
        return c.Control(func(fd uintptr) {
            syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEPORT, 1)
        })
    },
}
```

### 4. Redis оптимизации

#### Pipeline
```go
pipe := rdb.Pipeline()
for _, pilot := range pilots {
    pipe.GeoAdd(ctx, "pilots:geo", &redis.GeoLocation{
        Name:      fmt.Sprintf("pilot:%d", pilot.Addr),
        Longitude: pilot.Position.Longitude,
        Latitude:  pilot.Position.Latitude,
    })
    pipe.HSet(ctx, fmt.Sprintf("pilot:%d", pilot.Addr), map[string]interface{}{
        "altitude": pilot.Altitude,
        "speed":    pilot.Speed,
        // ...
    })
}
_, err := pipe.Exec(ctx)
```

#### Connection pooling
```go
rdb := redis.NewClient(&redis.Options{
    Addr:         "localhost:6379",
    PoolSize:     100,
    MinIdleConns: 10,
    MaxRetries:   3,
    DialTimeout:  5 * time.Second,
    ReadTimeout:  3 * time.Second,
    WriteTimeout: 3 * time.Second,
    PoolTimeout:  4 * time.Second,
})
```

### 5. MySQL batch оптимизации

#### Асинхронный batch writer
```go
// Конфигурация высокопроизводительного batch writer
type BatchConfig struct {
    BatchSize     int           // 1000 записей в батче
    FlushInterval time.Duration // 5-секундный flush
    ChannelBuffer int           // 10k буфер очереди
    WorkerCount   int           // 10 worker'ов
    MaxRetries    int           // 3 попытки при ошибках
}

// Неблокирующая отправка в очередь
func (bw *BatchWriter) QueuePilot(pilot *models.Pilot) error {
    select {
    case bw.pilotChan <- pilot:
        return nil
    default:
        return fmt.Errorf("queue is full")
    }
}
```

#### Batch INSERT операции
```go
// Генерация placeholders для массовых вставок
func generatePlaceholders(count, fieldsPerRecord int) string {
    singleRecord := "(" + strings.Repeat("?,", fieldsPerRecord-1) + "?)"
    placeholders := make([]string, count)
    for i := 0; i < count; i++ {
        placeholders[i] = singleRecord
    }
    return strings.Join(placeholders, ",")
}

// Транзакционная безопасность
tx, err := db.BeginTx(ctx, nil)
defer tx.Rollback()
// ... batch operations
tx.Commit()
```

#### Метрики производительности
```go
type BatchMetrics struct {
    PilotsQueued    int64         // Количество в очереди
    PilotsBatched   int64         // Обработанные батчи
    QueueDepth      int64         // Глубина очереди
    LastFlushDuration time.Duration // Время последнего flush
}
```

### 6. Protobuf оптимизации

#### Streaming
```go
// Использование streaming для больших ответов
func (s *Server) StreamPilots(req *pb.StreamRequest, stream pb.FANET_StreamPilotsServer) error {
    // Отправляем по частям
    for _, pilot := range pilots {
        if err := stream.Send(pilot); err != nil {
            return err
        }
    }
    return nil
}
```

#### Buffer reuse
```go
// Глобальный buffer для сериализации
var protoBuffer = proto.Buffer{}

func SerializePilot(p *pb.Pilot) ([]byte, error) {
    protoBuffer.Reset()
    protoBuffer.Marshal(p)
    return protoBuffer.Bytes(), nil
}
```

## Бенчмарки

### Целевые показатели для бенчмарков

```
BenchmarkMQTTProcessing-8        1000000      1050 ns/op       0 B/op       0 allocs/op
BenchmarkRedisGeoQuery-8          200000      8500 ns/op     320 B/op       8 allocs/op
BenchmarkMySQLBatchInsert-8         1000   1200000 ns/op    2048 B/op      10 allocs/op
BenchmarkBatchWriterQueue-8     10000000       150 ns/op      64 B/op       1 allocs/op
BenchmarkProtobufMarshal-8       2000000       750 ns/op     192 B/op       1 allocs/op
BenchmarkWebSocketBroadcast-8     100000     15000 ns/op    1024 B/op      16 allocs/op
```

### Команды для тестирования

```bash
# CPU profiling
go test -bench=. -cpuprofile=cpu.prof
go tool pprof cpu.prof

# Memory profiling
go test -bench=. -memprofile=mem.prof
go tool pprof mem.prof

# Load testing
hey -n 100000 -c 100 -m GET http://localhost:8090/api/v1/pilots
wrk -t12 -c400 -d30s --latency http://localhost:8090/api/v1/pilots

# MySQL batch writer testing
MYSQL_DSN="root:password@tcp(localhost:3306)/fanet?parseTime=true" make dev
make mqtt-test-quick  # Тест 50 сообщений
make mqtt-test        # Полный нагрузочный тест
```

## Мониторинг производительности

### Prometheus метрики

```go
var (
    requestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "http_request_duration_seconds",
            Help:    "HTTP request latencies in seconds.",
            Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
        },
        []string{"handler", "method"},
    )
    
    mqttProcessingRate = prometheus.NewCounter(
        prometheus.CounterOpts{
            Name: "mqtt_messages_processed_total",
            Help: "Total number of MQTT messages processed.",
        },
    )
    
    activeWebsockets = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "websocket_active_connections",
            Help: "Number of active WebSocket connections.",
        },
    )
    
    // MySQL batch writer метрики
    mysqlBatchSize = prometheus.NewHistogram(
        prometheus.HistogramOpts{
            Name: "mysql_batch_size",
            Help: "Size of MySQL batch operations.",
            Buckets: []float64{100, 250, 500, 750, 1000, 1500, 2000},
        },
    )
    
    mysqlQueueDepth = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "mysql_queue_depth",
            Help: "Current depth of MySQL batch queues.",
        },
        []string{"queue_type"}, // pilots, thermals, stations
    )
    
    mysqlBatchDuration = prometheus.NewHistogram(
        prometheus.HistogramOpts{
            Name: "mysql_batch_duration_seconds",
            Help: "Time spent executing MySQL batch operations.",
            Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
        },
    )
)
```

### Grafana дашборды

- Request rate и latency
- Error rate по типам
- Goroutines и memory usage
- Redis операции и hit rate
- WebSocket connections
- MQTT processing rate
- MySQL batch writer метрики:
  - Queue depth по типам (pilots, thermals, stations)
  - Batch size distribution
  - Flush duration и frequency
  - Error rate и retry operations