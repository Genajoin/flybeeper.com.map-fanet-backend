package handler

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/flybeeper/fanet-backend/internal/geo"
	"github.com/flybeeper/fanet-backend/internal/models"
	"github.com/flybeeper/fanet-backend/pkg/pb"
	"github.com/flybeeper/fanet-backend/pkg/pool"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

// BroadcastManager efficiently broadcasts updates to WebSocket clients
type BroadcastManager struct {
	groups      map[string]*GeohashGroup // geohash -> group
	clients     map[*Client]*ClientInfo  // client -> info
	spatial     *geo.SpatialIndex
	mu          sync.RWMutex
	
	// Update channels
	updates     chan *UpdatePacket
	register    chan *ClientRegistration
	unregister  chan *Client
	
	// Batching
	batchSize   int
	batchTime   time.Duration
	
	// Metrics
	metrics     *BroadcastMetrics
	
	// Logging
	logger      *logrus.Entry
}

// GeohashGroup represents clients subscribed to a geohash region
type GeohashGroup struct {
	geohash     string
	clients     map[*Client]bool
	mu          sync.RWMutex
	lastUpdate  time.Time
}

// ClientInfo stores client subscription details
type ClientInfo struct {
	client      *Client
	geohashes   map[string]bool
	centerLat   float64
	centerLon   float64
	radiusKm    float64
	lastActive  time.Time
}

// ClientRegistration represents a new client subscription
type ClientRegistration struct {
	client      *Client
	centerLat   float64
	centerLon   float64
	radiusKm    float64
}

// UpdatePacket represents an update to broadcast
type UpdatePacket struct {
	Type        pb.UpdateType
	Pilot       *models.Pilot
	Thermal     *models.Thermal
	Station     *models.Station
	Timestamp   time.Time
}

// BroadcastMetrics tracks broadcast performance
type BroadcastMetrics struct {
	UpdatesReceived    uint64
	UpdatesBroadcast   uint64
	ClientsActive      uint64
	GroupsActive       uint64
	AvgBroadcastTimeMs float64
	AvgRecipientsCount float64
}

// NewBroadcastManager creates a new broadcast manager
func NewBroadcastManager(spatial *geo.SpatialIndex) *BroadcastManager {
	bm := &BroadcastManager{
		groups:     make(map[string]*GeohashGroup),
		clients:    make(map[*Client]*ClientInfo),
		spatial:    spatial,
		updates:    make(chan *UpdatePacket, 1000),
		register:   make(chan *ClientRegistration, 100),
		unregister: make(chan *Client, 100),
		batchSize:  50,
		batchTime:  100 * time.Millisecond,
		metrics:    &BroadcastMetrics{},
		logger:     logrus.WithField("component", "broadcast"),
	}
	
	// Start background workers
	go bm.run()
	go bm.metricsCollector()
	
	return bm
}

// Register subscribes a client to geohash regions
func (bm *BroadcastManager) Register(client *Client, centerLat, centerLon, radiusKm float64) {
	bm.register <- &ClientRegistration{
		client:    client,
		centerLat: centerLat,
		centerLon: centerLon,
		radiusKm:  radiusKm,
	}
}

// Unregister removes a client from all subscriptions
func (bm *BroadcastManager) Unregister(client *Client) {
	bm.unregister <- client
}

// Broadcast sends an update to relevant clients
func (bm *BroadcastManager) Broadcast(update *UpdatePacket) {
	select {
	case bm.updates <- update:
		atomic.AddUint64(&bm.metrics.UpdatesReceived, 1)
	default:
		bm.logger.Warn("Update channel full, dropping update")
	}
}

