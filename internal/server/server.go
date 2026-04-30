package server

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	_ "github.com/joho/godotenv/autoload"

	"flock_guardian_api/internal/brooders"
	"flock_guardian_api/internal/database"
	"flock_guardian_api/internal/mqtt"
)

type Server struct {
	HTTPServer *http.Server // exposed so main.go can call ListenAndServe
	port       int
	db         database.Service
	mqtt       *mqtt.Subscriber
	hub        *brooders.Hub
	alertHub   *brooders.AlertHub
	autoCtrl   *brooders.AutoController
	alertCtrl  *brooders.AlertController
}

// NewServer constructs the Server and its http.Server. The caller must call
// srv.HTTPServer.ListenAndServe() to start accepting connections, and
// srv.Shutdown() after it returns to clean up background goroutines.
func NewServer() *Server {
	port, _ := strconv.Atoi(os.Getenv("PORT"))

	mqttCfg := mqtt.Config{
		Host:     os.Getenv("MQTT_HOST"),
		Port:     8883,
		Username: os.Getenv("MQTT_USERNAME"),
		Password: os.Getenv("MQTT_PASSWORD"),
		ClientID: "flock-guardian-backend",
	}

	db := database.New()
	brooderRepo := brooders.NewGormRepository(db.DB())
	brooderService := brooders.NewService(brooderRepo)

	hub := brooders.NewHub()
	alertHub := brooders.NewAlertHub()
	alertCtrl := brooders.NewAlertController(alertHub)

	mqttSubscriber := mqtt.NewSubscriber(mqttCfg, brooderService, hub)
	if err := mqttSubscriber.Connect(); err != nil {
		fmt.Printf("[MQTT] Warning: could not connect: %v\n", err)
	} else {
		mqttSubscriber.Subscribe()
	}

	autoCtrl := brooders.NewAutoController(brooderService, mqttSubscriber)

	srv := &Server{
		port:      port,
		db:        db,
		mqtt:      mqttSubscriber,
		hub:       hub,
		alertHub:  alertHub,
		autoCtrl:  autoCtrl,
		alertCtrl: alertCtrl,
	}

	srv.HTTPServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      srv.RegisterRoutes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0, // must be 0 — SSE connections are long-lived
	}

	return srv
}

// Shutdown stops background goroutines. Call after HTTPServer.Shutdown().
func (s *Server) Shutdown() {
	if s.autoCtrl != nil {
		s.autoCtrl.Stop()
	}
	if s.alertCtrl != nil {
		s.alertCtrl.Stop()
	}
}
