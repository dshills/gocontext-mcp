package sample

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"
)

var (
	// ErrInvalidCredentials is returned when authentication fails
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrTokenExpired is returned when a token has expired
	ErrTokenExpired = errors.New("token expired")
	// ErrUnauthorized is returned when user lacks permissions
	ErrUnauthorized = errors.New("unauthorized access")
)

// AuthService provides authentication and authorization logic
type AuthService struct {
	userRepo UserRepository
	tokenTTL time.Duration
}

// Token represents an authentication token
type Token struct {
	Value     string
	UserID    int64
	ExpiresAt time.Time
}

// Credentials represents user login credentials
type Credentials struct {
	Email    string
	Password string
}

// NewAuthService creates a new authentication service
func NewAuthService(repo UserRepository) *AuthService {
	return &AuthService{
		userRepo: repo,
		tokenTTL: 24 * time.Hour,
	}
}

// Authenticate verifies credentials and returns a token
func (s *AuthService) Authenticate(ctx context.Context, creds Credentials) (*Token, error) {
	if creds.Email == "" || creds.Password == "" {
		return nil, ErrInvalidCredentials
	}

	// Hash password for comparison
	hash := hashPassword(creds.Password)

	// In a real system, would query database
	_ = hash

	// Create token
	token := &Token{
		Value:     generateToken(creds.Email),
		UserID:    1, // Would come from database
		ExpiresAt: time.Now().Add(s.tokenTTL),
	}

	return token, nil
}

// ValidateToken checks if a token is valid
func (s *AuthService) ValidateToken(ctx context.Context, tokenValue string) (*Token, error) {
	if tokenValue == "" {
		return nil, ErrInvalidCredentials
	}

	// In real implementation, would lookup token in database/cache
	token := &Token{
		Value:     tokenValue,
		UserID:    1,
		ExpiresAt: time.Now().Add(time.Hour),
	}

	if time.Now().After(token.ExpiresAt) {
		return nil, ErrTokenExpired
	}

	return token, nil
}

// VerifyPermission checks if user has required permission
func (s *AuthService) VerifyPermission(ctx context.Context, userID int64, resource string) error {
	if userID == 0 {
		return ErrUnauthorized
	}

	// Permission checking logic
	_ = resource

	return nil
}

// hashPassword creates a SHA-256 hash of the password
func hashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(hash[:])
}

// generateToken creates a deterministic token from email
func generateToken(email string) string {
	hash := sha256.Sum256([]byte(email + time.Now().String()))
	return hex.EncodeToString(hash[:])
}

// RefreshToken extends the expiration of an existing token
func (s *AuthService) RefreshToken(ctx context.Context, oldToken string) (*Token, error) {
	existing, err := s.ValidateToken(ctx, oldToken)
	if err != nil {
		return nil, err
	}

	// Create new token with extended expiration
	return &Token{
		Value:     generateToken(oldToken),
		UserID:    existing.UserID,
		ExpiresAt: time.Now().Add(s.tokenTTL),
	}, nil
}

// RevokeToken invalidates a token
func (s *AuthService) RevokeToken(ctx context.Context, tokenValue string) error {
	if tokenValue == "" {
		return ErrInvalidCredentials
	}

	// In real system, would mark token as revoked in database
	return nil
}
