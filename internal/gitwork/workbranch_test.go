package gitwork

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "stage-test@example.com"},
		{"config", "user.name", "Stage Test"},
		{"commit", "--allow-empty", "-qm", "baseline"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	return dir
}

func TestStageChangesWithNothingTracked(t *testing.T) {
	dir := gitTestRepo(t)
	if err := StageChanges(dir); err != nil {
		t.Fatalf("StageChanges on a repository with no tracked files: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plan.md"), []byte("plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := StageChanges(dir); err != nil {
		t.Fatalf("StageChanges with a new file: %v", err)
	}
	out, err := exec.Command("git", "-C", dir, "diff", "--cached", "--name-only").Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(out)) != "plan.md" {
		t.Fatalf("staged = %q, want plan.md", out)
	}
}

func TestStageChangesReportsRealFailures(t *testing.T) {
	if err := StageChanges(t.TempDir()); err == nil {
		t.Fatal("StageChanges outside a repository should fail")
	}
}
