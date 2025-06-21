package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// HTTP метрики
	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "fanet_http_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "endpoint", "status"},
	)

	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fanet_http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "endpoint", "status"},
	)

	// WebSocket метрики
	WebSocketConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "fanet_websocket_connections_active",
			Help: "Number of active WebSocket connections",
		},
	)

	WebSocketMessagesOut = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fanet_websocket_messages_out_total",
			Help: "Total number of WebSocket messages sent",
		},
		[]string{"type"},
	)

	WebSocketErrors = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "fanet_websocket_errors_total",
			Help: "Total number of WebSocket errors",
		},
	)

	// MQTT метрики
	MQTTMessagesReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fanet_mqtt_messages_received_total",
			Help: "Total number of MQTT messages received",
		},
		[]string{"packet_type"},
	)

	MQTTParseErrors = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "fanet_mqtt_parse_errors_total",
			Help: "Total number of MQTT message parse errors",
		},
	)

	MQTTConnectionStatus = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "fanet_mqtt_connection_status",
			Help: "MQTT connection status (1 = connected, 0 = disconnected)",
		},
	)

	// Redis метрики
	RedisOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "fanet_redis_operation_duration_seconds",
			Help:    "Duration of Redis operations in seconds",
			Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1},
		},
		[]string{"operation"},
	)

	RedisOperationErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fanet_redis_operation_errors_total",
			Help: "Total number of Redis operation errors",
		},
		[]string{"operation"},
	)

	// MySQL Batch Writer метрики
	MySQLBatchSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "fanet_mysql_batch_size",
			Help:    "Size of MySQL batch inserts",
			Buckets: []float64{1, 10, 50, 100, 250, 500, 1000, 2000, 5000},
		},
		[]string{"entity_type"}, // pilots, thermals, stations
	)

	MySQLBatchDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "fanet_mysql_batch_duration_seconds",
			Help:    "Duration of MySQL batch operations in seconds",
			Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		},
		[]string{"entity_type"}, // pilots, thermals, stations
	)

	MySQLQueueSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "fanet_mysql_queue_size",
			Help: "Current size of MySQL writer queues",
		},
		[]string{"queue_type"}, // pilots, thermals, stations
	)

	MySQLWriteErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fanet_mysql_write_errors_total",
			Help: "Total number of MySQL write errors",
		},
		[]string{"entity_type"}, // pilots, thermals, stations
	)
	
	MySQLBatchesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fanet_mysql_batches_total",
			Help: "Total number of MySQL batches processed",
		},
		[]string{"entity_type", "status"}, // entity_type: pilots/thermals/stations, status: success/error
	)
	
	MySQLRecordsProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fanet_mysql_records_processed_total",
			Help: "Total number of records processed by MySQL batch writer",
		},
		[]string{"entity_type"}, // pilots, thermals, stations
	)

	// Общие метрики приложения
	AppInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "fanet_app_info",
			Help: "Application information",
		},
		[]string{"version", "commit", "build_time"},
	)

	ActivePilots = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "fanet_active_pilots_total",
			Help: "Total number of active pilots in the system",
		},
	)

	ActiveThermals = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "fanet_active_thermals_total",
			Help: "Total number of active thermals in the system",
		},
	)

	ActiveStations = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "fanet_active_stations_total",
			Help: "Total number of active ground stations in the system",
		},
	)

	// Database connection status
	MySQLConnectionStatus = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "fanet_mysql_connection_status",
			Help: "MySQL connection status (1 = connected, 0 = disconnected)",
		},
	)

	RedisConnectionStatus = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "fanet_redis_connection_status",
			Help: "Redis connection status (1 = connected, 0 = disconnected)",
		},
	)
)

// SetAppInfo устанавливает информацию о версии приложения
func SetAppInfo(version, commit, buildTime string) {
	AppInfo.WithLabelValues(version, commit, buildTime).Set(1)
}