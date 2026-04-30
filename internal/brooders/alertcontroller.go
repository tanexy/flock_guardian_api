package brooders

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// Alert thresholds — reuse the same constants as the AutoController.
// ---------------------------------------------------------------------------
// TempFanOnThreshold    = 38.0  → temperature too high
// TempFanOffThreshold   = 37.0  → temperature too low
// HumidityPumpOnThreshold  = 55.0  → humidity too low
// HumidityPumpOffThreshold = 65.0  → humidity too high
// WaterLevelPumpOnThreshold  = 20.0  → water level low
// FeedLevelServoOnThreshold  = 20.0  → feed level low

const alertRepeatInterval = 3 * time.Minute

// ---------------------------------------------------------------------------
// Alert types
// ---------------------------------------------------------------------------

// AlertSeverity indicates how critical an alert is.
type AlertSeverity string

const (
	SeverityWarning  AlertSeverity = "warning"
	SeverityCritical AlertSeverity = "critical"
)

// Alert is the payload pushed to SSE clients.
type Alert struct {
	BrooderID uint          `json:"brooder_id"`
	Condition string        `json:"condition"` // e.g. "temperature_high"
	Message   string        `json:"message"`
	Value     float64       `json:"value"`
	Severity  AlertSeverity `json:"severity"`
	Timestamp time.Time     `json:"timestamp"`
}

// ---------------------------------------------------------------------------
// Per-brooder alert state
// ---------------------------------------------------------------------------

// alertCondition tracks repeat-suppression for one condition on one brooder.
type alertCondition struct {
	active   bool      // is the condition currently breached?
	lastSent time.Time // when the last alert was sent
}

// shouldAlert returns true if an alert should fire right now:
//   - first breach (active was false): always fire immediately
//   - ongoing breach: fire only if repeatInterval has elapsed since lastSent
func (a *alertCondition) shouldAlert(now time.Time) bool {
	if !a.active {
		return true // first breach — fire immediately
	}
	return now.Sub(a.lastSent) >= alertRepeatInterval
}

// brooderAlertState holds one alertCondition per monitored condition.
type brooderAlertState struct {
	tempHigh  alertCondition
	tempLow   alertCondition
	humidHigh alertCondition
	humidLow  alertCondition
	waterLow  alertCondition
	feedLow   alertCondition
}

// ---------------------------------------------------------------------------
// AlertHub — fan-out to SSE clients
// ---------------------------------------------------------------------------

// AlertHub manages SSE subscriber channels for the alert stream.
// Each brooder has its own set of subscribers.
type AlertHub struct {
	mu          sync.RWMutex
	subscribers map[uint][]chan Alert
}

func NewAlertHub() *AlertHub {
	return &AlertHub{subscribers: make(map[uint][]chan Alert)}
}

// Subscribe returns a channel that will receive alerts for the given brooder.
func (h *AlertHub) Subscribe(brooderID uint) chan Alert {
	ch := make(chan Alert, 8)
	h.mu.Lock()
	h.subscribers[brooderID] = append(h.subscribers[brooderID], ch)
	h.mu.Unlock()
	return ch
}

// Unsubscribe removes a channel from the subscriber list.
func (h *AlertHub) Unsubscribe(brooderID uint, ch chan Alert) {
	h.mu.Lock()
	defer h.mu.Unlock()
	subs := h.subscribers[brooderID]
	for i, s := range subs {
		if s == ch {
			h.subscribers[brooderID] = append(subs[:i], subs[i+1:]...)
			close(ch)
			return
		}
	}
}

// publish fans an alert out to all current subscribers for a brooder.
func (h *AlertHub) publish(a Alert) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, ch := range h.subscribers[a.BrooderID] {
		select {
		case ch <- a:
		default:
			log.Printf("[AlertHub] subscriber channel full for brooder %d, dropping alert", a.BrooderID)
		}
	}
}

// ---------------------------------------------------------------------------
// AlertController
// ---------------------------------------------------------------------------

// AlertController evaluates sensor updates against alert thresholds and
// pushes Alert events to the AlertHub. All mutable state lives in a single
// goroutine — no mutexes needed on the state maps.
type AlertController struct {
	hub      *AlertHub
	sensorCh chan sensorEvent // reuses the same sensorEvent type as AutoController
	stopCh   chan struct{}
	states   map[uint]*brooderAlertState
}

// NewAlertController creates and starts the alert controller.
// Call Stop() to shut it down cleanly.
func NewAlertController(hub *AlertHub) *AlertController {
	ac := &AlertController{
		hub:      hub,
		sensorCh: make(chan sensorEvent, 64),
		stopCh:   make(chan struct{}),
		states:   make(map[uint]*brooderAlertState),
	}
	go ac.run()
	return ac
}

