package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tencent/LoopForge/internal/domain"
	"github.com/Tencent/LoopForge/internal/runtime"
)

func TestRunnerGitEnvironmentUsesAskPassInsteadOfRemoteURL(t *testing.T) {
	workspace := t.TempDir()
	inv := &domain.RunnerInvocation{Repositories: []domain.RunnerRepository{
		{CloneURL: "https://github.com/octo/public.git"},
		{CloneURL: "https://github.com/octo/private.git", GitToken: "tok-123"},
	}}
	env, err := runtime.GitEnvironment(nil, workspace, invocationGitToken(inv))
	if err != nil {
		t.Fatal(err)
	}
	if env["LOOPFORGE_GIT_TOKEN"] != "tok-123" || env["GIT_TERMINAL_PROMPT"] != "0" {
		t.Fatalf("git credential env missing: %v", env)
	}
	askpass := env["GIT_ASKPASS"]
	if askpass != filepath.Join(workspace, ".loopforge", "runtime", "git-askpass") {
		t.Fatalf("askpass path = %q", askpass)
	}
	script, err := os.ReadFile(askpass)
	if err != nil || strings.Contains(string(script), "tok-123") {
		t.Fatalf("askpass script must read the token from env, got %q (%v)", script, err)
	}
	publicEnv, err := runtime.GitEnvironment(nil, workspace, invocationGitToken(&domain.RunnerInvocation{}))
	if err != nil || publicEnv["LOOPFORGE_GIT_TOKEN"] != "" || publicEnv["GIT_ASKPASS"] != "" {
		t.Fatalf("public repositories need no credentials: %v, %v", publicEnv, err)
	}
}
