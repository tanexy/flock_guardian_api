package users

import "gorm.io/gorm"

type User struct {
	gorm.Model
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `gorm:"unique"`
	Farm     string `json:"farm"`
}
