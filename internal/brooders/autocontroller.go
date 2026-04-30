package brooders

import "log"

// ---------------------------------------------------------------------------
// Thresholds — industry-standard brooder values.
// All temperatures in °C, humidity/levels in %.
// ---------------------------------------------------------------------------
const (
	// Temperature → fan
	TempFanOnThreshold  = 38.0 // fan turns ON  when temp rises above this
	TempFanOffThreshold = 37.0 // fan turns OFF when temp falls below this

	// Humidity → pump
	HumidityPumpOnThreshold  = 55.0 // pump turns ON  when humidity drops below this
	HumidityPumpOffThreshold = 65.0 // pump turns OFF when humidity rises above this

	// Water level → pump (overrides humidity-off; pump stays on if water is low)
	WaterLevelPumpOnThreshold  = 20.0 // pump turns ON  when water level drops below this
	WaterLevelPumpOffThreshold = 30.0 // pump turns OFF when water level rises above this

	// Feed level → servo (feeder)
	FeedLevelServoOnThreshold  = 20.0 // servo turns ON  when feed drops below this
	FeedLevelServoOffThreshold = 30.0 // servo turns OFF when feed rises above this
)

// ---------------------------------------------------------------------------
// Per-actuator bang-bang state
// ---------------------------------------------------------------------------

// actuatorState tracks the last commanded ON/OFF state for one actuator on
// one brooder. known=false (zero value) means no command has been sent yet
// this session — the first evaluation always fires to initialise the device.
type actuatorState struct {
	on    bool
	known bool
}

// set sends a command only when the desired state differs from the last
// commanded state — this is the bang-bang behaviour. Returns true if a
// command was sent.
func (s *actuatorState) set(wantOn bool, brooderID uint, onCmd, offCmd string, ac *AutoController) bool {
	if s.known && s.on == wantOn {
		return false // no state change; suppress the command
	}
	s.on = wantOn
	s.known = true
	if wantOn {
		ac.send(brooderID, onCmd)
	} else {
		ac.send(brooderID, offCmd)
	}
	return true
}

// brooderState holds bang-bang state for every controlled actuator on a
// single brooder.
type brooderState struct {
	fan   actuatorState
	pump  actuatorState
	servo actuatorState
}

// ---------------------------------------------------------------------------
// AutoController
// ---------------------------------------------------------------------------

type sensorEvent struct {
	brooderID uint
	data      SensorUpdate
}

// AutoController listens for sensor updates forwarded by the HTTP handler
// and publishes MQTT commands whenever a brooder is in auto mode.
// All decisions and mutable state live inside a single goroutine — no
// mutexes needed.
type AutoController struct {
	service  Service
	mqtt     MQTTPublisher
	sensorCh chan sensorEvent
	stopCh   chan struct{}

	// states holds the last commanded actuator state per brooder.
	// Keyed by brooder ID; entries are created lazily on first event.
	states map[uint]*brooderState
}

// NewAutoController creates and starts the controller.
// Call Stop() to shut it down cleanly.
func NewAutoController(service Service, mqtt MQTTPublisher) *AutoController {
	ac := &AutoController{
		service:  service,
		mqtt:     mqtt,
		sensorCh: make(chan sensorEvent, 64),
		stopCh:   make(chan struct{}),
		states:   make(map[uint]*brooderState),
	}
	go ac.run()
	return ac
}

// NotifySensorUpdate is called by the HTTP handler after a successful sensor
// update. It is non-blocking; events are dropped if the channel is full.
func (ac *AutoController) NotifySensorUpdate(brooderID uint, data SensorUpdate) {
	select {
	case ac.sensorCh <- sensorEvent{brooderID: brooderID, data: data}:
	default:
		log.Printf("[AutoController] sensor channel full, dropping update for brooder %d", brooderID)
	}
}

// Stop shuts down the background goroutine gracefully.
func (ac *AutoController) Stop() {
	close(ac.stopCh)
}

