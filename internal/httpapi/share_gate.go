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

	"github.com/Tencent/LoopForge/internal/domain"
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
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(shareGateHTML(shareGateView{Title: "链接无效或已过期", Message: err.Error()})))
		return
	}
	run, ok := h.repo.Run(p.RunID)
	if !ok {
		c.Data(http.StatusNotFound, "text/html; charset=utf-8", []byte(shareGateHTML(shareGateView{Title: "运行不存在", Message: "run not found"})))
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
	view := shareGateView{Title: run.WorkflowSnapshot.Name, NodeName: nodeName, RunID: p.RunID}
	if view.Title == "" {
		view.Title = run.WorkflowID
	}
	if issue, ok := h.repo.Issue(run.ExecutionContext.IssueID); ok && strings.TrimSpace(issue.Title) != "" {
		view.Workflow = view.Title
		view.Title = issue.Title
	}
	view.Waiting = node != nil && node.Status == "waiting_approval" && run.Status == "waiting"
	if view.Waiting {
		view.Token = token
		view.Message = "请确认后批准。批准后流程继续；驳回需登录平台指定返工目标。"
		view.Upstream = shareGateUpstream(run, p.NodeID)
	} else {
		view.Message = "此门禁已处理或运行已结束，链接只读。"
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(shareGateHTML(view)))
}

type shareGateView struct {
	Title, Workflow, NodeName, RunID, Token, Message string
	Waiting                                          bool
	Upstream                                         []shareGateExcerpt
}

type shareGateExcerpt struct{ Name, Text string }

// shareGateUpstream gives an external reviewer what the gate is about: the
// results of the nodes that feed it.
func shareGateUpstream(run domain.Run, nodeID string) []shareGateExcerpt {
	names := map[string]string{}
	for _, n := range run.WorkflowSnapshot.Nodes {
		names[n.ID] = n.Name
	}
	var out []shareGateExcerpt
	for _, edge := range run.WorkflowSnapshot.Edges {
		if edge.Target != nodeID {
			continue
		}
		nr := run.Nodes[edge.Source]
		if nr == nil {
			continue
		}
		text := ""
		if nr.GateResult != nil {
			text = strings.TrimSpace(nr.GateResult.Summary)
		}
		if text == "" {
			text = strings.TrimSpace(nr.Output)
		}
		if text == "" {
			continue
		}
		if r := []rune(text); len(r) > 1500 {
			text = string(r[:1500]) + "…"
		}
		name := names[edge.Source]
		if name == "" {
			name = edge.Source
		}
		out = append(out, shareGateExcerpt{Name: name, Text: text})
	}
	return out
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

func shareGateHTML(v shareGateView) string {
	title, nodeName, runID, token, msg := v.Title, v.NodeName, v.RunID, v.Token, v.Message
	actions := ""
	if v.Waiting && token != "" {
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
:root{color-scheme:dark}
body{margin:0;min-height:100vh;font:15px/1.6 Inter,ui-sans-serif,-apple-system,"PingFang SC","Microsoft YaHei",sans-serif;background:radial-gradient(720px 340px at 50% -6%,rgba(139,123,255,.22),transparent 70%),#08090c;color:#ededf2}
main.card{max-width:540px;margin:7vh auto;padding:28px;background:#111217;border:1px solid rgba(255,255,255,.08);border-radius:18px;box-shadow:0 30px 80px -30px rgba(0,0,0,.8)}
.kicker{display:inline-flex;align-items:center;gap:8px;color:#f5a524;font-size:12px;font-weight:650;letter-spacing:.04em}
.kicker::before{content:"";width:8px;height:8px;border-radius:50%;background:#f5a524;box-shadow:0 0 12px rgba(245,165,36,.8)}
h1{margin:8px 0;font-size:22px;letter-spacing:-.02em}p{color:#a3a5b5}p b{color:#ededf2}
label{display:grid;gap:6px;margin:12px 0;font-size:13px;color:#a3a5b5}
input,textarea{padding:11px 12px;border-radius:10px;border:1px solid rgba(255,255,255,.12);background:#16171e;color:#ededf2;font:inherit}
input:focus,textarea:focus{outline:none;border-color:#8b7bff;box-shadow:0 0 0 3px rgba(139,123,255,.18)}
button.primary{margin-top:8px;width:100%;min-height:46px;padding:12px;border:0;border-radius:11px;background:linear-gradient(135deg,#8b7bff,#5b8cff 55%,#38c6f4);color:#fff;font-weight:700;font-size:15px;cursor:pointer;box-shadow:0 12px 30px -12px rgba(139,123,255,.7)}
.meta{font-size:12px;color:#6c6f80;margin-top:16px}
details{margin:12px 0;border:1px solid rgba(255,255,255,.08);border-radius:10px;padding:10px 12px;background:#16171e}
summary{cursor:pointer;font-size:13px;color:#ededf2}
pre{white-space:pre-wrap;word-break:break-word;margin:8px 0 0;font:13px/1.55 ui-monospace,monospace;color:#c9cbd6;max-height:320px;overflow:auto}
@media(max-width:600px){main.card{margin:0;min-height:100vh;border-radius:0;border:0;padding:22px 18px 110px}
.actions button.primary{position:fixed;left:16px;right:16px;bottom:calc(16px + env(safe-area-inset-bottom));width:auto;margin:0}}
</style></head><body><main class="card">
<p class="kicker">外部一次决策</p>
<h1>` + htmlEsc(title) + `</h1>
<p>` + shareGateWhere(v.Workflow, nodeName) + `</p>
<p>` + htmlEsc(msg) + `</p>
` + shareGateUpstreamHTML(v.Upstream) + actions + `
<p class="meta">Run ` + htmlEsc(runID) + ` · 链接 24 小时内有效</p>
</main></body></html>`
}

func shareGateWhere(workflow, nodeName string) string {
	where := ""
	if workflow != "" {
		where = "流程：<b>" + htmlEsc(workflow) + "</b> · "
	}
	if nodeName == "" {
		return where
	}
	return where + "节点：<b>" + htmlEsc(nodeName) + "</b>"
}

func shareGateUpstreamHTML(items []shareGateExcerpt) string {
	var b strings.Builder
	for i, item := range items {
		open := ""
		if i == 0 {
			open = " open"
		}
		b.WriteString("<details" + open + "><summary>上游「" + htmlEsc(item.Name) + "」的结果</summary><pre>" + htmlEsc(item.Text) + "</pre></details>")
	}
	return b.String()
}

func htmlEsc(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}
