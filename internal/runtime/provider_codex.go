package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/Tencent/LoopForge/internal/domain"
)

type codexProvider struct{ baseProvider }

func (codexProvider) Kind() string { return "codex" }

func (codexProvider) Materialize(ctx *providerMaterializeContext) error {
	ctx.Compiled.Args = append(ctx.Compiled.Args, codexConfigArgs(string(ctx.Content), ctx.SkillPaths, ctx.MCPServers)...)
	ctx.Compiled.Args = append(ctx.Compiled.Args, codexHookConfigArgs(ctx.Compiled.Hooks)...)
	return nil
}

func (codexProvider) Prepare(in providerInvocation) (CommandSpec, error) {
	argv := buildProviderArgs(in)
	argv = dropCodexResumeArgs(argv)
	// exec options must precede the `resume` subcommand, so the effort config
	// override is appended here rather than after the resume session ID.
	if effort := strings.TrimSpace(in.Executor.Effort); effort != "" && !codexHasReasoningEffort(argv) {
		argv = append(argv, "-c", "model_reasoning_effort="+strconv.Quote(effort))
	}
	if len(in.Compiled.Hooks) > 0 && !hasArg(argv, "--dangerously-bypass-hook-trust") {
		argv = append(argv, "--dangerously-bypass-hook-trust")
	}
	if in.Driver == "local" {
		for _, flag := range []string{"--ignore-user-config", "--ignore-rules"} {
			if !hasArg(argv, flag) {
				argv = append(argv, flag)
			}
		}
	} else {
		// User config stays enabled for custom model providers, but its MCP
		// servers must not join the node: a personal `loopforge` entry would
		// point the agent at another platform instance.
		argv = append(argv, codexDisableUserMCPArgs(argv, codexUserConfigPath())...)
	}
	// ChatGPT account connectors (codex_apps) bypass the MCP servers the
	// Agent definition declares.
	if !codexConfiguresFeature(argv, "apps") {
		argv = append(argv, "-c", "features.apps=false")
	}
	root := in.Workspace
	if in.Driver == "docker" {
		root = "/workspace"
	}
	argv = codexAbsoluteSkillPaths(argv, root)
	if in.Driver == "docker" {
		// Codex's own bwrap sandbox cannot create namespaces in a container
		// started with --cap-drop=ALL; the container is the sandbox boundary.
		argv = codexExternallySandboxed(argv)
	}
	if sessionID := strings.TrimSpace(in.Options.ResumeSessionID); sessionID != "" {
		// `resume` is a subcommand of `codex exec`; all exec options must precede it.
		argv = append(argv, "resume", sessionID)
	}
	return CommandSpec{Argv: append(argv, positionalPrompt(in)), Provider: "codex", CredentialBindings: []CredentialBinding{BindCodex, BindGit, BindMCP}, PrivateHome: true}, nil
}

const codexBypassSandboxFlag = "--dangerously-bypass-approvals-and-sandbox"

func codexExternallySandboxed(argv []string) []string {
	out := make([]string, 0, len(argv)+1)
	for i := 0; i < len(argv); i++ {
		switch arg := argv[i]; {
		case arg == "--sandbox" || arg == "-s" || arg == "--ask-for-approval" || arg == "-a":
			i++
		case strings.HasPrefix(arg, "--sandbox=") || strings.HasPrefix(arg, "--ask-for-approval=") || arg == "--full-auto":
		default:
			out = append(out, arg)
		}
	}
	if !hasArg(out, codexBypassSandboxFlag) {
		out = append(out, codexBypassSandboxFlag)
	}
	return out
}

