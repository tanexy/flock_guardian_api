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
}

type GormRepository struct {
	db *gorm.DB
}

func NewGormRepository(db *gorm.DB) Repository {
	// Auto migrate
	db.AutoMigrate(&Brooder{})
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
