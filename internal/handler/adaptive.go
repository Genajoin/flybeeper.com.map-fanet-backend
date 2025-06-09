package handler

import (
	"math"
	"sync"
	"time"

	"github.com/flybeeper/fanet-backend/internal/geo"
	"github.com/flybeeper/fanet-backend/internal/models"
	"github.com/sirupsen/logrus"
)

// AdaptiveScheduler управляет адаптивными интервалами обновлений
type AdaptiveScheduler struct {
	mu              sync.RWMutex
	clients         map[*Client]*ClientSchedule
	baseInterval    time.Duration
	minInterval     time.Duration
	maxInterval     time.Duration
	logger          *logrus.Entry
}

// ClientSchedule хранит расписание обновлений для клиента
type ClientSchedule struct {
	client          *Client
	currentInterval time.Duration
	lastUpdate      time.Time
	activityScore   float64 // 0.0 - 1.0
	updateCount     uint64
	regionDensity   float64 // Плотность объектов в регионе
	nextUpdate      time.Time
	timer           *time.Timer
}

// ActivityMetrics метрики активности региона
type ActivityMetrics struct {
	ObjectCount     int
	UpdateFrequency float64 // Обновлений в секунду
	AverageSpeed    float64 // Средняя скорость объектов
	ThermalActivity float64 // Активность термиков
}

// NewAdaptiveScheduler создает новый адаптивный планировщик
func NewAdaptiveScheduler(baseInterval time.Duration, logger *logrus.Entry) *AdaptiveScheduler {
	return &AdaptiveScheduler{
		clients:      make(map[*Client]*ClientSchedule),
		baseInterval: baseInterval,
		minInterval:  100 * time.Millisecond,  // Минимум 100ms при высокой активности
		maxInterval:  30 * time.Second,        // Максимум 30s при низкой активности
		logger:       logger.WithField("component", "adaptive_scheduler"),
	}
}

// RegisterClient регистрирует клиента в адаптивном планировщике
func (as *AdaptiveScheduler) RegisterClient(client *Client, initialMetrics ActivityMetrics) {
	as.mu.Lock()
	defer as.mu.Unlock()
	
	schedule := &ClientSchedule{
		client:          client,
		currentInterval: as.baseInterval,
		lastUpdate:      time.Now(),
		activityScore:   as.calculateActivityScore(initialMetrics),
		updateCount:     0,
		regionDensity:   float64(initialMetrics.ObjectCount),
	}
	
	// Вычисляем начальный интервал на основе активности
	schedule.currentInterval = as.calculateInterval(schedule.activityScore)
	schedule.nextUpdate = time.Now().Add(schedule.currentInterval)
	
	// Запускаем таймер для следующего обновления
	schedule.timer = time.AfterFunc(schedule.currentInterval, func() {
		as.triggerUpdate(client)
	})
	
	as.clients[client] = schedule
	
	as.logger.WithFields(logrus.Fields{
		"client":          client.conn.RemoteAddr(),
		"initial_interval": schedule.currentInterval,
		"activity_score":  schedule.activityScore,
		"object_count":    initialMetrics.ObjectCount,
	}).Debug("Client registered with adaptive schedule")
}

// UnregisterClient удаляет клиента из планировщика
func (as *AdaptiveScheduler) UnregisterClient(client *Client) {
	as.mu.Lock()
	defer as.mu.Unlock()
	
	if schedule, exists := as.clients[client]; exists {
		if schedule.timer != nil {
			schedule.timer.Stop()
		}
		delete(as.clients, client)
	}
}

// UpdateMetrics обновляет метрики активности для клиента
func (as *AdaptiveScheduler) UpdateMetrics(client *Client, metrics ActivityMetrics) {
	as.mu.Lock()
	defer as.mu.Unlock()
	
	schedule, exists := as.clients[client]
	if !exists {
		return
	}
	
	// Обновляем счетчик активности
	oldScore := schedule.activityScore
	newScore := as.calculateActivityScore(metrics)
	
	// Экспоненциальное скользящее среднее для сглаживания
	alpha := 0.3
	schedule.activityScore = oldScore*(1-alpha) + newScore*alpha
	schedule.regionDensity = float64(metrics.ObjectCount)
	schedule.updateCount++
	
	// Пересчитываем интервал
	newInterval := as.calculateInterval(schedule.activityScore)
	
	// Если интервал значительно изменился, перепланируем
	if math.Abs(float64(newInterval-schedule.currentInterval)) > float64(schedule.currentInterval)*0.2 {
		schedule.currentInterval = newInterval
		
		// Отменяем старый таймер
		if schedule.timer != nil {
			schedule.timer.Stop()
		}
		
		// Планируем следующее обновление
		timeUntilNext := schedule.nextUpdate.Sub(time.Now())
		if timeUntilNext < 0 || timeUntilNext > newInterval {
			timeUntilNext = newInterval
		}
		
		schedule.nextUpdate = time.Now().Add(timeUntilNext)
		schedule.timer = time.AfterFunc(timeUntilNext, func() {
			as.triggerUpdate(client)
		})
		
		as.logger.WithFields(logrus.Fields{
			"client":       client.conn.RemoteAddr(),
			"old_interval": schedule.currentInterval,
			"new_interval": newInterval,
			"activity":     schedule.activityScore,
		}).Debug("Update interval adjusted")
	}
}

