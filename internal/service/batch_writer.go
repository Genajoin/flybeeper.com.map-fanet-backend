package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/flybeeper/fanet-backend/internal/metrics"
	"github.com/flybeeper/fanet-backend/internal/models"
	"github.com/flybeeper/fanet-backend/internal/repository"
	"github.com/flybeeper/fanet-backend/pkg/utils"
)

// BatchWriter асинхронный writer для батчевого сохранения в MySQL
type BatchWriter struct {
	mysqlRepo repository.MySQLRepositoryInterface
	logger    *utils.Logger
	config    *BatchConfig

	// Каналы для разных типов данных
	pilotChan   chan *models.Pilot
	thermalChan chan *models.Thermal
	stationChan chan *models.Station

	// Буферы для батчинга
	pilotBuffer   []*models.Pilot
	thermalBuffer []*models.Thermal
	stationBuffer []*models.Station

	// Контроль жизненного цикла
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Метрики
	metrics *BatchMetrics
}

// BatchConfig конфигурация батчера
type BatchConfig struct {
	BatchSize       int           `json:"batch_size"`        // Размер батча
	FlushInterval   time.Duration `json:"flush_interval"`    // Интервал принудительного flush
	ChannelBuffer   int           `json:"channel_buffer"`    // Размер буфера канала
	WorkerCount     int           `json:"worker_count"`      // Количество worker'ов
	MaxRetries      int           `json:"max_retries"`       // Максимум повторов
	RetryDelay      time.Duration `json:"retry_delay"`       // Задержка между повторами
}

// BatchMetrics метрики производительности
type BatchMetrics struct {
	mu sync.RWMutex

	// Счетчики
	PilotsQueued    int64 `json:"pilots_queued"`
	PilotsBatched   int64 `json:"pilots_batched"`
	PilotsProcessed int64 `json:"pilots_processed"`
	PilotsErrors    int64 `json:"pilots_errors"`

	ThermalsQueued    int64 `json:"thermals_queued"`
	ThermalsBatched   int64 `json:"thermals_batched"`
	ThermalsProcessed int64 `json:"thermals_processed"`
	ThermalsErrors    int64 `json:"thermals_errors"`

	StationsQueued    int64 `json:"stations_queued"`
	StationsBatched   int64 `json:"stations_batched"`
	StationsProcessed int64 `json:"stations_processed"`
	StationsErrors    int64 `json:"stations_errors"`

	// Производительность
	QueueDepthPilots   int64         `json:"queue_depth_pilots"`
	QueueDepthThermals int64         `json:"queue_depth_thermals"`
	QueueDepthStations int64         `json:"queue_depth_stations"`
	LastFlushDuration  time.Duration `json:"last_flush_duration"`
	LastBatchSize      int           `json:"last_batch_size"`
}

// DefaultBatchConfig возвращает конфигурацию по умолчанию
func DefaultBatchConfig() *BatchConfig {
	return &BatchConfig{
		BatchSize:     1000,                // 1000 записей в батче
		FlushInterval: 5 * time.Second,     // Flush каждые 5 секунд
		ChannelBuffer: 10000,               // Буфер канала 10k записей
		WorkerCount:   10,                  // 10 worker'ов для MySQL
		MaxRetries:    3,                   // 3 попытки при ошибках
		RetryDelay:    100 * time.Millisecond, // 100ms между попытками
	}
}

