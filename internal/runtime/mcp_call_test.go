package runtime

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tencent/LoopForge/internal/domain"
)

func TestMain(m *testing.M) {
	if RunMCPCallCLI(os.Args[1:]) {
		return
	}
	os.Exit(m.Run())
}

func TestBindPersonalMCPHeadersOptionalSkipsMissingPAT(t *testing.T) {
	assets := domain.AgentAssetBundle{MCPServers: []domain.MCPServerDefinition{
		{ID: "wiki", Name: "Wiki", Transport: "http", URL: "https://example.com", AuthType: MCPAuthPersonalToken},
	}}
	bound := BindPersonalMCPHeadersOptional(assets, "")
	if _, ok := bound.MCPServers[0].Headers["Authorization"]; ok {
		t.Fatal("missing PAT should not invent an Authorization header")
	}
	bound = BindPersonalMCPHeadersOptional(assets, "tok")
	if bound.MCPServers[0].Headers["Authorization"] != "Bearer tok" {
		t.Fatalf("authorization = %q", bound.MCPServers[0].Headers["Authorization"])
	}
}

func TestMCPCallAliasesMatchURLAndInstallID(t *testing.T) {
	def := domain.MCPServerDefinition{
		ID: "mcp-linear-project-dns", Name: "Linear", MarketID: "linear", URL: "https://mcp.example.com/issues",
	}
	server := mcpCallServer{ID: def.ID, Name: def.Name, MarketID: def.MarketID, URL: def.URL, Aliases: mcpCallAliases(def)}
	got, err := matchMCPCallServer([]mcpCallServer{server}, "issues")
	if err != nil || got.ID != server.ID {
		t.Fatalf("issues = %+v err=%v aliases=%v", got, err, server.Aliases)
	}
	got, err = matchMCPCallServer([]mcpCallServer{server}, "linear")
	if err != nil || got.ID != server.ID {
		t.Fatalf("linear = %+v err=%v", got, err)
	}
}