func codexUserConfigPath() string {
	if home := strings.TrimSpace(os.Getenv("CODEX_HOME")); home != "" {
		return filepath.Join(home, "config.toml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codex", "config.toml")
}

var codexMCPNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// codexDisableUserMCPArgs disables the MCP servers declared in the user's
// Codex config, except names LoopForge itself configures in argv.
func codexDisableUserMCPArgs(argv []string, configPath string) []string {
	if configPath == "" {
		return nil
	}
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}
	var cfg struct {
		MCPServers map[string]any `toml:"mcp_servers"`
	}
	if toml.Unmarshal(raw, &cfg) != nil || len(cfg.MCPServers) == 0 {
		return nil
	}
	names := make([]string, 0, len(cfg.MCPServers))
	for name := range cfg.MCPServers {
		if codexMCPNamePattern.MatchString(name) && !codexConfiguresMCP(argv, name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	args := make([]string, 0, len(names)*2)
	for _, name := range names {
		args = append(args, "-c", "mcp_servers."+name+".enabled=false")
	}
	return args
}

var codexRelativeSkillPath = regexp.MustCompile(`path = "([^"/][^"]*)"`)

// codexAbsoluteSkillPaths anchors skills.config paths at root. Codex 0.159
// fails with "AbsolutePathBuf deserialized without a base path" when another
// -c override touches a user-config table and skill paths are relative.
func codexAbsoluteSkillPaths(argv []string, root string) []string {
	if strings.TrimSpace(root) == "" {
		return argv
	}
	out := append([]string(nil), argv...)
	for i := 0; i+1 < len(out); i++ {
		if out[i] != "-c" || !strings.HasPrefix(out[i+1], "skills.config=") {
			continue
		}
		out[i+1] = codexRelativeSkillPath.ReplaceAllStringFunc(out[i+1], func(match string) string {
			rel, err := strconv.Unquote(strings.TrimPrefix(match, "path = "))
			if err != nil || filepath.IsAbs(rel) {
				return match
			}
			return "path = " + strconv.Quote(filepath.ToSlash(filepath.Join(root, rel)))
		})
	}
	return out
}

func codexConfiguresFeature(argv []string, name string) bool {
	for i := 0; i+1 < len(argv); i++ {
		if (argv[i] == "-c" && strings.HasPrefix(argv[i+1], "features."+name+"=")) || ((argv[i] == "--enable" || argv[i] == "--disable") && argv[i+1] == name) {
			return true
		}
	}
	return false
}

func codexConfiguresMCP(argv []string, name string) bool {
	prefix := "mcp_servers." + name + "."
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == "-c" && strings.HasPrefix(argv[i+1], prefix) {
			return true
		}
	}
	return false
}

func dropCodexResumeArgs(argv []string) []string {
	for i := 0; i < len(argv); i++ {
		if argv[i] != "resume" {
			continue
		}
		end := i + 1
		if end < len(argv) {
			end++
		}
		return append(append([]string{}, argv[:i]...), argv[end:]...)
	}
	return argv
}

func codexHasReasoningEffort(argv []string) bool {
	for _, arg := range argv {
		if strings.Contains(arg, "model_reasoning_effort") {
			return true
		}
	}
	return false
}

// withCodexIsolationEnv copies process-sandbox isolation variables into Codex
// tool subprocesses. inherit=core only keeps PATH/HOME/USER/SHELL/TERM/LANG,
// so ZDOTDIR never reaches `zsh -lc` unless it is explicitly set here.
func withCodexIsolationEnv(argv []string, overlay map[string]string) []string {
	if len(argv) == 0 || len(overlay) == 0 {
		return argv
	}
	keys := make([]string, 0, len(overlay))
	for key, value := range overlay {
		if !codexIsolationEnvKey(key) || strings.TrimSpace(value) == "" {
			continue
		}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return argv
	}
	sort.Strings(keys)
	extra := make([]string, 0, len(keys)*2)
	for _, key := range keys {
		extra = append(extra, "-c", "shell_environment_policy.set."+key+"="+strconv.Quote(overlay[key]))
	}
	out := make([]string, 0, len(argv)+len(extra))
	out = append(out, argv[:len(argv)-1]...)
	out = append(out, extra...)
	return append(out, argv[len(argv)-1])
}

func codexIsolationEnvKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	upper := strings.ToUpper(key)
	for _, needle := range []string{"KEY", "TOKEN", "SECRET", "PASSWORD", "AUTHORIZATION"} {
		if strings.Contains(upper, needle) {
			return false
		}
	}
	switch {
	case strings.HasPrefix(upper, "CODEBUDDY_"), strings.HasPrefix(upper, "GIT_"), strings.HasPrefix(upper, "SSH_"):
		return false
	default:
		return true
	}
}

func (codexProvider) Protocol(ex domain.ExecutorDefinition, sink EventSink) *protocolAdapter {
	return makeProtocolAdapter(ex, sink, protocolCodex, "codex-jsonl-v1")
}

func (codexProvider) SandboxRequirements() SandboxRequirements {
	return SandboxRequirements{PrivateHome: true}
}

func codexConfigArgs(instructions string, skillPaths []string, servers []domain.MCPServerDefinition) []string {
	args := []string{
		"-c", "developer_instructions=" + strconv.Quote(instructions),
		// core keeps PATH/HOME/USER/SHELL/TERM/LANG and drops everything else,
		// including ZDOTDIR. Isolation vars are re-injected at Exec via set.*.
		"-c", `shell_environment_policy.inherit="core"`,
	}
	if len(skillPaths) > 0 {
		entries := make([]string, 0, len(skillPaths))
		for _, skillPath := range skillPaths {
			entries = append(entries, fmt.Sprintf("{ path = %s, enabled = true }", strconv.Quote(skillPath)))
		}
		args = append(args, "-c", "skills.config=["+strings.Join(entries, ", ")+"]")
	}
	for _, m := range servers {
		prefix := "mcp_servers." + safeID(m.ID) + "."
		if m.Transport == "http" {
			args = append(args, "-c", prefix+"url="+strconv.Quote(m.URL))
		} else {
			args = append(args, "-c", prefix+"command="+strconv.Quote(m.Command))
			if cwd := MCPRuntimeWorkingDirectory(m); cwd != "" {
				args = append(args, "-c", prefix+"cwd="+strconv.Quote(cwd))
			}
			if len(m.Args) > 0 {
				parts := make([]string, len(m.Args))
				for i, value := range m.Args {
					parts[i] = strconv.Quote(value)
				}
				args = append(args, "-c", prefix+"args=["+strings.Join(parts, ",")+"]")
			}
			env := resolveMap(m.Environment)
			if len(env) > 0 {
				keys := make([]string, 0, len(env))
				for key := range env {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				pairs := make([]string, 0, len(keys))
				for _, key := range keys {
					pairs = append(pairs, fmt.Sprintf("%s = %s", key, strconv.Quote(env[key])))
				}
				args = append(args, "-c", prefix+"env={ "+strings.Join(pairs, ", ")+" }")
			}
		}
		if m.TimeoutSec > 0 {
			args = append(args, "-c", fmt.Sprintf("%stool_timeout_sec=%d.0", prefix, m.TimeoutSec))
		}
	}
	return args
}
