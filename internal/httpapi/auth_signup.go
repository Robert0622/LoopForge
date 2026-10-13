package httpapi

import (
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/Tencent/LoopForge/internal/auth"
	"github.com/Tencent/LoopForge/internal/domain"
	"github.com/Tencent/LoopForge/internal/repo"
	"github.com/gin-gonic/gin"
)

// setupMu serialises first-admin creation so two visitors racing on a fresh
// instance cannot both become admin.
var setupMu sync.Mutex

type signupRequest struct {
	LoginName   string `json:"loginName"`
	DisplayName string `json:"displayName"`
	Password    string `json:"password"`
}

// authSetup creates the first account on an instance nobody can sign in to
// yet. That account is the platform admin and owns the projects seeded at
// startup.
func (h *authHandlers) authSetup(c *gin.Context) {
	var req signupRequest
	if !decode(c, &req) {
		return
	}
	setupMu.Lock()
	defer setupMu.Unlock()
	if h.repo.HasPasswordAccount() {
		c.JSON(http.StatusConflict, gin.H{"error": "管理员已创建，请直接登录"})
		return
	}
	user, err := EnsureLocalUser(h.repo, req.LoginName, req.DisplayName, req.Password, true)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.adoptOwnerlessProjects(user.ID)
	h.signIn(c, user)
}

// authRegister lets a visitor create an ordinary account while sign-up is open.
func (h *authHandlers) authRegister(c *gin.Context) {
	var req signupRequest
	if !decode(c, &req) {
		return
	}
	if !h.repo.HasPasswordAccount() {
		c.JSON(http.StatusConflict, gin.H{"error": "请先创建管理员账号"})
		return
	}
	if !h.repo.AuthSettings().SignupOpen {
		c.JSON(http.StatusForbidden, gin.H{"error": "管理员已关闭注册，请联系管理员创建账号"})
		return
	}
	login := strings.ToLower(strings.TrimSpace(req.LoginName))
	if _, exists := h.repo.UserByLoginName(login); exists {
		c.JSON(http.StatusConflict, gin.H{"error": "登录名已被占用"})
		return
	}
	user, err := EnsureLocalUser(h.repo, login, req.DisplayName, req.Password, false)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.signIn(c, user)
}

// authSettings reads or (platform admins only) changes the sign-up rule.
func (h *authHandlers) authSettings(c *gin.Context) {
	if c.Request.Method == http.MethodGet {
		c.JSON(http.StatusOK, h.repo.AuthSettings())
		return
	}
	if !h.isPlatformAdmin(c.Request) {
		c.JSON(http.StatusForbidden, gin.H{"error": "只有平台管理员可以修改注册设置"})
		return
	}
	var req repo.AuthSettings
	if !decode(c, &req) {
		return
	}
	if err := h.repo.PutAuthSettings(req); err != nil {
		respond(c, err)
		return
	}
	c.JSON(http.StatusOK, h.repo.AuthSettings())
}

func (h *authHandlers) signIn(c *gin.Context, stored domain.User) {
	user := domainUserToAuth(stored)
	token, err := h.getAuthClient().SignUser(user)
	if err != nil {
		respond(c, err)
		return
	}
	h.setAuthCookie(c.Writer, c.Request, token)
	c.JSON(http.StatusOK, h.currentPayload(user))
}

// adoptOwnerlessProjects hands the projects seeded before any account
// existed to the first admin.
func (h *authHandlers) adoptOwnerlessProjects(ownerID int64) {
	for _, project := range h.repo.Projects() {
		if project.OwnerID != 0 {
			continue
		}
		project.OwnerID = ownerID
		if len(project.Members) == 0 {
			project.Members = []domain.ProjectMember{{UserID: ownerID, Role: "maintainer"}}
		}
		if err := h.repo.PutProject(project); err != nil {
			log.Printf("[auth] assign project %s to the first admin: %v", project.ID, err)
		}
	}
}

// SignSessionCookie signs a browser session for user with the process's
// session secret. The in-container self-test uses it so a fresh instance can
// be checked without knowing anyone's password.
func SignSessionCookie(user domain.User) (*http.Cookie, error) {
	token, err := auth.NewClientFromEnv().SignUser(domainUserToAuth(user))
	if err != nil {
		return nil, err
	}
	return &http.Cookie{Name: authCookieName, Value: token, Path: "/"}, nil
}
