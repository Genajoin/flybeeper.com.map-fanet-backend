package handler

import (
	"strconv"
	"time"

	"github.com/flybeeper/fanet-backend/internal/models"
	"github.com/flybeeper/fanet-backend/pkg/pb"
)

// Конвертеры из внутренних моделей в Protobuf

func convertPilotsToProto(pilots []*models.Pilot) []*pb.Pilot {
	result := make([]*pb.Pilot, len(pilots))
	for i, pilot := range pilots {
		result[i] = convertPilotToProto(pilot)
	}
	return result
}

func convertPilotToProto(pilot *models.Pilot) *pb.Pilot {
	// Конвертируем DeviceID из hex string в uint32
	addr, _ := strconv.ParseUint(pilot.DeviceID, 16, 32)

	return &pb.Pilot{
		Addr: uint32(addr),
		Name: pilot.Name,
		Type: convertAircraftTypeToProto(pilot.AircraftType),
		Position: &pb.GeoPoint{
			Latitude:  pilot.Position.Latitude,
			Longitude: pilot.Position.Longitude,
		},
		Altitude:    int32(pilot.Position.Altitude),
		Speed:       float32(pilot.Speed),
		Climb:       float32(pilot.ClimbRate) / 10, // ClimbRate в 0.1 м/с -> м/с
		Course:      float32(pilot.Heading),
		LastUpdate:  pilot.LastUpdate.Unix(),
		TrackOnline: pilot.TrackOnline,
		Battery:     uint32(pilot.Battery),
	}
}

func convertAircraftTypeToProto(t uint8) pb.PilotType {
	switch t {
	case 1:
		return pb.PilotType_PILOT_TYPE_PARAGLIDER
	case 2:
		return pb.PilotType_PILOT_TYPE_HANGGLIDER
	case 3:
		return pb.PilotType_PILOT_TYPE_GLIDER
	case 4:
		return pb.PilotType_PILOT_TYPE_POWERED
	case 5:
		return pb.PilotType_PILOT_TYPE_HELICOPTER
	case 6:
		return pb.PilotType_PILOT_TYPE_UAV
	case 7:
		return pb.PilotType_PILOT_TYPE_BALLOON
	default:
		return pb.PilotType_PILOT_TYPE_UNKNOWN
	}
}

func convertThermalsToProto(thermals []*models.Thermal) []*pb.Thermal {
	result := make([]*pb.Thermal, len(thermals))
	for i, thermal := range thermals {
		result[i] = convertThermalToProto(thermal)
	}
	return result
}

func convertThermalToProto(thermal *models.Thermal) *pb.Thermal {
	// Конвертируем ID и ReportedBy
	id, _ := strconv.ParseUint(thermal.ID, 10, 64)
	addr, _ := strconv.ParseUint(thermal.ReportedBy, 16, 32)

	return &pb.Thermal{
		Id:   id,
		Addr: uint32(addr),
		Position: &pb.GeoPoint{
			Latitude:  thermal.Center.Latitude,
			Longitude: thermal.Center.Longitude,
		},
		Altitude:    thermal.Altitude,
		Quality:     uint32(thermal.Quality),
		Climb:       float32(thermal.ClimbRate) / 10, // ClimbRate в 0.1 м/с -> м/с
		WindSpeed:   float32(thermal.WindSpeed) / 3.6, // км/ч -> м/с
		WindHeading: float32(thermal.WindDirection),
		Timestamp:   thermal.Timestamp.Unix(),
	}
}

func convertStationsToProto(stations []*models.Station) []*pb.Station {
	result := make([]*pb.Station, len(stations))
	for i, station := range stations {
		result[i] = convertStationToProto(station)
	}
	return result
}

func convertStationToProto(station *models.Station) *pb.Station {
	// Конвертируем ID
	addr, _ := strconv.ParseUint(station.ID, 16, 32)

	return &pb.Station{
		Addr: uint32(addr),
		Name: station.Name,
		Position: &pb.GeoPoint{
			Latitude:  station.Position.Latitude,
			Longitude: station.Position.Longitude,
		},
		Temperature: float32(station.Temperature),
		WindSpeed:   float32(station.WindSpeed) / 3.6, // км/ч -> м/с
		WindHeading: float32(station.WindDirection),
		WindGusts:   float32(station.WindGusts) / 3.6, // км/ч -> м/с
		Humidity:    uint32(station.Humidity),
		Pressure:    float32(station.Pressure),
		Battery:     uint32(station.Battery),
		LastUpdate:  station.LastUpdate.Unix(),
	}
}

