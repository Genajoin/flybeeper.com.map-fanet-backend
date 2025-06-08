package mqtt

import (
	"context"
	"fmt"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/flybeeper/fanet-backend/internal/config"
	"github.com/flybeeper/fanet-backend/internal/metrics"
	"github.com/flybeeper/fanet-backend/pkg/utils"
)

// Client представляет MQTT клиент для получения данных от FANET устройств
type Client struct {
	client    mqtt.Client
	config    *config.MQTTConfig
	logger    *utils.Logger
	parser    *Parser
	handler   MessageHandler
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	connected bool
	mu        sync.RWMutex
}

// MessageHandler функция обработки входящих MQTT сообщений
type MessageHandler func(msg *FANETMessage) error

// NewClient создает новый MQTT клиент
func NewClient(cfg *config.MQTTConfig, logger *utils.Logger, handler MessageHandler) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	ctx, cancel := context.WithCancel(context.Background())

	parser := NewParser(logger)
	
	c := &Client{
		config:  cfg,
		logger:  logger,
		parser:  parser,
		handler: handler,
		ctx:     ctx,
		cancel:  cancel,
	}

	// Настройка MQTT клиента
	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.URL)
	opts.SetClientID(cfg.ClientID)
	opts.SetCleanSession(cfg.CleanSession)
	opts.SetOrderMatters(cfg.OrderMatters)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(5 * time.Second)
	opts.SetMaxReconnectInterval(60 * time.Second)

	if cfg.Username != "" {
		opts.SetUsername(cfg.Username)
	}
	if cfg.Password != "" {
		opts.SetPassword(cfg.Password)
	}

	// Callback при подключении
	opts.SetOnConnectHandler(func(client mqtt.Client) {
		c.mu.Lock()
		c.connected = true
		c.mu.Unlock()
		
		c.logger.WithField("broker", cfg.URL).Info("Connected to MQTT broker")
		metrics.MQTTConnectionStatus.Set(1)
		
		// Подписка на топик после подключения
		if token := client.Subscribe(cfg.TopicPrefix, 1, c.messageHandler()); token.Wait() && token.Error() != nil {
			c.logger.WithFields(map[string]interface{}{
				"topic": cfg.TopicPrefix,
				"error": token.Error(),
			}).Error("Failed to subscribe to topic")
		} else {
			c.logger.WithField("topic", cfg.TopicPrefix).Info("Subscribed to MQTT topic")
		}
	})

	// Callback при потере соединения
	opts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		c.mu.Lock()
		c.connected = false
		c.mu.Unlock()
		
		c.logger.WithField("error", err).Warn("Lost connection to MQTT broker")
		metrics.MQTTConnectionStatus.Set(0)
	})

	// Callback при восстановлении соединения не поддерживается в данной версии

	c.client = mqtt.NewClient(opts)

	return c, nil
}

// Connect подключается к MQTT брокеру
func (c *Client) Connect() error {
	c.logger.WithField("broker", c.config.URL).Info("Connecting to MQTT broker")

	token := c.client.Connect()
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to connect to MQTT broker: %w", token.Error())
	}

	// Ждем подтверждения подключения
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("connection timeout")
		case <-ticker.C:
			c.mu.RLock()
			connected := c.connected
			c.mu.RUnlock()
			
			if connected {
				return nil
			}
		case <-c.ctx.Done():
			return c.ctx.Err()
		}
	}
}

// Disconnect отключается от MQTT брокера
func (c *Client) Disconnect() {
	c.logger.Info("Disconnecting from MQTT broker")
	
	c.cancel()
	
	if c.client.IsConnected() {
		c.client.Disconnect(1000) // 1 секунда на graceful disconnect
	}
	
	c.wg.Wait()
	c.logger.Info("MQTT client disconnected")
}

// IsConnected проверяет статус подключения
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected && c.client.IsConnected()
}

// messageHandler создает обработчик MQTT сообщений
func (c *Client) messageHandler() mqtt.MessageHandler {
	return func(client mqtt.Client, msg mqtt.Message) {
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			
			// Извлекаем информацию о топике
			topic := msg.Topic()
			payload := msg.Payload()
			
			c.logger.WithFields(map[string]interface{}{
				"topic": topic,
				"payload_size": len(payload),
				"qos": msg.Qos(),
				"retained": msg.Retained(),
			}).Debug("Received MQTT message")
			
			// Парсим FANET сообщение
			fanetMsg, err := c.parser.Parse(topic, payload)
			if err != nil {
				c.logger.WithFields(map[string]interface{}{
					"topic": topic,
					"error": err,
					"payload_size": len(payload),
				}).Error("Failed to parse FANET message")
				metrics.MQTTParseErrors.Inc()
				return
			}
			
			if fanetMsg == nil {
				// Сообщение не является валидным FANET пакетом или не поддерживается
				c.logger.WithField("topic", topic).Debug("Skipping non-FANET or unsupported message")
				return
			}
			
			// Передаем сообщение обработчику
			if c.handler != nil {
				if err := c.handler(fanetMsg); err != nil {
					c.logger.WithFields(map[string]interface{}{
						"topic": topic,
						"message_type": fanetMsg.Type,
						"device_id": fanetMsg.DeviceID,
						"error": err,
					}).Error("Message handler failed")
				} else {
					c.logger.WithFields(map[string]interface{}{
						"topic": topic,
						"message_type": fanetMsg.Type,
						"device_id": fanetMsg.DeviceID,
					}).Debug("Successfully processed FANET message")
					// Увеличиваем счетчик по типу пакета
					packetType := fmt.Sprintf("%d", fanetMsg.Type)
					metrics.MQTTMessagesReceived.WithLabelValues(packetType).Inc()
				}
			} else {
				c.logger.WithField("topic", topic).Warn("Message handler is nil")
			}
		}()
	}
}

// GetStats возвращает статистику клиента
func (c *Client) GetStats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	return map[string]interface{}{
		"connected":    c.connected,
		"client_id":    c.config.ClientID,
		"broker_url":   c.config.URL,
		"topic_prefix": c.config.TopicPrefix,
		"clean_session": c.config.CleanSession,
	}
}

// PublishMessage отправляет сообщение в MQTT топик (для отладки)
func (c *Client) PublishMessage(topic string, payload []byte, qos byte, retained bool) error {
	if !c.IsConnected() {
		return fmt.Errorf("MQTT client is not connected")
	}
	
	token := c.client.Publish(topic, qos, retained, payload)
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to publish message: %w", token.Error())
	}
	
	c.logger.WithFields(map[string]interface{}{
		"topic": topic,
		"payload_size": len(payload),
		"qos": qos,
		"retained": retained,
	}).Debug("Published MQTT message")
	
	return nil
}