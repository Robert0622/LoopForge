package repo

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Tencent/LoopForge/internal/domain"
	"github.com/Tencent/LoopForge/internal/extsrc"
)

//go:embed templates/*.json
var builtinTemplateFS embed.FS

// PlatformProjectID owns the shared catalog (executors, sandboxes, rules,
// skills, roles) that every project can reference.
const PlatformProjectID = "platform"

// BuiltinGenericTemplates returns the SOP templates shipped with the binary.
func BuiltinGenericTemplates() ([]GenericTemplate, error) {
	entries, err := fs.ReadDir(builtinTemplateFS, "templates")
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	out := make([]GenericTemplate, 0, len(entries))
	for _, entry := range entries {
		raw, err := builtinTemplateFS.ReadFile("templates/" + entry.Name())
		if err != nil {
			return nil, err
		}
		var template GenericTemplate
		if err := json.Unmarshal(raw, &template); err != nil {
			return nil, fmt.Errorf("template %s: %w", entry.Name(), err)
		}
		template.Workflow.Generic = true
		out = append(out, template)
	}
	return out, nil
}

// Bootstrap fills an empty database with a runnable default catalog. Every
// step only inserts what is missing, so it is safe to call on every start and
// never overwrites operator edits. ownerID is the account that owns the
// default project (0 leaves it unowned).
func Bootstrap(s *Store, ownerID int64) error {
	now := time.Now().UTC()
	if len(s.Executors()) == 0 {
		for _, v := range BuiltInExecutors() {
			if err := s.PutExecutor(v); err != nil {
				return err
			}
		}
	}
	if _, ok := s.Dockerfile(GenericBackendDockerfileID); !ok {
		if err := s.PutDockerfile(GenericBackendDockerfile()); err != nil {
			return err
		}
	}
	if len(s.Sandboxes()) == 0 {
		for _, v := range []domain.SandboxProfile{
			{ID: "local", Name: "Local (no isolation)", ProjectID: PlatformProjectID, Shared: true, Driver: "local"},
			{ID: "linux-bwrap", Name: "Linux bubblewrap", ProjectID: PlatformProjectID, Shared: true, Driver: "bwrap"},
			{ID: "macos-seatbelt", Name: "macOS Seatbelt", ProjectID: PlatformProjectID, Shared: true, Driver: "seatbelt"},
			GenericDockerSandbox(),
		} {
			if err := s.PutSandbox(v); err != nil {
				return err
			}
		}
	}
	if len(s.Rules()) == 0 {
		for _, v := range []domain.RuleDefinition{
			{ID: "engineering", Name: "Engineering discipline", ProjectID: PlatformProjectID, Shared: true, Description: "Baseline rules for code changes", Priority: 100, Content: "Read the existing implementation first; keep the change minimal; never skip or weaken tests to hide failures; every completion claim must include fresh verification evidence.", UpdatedAt: now},
			{ID: "security", Name: "Security boundary", ProjectID: PlatformProjectID, Shared: true, Description: "Sandbox safety rules", Priority: 90, Content: "Do not read files outside the workspace; never print credentials; do not bypass access controls; network access must follow the sandbox policy.", UpdatedAt: now},
			{ID: "review", Name: "Review standard", ProjectID: PlatformProjectID, Shared: true, Description: "Independent review criteria", Priority: 100, Content: "Review for correctness, regression risk, security, and test evidence. Every blocking issue must name the file, location, impact, and a suggested fix.", UpdatedAt: now},
		} {
			if err := s.PutRule(v); err != nil {
				return err
			}
		}
	}
	if len(s.Skills()) == 0 {
		for _, v := range []domain.SkillDefinition{
			{ID: "repository-navigation", Name: "Repository navigation", ProjectID: PlatformProjectID, Shared: true, Description: "Locate relevant code and constraints quickly", Content: "# Repository navigation\n\nRead repository-level rules first, use fast text search to find entry points, callers, and tests, then write down a change plan.", UpdatedAt: now},
			{ID: "test-driven-change", Name: "Test-driven change", ProjectID: PlatformProjectID, Shared: true, Description: "Capture failing evidence before implementing", Content: "# Test-driven change\n\nFind the smallest verification scope; reproduce and record the failure when possible; after implementing, run the targeted tests and related regressions.", UpdatedAt: now},
			{ID: "code-review", Name: "Code review", ProjectID: PlatformProjectID, Shared: true, Description: "Structured code review", Content: "# Code review\n\nLook first for issues that affect users, data, or security boundaries. Report only actionable, reproducible findings.", UpdatedAt: now},
		} {
			if err := s.PutSkill(v); err != nil {
				return err
			}
		}
	}
	if len(s.MCPServers()) == 0 {
		github := domain.MCPServerDefinition{
			ID: "mcp-github-project-platform", MarketID: "github", Name: "GitHub", ProjectID: PlatformProjectID, Scope: "project",
			Description: "GitHub's hosted MCP server (issues, pull requests, code search). Authenticates with each run owner's personal MCP token — use a GitHub personal access token.",
			Transport:   "http", URL: "https://api.githubcopilot.com/mcp/", AuthType: "personal-token", TimeoutSec: 60, UpdatedAt: now,
		}
		if err := s.PutMCPServer(github); err != nil {
			return err
		}
	}
	if len(s.Agents()) == 0 {
		for _, v := range []domain.AgentDefinition{
			{ID: "developer", Name: "Developer", Version: 1, ProjectID: PlatformProjectID, Shared: true, ExecutorID: "claude", Description: "Implements changes under the shared rules and skills", Instructions: "Understand the request and the existing code first, then make the smallest verifiable change.", Rules: []string{"engineering", "security"}, Skills: []string{"repository-navigation", "test-driven-change"}, UpdatedAt: now},
			{ID: "reviewer", Name: "Reviewer", Version: 1, ProjectID: PlatformProjectID, Shared: true, ExecutorID: "claude", Description: "Independently reviews the change and its evidence", Instructions: "Check correctness, regression risk, security boundaries, and verification evidence; every rejection must state a concrete reason.", Rules: []string{"review"}, Skills: []string{"code-review"}, UpdatedAt: now},
		} {
			if err := s.PutAgent(v); err != nil {
				return err
			}
		}
	}
	templates, err := BuiltinGenericTemplates()
	if err != nil {
		return err
	}
	for _, template := range templates {
		// Built-in templates follow the external-template rule: a stored copy
		// is replaced only when the binary ships a higher Workflow.Version.
		if stored, ok := s.GenericTemplateByWorkflow(template.Workflow.ID); ok {
			if stored.Workflow.Version >= template.Workflow.Version {
				continue
			}
			template.Workflow.Revision = stored.Workflow.Revision + 1
			template.Workflow.CreatedAt = stored.Workflow.CreatedAt
			template.Workflow.SandboxProfileID = stored.Workflow.SandboxProfileID
		} else if sandbox := TemplateSandbox(); sandbox != "" {
			template.Workflow.SandboxProfileID = sandbox
		}
		if err := s.PutGenericTemplate(template); err != nil {
			return fmt.Errorf("template %s: %w", template.Workflow.ID, err)
		}
	}
	if err := seedExternalTemplates(s); err != nil {
		return err
	}
	if err := seedQuickFix(s, now); err != nil {
		return err
	}
	if err := rebindMissingProductionSOPs(s); err != nil {
		return err
	}
	if len(s.Projects()) == 0 {
		project := domain.Project{
			ID: PlatformProjectID, Key: "LF", Name: "My first project", Description: "Default project created on first start. Rename it or add more projects.",
			Color: "#3867f4", WorkflowID: QuickFixWorkflowID, Clarification: domain.DefaultClarificationPolicy(),
			CreatedAt: now, UpdatedAt: now,
		}
		if DemoMode() {
			project.WorkflowID = DemoWorkflowID
		}
		if ownerID > 0 {
			project.OwnerID = ownerID
			project.Members = []domain.ProjectMember{{UserID: ownerID, Role: "maintainer"}}
		}
		if err := s.PutProject(project); err != nil {
			return err
		}
	}
	if DemoMode() {
		return seedDemo(s, now)
	}
	return nil
}

