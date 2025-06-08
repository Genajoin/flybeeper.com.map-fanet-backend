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
	// Простая очистка - можно расширить позже
	pilot.DeviceID = ""
	pilot.Name = ""
	p.pilotPool.Put(pilot)
}

// GetThermal получает объект Thermal из пула
func (p *ObjectPools) GetThermal() *models.Thermal {
	return p.thermalPool.Get().(*models.Thermal)
}

// PutThermal возвращает объект Thermal в пул
func (p *ObjectPools) PutThermal(thermal *models.Thermal) {
	thermal.ID = ""
	p.thermalPool.Put(thermal)
}

// GetStation получает объект Station из пула
func (p *ObjectPools) GetStation() *models.Station {
	return p.stationPool.Get().(*models.Station)
}

// PutStation возвращает объект Station в пул
func (p *ObjectPools) PutStation(station *models.Station) {
	station.ID = ""
	p.stationPool.Put(station)
}

// GetPbPilot получает Protobuf Pilot из пула
func (p *ObjectPools) GetPbPilot() *pb.Pilot {
	return p.pbPilotPool.Get().(*pb.Pilot)
}

// PutPbPilot возвращает Protobuf Pilot в пул
func (p *ObjectPools) PutPbPilot(pilot *pb.Pilot) {
	// Минимальная очистка
	pilot.Addr = 0
	pilot.Name = ""
	p.pbPilotPool.Put(pilot)
}

// GetPbThermal получает Protobuf Thermal из пула
func (p *ObjectPools) GetPbThermal() *pb.Thermal {
	return p.pbThermalPool.Get().(*pb.Thermal)
}

// PutPbThermal возвращает Protobuf Thermal в пул
func (p *ObjectPools) PutPbThermal(thermal *pb.Thermal) {
	thermal.Id = 0
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
	p.pbStationPool.Put(station)
}

// GetPbUpdate получает Protobuf Update из пула
func (p *ObjectPools) GetPbUpdate() *pb.Update {
	return p.pbUpdatePool.Get().(*pb.Update)
}

// PutPbUpdate возвращает Protobuf Update в пул
func (p *ObjectPools) PutPbUpdate(update *pb.Update) {
	// Минимальная очистка
	update.Reset()
	p.pbUpdatePool.Put(update)
}

// GetPbUpdateBatch получает Protobuf UpdateBatch из пула
func (p *ObjectPools) GetPbUpdateBatch() *pb.UpdateBatch {
	return p.pbUpdateBatchPool.Get().(*pb.UpdateBatch)
}

// PutPbUpdateBatch возвращает Protobuf UpdateBatch в пул
func (p *ObjectPools) PutPbUpdateBatch(batch *pb.UpdateBatch) {
	batch.Reset()
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

// GetByteSlice получает []byte из пула
func (p *ObjectPools) GetByteSlice() []byte {
	return p.byteSlicePool.Get().([]byte)[:0]
}

// PutByteSlice возвращает []byte в пул
func (p *ObjectPools) PutByteSlice(b []byte) {
	p.byteSlicePool.Put(b)
}