func TestMCPCallCLIInvokesBoundHTTPServer(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ghp_test" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req struct {
			ID     int64  `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "initialize":
			w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/call":
			w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"env-ok"}]}}`))
		default:
			t.Fatalf("method %s", req.Method)
		}
	}))
	t.Cleanup(gateway.Close)

	workspace := t.TempDir()
	sb := &localSandbox{workspace: workspace}
	assets := BindPersonalMCPHeadersOptional(domain.AgentAssetBundle{MCPServers: []domain.MCPServerDefinition{{
		ID: "mcp-linear-project-dns", Name: "Linear", MarketID: "linear", Transport: "http",
		URL: gateway.URL, AuthType: MCPAuthPersonalToken,
	}}}, "ghp_test")
	env, err := ScriptMCPEnvironment(sb, workspace, assets.MCPServers)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range env {
		t.Setenv(key, value)
	}
	stdout, stderr, code := captureMCPCall(t, []string{"call", "linear", "list_issues", `{"query":"open"}`})
	if code != 0 {
		t.Fatalf("exit %d stderr=%q stdout=%q", code, stderr, stdout)
	}
	if strings.TrimSpace(stdout) != "env-ok" {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestMCPCallRequiresPersonalMCPTokenAtCallTime(t *testing.T) {
	workspace := t.TempDir()
	sb := &localSandbox{workspace: workspace}
	assets := BindPersonalMCPHeadersOptional(domain.AgentAssetBundle{MCPServers: []domain.MCPServerDefinition{{
		ID: "wiki", Name: "Wiki", Transport: "http", URL: "https://example.com/mcp", AuthType: MCPAuthPersonalToken,
	}}}, "")
	env, err := ScriptMCPEnvironment(sb, workspace, assets.MCPServers)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range env {
		t.Setenv(key, value)
	}
	_, stderr, code := captureMCPCall(t, []string{"call", "wiki", "search"})
	if code != 1 || !strings.Contains(stderr, "个人 MCP 令牌") {
		t.Fatalf("exit %d stderr = %q", code, stderr)
	}
}

func TestBindMCPWritesWorkspaceBinWithoutPATH(t *testing.T) {
	workspace := t.TempDir()
	sb := &localSandbox{workspace: workspace, environment: map[string]string{"PATH": "/usr/bin"}}
	command := CommandSpec{Argv: []string{"sh", "-lc", "loopforge mcp list"}, CredentialBindings: []CredentialBinding{BindMCP}}
	bound, err := bindCommand(sb, workspace, command, RunCredentials{MCPServers: []domain.MCPServerDefinition{{
		ID: "tracker", Name: "Tracker", Transport: "http", URL: "https://example.com/tracker",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := bound.environment["PATH"]; ok {
		t.Fatalf("local unisolated MCP bind must not rewrite PATH, got %q", bound.environment["PATH"])
	}
	helper := filepath.Join(workspace, ".loopforge", "runtime", "bin", "loopforge")
	if _, err := os.Stat(helper); err != nil {
		t.Fatal(err)
	}
	if bound.environment[mcpCallBinEnv] != helper {
		t.Fatalf("LOOPFORGE_BIN = %q want %q", bound.environment[mcpCallBinEnv], helper)
	}
	if bound.environment[mcpCallConfigEnv] == "" {
		t.Fatal("missing MCP config env")
	}
	if len(bound.Argv) < 3 || !strings.Contains(bound.Argv[2], "loopforge()") {
		t.Fatalf("local shell should define loopforge function, argv=%v", bound.Argv)
	}
	raw, err := os.ReadFile(bound.environment[mcpCallConfigEnv])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "Authorization") && strings.Contains(string(raw), "Bearer") {
		t.Fatal("unbound custom MCP should not carry a personal-token bearer")
	}
}

func TestDockerMCPCallPointsAtWorkspaceBin(t *testing.T) {
	workspace := t.TempDir()
	sb := &localSandbox{workspace: workspace, driver: "docker"}
	env, err := ScriptMCPEnvironment(sb, workspace, []domain.MCPServerDefinition{{
		ID: "tracker", Name: "Tracker", Transport: "http", URL: "https://example.com/tracker",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := env["PATH"]; ok {
		t.Fatalf("docker MCP env must not set PATH (login shell would replace it with the host PATH), got %q", env["PATH"])
	}
	want := "/workspace/.loopforge/runtime/bin/loopforge"
	if env[mcpCallBinEnv] != want {
		t.Fatalf("LOOPFORGE_BIN = %q want %q", env[mcpCallBinEnv], want)
	}
	if env[mcpCallConfigEnv] != "/workspace/.loopforge/runtime/mcp-servers.json" {
		t.Fatalf("config = %q", env[mcpCallConfigEnv])
	}
	wrapper, err := os.ReadFile(filepath.Join(workspace, ".loopforge", "runtime", "bin", "loopforge"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(wrapper)
	if !strings.Contains(text, "/workspace/.loopforge/runtime/bin/loopforge-helper") {
		t.Fatalf("docker wrapper should exec the workspace helper, got %s", text)
	}
	if strings.Contains(text, "/data/app/loopforge") {
		t.Fatal("docker wrapper must not exec the host platform binary")
	}
	argv := BindMCPCallArgv(sb, []string{"sh", "-lc", "loopforge mcp list"})
	if argv[1] != "-lc" || strings.Contains(argv[2], "loopforge()") {
		t.Fatalf("docker argv is rewritten by dockerWorkspaceExecArgv, BindMCPCallArgv should leave it, got %v", argv)
	}
}

func TestProcessSandboxMCPCallUsesAbsoluteManagedLauncher(t *testing.T) {
	workspace := t.TempDir()
	sb := &localSandbox{workspace: workspace, driver: "bwrap"}
	env, err := PrepareMCPCall(sb, workspace, []domain.MCPServerDefinition{{
		ID: "replace-batch", Name: "Replace Batch", Transport: "stdio", Command: "python3", Args: []string{"server.py"}, FileRefs: []domain.MCPFileRef{{Path: "server.py"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(env[mcpCallConfigEnv])
	if err != nil {
		t.Fatal(err)
	}
	var cfg mcpCallConfigFile
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(workspace, ".loopforge", "mcp-runtime", "replace-batch", "launch")
	if len(cfg.Servers) != 1 || cfg.Servers[0].Command != want || cfg.Servers[0].CWD != "" || len(cfg.Servers[0].Args) != 0 {
		t.Fatalf("managed stdio config = %#v, want command %q", cfg.Servers, want)
	}
}

func captureMCPCall(t *testing.T, args []string) (string, string, int) {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = outW, errW
	code := mcpCallMain(args)
	_ = outW.Close()
	_ = errW.Close()
	os.Stdout, os.Stderr = oldOut, oldErr
	stdout, err := io.ReadAll(outR)
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := io.ReadAll(errR)
	if err != nil {
		t.Fatal(err)
	}
	_ = outR.Close()
	_ = errR.Close()
	return string(stdout), string(stderr), code
}

func TestHookScriptCanCallMCP(t *testing.T) {
	workspace := t.TempDir()
	configRoot := filepath.Join(workspace, ".loopforge", "config", "reviewer")
	if err := os.MkdirAll(configRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	hook := domain.AgentHook{ID: "audit", Event: "Stop", Language: "shell", Script: "loopforge mcp list > hook-mcp.json\n"}
	compiled, err := materializeAgentHooks(configRoot, ".loopforge/reviewer", "codebuddy", []domain.AgentHook{hook})
	if err != nil || len(compiled) != 1 {
		t.Fatalf("compile hook: %v %#v", err, compiled)
	}
	sb := &localSandbox{workspace: workspace}
	command := CommandSpec{Argv: []string{"sh", "-lc", compiled[0].Command}, CredentialBindings: []CredentialBinding{BindMCP}}
	bound, err := bindCommand(sb, workspace, command, RunCredentials{MCPServers: []domain.MCPServerDefinition{{
		ID: "tracker", Name: "Tracker", MarketID: "linear", Transport: "http", URL: "https://example.com/tracker",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sb.Exec(context.Background(), bound, nil); err != nil {
		t.Fatalf("hook exec: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(workspace, "hook-mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "tracker") {
		t.Fatalf("hook mcp list = %s", raw)
	}
}