var templateSources atomic.Value

// SetTemplateSources sets extra SOP template sources seeded by Bootstrap after
// the built-in ones: JSON files, directories of JSON files or http(s) URLs.
func SetTemplateSources(sources []string) {
	templateSources.Store(append([]string(nil), sources...))
}

// ExternalGenericTemplates loads the templates from SetTemplateSources. Each
// document holds one template, an array of them or {"templates": [...]}.
func ExternalGenericTemplates(ctx context.Context) ([]GenericTemplate, error) {
	sources, _ := templateSources.Load().([]string)
	docs, err := extsrc.LoadAll(ctx, sources, ".json")
	if err != nil {
		return nil, fmt.Errorf("SOP templates: %w", err)
	}
	var out []GenericTemplate
	for _, doc := range docs {
		raws, err := extsrc.JSONList(doc.Data, "templates")
		if err != nil {
			return nil, fmt.Errorf("SOP template %s: %w", doc.Name, err)
		}
		for _, raw := range raws {
			var template GenericTemplate
			if err := json.Unmarshal(raw, &template); err != nil {
				return nil, fmt.Errorf("SOP template %s: %w", doc.Name, err)
			}
			template.Workflow.ID = strings.TrimSpace(template.Workflow.ID)
			if template.Workflow.ID == "" {
				return nil, fmt.Errorf("SOP template %s: Workflow.ID is required", doc.Name)
			}
			if len(template.Workflow.Nodes) == 0 {
				return nil, fmt.Errorf("SOP template %s (%s): Workflow.Nodes is empty", doc.Name, template.Workflow.ID)
			}
			if err := validateTemplateRefs(template); err != nil {
				return nil, fmt.Errorf("SOP template %s (%s): %w", doc.Name, template.Workflow.ID, err)
			}
			template.Workflow.Generic = true
			if template.Workflow.Version <= 0 {
				template.Workflow.Version = 1
			}
			out = append(out, template)
		}
	}
	return out, nil
}

