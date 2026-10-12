package httpapi

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/Tencent/LoopForge/internal/domain"
	"github.com/Tencent/LoopForge/internal/tracker"
)

const (
	clarifyDefaultModel = assistantDefaultModel
)

var forgeIssueURLRe = regexp.MustCompile(`(?i)https?://[a-z0-9.-]+/[^/\s<>"'）】\]]+/[^/\s<>"'）】\]]+/issues/\d+`)

func clarifyAllowedModel(id string) string {
	return assistantAllowedModel(id, assistantFixedCatalog())
}

func asStringKeyMap(v any) map[string]any {
	m, ok := v.(map[string]any)
	if !ok || m == nil {
		return map[string]any{}
	}
	return m
}

func stripClarifyMCPCalls(parsed any) any {
	raw, ok := parsed.(map[string]any)
	if !ok || raw == nil {
		return parsed
	}
	delete(raw, "mcpCalls")
	return raw
}

func classifyClarifyMCP(item domain.MCPServerDefinition) string {
	mid := strings.ToLower(strings.TrimSpace(item.MarketID))
	blob := strings.ToLower(item.Name + " " + item.ID + " " + mid)
	switch {
	case mid == "github" || strings.Contains(blob, "github"):
		return "github"
	default:
		return "other"
	}
}

func (h *streamHandlers) clarifyHTTPMCPs(projectID string, userID int64) []domain.MCPServerDefinition {
	if h == nil || h.repo == nil {
		return nil
	}
	out := []domain.MCPServerDefinition{}
	for _, item := range assistantOutboundMCPServers(h.repo.MCPServers(), projectID, userID) {
		transport := strings.ToLower(strings.TrimSpace(item.Transport))
		if transport == "" && strings.TrimSpace(item.URL) != "" {
			transport = "http"
		}
		if transport != "http" && transport != "sse" {
			continue
		}
		out = append(out, item)
	}
	return out
}

func (h *streamHandlers) clarifyRepoPath(issue domain.Issue) string {
	if issue.ExternalRef != nil {
		if path := forgeProjectPathFromClarifyURL(issue.ExternalRef.ExternalURL); path != "" {
			return path
		}
	}
	if h == nil || h.repo == nil {
		return ""
	}
	for _, id := range issue.EffectiveRepositoryIDs() {
		repo, ok := h.repo.Repository(id)
		if ok && strings.TrimSpace(repo.RepoPath) != "" {
			return strings.TrimSpace(repo.RepoPath)
		}
	}
	project, ok := h.repo.Project(issue.ProjectID)
	if !ok || strings.TrimSpace(project.DefaultRepositoryID) == "" {
		return ""
	}
	repo, ok := h.repo.Repository(project.DefaultRepositoryID)
	if !ok {
		return ""
	}
	return strings.TrimSpace(repo.RepoPath)
}

type clarifyForgeHit struct {
	Project string
	IID     int
	URL     string
}

func extractForgeIssueRefs(texts ...string) []clarifyForgeHit {
	seen := map[string]bool{}
	var out []clarifyForgeHit
	for _, text := range texts {
		for _, match := range forgeIssueURLRe.FindAllString(text, -1) {
			raw := strings.TrimRight(match, ".,;:，。；：")
			project := forgeProjectPathFromClarifyURL(raw)
			iid := forgeIssueIIDFromURL(raw)
			key := project + "#" + strconv.Itoa(iid)
			if project == "" || iid <= 0 || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, clarifyForgeHit{Project: project, IID: iid, URL: raw})
		}
	}
	return out
}

func forgeProjectPathFromClarifyURL(raw string) string {
	ref, ok := tracker.ParseIssueURL(raw)
	if !ok {
		return ""
	}
	return ref.Repo
}

func forgeIssueIIDFromURL(raw string) int {
	ref, _ := tracker.ParseIssueURL(raw)
	return ref.Number
}

func hydrateClarifyIssueFromText(issue *domain.Issue, extras ...string) {
	if issue == nil || issue.ExternalRef != nil {
		return
	}
	texts := append([]string{issue.Title, issue.Description}, extras...)
	if hits := extractForgeIssueRefs(texts...); len(hits) > 0 {
		hit := hits[0]
		issue.Source = "github"
		issue.ExternalRef = &domain.ExternalRef{Source: "github", ExternalID: strconv.Itoa(hit.IID), ExternalIID: hit.IID, ExternalType: "issue", ExternalURL: hit.URL}
	}
}

func clarifyRequestTexts(request chatClarifyRequest) []string {
	texts := []string{request.Title, request.Description, request.Message}
	for _, turn := range request.History {
		texts = append(texts, turn.Content)
	}
	return texts
}

func (h *streamHandlers) clarifyMCPInstructions(issue domain.Issue, user domain.User, resumed bool, extras ...string) string {
	return h.clarifyMCPInstructionsFor(issue, user, resumed, extras...)
}

