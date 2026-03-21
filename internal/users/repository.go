package users

import (
	"errors"

	"gorm.io/gorm"
)

// Repository defines the data access methods
type Repository interface {
	FindByEmail(email string) (*User, error)
	Create(user *User) error
}

// GormRepository implements Repository using GORM
type GormRepository struct {
	db *gorm.DB
}

// NewGormRepository creates a new GormRepository
func NewGormRepository(db *gorm.DB) *GormRepository {
	return &GormRepository{db: db}
}

// FindByEmail fetches a user by email
func (r *GormRepository) FindByEmail(email string) (*User, error) {
	var user User
	result := r.db.Where("email = ?", email).First(&user)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, errors.New("user not found")
	}
	return &user, result.Error
}

// Create inserts a new user into the database
func (r *GormRepository) Create(user *User) error {
	return r.db.Create(user).Error
}
