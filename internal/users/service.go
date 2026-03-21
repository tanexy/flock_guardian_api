package users

import (
	"errors"
	"os"

	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	repo          Repository
	jwtSecret     []byte
	refreshSecret []byte
}

func NewService(r Repository) *Service {
	return &Service{
		repo:          r,
		jwtSecret:     []byte(os.Getenv("JWT_SECRET")),
		refreshSecret: []byte(os.Getenv("JWT_REFRESH_SECRET")),
	}
}

// Authenticate checks user credentials
func (s *Service) Authenticate(email, password string) (*User, string, string, error) {
	user, err := s.repo.FindByEmail(email)
	if err != nil {
		return nil, "", "", errors.New("invalid credentials")
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password))
	if err != nil {
		return nil, "", "", errors.New("invalid credentials")
	}

	accessToken, err := s.GenerateToken(user)
	if err != nil {
		return nil, "", "", err
	}

	refreshToken, err := s.GenerateRefreshToken(user)
	if err != nil {
		return nil, "", "", err
	}

	return user, accessToken, refreshToken, nil
}
func (s *Service) Create(user *User) error {
	// Check if user already exists
	existingUser, err := s.repo.FindByEmail(user.Email)
	if err == nil && existingUser != nil {
		return errors.New("user with this email already exists")
	}

	// Hash password before saving
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	user.Password = string(hashedPassword)

	// Create user
	return s.repo.Create(user)
}
