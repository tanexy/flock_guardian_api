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
	port int
	db   database.Service
	mqtt *mqtt.Subscriber
	hub  *brooders.Hub
}

func NewServer() *http.Server {
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

	// Hub shared between MQTT subscriber and HTTP stream handler
	hub := brooders.NewHub()

	mqttSubscriber := mqtt.NewSubscriber(mqttCfg, brooderService, hub)
	if err := mqttSubscriber.Connect(); err != nil {
		fmt.Printf("[MQTT] Warning: could not connect: %v\n", err)
	} else {
		mqttSubscriber.Subscribe()
	}

	newServer := &Server{
		port: port,
		db:   db,
		mqtt: mqttSubscriber,
		hub:  hub,
	}

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", newServer.port),
		Handler:      newServer.RegisterRoutes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0, // must be 0 — SSE connections are long-lived
	}

	return server
}
