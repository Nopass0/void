// Package auth provides JWT-based authentication for VoidDB.
// Users are stored in-memory and can be persisted via the Store interface.
// Tokens are signed with HMAC-SHA256 using the configured JWTSecret.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Role defines what operations a user may perform.
type Role string

const (
	// RoleAdmin can do everything including manage users.
	RoleAdmin Role = "admin"
	// RoleReadWrite can read and write data but not manage users.
	RoleReadWrite Role = "readwrite"
	// RoleReadOnly can only read data.
	RoleReadOnly Role = "readonly"
)

// User is a VoidDB user account.
type User struct {
	// ID is the unique user identifier (username).
	ID string `json:"id"`
	// PasswordHash is a hex-encoded iterated SHA-256 hash of the user's password.
	PasswordHash string `json:"password_hash,omitempty"`
	// Salt is a random 16-byte hex string mixed into the password hash.
	Salt string `json:"salt,omitempty"`
	// Role is the user's permission level.
	Role Role `json:"role"`
	// CreatedAt is the UTC creation timestamp (Unix seconds).
	CreatedAt int64 `json:"created_at"`
	// Databases is an optional allowlist of databases this user may access.
	// An empty slice means "all databases".
	Databases []string `json:"databases,omitempty"`
}

// Claims are the JWT payload fields.
type Claims struct {
	jwt.RegisteredClaims
	// UserID duplicates Subject for convenience.
	UserID string `json:"uid"`
	// Role carries the user's permission level so middleware can check without
	// a database round-trip.
	Role Role `json:"role"`
}

// TokenPair holds a short-lived access token and a longer-lived refresh token.
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	// ExpiresAt is the Unix timestamp when the access token expires.
	ExpiresAt int64 `json:"expires_at"`
}

// Service manages users and issues JWT tokens.
// It is safe for concurrent use by multiple goroutines.
type Service struct {
	mu            sync.RWMutex
	users         map[string]*User
	secret        []byte
	tokenExpiry   time.Duration
	refreshExpiry time.Duration
}

// NewService creates a new auth Service.
// jwtSecret must be a non-empty secret key (at least 32 characters recommended).
func NewService(jwtSecret string, tokenExpiry, refreshExpiry time.Duration) *Service {
	return &Service{
		users:         make(map[string]*User),
		secret:        []byte(jwtSecret),
		tokenExpiry:   tokenExpiry,
		refreshExpiry: refreshExpiry,
	}
}

// Bootstrap creates the initial admin user if no users exist yet.
// It is idempotent – calling it multiple times is safe.
func (s *Service) Bootstrap(adminPassword string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.users) > 0 {
		return nil
	}
	u, err := buildUser("admin", adminPassword, RoleAdmin)
	if err != nil {
		return err
	}
	s.users["admin"] = u
	return nil
}

// CreateUser adds a new user account.
func (s *Service) CreateUser(id, password string, role Role) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.users[id]; exists {
		return fmt.Errorf("auth: user %q already exists", id)
	}
	u, err := buildUser(id, password, role)
	if err != nil {
		return err
	}
	s.users[id] = u
	return nil
}

// DeleteUser removes a user by ID. Returns an error if the user is not found.
func (s *Service) DeleteUser(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[id]; !ok {
		return fmt.Errorf("auth: user %q not found", id)
	}
	delete(s.users, id)
	return nil
}

// ListUsers returns all user accounts stripped of credential fields.
func (s *Service) ListUsers() []*User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*User, 0, len(s.users))
	for _, u := range s.users {
		safe := *u
		safe.PasswordHash = ""
		safe.Salt = ""
		out = append(out, &safe)
	}
	return out
}

// GetUser returns a user by ID without credential fields.
func (s *Service) GetUser(id string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[id]
	if !ok {
		return nil, fmt.Errorf("auth: user %q not found", id)
	}
	safe := *u
	safe.PasswordHash = ""
	safe.Salt = ""
	return &safe, nil
}