func convertTrackToProto(points []models.GeoPoint) []*pb.TrackPoint {
	result := make([]*pb.TrackPoint, len(points))
	for i, point := range points {
		result[i] = &pb.TrackPoint{
			Position: &pb.GeoPoint{
				Latitude:  point.Latitude,
				Longitude: point.Longitude,
			},
			Altitude:  int32(point.Altitude),
			Speed:     0, // Не сохраняется в текущей схеме
			Climb:     0, // Не сохраняется в текущей схеме
			Timestamp: time.Now().Unix(), // Приблизительное время
		}
	}
	return result
}

// Конвертеры в JSON для fallback

func convertSnapshotToJSON(response *pb.SnapshotResponse) map[string]interface{} {
	return map[string]interface{}{
		"pilots":   convertPilotsToJSONArray(protoToModelsPilots(response.Pilots)),
		"thermals": convertThermalsToJSONArray(protoToModelsThermals(response.Thermals)),
		"stations": convertStationsToJSONArray(protoToModelsStations(response.Stations)),
		"sequence": response.Sequence,
	}
}

func convertPilotsToJSONArray(pilots []*models.Pilot) []map[string]interface{} {
	result := make([]map[string]interface{}, len(pilots))
	for i, pilot := range pilots {
		result[i] = convertPilotToJSON(pilot)
	}
	return result
}

func convertPilotToJSON(pilot *models.Pilot) map[string]interface{} {
	addr, _ := strconv.ParseUint(pilot.DeviceID, 16, 32)
	
	return map[string]interface{}{
		"addr": addr,
		"name": pilot.Name,
		"type": getAircraftTypeName(pilot.AircraftType),
		"position": map[string]interface{}{
			"latitude":  pilot.Position.Latitude,
			"longitude": pilot.Position.Longitude,
		},
		"altitude":     pilot.Position.Altitude,
		"speed":        pilot.Speed,
		"climb":        float32(pilot.ClimbRate) / 10,
		"course":       pilot.Heading,
		"last_update":  pilot.LastUpdate.Unix(),
		"track_online": pilot.TrackOnline,
		"battery":      pilot.Battery,
	}
}

func convertThermalsToJSONArray(thermals []*models.Thermal) []map[string]interface{} {
	result := make([]map[string]interface{}, len(thermals))
	for i, thermal := range thermals {
		result[i] = convertThermalToJSON(thermal)
	}
	return result
}

func convertThermalToJSON(thermal *models.Thermal) map[string]interface{} {
	id, _ := strconv.ParseUint(thermal.ID, 10, 64)
	addr, _ := strconv.ParseUint(thermal.ReportedBy, 16, 32)

	return map[string]interface{}{
		"id":   id,
		"addr": addr,
		"position": map[string]interface{}{
			"latitude":  thermal.Center.Latitude,
			"longitude": thermal.Center.Longitude,
		},
		"altitude":     thermal.Altitude,
		"quality":      thermal.Quality,
		"climb":        float32(thermal.ClimbRate) / 10,
		"wind_speed":   float32(thermal.WindSpeed) / 3.6,
		"wind_heading": thermal.WindDirection,
		"timestamp":    thermal.Timestamp.Unix(),
	}
}

func convertStationsToJSONArray(stations []*models.Station) []map[string]interface{} {
	result := make([]map[string]interface{}, len(stations))
	for i, station := range stations {
		result[i] = convertStationToJSON(station)
	}
	return result
}

func convertStationToJSON(station *models.Station) map[string]interface{} {
	addr, _ := strconv.ParseUint(station.ID, 16, 32)

	return map[string]interface{}{
		"addr": addr,
		"name": station.Name,
		"position": map[string]interface{}{
			"latitude":  station.Position.Latitude,
			"longitude": station.Position.Longitude,
		},
		"temperature":  station.Temperature,
		"wind_speed":   float32(station.WindSpeed) / 3.6,
		"wind_heading": station.WindDirection,
		"wind_gusts":   float32(station.WindGusts) / 3.6,
		"humidity":     station.Humidity,
		"pressure":     station.Pressure,
		"battery":      station.Battery,
		"last_update":  station.LastUpdate.Unix(),
	}
}

