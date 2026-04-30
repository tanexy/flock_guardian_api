package brooders

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// GET /api/v1/brooders/:id/alerts/stream
//
// Separate SSE endpoint for alert events. The existing /stream endpoint
// (sensor data) is untouched.
//
// Each event has:
//
//	event: alert
//	data:  {"brooder_id":1,"condition":"temperature_high","message":"...","value":39.2,"severity":"critical","timestamp":"..."}
func (h *Handler) StreamAlerts(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")

	ch := h.alertHub.Subscribe(uint(id))
	defer h.alertHub.Unsubscribe(uint(id), ch)

	clientGone := c.Request.Context().Done()

	for {
		select {
		case <-clientGone:
			return
		case alert := <-ch:
			c.SSEvent("alert", alert)
			c.Writer.Flush()
		}
	}
}
