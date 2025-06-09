package benchmarks

import (
	"testing"

	"github.com/flybeeper/fanet-backend/internal/mqtt"
)

// Sample FANET packets for benchmarking
var (
	// Type 1 (Air tracking) packet
	airPacket = []byte{
		0x11, 0x23, 0x45, 0x67, // Header + address
		0x78, 0x9A, 0xBC,       // Latitude
		0xDE, 0xF0, 0x12,       // Longitude
		0x34, 0x05,             // Altitude
		0x96,                   // Speed + heading
		0x87,                   // Climb rate
	}
	
	// Type 2 (Name) packet
	namePacket = []byte{
		0x12, 0x23, 0x45, 0x67,                         // Header + address
		'T', 'e', 's', 't', ' ', 'P', 'i', 'l', 'o', 't', // Name
	}
	
	// Type 4 (Service) packet
	servicePacket = []byte{
		0x14, 0x23, 0x45, 0x67, // Header + address
		0xAB,                   // Service info
		0x34, 0x05,             // Temperature
		0x78,                   // Wind
		0x9A,                   // Humidity
		0xBC, 0xDE,             // Pressure
	}
	
	// Type 7 (Ground tracking) packet  
	groundPacket = []byte{
		0x17, 0x23, 0x45, 0x67, // Header + address
		0x78, 0x9A, 0xBC,       // Latitude
		0xDE, 0xF0, 0x12,       // Longitude
		0x02,                   // Status
	}
	
	// Type 9 (Thermal) packet
	thermalPacket = []byte{
		0x19, 0x23, 0x45, 0x67, // Header + address
		0x78, 0x9A, 0xBC,       // Latitude
		0xDE, 0xF0, 0x12,       // Longitude
		0x34, 0x05,             // Altitude
		0xA5,                   // Climb rate
		0x50,                   // Pilot count
	}
)

// BenchmarkParseFANETPacket benchmarks parsing different packet types
func BenchmarkParseFANETPacket(b *testing.B) {
	parser := mqtt.NewParser(nil)
	
	testCases := []struct {
		name   string
		packet []byte
	}{
		{"Type1_Air", airPacket},
		{"Type2_Name", namePacket},
		{"Type4_Service", servicePacket},
		{"Type7_Ground", groundPacket},
		{"Type9_Thermal", thermalPacket},
	}
	
	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = parser.Parse("fb/b/test/f/1", tc.packet)
			}
		})
	}
}

// BenchmarkParseHeader benchmarks header parsing
func BenchmarkParseHeader(b *testing.B) {
	header := []byte{0x11, 0x23, 0x45, 0x67}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate header parsing
		_ = header[0] & 0x07
	}
}

// BenchmarkParseCoordinates benchmarks coordinate parsing
func BenchmarkParseCoordinates(b *testing.B) {
	// 24-bit coordinates
	latBytes := []byte{0x78, 0x9A, 0xBC}
	lonBytes := []byte{0xDE, 0xF0, 0x12}
	
	b.Run("ParseLatitude", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Simulate latitude parsing
			_ = int32(latBytes[0])<<16 | int32(latBytes[1])<<8 | int32(latBytes[2])
		}
	})
	
	b.Run("ParseLongitude", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Simulate longitude parsing
			_ = int32(lonBytes[0])<<16 | int32(lonBytes[1])<<8 | int32(lonBytes[2])
		}
	})
}

// BenchmarkParseAltitude benchmarks altitude parsing
func BenchmarkParseAltitude(b *testing.B) {
	altBytes := []byte{0x34, 0x05} // 1332m
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate altitude parsing
		_ = int32(altBytes[0])<<8 | int32(altBytes[1])
	}
}

// BenchmarkParseSpeedHeading benchmarks speed and heading parsing
func BenchmarkParseSpeedHeading(b *testing.B) {
	speedHeadingByte := byte(0x96)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate speed/heading parsing
		_ = speedHeadingByte >> 3
		_ = speedHeadingByte & 0x07
	}
}

// BenchmarkBatchParsing benchmarks parsing multiple packets
func BenchmarkBatchParsing(b *testing.B) {
	parser := mqtt.NewParser(nil)
	
	// Create a batch of mixed packet types
	packets := [][]byte{
		airPacket,
		namePacket,
		servicePacket,
		groundPacket,
		thermalPacket,
	}
	
	b.Run("Sequential", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, packet := range packets {
				_, _ = parser.Parse("fb/b/test/f/1", packet)
			}
		}
	})
	
	b.Run("WithValidation", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, packet := range packets {
				result, err := parser.Parse("fb/b/test/f/1", packet)
				if err == nil && result != nil {
					// Simulate validation
					switch result.Data.(type) {
					case *mqtt.AirTrackingData:
						// Validate tracking data
					case *mqtt.NameData:
						// Validate name data
					}
				}
			}
		}
	})
}

// BenchmarkMemoryAllocations benchmarks memory allocations during parsing
func BenchmarkMemoryAllocations(b *testing.B) {
	parser := mqtt.NewParser(nil)
	
	b.Run("Type1_Allocations", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = parser.Parse("fb/b/test/f/1", airPacket)
		}
	})
	
	b.Run("Type2_Allocations", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = parser.Parse("fb/b/test/f/2", namePacket)
		}
	})
}