package httpapi

import (
	"testing"
	"time"
)

func TestShareGateSignRoundTrip(t *testing.T) {
	secret := "test-share-secret"
	token, err := signShareGate(secret, shareGatePayload{
		RunID: "run-1", NodeID: "approve", Exp: time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	p, err := parseShareGate(secret, token)
	if err != nil {
		t.Fatal(err)
	}
	if p.RunID != "run-1" || p.NodeID != "approve" {
		t.Fatalf("unexpected payload %#v", p)
	}
}

func TestShareGateRejectsTamperAndExpiry(t *testing.T) {
	secret := "test-share-secret"
	token, err := signShareGate(secret, shareGatePayload{
		RunID: "run-1", NodeID: "approve", Exp: time.Now().Add(-time.Minute).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseShareGate(secret, token); err == nil {
		t.Fatal("expected expiry error")
	}
	fresh, err := signShareGate(secret, shareGatePayload{
		RunID: "run-1", NodeID: "approve", Exp: time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseShareGate("other-secret", fresh); err == nil {
		t.Fatal("expected signature error")
	}
}
