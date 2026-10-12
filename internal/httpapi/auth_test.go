package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tencent/LoopForge/internal/auth"
	"github.com/Tencent/LoopForge/internal/domain"
	"github.com/Tencent/LoopForge/internal/engine"
	"github.com/Tencent/LoopForge/internal/repo"
	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newTestAuthServer(t *testing.T, authClient *auth.Client) (*gin.Engine, *repo.Store) {
	t.Helper()
	s, err := repo.OpenTest()
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	e := engine.New(s, nil)
	srv := &Server{
		deps: &deps{repo: s, engine: e, authClient: authClient},
	}
	srv.auth = &authHandlers{deps: srv.deps}
	srv.users = &usersHandlers{deps: srv.deps}

	g := gin.New()
	g.GET("/api/auth/current", srv.auth.authCurrent)
	g.POST("/api/auth/login", srv.auth.authLogin)
	g.POST("/api/auth/password", srv.auth.authPassword)
	g.POST("/api/auth/logout", srv.auth.authLogout)
	g.POST("/api/users", srv.auth.createUser)
	g.GET("/api/users", srv.users.users)
	return g, s
}

func testAuthClient() *auth.Client {
	return auth.NewClient(auth.Config{SessionSecret: "test-secret-please-change"})
}

func doJSON(t *testing.T, h http.Handler, method, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == authCookieName && c.Value != "" {
			return c
		}
	}
	t.Fatalf("no session cookie in response: %v", rec.Result().Header)
	return nil
}

