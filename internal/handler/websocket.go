package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/flybeeper/fanet-backend/internal/geo"
	"github.com/flybeeper/fanet-backend/internal/metrics"
	"github.com/flybeeper/fanet-backend/internal/models"
	"github.com/flybeeper/fanet-backend/internal/repository"
	"github.com/flybeeper/fanet-backend/pkg/pb"
	"github.com/flybeeper/fanet-backend/pkg/utils"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

// WebSocketHandler обрабатывает WebSocket соединения для real-time обновлений
type WebSocketHandler struct {
	upgrader   websocket.Upgrader
	repository repository.Repository
	logger     *logrus.Entry
	// broadcast  *BroadcastManager // Временно отключено
	spatial    *geo.SpatialIndex
	sequence   uint64
	sequenceMu sync.Mutex
}

// Client представляет WebSocket соединение
type Client struct {
	conn          *websocket.Conn
	send          chan []byte
	updateSignal  chan bool
	handler       *WebSocketHandler
	center        models.GeoPoint
	radius        int32
	geohashes     []string
	lastSequence  uint64
	authenticated bool
	mu            sync.RWMutex
}

// NewWebSocketHandler создает новый WebSocket handler
func NewWebSocketHandler(repo repository.Repository, logger interface{}) *WebSocketHandler {
	// Конвертируем logger в правильный тип
	var logEntry *logrus.Entry
	switch l := logger.(type) {
	case *logrus.Entry:
		logEntry = l
	default:
		// Создаем новый logrus logger если не подходит тип
		logrusLogger := logrus.New()
		logEntry = logrusLogger.WithField("component", "websocket")
	}
	
	// Create spatial index with 5 minute TTL for hot data
	spatial := geo.NewSpatialIndex(5*time.Minute, 1000, 30*time.Second)
	
	return &WebSocketHandler{
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				// TODO: Добавить проверку Origin для production
				return true
			},
		},
		repository: repo,
		logger:     logEntry,
		spatial:    spatial,
	}
}

// HandleWebSocket обрабатывает WebSocket подключения
func (h *WebSocketHandler) HandleWebSocket(c *gin.Context) {
	// Извлекаем параметры подключения
	latStr := c.Query("lat")
	lonStr := c.Query("lon")
	radiusStr := c.Query("radius")
	token := c.Query("token")

	if latStr == "" || lonStr == "" || radiusStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "lat, lon, radius are required"})
		return
	}

	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil || lat < -90 || lat > 90 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid latitude"})
		return
	}

	lon, err := strconv.ParseFloat(lonStr, 64)
	if err != nil || lon < -180 || lon > 180 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid longitude"})
		return
	}

	radius, err := strconv.ParseInt(radiusStr, 10, 32)
	if err != nil || radius <= 0 || radius > 200 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid radius (1-200 km)"})
		return
	}

	// Проверяем аутентификацию если токен предоставлен
	authenticated := false
	if token != "" {
		// TODO: Валидация токена через Laravel API
		authenticated = len(token) > 10 // Базовая проверка
	}

	// Обновляем до WebSocket
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.WithField("error", err).Error("Failed to upgrade to WebSocket")
		return
	}

	client := &Client{
		conn:         conn,
		send:         make(chan []byte, 256),
		updateSignal: make(chan bool, 1),
		handler:      h,
		center:       models.GeoPoint{Latitude: lat, Longitude: lon},
		radius:       int32(radius),
		authenticated: authenticated,
	}

	// TODO: Регистрируем клиента в broadcast manager
	// h.broadcast.Register(client, lat, lon, float64(radius))

	h.logger.WithFields(logrus.Fields{
		"client_ip": c.ClientIP(),
		"lat":       lat,
		"lon":       lon,
		"radius":    radius,
		"auth":      authenticated,
	}).Info("WebSocket client connected")
	
	// Увеличиваем счетчик активных соединений
	metrics.WebSocketConnections.Inc()

	// Запускаем goroutines для клиента
	go client.writePump()
	go client.readPump()

	// Отправляем приветственное сообщение
	client.sendWelcome()

	// Подписываем на регион
	client.subscribeToRegion()
}

