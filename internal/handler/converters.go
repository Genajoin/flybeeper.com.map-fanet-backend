package handler

import (
	"strconv"
	"time"

	"github.com/flybeeper/fanet-backend/internal/filter"
	"github.com/flybeeper/fanet-backend/internal/models"
	"github.com/flybeeper/fanet-backend/pkg/pb"
	"github.com/flybeeper/fanet-backend/pkg/utils"
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
		Type: pb.PilotType(pilot.Type), // Используем pilot.Type вместо pilot.AircraftType
		Position: &pb.GeoPoint{
			Latitude:  pilot.Position.Latitude,
			Longitude: pilot.Position.Longitude,
			Altitude:  pilot.Position.Altitude,
		},
		Speed:       float32(pilot.Speed),
		Climb:       float32(pilot.ClimbRate) / 10, // ClimbRate в 0.1 м/с -> м/с
		Course:      float32(pilot.Heading),
		LastUpdate:  pilot.LastUpdate.Unix(),
		TrackOnline: pilot.TrackOnline,
		Battery:     uint32(pilot.Battery),
	}
}

func convertGroundObjectsToProto(groundObjects []*models.GroundObject) []*pb.GroundObject {
	result := make([]*pb.GroundObject, len(groundObjects))
	for i, obj := range groundObjects {
		result[i] = convertGroundObjectToProto(obj)
	}
	return result
}

func convertGroundObjectToProto(groundObject *models.GroundObject) *pb.GroundObject {
	// Конвертируем DeviceID из hex string в uint32
	addr, _ := strconv.ParseUint(groundObject.DeviceID, 16, 32)

	return &pb.GroundObject{
		Addr: uint32(addr),
		Name: groundObject.Name,
		Type: pb.GroundType(groundObject.Type),
		Position: &pb.GeoPoint{
			Latitude:  groundObject.Position.Latitude,
			Longitude: groundObject.Position.Longitude,
		},
		TrackOnline: groundObject.TrackOnline,
		LastUpdate:  groundObject.LastUpdate.Unix(),
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
			Latitude:  thermal.Position.Latitude,
			Longitude: thermal.Position.Longitude,
			Altitude:  thermal.Position.Altitude,
		},
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
				Altitude:  point.Altitude,
			},
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
		"pilots":         convertPilotsToJSONArray(protoToModelsPilots(response.Pilots)),
		"ground_objects": convertGroundObjectsToJSONArray(protoToModelsGroundObjects(response.GroundObjects)),
		"thermals":       convertThermalsToJSONArray(protoToModelsThermals(response.Thermals)),
		"stations":       convertStationsToJSONArray(protoToModelsStations(response.Stations)),
		"sequence":       response.Sequence,
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
		"type": getAircraftTypeName(uint8(pilot.Type)), // Используем pilot.Type
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

func convertGroundObjectsToJSONArray(groundObjects []*models.GroundObject) []map[string]interface{} {
	result := make([]map[string]interface{}, len(groundObjects))
	for i, obj := range groundObjects {
		result[i] = convertGroundObjectToJSON(obj)
	}
	return result
}