// NewBatchWriter создает новый BatchWriter
func NewBatchWriter(mysqlRepo repository.MySQLRepositoryInterface, logger *utils.Logger, config *BatchConfig) *BatchWriter {
	if config == nil {
		config = DefaultBatchConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	bw := &BatchWriter{
		mysqlRepo: mysqlRepo,
		logger:    logger,
		config:    config,
		ctx:       ctx,
		cancel:    cancel,

		// Каналы с буферизацией
		pilotChan:   make(chan *models.Pilot, config.ChannelBuffer),
		thermalChan: make(chan *models.Thermal, config.ChannelBuffer),
		stationChan: make(chan *models.Station, config.ChannelBuffer),

		// Буферы для батчинга
		pilotBuffer:   make([]*models.Pilot, 0, config.BatchSize),
		thermalBuffer: make([]*models.Thermal, 0, config.BatchSize),
		stationBuffer: make([]*models.Station, 0, config.BatchSize),

		metrics: &BatchMetrics{},
	}

	// Запускаем worker'ы
	bw.start()

	return bw
}

// start запускает worker'ы для обработки батчей
func (bw *BatchWriter) start() {
	// Worker для пилотов
	bw.wg.Add(1)
	go bw.pilotWorker()

	// Worker для термиков
	bw.wg.Add(1)
	go bw.thermalWorker()

	// Worker для станций
	bw.wg.Add(1)
	go bw.stationWorker()
	
	// Worker для периодического обновления метрик
	bw.wg.Add(1)
	go bw.metricsWorker()

	bw.logger.WithField("batch_size", bw.config.BatchSize).
		WithField("flush_interval", bw.config.FlushInterval).
		WithField("worker_count", bw.config.WorkerCount).
		Info("Started MySQL batch writer")
}

// QueuePilot добавляет пилота в очередь для сохранения
func (bw *BatchWriter) QueuePilot(pilot *models.Pilot) error {
	select {
	case bw.pilotChan <- pilot:
		bw.metrics.mu.Lock()
		bw.metrics.PilotsQueued++
		bw.metrics.QueueDepthPilots = int64(len(bw.pilotChan))
		bw.metrics.mu.Unlock()
		
		// Обновляем Prometheus метрику
		metrics.MySQLQueueSize.WithLabelValues("pilots").Set(float64(len(bw.pilotChan)))
		return nil
	case <-bw.ctx.Done():
		return fmt.Errorf("batch writer is shutting down")
	default:
		bw.metrics.mu.Lock()
		bw.metrics.PilotsErrors++
		bw.metrics.mu.Unlock()
		
		// Увеличиваем счетчик ошибок
		metrics.MySQLWriteErrors.WithLabelValues("queue_full").Inc()
		return fmt.Errorf("pilot queue is full")
	}
}

// QueueThermal добавляет термик в очередь для сохранения
func (bw *BatchWriter) QueueThermal(thermal *models.Thermal) error {
	select {
	case bw.thermalChan <- thermal:
		bw.metrics.mu.Lock()
		bw.metrics.ThermalsQueued++
		bw.metrics.QueueDepthThermals = int64(len(bw.thermalChan))
		bw.metrics.mu.Unlock()
		
		// Обновляем Prometheus метрику
		metrics.MySQLQueueSize.WithLabelValues("thermals").Set(float64(len(bw.thermalChan)))
		return nil
	case <-bw.ctx.Done():
		return fmt.Errorf("batch writer is shutting down")
	default:
		bw.metrics.mu.Lock()
		bw.metrics.ThermalsErrors++
		bw.metrics.mu.Unlock()
		
		// Увеличиваем счетчик ошибок
		metrics.MySQLWriteErrors.WithLabelValues("queue_full").Inc()
		return fmt.Errorf("thermal queue is full")
	}
}

// QueueStation добавляет станцию в очередь для сохранения
func (bw *BatchWriter) QueueStation(station *models.Station) error {
	select {
	case bw.stationChan <- station:
		bw.metrics.mu.Lock()
		bw.metrics.StationsQueued++
		bw.metrics.QueueDepthStations = int64(len(bw.stationChan))
		bw.metrics.mu.Unlock()
		
		// Обновляем Prometheus метрику
		metrics.MySQLQueueSize.WithLabelValues("stations").Set(float64(len(bw.stationChan)))
		return nil
	case <-bw.ctx.Done():
		return fmt.Errorf("batch writer is shutting down")
	default:
		bw.metrics.mu.Lock()
		bw.metrics.StationsErrors++
		bw.metrics.mu.Unlock()
		
		// Увеличиваем счетчик ошибок
		metrics.MySQLWriteErrors.WithLabelValues("queue_full").Inc()
		return fmt.Errorf("station queue is full")
	}
}

// pilotWorker обрабатывает батчи пилотов
func (bw *BatchWriter) pilotWorker() {
	defer bw.wg.Done()

	ticker := time.NewTicker(bw.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case pilot := <-bw.pilotChan:
			bw.pilotBuffer = append(bw.pilotBuffer, pilot)
			
			// Флашим при достижении размера батча
			if len(bw.pilotBuffer) >= bw.config.BatchSize {
				metrics.MySQLBatchFlushes.WithLabelValues("pilots", "size_limit").Inc()
				bw.flushPilots()
			}

		case <-ticker.C:
			// Периодический flush даже если батч не полный
			if len(bw.pilotBuffer) > 0 {
				metrics.MySQLBatchFlushes.WithLabelValues("pilots", "interval").Inc()
				bw.flushPilots()
			}

		case <-bw.ctx.Done():
			// Финальный flush при завершении
			if len(bw.pilotBuffer) > 0 {
				metrics.MySQLBatchFlushes.WithLabelValues("pilots", "shutdown").Inc()
				bw.flushPilots()
			}
			return
		}
	}
}

