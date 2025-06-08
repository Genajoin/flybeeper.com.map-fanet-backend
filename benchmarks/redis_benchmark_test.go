package benchmarks

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/flybeeper/fanet-backend/internal/models"
	"github.com/flybeeper/fanet-backend/internal/repository"
	"github.com/redis/go-redis/v9"
)

// setupRedisForBenchmark creates a Redis client for benchmarking
func setupRedisForBenchmark() *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr:         "localhost:6379",
		DB:           15, // Use separate DB for tests
		MaxRetries:   1,
		MinIdleConns: 10,
		MaxIdleConns: 100,
	})
	
	// Clear test DB
	ctx := context.Background()
	client.FlushDB(ctx)
	
	return client
}

// BenchmarkRedisOperations benchmarks basic Redis operations
func BenchmarkRedisOperations(b *testing.B) {
	client := setupRedisForBenchmark()
	defer client.Close()
	
	ctx := context.Background()
	
	b.Run("GeoAdd", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			client.GeoAdd(ctx, "bench:geo", &redis.GeoLocation{
				Name:      fmt.Sprintf("obj%d", i),
				Longitude: 6.5 + float64(i%100)/1000,
				Latitude:  46.5 + float64(i%100)/1000,
			})
		}
	})
	
	b.Run("GeoRadius", func(b *testing.B) {
		// Populate with test data
		for i := 0; i < 1000; i++ {
			client.GeoAdd(ctx, "bench:geo", &redis.GeoLocation{
				Name:      fmt.Sprintf("obj%d", i),
				Longitude: 6.5 + float64(i%100)/100,
				Latitude:  46.5 + float64(i/100)/100,
			})
		}
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			client.GeoRadius(ctx, "bench:geo", 6.5, 46.5, &redis.GeoRadiusQuery{
				Radius: 50,
				Unit:   "km",
				Count:  100,
			})
		}
	})
	
	b.Run("Pipeline_10commands", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			pipe := client.Pipeline()
			for j := 0; j < 10; j++ {
				pipe.HSet(ctx, fmt.Sprintf("bench:hash:%d", j), "field", "value")
			}
			pipe.Exec(ctx)
		}
	})
}

// BenchmarkOptimizedRedisRepository benchmarks the optimized repository
func BenchmarkOptimizedRedisRepository(b *testing.B) {
	client := setupRedisForBenchmark()
	defer client.Close()
	
	repo := repository.NewOptimizedRedisRepository(client, 200.0)
	ctx := context.Background()
	
	b.Run("SavePilot_Single", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			pilot := &models.Pilot{
				Address:  fmt.Sprintf("pilot%d", i),
				Name:     "Test Pilot",
				Position: &models.GeoPoint{Latitude: 46.5, Longitude: 6.5},
				Altitude: 1500,
				Speed:    45.0,
				LastSeen: time.Now(),
			}
			repo.SavePilot(ctx, pilot)
		}
	})
	
	b.Run("SavePilot_Batch10", func(b *testing.B) {
		pilots := make([]*models.Pilot, 10)
		for i := range pilots {
			pilots[i] = &models.Pilot{
				Address:  fmt.Sprintf("pilot%d", i),
				Name:     "Test Pilot",
				Position: &models.GeoPoint{Latitude: 46.5, Longitude: 6.5},
				Altitude: 1500,
				Speed:    45.0,
				LastSeen: time.Now(),
			}
		}
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Update IDs to avoid conflicts
			for j := range pilots {
				pilots[j].Address = fmt.Sprintf("pilot%d_%d", i, j)
			}
			repo.SavePilotBatch(ctx, pilots)
		}
	})
	
	b.Run("GetPilotsInRadius_Cached", func(b *testing.B) {
		// Populate with test data
		for i := 0; i < 100; i++ {
			pilot := &models.Pilot{
				Address:  fmt.Sprintf("pilot%d", i),
				Position: &models.GeoPoint{
					Latitude:  46.5 + float64(i%10)/100,
					Longitude: 6.5 + float64(i/10)/100,
				},
				Altitude: 1500,
				LastSeen: time.Now(),
			}
			repo.SavePilot(ctx, pilot)
		}
		
		// Warm up spatial index
		repo.GetPilotsInRadius(ctx, models.GeoPoint{Latitude: 46.5, Longitude: 6.5}, 50.0)
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			repo.GetPilotsInRadius(ctx, models.GeoPoint{Latitude: 46.5, Longitude: 6.5}, 50.0)
		}
	})
}

// BenchmarkPipelineFlush benchmarks pipeline flush performance
func BenchmarkPipelineFlush(b *testing.B) {
	client := setupRedisForBenchmark()
	defer client.Close()
	
	ctx := context.Background()
	
	testCases := []struct {
		name     string
		commands int
	}{
		{"10commands", 10},
		{"50commands", 50},
		{"100commands", 100},
		{"500commands", 500},
	}
	
	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				pipe := client.Pipeline()
				
				// Add commands to pipeline
				for j := 0; j < tc.commands; j++ {
					pipe.HSet(ctx, fmt.Sprintf("bench:key:%d:%d", i, j), map[string]interface{}{
						"field1": "value1",
						"field2": j,
						"field3": time.Now().Unix(),
					})
				}
				
				// Execute pipeline
				_, err := pipe.Exec(ctx)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkRedisMemoryUsage benchmarks memory usage patterns
func BenchmarkRedisMemoryUsage(b *testing.B) {
	client := setupRedisForBenchmark()
	defer client.Close()
	
	ctx := context.Background()
	
	b.Run("Pilot_Storage", func(b *testing.B) {
		// Measure memory for storing pilots
		for i := 0; i < 1000; i++ {
			pilotKey := fmt.Sprintf("bench:pilot:%d", i)
			client.HSet(ctx, pilotKey, map[string]interface{}{
				"name":       "Test Pilot",
				"type":       1,
				"lat":        46.5,
				"lon":        6.5,
				"alt":        1500,
				"speed":      45.0,
				"heading":    180.0,
				"climb_rate": 2.5,
				"last_seen":  time.Now().Unix(),
			})
			
			// Also add to geo index
			client.GeoAdd(ctx, "bench:pilots:geo", &redis.GeoLocation{
				Name:      fmt.Sprintf("%d", i),
				Longitude: 6.5,
				Latitude:  46.5,
			})
		}
		
		// Check memory usage
		info, _ := client.Info(ctx, "memory").Result()
		b.Logf("Memory info after 1000 pilots: %s", info)
	})
}