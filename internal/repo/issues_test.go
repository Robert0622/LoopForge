package repo

import (
	"testing"
	"time"

	"github.com/Tencent/LoopForge/internal/domain"
)

func TestDefaultWorkflowID(t *testing.T) {
	if got := DefaultWorkflowID("github", ""); got != issueFixWorkflowID {
		t.Fatalf("forge default: %s", got)
	}
	if got := DefaultWorkflowID("github", "requirement"); got != requirementWorkflowID {
		t.Fatalf("imported requirement default: %s", got)
	}
	if got := DefaultWorkflowID("local", "issue"); got != issueFixWorkflowID {
		t.Fatalf("local issue default: %s", got)
	}
}

func TestPutIssueDefaultsWorkflowWithoutOverwritingExplicitSelection(t *testing.T) {
	s, err := OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.PutIssue(domain.Issue{ID: "gf-1", ProjectID: "platform", Source: "github", Title: "bug", WorkflowID: requirementWorkflowID}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutIssue(domain.Issue{ID: "req-1", ProjectID: "platform", Source: "local", Title: "story", WorkflowID: issueFixWorkflowID}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutIssue(domain.Issue{ID: "local-1", ProjectID: "platform", Title: "local"}); err != nil {
		t.Fatal(err)
	}
	gf, _ := s.Issue("gf-1")
	if gf.WorkflowID != requirementWorkflowID || gf.Kind != "issue" {
		t.Fatalf("forge: %+v", gf)
	}
	req, _ := s.Issue("req-1")
	if req.WorkflowID != issueFixWorkflowID {
		t.Fatalf("requirement: %+v", req)
	}
	local, _ := s.Issue("local-1")
	if local.WorkflowID != requirementWorkflowID {
		t.Fatalf("local: %+v", local)
	}
}

func TestPutIssueClassifiesTrackerClosureWithoutSourceHint(t *testing.T) {
	s, err := OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	issue := domain.Issue{
		ID: "gf-closed-task", ProjectID: "platform", Source: "github", Status: "todo",
		ExternalRef: &domain.ExternalRef{Source: "github", ExternalState: "opened"},
	}
	if err := s.PutIssue(issue); err != nil {
		t.Fatal(err)
	}
	issue.Status = "closed" // A task refresh can set status before a tracker pull.
	issue.ExternalRef.ExternalState = "closed"
	if err := s.PutIssue(issue); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Issue(issue.ID)
	if got.Status != "closed" || got.CloseSource != "external" {
		t.Fatalf("tracker closure classified as manual: %+v", got)
	}
}

func TestPutIssueUsesProjectDefaultRequirementWorkflow(t *testing.T) {
	s, err := OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	const projectSOP = "igtm-requirement-dev"
	if err := s.PutProject(domain.Project{ID: "igtm", Key: "IGTM", Name: "igtm", WorkflowID: projectSOP}); err != nil {
		t.Fatal(err)
	}

	// New requirement uses the project's configured default SOP.
	if err := s.PutIssue(domain.Issue{ID: "req-1", ProjectID: "igtm", Source: "local", Title: "story"}); err != nil {
		t.Fatal(err)
	}
	if req, _ := s.Issue("req-1"); req.WorkflowID != projectSOP {
		t.Fatalf("requirement should use project default %q, got %+v", projectSOP, req)
	}

	// Local requirement (no source) also uses the project default SOP.
	if err := s.PutIssue(domain.Issue{ID: "req-2", ProjectID: "igtm", Title: "local requirement"}); err != nil {
		t.Fatal(err)
	}
	if req, _ := s.Issue("req-2"); req.WorkflowID != projectSOP {
		t.Fatalf("local requirement should use project default %q, got %+v", projectSOP, req)
	}

	// Defects (forge) stay on the production repair loop unless the project
	// has its own {projectId}-issue-fix SOP.
	if err := s.PutIssue(domain.Issue{ID: "bug-1", ProjectID: "igtm", Source: "github", Title: "bug"}); err != nil {
		t.Fatal(err)
	}
	if bug, _ := s.Issue("bug-1"); bug.WorkflowID != issueFixWorkflowID || bug.Kind != "issue" {
		t.Fatalf("forge defect should use repair loop, got %+v", bug)
	}

	if err := s.PutProject(domain.Project{ID: "shop", Key: "SHOP", Name: "SHOP"}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutWorkflow(domain.WorkflowDefinition{ID: "shop-issue-fix", Name: "SHOP Issue", SandboxProfileID: "local"}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutIssue(domain.Issue{ID: "bug-shop", ProjectID: "shop", Source: "github", Title: "bug"}); err != nil {
		t.Fatal(err)
	}
	if bug, _ := s.Issue("bug-shop"); bug.WorkflowID != "shop-issue-fix" {
		t.Fatalf("shop defect should use shop-issue-fix, got %+v", bug)
	}

	// Explicit selection is never overwritten by the project default.
	if err := s.PutIssue(domain.Issue{ID: "req-3", ProjectID: "igtm", Source: "local", Title: "explicit", WorkflowID: "custom-flow"}); err != nil {
		t.Fatal(err)
	}
	if req, _ := s.Issue("req-3"); req.WorkflowID != "custom-flow" {
		t.Fatalf("explicit workflow was overwritten: %+v", req)
	}

	// Project without a configured SOP falls back to the production default.
	if err := s.PutProject(domain.Project{ID: "bare", Key: "BARE", Name: "bare"}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutIssue(domain.Issue{ID: "req-4", ProjectID: "bare", Source: "local", Title: "story"}); err != nil {
		t.Fatal(err)
	}
	if req, _ := s.Issue("req-4"); req.WorkflowID != requirementWorkflowID {
		t.Fatalf("bare project requirement should fall back to %q, got %+v", requirementWorkflowID, req)
	}
}

// 新项目默认 SOP 是内置通用模板（后端交付）时，同步/新建的需求与
// GitHub 缺陷都默认使用它，保证开箱即用。
func TestPutIssueBindsGenericProjectDefaultForAllIssues(t *testing.T) {
	s, err := OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.PutProject(domain.Project{ID: "newproj", Key: "NEW", Name: "new", WorkflowID: GenericBackendDeliveryWorkflowID}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutIssue(domain.Issue{ID: "req-np", ProjectID: "newproj", Source: "local", Title: "story"}); err != nil {
		t.Fatal(err)
	}
	if req, _ := s.Issue("req-np"); req.WorkflowID != GenericBackendDeliveryWorkflowID {
		t.Fatalf("requirement should use generic project default, got %+v", req)
	}
	if err := s.PutIssue(domain.Issue{ID: "bug-np", ProjectID: "newproj", Source: "github", Title: "bug"}); err != nil {
		t.Fatal(err)
	}
	if bug, _ := s.Issue("bug-np"); bug.WorkflowID != GenericBackendDeliveryWorkflowID || bug.Kind != "issue" {
		t.Fatalf("forge issue should use generic project default, got %+v", bug)
	}
	// 项目专属缺陷 SOP 仍然优先于项目通用默认。
	if err := s.PutWorkflow(domain.WorkflowDefinition{ID: "newproj-issue-fix", Name: "Newproj fix", SandboxProfileID: "local"}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutIssue(domain.Issue{ID: "bug-np-2", ProjectID: "newproj", Source: "github", Title: "bug 2"}); err != nil {
		t.Fatal(err)
	}
	if bug, _ := s.Issue("bug-np-2"); bug.WorkflowID != "newproj-issue-fix" {
		t.Fatalf("project issue-fix SOP should win, got %+v", bug)
	}
}

func TestPutIssuePreservesDeliveryFieldsOnSyncOverwrite(t *testing.T) {
	s, err := OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	deliveredAt := time.Now().UTC()
	if err := s.PutIssue(domain.Issue{
		ID:                "gf-5",
		ProjectID:         "platform",
		Source:            "github",
		Title:             "delivered",
		Status:            "done",
		DeliveryState:     "merged",
		DeliveryMRURL:     "https://github.com/org/repo/pull/5",
		DeliveryMRState:   "merged",
		DeliveryUpdatedAt: deliveredAt,
		ExternalRef:       &domain.ExternalRef{Source: "github", ExternalIID: 5, ExternalState: "closed"},
	}); err != nil {
		t.Fatal(err)
	}
	// Forge pull snapshot without delivery fields (sync race).
	if err := s.PutIssue(domain.Issue{
		ID:          "gf-5",
		ProjectID:   "platform",
		Source:      "github",
		Title:       "delivered",
		Status:      "done",
		ExternalRef: &domain.ExternalRef{Source: "github", ExternalIID: 5, ExternalState: "closed"},
	}); err != nil {
		t.Fatal(err)
	}
	got, ok := s.Issue("gf-5")
	if !ok || got.DeliveryState != "merged" || got.DeliveryMRState != "merged" {
		t.Fatalf("delivery fields lost: %+v", got)
	}
}

func TestRestoreTrackerBindingFromStableID(t *testing.T) {
	var issue domain.Issue
	RestoreTrackerBinding(&issue, domain.Issue{ID: "gh-repo-1-42"})
	if !IssueBoundToTracker(issue) || issue.Source != "github" || issue.ExternalRef.ExternalIID != 42 {
		t.Fatalf("github restore: %+v", issue)
	}
	bound := domain.Issue{Source: "github", Kind: "requirement", ExternalRef: &domain.ExternalRef{Source: "github", ExternalID: "keep-me", ExternalURL: "https://github.com/acme/app/issues/1"}}
	wiped := domain.Issue{ID: bound.ID}
	RestoreTrackerBinding(&wiped, bound)
	if wiped.ExternalRef == nil || wiped.ExternalRef.ExternalID != "keep-me" {
		t.Fatalf("existing snapshot should win: %+v", wiped)
	}
}

func TestPutIssueNeverBindsToAMissingProductionSOP(t *testing.T) {
	s, err := OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.PutWorkflow(domain.WorkflowDefinition{ID: QuickFixWorkflowID, Name: "快速修复", SandboxProfileID: "local"}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutProject(domain.Project{ID: "oss", Key: "OSS", Name: "oss", WorkflowID: DemoWorkflowID}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutIssue(domain.Issue{ID: "bug-oss", ProjectID: "oss", Source: "github", Title: "synced bug"}); err != nil {
		t.Fatal(err)
	}
	if bug, _ := s.Issue("bug-oss"); bug.WorkflowID != QuickFixWorkflowID {
		t.Fatalf("synced bug should fall back to the built-in quick-fix SOP, got %q", bug.WorkflowID)
	}
}

func TestBootstrapRebindsIssuesOffMissingProductionSOPs(t *testing.T) {
	s, err := OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.PutWorkflow(domain.WorkflowDefinition{ID: QuickFixWorkflowID, Name: "快速修复", SandboxProfileID: "local"}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutIssue(domain.Issue{ID: "old-bug", Source: "github", Title: "synced before upgrade", WorkflowID: issueFixWorkflowID}); err != nil {
		t.Fatal(err)
	}
	if err := rebindMissingProductionSOPs(s); err != nil {
		t.Fatal(err)
	}
	if bug, _ := s.Issue("old-bug"); bug.WorkflowID != QuickFixWorkflowID {
		t.Fatalf("stale production SOP binding should move to quick-fix, got %q", bug.WorkflowID)
	}
}

func TestSeedQuickFixUpgradesUntouchedCopyWithReworkTarget(t *testing.T) {
	s, err := OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.PutWorkflow(domain.WorkflowDefinition{ID: QuickFixWorkflowID, Name: "快速修复", Version: 1, Revision: 1, SandboxProfileID: "local",
		Nodes: []domain.WorkflowNode{{ID: "implement", Type: "agent"}, {ID: "approve", Type: "approval"}}}); err != nil {
		t.Fatal(err)
	}
	if err := seedQuickFix(s, time.Now()); err != nil {
		t.Fatal(err)
	}
	wf, _ := s.Workflow(QuickFixWorkflowID)
	var approve domain.WorkflowNode
	for _, n := range wf.Nodes {
		if n.ID == "approve" {
			approve = n
		}
	}
	if approve.RejectTo != "implement" || wf.Version != 2 {
		t.Fatalf("untouched quick-fix should upgrade so its gate can send work back, got version %d rejectTo %q", wf.Version, approve.RejectTo)
	}
	wf.Name = "我的快速修复"
	if err := s.PutWorkflow(wf); err != nil {
		t.Fatal(err)
	}
	if err := seedQuickFix(s, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Workflow(QuickFixWorkflowID); got.Name != "我的快速修复" {
		t.Fatalf("a user-edited quick-fix must be kept, got %q", got.Name)
	}
}
