package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	_ "github.com/joho/godotenv/autoload"
	_ "github.com/tursodatabase/libsql-client-go/libsql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type Service interface {
	Health() map[string]string
	Close() error
	DB() *gorm.DB
}

type service struct {
	db *gorm.DB
}

var dbInstance *service

func New() Service {
	if dbInstance != nil {
		return dbInstance
	}

	dbURL := os.Getenv("TURSO_DSN")
	authToken := os.Getenv("TURSO_AUTH_TOKEN")

	if dbURL == "" {
		log.Fatal("TURSO_DSN is not set")
	}
	if authToken == "" {
		log.Fatal("TURSO_AUTH_TOKEN is not set")
	}

	dsn := fmt.Sprintf("%s?authToken=%s", dbURL, authToken)

	sqlDB, err := sql.Open("libsql", dsn)
	if err != nil {
		log.Fatal("failed to open Turso database:", err)
	}

	db, err := gorm.Open(sqlite.Dialector{Conn: sqlDB}, &gorm.Config{})
	if err != nil {
		log.Fatal("failed to connect to Turso database:", err)
	}

	dbInstance = &service{db: db}
	return dbInstance
}

func (s *service) DB() *gorm.DB {
	return s.db
}

func (s *service) Health() map[string]string {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	stats := make(map[string]string)

	sqlDB, err := s.db.WithContext(ctx).DB()
	if err != nil {
		stats["status"] = "down"
		stats["error"] = fmt.Sprintf("failed to get sql.DB: %v", err)
		return stats
	}

	if err := sqlDB.PingContext(ctx); err != nil {
		stats["status"] = "down"
		stats["error"] = fmt.Sprintf("db down: %v", err)
		return stats
	}

	stats["status"] = "up"
	stats["message"] = "It's healthy"

	dbStats := sqlDB.Stats()
	stats["open_connections"] = strconv.Itoa(dbStats.OpenConnections)
	stats["in_use"] = strconv.Itoa(dbStats.InUse)
	stats["idle"] = strconv.Itoa(dbStats.Idle)

	return stats
}

func (s *service) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	log.Println("Disconnected from Turso database")
	return sqlDB.Close()
}
