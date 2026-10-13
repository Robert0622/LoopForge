package runtime

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/Tencent/LoopForge/internal/domain"
)

const MCPAuthPersonalToken = "personal-token"

// BindPersonalMCPHeaders returns a task-private copy of the asset bundle and
// injects the responsible user's personal MCP token only into MCPs that explicitly opt
// in to personal-token authentication. Stored MCP assets are never mutated.
func BindPersonalMCPHeaders(assets domain.AgentAssetBundle, personalMCPToken string) (domain.AgentAssetBundle, error) {
	out := assets
	out.MCPServers = append([]domain.MCPServerDefinition(nil), assets.MCPServers...)
	for i := range out.MCPServers {
		m := &out.MCPServers[i]
		if strings.TrimSpace(m.AuthType) != MCPAuthPersonalToken {
			continue
		}
		token := strings.TrimSpace(personalMCPToken)
		if token == "" {
			return domain.AgentAssetBundle{}, fmt.Errorf("MCP %s 需要个人 MCP 令牌：请先在个人凭证中配置", m.Name)
		}
		m.Headers = cloneHeaders(m.Headers)
		for key := range m.Headers {
			if strings.EqualFold(strings.TrimSpace(key), "authorization") {
				delete(m.Headers, key)
			}
		}
		m.Headers["Authorization"] = "Bearer " + token
	}
	return out, nil
}

// PersonalMCPTokenFor returns the token to bind into personal-token MCPs. When
// the user set no dedicated MCP token and every such MCP is GitHub's own
// server, their GitHub token serves both: it is never sent to another host.
func PersonalMCPTokenFor(assets domain.AgentAssetBundle, personalMCPToken, gitHubToken string) string {
	if strings.TrimSpace(personalMCPToken) != "" || strings.TrimSpace(gitHubToken) == "" {
		return personalMCPToken
	}
	found := false
	for _, m := range assets.MCPServers {
		if strings.TrimSpace(m.AuthType) != MCPAuthPersonalToken {
			continue
		}
		if !isGitHubMCPServer(m) {
			return personalMCPToken
		}
		found = true
	}
	if !found {
		return personalMCPToken
	}
	return gitHubToken
}

func isGitHubMCPServer(m domain.MCPServerDefinition) bool {
	u, err := url.Parse(strings.TrimSpace(m.URL))
	if err != nil || u.Scheme != "https" {
		return false
	}
	switch strings.ToLower(u.Hostname()) {
	case "api.githubcopilot.com", "api.github.com", "github.com":
		return true
	}
	return false
}

func RequiresPersonalMCPToken(assets domain.AgentAssetBundle) bool {
	for _, m := range assets.MCPServers {
		if strings.TrimSpace(m.AuthType) == MCPAuthPersonalToken {
			return true
		}
	}
	return false
}

func cloneHeaders(in map[string]string) map[string]string {
	out := make(map[string]string, len(in)+1)
	for k, v := range in {
		out[k] = v
	}
	return out
}
