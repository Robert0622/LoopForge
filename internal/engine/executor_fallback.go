package engine

import (
	"strings"

	"github.com/Tencent/LoopForge/internal/domain"
	"github.com/Tencent/LoopForge/internal/repo"
)

// providerFallbackOrder is the preference used when a node's CLI has no key.
var providerFallbackOrder = []string{"claude-code", "codebuddy", "codex"}

// userProviderKeys reports which built-in agent CLIs the user has a key for.
func userProviderKeys(s *repo.Store, userID int64) map[string]bool {
	keys := map[string]bool{"claude-code": false, "codebuddy": false, "codex": false}
	if s == nil || userID <= 0 {
		return keys
	}
	user, ok := s.User(userID)
	if !ok {
		return keys
	}
	keys["claude-code"] = strings.TrimSpace(user.ClaudeCodeAPIKey) != ""
	keys["codebuddy"] = strings.TrimSpace(user.CodeBuddyAPIKey) != ""
	keys["codex"] = strings.TrimSpace(user.CodexAPIKey) != ""
	return keys
}

// fallbackExecutor swaps a built-in agent CLI the user has no key for to one
// they do, so built-in SOPs run with whichever single key a new user set up.
// The model is cleared because model names are provider specific.
func fallbackExecutor(ex domain.ExecutorDefinition, keys map[string]bool) (domain.ExecutorDefinition, bool) {
	has, builtin := keys[ex.Kind]
	if !builtin || has {
		return ex, false
	}
	for _, kind := range providerFallbackOrder {
		if !keys[kind] {
			continue
		}
		alt, ok := repo.BuiltInExecutorForKind(kind)
		if !ok {
			continue
		}
		alt.Effort = ex.Effort
		return alt, true
	}
	return ex, false
}
