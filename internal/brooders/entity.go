package brooders

import (
	"time"

	"gorm.io/gorm"
)

type Brooder struct {
	gorm.Model
	ID             uint   `json:"id" gorm:"primaryKey"`
	UUID           string `json:"uuid" gorm:"uniqueIndex"`
	Name           string `json:"name"`
	Location       string `json:"location"`
	FlockSize      uint   `json:"flock_size"`
	MortalityCount uint   `json:"mortality_count"`

	// Ownership
	Farm string `json:"farm"`

	// Environmental Data
	Temperature float64 `json:"temperature"` // °C
	Humidity    float64 `json:"humidity"`    // %

	// Feed System
	FeedLevel         float64 `json:"feed_level"`
	WaterLevel        float64 `json:"water_level"`
	FeedCapacity      float64 `json:"feed_capacity"`
	TargetTemperature float64 `json:"target_temperature"`
	TargetHumidity    float64 `json:"target_humidity"`

	// Actuators / Controls
	FanOn         bool `json:"fan_on"`
	DispenseFeed  bool `json:"dispense_feed"`
	DispenseWater bool `json:"dispense_water"`
	HeaterOn      bool `json:"heater_on"`
	AutoMode      bool `json:"auto_mode"`

	// Alerts
	AlertActive  bool   `json:"alert_active" gorm:"default:true"`
	AlertMessage string `json:"alert_message" gorm:"default:'System initialized'"`

	// Metadata
	LastUpdated time.Time `json:"last_updated"`
}

// SensorUpdate — payload sent by ESP32
type SensorUpdate struct {
	Temperature float64 `json:"temperature"`
	Humidity    float64 `json:"humidity"`
	FeedLevel   float64 `json:"feed_level"`
	WaterLevel  float64 `json:"water_level"`
}

// ActuatorUpdate — payload to control devices
type ActuatorUpdate struct {
	FanOn         bool `json:"fan_on"`
	DispenseFeed  bool `json:"dispense_feed"`
	DispenseWater bool `json:"dispense_water"`
	HeaterOn      bool `json:"heater_on"`
}
type HistoricalSensorData struct {
	gorm.Model
	BrooderID   uint      `json:"brooder_id"`
	Temperature float64   `json:"temperature"`
	Humidity    float64   `json:"humidity"`
	FeedLevel   float64   `json:"feed_level"`
	WaterLevel  float64   `json:"water_level"`
	RecordedAt  time.Time `json:"recorded_at"` // from ESP32 ts field
}
