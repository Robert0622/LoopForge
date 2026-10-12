package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/Tencent/LoopForge/internal/application"
	"github.com/Tencent/LoopForge/internal/domain"
	"github.com/Tencent/LoopForge/internal/mcpclient"
)

const (
	agentToolCatalogTTL     = 60 * time.Second
	agentToolSessionMCPName = "loopforge_session"
)

var (
	errAgentToolInvalidSession    = errors.New("invalid agent tool session")
	errAgentToolTerminalCommitted = errors.New("终态已提交，本轮不能再次调用")
	errAgentToolUnknown           = errors.New("unknown tool")
	errAgentToolForbidden         = errors.New("该工具不在当前场景的允许列表中")
)

type agentToolDescriptor struct {
	Name         string
	Description  string
	InputSchema  json.RawMessage
	OutputSchema json.RawMessage
	Annotations  json.RawMessage
	Kind         string
	ServerID     string
	UpstreamName string
	Terminal     bool
}

type agentToolCatalogCache struct {
	mu    sync.Mutex
	items map[string]agentToolCatalogCacheEntry
}

type agentToolCatalogCacheEntry struct {
	tools     []mcpclient.Tool
	expiresAt time.Time
}

var agentToolUpstreamCache = &agentToolCatalogCache{items: map[string]agentToolCatalogCacheEntry{}}

func resetAgentToolCatalogCacheForTest() {
	agentToolUpstreamCache = &agentToolCatalogCache{items: map[string]agentToolCatalogCacheEntry{}}
}

func namespaceAgentTool(kind, name string) string {
	kind = strings.TrimSpace(kind)
	name = strings.TrimSpace(name)
	if kind == "" {
		return name
	}
	return kind + "__" + name
}

