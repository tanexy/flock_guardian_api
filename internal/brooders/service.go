package brooders

import (
	"fmt"
	"math"
	"sort"
	"time"
)

type Service interface {
	GetAll() ([]Brooder, error)
	GetByID(id uint) (*Brooder, error)
	Create(brooder *Brooder) error
	UpdateSensorData(id uint, data SensorUpdate) error
	UpdateActuators(id uint, data ActuatorUpdate) error
	BatchSaveHistoricalSensorData(brooderID uint, readings []HistoricalSensorData) error
	ComputeAnalytics(brooderID uint, period string) (*AnalyticsResponse, error)
	ComputeFCR(in FCRInput) (*FCRResult, error)
}

type BrooderService struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &BrooderService{repo: repo}
}

func (s *BrooderService) GetAll() ([]Brooder, error) {
	return s.repo.FindAll()
}

func (s *BrooderService) BatchSaveHistoricalSensorData(brooderID uint, readings []HistoricalSensorData) error {
	return s.repo.BatchInsertHistoricalSensorData(brooderID, readings)
}

func (s *BrooderService) GetByID(id uint) (*Brooder, error) {
	return s.repo.FindByID(id)
}

func (s *BrooderService) Create(brooder *Brooder) error {
	return s.repo.Create(brooder)
}

func (s *BrooderService) UpdateSensorData(id uint, data SensorUpdate) error {
	return s.repo.UpdateSensorData(id, data)
}

func (s *BrooderService) UpdateActuators(id uint, data ActuatorUpdate) error {
	return s.repo.UpdateActuators(id, data)
}

func (s *BrooderService) ComputeAnalytics(brooderID uint, period string) (*AnalyticsResponse, error) {
	days := parsePeriodDays(period)
	since := time.Now().UTC().AddDate(0, 0, -days)

	rows, err := s.repo.GetHistoricalSensorData(brooderID, since)
	if err != nil {
		return nil, fmt.Errorf("analytics: fetch history: %w", err)
	}

	summary, daily := aggregate(rows, days)

	return &AnalyticsResponse{
		BrooderID:      brooderID,
		Period:         period,
		Summary:        summary,
		DailyBreakdown: daily,
		AlertSummary:   buildAlertSummary(rows),
	}, nil
}

func (s *BrooderService) ComputeFCR(in FCRInput) (*FCRResult, error) {
	if in.TotalFeedKg <= 0 {
		return nil, fmt.Errorf("fcr: total_feed_kg must be > 0")
	}
	if in.EndWeightKg <= in.StartWeightKg {
		return nil, fmt.Errorf("fcr: end_weight_kg must exceed start_weight_kg")
	}

	brooder, err := s.repo.FindByUUID(in.BrooderUUID)
	if err != nil {
		return nil, fmt.Errorf("fcr: brooder not found: %w", err)
	}

	numberOfBirds := int(brooder.FlockSize)
	mortalityCount := int(brooder.MortalityCount)

	if numberOfBirds <= 0 {
		return nil, fmt.Errorf("fcr: brooder has no flock size set")
	}
	if mortalityCount < 0 || mortalityCount >= numberOfBirds {
		return nil, fmt.Errorf("fcr: mortality_count out of range [0, flock_size)")
	}

	adjustedBirds := numberOfBirds - mortalityCount
	totalWeightGain := (in.EndWeightKg - in.StartWeightKg) * float64(adjustedBirds)

	if totalWeightGain <= 0 {
		return nil, fmt.Errorf("fcr: computed total weight gain is zero or negative")
	}

	fcr := round2(in.TotalFeedKg / totalWeightGain)
	feedPerBird := round2(in.TotalFeedKg / float64(adjustedBirds))

	return &FCRResult{
		BrooderUUID:       in.BrooderUUID,
		FlockID:           in.FlockID,
		FCR:               fcr,
		Rating:            fcrRating(fcr),
		TotalFeedKg:       round2(in.TotalFeedKg),
		TotalWeightGainKg: round2(totalWeightGain),
		AdjustedBirds:     adjustedBirds,
		FeedPerBirdKg:     feedPerBird,
	}, nil
}

func fcrRating(fcr float64) string {
	switch {
	case fcr < 1.6:
		return "Excellent"
	case fcr < 1.9:
		return "Good"
	case fcr < 2.2:
		return "Fair"
	default:
		return "Poor"
	}
}

func parsePeriodDays(period string) int {
	switch period {
	case "14d":
		return 14
	case "30d":
		return 30
	default:
		return 7
	}
}