// thermalWorker обрабатывает батчи термиков
func (bw *BatchWriter) thermalWorker() {
	defer bw.wg.Done()

	ticker := time.NewTicker(bw.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case thermal := <-bw.thermalChan:
			bw.thermalBuffer = append(bw.thermalBuffer, thermal)
			
			if len(bw.thermalBuffer) >= bw.config.BatchSize {
				bw.flushThermals()
			}

		case <-ticker.C:
			if len(bw.thermalBuffer) > 0 {
				bw.flushThermals()
			}

		case <-bw.ctx.Done():
			if len(bw.thermalBuffer) > 0 {
				bw.flushThermals()
			}
			return
		}
	}
}

// stationWorker обрабатывает батчи станций
func (bw *BatchWriter) stationWorker() {
	defer bw.wg.Done()

	ticker := time.NewTicker(bw.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case station := <-bw.stationChan:
			bw.stationBuffer = append(bw.stationBuffer, station)
			
			if len(bw.stationBuffer) >= bw.config.BatchSize {
				bw.flushStations()
			}

		case <-ticker.C:
			if len(bw.stationBuffer) > 0 {
				bw.flushStations()
			}

		case <-bw.ctx.Done():
			if len(bw.stationBuffer) > 0 {
				bw.flushStations()
			}
			return
		}
	}
}

// flushPilots сохраняет батч пилотов в MySQL
func (bw *BatchWriter) flushPilots() {
	if len(bw.pilotBuffer) == 0 {
		return
	}

	start := time.Now()
	batch := make([]*models.Pilot, len(bw.pilotBuffer))
	copy(batch, bw.pilotBuffer)
	bw.pilotBuffer = bw.pilotBuffer[:0] // Очищаем буфер
	
	// Трекаем размер батча
	batchSize := len(batch)
	metrics.MySQLBatchSize.WithLabelValues("pilots").Observe(float64(batchSize))
	
	// Логируем детали батча
	bw.logger.WithField("batch_size", batchSize).
		WithField("queue_depth", len(bw.pilotChan)).
		Info("Flushing pilots batch to MySQL")

	// Выполняем с retry
	err := bw.retryOperation(func() error {
		return bw.mysqlRepo.SavePilotsBatch(bw.ctx, batch)
	})

	duration := time.Since(start)
	
	// Трекаем длительность операции
	metrics.MySQLBatchDuration.WithLabelValues("pilots").Observe(duration.Seconds())

	bw.metrics.mu.Lock()
	if err != nil {
		bw.metrics.PilotsErrors += int64(len(batch))
		bw.logger.WithField("batch_size", len(batch)).
			WithField("duration", duration).
			WithField("error", err).
			Error("Failed to flush pilots batch")
		
		// Увеличиваем счетчик ошибок
		metrics.MySQLWriteErrors.WithLabelValues("queue_full").Inc()
	} else {
		bw.metrics.PilotsBatched++
		bw.metrics.PilotsProcessed += int64(len(batch))
		bw.logger.WithField("batch_size", len(batch)).
			WithField("duration", duration).
			Debug("Flushed pilots batch to MySQL")
	}
	bw.metrics.LastFlushDuration = duration
	bw.metrics.LastBatchSize = len(batch)
	bw.metrics.mu.Unlock()
	
	// Обновляем метрику размера очереди после flush
	metrics.MySQLQueueSize.WithLabelValues("pilots").Set(float64(len(bw.pilotChan)))
}

