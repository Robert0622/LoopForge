package engine

import (
	"context"
	"testing"

	"github.com/Tencent/LoopForge/internal/domain"
	"github.com/Tencent/LoopForge/internal/repo"
	"github.com/Tencent/LoopForge/internal/runtime"
)

func TestCloudPoolPicksOnlyPlatformRunnersForListedProjects(t *testing.T) {
	s, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	e := New(s, runtime.NewManager(t.TempDir()))
	runners := []domain.Runner{
		{ID: "user-labelled", OwnerUserID: 7, Status: domain.RunnerStatusOnline, RunnerVersion: "0.4.0", Labels: []string{"pool=cloud"}},
		{ID: "old-builtin", Status: domain.RunnerStatusOnline, RunnerVersion: "0.3.0", Labels: []string{"pool=cloud"}},
		{ID: "offline-builtin", Status: domain.RunnerStatusOffline, RunnerVersion: "0.4.0", Labels: []string{"pool=cloud"}},
		{ID: "unlabelled", Status: domain.RunnerStatusOnline, RunnerVersion: "0.4.0"},
		{ID: "builtin", Status: domain.RunnerStatusOnline, RunnerVersion: "0.4.0", Labels: []string{"os=linux", "pool=cloud"}},
	}
	for _, r := range runners {
		if err := s.PutRunner(r); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := e.cloudPoolRunner("e2e"); ok {
		t.Fatal("pool routed a run before it was configured")
	}
	e.ConfigureCloudRunnerPool(CloudRunnerPool{Label: "pool=cloud", Projects: ParseCloudRunnerProjects(" e2e , other ")})
	for i := 0; i < 20; i++ {
		runner, ok := e.cloudPoolRunner("e2e")
		if !ok || runner.ID != "builtin" {
			t.Fatalf("picked %q, %v; want the online platform runner", runner.ID, ok)
		}
	}
	if _, ok := e.cloudPoolRunner("platform"); ok {
		t.Fatal("pool served a project outside the rollout list")
	}
	e.ConfigureCloudRunnerPool(CloudRunnerPool{Label: "pool=cloud", Projects: []string{"*"}})
	if _, ok := e.cloudPoolRunner("platform"); !ok {
		t.Fatal(`"*" did not cover every project`)
	}
}

func TestBindPoolRunnerDiscardsLocalSandbox(t *testing.T) {
	s, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	e := New(s, runtime.NewManager(t.TempDir()))
	e.ConfigureCloudRunnerPool(CloudRunnerPool{Label: "pool=cloud", Projects: []string{"*"}, Required: true})
	if err := s.PutRunner(domain.Runner{ID: "builtin", Status: domain.RunnerStatusOnline, RunnerVersion: "0.4.0", Labels: []string{"pool=cloud"}}); err != nil {
		t.Fatal(err)
	}
	run := domain.Run{
		ID: "run-local", Status: "running",
		ExecutionContext: domain.TelemetryContext{ProjectID: "platform", ExecutionMode: domain.ExecutionModeCloud},
		Nodes:            map[string]*domain.NodeRun{"fix": {NodeID: "fix", Status: "running"}},
	}
	if err := s.PutRun(run); err != nil {
		t.Fatal(err)
	}
	sandbox := &recordingSandbox{workspace: e.sandboxes.RunWorkspace(run.ID)}
	e.runEnvs[run.ID] = &runEnvironment{sandbox: sandbox}

	bound, err := e.bindPoolRunner(context.Background(), run, "fix")
	if err != nil {
		t.Fatal(err)
	}
	if bound.ExecutionContext.ExecutionRunnerID != "builtin" {
		t.Fatalf("runner=%q", bound.ExecutionContext.ExecutionRunnerID)
	}
	if !sandbox.closed {
		t.Fatal("moving a run onto the pool runner must remove the control-plane container")
	}
}
