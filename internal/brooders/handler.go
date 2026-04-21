package brooders

import (
	"log"
	"net/http"
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// ── WebSocket Hub ─────────────────────────────────────────────────────────────

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type CommandMessage struct {
	Command string `json:"command"`
}

type Hub struct {
	mu      sync.Mutex
	clients map[uint]*websocket.Conn
}

var GlobalHub = &Hub{
	clients: make(map[uint]*websocket.Conn),
}

func (h *Hub) Register(brooderID uint, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[brooderID] = conn
	log.Printf("[WS] ESP32 registered for brooder %d", brooderID)
}

func (h *Hub) Unregister(brooderID uint) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, brooderID)
	log.Printf("[WS] ESP32 unregistered for brooder %d", brooderID)
}

func (h *Hub) Send(brooderID uint, msg CommandMessage) error {
	h.mu.Lock()
	conn, ok := h.clients[brooderID]
	h.mu.Unlock()
	if !ok {
		log.Printf("[WS] No ESP32 connected for brooder %d", brooderID)
		return nil
	}
	return conn.WriteJSON(msg)
}

// ── Handler ───────────────────────────────────────────────────────────────────

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
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
// Called by ESP32 to push sensor readings
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
// Called by mobile app to control devices instantly
// Body: {"command": "fan on"} | "fan off" | "pump on" | "pump off" | "heater on" | "heater off" | "feed on" | "feed off"
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

	// Get current state so we only change what the command targets
	brooder, err := h.service.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "brooder not found"})
		return
	}

	data := ActuatorUpdate{
		FanOn:         brooder.FanOn,
		DispenseFeed:  brooder.DispenseFeed,
		DispenseWater: brooder.DispenseWater,
		HeaterOn:      brooder.HeaterOn,
	}

	switch body.Command {
	case "fan on":
		data.FanOn = true
	case "fan off":
		data.FanOn = false
	case "pump on":
		data.DispenseWater = true
	case "pump off":
		data.DispenseWater = false
	case "feed on":
		data.DispenseFeed = true
	case "feed off":
		data.DispenseFeed = false
	case "heater on":
		data.HeaterOn = true
	case "heater off":
		data.HeaterOn = false
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown command: " + body.Command})
		return
	}

	// Save new state to DB
	if err := h.service.UpdateActuators(uint(id), data); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Push to ESP32 instantly via WebSocket
	if err := GlobalHub.Send(uint(id), CommandMessage{Command: body.Command}); err != nil {
		log.Printf("[WS] Failed to send command to ESP32: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "command sent",
		"command": body.Command,
	})
}

// GET /api/v1/brooders/:id/ws
// ESP32 connects here via WebSocket to receive instant commands
func (h *Handler) HandleWebSocket(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("[WS] Upgrade error:", err)
		return
	}
	defer conn.Close()

	brooderID := uint(id)
	GlobalHub.Register(brooderID, conn)
	defer GlobalHub.Unregister(brooderID)

	// Keep connection alive — block until ESP32 disconnects
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			log.Printf("[WS] ESP32 brooder %d disconnected: %v", brooderID, err)
			break
		}
	}
}
