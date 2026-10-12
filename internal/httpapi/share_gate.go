package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const shareGateTTL = 24 * time.Hour

type shareGatePayload struct {
	RunID  string `json:"r"`
	NodeID string `json:"n"`
	Exp    int64  `json:"e"`
}

func shareGateSecret(d *deps) string {
	if d != nil {
		if secret := strings.TrimSpace(d.getAuthClient().Config().SessionSecret); secret != "" {
			return secret
		}
	}
	if v := strings.TrimSpace(os.Getenv("LOOPFORGE_SESSION_SECRET")); v != "" {
		return v
	}
	return "loopforge-share-dev"
}

func signShareGate(secret string, p shareGatePayload) (string, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(raw)
	sig := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func parseShareGate(secret, token string) (shareGatePayload, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return shareGatePayload{}, fmt.Errorf("invalid token")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return shareGatePayload{}, fmt.Errorf("invalid token")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return shareGatePayload{}, fmt.Errorf("invalid token")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(raw)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return shareGatePayload{}, fmt.Errorf("invalid token")
	}
	var p shareGatePayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return shareGatePayload{}, fmt.Errorf("invalid token")
	}
	if p.Exp > 0 && time.Now().Unix() > p.Exp {
		return shareGatePayload{}, fmt.Errorf("link expired")
	}
	return p, nil
}

// createShareGateLink issues a 24h link for an approval gate (login required to create).
func (h *runsHandlers) createShareGateLink(c *gin.Context) {
	runID := c.Param("id")
	var req struct {
		NodeID string `json:"nodeId"`
	}
	_ = c.ShouldBindJSON(&req)
	run, ok := h.repo.Run(runID)
	if !ok {
		c.JSON(http.StatusNotFound, map[string]string{"error": "run not found"})
		return
	}
	nodeID := strings.TrimSpace(req.NodeID)
	if nodeID == "" {
		for id, n := range run.Nodes {
			if n != nil && n.Status == "waiting_approval" {
				nodeID = id
				break
			}
		}
	}
	if nodeID == "" || run.Nodes[nodeID] == nil || run.Nodes[nodeID].Status != "waiting_approval" {
		c.JSON(http.StatusBadRequest, map[string]string{"error": "run is not waiting on that approval node"})
		return
	}
	token, err := signShareGate(shareGateSecret(h.deps), shareGatePayload{
		RunID: runID, NodeID: nodeID, Exp: time.Now().Add(shareGateTTL).Unix(),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	base := publicBaseURL(c.Request)
	c.JSON(http.StatusOK, map[string]any{
		"url":       base + "/share/gate?token=" + token,
		"expiresAt": time.Now().UTC().Add(shareGateTTL).Format(time.RFC3339),
		"nodeId":    nodeID,
	})
}

func publicBaseURL(r *http.Request) string {
	if v := strings.TrimSpace(os.Getenv("LOOPFORGE_PUBLIC_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	host := r.Host
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		host = fwd
	}
	return scheme + "://" + host
}

func (h *runsHandlers) shareGatePage(c *gin.Context) {
	token := strings.TrimSpace(c.Query("token"))
	p, err := parseShareGate(shareGateSecret(h.deps), token)
	if err != nil {
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(shareGateHTML("链接无效或已过期", "", "", "", false, err.Error())))
		return
	}
	run, ok := h.repo.Run(p.RunID)
	if !ok {
		c.Data(http.StatusNotFound, "text/html; charset=utf-8", []byte(shareGateHTML("运行不存在", "", "", "", false, "run not found")))
		return
	}
	node := run.Nodes[p.NodeID]
	nodeName := p.NodeID
	for _, n := range run.WorkflowSnapshot.Nodes {
		if n.ID == p.NodeID {
			nodeName = n.Name
			break
		}
	}
	title := run.WorkflowSnapshot.Name
	if title == "" {
		title = run.WorkflowID
	}
	waiting := node != nil && node.Status == "waiting_approval" && run.Status == "waiting"
	msg := "请确认后批准。批准后流程继续；驳回需登录平台指定返工目标。"
	if !waiting {
		msg = "此门禁已处理或运行已结束，链接只读。"
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(shareGateHTML(title, nodeName, p.RunID, token, waiting, msg)))
}

func (h *runsHandlers) shareGateDecide(c *gin.Context) {
	var req struct {
		Token    string `json:"token"`
		Decision string `json:"decision"`
		Name     string `json:"name"`
		Comment  string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	p, err := parseShareGate(shareGateSecret(h.deps), strings.TrimSpace(req.Token))
	if err != nil {
		c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if strings.TrimSpace(req.Decision) != "approve" {
		c.JSON(http.StatusBadRequest, map[string]string{"error": "此公开链接仅支持批准；驳回请登录平台"})
		return
	}
	ctx := strings.TrimSpace(req.Name)
	if ctx == "" {
		ctx = "外部审批"
	}
	if comment := strings.TrimSpace(req.Comment); comment != "" {
		ctx = ctx + "：" + comment
	}
	if err := h.engine.Approve(p.RunID, p.NodeID, ctx); err != nil {
		c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, map[string]string{"status": "approved"})
}

func shareGateHTML(title, nodeName, runID, token string, waiting bool, msg string) string {
	actions := ""
	if waiting && token != "" {
		actions = `<form id="f" class="actions">
  <label>你的姓名<input name="name" required placeholder="张三" maxlength="40"></label>
  <label>意见（可选）<textarea name="comment" rows="3" maxlength="500" placeholder="确认方案可行"></textarea></label>
  <button type="submit" class="primary">确认通过，继续下游</button>
</form>
<script>
const f=document.getElementById('f');
f.onsubmit=async(e)=>{
  e.preventDefault();
  const body={token:` + strconv.Quote(token) + `,decision:'approve',name:f.name.value,comment:f.comment.value};
  const r=await fetch('/api/share/gate/decide',{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify(body)});
  const d=await r.json().catch(()=>({}));
  if(!r.ok){alert(d.error||'失败');return;}
  document.body.innerHTML='<main class="card"><h1>已批准</h1><p>流程将继续。可以关闭此页。</p></main>';
};
</script>`
	}
	return `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>外部审批 · LoopForge</title>
<style>
body{margin:0;font:15px/1.5 system-ui,sans-serif;background:#0f172a;color:#e2e8f0}
main.card{max-width:520px;margin:8vh auto;padding:28px;background:#1e293b;border:1px solid #334155;border-radius:16px}
.kicker{color:#93c5fd;font-size:12px;font-weight:650;letter-spacing:.04em}
h1{margin:8px 0;font-size:22px}p{color:#94a3b8}
label{display:grid;gap:6px;margin:12px 0;font-size:13px;color:#cbd5e1}
input,textarea{padding:10px 12px;border-radius:10px;border:1px solid #475569;background:#0f172a;color:#e2e8f0;font:inherit}
button.primary{margin-top:8px;width:100%;padding:12px;border:0;border-radius:10px;background:#3867f4;color:#fff;font-weight:650;cursor:pointer}
.meta{font-size:12px;color:#64748b;margin-top:16px}
</style></head><body><main class="card">
<p class="kicker">外部一次决策</p>
<h1>` + htmlEsc(title) + `</h1>
<p>节点：<b>` + htmlEsc(nodeName) + `</b></p>
<p>` + htmlEsc(msg) + `</p>
` + actions + `
<p class="meta">Run ` + htmlEsc(runID) + ` · 链接 24 小时内有效</p>
</main></body></html>`
}

func htmlEsc(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}
