package brooders

import (
	"time"

	"gorm.io/gorm"
)

type Repository interface {
	FindAll() ([]Brooder, error)
	FindByID(id uint) (*Brooder, error)
	Create(brooder *Brooder) error
	UpdateSensorData(id uint, data SensorUpdate) error
	UpdateActuators(id uint, data ActuatorUpdate) error
	BatchInsertHistoricalSensorData(brooderID uint, readings []HistoricalSensorData) error
	GetHistoricalSensorData(brooderID uint, since time.Time) ([]HistoricalSensorData, error)
	FindByUUID(uuid string) (*Brooder, error)
}

type GormRepository struct {
	db *gorm.DB
}

func NewGormRepository(db *gorm.DB) Repository {
	// Auto migrate
	err := db.AutoMigrate(&Brooder{}, &HistoricalSensorData{}, &FCRInput{}, &FCRResult{})
	if err != nil {
		return nil
	}
	return &GormRepository{db: db}
}

func (r *GormRepository) FindAll() ([]Brooder, error) {
	var brooders []Brooder
	result := r.db.Find(&brooders)
	return brooders, result.Error
}

func (r *GormRepository) FindByID(id uint) (*Brooder, error) {
	var brooder Brooder
	result := r.db.First(&brooder, id)
	return &brooder, result.Error
}

func (r *GormRepository) Create(brooder *Brooder) error {
	return r.db.Create(brooder).Error
}

func (r *GormRepository) UpdateSensorData(id uint, data SensorUpdate) error {
	return r.db.Model(&Brooder{}).Where("id = ?", id).Updates(map[string]interface{}{
		"temperature":  data.Temperature,
		"humidity":     data.Humidity,
		"feed_level":   data.FeedLevel,
		"water_level":  data.WaterLevel,
		"last_updated": time.Now(),
	}).Error
}

func (r *GormRepository) UpdateActuators(id uint, data ActuatorUpdate) error {
	return r.db.Model(&Brooder{}).Where("id = ?", id).Updates(map[string]interface{}{
		"fan_on":         data.FanOn,
		"dispense_feed":  data.DispenseFeed,
		"dispense_water": data.DispenseWater,
		"heater_on":      data.HeaterOn,
		"last_updated":   time.Now(),
	}).Error
}
func (r *GormRepository) BatchInsertHistoricalSensorData(brooderID uint, readings []HistoricalSensorData) error {
	for i := range readings {
		readings[i].BrooderID = brooderID
	}
	return r.db.CreateInBatches(readings, 100).Error
}
func (r *GormRepository) FindByUUID(uuid string) (*Brooder, error) {
	var brooder Brooder

	result := r.db.Where("uuid = ?", uuid).First(&brooder)

	return &brooder, result.Error
}
func (r *GormRepository) GetHistoricalSensorData(
	brooderID uint,
	since time.Time,
) ([]HistoricalSensorData, error) {
	var rows []HistoricalSensorData
	err := r.db.
		Where("brooder_id = ? AND recorded_at >= ?", brooderID, since).
		Order("recorded_at ASC").
		Find(&rows).Error
	return rows, err
}