// flushThermals сохраняет батч термиков в MySQL
func (bw *BatchWriter) flushThermals() {
	if len(bw.thermalBuffer) == 0 {
		return
	}

	start := time.Now()
	batch := make([]*models.Thermal, len(bw.thermalBuffer))
	copy(batch, bw.thermalBuffer)
	bw.thermalBuffer = bw.thermalBuffer[:0]
	
	// Трекаем размер батча
	batchSize := len(batch)
	metrics.MySQLBatchSize.WithLabelValues("thermals").Observe(float64(batchSize))

	err := bw.retryOperation(func() error {
		return bw.mysqlRepo.SaveThermalsBatch(bw.ctx, batch)
	})

	duration := time.Since(start)
	
	// Трекаем длительность операции
	metrics.MySQLBatchDuration.WithLabelValues("pilots").Observe(duration.Seconds())

	bw.metrics.mu.Lock()
	if err != nil {
		bw.metrics.ThermalsErrors += int64(len(batch))
		bw.logger.WithField("batch_size", len(batch)).
			WithField("duration", duration).
			WithField("error", err).
			Error("Failed to flush thermals batch")
		
		// Увеличиваем счетчик ошибок
		metrics.MySQLWriteErrors.WithLabelValues("queue_full").Inc()
	} else {
		bw.metrics.ThermalsBatched++
		bw.metrics.ThermalsProcessed += int64(len(batch))
		bw.logger.WithField("batch_size", len(batch)).
			WithField("duration", duration).
			Debug("Flushed thermals batch to MySQL")
	}
	bw.metrics.mu.Unlock()
	
	// Обновляем метрику размера очереди после flush
	metrics.MySQLQueueSize.WithLabelValues("thermals").Set(float64(len(bw.thermalChan)))
}

// flushStations сохраняет батч станций в MySQL
func (bw *BatchWriter) flushStations() {
	if len(bw.stationBuffer) == 0 {
		return
	}

	start := time.Now()
	batch := make([]*models.Station, len(bw.stationBuffer))
	copy(batch, bw.stationBuffer)
	bw.stationBuffer = bw.stationBuffer[:0]
	
	// Трекаем размер батча
	batchSize := len(batch)
	metrics.MySQLBatchSize.WithLabelValues("thermals").Observe(float64(batchSize))

	err := bw.retryOperation(func() error {
		return bw.mysqlRepo.SaveStationsBatch(bw.ctx, batch)
	})

	duration := time.Since(start)
	
	// Трекаем длительность операции
	metrics.MySQLBatchDuration.WithLabelValues("pilots").Observe(duration.Seconds())

	bw.metrics.mu.Lock()
	if err != nil {
		bw.metrics.StationsErrors += int64(len(batch))
		bw.logger.WithField("batch_size", len(batch)).
			WithField("duration", duration).
			WithField("error", err).
			Error("Failed to flush stations batch")
		
		// Увеличиваем счетчик ошибок
		metrics.MySQLWriteErrors.WithLabelValues("queue_full").Inc()
	} else {
		bw.metrics.StationsBatched++
		bw.metrics.StationsProcessed += int64(len(batch))
		bw.logger.WithField("batch_size", len(batch)).
			WithField("duration", duration).
			Debug("Flushed stations batch to MySQL")
	}
	bw.metrics.mu.Unlock()
	
	// Обновляем метрику размера очереди после flush
	metrics.MySQLQueueSize.WithLabelValues("stations").Set(float64(len(bw.stationChan)))
}

// retryOperation выполняет операцию с повторами
func (bw *BatchWriter) retryOperation(operation func() error) error {
	var lastErr error
	
	for attempt := 0; attempt <= bw.config.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(bw.config.RetryDelay * time.Duration(attempt)):
			case <-bw.ctx.Done():
				return bw.ctx.Err()
			}
		}

		lastErr = operation()
		if lastErr == nil {
			return nil
		}

		bw.logger.WithField("attempt", attempt+1).
			WithField("max_retries", bw.config.MaxRetries).
			WithField("error", lastErr).
			Warn("MySQL batch operation failed, retrying")
	}

	return fmt.Errorf("operation failed after %d retries: %w", bw.config.MaxRetries, lastErr)
}

