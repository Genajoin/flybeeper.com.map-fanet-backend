package pool

import (
	"sync"

	"github.com/flybeeper/fanet-backend/internal/models"
	"github.com/flybeeper/fanet-backend/pkg/pb"
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
	pbGeoPointPool    sync.Pool
	
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
			return &models.Thermal{}
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
				Position: &pb.GeoPoint{},
			}
		},
	},
	pbThermalPool: sync.Pool{
		New: func() interface{} {
			return &pb.Thermal{
				Position: &pb.GeoPoint{},
			}
		},
	},
	pbStationPool: sync.Pool{
		New: func() interface{} {
			return &pb.Station{
				Position: &pb.GeoPoint{},
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
	pbGeoPointPool: sync.Pool{
		New: func() interface{} {
			return &pb.GeoPoint{}
		},
	},
	
	// Слайсы
	stringMapPool: sync.Pool{
		New: func() interface{} {
			return make(map[string]string)
		},
	},
	byteSlicePool: sync.Pool{
		New: func() interface{} {
			return make([]byte, 0, 256)
		},
	},
}

// GetPilot получает объект Pilot из пула
func (p *ObjectPools) GetPilot() *models.Pilot {
	return p.pilotPool.Get().(*models.Pilot)
}

// PutPilot возвращает объект Pilot в пул
func (p *ObjectPools) PutPilot(pilot *models.Pilot) {
	// Расширенная очистка объекта перед возвратом в пул
	pilot.DeviceID = ""
	pilot.Address = ""
	pilot.Name = ""
	pilot.Type = 0
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
	// Расширенная очистка объекта
	thermal.ID = ""
	thermal.ReportedBy = ""
	thermal.Position.Latitude = 0
	thermal.Position.Longitude = 0
	thermal.Quality = 0
	thermal.ClimbRate = 0
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
	// Расширенная очистка объекта
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

// GetPbPilot получает Protobuf Pilot из пула
func (p *ObjectPools) GetPbPilot() *pb.Pilot {
	return p.pbPilotPool.Get().(*pb.Pilot)
}

// PutPbPilot возвращает Protobuf Pilot в пул
func (p *ObjectPools) PutPbPilot(pilot *pb.Pilot) {
	// Расширенная очистка объекта
	pilot.Addr = 0
	pilot.Name = ""
	pilot.Type = 0
	pilot.Altitude = 0
	pilot.Speed = 0
	pilot.Course = 0
	pilot.Climb = 0
	pilot.LastUpdate = 0
	pilot.TrackOnline = false
	pilot.Battery = 0
	
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
	thermal.Id = 0
	thermal.Addr = 0
	thermal.Altitude = 0
	thermal.Quality = 0
	thermal.Climb = 0
	thermal.WindSpeed = 0
	thermal.WindHeading = 0
	thermal.Timestamp = 0
	
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
	station.Addr = 0
	station.Name = ""
	station.Temperature = 0
	station.WindSpeed = 0
	station.WindHeading = 0
	station.WindGusts = 0
	station.Humidity = 0
	station.Pressure = 0
	station.Battery = 0
	station.LastUpdate = 0
	
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
	update.Action = 0
	update.Data = nil
	update.Sequence = 0
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
	batch.Timestamp = 0
	
	p.pbUpdateBatchPool.Put(batch)
}

// GetStringMap получает map[string]string из пула
func (p *ObjectPools) GetStringMap() map[string]string {
	m := p.stringMapPool.Get().(map[string]string)
	// Очищаем map
	for k := range m {
		delete(m, k)
	}
	return m
}

// PutStringMap возвращает map[string]string в пул
func (p *ObjectPools) PutStringMap(m map[string]string) {
	p.stringMapPool.Put(m)
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

// GetPbGeoPoint получает Protobuf GeoPoint из пула
func (p *ObjectPools) GetPbGeoPoint() *pb.GeoPoint {
	return p.pbGeoPointPool.Get().(*pb.GeoPoint)
}

// PutPbGeoPoint возвращает Protobuf GeoPoint в пул
func (p *ObjectPools) PutPbGeoPoint(point *pb.GeoPoint) {
	point.Latitude = 0
	point.Longitude = 0
	p.pbGeoPointPool.Put(point)
}

// GetByteSlice получает []byte из пула с указанным размером
func (p *ObjectPools) GetByteSlice(size int) []byte {
	if size <= 256 {
		b := p.byteSlicePool.Get().([]byte)
		return b[:0]
	}
	// Для больших размеров создаем новый слайс
	return make([]byte, 0, size)
}

// PutByteSlice возвращает []byte в пул
func (p *ObjectPools) PutByteSlice(b []byte) {
	// Возвращаем только небольшие слайсы
	if cap(b) <= 256 {
		p.byteSlicePool.Put(b[:0])
	}
}