// sendWelcome отправляет приветственное сообщение
func (c *Client) sendWelcome() {
	welcome := &pb.Welcome{
		ServerTime:    uint64(time.Now().Unix()),
		Sequence:      c.handler.getNextSequence(),
		ServerVersion: "1.0.0",
	}

	data, err := proto.Marshal(welcome)
	if err != nil {
		c.handler.logger.WithField("error", err).Error("Failed to marshal welcome message")
		return
	}

	select {
	case c.send <- data:
	case <-time.After(5 * time.Second):
		c.handler.logger.Warn("Welcome message send timeout")
	}
}

// subscribeToRegion подписывает клиента на геопространственный регион
func (c *Client) subscribeToRegion() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Вычисляем geohash ячейки для региона
	precision := geo.OptimalGeohashPrecision(float64(c.radius))
	geohashes := geo.Cover(c.center.Latitude, c.center.Longitude, float64(c.radius), precision)
	c.geohashes = geohashes

	// Отправляем подтверждение подписки
	response := &pb.SubscribeResponse{
		Success:   true,
		Geohashes: c.geohashes,
	}

	data, err := proto.Marshal(response)
	if err != nil {
		c.handler.logger.WithField("error", err).Error("Failed to marshal subscribe response")
		return
	}

	select {
	case c.send <- data:
	case <-time.After(5 * time.Second):
		c.handler.logger.Warn("Subscribe response send timeout")
	}

	c.handler.logger.WithFields(logrus.Fields{
		"center":    fmt.Sprintf("%.4f,%.4f", c.center.Latitude, c.center.Longitude),
		"radius":    c.radius,
		"geohashes": len(c.geohashes),
		"precision": precision,
	}).Debug("Client subscribed to region")
}

// readPump обрабатывает входящие сообщения от клиента
func (c *Client) readPump() {
	defer func() {
		c.handler.unregisterClient(c)
		c.conn.Close()
		// Уменьшаем счетчик активных соединений
		metrics.WebSocketConnections.Dec()
	}()

	// Настройки тайм-аутов
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.handler.logger.WithField("error", err).Error("WebSocket read error")
			}
			break
		}

		// Обрабатываем входящие сообщения
		c.handleMessage(message)
	}
}

// writePump отправляет сообщения клиенту
func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second) // Heartbeat каждые 30 секунд
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteMessage(websocket.BinaryMessage, message); err != nil {
				c.handler.logger.WithField("error", err).Error("WebSocket write error")
				metrics.WebSocketErrors.Inc()
				return
			}
			
			// Учитываем отправленное сообщение
			metrics.WebSocketMessagesOut.WithLabelValues("update").Inc()

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			
			// Отправляем ping
			ping := &pb.Ping{
				Timestamp: time.Now().Unix(),
			}
			
			data, err := proto.Marshal(ping)
			if err != nil {
				c.handler.logger.WithField("error", err).Error("Failed to marshal ping")
				continue
			}

			if err := c.conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
				c.handler.logger.WithField("error", err).Error("Ping write error")
				metrics.WebSocketErrors.Inc()
				return
			}
			
			// Учитываем ping сообщение
			metrics.WebSocketMessagesOut.WithLabelValues("ping").Inc()
		}
	}
}

// handleMessage обрабатывает входящие сообщения от клиента
func (c *Client) handleMessage(message []byte) {
	// Для простоты сначала пытаемся разобрать как JSON для отладки
	var msgType struct {
		Type string `json:"type"`
	}
	
	if err := json.Unmarshal(message, &msgType); err == nil {
		switch msgType.Type {
		case "subscribe":
			// Обработка изменения подписки
			var req struct {
				Type   string  `json:"type"`
				Lat    float64 `json:"lat"`
				Lon    float64 `json:"lon"`
				Radius int32   `json:"radius"`
			}
			
			if err := json.Unmarshal(message, &req); err == nil {
				c.updateSubscription(req.Lat, req.Lon, req.Radius)
			}
			
		case "pong":
			// Обработка pong ответа
			c.handler.logger.Debug("Received pong from client")
		}
	}
}