// GetMetrics возвращает метрики производительности
func (bw *BatchWriter) GetMetrics() BatchMetrics {
	bw.metrics.mu.RLock()
	defer bw.metrics.mu.RUnlock()

	// Обновляем текущую глубину очередей
	bw.metrics.QueueDepthPilots = int64(len(bw.pilotChan))
	bw.metrics.QueueDepthThermals = int64(len(bw.thermalChan))
	bw.metrics.QueueDepthStations = int64(len(bw.stationChan))

	return *bw.metrics
}

// metricsWorker периодически обновляет метрики
func (bw *BatchWriter) metricsWorker() {
	defer bw.wg.Done()
	
	metricsTicker := time.NewTicker(10 * time.Second) // Обновляем метрики каждые 10 секунд
	statusTicker := time.NewTicker(60 * time.Second)  // Логируем статус каждую минуту
	defer metricsTicker.Stop()
	defer statusTicker.Stop()
	
	for {
		select {
		case <-metricsTicker.C:
			// Обновляем метрики размеров очередей
			pilotsQueueSize := len(bw.pilotChan)
			thermalsQueueSize := len(bw.thermalChan)
			stationsQueueSize := len(bw.stationChan)
			
			metrics.MySQLQueueSize.WithLabelValues("pilots").Set(float64(pilotsQueueSize))
			metrics.MySQLQueueSize.WithLabelValues("thermals").Set(float64(thermalsQueueSize))
			metrics.MySQLQueueSize.WithLabelValues("stations").Set(float64(stationsQueueSize))
			
			// Обновляем статусные метрики
			bw.metrics.mu.RLock()
			metrics.MySQLBatchWriterStatus.WithLabelValues("pilots_queued").Set(float64(bw.metrics.PilotsQueued))
			metrics.MySQLBatchWriterStatus.WithLabelValues("pilots_processed").Set(float64(bw.metrics.PilotsProcessed))
			metrics.MySQLBatchWriterStatus.WithLabelValues("pilots_errors").Set(float64(bw.metrics.PilotsErrors))
			metrics.MySQLBatchWriterStatus.WithLabelValues("last_batch_size").Set(float64(bw.metrics.LastBatchSize))
			bw.metrics.mu.RUnlock()
			
		case <-statusTicker.C:
			// Периодическое логирование статуса
			bw.metrics.mu.RLock()
			bw.logger.WithFields(map[string]interface{}{
				"pilots_queued":     bw.metrics.PilotsQueued,
				"pilots_processed":  bw.metrics.PilotsProcessed,
				"pilots_errors":     bw.metrics.PilotsErrors,
				"pilots_queue_size": len(bw.pilotChan),
				"thermals_queued":   bw.metrics.ThermalsQueued,
				"thermals_processed": bw.metrics.ThermalsProcessed,
				"stations_queued":   bw.metrics.StationsQueued,
				"stations_processed": bw.metrics.StationsProcessed,
				"last_batch_size":   bw.metrics.LastBatchSize,
				"last_flush_ms":     bw.metrics.LastFlushDuration.Milliseconds(),
			}).Info("Batch writer status")
			bw.metrics.mu.RUnlock()
			
		case <-bw.ctx.Done():
			return
		}
	}
}

// Stop останавливает BatchWriter и дожидается завершения всех операций
func (bw *BatchWriter) Stop() error {
	bw.logger.Info("Stopping MySQL batch writer...")

	// Сигнализируем о завершении
	bw.cancel()

	// Ждем завершения всех worker'ов
	bw.wg.Wait()

	// Закрываем каналы
	close(bw.pilotChan)
	close(bw.thermalChan)
	close(bw.stationChan)

	bw.logger.Info("MySQL batch writer stopped")
	return nil
}

// Flush принудительно флашит все буферы
func (bw *BatchWriter) Flush() error {
	// Отправляем пустые объекты для принудительного flush
	// Это безопасно, так как flush проверяет размер буфера
	
	select {
	case bw.pilotChan <- nil:
	case <-time.After(time.Second):
		return fmt.Errorf("timeout flushing pilots")
	}

	select {
	case bw.thermalChan <- nil:
	case <-time.After(time.Second):
		return fmt.Errorf("timeout flushing thermals")
	}

	select {
	case bw.stationChan <- nil:
	case <-time.After(time.Second):
		return fmt.Errorf("timeout flushing stations")
	}

	return nil
}