func convertTrackToJSON(track *pb.Track) map[string]interface{} {
	points := make([]map[string]interface{}, len(track.Points))
	for i, point := range track.Points {
		points[i] = map[string]interface{}{
			"position": map[string]interface{}{
				"latitude":  point.Position.Latitude,
				"longitude": point.Position.Longitude,
			},
			"altitude":  point.Altitude,
			"speed":     point.Speed,
			"climb":     point.Climb,
			"timestamp": point.Timestamp,
		}
	}

	return map[string]interface{}{
		"track": map[string]interface{}{
			"addr":       track.Addr,
			"points":     points,
			"start_time": track.StartTime,
			"end_time":   track.EndTime,
		},
	}
}

// Вспомогательные функции

func protoToModelsPilots(pilots []*pb.Pilot) []*models.Pilot {
	result := make([]*models.Pilot, len(pilots))
	for i, pilot := range pilots {
		result[i] = &models.Pilot{
			DeviceID:     formatAddr(pilot.Addr),
			Name:         pilot.Name,
			AircraftType: protoToAircraftType(pilot.Type),
			Position: &models.GeoPoint{
				Latitude:  pilot.Position.Latitude,
				Longitude: pilot.Position.Longitude,
				Altitude:  int16(pilot.Altitude),
			},
			Speed:       float32(pilot.Speed),
			ClimbRate:   int16(pilot.Climb * 10),
			Heading:     float32(pilot.Course),
			TrackOnline: pilot.TrackOnline,
			Battery:     uint8(pilot.Battery),
			LastUpdate:  time.Unix(pilot.LastUpdate, 0),
		}
	}
	return result
}

func protoToModelsThermals(thermals []*pb.Thermal) []*models.Thermal {
	result := make([]*models.Thermal, len(thermals))
	for i, thermal := range thermals {
		result[i] = &models.Thermal{
			ID:         strconv.FormatUint(thermal.Id, 10),
			ReportedBy: formatAddr(thermal.Addr),
			Center: models.GeoPoint{
				Latitude:  thermal.Position.Latitude,
				Longitude: thermal.Position.Longitude,
			},
			Altitude:      thermal.Altitude,
			Quality:       int32(thermal.Quality),
			ClimbRate:     float32(thermal.Climb),
			WindSpeed:     uint8(thermal.WindSpeed * 3.6),
			WindDirection: uint16(thermal.WindHeading),
			Timestamp:     time.Unix(thermal.Timestamp, 0),
		}
	}
	return result
}

func protoToModelsStations(stations []*pb.Station) []*models.Station {
	result := make([]*models.Station, len(stations))
	for i, station := range stations {
		result[i] = &models.Station{
			ID:   formatAddr(station.Addr),
			Name: station.Name,
			Position: &models.GeoPoint{
				Latitude:  station.Position.Latitude,
				Longitude: station.Position.Longitude,
			},
			Temperature:   int8(station.Temperature),
			WindSpeed:     uint8(station.WindSpeed * 3.6),
			WindDirection: uint16(station.WindHeading),
			WindGusts:     uint8(station.WindGusts * 3.6),
			Humidity:      uint8(station.Humidity),
			Pressure:      uint16(station.Pressure),
			Battery:       uint8(station.Battery),
			LastUpdate:    time.Unix(station.LastUpdate, 0),
		}
	}
	return result
}

func formatAddr(addr uint32) string {
	return strconv.FormatUint(uint64(addr), 16)
}

func protoToAircraftType(t pb.PilotType) uint8 {
	switch t {
	case pb.PilotType_PILOT_TYPE_PARAGLIDER:
		return 1
	case pb.PilotType_PILOT_TYPE_HANGGLIDER:
		return 2
	case pb.PilotType_PILOT_TYPE_GLIDER:
		return 3
	case pb.PilotType_PILOT_TYPE_POWERED:
		return 4
	case pb.PilotType_PILOT_TYPE_HELICOPTER:
		return 5
	case pb.PilotType_PILOT_TYPE_UAV:
		return 6
	case pb.PilotType_PILOT_TYPE_BALLOON:
		return 7
	default:
		return 0
	}
}

func getAircraftTypeName(t uint8) string {
	switch t {
	case 1:
		return "PARAGLIDER"
	case 2:
		return "HANGGLIDER"
	case 3:
		return "GLIDER"
	case 4:
		return "POWERED"
	case 5:
		return "HELICOPTER"
	case 6:
		return "UAV"
	case 7:
		return "BALLOON"
	default:
		return "UNKNOWN"
	}
}