package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tencent/LoopForge/internal/auth"
	"github.com/Tencent/LoopForge/internal/domain"
	"github.com/Tencent/LoopForge/internal/engine"
	"github.com/Tencent/LoopForge/internal/repo"
	"github.com/Tencent/LoopForge/internal/runtime"
	"github.com/Tencent/LoopForge/internal/tracker"
)

func TestVerifyCodeBuddyKeySwitchesToTheWorkingSite(t *testing.T) {
	t.Setenv("LOOPFORGE_SESSION_SECRET", "codebuddy-verify-secret")
	oldBin, oldProbe := codeBuddyVerifyBin, codeBuddyProbe
	t.Cleanup(func() { codeBuddyVerifyBin, codeBuddyProbe = oldBin, oldProbe })
	codeBuddyVerifyBin = func() (string, error) { return "/bin/codebuddy", nil }
	var tried []string
	codeBuddyProbe = func(_ context.Context, _, key, environment string) (bool, string) {
		tried = append(tried, environment)
		return key == "ck_china" && environment == "internal", "401 Authentication required"
	}

	s, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	h := New(s, engine.New(s, runtime.NewManager(filepath.Join(t.TempDir(), "ws"))), tracker.NewService(), "", "")
	if err := s.PutUser(domain.User{LoginName: "dora", DisplayName: "Dora"}); err != nil {
		t.Fatal(err)
	}
	dora, _ := s.UserByLoginName("dora")
	if _, err := s.UpdateUserCredentials(dora.ID, repo.UserCredentialUpdate{CodeBuddyAPIKey: "ck_china"}); err != nil {
		t.Fatal(err)
	}
	token, err := auth.NewClientFromEnv().SignUser(&auth.User{ID: dora.ID, LoginName: dora.LoginName, AuthTime: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/users/%d/codebuddy/verify", dora.ID), nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: token})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var result codeBuddyVerifyResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !result.OK || !result.Switched || result.Environment != "internal" {
		t.Fatalf("expected a switch to the China site, got %+v (tried %v)", result, tried)
	}
	stored, _ := s.User(dora.ID)
	if stored.CodeBuddyInternetEnvironment != "internal" {
		t.Fatalf("working environment must be saved, got %q", stored.CodeBuddyInternetEnvironment)
	}
}
