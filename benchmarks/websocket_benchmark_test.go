package benchmarks

import (
	"fmt"
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
	// Упрощенный benchmark без создания реальных клиентов
	// Тестируем только производительность создания BroadcastManager и UpdatePacket
	
	logger := logrus.NewEntry(logrus.New())
	logger.Logger.SetLevel(logrus.WarnLevel)
	
	spatial := geo.NewSpatialIndex(5*time.Minute, 1000, 30*time.Second)
	
	b.Run("CreateBroadcastManager", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = handler.NewBroadcastManager(spatial)
		}
	})
	
	b.Run("CreateUpdatePacket", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			update := &handler.UpdatePacket{
				Type: pb.UpdateType_UPDATE_TYPE_PILOT,
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
			_ = update // Используем переменную
		}
	})
}

// BenchmarkGeohashGrouping benchmarks geohash operations
func BenchmarkGeohashGrouping(b *testing.B) {
	// Упрощенный benchmark геохеширования без реальных клиентов
	
	b.Run("GeohashCalculation", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			lat := 40.0 + float64(i%100)/10.0
			lon := -74.0 + float64(i/100)/10.0
			_ = geo.Encode(lat, lon, 5) // Precision 5
		}
	})
	
	b.Run("UpdatePacketCreation", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			update := &handler.UpdatePacket{
				Type: pb.UpdateType_UPDATE_TYPE_PILOT,
				Pilot: &models.Pilot{
					Address:  fmt.Sprintf("pilot_%d", i),
					Position: &models.GeoPoint{
						Latitude:  40.0 + float64(i%10)/10.0,
						Longitude: -74.0 + float64(i/10)/10.0,
					},
					LastSeen: time.Now(),
				},
				Timestamp: time.Now(),
			}
			_ = update
		}
	})
}

// BenchmarkAdaptiveScheduler benchmarks adaptive scheduling calculations
func BenchmarkAdaptiveScheduler(b *testing.B) {
	// Упрощенный benchmark вычислений без реальных клиентов
	
	b.Run("ActivityMetricsCalculation", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			metrics := handler.ActivityMetrics{
				ObjectCount:     50 + i%100,
				UpdateFrequency: 5.0 + float64(i%10),
				AverageSpeed:    30.0 + float64(i%50),
				ThermalActivity: 0.3 + float64(i%3)/10.0,
			}
			// Симулируем вычисления
			_ = metrics.ObjectCount * 2
			_ = metrics.UpdateFrequency * 1.5
			_ = metrics.AverageSpeed / 10.0
		}
	})
	
	b.Run("SchedulingDecision", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Симулируем принятие решения об обновлении
			objectCount := i % 100
			frequency := float64(i%10) / 2.0
			
			// Простая логика планирования
			var interval time.Duration
			if objectCount > 50 {
				interval = time.Duration(1000/frequency) * time.Millisecond
			} else {
				interval = 2 * time.Second
			}
			_ = interval
		}
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
					Type:     pb.UpdateType_UPDATE_TYPE_PILOT,
					Sequence: uint64(i),
				}
			}
			
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				batch := &pb.UpdateBatch{
					Updates: updates,
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