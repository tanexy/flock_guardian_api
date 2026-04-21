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
}

func NewServer() *http.Server {
	port, _ := strconv.Atoi(os.Getenv("PORT"))

	// Setup MQTT
	mqttCfg := mqtt.Config{
		Host:     os.Getenv("MQTT_HOST"), // a14b5244.ala.eu-central-1.emqxsl.com
		Port:     8883,
		Username: os.Getenv("MQTT_USERNAME"),
		Password: os.Getenv("MQTT_PASSWORD"),
		ClientID: "flock-guardian-backend",
	}

	// Setup DB and brooder service for MQTT subscriber
	db := database.New()
	brooderRepo := brooders.NewGormRepository(db.DB())
	brooderService := brooders.NewService(brooderRepo)

	// Create MQTT subscriber
	mqttSubscriber := mqtt.NewSubscriber(mqttCfg, brooderService)
	if err := mqttSubscriber.Connect(); err != nil {
		fmt.Printf("[MQTT] Warning: could not connect: %v\n", err)
	} else {
		mqttSubscriber.Subscribe()
	}

	newServer := &Server{
		port: port,
		db:   db,
		mqtt: mqttSubscriber,
	}

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", newServer.port),
		Handler:      newServer.RegisterRoutes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	return server
}
