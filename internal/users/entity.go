package users

import "gorm.io/gorm"

type User struct {
	gorm.Model
	Username string
	Password string `gorm:"unique"`
	Email    string `gorm:"unique"`
	Farm     string
}