// run is the single background goroutine — all decisions happen here.
func (ac *AutoController) run() {
	for {
		select {
		case <-ac.stopCh:
			return
		case evt := <-ac.sensorCh:
			ac.evaluate(evt)
		}
	}
}

// stateFor returns (creating if necessary) the brooderState for a given ID.
func (ac *AutoController) stateFor(id uint) *brooderState {
	if s, ok := ac.states[id]; ok {
		return s
	}
	s := &brooderState{}
	ac.states[id] = s
	return s
}

// evaluate applies bang-bang + hysteresis rules for a single sensor update.
//
// Each actuator has two thresholds forming a dead band:
//
//	Fan:   temp  > 38.0 °C → ON   |  temp  < 37.0 °C → OFF  |  37–38 °C → hold
//	Pump:  humid < 55 %   → ON   |  humid > 65 %   → OFF  |  55–65 % → hold
//	       water < 20 %   → ON   |  water > 30 %   → OFF  |  (either condition wins)
//	Servo: feed  < 20 %   → ON   |  feed  > 30 %   → OFF  |  20–30 % → hold
//
// A command is only published when the desired state CHANGES (OFF→ON or
// ON→OFF). Readings that fall inside the dead band leave the actuator alone.
func (ac *AutoController) evaluate(evt sensorEvent) {
	brooder, err := ac.service.GetByID(evt.brooderID)
	if err != nil {
		log.Printf("[AutoController] brooder %d not found: %v", evt.brooderID, err)
		return
	}
	if !brooder.AutoMode {
		return
	}

	d := evt.data
	st := ac.stateFor(evt.brooderID)

	// -------------------------------------------------------------------------
	// Fan  (temperature)
	// Dead band 37–38 °C — hold current state inside the band.
	// -------------------------------------------------------------------------
	switch {
	case d.Temperature > TempFanOnThreshold:
		st.fan.set(true, evt.brooderID, "fan on", "fan off", ac)
	case d.Temperature < TempFanOffThreshold:
		st.fan.set(false, evt.brooderID, "fan on", "fan off", ac)
		// else: inside dead band — do nothing
	}

	// -------------------------------------------------------------------------
	// Pump  (humidity + water level)
	// ON if either reading is low; OFF only when both have recovered.
	// Dead bands: humidity 55–65 %, water level 20–30 %.
	// -------------------------------------------------------------------------
	lowHumidity := d.Humidity < HumidityPumpOnThreshold
	lowWater := d.WaterLevel < WaterLevelPumpOnThreshold
	humidityOk := d.Humidity > HumidityPumpOffThreshold
	waterOk := d.WaterLevel > WaterLevelPumpOffThreshold

	switch {
	case lowHumidity || lowWater:
		st.pump.set(true, evt.brooderID, "pump on", "pump off", ac)
	case humidityOk && waterOk:
		st.pump.set(false, evt.brooderID, "pump on", "pump off", ac)
		// else: at least one reading is inside its dead band — hold
	}

	// -------------------------------------------------------------------------
	// Servo / Feeder  (feed level)
	// Dead band 20–30 % — hold current state inside the band.
	// -------------------------------------------------------------------------
	switch {
	case d.FeedLevel < FeedLevelServoOnThreshold:
		st.servo.set(true, evt.brooderID, "servo on", "servo off", ac)
	case d.FeedLevel > FeedLevelServoOffThreshold:
		st.servo.set(false, evt.brooderID, "servo on", "servo off", ac)
		// else: inside dead band — do nothing
	}
}

// send logs and publishes an MQTT command.
func (ac *AutoController) send(brooderID uint, command string) {
	log.Printf("[AutoController] brooder %d → %s", brooderID, command)
	if err := ac.mqtt.Publish(brooderID, command); err != nil {
		log.Printf("[AutoController] MQTT publish failed (brooder %d, cmd %q): %v", brooderID, command, err)
	}
}
