package brooders

import "gorm.io/gorm"

type FCRInput struct {
	gorm.Model

	BrooderUUID string  `json:"brooder_uuid"`
	Brooder     Brooder `gorm:"foreignKey:BrooderUUID;references:UUID" json:"-"`

	FlockID       string  `json:"flock_id"`
	TotalFeedKg   float64 `json:"total_feed_kg"`
	StartWeightKg float64 `json:"start_weight_kg"`
	EndWeightKg   float64 `json:"end_weight_kg"`
}

type FCRResult struct {
	gorm.Model

	BrooderUUID string  `json:"brooder_uuid"`
	Brooder     Brooder `gorm:"foreignKey:BrooderUUID;references:UUID" json:"-"`

	FlockID           string  `json:"flock_id"`
	FCR               float64 `json:"fcr"`
	Rating            string  `json:"rating"`
	TotalFeedKg       float64 `json:"total_feed_kg"`
	TotalWeightGainKg float64 `json:"total_weight_gain_kg"`
	AdjustedBirds     int     `json:"adjusted_birds"`
	FeedPerBirdKg     float64 `json:"feed_per_bird_kg"`
}
