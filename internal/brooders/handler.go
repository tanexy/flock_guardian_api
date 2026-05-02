package brooders

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type MQTTPublisher interface {
	Publish(brooderID uint, command string) error
}

type Handler struct {
	service   Service
	mqtt      MQTTPublisher
	hub       *Hub
	autoCtrl  *AutoController // nil-safe: auto mode skipped if not wired
	alertHub  *AlertHub
	alertCtrl *AlertController // nil-safe: alerts skipped if not wired
}
type BatchSensorReading struct {
	Temperature float64 `json:"temperature"`
	Humidity    float64 `json:"humidity"`
	FeedLevel   float64 `json:"feed_level"`
	WaterLevel  float64 `json:"water_level"`
	Ts          int64   `json:"ts"` // millis from ESP32
}

type BatchSensorUpload struct {
	Readings []BatchSensorReading `json:"readings"`
}

func NewHandler(
	service Service,
	mqtt MQTTPublisher,
	hub *Hub,
	autoCtrl *AutoController,
	alertHub *AlertHub,
	alertCtrl *AlertController,
) *Handler {
	return &Handler{
		service:   service,
		mqtt:      mqtt,
		hub:       hub,
		autoCtrl:  autoCtrl,
		alertHub:  alertHub,
		alertCtrl: alertCtrl,
	}
}

// GET /api/v1/brooders
func (h *Handler) GetAll(c *gin.Context) {
	brooders, err := h.service.GetAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, brooders)
}

// GET /api/v1/brooders/:id
func (h *Handler) GetByID(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	brooder, err := h.service.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "brooder not found"})
		return
	}
	c.JSON(http.StatusOK, brooder)
}

// POST /api/v1/brooders
func (h *Handler) Create(c *gin.Context) {
	var brooder Brooder
	if err := c.ShouldBindJSON(&brooder); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.service.Create(&brooder); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, brooder)
}

// PATCH /api/v1/brooders/:id/sensors
func (h *Handler) UpdateSensors(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var data SensorUpdate
	if err := c.ShouldBindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.service.UpdateSensorData(uint(id), data); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Push raw sensor data to SSE sensor-stream clients.
	h.hub.Publish(uint(id), data)

	// Forward to auto-controller for actuator decisions.
	if h.autoCtrl != nil {
		h.autoCtrl.NotifySensorUpdate(uint(id), data)
	}

	// Forward to alert controller for threshold breach checks.
	if h.alertCtrl != nil {
		h.alertCtrl.NotifySensorUpdate(uint(id), data)
	}

	c.JSON(http.StatusOK, gin.H{"message": "sensor data updated"})
}

// PATCH /api/v1/brooders/:id/actuators
func (h *Handler) UpdateActuators(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var data ActuatorUpdate
	if err := c.ShouldBindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.service.UpdateActuators(uint(id), data); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "actuators updated"})
}

// POST /api/v1/brooders/:id/command
func (h *Handler) SendCommand(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var body struct {
		Command string `json:"command"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	validCommands := map[string]bool{
		"fan on": true, "fan off": true,
		"pump on": true, "pump off": true,
		"servo on": true, "servo off": true,
		"heater on": true, "heater off": true,
		"feed on": true, "feed off": true,
	}
	if !validCommands[body.Command] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown command: " + body.Command})
		return
	}

	if h.mqtt != nil {
		if err := h.mqtt.Publish(uint(id), body.Command); err != nil {
			log.Printf("[MQTT] Failed to publish command: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to send command"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "command sent",
		"command": body.Command,
	})
}

// GET /api/v1/brooders/:id/stream
func (h *Handler) StreamSensors(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")

	ch := h.hub.Subscribe(uint(id))
	defer h.hub.Unsubscribe(uint(id), ch)

	clientGone := c.Request.Context().Done()

	for {
		select {
		case <-clientGone:
			return
		case data := <-ch:
			c.SSEvent("sensors", data)
			c.Writer.Flush()
		}
	}
}
func (h *Handler) BatchUploadSensorHistory(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	// Accept either {"readings": [...]} or a raw array [...]
	var rows []BatchSensorReading
	bodyBytes, _ := io.ReadAll(c.Request.Body)

	// Try raw array first (what ESP32 sends)
	if err := json.Unmarshal(bodyBytes, &rows); err != nil {
		// Try wrapped object
		var wrapped BatchSensorUpload
		if err2 := json.Unmarshal(bodyBytes, &wrapped); err2 != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
			return
		}
		rows = wrapped.Readings
	}

	if len(rows) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no readings provided"})
		return
	}

	// Convert to model
	now := time.Now()
	records := make([]HistoricalSensorData, len(rows))
	for i, r := range rows {
		records[i] = HistoricalSensorData{
			Temperature: r.Temperature,
			Humidity:    r.Humidity,
			FeedLevel:   r.FeedLevel,
			WaterLevel:  r.WaterLevel,
			RecordedAt:  now, // use now if no RTC on ESP32
		}
	}

	if err := h.service.BatchSaveHistoricalSensorData(uint(id), records); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "historical data saved",
		"count":   len(records),
	})
}
