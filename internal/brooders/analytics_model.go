package brooders

import (
	"time"

	"gorm.io/gorm"
)

type WeightLog struct {
	gorm.Model
	BrooderID       uint      `json:"brooder_id"        gorm:"not null;index"`
	WeekStart       time.Time `json:"week_start"        gorm:"not null"` // Monday 00:00 UTC of the logged week
	CurrentWeightG  float64   `json:"current_weight_g"  gorm:"not null"` // avg bird weight this week (g)
	PreviousWeightG float64   `json:"previous_weight_g" gorm:"not null"` // avg bird weight last week (g)
	TotalFeedKg     float64   `json:"total_feed_kg"     gorm:"not null"` // total feed consumed this week (kg)
	FlockSize       int       `json:"flock_size"        gorm:"not null"` // live birds at time of log
	LoggedAt        time.Time `json:"logged_at"`                         // when the farmer submitted the entry
}

// =============================================================================
// COMPUTED / RESPONSE TYPES  (never persisted )
// =============================================================================

type AnalyticsResponse struct {
	BrooderID uint   `json:"brooder_id"`
	Period    string `json:"period"`

	Summary        AnalyticsSummary `json:"summary"`
	DailyBreakdown []DailyAnalytics `json:"daily_breakdown"`
	AlertSummary   AlertSummary     `json:"alert_summary"`

	// Included when a WeightLog entry exists for the requested period.
	// Nil when the farmer has not yet logged a weigh-in.
	WeightMetrics *WeightMetrics `json:"weight_metrics,omitempty"`
}

// AnalyticsSummary holds period-level aggregated KPIs (sensor data only).
type AnalyticsSummary struct {
	AvgTemperature float64 `json:"avg_temperature"`
	MinTemperature float64 `json:"min_temperature"`
	MaxTemperature float64 `json:"max_temperature"`
	TempVariance   float64 `json:"temp_variance"` // std-dev °C

	AvgHumidity float64 `json:"avg_humidity"`
	MinHumidity float64 `json:"min_humidity"`
	MaxHumidity float64 `json:"max_humidity"`

	AvgFeedLevel  float64 `json:"avg_feed_level"`
	AvgWaterLevel float64 `json:"avg_water_level"`

	DaysInTempTarget     int `json:"days_in_temp_target"`
	DaysInHumidityTarget int `json:"days_in_humidity_target"`
	TotalReadings        int `json:"total_readings"`
}

// DailyAnalytics is one calendar day's rolled-up sensor values.
// Ordered oldest → newest; maps directly to chart x-axis labels.
type DailyAnalytics struct {
	Date     string `json:"date"`      // "2025-05-01"
	DayLabel string `json:"day_label"` // "Mon", "Tue", …

	AvgTemperature float64 `json:"avg_temperature"`
	MinTemperature float64 `json:"min_temperature"`
	MaxTemperature float64 `json:"max_temperature"`

	AvgHumidity float64 `json:"avg_humidity"`

	FeedLevel  float64 `json:"feed_level"`  // end-of-day snapshot
	WaterLevel float64 `json:"water_level"` // end-of-day snapshot

	TempInTarget bool `json:"temp_in_target"` // drives bar colour on chart
	ReadingCount int  `json:"reading_count"`
}

// AlertSummary is a lightweight incident breakdown for the period.
type AlertSummary struct {
	TotalAlerts    int `json:"total_alerts"`
	TempAlerts     int `json:"temp_alerts"`
	HumidityAlerts int `json:"humidity_alerts"`
	FeedAlerts     int `json:"feed_alerts"`
	WaterAlerts    int `json:"water_alerts"`
}

// WeightMetrics is derived from the most recent WeightLog in the period.
// Computed at query time — never stored.
type WeightMetrics struct {
	// Source data echoed back so the client can show "as of week of …"
	WeekStart       time.Time `json:"week_start"`
	CurrentWeightG  float64   `json:"current_weight_g"`
	PreviousWeightG float64   `json:"previous_weight_g"`
	TotalFeedKg     float64   `json:"total_feed_kg"`
	FlockSize       int       `json:"flock_size"`

	// Derived — computed from the four fields above.
	DailyGainG float64 `json:"daily_gain_g"` // (current - previous) / 7
	FCR        float64 `json:"fcr"`          // (feed_kg * 1000) / (gain_g * flock_size)
}

const (
	DefaultTempMin     = 33.5
	DefaultTempMax     = 35.0
	DefaultHumidityMin = 55.0
	DefaultHumidityMax = 70.0
	DefaultFeedAlert   = 20.0 // % — alert when feed level drops below this
	DefaultWaterAlert  = 20.0 // % — alert when water level drops below this
)
