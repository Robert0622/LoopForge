package auth

import (
	"testing"
	"time"
)

func TestSignAndParseSignedUser(t *testing.T) {
	c := NewClient(Config{SessionSecret: "test-secret"})
	u := &User{ID: 7, LoginName: "alice", DisplayName: "Alice", AuthTime: time.Now().UTC()}
	token, err := c.SignUser(u)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.ParseSignedUser(token)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != 7 || got.LoginName != "alice" || got.DisplayName != "Alice" {
		t.Fatalf("unexpected user %+v", got)
	}
	if _, err := NewClient(Config{SessionSecret: "other"}).ParseSignedUser(token); err == nil {
		t.Fatal("token signed with another secret must be rejected")
	}
	if _, err := c.ParseSignedUser(token + "0"); err == nil {
		t.Fatal("tampered token must be rejected")
	}
}

func TestPasswordHashing(t *testing.T) {
	if _, err := HashPassword("short"); err != ErrWeakPassword {
		t.Fatalf("expected weak password error, got %v", err)
	}
	hash, err := HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(hash, "correct horse") {
		t.Fatal("expected password to match")
	}
	if CheckPassword(hash, "wrong horse") || CheckPassword("", "correct horse") {
		t.Fatal("unexpected password match")
	}
}

func TestEmptySecretIsRandomized(t *testing.T) {
	a, b := NewClient(Config{}), NewClient(Config{})
	if a.Config().SessionSecret == "" || a.Config().SessionSecret == b.Config().SessionSecret {
		t.Fatal("expected distinct random secrets")
	}
}