// run is the main event loop
func (bm *BroadcastManager) run() {
	ticker := time.NewTicker(bm.batchTime)
	defer ticker.Stop()
	
	batch := make([]*UpdatePacket, 0, bm.batchSize)
	
	for {
		select {
		case reg := <-bm.register:
			bm.handleRegister(reg)
			
		case client := <-bm.unregister:
			bm.handleUnregister(client)
			
		case update := <-bm.updates:
			batch = append(batch, update)
			if len(batch) >= bm.batchSize {
				bm.processBatch(batch)
				batch = batch[:0]
			}
			
		case <-ticker.C:
			if len(batch) > 0 {
				bm.processBatch(batch)
				batch = batch[:0]
			}
		}
	}
}

// handleRegister subscribes a client to relevant geohash groups
func (bm *BroadcastManager) handleRegister(reg *ClientRegistration) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	
	// Calculate geohashes covering the client's area
	precision := geo.OptimalGeohashPrecision(reg.radiusKm)
	geohashes := geo.Cover(reg.centerLat, reg.centerLon, reg.radiusKm, precision)
	
	// Create client info
	info := &ClientInfo{
		client:     reg.client,
		geohashes:  make(map[string]bool),
		centerLat:  reg.centerLat,
		centerLon:  reg.centerLon,
		radiusKm:   reg.radiusKm,
		lastActive: time.Now(),
	}
	
	// Subscribe to geohash groups
	for _, gh := range geohashes {
		info.geohashes[gh] = true
		
		// Get or create group
		group, exists := bm.groups[gh]
		if !exists {
			group = &GeohashGroup{
				geohash:    gh,
				clients:    make(map[*Client]bool),
				lastUpdate: time.Now(),
			}
			bm.groups[gh] = group
		}
		
		// Add client to group
		group.mu.Lock()
		group.clients[reg.client] = true
		group.mu.Unlock()
	}
	
	bm.clients[reg.client] = info
	atomic.StoreUint64(&bm.metrics.ClientsActive, uint64(len(bm.clients)))
	atomic.StoreUint64(&bm.metrics.GroupsActive, uint64(len(bm.groups)))
	
	bm.logger.WithFields(logrus.Fields{
		"client":    reg.client.conn.RemoteAddr(),
		"geohashes": len(geohashes),
		"radius":    reg.radiusKm,
	}).Debug("Client registered for broadcast")
}

// handleUnregister removes a client from all groups
func (bm *BroadcastManager) handleUnregister(client *Client) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	
	info, exists := bm.clients[client]
	if !exists {
		return
	}
	
	// Remove from all groups
	for gh := range info.geohashes {
		if group, exists := bm.groups[gh]; exists {
			group.mu.Lock()
			delete(group.clients, client)
			
			// Remove empty groups
			if len(group.clients) == 0 {
				delete(bm.groups, gh)
			}
			group.mu.Unlock()
		}
	}
	
	delete(bm.clients, client)
	atomic.StoreUint64(&bm.metrics.ClientsActive, uint64(len(bm.clients)))
	atomic.StoreUint64(&bm.metrics.GroupsActive, uint64(len(bm.groups)))
	
	bm.logger.WithField("client", client.conn.RemoteAddr()).Debug("Client unregistered from broadcast")
}