func convertGroundObjectToJSON(groundObject *models.GroundObject) map[string]interface{} {
	addr, _ := strconv.ParseUint(groundObject.DeviceID, 16, 32)
	
	return map[string]interface{}{
		"addr": addr,
		"name": groundObject.Name,
		"type": getGroundTypeName(uint8(groundObject.Type)),
		"position": map[string]interface{}{
			"latitude":  groundObject.Position.Latitude,
			"longitude": groundObject.Position.Longitude,
		},
		"last_update":  groundObject.LastUpdate.Unix(),
		"track_online": groundObject.TrackOnline,
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
			"latitude":  thermal.Position.Latitude,
			"longitude": thermal.Position.Longitude,
			"altitude":  thermal.Position.Altitude,
		},
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
			"altitude":  point.Position.Altitude,
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

func convertTrackToGeoJSON(track *pb.Track) map[string]interface{} {
	// Создаем координаты LineString в формате [longitude, latitude]
	coordinates := make([][]float64, len(track.Points))
	for i, point := range track.Points {
		coordinates[i] = []float64{
			point.Position.Longitude,
			point.Position.Latitude,
		}
	}

	// Генерируем цвет на основе адреса
	color := generateColorFromAddr(track.Addr)

	// GeoJSON FeatureCollection - точно как в референсе
	return map[string]interface{}{
		"type": "FeatureCollection",
		"features": []map[string]interface{}{
			{
				"type": "Feature",
				"properties": map[string]interface{}{
					"addr":  track.Addr,
					"color": color,
				},
				"geometry": map[string]interface{}{
					"type":        "LineString",
					"coordinates": coordinates,
				},
			},
		},
	}
}

// generateColorFromAddr генерирует цвет на основе адреса устройства
func generateColorFromAddr(addr uint32) string {
	// Простая цветовая схема на основе адреса
	colors := []string{
		"#1bb12e", "#ff6b35", "#f7931e", "#c149ad", "#00b4d8",
		"#0077b6", "#90e0ef", "#e63946", "#f77f00", "#fcbf49",
	}
	return colors[addr%uint32(len(colors))]
}

// generateSegmentColor генерирует цвет для сегмента на основе средней скорости
func generateSegmentColor(avgSpeed float64) string {
	return filter.GenerateColorBySpeed(avgSpeed)
}

// Вспомогательные функции

func protoToModelsPilots(pilots []*pb.Pilot) []*models.Pilot {
	result := make([]*models.Pilot, len(pilots))
	for i, pilot := range pilots {
		result[i] = &models.Pilot{
			DeviceID:     formatAddr(pilot.Addr),
			Name:         pilot.Name,
			Type:         models.PilotType(pilot.Type),
			Position: &models.GeoPoint{
				Latitude:  pilot.Position.Latitude,
				Longitude: pilot.Position.Longitude,
				Altitude:  pilot.Position.Altitude,
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
			Position: &models.GeoPoint{
				Latitude:  thermal.Position.Latitude,
				Longitude: thermal.Position.Longitude,
				Altitude: thermal.Position.Altitude,
			},
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


func getAircraftTypeName(t uint8) string {
	// FANET спецификация: 0=Unknown, 1=Paraglider, 2=Hangglider, 3=Balloon, 4=Glider, 5=Powered, 6=Helicopter, 7=UAV
	switch t {
	case 0:
		return "UNKNOWN"
	case 1:
		return "PARAGLIDER"
	case 2:
		return "HANGGLIDER"
	case 3:
		return "BALLOON"
	case 4:
		return "GLIDER"
	case 5:
		return "POWERED"
	case 6:
		return "HELICOPTER"
	case 7:
		return "UAV"
	default:
		return "UNKNOWN"
	}
}

// ==================== Функции для работы с фильтрами ====================

// convertGeoPointsToTrackData конвертирует MySQL данные в формат для фильтрации
func convertGeoPointsToTrackData(points []models.GeoPoint, deviceID string, aircraftType models.PilotType) *filter.TrackData {
	trackPoints := make([]filter.TrackPoint, len(points))
	
	// Для простоты создаем временные метки с интервалом 1 минута
	baseTime := time.Now().Add(-time.Duration(len(points)) * time.Minute)
	
	for i, point := range points {
		trackPoints[i] = filter.TrackPoint{
			Position:  point,
			Timestamp: baseTime.Add(time.Duration(i) * time.Minute),
		}
	}
	
	return &filter.TrackData{
		DeviceID:     deviceID,
		AircraftType: aircraftType,
		Points:       trackPoints,
	}
}

// convertTrackGeoPointsToTrackData конвертирует MySQL данные с временными метками в формат для фильтрации
func convertTrackGeoPointsToTrackData(points []models.TrackGeoPoint, deviceID string, aircraftType models.PilotType) *filter.TrackData {
	trackPoints := make([]filter.TrackPoint, len(points))
	
	for i, point := range points {
		trackPoints[i] = filter.TrackPoint{
			Position:  point.GeoPoint,
			Timestamp: point.Timestamp,
		}
	}
	
	return &filter.TrackData{
		DeviceID:     deviceID,
		AircraftType: aircraftType,
		Points:       trackPoints,
	}
}

// applyTrackFilters применяет фильтры к треку
func applyTrackFilters(points []models.GeoPoint, deviceID string, aircraftType models.PilotType, logger *utils.Logger) (*filter.FilterResult, error) {
	// Создаем конфигурацию фильтров
	config := filter.DefaultFilterConfig()
	
	// Создаем цепочку фильтров
	filterChain := filter.NewFilterChain(config, logger)
	
	// Конвертируем данные для фильтрации
	trackData := convertGeoPointsToTrackData(points, deviceID, aircraftType)
	
	// Применяем фильтры
	return filterChain.Filter(trackData)
}

// applyTrackFiltersWithTimestamps применяет фильтры к треку с временными метками
func applyTrackFiltersWithTimestamps(points []models.TrackGeoPoint, deviceID string, aircraftType models.PilotType, filterLevel int, logger *utils.Logger) (*filter.FilterResult, error) {
	// Создаем конфигурацию фильтров
	config := filter.DefaultFilterConfig()
	
	// Конвертируем данные для фильтрации
	trackData := convertTrackGeoPointsToTrackData(points, deviceID, aircraftType)
	
	// Если уровень 0 - возвращаем без фильтрации
	if filterLevel == 0 {
		return &filter.FilterResult{
			OriginalCount: len(points),
			FilteredCount: 0,
			Points:        trackData.Points,
			Statistics:    filter.FilterStats{},
		}, nil
	}
	
	// Выбираем цепочку фильтров в зависимости от уровня
	var filterChain filter.TrackFilter
	
	switch filterLevel {
	case 1:
		filterChain = filter.NewLevel1FilterChain(config, logger)
	case 2:
		filterChain = filter.NewLevel2FilterChain(config, logger)
	case 3:
		filterChain = filter.NewLevel3FilterChain(config, logger)
	default:
		// Для неизвестных уровней используем уровень 1
		filterChain = filter.NewLevel1FilterChain(config, logger)
	}
	
	// Применяем фильтры
	return filterChain.Filter(trackData)
}

// convertFilteredTrackToGeoPoints конвертирует отфильтрованный трек обратно в GeoPoint
func convertFilteredTrackToGeoPoints(filterResult *filter.FilterResult) []models.GeoPoint {
	points := make([]models.GeoPoint, 0, len(filterResult.Points))
	
	for _, trackPoint := range filterResult.Points {
		if !trackPoint.Filtered {
			points = append(points, trackPoint.Position)
		}
	}
	
	return points
}

// convertTrackToGeoJSONWithFilter создает GeoJSON с информацией о фильтрации
func convertTrackToGeoJSONWithFilter(track *pb.Track, filterResult *filter.FilterResult) map[string]interface{} {
	// Проверяем, есть ли сегменты (либо в массиве Segments, либо SegmentCount > 1)
	if len(filterResult.Statistics.Segments) > 1 || filterResult.Statistics.SegmentCount > 1 {
		return convertTrackToGeoJSONWithSegments(track, filterResult)
	}
	
	// Базовый GeoJSON для одного сегмента
	geoJSON := convertTrackToGeoJSON(track)
	
	// Добавляем статистику фильтрации в properties
	if features, ok := geoJSON["features"].([]map[string]interface{}); ok && len(features) > 0 {
		properties := features[0]["properties"].(map[string]interface{})
		
		// Добавляем информацию о фильтрации
		properties["filter_applied"] = true
		properties["original_points"] = filterResult.OriginalCount
		properties["filtered_points"] = filterResult.FilteredCount
		properties["final_points"] = len(filterResult.Points)
		
		// Статистика фильтрации
		if filterResult.Statistics.SpeedViolations > 0 {
			properties["speed_violations"] = filterResult.Statistics.SpeedViolations
		}
		if filterResult.Statistics.Duplicates > 0 {
			properties["duplicates_removed"] = filterResult.Statistics.Duplicates
		}
		if filterResult.Statistics.Outliers > 0 {
			properties["outliers_removed"] = filterResult.Statistics.Outliers
		}
		if filterResult.Statistics.Teleportations > 0 {
			properties["teleportations_removed"] = filterResult.Statistics.Teleportations
		}
		if filterResult.Statistics.MaxSpeedDetected > 0 {
			properties["max_speed_detected"] = filterResult.Statistics.MaxSpeedDetected
		}
		if filterResult.Statistics.AvgSpeed > 0 {
			properties["avg_speed"] = filterResult.Statistics.AvgSpeed
		}
		if filterResult.Statistics.MaxDistanceJump > 0 {
			properties["max_distance_jump"] = filterResult.Statistics.MaxDistanceJump
		}
		if filterResult.Statistics.SegmentCount > 0 {
			properties["segment_count"] = filterResult.Statistics.SegmentCount
		}
		if filterResult.Statistics.SegmentBreaks > 0 {
			properties["segment_breaks"] = filterResult.Statistics.SegmentBreaks
		}
	}
	
	return geoJSON
}

// convertTrackToGeoJSONWithSegments создает GeoJSON с MultiLineString для сегментированного трека
func convertTrackToGeoJSONWithSegments(track *pb.Track, filterResult *filter.FilterResult) map[string]interface{} {
	// Если нет сегментов в статистике, создаем их из точек
	segments := filterResult.Statistics.Segments
	if len(segments) == 0 {
		// Группируем точки по SegmentID
		segmentMap := make(map[int][]int)
		for i, filterPoint := range filterResult.Points {
			if !filterPoint.Filtered {
				segmentID := filterPoint.SegmentID
				if segmentID == 0 {
					segmentID = 1 // Default segment
				}
				segmentMap[segmentID] = append(segmentMap[segmentID], i)
			}
		}
		
		// Создаем базовые SegmentInfo для каждого найденного сегмента
		for segmentID, indices := range segmentMap {
			if len(indices) > 1 {
				// Вычисляем среднюю скорость для сегмента
				totalSpeed := 0.0
				speedCount := 0
				for _, idx := range indices {
					if idx < len(filterResult.Points) && !filterResult.Points[idx].Filtered {
						if filterResult.Points[idx].Speed > 0 {
							totalSpeed += filterResult.Points[idx].Speed
							speedCount++
						}
					}
				}
				avgSpeed := 0.0
				if speedCount > 0 {
					avgSpeed = totalSpeed / float64(speedCount)
				}
				
				segments = append(segments, filter.SegmentInfo{
					ID:         segmentID,
					Color:      generateSegmentColor(avgSpeed),
					PointCount: len(indices),
					AvgSpeed:   avgSpeed,
				})
			}
		}
	}
	
	// Создаем features для каждого сегмента
	features := make([]map[string]interface{}, 0, len(segments))
	
	// Используем информацию о сегментах
	for _, segmentInfo := range segments {
		// Собираем координаты для этого сегмента
		coordinates := make([][]float64, 0)
		
		// Проходим по точкам и собираем координаты для сегмента
		for _, filterPoint := range filterResult.Points {
			// Пропускаем отфильтрованные точки
			if filterPoint.Filtered {
				continue
			}
			
			// Проверяем, что точка принадлежит текущему сегменту
			if filterPoint.SegmentID == segmentInfo.ID || (filterPoint.SegmentID == 0 && segmentInfo.ID == 1) {
				// Используем позицию из filterPoint (она уже правильная)
				coordinates = append(coordinates, []float64{
					filterPoint.Position.Longitude,
					filterPoint.Position.Latitude,
				})
			}
		}
		
		// Создаем feature для сегмента
		if len(coordinates) > 1 { // Минимум 2 точки для LineString
			properties := map[string]interface{}{
				"addr":       track.Addr,
				"color":      generateSegmentColor(segmentInfo.AvgSpeed),
				"segment_id": segmentInfo.ID,
			}
			
			// Добавляем дополнительные свойства если они есть
			if segmentInfo.StartTime.Unix() > 0 {
				properties["start_time"] = segmentInfo.StartTime.Unix()
			}
			if segmentInfo.EndTime.Unix() > 0 {
				properties["end_time"] = segmentInfo.EndTime.Unix()
			}
			if segmentInfo.Duration > 0 {
				properties["duration_minutes"] = segmentInfo.Duration
			}
			if segmentInfo.Distance > 0 {
				properties["distance_km"] = segmentInfo.Distance
			}
			if segmentInfo.AvgSpeed > 0 {
				properties["avg_speed_kmh"] = segmentInfo.AvgSpeed
			}
			if segmentInfo.MaxSpeed > 0 {
				properties["max_speed_kmh"] = segmentInfo.MaxSpeed
			}
			properties["point_count"] = len(coordinates)
			
			feature := map[string]interface{}{
				"type":       "Feature",
				"properties": properties,
				"geometry": map[string]interface{}{
					"type":        "LineString",
					"coordinates": coordinates,
				},
			}
			
			features = append(features, feature)
		}
	}
	
	// GeoJSON FeatureCollection с множественными сегментами
	return map[string]interface{}{
		"type": "FeatureCollection",
		"properties": map[string]interface{}{
			"addr":              track.Addr,
			"filter_applied":    true,
			"original_points":   filterResult.OriginalCount,
			"filtered_points":   filterResult.FilteredCount,
			"final_points":      len(filterResult.Points) - filterResult.FilteredCount,
			"segment_count":     filterResult.Statistics.SegmentCount,
			"segment_breaks":    filterResult.Statistics.SegmentBreaks,
			"speed_violations":     filterResult.Statistics.SpeedViolations,
			"duplicates_removed":   filterResult.Statistics.Duplicates,
			"outliers_removed":     filterResult.Statistics.Outliers,
			"teleportations_removed": filterResult.Statistics.Teleportations,
			"max_speed_detected": filterResult.Statistics.MaxSpeedDetected,
			"avg_speed":         filterResult.Statistics.AvgSpeed,
			"max_distance_jump": filterResult.Statistics.MaxDistanceJump,
		},
		"features": features,
	}
}

// convertTrackToJSONWithFilter создает JSON с информацией о фильтрации  
func convertTrackToJSONWithFilter(track *pb.Track, filterResult *filter.FilterResult) map[string]interface{} {
	// Базовый JSON
	result := convertTrackToJSON(track)
	
	// Добавляем информацию о фильтрации
	trackData := result["track"].(map[string]interface{})
	trackData["filter_applied"] = true
	trackData["original_points"] = filterResult.OriginalCount
	trackData["filtered_points"] = filterResult.FilteredCount
	trackData["final_points"] = len(filterResult.Points)
	
	// Статистика фильтрации
	trackData["filter_statistics"] = map[string]interface{}{
		"speed_violations":     filterResult.Statistics.SpeedViolations,
		"duplicates_removed":   filterResult.Statistics.Duplicates,
		"outliers_removed":     filterResult.Statistics.Outliers,
		"max_speed_detected":   filterResult.Statistics.MaxSpeedDetected,
		"avg_speed":            filterResult.Statistics.AvgSpeed,
		"max_distance_jump":    filterResult.Statistics.MaxDistanceJump,
	}
	
	return result
}

// getAircraftTypeFromAircraft конвертирует uint8 в PilotType
func getAircraftTypeFromAircraft(aircraftType uint8) models.PilotType {
	// FANET значения напрямую соответствуют PilotType enum
	if aircraftType <= 7 {
		return models.PilotType(aircraftType)
	}
	return models.PilotTypeUnknown
}

func protoToModelsGroundObjects(groundObjects []*pb.GroundObject) []*models.GroundObject {
	result := make([]*models.GroundObject, len(groundObjects))
	for i, obj := range groundObjects {
		result[i] = &models.GroundObject{
			DeviceID:     formatAddr(obj.Addr),
			Name:         obj.Name,
			Type:         models.GroundType(obj.Type),
			Position: &models.GeoPoint{
				Latitude:  obj.Position.Latitude,
				Longitude: obj.Position.Longitude,
			},
			TrackOnline: obj.TrackOnline,
			LastUpdate:  time.Unix(obj.LastUpdate, 0),
		}
	}
	return result
}

func getGroundTypeName(t uint8) string {
	// FANET спецификация для наземных объектов
	switch t {
	case 0:
		return "OTHER"
	case 1:
		return "WALKING"
	case 2:
		return "VEHICLE"
	case 3:
		return "BIKE"
	case 4:
		return "BOOT"
	case 8:
		return "NEED_RIDE"
	case 9:
		return "LANDED_WELL"
	case 12:
		return "NEED_TECHNICAL_SUPPORT"
	case 13:
		return "NEED_MEDICAL_HELP"
	case 14:
		return "DISTRESS_CALL"
	case 15:
		return "DISTRESS_CALL_AUTO"
	default:
		return "OTHER"
	}
}