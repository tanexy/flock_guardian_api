package users

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

type Claims struct {
	UserID uint   `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

func (s *Service) GenerateToken(user *User) (string, error) {
	claims := Claims{
		UserID: user.ID,
		Email:  user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "flock-guardian",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

func (s *Service) GenerateRefreshToken(user *User) (string, error) {
	claims := Claims{
		UserID: user.ID,
		Email:  user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)), // 7 days
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "your-app-name",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.refreshSecret)
}

func (s *Service) RefreshAccessToken(refreshToken string) (string, error) {
	token, err := jwt.ParseWithClaims(refreshToken, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return s.refreshSecret, nil
	})
	if err != nil || !token.Valid {
		return "", errors.New("invalid refresh token")
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		return "", errors.New("invalid claims")
	}

	// Issue a new access token
	user := &User{
		Model: gorm.Model{ID: claims.UserID},
		Email: claims.Email,
	}
	return s.GenerateToken(user)
}

func (s *Service) ValidateToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return s.jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return nil, errors.New("invalid token")
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, errors.New("invalid claims")
	}
	return claims, nil
}
