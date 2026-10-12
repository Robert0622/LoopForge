package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tencent/LoopForge/internal/domain"
)

// User is the identity carried by a browser session.
type User struct {
	ID              int64     `json:"id"`
	LoginName       string    `json:"loginName"`
	DisplayName     string    `json:"displayName"`
	IsGuest         bool      `json:"isGuest"`
	IsPlatformAdmin bool      `json:"isPlatformAdmin,omitempty"`
	AuthTime        time.Time `json:"authTime"`
}

// ToDomain converts the session identity to a persisted user.
func (u *User) ToDomain() domain.User {
	if u == nil {
		return domain.User{}
	}
	return domain.User{
		ID:          u.ID,
		LoginName:   u.LoginName,
		DisplayName: u.DisplayName,
	}
}

// SignUser encodes u and appends an HMAC-SHA256 signature.
func SignUser(u *User, secret string) (string, error) {
	if u == nil {
		return "", errors.New("user is nil")
	}
	payload, err := json.Marshal(u)
	if err != nil {
		return "", err
	}
	b64Payload := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(b64Payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("%s.%s", b64Payload, sig), nil
}

// ParseSignedUser verifies and decodes a token produced by SignUser.
func ParseSignedUser(tokenStr, secret string) (*User, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 2 {
		return nil, errors.New("invalid token format")
	}
	b64Payload, sig := parts[0], parts[1]
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(b64Payload))
	expectedSig := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
		return nil, errors.New("invalid token signature")
	}

	raw, err := base64.RawURLEncoding.DecodeString(b64Payload)
	if err != nil {
		return nil, err
	}
	var u User
	if err := json.Unmarshal(raw, &u); err != nil {
		return nil, err
	}
	return &u, nil
}
