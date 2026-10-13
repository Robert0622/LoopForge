package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Tencent/LoopForge/internal/domain"
)

func newSignupServer(t *testing.T) (http.Handler, *authHandlers) {
	t.Helper()
	g, s := newTestAuthServer(t, testAuthClient())
	h := &authHandlers{deps: &deps{repo: s, authClient: testAuthClient()}}
	g.POST("/api/auth/setup", h.authSetup)
	g.POST("/api/auth/register", h.authRegister)
	g.GET("/api/auth/settings", h.authSettings)
	g.PUT("/api/auth/settings", h.authSettings)
	return g, h
}

func TestFirstVisitorCreatesTheAdminOnce(t *testing.T) {
	t.Setenv("LOOPFORGE_SIGNUP", "")
	g, h := newSignupServer(t)
	if err := h.repo.PutProject(domain.Project{ID: "default", Name: "Default", Key: "DEF"}); err != nil {
		t.Fatal(err)
	}
	if body := doJSON(t, g, "GET", "/api/auth/current", "").Body.String(); !strings.Contains(body, `"needsSetup":true`) {
		t.Fatalf("fresh instance must ask for setup: %s", body)
	}
	if rec := doJSON(t, g, "POST", "/api/auth/register", `{"loginName":"bob","password":"bob-password"}`); rec.Code != http.StatusConflict {
		t.Fatalf("register before setup = %d, want 409", rec.Code)
	}
	rec := doJSON(t, g, "POST", "/api/auth/setup", `{"loginName":"Alice","displayName":"Alice","password":"alice-password"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup = %d %s", rec.Code, rec.Body.String())
	}
	sessionCookie(t, rec)
	alice, ok := h.repo.UserByLoginName("alice")
	if !ok || !h.repo.IsPlatformAdmin("alice") {
		t.Fatalf("setup account must be a platform admin: %+v", alice)
	}
	if project, _ := h.repo.Project("default"); project.OwnerID != alice.ID || len(project.Members) != 1 {
		t.Fatalf("seeded project must belong to the first admin: %+v", project)
	}
	if rec := doJSON(t, g, "POST", "/api/auth/setup", `{"loginName":"mallory","password":"mallory-pass"}`); rec.Code != http.StatusConflict {
		t.Fatalf("second setup = %d, want 409", rec.Code)
	}
	if body := doJSON(t, g, "GET", "/api/auth/current", "").Body.String(); !strings.Contains(body, `"needsSetup":false`) {
		t.Fatalf("setup must be done: %s", body)
	}
}

func TestRegistrationFollowsTheSignupSwitch(t *testing.T) {
	t.Setenv("LOOPFORGE_SIGNUP", "")
	g, h := newSignupServer(t)
	admin := sessionCookie(t, doJSON(t, g, "POST", "/api/auth/setup", `{"loginName":"admin","password":"admin-password"}`))

	rec := doJSON(t, g, "POST", "/api/auth/register", `{"loginName":"bob","displayName":"Bob","password":"bob-password"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("open sign-up register = %d %s", rec.Code, rec.Body.String())
	}
	bob := sessionCookie(t, rec)
	if h.repo.IsPlatformAdmin("bob") {
		t.Fatal("a registered account must not be an admin")
	}
	if rec := doJSON(t, g, "POST", "/api/auth/register", `{"loginName":"BOB","password":"other-password"}`); rec.Code != http.StatusConflict {
		t.Fatalf("taking an existing login = %d, want 409", rec.Code)
	}
	if rec := doJSON(t, g, "PUT", "/api/auth/settings", `{"signupOpen":false}`, bob); rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin closing sign-up = %d, want 403", rec.Code)
	}
	if rec := doJSON(t, g, "PUT", "/api/auth/settings", `{"signupOpen":false}`, admin); rec.Code != http.StatusOK {
		t.Fatalf("admin closing sign-up = %d %s", rec.Code, rec.Body.String())
	}
	if rec := doJSON(t, g, "POST", "/api/auth/register", `{"loginName":"carol","password":"carol-password"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("closed sign-up register = %d, want 403", rec.Code)
	}
	if body := doJSON(t, g, "GET", "/api/auth/current", "").Body.String(); !strings.Contains(body, `"signupOpen":false`) {
		t.Fatalf("login page must learn sign-up is closed: %s", body)
	}
}

func TestSignupDefaultsFromEnvironment(t *testing.T) {
	t.Setenv("LOOPFORGE_SIGNUP", "closed")
	_, h := newSignupServer(t)
	if h.repo.AuthSettings().SignupOpen {
		t.Fatal("LOOPFORGE_SIGNUP=closed must start with sign-up closed")
	}
}
