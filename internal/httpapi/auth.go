package httpapi

import (
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Tencent/LoopForge/internal/auth"
	"github.com/Tencent/LoopForge/internal/domain"
	"github.com/gin-gonic/gin"
)

const authCookieName = "loopforge_user"

// authSessionTTL bounds both the cookie MaxAge and the AuthTime check in
// requestUser. They must share one duration, otherwise a renewed cookie can
// outlive its AuthTime and leave the browser "signed in" but rejected by
// every protected endpoint.
const authSessionTTL = 7 * 24 * time.Hour

// authSessionExpired reports whether a signed session exceeded authSessionTTL.
// A zero AuthTime (proxy identities) never expires here.
func authSessionExpired(user *auth.User) bool {
	return user != nil && !user.AuthTime.IsZero() && time.Since(user.AuthTime) > authSessionTTL
}

type authHandlers struct {
	*deps
}

// Trusted reverse-proxy identity (oauth2-proxy, Authelia, Pomerium, ...).
// Only honored when explicitly enabled and the direct peer is in
// LOOPFORGE_TRUSTED_PROXY_CIDRS (default loopback), so a client that reaches
// the server directly cannot spoof the header.
func trustGatewayIdentityHeaders() bool {
	enabled, err := strconv.ParseBool(strings.TrimSpace(os.Getenv("LOOPFORGE_TRUST_IDENTITY_HEADERS")))
	return err == nil && enabled
}

func identityHeaderName() string {
	if name := strings.TrimSpace(os.Getenv("LOOPFORGE_IDENTITY_HEADER")); name != "" {
		return name
	}
	return "X-Forwarded-User"
}

func trustedProxyPeer(remote string) bool {
	host := remote
	if h, _, err := net.SplitHostPort(remote); err == nil {
		host = h
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	raw := strings.TrimSpace(os.Getenv("LOOPFORGE_TRUSTED_PROXY_CIDRS"))
	if raw == "" {
		return ip.IsLoopback()
	}
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' || r == ';' }) {
		if _, cidr, err := net.ParseCIDR(strings.TrimSpace(part)); err == nil && cidr.Contains(ip) {
			return true
		}
	}
	return false
}

func gatewayLoginName(r *http.Request) string {
	if !trustGatewayIdentityHeaders() || !trustedProxyPeer(r.RemoteAddr) {
		return ""
	}
	return strings.TrimSpace(r.Header.Get(identityHeaderName()))
}

func secureRequest(r *http.Request) bool {
	if r.TLS != nil || strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https") {
		return true
	}
	enabled, err := strconv.ParseBool(strings.TrimSpace(os.Getenv("LOOPFORGE_SECURE_COOKIES")))
	return err == nil && enabled
}

func (h *authHandlers) currentPayload(user *auth.User) map[string]any {
	return map[string]any{
		"user":         h.annotateUser(user),
		"authRequired": authRequired(),
		"loginMode":    "password",
		"proxyAuth":    trustGatewayIdentityHeaders(),
		// The login form shows the fixed demo account only while it applies.
		"demoLogin": demoDefaultLogin(),
	}
}

func demoDefaultLogin() bool {
	demo, _ := strconv.ParseBool(strings.TrimSpace(os.Getenv("LOOPFORGE_DEMO_MODE")))
	return demo && strings.TrimSpace(os.Getenv("LOOPFORGE_ADMIN_PASSWORD")) == ""
}

func (h *authHandlers) authCurrent(c *gin.Context) {
	r := c.Request
	if login := gatewayLoginName(r); login != "" {
		user := &auth.User{LoginName: login, DisplayName: login, AuthTime: time.Now().UTC()}
		if h.repo != nil {
			if existing, ok := h.repo.UserByLoginName(login); ok {
				user.ID = existing.ID
				if existing.DisplayName != "" {
					user.DisplayName = existing.DisplayName
				}
			} else if err := h.repo.PutUser(user.ToDomain()); err != nil {
				log.Printf("[auth] persist proxy user %s failed: %v", login, err)
			} else if saved, ok := h.repo.UserByLoginName(login); ok {
				user.ID = saved.ID
			}
		}
		if token, err := h.getAuthClient().SignUser(user); err == nil {
			h.setAuthCookie(c.Writer, r, token)
		}
		c.JSON(http.StatusOK, h.currentPayload(user))
		return
	}
	if user, ok := h.requestUser(r, false); ok {
		c.JSON(http.StatusOK, h.currentPayload(user))
		return
	}
	if cookie, err := r.Cookie(authCookieName); err == nil && cookie.Value != "" {
		h.clearAuthCookie(c.Writer, r)
	}
	c.JSON(http.StatusOK, h.currentPayload(auth.FallbackGuestUser()))
}

// loginLimiter slows password guessing: after 5 failures a login name is
// locked for a minute.
var loginLimiter = struct {
	sync.Mutex
	failures map[string][]time.Time
}{failures: map[string][]time.Time{}}

