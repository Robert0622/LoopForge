package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tencent/LoopForge/internal/auth"
	"github.com/Tencent/LoopForge/internal/domain"
	"github.com/Tencent/LoopForge/internal/engine"
	"github.com/Tencent/LoopForge/internal/repo"
	"github.com/Tencent/LoopForge/internal/runtime"
	"github.com/Tencent/LoopForge/internal/tracker"
)

func TestDoctorReportsMissingKeysWithoutLeakingSecrets(t *testing.T) {
	t.Setenv("LOOPFORGE_SESSION_SECRET", "doctor-test-session-secret")
	repo.SetTemplateSandbox("local")
	t.Cleanup(func() { repo.SetTemplateSandbox("") })
	s, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	h := New(s, engine.New(s, runtime.NewManager(filepath.Join(t.TempDir(), "workspaces"))), tracker.NewService(), "", "")
	if err := s.PutUser(domain.User{LoginName: "bob", DisplayName: "Bob"}); err != nil {
		t.Fatal(err)
	}
	bob, _ := s.UserByLoginName("bob")

	get := func() doctorReport {
		t.Helper()
		token, err := auth.NewClientFromEnv().SignUser(&auth.User{ID: bob.ID, LoginName: bob.LoginName, AuthTime: time.Now().UTC()})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodGet, "/api/doctor", nil)
		req.AddCookie(&http.Cookie{Name: authCookieName, Value: token})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("doctor status=%d body=%s", rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "ck_doctor_secret") {
			t.Fatalf("doctor leaked a secret: %s", rec.Body.String())
		}
		var report doctorReport
		if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
			t.Fatal(err)
		}
		return report
	}
	find := func(report doctorReport, id string) doctorCheck {
		t.Helper()
		for _, check := range report.Checks {
			if check.ID == id {
				return check
			}
		}
		t.Fatalf("doctor check %q missing: %+v", id, report.Checks)
		return doctorCheck{}
	}

	before := get()
	if before.Sandbox != "local" || find(before, "sandbox").Status != "warn" {
		t.Fatalf("local sandbox should be reported as a warning: %+v", before)
	}
	if key := find(before, "model-key"); key.Status != "warn" || key.Action != "setup-wizard" {
		t.Fatalf("missing model key should point at the setup wizard: %+v", key)
	}

	if _, err := s.UpdateUserCredentials(bob.ID, repo.UserCredentialUpdate{CodeBuddyAPIKey: "ck_doctor_secret"}); err != nil {
		t.Fatal(err)
	}
	after := get()
	if key := find(after, "model-key"); key.Status != "ok" || !strings.Contains(key.Detail, "CodeBuddy") {
		t.Fatalf("configured key should be reported: %+v", key)
	}
}

func TestDoctorPrewarmRequiresPlatformAdmin(t *testing.T) {
	t.Setenv("LOOPFORGE_SESSION_SECRET", "doctor-test-session-secret")
	s, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	h := New(s, engine.New(s, runtime.NewManager(filepath.Join(t.TempDir(), "workspaces"))), tracker.NewService(), "", "")
	if err := s.PutUser(domain.User{LoginName: "carol", DisplayName: "Carol"}); err != nil {
		t.Fatal(err)
	}
	carol, _ := s.UserByLoginName("carol")
	token, err := auth.NewClientFromEnv().SignUser(&auth.User{ID: carol.ID, LoginName: carol.LoginName, AuthTime: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/doctor/prewarm-sandbox", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: token})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin prewarm status=%d body=%s", rec.Code, rec.Body.String())
	}
}
