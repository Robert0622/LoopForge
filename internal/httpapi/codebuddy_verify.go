package httpapi

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/Tencent/LoopForge/internal/repo"
	"github.com/gin-gonic/gin"
)

type codeBuddyVerifyResult struct {
	OK bool `json:"ok"`
	// Environment is the CodeBuddy network environment that accepted the key:
	// "" (international site) or "internal" (China site).
	Environment string `json:"environment"`
	Switched    bool   `json:"switched,omitempty"`
	Skipped     bool   `json:"skipped,omitempty"`
	Error       string `json:"error,omitempty"`
}

// codeBuddyVerifyBin and codeBuddyProbe are replaced in tests.
var codeBuddyVerifyBin = resolveCodeBuddyBin

// codeBuddyProbe runs one tiny prompt.
var codeBuddyProbe = func(ctx context.Context, bin, key, environment string) (bool, string) {
	home, err := os.MkdirTemp("", "loopforge-cb-verify-")
	if err != nil {
		return false, err.Error()
	}
	defer os.RemoveAll(home)
	cmd := exec.CommandContext(ctx, bin, "-p", "Reply with the single word: ok", "--setting-sources", "none")
	cmd.Dir = home
	cmd.Env = append(os.Environ(), "HOME="+home, "CODEBUDDY_API_KEY="+key, "CODEBUDDY_INTERNET_ENVIRONMENT="+environment)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil || strings.Contains(text, "401") || strings.Contains(strings.ToLower(text), "authentication required") {
		if len(text) > 300 {
			text = text[:300]
		}
		if text == "" && err != nil {
			text = err.Error()
		}
		return false, text
	}
	return true, ""
}

// 路由: g.POST("/api/users/:id/codebuddy/verify", h.verifyCodeBuddyKey)
//
// Tries the saved key against the configured network environment, then the
// other one, and saves the environment that works: keys from the China site
// fail with 401 on the international default and new users cannot tell why.
func (h *usersHandlers) verifyCodeBuddyKey(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, codeBuddyVerifyResult{Error: "valid user id is required"})
		return
	}
	current, ok := h.requestUser(c.Request, false)
	if !ok || current.ID != id {
		c.JSON(http.StatusForbidden, codeBuddyVerifyResult{Error: "只能校验当前登录用户的凭据"})
		return
	}
	user, ok := h.repo.User(id)
	if !ok {
		c.JSON(http.StatusOK, codeBuddyVerifyResult{Error: "用户不存在"})
		return
	}
	key := strings.TrimSpace(user.CodeBuddyAPIKey)
	if key == "" {
		c.JSON(http.StatusOK, codeBuddyVerifyResult{Error: "请先填写并保存 CodeBuddy API Key"})
		return
	}
	bin, err := codeBuddyVerifyBin()
	if err != nil {
		c.JSON(http.StatusOK, codeBuddyVerifyResult{Skipped: true, Environment: user.CodeBuddyInternetEnvironment, Error: "本机没有 codebuddy CLI，跳过在线校验"})
		return
	}
	configured := strings.TrimSpace(user.CodeBuddyInternetEnvironment)
	candidates := []string{configured}
	for _, env := range []string{"", "internal"} {
		if env != configured {
			candidates = append(candidates, env)
		}
	}
	var lastErr string
	for _, env := range candidates {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
		passed, detail := codeBuddyProbe(ctx, bin, key, env)
		cancel()
		if !passed {
			lastErr = detail
			continue
		}
		result := codeBuddyVerifyResult{OK: true, Environment: env}
		if env != configured {
			value := env
			if _, err := h.repo.UpdateUserCredentials(id, repo.UserCredentialUpdate{CodeBuddyInternetEnvironment: &value}); err != nil {
				c.JSON(http.StatusOK, codeBuddyVerifyResult{Error: "保存网络环境失败：" + err.Error()})
				return
			}
			result.Switched = true
		}
		c.JSON(http.StatusOK, result)
		return
	}
	c.JSON(http.StatusOK, codeBuddyVerifyResult{Error: "国际站和中国站都拒绝了这个 Key，请检查是否复制完整：" + lastErr})
}