func (h *streamHandlers) clarifyMCPInstructionsFor(issue domain.Issue, user domain.User, resumed bool, extras ...string) string {
	projectID := strings.TrimSpace(issue.ProjectID)
	if projectID == "" {
		return ""
	}
	servers := h.clarifyHTTPMCPs(projectID, user.ID)
	var forge, other []domain.MCPServerDefinition
	for _, item := range servers {
		if classifyClarifyMCP(item) == "github" {
			forge = append(forge, item)
		} else {
			other = append(other, item)
		}
	}
	var b strings.Builder
	b.WriteString("\n\n## 可用工具（只读）\n使用当前项目和个人已安装的 HTTP MCP（github__* 及其他已安装工具）以及 loopforge__* 查询原单、评论和文档。stdio MCP 不可用。不要调用写入类工具。资料足够后调用 submit_clarification。\n")
	if len(servers) > 0 {
		b.WriteString("当前项目和个人可用 HTTP MCP：\n")
		for _, item := range servers {
			fmt.Fprintf(&b, "- %s（id=%s", item.Name, item.ID)
			if strings.TrimSpace(item.MarketID) != "" {
				fmt.Fprintf(&b, ", marketId=%s", item.MarketID)
			}
			b.WriteString("）\n")
		}
	}
	if len(forge) > 0 && strings.EqualFold(strings.TrimSpace(issue.Source), "github") {
		repoPath := h.clarifyRepoPath(issue)
		owner, name, _ := strings.Cut(repoPath, "/")
		number := 0
		if issue.ExternalRef != nil {
			number = issue.ExternalRef.ExternalIID
		}
		if resumed {
			b.WriteString("这是 GitHub Issue。对话里已有此前分析，不要再重拉原单和评论。\n")
		} else {
			fmt.Fprintf(&b, "这是 GitHub Issue。必须读取原单和评论：调用 github__get_issue（owner=%s, repo=%s, issue_number=%d）与 github__get_issue_comments（同参数）；新版 GitHub MCP 使用 github__issue_read（method=get / get_comments）。\n",
				owner, name, number)
		}
	}
	if issue.ID == "" && issue.ExternalRef == nil && len(servers) > 0 {
		b.WriteString("这是尚未建单的需求草稿，没有原单可拉。描述或对话里出现 Issue 或文档链接时必须先读取后再出题；没有链接不要为了找原单去 list。\n")
	}
	if len(other) > 0 {
		b.WriteString("其他已安装的 HTTP MCP 按工具名直接调用，仅限只读。\n")
	}
	b.WriteString("资料足够后调用 submit_clarification。\n")
	return b.String()
}

func clarifyMCPWriteName(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" || n == "tools/list" {
		return false
	}
	switch {
	case strings.Contains(n, "create"), strings.Contains(n, "update"), strings.Contains(n, "delete"),
		strings.Contains(n, "save"), strings.Contains(n, "write"), strings.HasPrefix(n, "add_"),
		strings.Contains(n, "move"), strings.Contains(n, "rename"), strings.Contains(n, "copy"):
		return true
	default:
		return false
	}
}

func clarifyMCPCallWrite(kind string, call assistantMCPCall) bool {
	return clarifyMCPWriteName(call.Tool)
}

func (h *streamHandlers) clarifyMCPUser(issue domain.Issue, responsible domain.User) domain.User {
	if h == nil || h.repo == nil {
		return responsible
	}
	project, ok := h.repo.Project(issue.ProjectID)
	if !ok || project.OwnerID <= 0 {
		return responsible
	}
	owner, ok := h.repo.User(project.OwnerID)
	if !ok || strings.TrimSpace(owner.PersonalMCPToken) == "" {
		return responsible
	}
	return owner
}

func (h *streamHandlers) dispatchClarifyMCPCall(asst *assistantHandlers, user domain.User, projectID string, call assistantMCPCall, emit func(event string, payload any)) (string, error) {
	if assistantPlatformServer(call.Server) && strings.TrimSpace(call.Tool) != "" && call.Tool != "tools/list" {
		return "", fmt.Errorf("澄清只允许调用当前项目的 HTTP MCP，不能调用平台写入工具")
	}
	server, ok := asst.lookupAssistantMCP(projectID, user.ID, call.Server)
	if !ok {
		return "", fmt.Errorf("当前项目没有名为 %q 的 MCP，或你无权使用", strings.TrimSpace(call.Server))
	}
	kind := classifyClarifyMCP(server)
	if clarifyMCPCallWrite(kind, call) {
		return "", fmt.Errorf("澄清只允许只读 MCP，拒绝 %s", strings.TrimSpace(call.Tool))
	}
	if emit != nil {
		detail := strings.TrimSpace(call.Tool)
		emit("thought", map[string]string{"kind": "tool", "tool": "MCP", "detail": server.Name + " " + detail})
	}
	caller := user
	if server.Scope != "personal" {
		caller = h.clarifyMCPUser(domain.Issue{ProjectID: projectID}, user)
	}
	return asst.callAssistantMCP(caller, projectID, call)
}

func (h *streamHandlers) openClarifyToolSession(ctx context.Context, user domain.User, issue domain.Issue, cancel context.CancelFunc) (*AgentToolSession, agentToolFiles, error) {
	return startAgentToolSession(ctx, user, issue.ProjectID, AgentToolClarify, assistantExecMode{}, issue.ID, h.clarifyHTTPMCPs(issue.ProjectID, user.ID), cancel, filepath.Join(clarifyCLIHome(user.ID), "mcp-sessions"))
}
