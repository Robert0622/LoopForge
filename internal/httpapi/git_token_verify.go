package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Tencent/LoopForge/internal/tracker"
	"github.com/gin-gonic/gin"
)

// requiredGitTokenScopes are the classic-PAT scopes needed to clone, push work
// branches and open pull requests. Fine-grained tokens do not report scopes,
// so for them only validity is checked.
var requiredGitTokenScopes = []string{"repo"}

type gitTokenVerifyResult struct {
	Valid   bool     `json:"valid"`
	Login   string   `json:"login,omitempty"`
	Scopes  []string `json:"scopes,omitempty"`
	Missing []string `json:"missing,omitempty"`
	Error   string   `json:"error,omitempty"`
}

// verifyGitToken checks a Git token against the GitHub API: the token in a
// POST body (typed but not saved yet) or else the signed-in user's stored one.
// It always answers 200 with a structured result for the UI.
func (h *usersHandlers) verifyGitToken(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gitTokenVerifyResult{Error: "valid user id is required"})
		return
	}
	current, ok := h.requestUser(c.Request, false)
	if !ok || current.ID != id {
		c.JSON(http.StatusForbidden, gitTokenVerifyResult{Error: "只能校验当前登录用户的凭据"})
		return
	}
	if h.repo == nil {
		c.JSON(http.StatusOK, gitTokenVerifyResult{Error: "存储不可用"})
		return
	}
	user, ok := h.repo.User(id)
	if !ok {
		c.JSON(http.StatusOK, gitTokenVerifyResult{Error: "用户不存在"})
		return
	}
	token := user.GitToken
	if c.Request.Method == http.MethodPost {
		var req struct {
			Token string `json:"token"`
		}
		if !decode(c, &req) {
			return
		}
		if typed := strings.TrimSpace(req.Token); typed != "" {
			token = typed
		}
	}
	if token == "" {
		c.JSON(http.StatusOK, gitTokenVerifyResult{Error: "请先在上方填写 Git Token"})
		return
	}
	base := tracker.DefaultGitHubAPIURL
	if h.sync != nil {
		base = h.sync.APIBaseURL()
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	c.JSON(http.StatusOK, verifyGitHubToken(ctx, base, token))
}

func verifyGitHubToken(ctx context.Context, apiBase, token string) gitTokenVerifyResult {
	scopes, login, err := githubTokenUser(ctx, apiBase, token)
	if err != nil {
		return gitTokenVerifyResult{Valid: false, Error: err.Error()}
	}
	missing := missingGitTokenScopes(scopes, requiredGitTokenScopes)
	return gitTokenVerifyResult{Valid: len(missing) == 0, Login: login, Scopes: scopes, Missing: missing}
}

// githubTokenUser calls GET /user. Classic tokens report their scopes in the
// X-OAuth-Scopes header; fine-grained tokens omit it.
func githubTokenUser(ctx context.Context, apiBase, token string) ([]string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(apiBase, "/")+"/user", nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("无法连接 GitHub API（%s）：%v", apiBase, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, "", errors.New("令牌无效或已过期：请确认填写的是 GitHub 个人访问令牌")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("GitHub 返回 HTTP %d", resp.StatusCode)
	}
	var info struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&info); err != nil {
		return nil, "", fmt.Errorf("GitHub 返回内容无法解析（HTTP %d）", resp.StatusCode)
	}
	var scopes []string
	for _, scope := range strings.Split(resp.Header.Get("X-OAuth-Scopes"), ",") {
		if scope = strings.TrimSpace(scope); scope != "" {
			scopes = append(scopes, scope)
		}
	}
	return scopes, info.Login, nil
}

func missingGitTokenScopes(scopes, required []string) []string {
	if len(scopes) == 0 {
		return nil
	}
	set := make(map[string]bool, len(scopes))
	for _, s := range scopes {
		set[s] = true
	}
	var missing []string
	for _, s := range required {
		if !set[s] {
			missing = append(missing, s)
		}
	}
	return missing
}
