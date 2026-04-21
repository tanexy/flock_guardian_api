package mqtt

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"flock_guardian_api/internal/brooders"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"
)

// Config holds MQTT broker connection details
type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	ClientID string
}

// Subscriber listens to MQTT topics and saves data to DB
type Subscriber struct {
	client  pahomqtt.Client
	service brooders.Service
}

// SensorPayload matches what ESP32 publishes
type SensorPayload struct {
	Temperature float64 `json:"temperature"`
	Humidity    float64 `json:"humidity"`
	FeedLevel   float64 `json:"feed_level"`
	WaterLevel  float64 `json:"water_level"`
}

func NewSubscriber(cfg Config, service brooders.Service) *Subscriber {
	opts := pahomqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tls://%s:%d", cfg.Host, cfg.Port))
	opts.SetClientID(cfg.ClientID)
	opts.SetUsername(cfg.Username)
	opts.SetPassword(cfg.Password)
	opts.SetKeepAlive(60 * time.Second)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(3 * time.Second)

	opts.OnConnect = func(c pahomqtt.Client) {
		log.Println("[MQTT] Backend connected to broker")
	}
	opts.OnConnectionLost = func(c pahomqtt.Client, err error) {
		log.Printf("[MQTT] Connection lost: %v", err)
	}

	client := pahomqtt.NewClient(opts)
	return &Subscriber{client: client, service: service}
}

func (s *Subscriber) Connect() error {
	token := s.client.Connect()
	token.Wait()
	if err := token.Error(); err != nil {
		return fmt.Errorf("MQTT connect failed: %w", err)
	}
	log.Println("[MQTT] Connected to EMQX broker")
	return nil
}

func (s *Subscriber) Subscribe() {
	// Subscribe to all brooder sensor topics
	// brooder/+/sensors matches brooder/1/sensors, brooder/2/sensors etc
	topic := "brooder/+/sensors"
	s.client.Subscribe(topic, 1, s.handleSensorMessage)
	log.Printf("[MQTT] Subscribed to %s", topic)

	// Subscribe to status topics
	s.client.Subscribe("brooder/+/status", 1, s.handleStatusMessage)
	log.Println("[MQTT] Subscribed to brooder/+/status")
}

func (s *Subscriber) handleSensorMessage(client pahomqtt.Client, msg pahomqtt.Message) {
	log.Printf("[MQTT] Sensor data received on %s: %s", msg.Topic(), msg.Payload())

	var payload SensorPayload
	if err := json.Unmarshal(msg.Payload(), &payload); err != nil {
		log.Printf("[MQTT] Failed to parse sensor payload: %v", err)
		return
	}

	// Extract brooder ID from topic e.g. "brooder/1/sensors" → 1
	var brooderID uint
	fmt.Sscanf(msg.Topic(), "brooder/%d/sensors", &brooderID)
	if brooderID == 0 {
		log.Println("[MQTT] Could not extract brooder ID from topic")
		return
	}

	// Save to database
	update := brooders.SensorUpdate{
		Temperature: payload.Temperature,
		Humidity:    payload.Humidity,
		FeedLevel:   payload.FeedLevel,
		WaterLevel:  payload.WaterLevel,
	}
	if err := s.service.UpdateSensorData(brooderID, update); err != nil {
		log.Printf("[MQTT] Failed to save sensor data: %v", err)
		return
	}

	log.Printf("[MQTT] Brooder %d sensor data saved to DB", brooderID)
}

func (s *Subscriber) handleStatusMessage(client pahomqtt.Client, msg pahomqtt.Message) {
	log.Printf("[MQTT] Status received on %s: %s", msg.Topic(), msg.Payload())

	var status struct {
		Fan   bool `json:"fan"`
		Pump  bool `json:"pump"`
		Servo bool `json:"servo"`
	}
	if err := json.Unmarshal(msg.Payload(), &status); err != nil {
		log.Printf("[MQTT] Failed to parse status payload: %v", err)
		return
	}

	var brooderID uint
	fmt.Sscanf(msg.Topic(), "brooder/%d/status", &brooderID)
	if brooderID == 0 {
		return
	}

	// Update actuator state in DB
	update := brooders.ActuatorUpdate{
		FanOn:         status.Fan,
		DispenseWater: status.Pump,
	}
	if err := s.service.UpdateActuators(brooderID, update); err != nil {
		log.Printf("[MQTT] Failed to save status: %v", err)
		return
	}
	log.Printf("[MQTT] Brooder %d status saved to DB", brooderID)
}

// Publish sends a command to a brooder via MQTT
// Called by the HTTP handler when mobile app sends a command
func (s *Subscriber) Publish(brooderID uint, command string) error {
	topic := fmt.Sprintf("brooder/%d/commands", brooderID)
	payload := fmt.Sprintf(`{"command":"%s"}`, command)
	token := s.client.Publish(topic, 1, false, payload)
	token.Wait()
	if err := token.Error(); err != nil {
		return fmt.Errorf("MQTT publish failed: %w", err)
	}
	log.Printf("[MQTT] Published to %s: %s", topic, payload)
	return nil
}
