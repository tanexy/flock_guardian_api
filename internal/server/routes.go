package server

import (
	"flock_guardian_api/internal/brooders"
	"flock_guardian_api/internal/users"
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func Users(rg *gin.RouterGroup, handler *users.Handler) {
	auth := rg.Group("/auth")
	{
		auth.POST("/login", handler.Login)
		auth.POST("/register", handler.Register)
		auth.POST("/logout", handler.Logout)
	}
}

func Brooders(rg *gin.RouterGroup, handler *brooders.Handler) {
	b := rg.Group("/brooders")
	{
		b.GET("", handler.GetAll)
		b.POST("", handler.Create)
		b.GET("/:id", handler.GetByID)
		b.GET("/:id/ws", handler.HandleWebSocket)
		b.PATCH("/:id/sensors", handler.UpdateSensors)
		b.PATCH("/:id/actuators", handler.UpdateActuators)
		b.POST("/:id/command", handler.SendCommand) // ← add this
	}
}

func (s *Server) RegisterRoutes() http.Handler {
	r := gin.Default()

	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowHeaders:     []string{"Accept", "Authorization", "Content-Type", "Accept-Version"},
		AllowCredentials: true,
	}))

	db := s.db.DB()

	// Users
	userRepo := users.NewGormRepository(db)
	userService := users.NewService(userRepo)
	userHandler := users.NewHandler(userService)

	// Brooders
	brooderRepo := brooders.NewGormRepository(db)
	brooderService := brooders.NewService(brooderRepo)
	brooderHandler := brooders.NewHandler(brooderService)

	api := r.Group("/api/v1")
	Users(api, userHandler)
	Brooders(api, brooderHandler)

	r.GET("/", s.HelloWorldHandler)
	r.GET("/health", s.healthHandler)

	return r
}

func (s *Server) HelloWorldHandler(c *gin.Context) {
	resp := make(map[string]string)
	resp["message"] = "Hello World"
	c.JSON(http.StatusOK, resp)
}

func (s *Server) healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, s.db.Health())
}