func currentUser(t *testing.T, rec *httptest.ResponseRecorder) auth.User {
	t.Helper()
	var body struct {
		User auth.User `json:"user"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, rec.Body.String())
	}
	return body.User
}

func TestAuthCurrent_NoCookieIsGuest(t *testing.T) {
	handler, _ := newTestAuthServer(t, testAuthClient())
	rec := doJSON(t, handler, http.MethodGet, "/api/auth/current", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if u := currentUser(t, rec); !u.IsGuest {
		t.Fatalf("expected guest, got %+v", u)
	}
	if !strings.Contains(rec.Body.String(), `"loginMode":"password"`) {
		t.Fatalf("current payload must advertise password login: %s", rec.Body.String())
	}
}

func TestPasswordLoginFlow(t *testing.T) {
	handler, st := newTestAuthServer(t, testAuthClient())
	if _, err := EnsureLocalUser(st, "Alice", "Alice", "correct-horse", false); err != nil {
		t.Fatal(err)
	}

	if rec := doJSON(t, handler, http.MethodPost, "/api/auth/login", `{"loginName":"alice","password":"wrong-pass"}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad password: %d %s", rec.Code, rec.Body.String())
	}
	rec := doJSON(t, handler, http.MethodPost, "/api/auth/login", `{"loginName":"ALICE","password":"correct-horse"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("login: %d %s", rec.Code, rec.Body.String())
	}
	cookie := sessionCookie(t, rec)
	if !cookie.HttpOnly {
		t.Fatal("session cookie must be HttpOnly")
	}
	if u := currentUser(t, rec); u.LoginName != "alice" || u.IsGuest {
		t.Fatalf("login user = %+v", u)
	}

	rec = doJSON(t, handler, http.MethodGet, "/api/auth/current", "", cookie)
	if u := currentUser(t, rec); u.LoginName != "alice" || u.ID <= 0 {
		t.Fatalf("current with cookie = %+v", u)
	}

	rec = doJSON(t, handler, http.MethodPost, "/api/auth/password", `{"currentPassword":"nope","newPassword":"battery-staple"}`, cookie)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("wrong current password: %d", rec.Code)
	}
	rec = doJSON(t, handler, http.MethodPost, "/api/auth/password", `{"currentPassword":"correct-horse","newPassword":"short"}`, cookie)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("weak password must be rejected: %d", rec.Code)
	}
	rec = doJSON(t, handler, http.MethodPost, "/api/auth/password", `{"currentPassword":"correct-horse","newPassword":"battery-staple"}`, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("change password: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doJSON(t, handler, http.MethodPost, "/api/auth/login", `{"loginName":"alice","password":"battery-staple"}`); rec.Code != http.StatusOK {
		t.Fatalf("login with new password: %d", rec.Code)
	}

	rec = doJSON(t, handler, http.MethodPost, "/api/auth/logout", "", cookie)
	for _, c := range rec.Result().Cookies() {
		if c.Name == authCookieName && c.MaxAge >= 0 {
			t.Fatalf("logout must expire the cookie: %+v", c)
		}
	}
}

func TestLoginLocksAfterRepeatedFailures(t *testing.T) {
	handler, st := newTestAuthServer(t, testAuthClient())
	if _, err := EnsureLocalUser(st, "locked-user", "", "correct-horse", false); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		doJSON(t, handler, http.MethodPost, "/api/auth/login", `{"loginName":"locked-user","password":"bad-password"}`)
	}
	rec := doJSON(t, handler, http.MethodPost, "/api/auth/login", `{"loginName":"locked-user","password":"correct-horse"}`)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected lockout, got %d", rec.Code)
	}
}

func TestAuthCurrent_TamperedCookieIsCleared(t *testing.T) {
	handler, _ := newTestAuthServer(t, testAuthClient())
	rec := doJSON(t, handler, http.MethodGet, "/api/auth/current", "", &http.Cookie{Name: authCookieName, Value: "forged.value"})
	if u := currentUser(t, rec); !u.IsGuest {
		t.Fatalf("tampered cookie must not authenticate: %+v", u)
	}
}

func TestAuthCurrent_CookieFromOtherSecretRejected(t *testing.T) {
	handler, st := newTestAuthServer(t, testAuthClient())
	if err := st.PutUser(domain.User{LoginName: "bob", DisplayName: "Bob"}); err != nil {
		t.Fatal(err)
	}
	bob, _ := st.UserByLoginName("bob")
	token, err := auth.NewClient(auth.Config{SessionSecret: "another-secret"}).SignUser(&auth.User{ID: bob.ID, LoginName: "bob"})
	if err != nil {
		t.Fatal(err)
	}
	rec := doJSON(t, handler, http.MethodGet, "/api/auth/current", "", &http.Cookie{Name: authCookieName, Value: token})
	if u := currentUser(t, rec); !u.IsGuest {
		t.Fatalf("cookie signed by another secret must be rejected: %+v", u)
	}
}

func TestAuthCurrent_TrustedProxyHeader(t *testing.T) {
	t.Setenv("LOOPFORGE_TRUST_IDENTITY_HEADERS", "true")
	handler, st := newTestAuthServer(t, testAuthClient())

	req := httptest.NewRequest(http.MethodGet, "/api/auth/current", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	req.Header.Set("X-Forwarded-User", "carol")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if u := currentUser(t, rec); u.LoginName != "carol" || u.ID <= 0 {
		t.Fatalf("proxy identity = %+v", u)
	}
	if _, ok := st.UserByLoginName("carol"); !ok {
		t.Fatal("proxy identity must be persisted")
	}

	req = httptest.NewRequest(http.MethodGet, "/api/auth/current", nil)
	req.RemoteAddr = "203.0.113.9:5555"
	req.Header.Set("X-Forwarded-User", "mallory")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if u := currentUser(t, rec); !u.IsGuest {
		t.Fatalf("header from an untrusted peer must be ignored: %+v", u)
	}
}

func TestAuthCurrent_ProxyHeaderIgnoredWhenDisabled(t *testing.T) {
	handler, _ := newTestAuthServer(t, testAuthClient())
	req := httptest.NewRequest(http.MethodGet, "/api/auth/current", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	req.Header.Set("X-Forwarded-User", "carol")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if u := currentUser(t, rec); !u.IsGuest {
		t.Fatalf("identity header must be ignored unless enabled: %+v", u)
	}
}

func TestAuthCookieSecureBehindHTTPSProxy(t *testing.T) {
	handler, st := newTestAuthServer(t, testAuthClient())
	if _, err := EnsureLocalUser(st, "dave", "", "correct-horse", false); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"loginName":"dave","password":"correct-horse"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if c := sessionCookie(t, rec); !c.Secure {
		t.Fatal("cookie must be Secure behind an HTTPS proxy")
	}
}

func TestCreateUserRequiresPlatformAdmin(t *testing.T) {
	handler, st := newTestAuthServer(t, testAuthClient())
	if _, err := EnsureLocalUser(st, "root", "", "correct-horse", true); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureLocalUser(st, "eve", "", "correct-horse", false); err != nil {
		t.Fatal(err)
	}
	login := func(name string) *http.Cookie {
		rec := doJSON(t, handler, http.MethodPost, "/api/auth/login", `{"loginName":"`+name+`","password":"correct-horse"}`)
		return sessionCookie(t, rec)
	}
	body := `{"loginName":"frank","displayName":"Frank","password":"frank-password"}`
	if rec := doJSON(t, handler, http.MethodPost, "/api/users", body, login("eve")); rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin create user: %d", rec.Code)
	}
	rec := doJSON(t, handler, http.MethodPost, "/api/users", body, login("root"))
	if rec.Code != http.StatusOK {
		t.Fatalf("admin create user: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "passwordHash") || strings.Contains(rec.Body.String(), "$2a$") {
		t.Fatalf("password hash leaked: %s", rec.Body.String())
	}
	if rec := doJSON(t, handler, http.MethodPost, "/api/auth/login", `{"loginName":"frank","password":"frank-password"}`); rec.Code != http.StatusOK {
		t.Fatalf("created user login: %d", rec.Code)
	}
}