func validateTemplateRefs(template GenericTemplate) error {
	if err := domain.ValidateWorkflowMetricStages(template.Workflow); err != nil {
		return err
	}
	agents := map[string]bool{}
	for _, agent := range template.Agents {
		agents[agent.ID] = true
	}
	nodes := map[string]bool{}
	for _, node := range template.Workflow.Nodes {
		nodes[node.ID] = true
		if node.Type == "agent" && !agents[node.AgentID] {
			return fmt.Errorf("node %q uses agent %q, which is not in Agents", node.ID, node.AgentID)
		}
	}
	for _, edge := range template.Workflow.Edges {
		if !nodes[edge.Source] || !nodes[edge.Target] {
			return fmt.Errorf("edge %s -> %s references an unknown node", edge.Source, edge.Target)
		}
	}
	return nil
}

// seedExternalTemplates inserts missing external templates and replaces a
// stored one only when the file declares a higher Workflow.Version, so UI
// edits survive restarts until the maintainers publish a new version.
func seedExternalTemplates(s *Store) error {
	templates, err := ExternalGenericTemplates(context.Background())
	if err != nil {
		return err
	}
	for _, template := range templates {
		if stored, ok := s.GenericTemplateByWorkflow(template.Workflow.ID); ok {
			if stored.Workflow.Version >= template.Workflow.Version {
				continue
			}
			template.Workflow.Revision = stored.Workflow.Revision + 1
			template.Workflow.CreatedAt = stored.Workflow.CreatedAt
		}
		if strings.TrimSpace(template.Workflow.SandboxProfileID) == "" {
			template.Workflow.SandboxProfileID = TemplateSandbox()
		}
		template.Workflow.UpdatedAt = time.Time{}
		if err := s.PutGenericTemplate(template); err != nil {
			return fmt.Errorf("template %s: %w", template.Workflow.ID, err)
		}
	}
	return nil
}

var templateSandbox atomic.Value

// SetTemplateSandbox sets the sandbox profile written into built-in SOP
// templates when they are first seeded. Empty keeps each template's own.
func SetTemplateSandbox(id string) { templateSandbox.Store(strings.TrimSpace(id)) }

func TemplateSandbox() string {
	id, _ := templateSandbox.Load().(string)
	return id
}

const (
	DemoExecutorID     = "mock"
	DemoWorkflowID     = "demo-delivery"
	QuickFixWorkflowID = "quick-fix"
)

// rebindMissingProductionSOPs moves issues off production SOP ids this
// install does not ship, so they can be triggered instead of failing with
// "workflow not found".
func rebindMissingProductionSOPs(s *Store) error {
	for _, issue := range s.Issues() {
		if !IsProductionDefaultWorkflowID(issue.WorkflowID) {
			continue
		}
		if _, ok := s.Workflow(issue.WorkflowID); ok {
			continue
		}
		issue.WorkflowID = ""
		if err := s.PutIssue(issue); err != nil {
			return fmt.Errorf("rebind issue %s: %w", issue.ID, err)
		}
	}
	return nil
}

