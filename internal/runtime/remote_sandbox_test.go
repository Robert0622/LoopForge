package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/Tencent/LoopForge/internal/domain"
)

func TestOpenRemoteSandboxUsesExactRunnerWorkspace(t *testing.T) {
	workspace := t.TempDir()
	sandbox, err := OpenRemoteSandbox(context.Background(), "runner-task", workspace, domain.SandboxProfile{Driver: "local"}, SandboxRequirements{}, SandboxResources{})
	if err != nil {
		t.Fatal(err)
	}
	defer sandbox.Close(context.Background())
	if sandbox.Workspace() != workspace {
		t.Fatalf("workspace = %q, want %q", sandbox.Workspace(), workspace)
	}
	command := WithCommandEnvironment(CommandSpec{Argv: []string{"sh", "-lc", `printf '%s' "$RUNNER_SANDBOX_MARKER"`}}, map[string]string{"RUNNER_SANDBOX_MARKER": "runner-owned"})
	output, err := sandbox.Exec(context.Background(), command, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(output) != "runner-owned" {
		t.Fatalf("output = %q", output)
	}
}
