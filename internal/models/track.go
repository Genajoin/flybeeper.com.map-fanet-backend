package models

import (
	"time"
	
	"github.com/flybeeper/fanet-backend/pkg/pb"
)

// TrackPoint представляет точку трека полета
type TrackPoint struct {
	// Позиция
	Position *GeoPoint `json:"position"`  // Координаты
	Altitude int32      `json:"altitude"`  // Высота (м)
	
	// Движение
	Speed float32 `json:"speed"` // Скорость (км/ч)
	Climb float32 `json:"climb"` // Вариометр (м/с)
	
	// Время
	Timestamp time.Time `json:"timestamp"` // Unix timestamp
}

// GetID возвращает уникальный идентификатор для geo.Object
func (tp *TrackPoint) GetID() string {
	return tp.Timestamp.Format("20060102150405")
}

// GetLatitude возвращает широту для geo.Object
func (tp *TrackPoint) GetLatitude() float64 {
	if tp.Position != nil {
		return tp.Position.Latitude
	}
	return 0
}

// GetLongitude возвращает долготу для geo.Object
func (tp *TrackPoint) GetLongitude() float64 {
	if tp.Position != nil {
		return tp.Position.Longitude
	}
	return 0
}

// GetTimestamp возвращает время для geo.Object
func (tp *TrackPoint) GetTimestamp() time.Time {
	return tp.Timestamp
}

// ToProto конвертирует TrackPoint в protobuf представление
func (tp *TrackPoint) ToProto() *pb.TrackPoint {
	trackPoint := &pb.TrackPoint{
		Altitude:  tp.Altitude,
		Speed:     tp.Speed,
		Climb:     tp.Climb,
		Timestamp: tp.Timestamp.Unix(),
	}
	
	if tp.Position != nil {
		trackPoint.Position = &pb.GeoPoint{
			Latitude:  tp.Position.Latitude,
			Longitude: tp.Position.Longitude,
		}
	}
	
	return trackPoint
}

// Track представляет полный трек полета
type Track struct {
	// Идентификация
	Addr uint32 `json:"addr"`  // FANET адрес пилота
	
	// Точки трека
	Points []*TrackPoint `json:"points"`
	
	// Время
	StartTime time.Time `json:"start_time"` // Начало трека
	EndTime   time.Time `json:"end_time"`   // Конец трека
}

// ToProto конвертирует Track в protobuf представление
func (t *Track) ToProto() *pb.Track {
	track := &pb.Track{
		Addr:      t.Addr,
		StartTime: t.StartTime.Unix(),
		EndTime:   t.EndTime.Unix(),
	}
	
	track.Points = make([]*pb.TrackPoint, len(t.Points))
	for i, point := range t.Points {
		track.Points[i] = point.ToProto()
	}
	
	return track
}

// GetDuration возвращает продолжительность трека
func (t *Track) GetDuration() time.Duration {
	if t.EndTime.IsZero() || t.StartTime.IsZero() {
		return 0
	}
	return t.EndTime.Sub(t.StartTime)
}

// GetDistance возвращает общую дистанцию трека (приблизительно)
func (t *Track) GetDistance() float64 {
	if len(t.Points) < 2 {
		return 0
	}
	
	totalDistance := 0.0
	for i := 1; i < len(t.Points); i++ {
		prev := t.Points[i-1].Position
		curr := t.Points[i].Position
		
		if prev != nil && curr != nil {
			distance := prev.DistanceTo(*curr)
			totalDistance += distance
		}
	}
	
	return totalDistance
}

// GetMaxAltitude возвращает максимальную высоту в треке
func (t *Track) GetMaxAltitude() int32 {
	if len(t.Points) == 0 {
		return 0
	}
	
	maxAlt := t.Points[0].Altitude
	for _, point := range t.Points {
		if point.Altitude > maxAlt {
			maxAlt = point.Altitude
		}
	}
	
	return maxAlt
}

// GetMaxClimb возвращает максимальную скороподъемность в треке
func (t *Track) GetMaxClimb() float32 {
	if len(t.Points) == 0 {
		return 0
	}
	
	maxClimb := t.Points[0].Climb
	for _, point := range t.Points {
		if point.Climb > maxClimb {
			maxClimb = point.Climb
		}
	}
	
	return maxClimb
}