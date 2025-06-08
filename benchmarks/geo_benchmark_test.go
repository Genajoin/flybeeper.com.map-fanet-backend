package benchmarks

import (
	"math/rand"
	"testing"
	"time"

	"github.com/flybeeper/fanet-backend/internal/geo"
	"github.com/flybeeper/fanet-backend/internal/models"
)

// mockObject implements geo.Object interface for testing
type mockObject struct {
	id        string
	lat       float64
	lon       float64
	timestamp time.Time
}

func (m *mockObject) GetID() string        { return m.id }
func (m *mockObject) GetLatitude() float64 { return m.lat }
func (m *mockObject) GetLongitude() float64 { return m.lon }
func (m *mockObject) GetTimestamp() time.Time { return m.timestamp }

// BenchmarkGeohashEncode benchmarks geohash encoding
func BenchmarkGeohashEncode(b *testing.B) {
	testCases := []struct {
		name      string
		precision int
	}{
		{"Precision5", 5},
		{"Precision7", 7},
		{"Precision9", 9},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			lat := 46.52
			lon := 6.57
			
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = geo.Encode(lat, lon, tc.precision)
			}
		})
	}
}

// BenchmarkGeohashDecode benchmarks geohash decoding
func BenchmarkGeohashDecode(b *testing.B) {
	geohashes := []string{
		"u0k9q",     // precision 5
		"u0k9qxp",   // precision 7
		"u0k9qxpbp", // precision 9
	}

	for _, gh := range geohashes {
		b.Run("Precision"+string(rune(len(gh)+'0')), func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = geo.Decode(gh)
			}
		})
	}
}

// BenchmarkGeohashCover benchmarks geohash coverage calculation
func BenchmarkGeohashCover(b *testing.B) {
	testCases := []struct {
		name     string
		radiusKm float64
	}{
		{"5km", 5.0},
		{"50km", 50.0},
		{"200km", 200.0},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			lat := 46.52
			lon := 6.57
			
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = geo.Cover(lat, lon, tc.radiusKm, 0)
			}
		})
	}
}

// BenchmarkDistance benchmarks distance calculation
func BenchmarkDistance(b *testing.B) {
	lat1, lon1 := 46.52, 6.57
	lat2, lon2 := 46.53, 6.58
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = geo.Distance(lat1, lon1, lat2, lon2)
	}
}

// BenchmarkQuadTreeInsert benchmarks QuadTree insertion
func BenchmarkQuadTreeInsert(b *testing.B) {
	testCases := []struct {
		name  string
		count int
	}{
		{"100objects", 100},
		{"1000objects", 1000},
		{"10000objects", 10000},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			objects := generateMockObjects(tc.count)
			
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tree := geo.NewQuadTree(5 * time.Minute)
				for _, obj := range objects {
					tree.Insert(obj)
				}
			}
		})
	}
}

// BenchmarkQuadTreeQuery benchmarks QuadTree queries
func BenchmarkQuadTreeQuery(b *testing.B) {
	testCases := []struct {
		name      string
		objects   int
		radiusKm  float64
	}{
		{"1000objects_10km", 1000, 10.0},
		{"10000objects_10km", 10000, 10.0},
		{"10000objects_50km", 10000, 50.0},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			tree := geo.NewQuadTree(5 * time.Minute)
			objects := generateMockObjects(tc.objects)
			
			// Populate tree
			for _, obj := range objects {
				tree.Insert(obj)
			}
			
			centerLat := 46.52
			centerLon := 6.57
			
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = tree.QueryRadius(centerLat, centerLon, tc.radiusKm)
			}
		})
	}
}

// BenchmarkSpatialIndex benchmarks complete spatial index operations
func BenchmarkSpatialIndex(b *testing.B) {
	index := geo.NewSpatialIndex(5*time.Minute, 1000, 30*time.Second)
	
	// Populate with initial data
	objects := generateMockObjects(5000)
	for _, obj := range objects {
		index.Insert(obj)
	}
	
	b.Run("Insert", func(b *testing.B) {
		obj := &mockObject{
			id:        "test",
			lat:       46.52,
			lon:       6.57,
			timestamp: time.Now(),
		}
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			obj.id = string(rune(i))
			index.Insert(obj)
		}
	})
	
	b.Run("QueryRadius_10km", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = index.QueryRadius(46.52, 6.57, 10.0)
		}
	})
	
	b.Run("QueryRadius_50km_cached", func(b *testing.B) {
		// Warm up cache
		index.QueryRadius(46.52, 6.57, 50.0)
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = index.QueryRadius(46.52, 6.57, 50.0)
		}
	})
}

// BenchmarkLRUCache benchmarks LRU cache operations
func BenchmarkLRUCache(b *testing.B) {
	cache := geo.NewLRUCache(1000, 5*time.Minute)
	
	b.Run("Set", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			key := string(rune(i % 1000))
			cache.Set(key, i, 1)
		}
	})
	
	b.Run("Get_Hit", func(b *testing.B) {
		// Populate cache
		for i := 0; i < 100; i++ {
			cache.Set(string(rune(i)), i, 1)
		}
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			key := string(rune(i % 100))
			cache.Get(key)
		}
	})
	
	b.Run("Get_Miss", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			key := "nonexistent" + string(rune(i))
			cache.Get(key)
		}
	})
}

// Helper function to generate mock objects
func generateMockObjects(count int) []geo.Object {
	objects := make([]geo.Object, count)
	
	// Center around Geneva region
	centerLat := 46.52
	centerLon := 6.57
	spread := 2.0 // degrees
	
	for i := 0; i < count; i++ {
		objects[i] = &mockObject{
			id:        string(rune(i)),
			lat:       centerLat + (rand.Float64()-0.5)*spread,
			lon:       centerLon + (rand.Float64()-0.5)*spread,
			timestamp: time.Now(),
		}
	}
	
	return objects
}