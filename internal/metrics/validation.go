package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// ValidationTotalPackets общее количество пакетов для валидации
	ValidationTotalPackets = promauto.NewCounter(prometheus.CounterOpts{
		Name: "fanet_validation_total_packets",
		Help: "Total number of packets processed for validation",
	})

	// ValidationValidatedPackets количество валидированных пакетов
	ValidationValidatedPackets = promauto.NewCounter(prometheus.CounterOpts{
		Name: "fanet_validation_validated_packets",
		Help: "Number of successfully validated packets",
	})

	// ValidationRejectedPackets количество отклоненных пакетов
	ValidationRejectedPackets = promauto.NewCounter(prometheus.CounterOpts{
		Name: "fanet_validation_rejected_packets",
		Help: "Number of rejected packets due to failed validation",
	})

	// ValidationInvalidatedDevices количество инвалидированных устройств
	ValidationInvalidatedDevices = promauto.NewCounter(prometheus.CounterOpts{
		Name: "fanet_validation_invalidated_devices",
		Help: "Number of devices that have been invalidated",
	})

	// ValidationSpeedViolations счетчик нарушений скорости по типам ЛА
	ValidationSpeedViolations = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fanet_validation_speed_violations",
		Help: "Number of speed violations by aircraft type",
	}, []string{"aircraft_type"})

	// ValidationActiveStates количество активных состояний валидации
	ValidationActiveStates = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "fanet_validation_active_states",
		Help: "Current number of active validation states in memory",
	})

	// ValidationScoreChanges счетчик изменений счета валидации
	ValidationScoreChanges = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fanet_validation_score_changes",
		Help: "Number of validation score changes by direction",
	}, []string{"direction"}) // direction: increase, decrease

	// ValidationScoreDistribution распределение счетов валидации
	ValidationScoreDistribution = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "fanet_validation_score_distribution",
		Help: "Distribution of validation scores",
		Buckets: []float64{0, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100},
	})

	// ValidationDevicesAboveThreshold количество устройств выше порога валидации
	ValidationDevicesAboveThreshold = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "fanet_validation_devices_above_threshold",
		Help: "Current number of devices with validation score above Redis threshold",
	})
)