// ChangePassword updates the hashed password for the given user.
func (s *Service) ChangePassword(id, newPassword string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return fmt.Errorf("auth: user %q not found", id)
	}
	salt, hash, err := generateSaltAndHash(newPassword)
	if err != nil {
		return err
	}
	u.Salt = salt
	u.PasswordHash = hash
	return nil
}

// Login verifies credentials and returns a TokenPair on success.
func (s *Service) Login(id, password string) (*TokenPair, error) {
	s.mu.RLock()
	u, ok := s.users[id]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("auth: invalid credentials")
	}
	expected, err := hashPassword(password, u.Salt)
	if err != nil {
		return nil, err
	}
	if subtle.ConstantTimeCompare([]byte(expected), []byte(u.PasswordHash)) != 1 {
		return nil, fmt.Errorf("auth: invalid credentials")
	}
	return s.issueTokenPair(u)
}

// RefreshToken validates a refresh token and issues a fresh TokenPair.
func (s *Service) RefreshToken(refreshTokenStr string) (*TokenPair, error) {
	claims, err := s.validateToken(refreshTokenStr)
	if err != nil {
		return nil, fmt.Errorf("auth: invalid refresh token: %w", err)
	}
	s.mu.RLock()
	u, ok := s.users[claims.UserID]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("auth: user no longer exists")
	}
	return s.issueTokenPair(u)
}

// ValidateAccessToken parses and validates an access token.
// Returns the Claims on success.
func (s *Service) ValidateAccessToken(tokenStr string) (*Claims, error) {
	return s.validateToken(tokenStr)
}

// --- internal helpers --------------------------------------------------------

// buildUser creates a User with a freshly generated salt and hashed password.
func buildUser(id, password string, role Role) (*User, error) {
	salt, hash, err := generateSaltAndHash(password)
	if err != nil {
		return nil, err
	}
	return &User{
		ID:           id,
		PasswordHash: hash,
		Salt:         salt,
		Role:         role,
		CreatedAt:    time.Now().UTC().Unix(),
	}, nil
}

// generateSaltAndHash creates a random 16-byte salt and returns (salt, hash).
func generateSaltAndHash(password string) (salt, hash string, err error) {
	saltBytes := make([]byte, 16)
	if _, err = rand.Read(saltBytes); err != nil {
		return "", "", fmt.Errorf("auth: generate salt: %w", err)
	}
	salt = hex.EncodeToString(saltBytes)
	hash, err = hashPassword(password, salt)
	return
}

// hashPassword returns the hex-encoded result of 10 000 rounds of SHA-256
// seeded with salt+password.  Not as strong as bcrypt/argon2 but extremely
// fast for our use case and avoids external dependencies.
func hashPassword(password, salt string) (string, error) {
	sum := sha256.Sum256([]byte(salt + password))
	b := sum[:]
	for i := 0; i < 9999; i++ {
		h := sha256.Sum256(b)
		b = h[:]
	}
	return hex.EncodeToString(b), nil
}

// issueTokenPair mints a new access + refresh JWT pair for the given user.
func (s *Service) issueTokenPair(u *User) (*TokenPair, error) {
	now := time.Now().UTC()
	accessExpiry := now.Add(s.tokenExpiry)

	accessClaims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   u.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(accessExpiry),
			Issuer:    "voiddb",
		},
		UserID: u.ID,
		Role:   u.Role,
	}
	at := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessStr, err := at.SignedString(s.secret)
	if err != nil {
		return nil, fmt.Errorf("auth: sign access token: %w", err)
	}

	refreshClaims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   u.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.refreshExpiry)),
			Issuer:    "voiddb-refresh",
		},
		UserID: u.ID,
		Role:   u.Role,
	}
	rt := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshStr, err := rt.SignedString(s.secret)
	if err != nil {
		return nil, fmt.Errorf("auth: sign refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessStr,
		RefreshToken: refreshStr,
		ExpiresAt:    accessExpiry.Unix(),
	}, nil
}

// validateToken parses and validates any VoidDB JWT (access or refresh).
func (s *Service) validateToken(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("auth: unexpected signing method %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, err
	}
	return claims, nil
}
