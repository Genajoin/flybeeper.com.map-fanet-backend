package benchmarks

// Реалистичные бенчмарки для FANET геопространственных операций
// 
// Ожидаемые результаты (цели производительности):
// - GeohashEncode: < 100 ns/op, < 2 allocs/op
// - Distance: < 100 ns/op, 0 allocs/op  
// - QuadTreeQuery (300 pilots, 50km): < 50µs, < 1KB allocs
// - SpatialIndex Insert: < 10µs, < 500B allocs
// - SpatialIndex Query (200km): < 30ms (Redis target)
// - SpatialIndex Query (50km, cached): < 15ms (WebSocket target)
//
// Реалистичные размеры данных:
// - 50-300 пилотов в альпийском регионе (200км)
// - 25-200км радиусы запросов
// - Швейцарские Альпы: 45-47°N, 6-10°E

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/flybeeper/fanet-backend/internal/geo"
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
	// Реалистичные радиусы для FANET системы
	testCases := []struct {
		name     string
		radiusKm float64
	}{
		{"25km", 25.0},   // Локальный полет
		{"50km", 50.0},   // WebSocket клиент
		{"200km", 200.0}, // DEFAULT_RADIUS_KM
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
	// Реалистичные размеры для FANET системы
	testCases := []struct {
		name  string
		count int
	}{
		{"50pilots", 50},     // Маленький регион
		{"300pilots", 300},   // Альпийский регион  
		{"1000pilots", 1000}, // Большое соревнование
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
	// Реалистичные запросы для FANET API
	testCases := []struct {
		name      string
		objects   int
		radiusKm  float64
	}{
		{"300pilots_50km", 300, 50.0},   // WebSocket клиент
		{"300pilots_200km", 300, 200.0}, // REST API snapshot
		{"1000pilots_100km", 1000, 100.0}, // Большой регион
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
	// Реалистичные настройки: 5min TTL, 1k cache, 30s cache TTL
	index := geo.NewSpatialIndex(5*time.Minute, 1000, 30*time.Second)
	
	// Реалистичное количество пилотов: 200-500 в регионе 200км (швейцарские Альпы)
	initialObjects := generateMockObjects(300)
	for _, obj := range initialObjects {
		index.Insert(obj)
	}
	
	b.Run("Insert_RealisticRate", func(b *testing.B) {
		// Цель: 10k MQTT msg/sec, каждое создает 1 insert
		// Но бенчмарк тестирует одну операцию, не нагрузку
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			obj := &mockObject{
				id: fmt.Sprintf("pilot_%d", 1000+i), // Уникальные ID
				// Швейцарские Альпы: 46-47°N, 6-10°E
				lat:       46.0 + rand.Float64(), // 46-47°N  
				lon:       6.0 + rand.Float64()*4.0, // 6-10°E
				timestamp: time.Now(),
			}
			b.StartTimer()
			
			index.Insert(obj)
		}
	})
	
	b.Run("QueryRadius_200km", func(b *testing.B) {
		// Реалистичный запрос: 200км (DEFAULT_RADIUS_KM)
		// Цель: < 30ms p95 (Redis query target)
		centerLat, centerLon := 46.52, 6.57 // Geneva
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = index.QueryRadius(centerLat, centerLon, 200.0)
		}
	})
	
	b.Run("QueryRadius_50km_Cached", func(b *testing.B) {
		// Типичный WebSocket клиент: 50км радиус
		// Цель: < 15ms p95 с кешем
		centerLat, centerLon := 46.52, 6.57
		
		// Warm up cache
		index.QueryRadius(centerLat, centerLon, 50.0)
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = index.QueryRadius(centerLat, centerLon, 50.0)
		}
	})
	
	b.Run("MixedOperations_Realistic", func(b *testing.B) {
		// Симуляция реального workload:
		// 90% queries, 10% inserts (как в production)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if i%10 == 0 {
				// 10% inserts
				obj := &mockObject{
					id:        fmt.Sprintf("pilot_%d", 2000+i),
					lat:       46.0 + rand.Float64(),
					lon:       6.0 + rand.Float64()*4.0,
					timestamp: time.Now(),
				}
				index.Insert(obj)
			} else {
				// 90% queries  
				_ = index.QueryRadius(46.52, 6.57, 100.0)
			}
		}
	})
}

// BenchmarkLRUCache benchmarks LRU cache operations
func BenchmarkLRUCache(b *testing.B) {
	cache := geo.NewLRUCache(1000, 5*time.Minute)
	
	b.Run("Set", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			key := fmt.Sprintf("key_%d", i%1000)
			cache.Set(key, i, 1)
		}
	})
	
	b.Run("Get_Hit", func(b *testing.B) {
		// Populate cache
		for i := 0; i < 100; i++ {
			cache.Set(fmt.Sprintf("key_%d", i), i, 1)
		}
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			key := fmt.Sprintf("key_%d", i%100)
			cache.Get(key)
		}
	})
	
	b.Run("Get_Miss", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			key := fmt.Sprintf("nonexistent_%d", i)
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
			id:        fmt.Sprintf("obj_%d", i),
			lat:       centerLat + (rand.Float64()-0.5)*spread,
			lon:       centerLon + (rand.Float64()-0.5)*spread,
			timestamp: time.Now(),
		}
	}
	
	return objects
}

// generateRealisticPilots generates realistic pilot data for Swiss Alps region
func generateRealisticPilots(count int) []geo.Object {
	objects := make([]geo.Object, count)
	
	// Популярные места для полетов в швейцарских Альпах
	flyingSites := []struct {
		name string
		lat  float64
		lon  float64
	}{
		{"Chamonix", 45.9237, 6.8694},      // Популярное место
		{"Verbier", 46.0967, 7.2286},       // Ski resort
		{"Interlaken", 46.6863, 7.8632},    // Туристический центр  
		{"Zermatt", 46.0207, 7.7491},       // Маттерхорн
		{"Grindelwald", 46.6244, 8.0411},   // Юнгфрауйох
		{"Davos", 46.8008, 9.8370},         // Восточные Альпы
		{"St.Moritz", 46.4908, 9.8355},     // Элитный курорт
		{"Geneva", 46.5197, 6.6323},        // Женевское озеро
	}
	
	for i := 0; i < count; i++ {
		// Выбираем случайное место для полета
		site := flyingSites[rand.Intn(len(flyingSites))]
		
		// Добавляем случайное отклонение (до 50км от центра)
		latOffset := (rand.Float64() - 0.5) * 0.9 // ~50км в широте
		lonOffset := (rand.Float64() - 0.5) * 1.2 // ~50км в долготе
		
		objects[i] = &mockObject{
			id:        fmt.Sprintf("pilot_%d", i),
			lat:       site.lat + latOffset,
			lon:       site.lon + lonOffset,
			timestamp: time.Now().Add(-time.Duration(rand.Intn(300)) * time.Second), // Последние 5 минут
		}
	}
	
	return objects
}