package engine

import (
	"testing"

	"github.com/Tencent/LoopForge/internal/domain"
	"github.com/Tencent/LoopForge/internal/repo"
)

func TestFallbackExecutorUsesTheConfiguredProvider(t *testing.T) {
	claude, _ := repo.BuiltInExecutor("claude")
	claude.Model = "claude-sonnet"

	got, swapped := fallbackExecutor(claude, map[string]bool{"claude-code": false, "codebuddy": true, "codex": false})
	if !swapped || got.Kind != "codebuddy" || got.Model != "" {
		t.Fatalf("expected a switch to CodeBuddy without the Claude model, got %+v swapped=%v", got, swapped)
	}

	same, swapped := fallbackExecutor(claude, map[string]bool{"claude-code": true, "codebuddy": true})
	if swapped || same.Kind != "claude-code" {
		t.Fatalf("a configured provider must be kept, got %+v", same)
	}

	none, swapped := fallbackExecutor(claude, map[string]bool{"claude-code": false, "codebuddy": false, "codex": false})
	if swapped || none.Kind != "claude-code" {
		t.Fatalf("without any key the original error path must be kept, got %+v", none)
	}

	mock := domain.ExecutorDefinition{ID: "mock", Kind: "mock"}
	if _, swapped := fallbackExecutor(mock, map[string]bool{"codebuddy": true}); swapped {
		t.Fatal("non-CLI executors must never be swapped")
	}
}