func sanitizeAgentToolNamespace(id, name string) string {
	raw := strings.ToLower(strings.TrimSpace(id))
	if raw == "" {
		raw = strings.ToLower(strings.TrimSpace(name))
	}
	var b strings.Builder
	for _, r := range raw {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	for strings.Contains(out, "__") {
		out = strings.ReplaceAll(out, "__", "_")
	}
	switch out {
	case "", "github", "loopforge":
		return "mcp"
	default:
		return out
	}
}

func stripAgentToolServerPrefix(name string) string {
	name = strings.TrimSpace(name)
	for _, prefix := range []string{
		"mcp__" + agentToolSessionMCPName + "__",
		agentToolSessionMCPName + "__",
	} {
		name = strings.TrimPrefix(name, prefix)
	}
	return name
}

func parseNamespacedAgentTool(name string) (kind, tool string, ok bool) {
	name = stripAgentToolServerPrefix(name)
	if name == "" {
		return "", "", false
	}
	kind, tool, found := strings.Cut(name, "__")
	if !found || strings.TrimSpace(kind) == "" || strings.TrimSpace(tool) == "" {
		return "", name, false
	}
	return kind, tool, true
}

func agentToolKindAllowlist(mode AgentToolMode, kind string) map[string]bool {
	out := map[string]bool{}
	for _, name := range agentToolKindAllowlistNames(mode, kind) {
		out[name] = true
	}
	return out
}

func agentToolKindAllowlistNames(mode AgentToolMode, kind string) []string {
	if mode == AgentToolScheduled && kind != "loopforge" {
		return nil
	}
	switch kind {
	case "github":
		names := []string{"get_issue", "get_issue_comments", "issue_read"}
		if agentToolAllowsAssistantReads(mode) {
			names = append(names,
				"list_issues", "search_issues", "get_file_contents", "list_commits", "get_commit",
				"search_code", "search_repositories", "list_pull_requests", "search_pull_requests",
				"get_pull_request", "get_pull_request_files", "get_pull_request_diff", "pull_request_read",
				"list_branches", "list_tags",
			)
		}
		return names
	case "loopforge":
		if mode == AgentToolClarify {
			return []string{"issues_get", "issues_list", "repositories_get", "projects_get"}
		}
		if mode == AgentToolAssistant {
			return loopforgeAssistantReadToolNames()
		}
		if mode == AgentToolScheduled {
			return loopforgeScheduledToolNames()
		}
		return nil
	default:
		return nil
	}
}

func loopforgeNoisyTool(name string) bool {
	switch name {
	case "health_get", "capabilities_get", "openapi_get", "bootstrap_get", "observability_spec_get", "analytics_get", "workflow_metrics_get":
		return true
	default:
		return false
	}
}

func loopforgeAssistantReadToolNames() []string {
	var names []string
	for _, op := range application.PublicOperations() {
		if !op.ReadOnly || loopforgeNoisyTool(op.Name) {
			continue
		}
		names = append(names, op.Name)
	}
	return names
}

// loopforgeAssistantWriteToolNames 返回审批模式放行直连的平台写入工具：
// assisted 放行保存类（危险类仍走草稿），auto 放行全部写入。
func loopforgeAssistantWriteToolNames(exec assistantExecMode) []string {
	if !exec.AutoWrite {
		return nil
	}
	var names []string
	for _, op := range application.PublicOperations() {
		if op.ReadOnly || loopforgeNoisyTool(op.Name) {
			continue
		}
		if exec.ConfirmDangerous && assistantRiskyToolName(op.Name) {
			continue
		}
		names = append(names, op.Name)
	}
	return names
}

// assistantPlatformToolAllowed 校验 assistant 会话允许调用的平台工具：
// 只读工具始终可用，写入工具按审批模式放行（与目录生成一致）。
func assistantPlatformToolAllowed(exec assistantExecMode, name string) bool {
	op, ok := assistantPublicOperations[name]
	if !ok || loopforgeNoisyTool(name) {
		return false
	}
	if op.ReadOnly {
		return true
	}
	if !exec.AutoWrite {
		return false
	}
	return !exec.ConfirmDangerous || !assistantRiskyToolName(name)
}

func loopforgeScheduledToolNames() []string {
	var names []string
	for _, op := range application.PublicOperations() {
		if loopforgeNoisyTool(op.Name) {
			continue
		}
		names = append(names, op.Name)
	}
	return names
}

func agentToolAllowsAssistantReads(mode AgentToolMode) bool {
	return mode == AgentToolAssistant || mode == AgentToolScheduled
}

func agentToolNameAllowed(mode AgentToolMode, kind, tool string) bool {
	tool = strings.TrimSpace(tool)
	if tool == "" {
		return false
	}
	if mode == AgentToolScheduled && kind != "loopforge" {
		return true
	}
	if allowed := agentToolKindAllowlist(mode, kind); len(allowed) > 0 {
		return allowed[tool]
	}
	if kind == "other" {
		return !clarifyMCPWriteName(tool)
	}
	return false
}

func (h *agentToolGateway) buildCatalog(ctx context.Context, session *AgentToolSession) []agentToolDescriptor {
	if session == nil {
		return nil
	}
	session.mu.Lock()
	if session.terminalOnly {
		session.mu.Unlock()
		return terminalToolDescriptors(session.Mode, session.ExecMode)
	}
	if len(session.catalog) > 0 {
		out := append([]agentToolDescriptor{}, session.catalog...)
		session.mu.Unlock()
		return out
	}
	session.mu.Unlock()

	out := make([]agentToolDescriptor, 0, 16)
	seen := map[string]bool{}
	for _, server := range session.MCPServers {
		kind := classifyClarifyMCP(server)
		prefix := kind
		if kind == "other" {
			prefix = sanitizeAgentToolNamespace(server.ID, server.Name)
		}
		tools, err := h.listUpstreamTools(ctx, session, server)
		if err != nil {
			logAgentToolEvent("catalog-hide", "mode="+string(session.Mode), "kind="+kind, "err="+truncateRunes(err.Error(), 120))
			continue
		}
		for _, tool := range tools {
			if !agentToolNameAllowed(session.Mode, kind, tool.Name) {
				continue
			}
			name := namespaceAgentTool(prefix, tool.Name)
			if seen[name] {
				continue
			}
			seen[name] = true
			desc := strings.TrimSpace(tool.Description)
			if desc == "" {
				desc = server.Name + " " + tool.Name
			}
			out = append(out, agentToolDescriptor{
				Name:         name,
				Description:  desc,
				InputSchema:  defaultToolSchema(tool.InputSchema),
				OutputSchema: tool.OutputSchema,
				Annotations:  tool.Annotations,
				Kind:         kind,
				ServerID:     server.ID,
				UpstreamName: tool.Name,
			})
		}
	}
	if session.Mode == AgentToolAssistant || session.Mode == AgentToolClarify || session.Mode == AgentToolScheduled {
		for _, name := range agentToolKindAllowlistNames(session.Mode, "loopforge") {
			if session.Mode == AgentToolScheduled && session.ExecMode.DeferSOP && assistantDeferredSOPTool(name) {
				continue
			}
			ns := namespaceAgentTool("loopforge", name)
			if seen[ns] {
				continue
			}
			seen[ns] = true
			op, ok := assistantPublicOperations[name]
			if !ok {
				continue
			}
			schema, _ := json.Marshal(platformToolInputSchema(op))
			out = append(out, agentToolDescriptor{
				Name:         ns,
				Description:  op.Description + " (" + op.Method + " " + op.Path + ")",
				InputSchema:  schema,
				Kind:         "loopforge",
				UpstreamName: name,
			})
		}
	}
	// 审批模式放行的平台写入工具（assisted 保存类 / auto 全部），模型直连
	// 调用即生效；confirm 模式目录里没有写工具，模型只能提交草稿。
	if session.Mode == AgentToolAssistant {
		for _, name := range loopforgeAssistantWriteToolNames(session.ExecMode) {
			ns := namespaceAgentTool("loopforge", name)
			if seen[ns] {
				continue
			}
			seen[ns] = true
			op, ok := assistantPublicOperations[name]
			if !ok {
				continue
			}
			schema, _ := json.Marshal(platformToolInputSchema(op))
			out = append(out, agentToolDescriptor{
				Name:         ns,
				Description:  "平台写入工具（已授权直接执行）：" + op.Description + " (" + op.Method + " " + op.Path + ")",
				InputSchema:  schema,
				Kind:         "loopforge",
				UpstreamName: name,
			})
		}
	}
	out = append(out, terminalToolDescriptors(session.Mode, session.ExecMode)...)
	session.mu.Lock()
	session.catalog = append([]agentToolDescriptor{}, out...)
	session.mu.Unlock()
	return out
}

func (h *agentToolGateway) listUpstreamTools(ctx context.Context, session *AgentToolSession, server domain.MCPServerDefinition) ([]mcpclient.Tool, error) {
	key := strings.TrimSpace(server.ID) + "|" + strings.TrimSpace(server.URL)
	if tools, ok := agentToolUpstreamCache.get(key); ok {
		return tools, nil
	}
	caller := session.User
	if server.Scope != "personal" && session.Mode == AgentToolClarify {
		caller = h.clarifyMCPUser(domain.Issue{ProjectID: session.ProjectID}, session.User)
	}
	bound, err := bindAssistantMCP(server, caller)
	if err != nil {
		return nil, err
	}
	client := mcpclient.New(bound.URL, bound.Headers).WithTimeout(20 * time.Second)
	tools, err := client.ListToolsContext(ctx)
	if err != nil {
		return nil, err
	}
	agentToolUpstreamCache.put(key, tools)
	return tools, nil
}

func (c *agentToolCatalogCache) get(key string) ([]mcpclient.Tool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	item, ok := c.items[key]
	if !ok || time.Now().After(item.expiresAt) {
		return nil, false
	}
	return item.tools, true
}

func (c *agentToolCatalogCache) put(key string, tools []mcpclient.Tool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.items == nil {
		c.items = map[string]agentToolCatalogCacheEntry{}
	}
	c.items[key] = agentToolCatalogCacheEntry{tools: tools, expiresAt: time.Now().Add(agentToolCatalogTTL)}
}

func defaultToolSchema(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return json.RawMessage(`{"type":"object"}`)
	}
	return raw
}

