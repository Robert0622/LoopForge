package repo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const externalTemplateJSON = `{
  "Workflow": {"id": "team-hotfix", "name": "Team hotfix", "version": %VERSION%,
    "nodes": [
      {"id": "fix", "type": "agent", "name": "Fix", "agentId": "hotfix-dev"},
      {"id": "check", "type": "agent", "name": "Check", "agentId": "hotfix-review"}
    ],
    "edges": [{"id": "e1", "source": "fix", "target": "check", "when": "success"}]},
  "Agents": [
    {"id": "hotfix-dev", "name": "Hotfix developer", "executorId": "claude"},
    {"id": "hotfix-review", "name": "Hotfix reviewer", "executorId": "claude"}
  ]
}`

func writeTemplate(t *testing.T, dir, version string) {
	t.Helper()
	body := strings.ReplaceAll(externalTemplateJSON, "%VERSION%", version)
	if err := os.WriteFile(filepath.Join(dir, "hotfix.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBootstrapSeedsAndUpgradesExternalTemplates(t *testing.T) {
	t.Cleanup(func() { SetTemplateSources(nil) })
	dir := t.TempDir()
	writeTemplate(t, dir, "1")
	SetTemplateSources([]string{dir})
	s, err := Open(filepath.Join(t.TempDir(), "af.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := Bootstrap(s, 0); err != nil {
		t.Fatal(err)
	}
	got, ok := s.GenericTemplateByWorkflow("team-hotfix")
	if !ok || !got.Workflow.Generic || len(got.Agents) != 2 || got.Workflow.Version != 1 {
		t.Fatalf("seeded template = %+v ok=%v", got.Workflow, ok)
	}
	if _, ok := s.GenericTemplateByWorkflow(GenericBackendDeliveryWorkflowID); !ok {
		t.Fatal("built-in templates must still be seeded")
	}

	edited := got.Workflow
	edited.Description = "edited in the UI"
	if _, err := s.UpdateGenericWorkflow(edited); err != nil {
		t.Fatal(err)
	}
	if err := Bootstrap(s, 0); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GenericTemplateByWorkflow("team-hotfix")
	if got.Workflow.Description != "edited in the UI" {
		t.Fatal("restart with the same version must keep UI edits")
	}

	writeTemplate(t, dir, "2")
	if err := Bootstrap(s, 0); err != nil {
		t.Fatal(err)
	}
	upgraded, _ := s.GenericTemplateByWorkflow("team-hotfix")
	if upgraded.Workflow.Version != 2 || upgraded.Workflow.Description != "" || upgraded.Workflow.Revision <= got.Workflow.Revision {
		t.Fatalf("version bump should replace the template: %+v (was revision %d)", upgraded.Workflow, got.Workflow.Revision)
	}
}

func TestExternalTemplatesRejectBrokenReferences(t *testing.T) {
	t.Cleanup(func() { SetTemplateSources(nil) })
	dir := t.TempDir()
	cases := map[string]string{
		"unknown agent": strings.ReplaceAll(externalTemplateJSON, `"agentId": "hotfix-review"`, `"agentId": "ghost"`),
		"unknown node":  strings.ReplaceAll(externalTemplateJSON, `"target": "check"`, `"target": "ghost"`),
		"no id":         strings.ReplaceAll(externalTemplateJSON, `"id": "team-hotfix", `, ""),
	}
	for name, body := range cases {
		body = strings.ReplaceAll(body, "%VERSION%", "1")
		path := filepath.Join(dir, strings.ReplaceAll(name, " ", "-")+".json")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		SetTemplateSources([]string{path})
		if _, err := ExternalGenericTemplates(t.Context()); err == nil {
			t.Fatalf("%s: expected error", name)
		}
	}
}