// GetNextUpdate возвращает время следующего обновления для клиента
func (as *AdaptiveScheduler) GetNextUpdate(client *Client) time.Time {
	as.mu.RLock()
	defer as.mu.RUnlock()
	
	if schedule, exists := as.clients[client]; exists {
		return schedule.nextUpdate
	}
	
	return time.Now()
}

// GetClientStats возвращает статистику для клиента
func (as *AdaptiveScheduler) GetClientStats(client *Client) map[string]interface{} {
	as.mu.RLock()
	defer as.mu.RUnlock()
	
	if schedule, exists := as.clients[client]; exists {
		return map[string]interface{}{
			"current_interval": schedule.currentInterval,
			"activity_score":   schedule.activityScore,
			"update_count":     schedule.updateCount,
			"region_density":   schedule.regionDensity,
			"next_update":      schedule.nextUpdate.Unix(),
		}
	}
	
	return nil
}

// calculateActivityScore вычисляет оценку активности от 0.0 до 1.0
func (as *AdaptiveScheduler) calculateActivityScore(metrics ActivityMetrics) float64 {
	score := 0.0
	
	// Фактор плотности объектов (0-0.3)
	densityScore := math.Min(float64(metrics.ObjectCount)/100.0, 1.0) * 0.3
	score += densityScore
	
	// Фактор частоты обновлений (0-0.3)
	updateScore := math.Min(metrics.UpdateFrequency/10.0, 1.0) * 0.3
	score += updateScore
	
	// Фактор средней скорости (0-0.2)
	speedScore := math.Min(metrics.AverageSpeed/50.0, 1.0) * 0.2
	score += speedScore
	
	// Фактор термической активности (0-0.2)
	thermalScore := math.Min(metrics.ThermalActivity, 1.0) * 0.2
	score += thermalScore
	
	return math.Min(score, 1.0)
}

// calculateInterval вычисляет интервал обновлений на основе активности
func (as *AdaptiveScheduler) calculateInterval(activityScore float64) time.Duration {
	// Инвертируем score: высокая активность = короткий интервал
	factor := 1.0 - activityScore
	
	// Экспоненциальная шкала для более плавного перехода
	factor = math.Pow(factor, 2)
	
	// Вычисляем интервал
	intervalRange := float64(as.maxInterval - as.minInterval)
	interval := as.minInterval + time.Duration(factor*intervalRange)
	
	// Округляем до ближайших 100ms для стабильности
	interval = (interval / (100 * time.Millisecond)) * (100 * time.Millisecond)
	
	return interval
}

// triggerUpdate инициирует обновление для клиента
func (as *AdaptiveScheduler) triggerUpdate(client *Client) {
	as.mu.Lock()
	schedule, exists := as.clients[client]
	if !exists {
		as.mu.Unlock()
		return
	}
	
	// Обновляем время последнего обновления
	schedule.lastUpdate = time.Now()
	schedule.nextUpdate = time.Now().Add(schedule.currentInterval)
	
	// Планируем следующее обновление
	schedule.timer = time.AfterFunc(schedule.currentInterval, func() {
		as.triggerUpdate(client)
	})
	as.mu.Unlock()
	
	// Отправляем сигнал клиенту о необходимости обновления
	select {
	case client.updateSignal <- true:
	default:
		// Канал занят, пропускаем это обновление
	}
}

// AnalyzeRegionActivity анализирует активность в регионе
func AnalyzeRegionActivity(spatial *geo.SpatialIndex, centerLat, centerLon, radiusKm float64) ActivityMetrics {
	metrics := ActivityMetrics{}
	
	// Получаем объекты в регионе
	objects := spatial.QueryRadius(centerLat, centerLon, radiusKm)
	metrics.ObjectCount = len(objects)
	
	if metrics.ObjectCount == 0 {
		return metrics
	}
	
	// Анализируем объекты
	now := time.Now()
	recentUpdates := 0
	totalSpeed := 0.0
	thermalCount := 0
	
	for _, obj := range objects {
		// Считаем недавние обновления (последние 30 секунд)
		if now.Sub(obj.GetTimestamp()) < 30*time.Second {
			recentUpdates++
		}
		
		// Средняя скорость для пилотов
		if pilot, ok := obj.(*models.Pilot); ok {
			totalSpeed += float64(pilot.Speed)
		}
		
		// Количество термиков
		if _, ok := obj.(*models.Thermal); ok {
			thermalCount++
		}
	}
	
	// Вычисляем метрики
	metrics.UpdateFrequency = float64(recentUpdates) / 30.0 // Обновлений в секунду
	
	pilotCount := metrics.ObjectCount - thermalCount
	if pilotCount > 0 {
		metrics.AverageSpeed = totalSpeed / float64(pilotCount)
	}
	
	if metrics.ObjectCount > 0 {
		metrics.ThermalActivity = float64(thermalCount) / float64(metrics.ObjectCount)
	}
	
	return metrics
}

// AdaptiveBatchSize вычисляет оптимальный размер батча на основе активности
func AdaptiveBatchSize(activityScore float64, minBatch, maxBatch int) int {
	// Высокая активность = больший батч для эффективности
	factor := activityScore
	
	batchRange := float64(maxBatch - minBatch)
	batchSize := minBatch + int(factor*batchRange)
	
	// Округляем до ближайших 10
	batchSize = (batchSize / 10) * 10
	if batchSize < minBatch {
		batchSize = minBatch
	}
	
	return batchSize
}