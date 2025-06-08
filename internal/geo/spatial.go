package geo

import (
	"math"
	"sync"
	"time"
)

// SpatialIndex combines QuadTree and caching for optimal performance
type SpatialIndex struct {
	tree      *QuadTree
	cache     *GeoCache
	mu        sync.RWMutex
	
	// Bloom filter for fast existence checks
	bloom     *BloomFilter
	
	// Metrics
	metrics   *Metrics
}

// Metrics tracks spatial index performance
type Metrics struct {
	mu              sync.RWMutex
	QueryCount      uint64
	UpdateCount     uint64
	CacheHits       uint64
	CacheMisses     uint64
	TreeQueries     uint64
	AvgQueryTimeMs  float64
}

// NewSpatialIndex creates a new spatial index
func NewSpatialIndex(maxAge time.Duration, cacheCapacity int, cacheTTL time.Duration) *SpatialIndex {
	return &SpatialIndex{
		tree:    NewQuadTree(maxAge),
		cache:   NewGeoCache(cacheCapacity, cacheTTL),
		bloom:   NewBloomFilter(100000, 0.01), // 100k items, 1% false positive
		metrics: &Metrics{},
	}
}

// Insert adds or updates an object in the spatial index
func (si *SpatialIndex) Insert(obj Object) {
	si.mu.Lock()
	defer si.mu.Unlock()
	
	// Update bloom filter
	si.bloom.Add(obj.GetID())
	
	// Update tree
	si.tree.Update(obj)
	
	// Invalidate cache for affected area
	si.cache.InvalidateArea(obj.GetLatitude(), obj.GetLongitude(), 50.0) // 50km radius
	
	si.metrics.UpdateCount++
}

// Remove removes an object from the spatial index
func (si *SpatialIndex) Remove(id string) {
	si.mu.Lock()
	defer si.mu.Unlock()
	
	si.tree.Remove(id)
	// Note: Can't remove from bloom filter, will be cleared on rebuild
	
	si.metrics.UpdateCount++
}

// QueryRadius returns all objects within a radius
func (si *SpatialIndex) QueryRadius(centerLat, centerLon, radiusKm float64) []Object {
	start := time.Now()
	defer func() {
		si.updateQueryMetrics(time.Since(start))
	}()
	
	si.metrics.QueryCount++
	
	// Check cache first
	if objects, ok := si.cache.GetRadius(centerLat, centerLon, radiusKm); ok {
		si.metrics.CacheHits++
		return objects
	}
	
	si.metrics.CacheMisses++
	
	// Query tree
	si.mu.RLock()
	objects := si.tree.QueryRadius(centerLat, centerLon, radiusKm)
	si.mu.RUnlock()
	
	si.metrics.TreeQueries++
	
	// Cache result
	si.cache.SetRadius(centerLat, centerLon, radiusKm, objects)
	
	return objects
}

// QueryBounds returns all objects within bounds
func (si *SpatialIndex) QueryBounds(bounds Bounds) []Object {
	start := time.Now()
	defer func() {
		si.updateQueryMetrics(time.Since(start))
	}()
	
	si.metrics.QueryCount++
	
	// Check cache first
	if objects, ok := si.cache.GetBounds(bounds); ok {
		si.metrics.CacheHits++
		return objects
	}
	
	si.metrics.CacheMisses++
	
	// Query tree
	si.mu.RLock()
	objects := si.tree.QueryBounds(bounds)
	si.mu.RUnlock()
	
	si.metrics.TreeQueries++
	
	// Cache result
	si.cache.SetBounds(bounds, objects)
	
	return objects
}

// QueryGeohash returns all objects within a geohash and its neighbors
func (si *SpatialIndex) QueryGeohash(geohash string) []Object {
	// Get bounds for geohash
	minLat, minLon, maxLat, maxLon := BoundingBox(geohash)
	
	// Query main geohash
	bounds := Bounds{
		MinLat: minLat,
		MinLon: minLon,
		MaxLat: maxLat,
		MaxLon: maxLon,
	}
	
	objects := si.QueryBounds(bounds)
	
	// Also query neighbors for edge cases
	neighbors := Neighbors(geohash)
	seen := make(map[string]bool)
	
	// Add objects from main query
	for _, obj := range objects {
		seen[obj.GetID()] = true
	}
	
	// Add objects from neighbors
	for _, neighborHash := range neighbors {
		nMinLat, nMinLon, nMaxLat, nMaxLon := BoundingBox(neighborHash)
		nBounds := Bounds{
			MinLat: nMinLat,
			MinLon: nMinLon,
			MaxLat: nMaxLat,
			MaxLon: nMaxLon,
		}
		
		neighborObjects := si.QueryBounds(nBounds)
		for _, obj := range neighborObjects {
			if !seen[obj.GetID()] {
				objects = append(objects, obj)
				seen[obj.GetID()] = true
			}
		}
	}
	
	return objects
}

