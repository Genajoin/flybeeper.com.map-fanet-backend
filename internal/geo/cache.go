package geo

import (
	"container/list"
	"fmt"
	"sync"
	"time"
)

// CacheEntry represents a cached query result
type CacheEntry struct {
	Key       string
	Value     interface{}
	Timestamp time.Time
	Size      int
}

// LRUCache implements a thread-safe LRU cache with TTL
type LRUCache struct {
	capacity  int
	ttl       time.Duration
	items     map[string]*list.Element
	evictList *list.List
	mu        sync.RWMutex
	
	// Metrics
	hits   uint64
	misses uint64
}

// NewLRUCache creates a new LRU cache
func NewLRUCache(capacity int, ttl time.Duration) *LRUCache {
	return &LRUCache{
		capacity:  capacity,
		ttl:       ttl,
		items:     make(map[string]*list.Element),
		evictList: list.New(),
	}
}

// Get retrieves a value from the cache
func (c *LRUCache) Get(key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if elem, ok := c.items[key]; ok {
		entry := elem.Value.(*CacheEntry)
		
		// Check TTL
		if time.Since(entry.Timestamp) > c.ttl {
			c.removeElement(elem)
			c.misses++
			return nil, false
		}
		
		// Move to front (most recently used)
		c.evictList.MoveToFront(elem)
		c.hits++
		return entry.Value, true
	}
	
	c.misses++
	return nil, false
}

// Set adds or updates a value in the cache
func (c *LRUCache) Set(key string, value interface{}, size int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	// Check if key exists
	if elem, ok := c.items[key]; ok {
		// Update existing entry
		c.evictList.MoveToFront(elem)
		entry := elem.Value.(*CacheEntry)
		entry.Value = value
		entry.Timestamp = time.Now()
		entry.Size = size
		return
	}
	
	// Add new entry
	entry := &CacheEntry{
		Key:       key,
		Value:     value,
		Timestamp: time.Now(),
		Size:      size,
	}
	
	elem := c.evictList.PushFront(entry)
	c.items[key] = elem
	
	// Evict if over capacity
	if c.evictList.Len() > c.capacity {
		c.removeOldest()
	}
}

// Delete removes a key from the cache
func (c *LRUCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if elem, ok := c.items[key]; ok {
		c.removeElement(elem)
	}
}

// Clear removes all entries from the cache
func (c *LRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.items = make(map[string]*list.Element)
	c.evictList.Init()
}

// Size returns the number of items in the cache
func (c *LRUCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	return len(c.items)
}

// Stats returns cache statistics
func (c *LRUCache) Stats() (hits, misses uint64, hitRate float64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	hits = c.hits
	misses = c.misses
	total := hits + misses
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}
	return
}

// Clean removes expired entries
func (c *LRUCache) Clean() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	removed := 0
	now := time.Now()
	
	// Iterate from oldest to newest
	for elem := c.evictList.Back(); elem != nil; {
		entry := elem.Value.(*CacheEntry)
		if now.Sub(entry.Timestamp) > c.ttl {
			prev := elem.Prev()
			c.removeElement(elem)
			removed++
			elem = prev
		} else {
			// All newer entries are also valid
			break
		}
	}
	
	return removed
}

// removeOldest removes the oldest entry
func (c *LRUCache) removeOldest() {
	elem := c.evictList.Back()
	if elem != nil {
		c.removeElement(elem)
	}
}

// removeElement removes an element from the cache
func (c *LRUCache) removeElement(elem *list.Element) {
	entry := elem.Value.(*CacheEntry)
	delete(c.items, entry.Key)
	c.evictList.Remove(elem)
}

// GeoCache provides caching for geospatial queries
type GeoCache struct {
	radiusCache *LRUCache
	boundsCache *LRUCache
	mu          sync.RWMutex
}

// NewGeoCache creates a new geospatial cache
func NewGeoCache(capacity int, ttl time.Duration) *GeoCache {
	return &GeoCache{
		radiusCache: NewLRUCache(capacity, ttl),
		boundsCache: NewLRUCache(capacity, ttl),
	}
}

// GetRadius retrieves cached radius query results
func (gc *GeoCache) GetRadius(centerLat, centerLon, radiusKm float64) ([]Object, bool) {
	key := gc.radiusKey(centerLat, centerLon, radiusKm)
	if value, ok := gc.radiusCache.Get(key); ok {
		return value.([]Object), true
	}
	return nil, false
}

// SetRadius caches radius query results
func (gc *GeoCache) SetRadius(centerLat, centerLon, radiusKm float64, objects []Object) {
	key := gc.radiusKey(centerLat, centerLon, radiusKm)
	gc.radiusCache.Set(key, objects, len(objects))
}

// GetBounds retrieves cached bounds query results
func (gc *GeoCache) GetBounds(bounds Bounds) ([]Object, bool) {
	key := gc.boundsKey(bounds)
	if value, ok := gc.boundsCache.Get(key); ok {
		return value.([]Object), true
	}
	return nil, false
}

// SetBounds caches bounds query results
func (gc *GeoCache) SetBounds(bounds Bounds, objects []Object) {
	key := gc.boundsKey(bounds)
	gc.boundsCache.Set(key, objects, len(objects))
}

// InvalidateArea invalidates cache entries that might contain objects in the given area
func (gc *GeoCache) InvalidateArea(lat, lon, radiusKm float64) {
	// For simplicity, clear all caches when an update occurs
	// In production, implement more sophisticated invalidation
	gc.radiusCache.Clear()
	gc.boundsCache.Clear()
}

// Stats returns combined cache statistics
func (gc *GeoCache) Stats() map[string]interface{} {
	radiusHits, radiusMisses, radiusHitRate := gc.radiusCache.Stats()
	boundsHits, boundsMisses, boundsHitRate := gc.boundsCache.Stats()
	
	return map[string]interface{}{
		"radius_cache": map[string]interface{}{
			"size":     gc.radiusCache.Size(),
			"hits":     radiusHits,
			"misses":   radiusMisses,
			"hit_rate": radiusHitRate,
		},
		"bounds_cache": map[string]interface{}{
			"size":     gc.boundsCache.Size(),
			"hits":     boundsHits,
			"misses":   boundsMisses,
			"hit_rate": boundsHitRate,
		},
	}
}

// Clean removes expired entries from all caches
func (gc *GeoCache) Clean() int {
	return gc.radiusCache.Clean() + gc.boundsCache.Clean()
}

// radiusKey generates a cache key for radius queries
func (gc *GeoCache) radiusKey(centerLat, centerLon, radiusKm float64) string {
	// Round coordinates to reduce key space
	lat := float64(int(centerLat*100)) / 100
	lon := float64(int(centerLon*100)) / 100
	radius := float64(int(radiusKm*10)) / 10
	
	return fmt.Sprintf("radius:%.2f,%.2f,%.1f", lat, lon, radius)
}

// boundsKey generates a cache key for bounds queries
func (gc *GeoCache) boundsKey(bounds Bounds) string {
	// Round bounds to reduce key space
	return fmt.Sprintf("bounds:%.2f,%.2f,%.2f,%.2f",
		float64(int(bounds.MinLat*100))/100,
		float64(int(bounds.MinLon*100))/100,
		float64(int(bounds.MaxLat*100))/100,
		float64(int(bounds.MaxLon*100))/100,
	)
}