package server

import (
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
func (s *Server) RegisterRoutes() http.Handler {
	r := gin.Default()

	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173"}, // Add your frontend URL
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowHeaders:     []string{"Accept", "Authorization", "Content-Type", "Accept-Version"},
		AllowCredentials: true, // Enable cookies/auth
	}))
	db := s.db.DB()
	repo := users.NewGormRepository(db)
	service := users.NewService(repo)
	userHandler := users.NewHandler(service)
	api := r.Group("/api/v1")
	Users(api, userHandler)
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
