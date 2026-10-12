package repo

import (
	"path/filepath"
	"strings"
	"testing"
)

func openEmptyStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open("sqlite://" + filepath.Join(t.TempDir(), "loopforge.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func TestBootstrapSeedsARunnableCatalogOnce(t *testing.T) {
	s := openEmptyStore(t)
	if err := Bootstrap(s, 1); err != nil {
		t.Fatal(err)
	}
	project, ok := s.Project(PlatformProjectID)
	if !ok || project.OwnerID != 1 || project.WorkflowID != QuickFixWorkflowID {
		t.Fatalf("default project = %+v ok=%v", project, ok)
	}
	if _, ok := s.Workflow(QuickFixWorkflowID); !ok {
		t.Fatal("quick-fix SOP must be seeded for first real runs")
	}
	for _, id := range []string{GenericBackendDeliveryWorkflowID, "generic-frontend-delivery", "generic-product-delivery"} {
		if _, ok := s.GenericTemplateByWorkflow(id); !ok {
			t.Fatalf("built-in template %s not seeded", id)
		}
	}
	if _, ok := s.Workflow(DemoWorkflowID); ok {
		t.Fatal("demo SOP must only be seeded in demo mode")
	}

	project.Name = "Renamed by operator"
	if err := s.PutProject(project); err != nil {
		t.Fatal(err)
	}
	if err := Bootstrap(s, 1); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Project(PlatformProjectID); got.Name != "Renamed by operator" {
		t.Fatalf("second bootstrap overwrote operator edits: %q", got.Name)
	}
}

func TestBootstrapUpgradesOlderBuiltinTemplates(t *testing.T) {
	s := openEmptyStore(t)
	if err := Bootstrap(s, 1); err != nil {
		t.Fatal(err)
	}
	shipped, _ := s.GenericTemplateByWorkflow(GenericBackendDeliveryWorkflowID)

	edited := shipped
	edited.Workflow.Description = "edited by operator"
	edited.Workflow.SandboxProfileID = "local"
	if err := s.PutGenericTemplate(edited); err != nil {
		t.Fatal(err)
	}
	if err := Bootstrap(s, 1); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GenericTemplateByWorkflow(GenericBackendDeliveryWorkflowID); got.Workflow.Description != "edited by operator" {
		t.Fatalf("same-version bootstrap overwrote operator edits: %q", got.Workflow.Description)
	}

	edited.Workflow.Version = shipped.Workflow.Version - 1
	if err := s.PutGenericTemplate(edited); err != nil {
		t.Fatal(err)
	}
	if err := Bootstrap(s, 1); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GenericTemplateByWorkflow(GenericBackendDeliveryWorkflowID)
	if got.Workflow.Version != shipped.Workflow.Version || got.Workflow.Description == "edited by operator" {
		t.Fatalf("older stored template was not upgraded: version=%d description=%q", got.Workflow.Version, got.Workflow.Description)
	}
	if got.Workflow.SandboxProfileID != "local" {
		t.Fatalf("upgrade must keep the stored sandbox, got %q", got.Workflow.SandboxProfileID)
	}
}

// A verdict gate only runs after the Agent submitted pass, so a failure means
// the report itself is malformed. Only the reporting node can fix its own
// report; rejecting upstream would loop without converging.
func TestBuiltinVerdictGatesSelfCorrect(t *testing.T) {
	s := openEmptyStore(t)
	if err := Bootstrap(s, 1); err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, id := range []string{GenericBackendDeliveryWorkflowID, "generic-frontend-delivery", "generic-product-delivery"} {
		template, _ := s.GenericTemplateByWorkflow(id)
		for _, agent := range template.Agents {
			for _, gate := range agent.Verification {
				if !strings.HasSuffix(gate.ID, "-verdict") {
					continue
				}
				found++
				if gate.OnFailure != "loop" || !agent.Loop.Enabled || agent.Loop.MaxIterations < 2 || agent.Loop.OnExhausted != "human" {
					t.Errorf("%s/%s: gate onFailure=%q loop=%+v", id, agent.ID, gate.OnFailure, agent.Loop)
				}
			}
		}
	}
	if found != 5 {
		t.Fatalf("expected 5 verdict gates across built-in templates, found %d", found)
	}
}

func TestBootstrapAppliesTemplateSandbox(t *testing.T) {
	SetTemplateSandbox("local")
	t.Cleanup(func() { SetTemplateSandbox("") })
	s := openEmptyStore(t)
	if err := Bootstrap(s, 0); err != nil {
		t.Fatal(err)
	}
	template, ok := s.GenericTemplateByWorkflow(GenericBackendDeliveryWorkflowID)
	if !ok || template.Workflow.SandboxProfileID != "local" {
		t.Fatalf("template sandbox = %q ok=%v", template.Workflow.SandboxProfileID, ok)
	}
	if _, ok := s.Sandbox("local"); !ok {
		t.Fatal("the local sandbox profile the templates point at must exist")
	}
}

func TestDemoModeSeedsARunnableDemoSOP(t *testing.T) {
	SetDemoMode(true)
	t.Cleanup(func() { SetDemoMode(false) })
	if _, ok := BuiltInExecutor(DemoExecutorID); !ok {
		t.Fatal("demo executor must be selectable in demo mode")
	}
	s := openEmptyStore(t)
	if err := Bootstrap(s, 1); err != nil {
		t.Fatal(err)
	}
	wf, ok := s.Workflow(DemoWorkflowID)
	if !ok || wf.Status != "published" || wf.SandboxProfileID != "local" {
		t.Fatalf("demo SOP = %+v ok=%v", wf, ok)
	}
	for _, node := range wf.Nodes {
		if node.Type != "agent" {
			continue
		}
		agent, ok := s.Agent(node.AgentID)
		if !ok || agent.ExecutorID != DemoExecutorID {
			t.Fatalf("demo node %s agent = %+v ok=%v", node.ID, agent, ok)
		}
	}
	if project, _ := s.Project(PlatformProjectID); project.WorkflowID != DemoWorkflowID {
		t.Fatalf("demo project default SOP = %q", project.WorkflowID)
	}

	SetDemoMode(false)
	if _, ok := BuiltInExecutor(DemoExecutorID); ok {
		t.Fatal("demo executor must disappear when demo mode is off")
	}
}