// processBatch broadcasts a batch of updates efficiently
func (bm *BroadcastManager) processBatch(batch []*UpdatePacket) {
	start := time.Now()
	
	// Group updates by geohash
	updatesByGeohash := make(map[string][]*UpdatePacket)
	
	for _, update := range batch {
		var lat, lon float64
		
		// Extract coordinates based on type
		switch update.Type {
		case pb.UpdateType_UPDATE_TYPE_PILOT:
			if update.Pilot != nil && update.Pilot.Position != nil {
				lat = update.Pilot.Position.Latitude
				lon = update.Pilot.Position.Longitude
			}
		case pb.UpdateType_UPDATE_TYPE_THERMAL:
			if update.Thermal != nil && update.Thermal.Position != nil {
				lat = update.Thermal.Position.Latitude
				lon = update.Thermal.Position.Longitude
			}
		case pb.UpdateType_UPDATE_TYPE_STATION:
			if update.Station != nil && update.Station.Position != nil {
				lat = update.Station.Position.Latitude
				lon = update.Station.Position.Longitude
			}
		}
		
		// Find affected geohashes
		for precision := 3; precision <= 7; precision++ {
			gh := geo.Encode(lat, lon, precision)
			updatesByGeohash[gh] = append(updatesByGeohash[gh], update)
		}
	}
	
	// Broadcast to groups
	totalRecipients := 0
	
	bm.mu.RLock()
	for gh, updates := range updatesByGeohash {
		if group, exists := bm.groups[gh]; exists {
			recipients := bm.broadcastToGroup(group, updates)
			totalRecipients += recipients
		}
	}
	bm.mu.RUnlock()
	
	// Update metrics
	atomic.AddUint64(&bm.metrics.UpdatesBroadcast, uint64(len(batch)))
	bm.updateBroadcastMetrics(time.Since(start), totalRecipients)
	
	bm.logger.WithFields(logrus.Fields{
		"batch_size": len(batch),
		"recipients": totalRecipients,
		"duration":   time.Since(start),
	}).Debug("Batch broadcast completed")
}

// broadcastToGroup sends updates to all clients in a group
func (bm *BroadcastManager) broadcastToGroup(group *GeohashGroup, updates []*UpdatePacket) int {
	group.mu.RLock()
	defer group.mu.RUnlock()
	
	if len(group.clients) == 0 {
		return 0
	}
	
	// Build update batch message using pool
	updateBatch := pool.Global.GetPbUpdateBatch()
	updateBatch.Timestamp = time.Now().Unix()
	
	// Deduplicate updates by object ID
	seen := make(map[string]bool)
	
	for _, update := range updates {
		var objID string
		var data []byte
		var err error
		
		pbUpdate := pool.Global.GetPbUpdate()
		pbUpdate.Type = update.Type
		pbUpdate.Action = pb.Action_ACTION_UPDATE
		pbUpdate.Sequence = uint64(time.Now().UnixNano())
		
		switch update.Type {
		case pb.UpdateType_UPDATE_TYPE_PILOT:
			if update.Pilot != nil {
				objID = update.Pilot.Address
				pbPilot := update.Pilot.ToProto()
				data, err = proto.Marshal(pbPilot)
			}
		case pb.UpdateType_UPDATE_TYPE_THERMAL:
			if update.Thermal != nil {
				objID = update.Thermal.ID
				pbThermal := update.Thermal.ToProto()
				data, err = proto.Marshal(pbThermal)
			}
		case pb.UpdateType_UPDATE_TYPE_STATION:
			if update.Station != nil {
				objID = update.Station.ChipID
				pbStation := update.Station.ToProto()
				data, err = proto.Marshal(pbStation)
			}
		}
		
		// Skip duplicates and marshal errors
		if objID != "" && !seen[objID] && err == nil {
			seen[objID] = true
			pbUpdate.Data = data
			updateBatch.Updates = append(updateBatch.Updates, pbUpdate)
		}
	}
	
	// Serialize message
	data, err := proto.Marshal(updateBatch)
	if err != nil {
		bm.logger.WithError(err).Error("Failed to marshal update batch")
		// Возвращаем объекты в пул даже при ошибке
		pool.Global.PutPbUpdateBatch(updateBatch)
		return 0
	}
	
	// Возвращаем batch в пул после использования
	defer pool.Global.PutPbUpdateBatch(updateBatch)
	
	// Send to all clients in group
	recipients := 0
	for client := range group.clients {
		// Additional filtering by exact radius
		if info, exists := bm.clients[client]; exists {
			// Check if any update is within client's radius
			inRadius := false
			for _, update := range updates {
				var lat, lon float64
				
				switch update.Type {
				case pb.UpdateType_UPDATE_TYPE_PILOT:
					if update.Pilot != nil && update.Pilot.Position != nil {
						lat = update.Pilot.Position.Latitude
						lon = update.Pilot.Position.Longitude
					}
				case pb.UpdateType_UPDATE_TYPE_THERMAL:
					if update.Thermal != nil && update.Thermal.Position != nil {
						lat = update.Thermal.Position.Latitude
						lon = update.Thermal.Position.Longitude
					}
				case pb.UpdateType_UPDATE_TYPE_STATION:
					if update.Station != nil && update.Station.Position != nil {
						lat = update.Station.Position.Latitude
						lon = update.Station.Position.Longitude
					}
				}
				
				dist := geo.Distance(info.centerLat, info.centerLon, lat, lon)
				if dist <= info.radiusKm {
					inRadius = true
					break
				}
			}
			
			if inRadius {
				select {
				case client.send <- data:
					recipients++
				default:
					// Client send buffer full, skip
					bm.logger.WithField("client", client.conn.RemoteAddr()).Warn("Client send buffer full")
				}
			}
		}
	}
	
	group.lastUpdate = time.Now()
	return recipients
}

