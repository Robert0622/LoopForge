package runtime

import (
	"strings"
	"testing"

	"github.com/Tencent/LoopForge/internal/domain"
)

func TestBindPersonalMCPHeaders(t *testing.T) {
	original := domain.AgentAssetBundle{MCPServers: []domain.MCPServerDefinition{
		{ID: "github", Name: "GitHub", Transport: "http", URL: "https://api.githubcopilot.com/mcp/", AuthType: MCPAuthPersonalToken, Headers: map[string]string{"X-Trace": "asset"}},
		{ID: "public", Name: "Public", Transport: "http", URL: "https://example.com/mcp", Headers: map[string]string{"X-Test": "public"}},
	}}
	bound, err := BindPersonalMCPHeaders(original, "ghp_test_secret")
	if err != nil {
		t.Fatal(err)
	}
	if got := bound.MCPServers[0].Headers["Authorization"]; got != "Bearer ghp_test_secret" {
		t.Fatalf("authorization = %q", got)
	}
	if _, ok := original.MCPServers[0].Headers["Authorization"]; ok {
		t.Fatal("stored asset headers were mutated")
	}
	if _, ok := bound.MCPServers[1].Headers["Authorization"]; ok {
		t.Fatal("credential leaked into non-personal-token MCP")
	}
	bound.MCPServers[0].Headers["X-Trace"] = "changed"
	if original.MCPServers[0].Headers["X-Trace"] != "asset" {
		t.Fatal("header maps were not cloned")
	}
}

func TestBindPersonalMCPHeadersRequiresPAT(t *testing.T) {
	assets := domain.AgentAssetBundle{MCPServers: []domain.MCPServerDefinition{{ID: "github", Name: "GitHub", Transport: "http", URL: "https://example.com", AuthType: MCPAuthPersonalToken}}}
	if !RequiresPersonalMCPToken(assets) {
		t.Fatal("personal-token requirement was not detected")
	}
	_, err := BindPersonalMCPHeaders(assets, "")
	if err == nil || !strings.Contains(err.Error(), "个人凭证") {
		t.Fatalf("missing PAT error = %v", err)
	}
}

func TestPersonalMCPTokenFallsBackToGitHubTokenOnlyForGitHub(t *testing.T) {
	github := domain.MCPServerDefinition{ID: "gh", AuthType: MCPAuthPersonalToken, URL: "https://api.githubcopilot.com/mcp/"}
	other := domain.MCPServerDefinition{ID: "other", AuthType: MCPAuthPersonalToken, URL: "https://mcp.example.com/"}
	if got := PersonalMCPTokenFor(domain.AgentAssetBundle{MCPServers: []domain.MCPServerDefinition{github}}, "", "ghp_git"); got != "ghp_git" {
		t.Fatalf("GitHub MCP should reuse the GitHub token, got %q", got)
	}
	if got := PersonalMCPTokenFor(domain.AgentAssetBundle{MCPServers: []domain.MCPServerDefinition{github, other}}, "", "ghp_git"); got != "" {
		t.Fatalf("GitHub token must not reach a third-party MCP, got %q", got)
	}
	if got := PersonalMCPTokenFor(domain.AgentAssetBundle{MCPServers: []domain.MCPServerDefinition{github}}, "mcp_tok", "ghp_git"); got != "mcp_tok" {
		t.Fatalf("a dedicated MCP token wins, got %q", got)
	}
}
