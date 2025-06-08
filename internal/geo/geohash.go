package geo

import (
	"math"
	"strings"
)

const (
	// Base32 encoding for geohash
	base32 = "0123456789bcdefghjkmnpqrstuvwxyz"
	
	// Maximum precision for geohash
	maxPrecision = 12
	
	// Earth radius in kilometers
	earthRadiusKm = 6371.0
)

// GeohashPrecisionKm maps geohash precision to approximate size in km
var GeohashPrecisionKm = map[int]float64{
	1: 5000.0,   // ±2500 km
	2: 1250.0,   // ±625 km
	3: 156.0,    // ±78 km
	4: 39.1,     // ±19.5 km
	5: 4.9,      // ±2.4 km
	6: 1.2,      // ±0.61 km
	7: 0.152,    // ±0.076 km
	8: 0.038,    // ±0.019 km
	9: 0.0048,   // ±0.0024 km
}

// Direction represents neighboring directions for geohash
type Direction int

const (
	North Direction = iota
	NorthEast
	East
	SouthEast
	South
	SouthWest
	West
	NorthWest
)

// Encode converts latitude and longitude to geohash with given precision
func Encode(lat, lon float64, precision int) string {
	if precision <= 0 || precision > maxPrecision {
		precision = 5 // default precision
	}

	idx := 0
	bit := 0
	evenBit := true
	geohash := make([]byte, 0, precision)

	latRange := [2]float64{-90.0, 90.0}
	lonRange := [2]float64{-180.0, 180.0}

	for len(geohash) < precision {
		if evenBit {
			mid := (lonRange[0] + lonRange[1]) / 2
			if lon >= mid {
				idx |= (1 << (4 - bit))
				lonRange[0] = mid
			} else {
				lonRange[1] = mid
			}
		} else {
			mid := (latRange[0] + latRange[1]) / 2
			if lat >= mid {
				idx |= (1 << (4 - bit))
				latRange[0] = mid
			} else {
				latRange[1] = mid
			}
		}

		evenBit = !evenBit
		bit++

		if bit == 5 {
			geohash = append(geohash, base32[idx])
			bit = 0
			idx = 0
		}
	}

	return string(geohash)
}

// Decode converts geohash to latitude and longitude
func Decode(geohash string) (lat, lon float64) {
	evenBit := true
	latRange := [2]float64{-90.0, 90.0}
	lonRange := [2]float64{-180.0, 180.0}

	for _, c := range geohash {
		idx := strings.IndexByte(base32, byte(c))
		if idx == -1 {
			continue
		}

		for i := 4; i >= 0; i-- {
			bit := (idx >> i) & 1
			if evenBit {
				mid := (lonRange[0] + lonRange[1]) / 2
				if bit == 1 {
					lonRange[0] = mid
				} else {
					lonRange[1] = mid
				}
			} else {
				mid := (latRange[0] + latRange[1]) / 2
				if bit == 1 {
					latRange[0] = mid
				} else {
					latRange[1] = mid
				}
			}
			evenBit = !evenBit
		}
	}

	lat = (latRange[0] + latRange[1]) / 2
	lon = (lonRange[0] + lonRange[1]) / 2
	return
}

// Neighbors returns all 8 neighboring geohashes
func Neighbors(geohash string) map[Direction]string {
	lat, lon := Decode(geohash)
	precision := len(geohash)
	
	// Calculate approximate size of one geohash cell
	cellSize := GeohashPrecisionKm[precision] / earthRadiusKm * 180 / math.Pi
	
	neighbors := make(map[Direction]string, 8)
	
	// Generate neighbors
	neighbors[North] = Encode(lat+cellSize, lon, precision)
	neighbors[NorthEast] = Encode(lat+cellSize, lon+cellSize, precision)
	neighbors[East] = Encode(lat, lon+cellSize, precision)
	neighbors[SouthEast] = Encode(lat-cellSize, lon+cellSize, precision)
	neighbors[South] = Encode(lat-cellSize, lon, precision)
	neighbors[SouthWest] = Encode(lat-cellSize, lon-cellSize, precision)
	neighbors[West] = Encode(lat, lon-cellSize, precision)
	neighbors[NorthWest] = Encode(lat+cellSize, lon-cellSize, precision)
	
	return neighbors
}