func loginLocked(login string) bool {
	loginLimiter.Lock()
	defer loginLimiter.Unlock()
	cutoff := time.Now().Add(-time.Minute)
	kept := loginLimiter.failures[login][:0]
	for _, at := range loginLimiter.failures[login] {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	loginLimiter.failures[login] = kept
	return len(kept) >= 5
}

func recordLoginFailure(login string) {
	loginLimiter.Lock()
	defer loginLimiter.Unlock()
	loginLimiter.failures[login] = append(loginLimiter.failures[login], time.Now())
}

func (h *authHandlers) authLogin(c *gin.Context) {
	var req struct {
		LoginName string `json:"loginName"`
		Password  string `json:"password"`
	}
	if !decode(c, &req) {
		return
	}
	login := strings.ToLower(strings.TrimSpace(req.LoginName))
	if login == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, map[string]string{"error": "loginName and password are required"})
		return
	}
	if loginLocked(login) {
		c.JSON(http.StatusTooManyRequests, map[string]string{"error": "too many failed attempts; try again in a minute"})
		return
	}
	stored, ok := h.repo.UserByLoginName(login)
	if !ok || !auth.CheckPassword(stored.PasswordHash, req.Password) {
		recordLoginFailure(login)
		c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid login name or password"})
		return
	}
	user := domainUserToAuth(stored)
	token, err := h.getAuthClient().SignUser(user)
	if err != nil {
		respond(c, err)
		return
	}
	h.setAuthCookie(c.Writer, c.Request, token)
	c.JSON(http.StatusOK, h.currentPayload(user))
}

func (h *authHandlers) authPassword(c *gin.Context) {
	user, ok := h.requestUser(c.Request, false)
	if !ok {
		c.JSON(http.StatusUnauthorized, map[string]string{"error": "sign in first"})
		return
	}
	var req struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if !decode(c, &req) {
		return
	}
	stored, ok := h.repo.User(user.ID)
	if !ok {
		c.JSON(http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}
	if stored.PasswordHash != "" && !auth.CheckPassword(stored.PasswordHash, req.CurrentPassword) {
		c.JSON(http.StatusForbidden, map[string]string{"error": "current password is incorrect"})
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := h.repo.SetUserPassword(stored.ID, hash); err != nil {
		respond(c, err)
		return
	}
	c.JSON(http.StatusOK, map[string]bool{"ok": true})
}

func (h *authHandlers) authLogout(c *gin.Context) {
	h.clearAuthCookie(c.Writer, c.Request)
	c.JSON(http.StatusOK, map[string]any{"user": auth.FallbackGuestUser()})
}

// createUser lets a platform admin add a local account or reset its password.
func (h *authHandlers) createUser(c *gin.Context) {
	if !h.isPlatformAdmin(c.Request) {
		c.JSON(http.StatusForbidden, map[string]string{"error": "only platform admins can manage accounts"})
		return
	}
	var req struct {
		LoginName   string `json:"loginName"`
		DisplayName string `json:"displayName"`
		Password    string `json:"password"`
		Admin       bool   `json:"admin"`
	}
	if !decode(c, &req) {
		return
	}
	user, err := EnsureLocalUser(h.repo, req.LoginName, req.DisplayName, req.Password, req.Admin)
	if err != nil {
		c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, domain.PublicUser(user))
}

// LocalUserStore is the subset of the store needed to manage local accounts.
type LocalUserStore interface {
	UserByLoginName(string) (domain.User, bool)
	PutUser(domain.User) error
	SetUserPassword(int64, string) error
}

// EnsureLocalUser creates or updates a password account. It is also used by
// the server entrypoint to bootstrap the first admin.
func EnsureLocalUser(st LocalUserStore, loginName, displayName, password string, admin bool) (domain.User, error) {
	login := strings.ToLower(strings.TrimSpace(loginName))
	if login == "" || strings.ContainsAny(login, " \t\r\n/\\") || len(login) > 64 {
		return domain.User{}, errors.New("loginName must be 1-64 characters without spaces or slashes")
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return domain.User{}, err
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		displayName = login
	}
	v := domain.User{LoginName: login, DisplayName: displayName}
	if admin {
		v.PlatformRole = domain.PlatformRoleAdmin
	}
	if err := st.PutUser(v); err != nil {
		return domain.User{}, err
	}
	saved, ok := st.UserByLoginName(login)
	if !ok {
		return domain.User{}, errors.New("user missing after save")
	}
	if err := st.SetUserPassword(saved.ID, hash); err != nil {
		return domain.User{}, err
	}
	saved.PasswordHash = hash
	return saved, nil
}

func (h *authHandlers) cookie(r *http.Request, value string, maxAge int) *http.Cookie {
	cookie := &http.Cookie{
		Name:     authCookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secureRequest(r),
		MaxAge:   maxAge,
	}
	if maxAge < 0 {
		cookie.Expires = time.Unix(0, 0)
	}
	host := r.Host
	if hh, _, err := net.SplitHostPort(host); err == nil {
		host = hh
	}
	if domainName := h.getAuthClient().Config().CookieDomain; domainName != "" && (host == domainName || strings.HasSuffix(host, "."+domainName)) {
		cookie.Domain = domainName
	}
	return cookie
}

func (h *authHandlers) setAuthCookie(w http.ResponseWriter, r *http.Request, signedToken string) {
	http.SetCookie(w, h.cookie(r, signedToken, int(authSessionTTL.Seconds())))
}

func (h *authHandlers) clearAuthCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, h.cookie(r, "", -1))
}