// NotifySensorUpdate is called by the HTTP handler after a successful sensor
// update. Non-blocking — drops the event if the channel is full.
func (ac *AlertController) NotifySensorUpdate(brooderID uint, data SensorUpdate) {
	select {
	case ac.sensorCh <- sensorEvent{brooderID: brooderID, data: data}:
	default:
		log.Printf("[AlertController] channel full, dropping update for brooder %d", brooderID)
	}
}

// Stop shuts down the background goroutine gracefully.
func (ac *AlertController) Stop() {
	close(ac.stopCh)
}

func (ac *AlertController) run() {
	for {
		select {
		case <-ac.stopCh:
			return
		case evt := <-ac.sensorCh:
			ac.evaluate(evt)
		}
	}
}

// stateFor returns (lazily creating) the brooderAlertState for an ID.
func (ac *AlertController) stateFor(id uint) *brooderAlertState {
	if s, ok := ac.states[id]; ok {
		return s
	}
	s := &brooderAlertState{}
	ac.states[id] = s
	return s
}

// evaluate checks every sensor reading against its thresholds and fires
// alerts where needed, respecting the repeat-suppression window.
func (ac *AlertController) evaluate(evt sensorEvent) {
	d := evt.data
	st := ac.stateFor(evt.brooderID)
	now := time.Now()

	// -------------------------------------------------------------------------
	// Temperature
	// -------------------------------------------------------------------------
	ac.check(evt.brooderID, now,
		d.Temperature > TempFanOnThreshold,
		&st.tempHigh,
		"temperature_high",
		fmt.Sprintf("Temperature is too high (%.1f°C). Safe range: %.1f–%.1f°C",
			d.Temperature, TempFanOffThreshold, TempFanOnThreshold),
		d.Temperature,
		SeverityCritical,
	)
	ac.check(evt.brooderID, now,
		d.Temperature < TempFanOffThreshold,
		&st.tempLow,
		"temperature_low",
		fmt.Sprintf("Temperature is too low (%.1f°C). Safe range: %.1f–%.1f°C",
			d.Temperature, TempFanOffThreshold, TempFanOnThreshold),
		d.Temperature,
		SeverityCritical,
	)

	// -------------------------------------------------------------------------
	// Humidity
	// -------------------------------------------------------------------------
	ac.check(evt.brooderID, now,
		d.Humidity > HumidityPumpOffThreshold,
		&st.humidHigh,
		"humidity_high",
		fmt.Sprintf("Humidity is too high (%.1f%%). Safe range: %.0f–%.0f%%",
			d.Humidity, HumidityPumpOnThreshold, HumidityPumpOffThreshold),
		d.Humidity,
		SeverityWarning,
	)
	ac.check(evt.brooderID, now,
		d.Humidity < HumidityPumpOnThreshold,
		&st.humidLow,
		"humidity_low",
		fmt.Sprintf("Humidity is too low (%.1f%%). Safe range: %.0f–%.0f%%",
			d.Humidity, HumidityPumpOnThreshold, HumidityPumpOffThreshold),
		d.Humidity,
		SeverityWarning,
	)

	// -------------------------------------------------------------------------
	// Water level
	// -------------------------------------------------------------------------
	ac.check(evt.brooderID, now,
		d.WaterLevel < WaterLevelPumpOnThreshold,
		&st.waterLow,
		"water_level_low",
		fmt.Sprintf("Water level is low (%.1f%%). Please refill the water reservoir.", d.WaterLevel),
		d.WaterLevel,
		SeverityWarning,
	)

	// -------------------------------------------------------------------------
	// Feed level
	// -------------------------------------------------------------------------
	ac.check(evt.brooderID, now,
		d.FeedLevel < FeedLevelServoOnThreshold,
		&st.feedLow,
		"feed_level_low",
		fmt.Sprintf("Feed level is low (%.1f%%). Please refill the feeder.", d.FeedLevel),
		d.FeedLevel,
		SeverityWarning,
	)
}

// check evaluates a single condition and publishes an alert if needed.
// It also clears the active flag when the condition is no longer breached.
func (ac *AlertController) check(
	brooderID uint,
	now time.Time,
	breached bool,
	state *alertCondition,
	condition string,
	message string,
	value float64,
	severity AlertSeverity,
) {
	if !breached {
		// Condition cleared — reset so the next breach fires immediately again.
		state.active = false
		return
	}

	if !state.shouldAlert(now) {
		return // still within the repeat-suppression window
	}

	state.active = true
	state.lastSent = now

	alert := Alert{
		BrooderID: brooderID,
		Condition: condition,
		Message:   message,
		Value:     value,
		Severity:  severity,
		Timestamp: now,
	}

	log.Printf("[AlertController] brooder %d — %s: %s", brooderID, condition, message)
	ac.hub.publish(alert)
}
