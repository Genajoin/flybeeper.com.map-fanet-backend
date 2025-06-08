package pool

import (
	"sync"

	"flybeeper.com/fanet-api/internal/models"
	"flybeeper.com/fanet-api/pkg/pb"
)

// ObjectPools содержит все пулы объектов для переиспользования
type ObjectPools struct {
	// Модели
	pilotPool    sync.Pool
	thermalPool  sync.Pool
	stationPool  sync.Pool
	geoPointPool sync.Pool
	
	// Protobuf сообщения
	pbPilotPool       sync.Pool
	pbThermalPool     sync.Pool
	pbStationPool     sync.Pool
	pbUpdatePool      sync.Pool
	pbUpdateBatchPool sync.Pool
	pbPositionPool    sync.Pool
	
	// Слайсы и мапы
	stringMapPool sync.Pool
	byteSlicePool sync.Pool
}

// Global пулы объектов
var Global = &ObjectPools{
	// Инициализация пулов для моделей
	pilotPool: sync.Pool{
		New: func() interface{} {
			return &models.Pilot{
				Position: &models.GeoPoint{},
			}
		},
	},
	thermalPool: sync.Pool{
		New: func() interface{} {
			return &models.Thermal{
				Position: &models.GeoPoint{},
			}
		},
	},
	stationPool: sync.Pool{
		New: func() interface{} {
			return &models.Station{
				Position: &models.GeoPoint{},
			}
		},
	},
	geoPointPool: sync.Pool{
		New: func() interface{} {
			return &models.GeoPoint{}
		},
	},
	
	// Инициализация пулов для Protobuf
	pbPilotPool: sync.Pool{
		New: func() interface{} {
			return &pb.Pilot{
				Position: &pb.Position{},
			}
		},
	},
	pbThermalPool: sync.Pool{
		New: func() interface{} {
			return &pb.Thermal{
				Position: &pb.Position{},
			}
		},
	},
	pbStationPool: sync.Pool{
		New: func() interface{} {
			return &pb.Station{
				Position: &pb.Position{},
			}
		},
	},
	pbUpdatePool: sync.Pool{
		New: func() interface{} {
			return &pb.Update{}
		},
	},
	pbUpdateBatchPool: sync.Pool{
		New: func() interface{} {
			return &pb.UpdateBatch{
				Updates: make([]*pb.Update, 0, 10),
			}
		},
	},
	pbPositionPool: sync.Pool{
		New: func() interface{} {
			return &pb.Position{}
		},
	},
	
	// Инициализация пулов для контейнеров
	stringMapPool: sync.Pool{
		New: func() interface{} {
			return make(map[string]string, 16)
		},
	},
	byteSlicePool: sync.Pool{
		New: func() interface{} {
			return make([]byte, 0, 1024)
		},
	},
}

// GetPilot получает объект Pilot из пула
func (p *ObjectPools) GetPilot() *models.Pilot {
	return p.pilotPool.Get().(*models.Pilot)
}

// PutPilot возвращает объект Pilot в пул
func (p *ObjectPools) PutPilot(pilot *models.Pilot) {
	// Очищаем объект перед возвратом в пул
	pilot.DeviceID = ""
	pilot.Address = ""
	pilot.Name = ""
	pilot.Type = 0
	pilot.AircraftType = 0
	pilot.Altitude = 0
	pilot.Speed = 0
	pilot.ClimbRate = 0
	pilot.Heading = 0
	pilot.TrackOnline = false
	pilot.Battery = 0
	pilot.LastUpdate = pilot.LastUpdate.Truncate(pilot.LastUpdate.Sub(pilot.LastUpdate))
	pilot.LastSeen = pilot.LastSeen.Truncate(pilot.LastSeen.Sub(pilot.LastSeen))
	
	// Сохраняем Position указатель, только очищаем значения
	if pilot.Position != nil {
		pilot.Position.Latitude = 0
		pilot.Position.Longitude = 0
		pilot.Position.Altitude = 0
	}
	
	p.pilotPool.Put(pilot)
}

// GetThermal получает объект Thermal из пула
func (p *ObjectPools) GetThermal() *models.Thermal {
	return p.thermalPool.Get().(*models.Thermal)
}

// PutThermal возвращает объект Thermal в пул
func (p *ObjectPools) PutThermal(thermal *models.Thermal) {
	// Очищаем объект
	thermal.ID = ""
	thermal.ReportedBy = ""
	thermal.Center.Latitude = 0
	thermal.Center.Longitude = 0
	thermal.Center.Altitude = 0
	thermal.Altitude = 0
	thermal.Quality = 0
	thermal.ClimbRate = 0
	thermal.PilotCount = 0
	thermal.WindSpeed = 0
	thermal.WindDirection = 0
	thermal.Timestamp = thermal.Timestamp.Truncate(thermal.Timestamp.Sub(thermal.Timestamp))
	thermal.LastSeen = thermal.LastSeen.Truncate(thermal.LastSeen.Sub(thermal.LastSeen))
	
	if thermal.Position != nil {
		thermal.Position.Latitude = 0
		thermal.Position.Longitude = 0
		thermal.Position.Altitude = 0
	}
	
	p.thermalPool.Put(thermal)
}

// GetStation получает объект Station из пула
func (p *ObjectPools) GetStation() *models.Station {
	return p.stationPool.Get().(*models.Station)
}

