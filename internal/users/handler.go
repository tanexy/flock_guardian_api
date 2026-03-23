package users

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type LoginForm struct {
	Email    string `form:"email" binding:"required"`
	Password string `form:"password" binding:"required"`
}
type SignUpForm struct {
	Email    string `form:"email" binding:"required"`
	Password string `form:"password" binding:"required"`
	Farm     string `form:"farm" binding:"required"`
}

type Handler struct {
	service *Service
}

func NewHandler(s *Service) *Handler {
	return &Handler{service: s}
}

func (h *Handler) Login(c *gin.Context) {
	var form LoginForm

	if err := c.ShouldBind(&form); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid input",
		})
		return
	}

	user, accessToken, refreshToken, err := h.service.Authenticate(form.Email, form.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "invalid credentials",
		})
		return
	}
	type UserResponse struct {
		Id           uint   `json:"id"`
		Email        string `json:"email"`
		Token        string `json:"XyZaccess_token"`
		RefreshToken string `json:"XyZrefresh_token"`
		Farm         string `json:"farm"`
	}

	c.JSON(http.StatusOK, gin.H{
		"user": UserResponse{
			Id:           user.ID,
			Email:        user.Email,
			Token:        accessToken,
			RefreshToken: refreshToken,
			Farm:         user.Farm,
		},
	})
}
func (h *Handler) Logout(c *gin.Context) {}
func (h *Handler) Register(c *gin.Context) {
	var form SignUpForm
	if err := c.ShouldBind(&form); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid input",
		})
		return
	}

	user := &User{
		Email:    form.Email,
		Password: form.Password,
		Farm:     form.Farm,
	}

	if err := h.service.Create(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "user created successfully"})
}
