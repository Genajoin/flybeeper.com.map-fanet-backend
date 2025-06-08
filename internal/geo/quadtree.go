package geo

import (
	"sync"
	"time"
)

const (
	// Maximum number of objects in a node before splitting
	nodeCapacity = 32
	
	// Maximum depth of the tree
	maxDepth = 12
	
	// Minimum node size in degrees
	minNodeSize = 0.001
)

// Object represents a spatial object in the QuadTree
type Object interface {
	GetID() string
	GetLatitude() float64
	GetLongitude() float64
	GetTimestamp() time.Time
}

// Bounds represents a rectangular area
type Bounds struct {
	MinLat float64
	MinLon float64
	MaxLat float64
	MaxLon float64
}

// Contains checks if a point is within bounds
func (b Bounds) Contains(lat, lon float64) bool {
	return lat >= b.MinLat && lat <= b.MaxLat &&
		lon >= b.MinLon && lon <= b.MaxLon
}

// Intersects checks if two bounds intersect
func (b Bounds) Intersects(other Bounds) bool {
	return !(b.MaxLat < other.MinLat || b.MinLat > other.MaxLat ||
		b.MaxLon < other.MinLon || b.MinLon > other.MaxLon)
}

// Width returns the width of the bounds in degrees
func (b Bounds) Width() float64 {
	return b.MaxLon - b.MinLon
}

// Height returns the height of the bounds in degrees
func (b Bounds) Height() float64 {
	return b.MaxLat - b.MinLat
}

// Center returns the center point of the bounds
func (b Bounds) Center() (lat, lon float64) {
	return (b.MinLat + b.MaxLat) / 2, (b.MinLon + b.MaxLon) / 2
}

// QuadTree represents a spatial index for fast geospatial queries
type QuadTree struct {
	root     *node
	mu       sync.RWMutex
	objects  map[string]Object // Fast lookup by ID
	maxAge   time.Duration     // Maximum age for objects
}

// node represents a node in the QuadTree
type node struct {
	bounds   Bounds
	objects  []Object
	depth    int
	
	// Child nodes (nil if leaf)
	nw *node // Northwest
	ne *node // Northeast
	sw *node // Southwest
	se *node // Southeast
}

// NewQuadTree creates a new QuadTree with world bounds
func NewQuadTree(maxAge time.Duration) *QuadTree {
	return &QuadTree{
		root: &node{
			bounds: Bounds{
				MinLat: -90.0,
				MinLon: -180.0,
				MaxLat: 90.0,
				MaxLon: 180.0,
			},
			objects: make([]Object, 0, nodeCapacity),
			depth:   0,
		},
		objects: make(map[string]Object),
		maxAge:  maxAge,
	}
}

// Insert adds an object to the QuadTree
func (qt *QuadTree) Insert(obj Object) {
	qt.mu.Lock()
	defer qt.mu.Unlock()
	
	// Update object map
	qt.objects[obj.GetID()] = obj
	
	// Insert into tree
	qt.root.insert(obj)
}

// Remove removes an object from the QuadTree
func (qt *QuadTree) Remove(id string) {
	qt.mu.Lock()
	defer qt.mu.Unlock()
	
	obj, exists := qt.objects[id]
	if !exists {
		return
	}
	
	delete(qt.objects, id)
	qt.root.remove(obj)
}

// Update updates an object's position in the QuadTree
func (qt *QuadTree) Update(obj Object) {
	qt.mu.Lock()
	defer qt.mu.Unlock()
	
	// Remove old position
	if oldObj, exists := qt.objects[obj.GetID()]; exists {
		qt.root.remove(oldObj)
	}
	
	// Insert new position
	qt.objects[obj.GetID()] = obj
	qt.root.insert(obj)
}

// QueryRadius returns all objects within a radius from a point
func (qt *QuadTree) QueryRadius(centerLat, centerLon, radiusKm float64) []Object {
	qt.mu.RLock()
	defer qt.mu.RUnlock()
	
	// Convert radius to approximate degrees
	radiusDeg := radiusKm / 111.0
	
	// Create bounding box for initial filtering
	bounds := Bounds{
		MinLat: centerLat - radiusDeg,
		MaxLat: centerLat + radiusDeg,
		MinLon: centerLon - radiusDeg,
		MaxLon: centerLon + radiusDeg,
	}
	
	// Query tree
	candidates := qt.root.query(bounds)
	
	// Filter by exact distance
	result := make([]Object, 0, len(candidates))
	for _, obj := range candidates {
		dist := Distance(centerLat, centerLon, obj.GetLatitude(), obj.GetLongitude())
		if dist <= radiusKm {
			result = append(result, obj)
		}
	}
	
	return result
}

// QueryBounds returns all objects within bounds
func (qt *QuadTree) QueryBounds(bounds Bounds) []Object {
	qt.mu.RLock()
	defer qt.mu.RUnlock()
	
	return qt.root.query(bounds)
}

// Size returns the number of objects in the tree
func (qt *QuadTree) Size() int {
	qt.mu.RLock()
	defer qt.mu.RUnlock()
	
	return len(qt.objects)
}