// Exists quickly checks if an object might exist (bloom filter)
func (si *SpatialIndex) Exists(id string) bool {
	return si.bloom.Contains(id)
}

// Size returns the number of objects in the index
func (si *SpatialIndex) Size() int {
	si.mu.RLock()
	defer si.mu.RUnlock()
	
	return si.tree.Size()
}

// Clean removes old objects and returns the count
func (si *SpatialIndex) Clean() int {
	si.mu.Lock()
	defer si.mu.Unlock()
	
	removed := si.tree.Clean()
	si.cache.Clean()
	
	// Rebuild bloom filter if many items were removed
	if removed > si.tree.Size()/4 {
		si.rebuildBloomFilter()
	}
	
	return removed
}

// GetMetrics returns performance metrics
func (si *SpatialIndex) GetMetrics() Metrics {
	si.metrics.mu.RLock()
	defer si.metrics.mu.RUnlock()
	
	return *si.metrics
}

// updateQueryMetrics updates query performance metrics
func (si *SpatialIndex) updateQueryMetrics(duration time.Duration) {
	si.metrics.mu.Lock()
	defer si.metrics.mu.Unlock()
	
	// Update average query time (exponential moving average)
	ms := float64(duration.Microseconds()) / 1000.0
	if si.metrics.AvgQueryTimeMs == 0 {
		si.metrics.AvgQueryTimeMs = ms
	} else {
		si.metrics.AvgQueryTimeMs = si.metrics.AvgQueryTimeMs*0.9 + ms*0.1
	}
}

// rebuildBloomFilter recreates the bloom filter from current objects
func (si *SpatialIndex) rebuildBloomFilter() {
	si.bloom = NewBloomFilter(100000, 0.01)
	// Note: In a real implementation, iterate through tree objects
	// and add them to the new bloom filter
}

// BloomFilter provides fast probabilistic existence checks
type BloomFilter struct {
	bits     []uint64
	size     uint64
	hashFunc []func(string) uint64
}

// NewBloomFilter creates a new bloom filter
func NewBloomFilter(expectedItems int, falsePositiveRate float64) *BloomFilter {
	// Calculate optimal size and hash functions
	m := uint64(math.Ceil(-float64(expectedItems) * math.Log(falsePositiveRate) / math.Pow(math.Log(2), 2)))
	k := int(math.Ceil(math.Log(2) * float64(m) / float64(expectedItems)))
	
	// Ensure minimum size
	if m < 64 {
		m = 64
	}
	if k < 1 {
		k = 1
	}
	if k > 10 {
		k = 10 // Limit hash functions
	}
	
	bf := &BloomFilter{
		bits:     make([]uint64, (m+63)/64),
		size:     m,
		hashFunc: make([]func(string) uint64, k),
	}
	
	// Create hash functions
	for i := 0; i < k; i++ {
		seed := uint64(i)
		bf.hashFunc[i] = func(s string) uint64 {
			h := seed
			for _, c := range s {
				h = h*31 + uint64(c)
			}
			return h % bf.size
		}
	}
	
	return bf
}

// Add adds an item to the bloom filter
func (bf *BloomFilter) Add(item string) {
	for _, hash := range bf.hashFunc {
		pos := hash(item)
		idx := pos / 64
		bit := pos % 64
		bf.bits[idx] |= 1 << bit
	}
}

// Contains checks if an item might be in the set
func (bf *BloomFilter) Contains(item string) bool {
	for _, hash := range bf.hashFunc {
		pos := hash(item)
		idx := pos / 64
		bit := pos % 64
		if bf.bits[idx]&(1<<bit) == 0 {
			return false
		}
	}
	return true
}

// OptimalGeohashPrecision calculates the best geohash precision for a given area
func OptimalGeohashPrecision(radiusKm float64) int {
	// Choose precision where cell size is about 1/4 of the radius
	for precision := 1; precision <= 9; precision++ {
		if cellSize, ok := GeohashPrecisionKm[precision]; ok {
			if cellSize <= radiusKm/4 {
				return precision
			}
		}
	}
	return 5 // default
}