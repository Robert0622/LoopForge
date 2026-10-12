package engine

import (
	"testing"
	"time"

	"github.com/Tencent/LoopForge/internal/domain"
	"github.com/Tencent/LoopForge/internal/repo"
	"github.com/Tencent/LoopForge/internal/runtime"
)

func TestSucceededRunnerWorkspaceQueuesImmediateDurableCleanup(t *testing.T) {
	store, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	engine := New(store, runtime.NewManager(t.TempDir()))
	runner := domain.Runner{ID: "runner-cleanup", Name: "cleanup runner", Status: domain.RunnerStatusOffline}
	if err := store.PutRunner(runner); err != nil {
		t.Fatal(err)
	}
	run := domain.Run{
		ID: "remote-cleanup", Status: "succeeded",
		ExecutionContext: domain.TelemetryContext{ExecutionMode: domain.ExecutionModeRunner, ExecutionRunnerID: runner.ID},
		Nodes:            map[string]*domain.NodeRun{},
	}
	if err := store.PutRun(run); err != nil {
		t.Fatal(err)
	}

	engine.reconcileTerminalWorkspaceCleanup()

	cleaned, _ := store.Run(run.ID)
	if cleaned.WorkspaceCleanedAt.IsZero() {
		t.Fatal("succeeded runner workspace was not marked unavailable")
	}
	task, ok := store.RunnerTask("rt-cleanup-" + run.ID)
	if !ok || task.Action != domain.RunnerTaskActionCleanupRun || task.RunnerID != runner.ID {
		t.Fatalf("durable runner cleanup was not queued: %+v", task)
	}
}

func TestRetryCancelsQueuedRunnerCleanupAndAllowsLaterCleanup(t *testing.T) {
	store, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	e := New(store, runtime.NewManager(t.TempDir()))
	e.executeRuns = false
	runner := domain.Runner{ID: "runner-replay", Name: "replay runner", Status: domain.RunnerStatusOnline, Runtimes: []domain.RunnerRuntime{{Provider: "terminal", Status: domain.RuntimeStatusOnline}}}
	if err := store.PutRunner(runner); err != nil {
		t.Fatal(err)
	}
	wf := domain.WorkflowDefinition{ID: "replay-remote", Nodes: []domain.WorkflowNode{{ID: "work", Type: "command"}}}
	run := domain.Run{ID: "run-replay-remote", WorkflowID: wf.ID, WorkflowSnapshot: wf, Status: "cancelled", WorkspaceCleanedAt: time.Now().UTC(), ExecutionContext: domain.TelemetryContext{ExecutionMode: domain.ExecutionModeRunner, ExecutionRunnerID: runner.ID}, Nodes: map[string]*domain.NodeRun{"work": {NodeID: "work", Status: "cancelled"}}}
	if err := store.PutRun(run); err != nil {
		t.Fatal(err)
	}
	cleanupID := "rt-cleanup-" + run.ID
	if err := store.PutRunnerTask(domain.RunnerTask{ID: cleanupID, RunnerID: runner.ID, Provider: "terminal", Action: domain.RunnerTaskActionCleanupRun, RunID: run.ID, Status: domain.RunnerTaskQueued}); err != nil {
		t.Fatal(err)
	}
	if err := e.Retry(run.ID, "work", "重跑", "补充上下文"); err != nil {
		t.Fatal(err)
	}
	got, _ := store.Run(run.ID)
	task, _ := store.RunnerTask(cleanupID)
	if got.Status != "running" || !got.WorkspaceCleanedAt.IsZero() || task.Status != domain.RunnerTaskCanceled {
		t.Fatalf("replayed run = %+v, cleanup = %+v", got, task)
	}
	// A previous completed cleanup must not suppress cleanup after this replay.
	task.Status = domain.RunnerTaskSuccess
	if err := store.PutRunnerTask(task); err != nil {
		t.Fatal(err)
	}
	if err := e.queueRunnerRunCleanup(got); err != nil {
		t.Fatal(err)
	}
	task, _ = store.RunnerTask(cleanupID)
	if task.Status != domain.RunnerTaskQueued {
		t.Fatalf("later cleanup = %+v", task)
	}
}

func TestFailedWorkspaceIsNeverCleanedAutomatically(t *testing.T) {
	store, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	engine := New(store, runtime.NewManager(t.TempDir()))
	run := domain.Run{ID: "failed-cleanup", Status: "failed", Nodes: map[string]*domain.NodeRun{}}
	if err := store.PutRun(run); err != nil {
		t.Fatal(err)
	}

	engine.reconcileTerminalWorkspaceCleanup()

	kept, _ := store.Run(run.ID)
	if !kept.WorkspaceCleanedAt.IsZero() {
		t.Fatal("failed workspace was cleaned automatically")
	}
}