func platformToolInputSchema(op application.Operation) map[string]any {
	properties := map[string]any{}
	required := []string{}
	if names := assistantPathParamNames(op.Path); len(names) > 0 {
		pathProps := map[string]any{}
		for _, name := range names {
			pathProps[name] = map[string]any{"type": "string"}
		}
		properties["path"] = map[string]any{"type": "object", "properties": pathProps, "required": names, "additionalProperties": false}
		required = append(required, "path")
	}
	properties["query"] = map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}}
	if op.Method == "POST" || op.Method == "PUT" {
		properties["body"] = map[string]any{"description": "JSON request body"}
	}
	schema := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func lookupAgentTool(catalog []agentToolDescriptor, name string) (agentToolDescriptor, bool) {
	name = stripAgentToolServerPrefix(name)
	for _, item := range catalog {
		if item.Name == name {
			return item, true
		}
	}
	return agentToolDescriptor{}, false
}

func redactAgentToolResult(text string, user domain.User) string {
	text = redactAssistantSecrets(truncateRunes(text, assistantMCPResultLimit), user)
	return redactAuthorizationValues(text)
}

func redactAuthorizationValues(text string) string {
	if text == "" {
		return text
	}
	var b strings.Builder
	lower := strings.ToLower(text)
	start := 0
	for {
		idx := strings.Index(lower[start:], "authorization")
		if idx < 0 {
			b.WriteString(text[start:])
			break
		}
		idx += start
		b.WriteString(text[start:idx])
		rest := text[idx:]
		colon := strings.IndexAny(rest, ":=")
		if colon < 0 || colon > 24 {
			b.WriteString("authorization")
			start = idx + len("authorization")
			continue
		}
		b.WriteString(rest[:colon+1])
		cursor := colon + 1
		for cursor < len(rest) && (rest[cursor] == ' ' || rest[cursor] == '"' || rest[cursor] == '\'') {
			b.WriteByte(rest[cursor])
			cursor++
		}
		b.WriteString("[redacted]")
		for cursor < len(rest) && rest[cursor] != '"' && rest[cursor] != '\'' && rest[cursor] != ',' && rest[cursor] != '}' && rest[cursor] != '\n' && rest[cursor] != '\r' {
			cursor++
		}
		start = idx + cursor
		if start > len(text) {
			break
		}
	}
	return b.String()
}

func mcpToolTextResult(text string, isError bool) map[string]any {
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"isError": isError,
	}
}
