// Package auth signs browser sessions and verifies local account passwords.
//
// LoopForge ships two interactive identity sources:
//   - local accounts (login name + bcrypt password) stored in the users table;
//   - an optional trusted reverse proxy that injects the login name into a
//     request header (for SSO gateways such as oauth2-proxy or Authelia).
//
// Both produce the same HMAC-signed session cookie. API and MCP clients use
// personal API tokens instead (see httpapi/auth_tokens.go).
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Config holds session signing settings.
type Config struct {
	// SessionSecret signs session cookies. It must be stable across restarts
	// and identical on every API instance.
	SessionSecret string
	// CookieDomain optionally widens the session cookie to a parent domain.
	CookieDomain string
}

// Client signs and parses session tokens.
type Client struct {
	cfg Config
}

// NewClient creates a Client. An empty SessionSecret is replaced by a random
// per-process secret, which logs everyone out on restart.
func NewClient(cfg Config) *Client {
	cfg.SessionSecret = strings.TrimSpace(cfg.SessionSecret)
	if cfg.SessionSecret == "" {
		cfg.SessionSecret = RandomSecret()
		log.Printf("[auth] LOOPFORGE_SESSION_SECRET is not set; using a random per-process secret (sessions end on restart)")
	}
	cfg.CookieDomain = strings.TrimSpace(cfg.CookieDomain)
	return &Client{cfg: cfg}
}

// NewClientFromEnv reads LOOPFORGE_SESSION_SECRET and LOOPFORGE_COOKIE_DOMAIN.
func NewClientFromEnv() *Client {
	return NewClient(Config{
		SessionSecret: os.Getenv("LOOPFORGE_SESSION_SECRET"),
		CookieDomain:  os.Getenv("LOOPFORGE_COOKIE_DOMAIN"),
	})
}

// Config returns a copy of the client configuration.
func (c *Client) Config() Config {
	return c.cfg
}

// SignUser signs u with the configured session secret.
func (c *Client) SignUser(u *User) (string, error) {
	return SignUser(u, c.cfg.SessionSecret)
}

// ParseSignedUser verifies and decodes a token produced by SignUser.
func (c *Client) ParseSignedUser(tokenStr string) (*User, error) {
	return ParseSignedUser(tokenStr, c.cfg.SessionSecret)
}

// FallbackGuestUser is the identity of an unauthenticated browser.
func FallbackGuestUser() *User {
	return &User{
		LoginName:   "guest",
		DisplayName: "Guest",
		IsGuest:     true,
		AuthTime:    time.Now().UTC(),
	}
}

// RandomSecret returns 32 random bytes, hex encoded.
func RandomSecret() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

// MinPasswordLength is enforced when a password is set or changed.
const MinPasswordLength = 8

var ErrWeakPassword = errors.New("password must be at least 8 characters")

// HashPassword returns a bcrypt hash of password.
func HashPassword(password string) (string, error) {
	if len(password) < MinPasswordLength {
		return "", ErrWeakPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword reports whether password matches hash. An empty hash never
// matches, so accounts without a password cannot sign in with one.
func CheckPassword(hash, password string) bool {
	if hash == "" || password == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
