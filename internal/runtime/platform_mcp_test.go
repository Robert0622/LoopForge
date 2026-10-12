package runtime

import (
	"strings"
	"testing"

	"github.com/Tencent/LoopForge/internal/domain"
)

func TestPrepareMCPServersRebasesForExecutionMachines(t *testing.T) {
	servers := []domain.MCPServerDefinition{
		{ID: domain.PlatformMCPServerID, URL: "/mcp"},
		{ID: "third-party", URL: "http://kb.example/mcp"},
	}
	got, warnings, err := PrepareMCPServers(domain.AgentAssetBundle{MCPServers: servers}, MCPTarget{ServerURL: "https://loopforge.example/"})
	if err != nil || len(warnings) != 0 {
		t.Fatalf("err = %v, warnings = %v", err, warnings)
	}
	want := []string{"https://loopforge.example/mcp", "http://kb.example/mcp"}
	for i := range want {
		if got.MCPServers[i].URL != want[i] {
			t.Fatalf("server %d URL = %q, want %q", i, got.MCPServers[i].URL, want[i])
		}
	}
	if servers[0].URL != "/mcp" {
		t.Fatal("prepare mutated the caller's slice")
	}
}

func TestPrepareMCPServersDropsUnaddressableForCloud(t *testing.T) {
	got, warnings, err := PrepareMCPServers(domain.AgentAssetBundle{MCPServers: []domain.MCPServerDefinition{
		{ID: domain.PlatformMCPServerID, URL: "/mcp"},
		{ID: "relative-third-party", URL: "/not-ours"},
	}}, MCPTarget{})
	if err != nil || len(got.MCPServers) != 1 || got.MCPServers[0].ID != "relative-third-party" {
		t.Fatalf("servers = %#v, err = %v", got.MCPServers, err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], domain.PlatformMCPServerID) || !strings.Contains(warnings[0], "LOOPFORGE_PLATFORM_MCP_URL") {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestPrepareMCPServersPersonalTokenPolicy(t *testing.T) {
	assets := domain.AgentAssetBundle{MCPServers: []domain.MCPServerDefinition{{ID: "github", Name: "GitHub", AuthType: MCPAuthPersonalToken, URL: "https://api.githubcopilot.com/mcp/"}}}
	if _, _, err := PrepareMCPServers(assets, MCPTarget{RequirePersonalMCPToken: true}); err == nil {
		t.Fatal("agent invocation without a personal MCP token must fail")
	}
	got, _, err := PrepareMCPServers(assets, MCPTarget{})
	if err != nil || got.MCPServers[0].Headers["Authorization"] != "" {
		t.Fatalf("optional bind without token = %#v, %v", got.MCPServers, err)
	}
	got, _, err = PrepareMCPServers(assets, MCPTarget{PersonalMCPToken: "pat", RequirePersonalMCPToken: true})
	if err != nil || got.MCPServers[0].Headers["Authorization"] != "Bearer pat" {
		t.Fatalf("strict bind = %#v, %v", got.MCPServers, err)
	}
}
