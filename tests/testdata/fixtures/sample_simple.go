package sample

import (
	"context"
	"fmt"
)

// User represents a user entity in the system
type User struct {
	ID        int64
	Name      string
	Email     string
	CreatedAt int64
}

// UserRepository provides access to user data
type UserRepository interface {
	Save(ctx context.Context, user *User) error
	FindByID(ctx context.Context, id int64) (*User, error)
}

// Greet returns a greeting message for the user
func (u *User) Greet() string {
	return fmt.Sprintf("Hello, %s!", u.Name)
}

// ValidateEmail checks if the email is valid
func ValidateEmail(email string) bool {
	return len(email) > 0
}

const (
	MaxNameLength = 100
	MinNameLength = 2
)

var (
	DefaultName  = "Guest"
	DefaultEmail = "guest@example.com"
)