// PutStation возвращает объект Station в пул
func (p *ObjectPools) PutStation(station *models.Station) {
	// Очищаем объект
	station.ID = ""
	station.ChipID = ""
	station.Name = ""
	station.Temperature = 0
	station.WindSpeed = 0
	station.WindDirection = 0
	station.WindGusts = 0
	station.Humidity = 0
	station.Pressure = 0
	station.Battery = 0
	station.LastUpdate = station.LastUpdate.Truncate(station.LastUpdate.Sub(station.LastUpdate))
	station.LastSeen = station.LastSeen.Truncate(station.LastSeen.Sub(station.LastSeen))
	
	if station.Position != nil {
		station.Position.Latitude = 0
		station.Position.Longitude = 0
		station.Position.Altitude = 0
	}
	
	p.stationPool.Put(station)
}

// GetGeoPoint получает объект GeoPoint из пула
func (p *ObjectPools) GetGeoPoint() *models.GeoPoint {
	return p.geoPointPool.Get().(*models.GeoPoint)
}

// PutGeoPoint возвращает объект GeoPoint в пул
func (p *ObjectPools) PutGeoPoint(point *models.GeoPoint) {
	point.Latitude = 0
	point.Longitude = 0
	point.Altitude = 0
	p.geoPointPool.Put(point)
}

// GetPbPilot получает Protobuf Pilot из пула
func (p *ObjectPools) GetPbPilot() *pb.Pilot {
	return p.pbPilotPool.Get().(*pb.Pilot)
}

// PutPbPilot возвращает Protobuf Pilot в пул
func (p *ObjectPools) PutPbPilot(pilot *pb.Pilot) {
	// Очищаем объект
	pilot.Address = ""
	pilot.Name = ""
	pilot.Type = 0
	pilot.Altitude = 0
	pilot.Speed = 0
	pilot.Heading = 0
	pilot.ClimbRate = 0
	pilot.LastSeen = 0
	
	if pilot.Position != nil {
		pilot.Position.Latitude = 0
		pilot.Position.Longitude = 0
	}
	
	p.pbPilotPool.Put(pilot)
}

// GetPbThermal получает Protobuf Thermal из пула
func (p *ObjectPools) GetPbThermal() *pb.Thermal {
	return p.pbThermalPool.Get().(*pb.Thermal)
}

// PutPbThermal возвращает Protobuf Thermal в пул
func (p *ObjectPools) PutPbThermal(thermal *pb.Thermal) {
	thermal.Id = ""
	thermal.ReportedBy = ""
	thermal.Altitude = 0
	thermal.Quality = 0
	thermal.ClimbRate = 0
	thermal.PilotCount = 0
	thermal.LastSeen = 0
	
	if thermal.Position != nil {
		thermal.Position.Latitude = 0
		thermal.Position.Longitude = 0
	}
	
	p.pbThermalPool.Put(thermal)
}

// GetPbStation получает Protobuf Station из пула
func (p *ObjectPools) GetPbStation() *pb.Station {
	return p.pbStationPool.Get().(*pb.Station)
}

// PutPbStation возвращает Protobuf Station в пул
func (p *ObjectPools) PutPbStation(station *pb.Station) {
	station.ChipId = ""
	station.Name = ""
	station.Temperature = 0
	station.WindSpeed = 0
	station.WindDirection = 0
	station.WindGusts = 0
	station.Humidity = 0
	station.Pressure = 0
	station.LastSeen = 0
	
	if station.Position != nil {
		station.Position.Latitude = 0
		station.Position.Longitude = 0
	}
	
	p.pbStationPool.Put(station)
}

// GetPbUpdate получает Protobuf Update из пула
func (p *ObjectPools) GetPbUpdate() *pb.Update {
	return p.pbUpdatePool.Get().(*pb.Update)
}

// PutPbUpdate возвращает Protobuf Update в пул
func (p *ObjectPools) PutPbUpdate(update *pb.Update) {
	update.Type = 0
	update.Timestamp = 0
	update.Sequence = 0
	update.Pilot = nil
	update.Thermal = nil
	update.Station = nil
	
	p.pbUpdatePool.Put(update)
}

// GetPbUpdateBatch получает Protobuf UpdateBatch из пула
func (p *ObjectPools) GetPbUpdateBatch() *pb.UpdateBatch {
	batch := p.pbUpdateBatchPool.Get().(*pb.UpdateBatch)
	// Очищаем слайс но сохраняем capacity
	batch.Updates = batch.Updates[:0]
	return batch
}

// PutPbUpdateBatch возвращает Protobuf UpdateBatch в пул
func (p *ObjectPools) PutPbUpdateBatch(batch *pb.UpdateBatch) {
	// Возвращаем updates в пул
	for _, update := range batch.Updates {
		p.PutPbUpdate(update)
	}
	
	// Очищаем слайс но сохраняем capacity
	batch.Updates = batch.Updates[:0]
	batch.Sequence = 0
	
	p.pbUpdateBatchPool.Put(batch)
}

// GetStringMap получает map[string]string из пула
func (p *ObjectPools) GetStringMap() map[string]string {
	m := p.stringMapPool.Get().(map[string]string)
	// Очищаем мапу
	for k := range m {
		delete(m, k)
	}
	return m
}

// PutStringMap возвращает map[string]string в пул
func (p *ObjectPools) PutStringMap(m map[string]string) {
	// Очищаем мапу если не слишком большая
	if len(m) > 100 {
		return // Не возвращаем слишком большие мапы
	}
	
	for k := range m {
		delete(m, k)
	}
	
	p.stringMapPool.Put(m)
}

// GetByteSlice получает []byte из пула
func (p *ObjectPools) GetByteSlice(size int) []byte {
	if size <= 1024 {
		b := p.byteSlicePool.Get().([]byte)
		return b[:0]
	}
	// Для больших размеров создаем новый слайс
	return make([]byte, 0, size)
}

// PutByteSlice возвращает []byte в пул
func (p *ObjectPools) PutByteSlice(b []byte) {
	// Возвращаем только небольшие слайсы
	if cap(b) <= 1024 {
		p.byteSlicePool.Put(b[:0])
	}
}