package runner

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tencent/LoopForge/internal/domain"
)

func TestReadRunnerWorkspaceProvidesBoundedReadOnlyViews(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "runs", "run-safe", "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", workspace}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	git("config", "user.name", "Runner Test")
	git("config", "user.email", "runner@example.com")
	if err := os.WriteFile(filepath.Join(workspace, "main.go"), []byte("package main\n\nfunc value() int { return 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "main.go")
	git("commit", "-qm", "base")
	if err := os.WriteFile(filepath.Join(workspace, "main.go"), []byte("package main\n\nfunc value() int { return 2 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".env"), []byte("SECRET=hidden\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, ".loopforge"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".loopforge", "session.json"), []byte("generated metadata\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	d := &Daemon{cfg: Config{DefaultWorkspaceRoot: root}}
	request := domain.RunnerWorkspaceRequest{RunID: "run-safe", RunStatus: "failed", WorkspaceLayout: domain.RunnerWorkspaceLayoutShared}

	request.Operation = domain.RunnerWorkspaceOperationInfo
	info := d.readRunnerWorkspace(context.Background(), request)
	if info.Error != "" {
		t.Fatal(info.Error)
	}
	var infoPayload struct {
		Available    bool                    `json:"available"`
		Changes      []runnerWorkspaceChange `json:"changes"`
		Repositories []string                `json:"repositories"`
	}
	if err := json.Unmarshal(info.Payload, &infoPayload); err != nil {
		t.Fatal(err)
	}
	if !infoPayload.Available || len(infoPayload.Changes) != 1 || infoPayload.Changes[0].Path != "main.go" {
		t.Fatalf("unexpected info payload: %+v", infoPayload)
	}
	if len(infoPayload.Repositories) != 1 || infoPayload.Repositories[0] != "workspace" || infoPayload.Changes[0].Repository != "workspace" {
		t.Fatalf("root git workspace should report a single repository, got %+v", infoPayload)
	}

	request.Operation = domain.RunnerWorkspaceOperationTree
	tree := d.readRunnerWorkspace(context.Background(), request)
	var treePayload struct {
		Entries []runnerWorkspaceEntry `json:"entries"`
	}
	if err := json.Unmarshal(tree.Payload, &treePayload); err != nil {
		t.Fatal(err)
	}
	for _, entry := range treePayload.Entries {
		if entry.Path == ".env" || entry.Path == ".git" || entry.Path == ".loopforge" || strings.HasPrefix(entry.Path, ".loopforge/") {
			t.Fatalf("sensitive path leaked in tree: %+v", entry)
		}
	}

	request.Operation = domain.RunnerWorkspaceOperationDiff
	request.Path = "main.go"
	diff := d.readRunnerWorkspace(context.Background(), request)
	if diff.Error != "" {
		t.Fatal(diff.Error)
	}
	var diffPayload struct {
		Original string `json:"original"`
		Modified string `json:"modified"`
		Diff     string `json:"diff"`
	}
	if err := json.Unmarshal(diff.Payload, &diffPayload); err != nil {
		t.Fatal(err)
	}
	if diffPayload.Original == diffPayload.Modified || diffPayload.Diff == "" {
		t.Fatalf("expected original and modified diff, got %+v", diffPayload)
	}

	request.Operation = domain.RunnerWorkspaceOperationFile
	request.Path = "../outside"
	if response := d.readRunnerWorkspace(context.Background(), request); response.Error == "" {
		t.Fatal("path traversal should be rejected")
	}
	request.Path = ".env"
	if response := d.readRunnerWorkspace(context.Background(), request); response.Error == "" {
		t.Fatal("sensitive file should be rejected")
	}

	request.Operation = domain.RunnerWorkspaceOperationLog
	request.Path = ""
	logResp := d.readRunnerWorkspace(context.Background(), request)
	if logResp.Error != "" {
		t.Fatal(logResp.Error)
	}
	var logPayload struct {
		Commits []runnerWorkspaceCommit `json:"commits"`
	}
	if err := json.Unmarshal(logResp.Payload, &logPayload); err != nil {
		t.Fatal(err)
	}
	if len(logPayload.Commits) == 0 || logPayload.Commits[0].ID == "" {
		t.Fatalf("unexpected log payload: %+v", logPayload)
	}
	request.Operation = domain.RunnerWorkspaceOperationCommit
	request.Ref = logPayload.Commits[0].ID
	commitResp := d.readRunnerWorkspace(context.Background(), request)
	if commitResp.Error != "" {
		t.Fatal(commitResp.Error)
	}
}

func TestRunnerWorkspaceRejectsUnsafeRunID(t *testing.T) {
	d := &Daemon{cfg: Config{DefaultWorkspaceRoot: t.TempDir()}}
	response := d.readRunnerWorkspace(context.Background(), domain.RunnerWorkspaceRequest{
		RunID: "../escape", Operation: domain.RunnerWorkspaceOperationInfo, WorkspaceLayout: domain.RunnerWorkspaceLayoutShared,
	})
	if response.Error == "" {
		t.Fatal("unsafe run id should be rejected")
	}
}
