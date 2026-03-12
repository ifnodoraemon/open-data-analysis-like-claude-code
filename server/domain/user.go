package domain

import "time"

type UserStatus string

const (
	UserStatusActive UserStatus = "active"
)

type User struct {
	ID           string
	Email        string
	PasswordHash string
	Name         string
	AvatarURL    string
	Status       UserStatus
	CreatedAt    time.Time
	UpdatedAt    time.Time
	LastLoginAt  *time.Time
}
