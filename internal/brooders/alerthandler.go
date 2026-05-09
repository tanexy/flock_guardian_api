package brooders

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

func (h *Handler) StreamAlerts(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming unsupported"})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("X-Accel-Buffering", "no") // critical on Render/nginx

	ch := h.alertHub.Subscribe(uint(id))
	defer h.alertHub.Unsubscribe(uint(id), ch)

	clientGone := c.Request.Context().Done()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	// Confirm stream is open
	_, err = fmt.Fprintf(c.Writer, ": ping\n\n")
	if err != nil {
		return
	}
	flusher.Flush()

	for {
		select {
		case <-clientGone:
			return
		case alert := <-ch:
			c.SSEvent("alert", alert)
			flusher.Flush()
		case <-ticker.C:
			// SSE comment keeps connection alive through proxies
			_, err := fmt.Fprintf(c.Writer, ": keepalive\n\n")
			if err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
