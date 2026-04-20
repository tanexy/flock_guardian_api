package database

import (
	"context"
	"flock_guardian_api/internal/brooders"
	"flock_guardian_api/internal/users"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	_ "github.com/joho/godotenv/autoload"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type Service interface {
	Health() map[string]string
	Close() error
	DB() *gorm.DB // Expose GORM DB for use in other packages
}

type service struct {
	db *gorm.DB
}

var (
	dburl      = os.Getenv("BLUEPRINT_DB_URL")
	dbInstance *service
)

func New() Service {
	if dbInstance != nil {
		return dbInstance
	}

	if dburl == "" {
		log.Fatal("BLUEPRINT_DB_URL is not set")
	}

	// Auto-create ./db/ directory if missing
	if dir := filepath.Dir(dburl); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Fatalf("failed to create db directory: %v", err)
		}
	}

	db, err := gorm.Open(sqlite.Open(dburl), &gorm.Config{})
	if err != nil {
		log.Fatal("failed to connect to database:", err)
	}

	// Auto-migrate your models
	err = db.AutoMigrate(&users.User{}, &brooders.Brooder{})
	if err != nil {
		log.Fatal("failed to migrate database:", err)
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

	// Get underlying *sql.DB to ping and get stats
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
	log.Printf("Disconnected from database: %s", dburl)
	return sqlDB.Close()
}