// updateBroadcastMetrics updates performance metrics
func (bm *BroadcastManager) updateBroadcastMetrics(duration time.Duration, recipients int) {
	ms := float64(duration.Microseconds()) / 1000.0
	
	// Update average broadcast time (exponential moving average)
	if bm.metrics.AvgBroadcastTimeMs == 0 {
		bm.metrics.AvgBroadcastTimeMs = ms
	} else {
		bm.metrics.AvgBroadcastTimeMs = bm.metrics.AvgBroadcastTimeMs*0.9 + ms*0.1
	}
	
	// Update average recipients count
	if bm.metrics.AvgRecipientsCount == 0 {
		bm.metrics.AvgRecipientsCount = float64(recipients)
	} else {
		bm.metrics.AvgRecipientsCount = bm.metrics.AvgRecipientsCount*0.9 + float64(recipients)*0.1
	}
}

// metricsCollector periodically cleans up and collects metrics
func (bm *BroadcastManager) metricsCollector() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		bm.mu.Lock()
		
		// Clean up inactive groups
		for gh, group := range bm.groups {
			if time.Since(group.lastUpdate) > 5*time.Minute && len(group.clients) == 0 {
				delete(bm.groups, gh)
			}
		}
		
		// Update metrics
		atomic.StoreUint64(&bm.metrics.ClientsActive, uint64(len(bm.clients)))
		atomic.StoreUint64(&bm.metrics.GroupsActive, uint64(len(bm.groups)))
		
		bm.mu.Unlock()
		
		// Log metrics
		bm.logger.WithFields(logrus.Fields{
			"clients":             bm.metrics.ClientsActive,
			"groups":              bm.metrics.GroupsActive,
			"updates_received":    bm.metrics.UpdatesReceived,
			"updates_broadcast":   bm.metrics.UpdatesBroadcast,
			"avg_broadcast_ms":    bm.metrics.AvgBroadcastTimeMs,
			"avg_recipients":      bm.metrics.AvgRecipientsCount,
		}).Info("Broadcast metrics")
	}
}

// GetMetrics returns current broadcast metrics
func (bm *BroadcastManager) GetMetrics() BroadcastMetrics {
	return BroadcastMetrics{
		UpdatesReceived:    atomic.LoadUint64(&bm.metrics.UpdatesReceived),
		UpdatesBroadcast:   atomic.LoadUint64(&bm.metrics.UpdatesBroadcast),
		ClientsActive:      atomic.LoadUint64(&bm.metrics.ClientsActive),
		GroupsActive:       atomic.LoadUint64(&bm.metrics.GroupsActive),
		AvgBroadcastTimeMs: bm.metrics.AvgBroadcastTimeMs,
		AvgRecipientsCount: bm.metrics.AvgRecipientsCount,
	}
}