package benchmarks

import (
	"testing"
	"time"

	"github.com/flybeeper/fanet-backend/internal/geo"
	"github.com/flybeeper/fanet-backend/internal/handler"
	"github.com/flybeeper/fanet-backend/internal/models"
	"github.com/flybeeper/fanet-backend/pkg/pb"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

// mockClient implements a minimal WebSocket client for testing
type mockClient struct {
	conn         *websocket.Conn
	send         chan []byte
	updateSignal chan bool
}

func (m *mockClient) Close() {
	close(m.send)
	close(m.updateSignal)
}

// BenchmarkBroadcastManager benchmarks the broadcast manager
func BenchmarkBroadcastManager(b *testing.B) {
	logger := logrus.NewEntry(logrus.New())
	logger.Logger.SetLevel(logrus.WarnLevel)
	
	spatial := geo.NewSpatialIndex(5*time.Minute, 1000, 30*time.Second)
	bm := handler.NewBroadcastManager(spatial)
	
	// Create mock clients
	clients := make([]*handler.Client, 0)
	for i := 0; i < 100; i++ {
		client := &handler.Client{
			// Minimal fields needed for testing
		}
		clients = append(clients, client)
		
		// Register clients distributed across regions
		lat := 46.0 + float64(i%10)/10.0
		lon := 6.0 + float64(i/10)/10.0
		bm.Register(client, lat, lon, 50.0)
	}
	
	// Create test update
	update := &handler.UpdatePacket{
		Type: pb.UpdateType_PILOT_UPDATE,
		Pilot: &models.Pilot{
			Address:  "test123",
			Name:     "Test Pilot",
			Position: &models.GeoPoint{Latitude: 46.5, Longitude: 6.5},
			Altitude: 1500,
			Speed:    45.0,
			LastSeen: time.Now(),
		},
		Timestamp: time.Now(),
	}
	
	b.Run("Broadcast_100clients", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			bm.Broadcast(update)
			// Allow some time for processing
			time.Sleep(1 * time.Millisecond)
		}
	})
	
	// Clean up
	for _, client := range clients {
		bm.Unregister(client)
	}
}

// BenchmarkGeohashGrouping benchmarks client grouping by geohash
func BenchmarkGeohashGrouping(b *testing.B) {
	testCases := []struct {
		name         string
		clientCount  int
		updateCount  int
	}{
		{"100clients_10updates", 100, 10},
		{"1000clients_10updates", 1000, 10},
		{"1000clients_100updates", 1000, 100},
	}
	
	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			logger := logrus.NewEntry(logrus.New())
			logger.Logger.SetLevel(logrus.WarnLevel)
			
			spatial := geo.NewSpatialIndex(5*time.Minute, 1000, 30*time.Second)
			bm := handler.NewBroadcastManager(spatial)
			
			// Register clients
			clients := make([]*handler.Client, tc.clientCount)
			for i := 0; i < tc.clientCount; i++ {
				client := &handler.Client{}
				clients[i] = client
				
				lat := 40.0 + float64(i%100)/10.0
				lon := -74.0 + float64(i/100)/10.0
				bm.Register(client, lat, lon, 50.0)
			}
			
			// Create updates
			updates := make([]*handler.UpdatePacket, tc.updateCount)
			for i := 0; i < tc.updateCount; i++ {
				updates[i] = &handler.UpdatePacket{
					Type: pb.UpdateType_PILOT_UPDATE,
					Pilot: &models.Pilot{
						Address:  string(rune(i)),
						Position: &models.GeoPoint{
							Latitude:  40.0 + float64(i%10)/10.0,
							Longitude: -74.0 + float64(i/10)/10.0,
						},
						LastSeen: time.Now(),
					},
					Timestamp: time.Now(),
				}
			}
			
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for _, update := range updates {
					bm.Broadcast(update)
				}
			}
			
			// Clean up
			for _, client := range clients {
				bm.Unregister(client)
			}
		})
	}
}

// BenchmarkAdaptiveScheduler benchmarks adaptive update scheduling
func BenchmarkAdaptiveScheduler(b *testing.B) {
	logger := logrus.NewEntry(logrus.New())
	as := handler.NewAdaptiveScheduler(1*time.Second, logger)
	
	// Create mock client
	client := &handler.Client{}
	
	// Test metrics calculation
	b.Run("CalculateActivityScore", func(b *testing.B) {
		metrics := handler.ActivityMetrics{
			ObjectCount:     50,
			UpdateFrequency: 5.0,
			AverageSpeed:    30.0,
			ThermalActivity: 0.3,
		}
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			as.RegisterClient(client, metrics)
			as.UnregisterClient(client)
		}
	})
	
	// Test update scheduling
	b.Run("UpdateMetrics", func(b *testing.B) {
		initialMetrics := handler.ActivityMetrics{
			ObjectCount: 10,
		}
		as.RegisterClient(client, initialMetrics)
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			newMetrics := handler.ActivityMetrics{
				ObjectCount:     i % 100,
				UpdateFrequency: float64(i%10) / 2.0,
				AverageSpeed:    float64(i % 50),
				ThermalActivity: float64(i%3) / 3.0,
			}
			as.UpdateMetrics(client, newMetrics)
		}
		
		as.UnregisterClient(client)
	})
}

// BenchmarkUpdateBatching benchmarks update batching performance
func BenchmarkUpdateBatching(b *testing.B) {
	testCases := []struct {
		name      string
		batchSize int
	}{
		{"Batch10", 10},
		{"Batch50", 50},
		{"Batch100", 100},
	}
	
	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			// Create updates
			updates := make([]*pb.Update, tc.batchSize)
			for i := 0; i < tc.batchSize; i++ {
				updates[i] = &pb.Update{
					Type:      pb.UpdateType_PILOT_UPDATE,
					Timestamp: time.Now().Unix(),
					Sequence:  uint64(i),
				}
			}
			
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				batch := &pb.UpdateBatch{
					Updates:  updates,
					Sequence: uint64(i),
				}
				_ = batch // Simulate processing
			}
		})
	}
}

// BenchmarkDeltaCompression benchmarks delta compression (placeholder)
func BenchmarkDeltaCompression(b *testing.B) {
	// Current position
	current := &models.Pilot{
		Position: &models.GeoPoint{Latitude: 46.5, Longitude: 6.5},
		Altitude: 1500,
		Speed:    45.0,
		Heading:  180.0,
	}
	
	// New position (small change)
	updated := &models.Pilot{
		Position: &models.GeoPoint{Latitude: 46.5001, Longitude: 6.5001},
		Altitude: 1505,
		Speed:    46.0,
		Heading:  182.0,
	}
	
	b.Run("CalculateDelta", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Simulate delta calculation
			_ = updated.Altitude - current.Altitude
			_ = updated.Speed - current.Speed
			_ = updated.Heading - current.Heading
		}
	})
}