// Cover returns geohashes that cover a circular area
func Cover(centerLat, centerLon, radiusKm float64, precision int) []string {
	// Determine optimal precision based on radius
	if precision <= 0 {
		precision = OptimalPrecision(radiusKm)
	}
	
	// Convert radius to degrees (approximate)
	radiusDeg := radiusKm / 111.0 // 1 degree ≈ 111 km
	
	// Calculate bounding box
	minLat := centerLat - radiusDeg
	maxLat := centerLat + radiusDeg
	minLon := centerLon - radiusDeg/(math.Cos(centerLat*math.Pi/180))
	maxLon := centerLon + radiusDeg/(math.Cos(centerLat*math.Pi/180))
	
	// Generate geohashes for the bounding box
	geohashes := make(map[string]bool)
	cellSize := GeohashPrecisionKm[precision] / 111.0
	
	for lat := minLat; lat <= maxLat; lat += cellSize {
		for lon := minLon; lon <= maxLon; lon += cellSize {
			// Check if point is within radius
			dist := Distance(centerLat, centerLon, lat, lon)
			if dist <= radiusKm {
				gh := Encode(lat, lon, precision)
				geohashes[gh] = true
				
				// Also add neighbors to ensure full coverage
				for _, neighbor := range Neighbors(gh) {
					nlat, nlon := Decode(neighbor)
					if Distance(centerLat, centerLon, nlat, nlon) <= radiusKm {
						geohashes[neighbor] = true
					}
				}
			}
		}
	}
	
	// Convert map to slice
	result := make([]string, 0, len(geohashes))
	for gh := range geohashes {
		result = append(result, gh)
	}
	
	return result
}

// OptimalPrecision returns the optimal geohash precision for a given radius
func OptimalPrecision(radiusKm float64) int {
	// Choose precision where cell size is about 1/4 of the radius
	targetSize := radiusKm / 4.0
	
	for precision := 1; precision <= maxPrecision; precision++ {
		if cellSize, ok := GeohashPrecisionKm[precision]; ok {
			if cellSize <= targetSize {
				return precision
			}
		}
	}
	
	return 5 // default precision
}

// Distance calculates the Haversine distance between two points in kilometers
func Distance(lat1, lon1, lat2, lon2 float64) float64 {
	lat1Rad := lat1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	deltaLat := (lat2 - lat1) * math.Pi / 180
	deltaLon := (lon2 - lon1) * math.Pi / 180

	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(deltaLon/2)*math.Sin(deltaLon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadiusKm * c
}

// BoundingBox returns the bounding box for a geohash
func BoundingBox(geohash string) (minLat, minLon, maxLat, maxLon float64) {
	evenBit := true
	latRange := [2]float64{-90.0, 90.0}
	lonRange := [2]float64{-180.0, 180.0}

	for _, c := range geohash {
		idx := strings.IndexByte(base32, byte(c))
		if idx == -1 {
			continue
		}

		for i := 4; i >= 0; i-- {
			bit := (idx >> i) & 1
			if evenBit {
				mid := (lonRange[0] + lonRange[1]) / 2
				if bit == 1 {
					lonRange[0] = mid
				} else {
					lonRange[1] = mid
				}
			} else {
				mid := (latRange[0] + latRange[1]) / 2
				if bit == 1 {
					latRange[0] = mid
				} else {
					latRange[1] = mid
				}
			}
			evenBit = !evenBit
		}
	}

	return latRange[0], lonRange[0], latRange[1], lonRange[1]
}

// Contains checks if a point is within a geohash bounding box
func Contains(geohash string, lat, lon float64) bool {
	minLat, minLon, maxLat, maxLon := BoundingBox(geohash)
	return lat >= minLat && lat <= maxLat && lon >= minLon && lon <= maxLon
}

// CommonPrefix returns the common prefix of two geohashes
func CommonPrefix(gh1, gh2 string) string {
	minLen := len(gh1)
	if len(gh2) < minLen {
		minLen = len(gh2)
	}
	
	for i := 0; i < minLen; i++ {
		if gh1[i] != gh2[i] {
			return gh1[:i]
		}
	}
	
	return gh1[:minLen]
}