// updateSubscription обновляет подписку клиента на новый регион
func (c *Client) updateSubscription(lat, lon float64, radius int32) {
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 || radius <= 0 || radius > 200 {
		c.handler.logger.Warn("Invalid subscription parameters")
		return
	}

	c.mu.Lock()
	c.center = models.GeoPoint{Latitude: lat, Longitude: lon}
	c.radius = radius
	c.mu.Unlock()

	c.subscribeToRegion()

	c.handler.logger.WithFields(map[string]interface{}{
		"new_center": fmt.Sprintf("%.4f,%.4f", lat, lon),
		"new_radius": radius,
	}).Debug("Client subscription updated")
}

// unregisterClient удаляет клиента из списка активных
func (h *WebSocketHandler) unregisterClient(client *Client) {
	h.logger.Debug("WebSocket client disconnected")
	// TODO: Реализовать управление клиентами
}

// getNextSequence возвращает следующий номер последовательности
func (h *WebSocketHandler) getNextSequence() uint64 {
	h.sequenceMu.Lock()
	defer h.sequenceMu.Unlock()
	h.sequence++
	return h.sequence
}

// BroadcastUpdate отправляет обновление всем подключенным клиентам
func (h *WebSocketHandler) BroadcastUpdate(updateType pb.UpdateType, action pb.Action, data interface{}) {
	// Простая реализация без сложной фильтрации
	h.logger.WithFields(logrus.Fields{
		"type":   updateType.String(),
		"action": action.String(),
	}).Debug("WebSocket update broadcast (simplified)")
	
	// TODO: Реализовать полноценный broadcast через BroadcastManager
}

// shouldSendUpdate проверяет, нужно ли отправлять обновление клиенту
func (h *WebSocketHandler) shouldSendUpdate(client *Client, data proto.Message) bool {
	client.mu.RLock()
	defer client.mu.RUnlock()

	// Получаем геопозицию из данных
	var lat, lon float64
	
	switch v := data.(type) {
	case *pb.Pilot:
		if v.Position == nil {
			return false
		}
		lat, lon = v.Position.Latitude, v.Position.Longitude
	case *pb.Thermal:
		if v.Position == nil {
			return false
		}
		lat, lon = v.Position.Latitude, v.Position.Longitude
	case *pb.Station:
		if v.Position == nil {
			return false
		}
		lat, lon = v.Position.Latitude, v.Position.Longitude
	default:
		return false
	}

	// Проверяем, находится ли объект в радиусе клиента
	distance := calculateDistance(client.center.Latitude, client.center.Longitude, lat, lon)
	return distance <= float64(client.radius)*1000 // конвертируем км в метры
}

// calculateDistance вычисляет расстояние между двумя точками в метрах (формула Haversine)
func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000 // Радиус Земли в метрах
	
	dLat := (lat2 - lat1) * 3.14159265359 / 180
	dLon := (lon2 - lon1) * 3.14159265359 / 180
	
	a := 0.5 - 0.5 * (dLat * dLat + dLat * dLon * dLon) // упрощенная формула для малых расстояний
	
	return R * 2 * a
}

// GetStats возвращает статистику WebSocket handler
func (h *WebSocketHandler) GetStats() map[string]interface{} {
	h.sequenceMu.Lock()
	currentSequence := h.sequence
	h.sequenceMu.Unlock()

	return map[string]interface{}{
		"current_sequence": currentSequence,
		"spatial_objects":  0, // TODO: восстановить когда будет spatial index
	}
}