// Clean removes objects older than maxAge
func (qt *QuadTree) Clean() int {
	qt.mu.Lock()
	defer qt.mu.Unlock()
	
	now := time.Now()
	removed := 0
	
	for id, obj := range qt.objects {
		if now.Sub(obj.GetTimestamp()) > qt.maxAge {
			delete(qt.objects, id)
			qt.root.remove(obj)
			removed++
		}
	}
	
	// Rebuild tree if many objects were removed
	if removed > len(qt.objects)/4 {
		qt.rebuild()
	}
	
	return removed
}

// rebuild reconstructs the tree from scratch
func (qt *QuadTree) rebuild() {
	oldRoot := qt.root
	qt.root = &node{
		bounds:  oldRoot.bounds,
		objects: make([]Object, 0, nodeCapacity),
		depth:   0,
	}
	
	for _, obj := range qt.objects {
		qt.root.insert(obj)
	}
}

// insert adds an object to the node
func (n *node) insert(obj Object) {
	lat, lon := obj.GetLatitude(), obj.GetLongitude()
	
	// Check if point is within bounds
	if !n.bounds.Contains(lat, lon) {
		return
	}
	
	// If node has children, insert into appropriate child
	if n.nw != nil {
		n.insertIntoChild(obj)
		return
	}
	
	// Add to this node
	n.objects = append(n.objects, obj)
	
	// Split if necessary
	if len(n.objects) > nodeCapacity && n.shouldSplit() {
		n.split()
	}
}

// insertIntoChild inserts object into the appropriate child node
func (n *node) insertIntoChild(obj Object) {
	lat, lon := obj.GetLatitude(), obj.GetLongitude()
	centerLat, centerLon := n.bounds.Center()
	
	if lat >= centerLat {
		if lon >= centerLon {
			n.ne.insert(obj)
		} else {
			n.nw.insert(obj)
		}
	} else {
		if lon >= centerLon {
			n.se.insert(obj)
		} else {
			n.sw.insert(obj)
		}
	}
}

// shouldSplit checks if node should be split
func (n *node) shouldSplit() bool {
	return n.depth < maxDepth &&
		n.bounds.Width() > minNodeSize &&
		n.bounds.Height() > minNodeSize
}

// split divides the node into four children
func (n *node) split() {
	centerLat, centerLon := n.bounds.Center()
	
	// Create child nodes
	n.nw = &node{
		bounds: Bounds{
			MinLat: centerLat,
			MinLon: n.bounds.MinLon,
			MaxLat: n.bounds.MaxLat,
			MaxLon: centerLon,
		},
		objects: make([]Object, 0, nodeCapacity),
		depth:   n.depth + 1,
	}
	
	n.ne = &node{
		bounds: Bounds{
			MinLat: centerLat,
			MinLon: centerLon,
			MaxLat: n.bounds.MaxLat,
			MaxLon: n.bounds.MaxLon,
		},
		objects: make([]Object, 0, nodeCapacity),
		depth:   n.depth + 1,
	}
	
	n.sw = &node{
		bounds: Bounds{
			MinLat: n.bounds.MinLat,
			MinLon: n.bounds.MinLon,
			MaxLat: centerLat,
			MaxLon: centerLon,
		},
		objects: make([]Object, 0, nodeCapacity),
		depth:   n.depth + 1,
	}
	
	n.se = &node{
		bounds: Bounds{
			MinLat: n.bounds.MinLat,
			MinLon: centerLon,
			MaxLat: centerLat,
			MaxLon: n.bounds.MaxLon,
		},
		objects: make([]Object, 0, nodeCapacity),
		depth:   n.depth + 1,
	}
	
	// Move objects to children
	oldObjects := n.objects
	n.objects = nil
	
	for _, obj := range oldObjects {
		n.insertIntoChild(obj)
	}
}

// remove removes an object from the node
func (n *node) remove(obj Object) bool {
	// If node has children, try to remove from appropriate child
	if n.nw != nil {
		lat, lon := obj.GetLatitude(), obj.GetLongitude()
		centerLat, centerLon := n.bounds.Center()
		
		if lat >= centerLat {
			if lon >= centerLon {
				return n.ne.remove(obj)
			}
			return n.nw.remove(obj)
		} else {
			if lon >= centerLon {
				return n.se.remove(obj)
			}
			return n.sw.remove(obj)
		}
	}
	
	// Remove from this node
	for i, o := range n.objects {
		if o.GetID() == obj.GetID() {
			n.objects = append(n.objects[:i], n.objects[i+1:]...)
			return true
		}
	}
	
	return false
}

// query returns all objects within the given bounds
func (n *node) query(bounds Bounds) []Object {
	// Check if bounds intersect
	if !n.bounds.Intersects(bounds) {
		return nil
	}
	
	var result []Object
	
	// If node has children, query them
	if n.nw != nil {
		result = append(result, n.nw.query(bounds)...)
		result = append(result, n.ne.query(bounds)...)
		result = append(result, n.sw.query(bounds)...)
		result = append(result, n.se.query(bounds)...)
		return result
	}
	
	// Check objects in this node
	for _, obj := range n.objects {
		if bounds.Contains(obj.GetLatitude(), obj.GetLongitude()) {
			result = append(result, obj)
		}
	}
	
	return result
}