func aggregate(rows []HistoricalSensorData, days int) (AnalyticsSummary, []DailyAnalytics) {
	type bucket struct {
		temps     []float64
		humids    []float64
		feeds     []float64
		waters    []float64
		lastFeed  float64
		lastWater float64
	}

	buckets := make(map[string]*bucket)

	for _, r := range rows {
		key := r.RecordedAt.UTC().Format("2006-01-02")
		b, ok := buckets[key]
		if !ok {
			b = &bucket{}
			buckets[key] = b
		}
		b.temps = append(b.temps, r.Temperature)
		b.humids = append(b.humids, r.Humidity)
		b.feeds = append(b.feeds, r.FeedLevel)
		b.waters = append(b.waters, r.WaterLevel)
		b.lastFeed = r.FeedLevel
		b.lastWater = r.WaterLevel
	}

	now := time.Now().UTC()
	dayKeys := make([]string, days)
	for i := 0; i < days; i++ {
		t := now.AddDate(0, 0, -(days - 1 - i))
		dayKeys[i] = t.Format("2006-01-02")
	}

	daily := make([]DailyAnalytics, 0, days)

	var (
		allTemps   []float64
		allHumids  []float64
		allFeeds   []float64
		allWaters  []float64
		daysInTemp int
		daysInHum  int
		totalRead  int
	)

	for _, key := range dayKeys {
		t, _ := time.Parse("2006-01-02", key)
		label := t.Weekday().String()[:3]

		b, exists := buckets[key]
		if !exists {
			daily = append(daily, DailyAnalytics{
				Date:     key,
				DayLabel: label,
			})
			continue
		}

		avgTemp := mean(b.temps)
		avgHumid := mean(b.humids)
		inTarget := avgTemp >= DefaultTempMin && avgTemp <= DefaultTempMax
		inHumid := avgHumid >= DefaultHumidityMin && avgHumid <= DefaultHumidityMax

		if inTarget {
			daysInTemp++
		}
		if inHumid {
			daysInHum++
		}

		n := len(b.temps)
		totalRead += n

		allTemps = append(allTemps, b.temps...)
		allHumids = append(allHumids, b.humids...)
		allFeeds = append(allFeeds, b.feeds...)
		allWaters = append(allWaters, b.waters...)

		daily = append(daily, DailyAnalytics{
			Date:           key,
			DayLabel:       label,
			AvgTemperature: round2(avgTemp),
			MinTemperature: round2(minSlice(b.temps)),
			MaxTemperature: round2(maxSlice(b.temps)),
			AvgHumidity:    round2(avgHumid),
			FeedLevel:      round2(b.lastFeed),
			WaterLevel:     round2(b.lastWater),
			TempInTarget:   inTarget,
			ReadingCount:   n,
		})
	}

	summary := AnalyticsSummary{
		AvgTemperature:       round2(mean(allTemps)),
		MinTemperature:       round2(minSlice(allTemps)),
		MaxTemperature:       round2(maxSlice(allTemps)),
		TempVariance:         round2(stdDev(allTemps)),
		AvgHumidity:          round2(mean(allHumids)),
		MinHumidity:          round2(minSlice(allHumids)),
		MaxHumidity:          round2(maxSlice(allHumids)),
		AvgFeedLevel:         round2(mean(allFeeds)),
		AvgWaterLevel:        round2(mean(allWaters)),
		DaysInTempTarget:     daysInTemp,
		DaysInHumidityTarget: daysInHum,
		TotalReadings:        totalRead,
	}

	return summary, daily
}

func buildAlertSummary(rows []HistoricalSensorData) AlertSummary {
	var s AlertSummary
	for _, r := range rows {
		triggered := false
		if r.Temperature < DefaultTempMin || r.Temperature > DefaultTempMax {
			s.TempAlerts++
			triggered = true
		}
		if r.Humidity < DefaultHumidityMin || r.Humidity > DefaultHumidityMax {
			s.HumidityAlerts++
			triggered = true
		}
		if r.FeedLevel < DefaultFeedAlert {
			s.FeedAlerts++
			triggered = true
		}
		if r.WaterLevel < DefaultWaterAlert {
			s.WaterAlerts++
			triggered = true
		}
		if triggered {
			s.TotalAlerts++
		}
	}
	return s
}

func mean(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	var sum float64
	for _, x := range v {
		sum += x
	}
	return sum / float64(len(v))
}

func stdDev(v []float64) float64 {
	if len(v) < 2 {
		return 0
	}
	m := mean(v)
	var variance float64
	for _, x := range v {
		d := x - m
		variance += d * d
	}
	return math.Sqrt(variance / float64(len(v)))
}

func minSlice(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	m := v[0]
	for _, x := range v[1:] {
		if x < m {
			m = x
		}
	}
	return m
}

func maxSlice(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	m := v[0]
	for _, x := range v[1:] {
		if x > m {
			m = x
		}
	}
	return m
}

func round2(f float64) float64 {
	return math.Round(f*100) / 100
}

func sortedKeys(m map[string]*struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