// seedQuickFix installs a short delivery path: implement → human gate →
// summary. Prefer this over the full generic templates when users want a
// first real run without learning the whole studio.
func seedQuickFix(s *Store, now time.Time) error {
	const version = 2
	if stored, ok := s.Workflow(QuickFixWorkflowID); ok && (stored.Version >= version || stored.Revision > 1) {
		return nil
	}
	return s.PutWorkflow(domain.WorkflowDefinition{
		ID: QuickFixWorkflowID, Name: "快速修复", Version: version, Revision: 1, Status: "published", ProjectID: PlatformProjectID,
		Owner: "平台", Tags: []string{"快速", "短链路"}, SandboxProfileID: TemplateSandbox(),
		Description: "短链路：实现改动 → 人工确认 → 写出小结。适合第一张真实需求；需要模型 API Key 和（可选）GitHub 仓库。",
		Nodes: []domain.WorkflowNode{
			{ID: "implement", Name: "实现改动", Type: "agent", AgentID: "developer", ExecutorID: "claude", MaxAttempts: 2, Position: domain.Position{X: 80, Y: 160},
				ArtifactOutputs: []domain.ArtifactContract{{Name: "change-notes", Path: "change-notes.md", Required: true}}},
			{ID: "approve", Name: "确认改动", Type: "approval", MaxAttempts: 1, RejectTo: "implement", MaxReworks: 3, Position: domain.Position{X: 340, Y: 160}},
			{ID: "summary", Name: "交付小结", Type: "script", ScriptLanguage: "shell", MaxAttempts: 1, Position: domain.Position{X: 600, Y: 160},
				Script:          `printf '# 快速修复小结\n\n- 实现改动：完成\n- 人工确认：已通过\n' > quick-fix-summary.md; if [ -f change-notes.md ]; then printf '\n## 改动说明\n\n' >> quick-fix-summary.md; cat change-notes.md >> quick-fix-summary.md; fi`,
				ArtifactOutputs: []domain.ArtifactContract{{Name: "小结", Path: "quick-fix-summary.md", Required: true}},
			},
		},
		Edges: []domain.WorkflowEdge{
			{ID: "e-implement-approve", Source: "implement", Target: "approve", When: "success"},
			{ID: "e-approve-summary", Source: "approve", Target: "summary", When: "success"},
		},
		CreatedAt: now, UpdatedAt: now,
	})
}

// seedDemo installs a runnable SOP backed by the demo executor. It needs no
// repository, model key or container runtime, so a fresh install can walk
// through dispatch, agent progress, a human approval gate and artifacts.
func seedDemo(s *Store, now time.Time) error {
	for _, v := range []domain.AgentDefinition{
		{ID: "demo-analyst", Name: "演示 · 需求分析", Version: 1, ProjectID: PlatformProjectID, ExecutorID: DemoExecutorID, Description: "演示执行器：模拟拆解需求并给出方案", Instructions: "拆解需求，列出实现要点与验收标准。", UpdatedAt: now},
		{ID: "demo-developer", Name: "演示 · 开发实现", Version: 1, ProjectID: PlatformProjectID, ExecutorID: DemoExecutorID, Description: "演示执行器：模拟按方案完成实现", Instructions: "按确认后的方案实现并自测。", UpdatedAt: now},
	} {
		if _, ok := s.Agent(v.ID); ok {
			continue
		}
		if err := s.PutAgent(v); err != nil {
			return err
		}
	}
	if _, ok := s.Workflow(DemoWorkflowID); ok {
		return nil
	}
	return s.PutWorkflow(domain.WorkflowDefinition{
		ID: DemoWorkflowID, Name: "演示交付流程", Version: 1, Revision: 1, Status: "published", ProjectID: PlatformProjectID,
		Owner: "平台", Tags: []string{"演示"}, SandboxProfileID: "local",
		Description: "无需模型 API Key 与代码仓库即可跑通：需求分析 → 人工确认 → 开发实现 → 生成交付报告。用于体验派发、Agent 进度、审批门禁与产物。",
		Nodes: []domain.WorkflowNode{
			{ID: "analyze", Name: "需求分析", Type: "agent", AgentID: "demo-analyst", ExecutorID: DemoExecutorID, MaxAttempts: 1, Position: domain.Position{X: 80, Y: 160}},
			{ID: "approve", Name: "确认方案", Type: "approval", MaxAttempts: 1, Position: domain.Position{X: 340, Y: 160}},
			{ID: "implement", Name: "开发实现", Type: "agent", AgentID: "demo-developer", ExecutorID: DemoExecutorID, MaxAttempts: 1, Position: domain.Position{X: 600, Y: 160}},
			{
				ID: "report", Name: "交付报告", Type: "script", ScriptLanguage: "shell", MaxAttempts: 1, Position: domain.Position{X: 860, Y: 160},
				Script:          `printf '# 交付报告\n\n- 需求分析：完成\n- 方案确认：已人工批准\n- 开发实现：完成\n' > delivery-report.md`,
				ArtifactOutputs: []domain.ArtifactContract{{Name: "交付报告", Path: "delivery-report.md", Required: true}},
			},
		},
		Edges: []domain.WorkflowEdge{
			{ID: "e-analyze-approve", Source: "analyze", Target: "approve", When: "success"},
			{ID: "e-approve-implement", Source: "approve", Target: "implement", When: "success"},
			{ID: "e-implement-report", Source: "implement", Target: "report", When: "success"},
		},
		CreatedAt: now, UpdatedAt: now,
	})
}
