package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/Tencent/LoopForge/internal/domain"
)

const gateMCPServerID = "loopforge-gate"

var gateMCPBuildMu sync.Mutex

func materializeGateMCP(configRoot, binaryRel, resultPath, driver string) (domain.MCPServerDefinition, error) {
	resultPath = filepath.ToSlash(filepath.Clean(strings.TrimSpace(resultPath)))
	if resultPath == "." || filepath.IsAbs(resultPath) || resultPath == ".." || strings.HasPrefix(resultPath, "../") {
		return domain.MCPServerDefinition{}, fmt.Errorf("invalid gate result path %q", resultPath)
	}
	targetOS := goruntime.GOOS
	if strings.EqualFold(strings.TrimSpace(driver), "docker") {
		targetOS = "linux"
	}
	source, err := resolveGateMCPBinary(targetOS, goruntime.GOARCH)
	if err != nil {
		return domain.MCPServerDefinition{}, err
	}
	raw, err := os.ReadFile(source)
	if err != nil {
		return domain.MCPServerDefinition{}, fmt.Errorf("read gate MCP binary: %w", err)
	}
	binaryPath := filepath.Join(configRoot, "gate-mcp")
	if err := writeManagedFile(binaryPath, raw, 0o700); err != nil {
		return domain.MCPServerDefinition{}, err
	}
	return domain.MCPServerDefinition{
		ID:        gateMCPServerID,
		Name:      "LoopForge Gate",
		Transport: "stdio",
		Command:   filepath.ToSlash(binaryRel),
		Environment: map[string]string{
			"LOOPFORGE_GATE_RESULT_PATH": resultPath,
		},
	}, nil
}

// resolveGateMCPBinary follows the runner distribution convention: production
// bundles may provide a {os}/{arch} path, while source checkouts build and
// cache the small helper on demand. A sidecar next to the current executable
// is also accepted so installed runner machines need no Go toolchain.
func resolveGateMCPBinary(targetOS, targetArch string) (string, error) {
	if targetOS != "darwin" && targetOS != "linux" {
		return "", fmt.Errorf("unsupported gate MCP operating system %q", targetOS)
	}
	if targetArch != "amd64" && targetArch != "arm64" {
		return "", fmt.Errorf("unsupported gate MCP architecture %q", targetArch)
	}
	if pattern := strings.TrimSpace(os.Getenv("LOOPFORGE_GATE_MCP_BINARY")); pattern != "" {
		path := expandPlatformPath(pattern, targetOS, targetArch)
		if validExecutableFile(path) {
			return path, nil
		}
		return "", fmt.Errorf("gate MCP binary %s/%s is not deployed at %s", targetOS, targetArch, path)
	}
	if executable, err := os.Executable(); err == nil {
		dir := filepath.Dir(executable)
		for _, name := range []string{
			fmt.Sprintf("loopforge-gate-mcp-%s-%s", targetOS, targetArch),
			"loopforge-gate-mcp",
		} {
			path := filepath.Join(dir, name)
			if validExecutableFile(path) && (name != "loopforge-gate-mcp" || targetOS == goruntime.GOOS && targetArch == goruntime.GOARCH) {
				return path, nil
			}
		}
	}
	root := gateMCPSourceRoot()
	if root == "" {
		return "", gateMCPMissingError(targetOS, targetArch)
	}
	gateMCPBuildMu.Lock()
	defer gateMCPBuildMu.Unlock()
	cache := filepath.Join(os.TempDir(), fmt.Sprintf("loopforge-gate-mcp-%s-%s", targetOS, targetArch))
	if !buildArtifactFresh(cache, root) {
		build := exec.Command("go", "build", "-o", cache, "./cmd/loopforge-gate-mcp")
		build.Dir = root
		build.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+targetOS, "GOARCH="+targetArch)
		out, err := build.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("build gate MCP binary %s/%s: %w: %s", targetOS, targetArch, err, strings.TrimSpace(string(out)))
		}
	}
	return cache, nil
}

func gateMCPMissingError(targetOS, targetArch string) error {
	return fmt.Errorf("gate MCP binary loopforge-gate-mcp-%s-%s not found: run the Docker image, build with `make build` and start bin/loopforge, or set LOOPFORGE_GATE_MCP_BINARY (prebuilt files) or LOOPFORGE_SOURCE_ROOT (source checkout with Go)", targetOS, targetArch)
}

// CheckGateMCPBinary reports, without building anything, whether runs that
// need the gate MCP for targetOS/targetArch will be able to provision it.
func CheckGateMCPBinary(targetOS, targetArch string) error {
	if pattern := strings.TrimSpace(os.Getenv("LOOPFORGE_GATE_MCP_BINARY")); pattern != "" {
		if path := expandPlatformPath(pattern, targetOS, targetArch); !validExecutableFile(path) {
			return fmt.Errorf("gate MCP binary %s/%s is not deployed at %s", targetOS, targetArch, path)
		}
		return nil
	}
	if executable, err := os.Executable(); err == nil {
		dir := filepath.Dir(executable)
		if validExecutableFile(filepath.Join(dir, fmt.Sprintf("loopforge-gate-mcp-%s-%s", targetOS, targetArch))) {
			return nil
		}
		if targetOS == goruntime.GOOS && targetArch == goruntime.GOARCH && validExecutableFile(filepath.Join(dir, "loopforge-gate-mcp")) {
			return nil
		}
	}
	if gateMCPSourceRoot() == "" {
		return gateMCPMissingError(targetOS, targetArch)
	}
	if _, err := exec.LookPath("go"); err != nil {
		return fmt.Errorf("gate MCP binary %s/%s must be built from source but Go is not installed: %w", targetOS, targetArch, err)
	}
	return nil
}

func expandPlatformPath(pattern, targetOS, targetArch string) string {
	return strings.ReplaceAll(strings.ReplaceAll(pattern, "{os}", targetOS), "{arch}", targetArch)
}

func validExecutableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() > 0
}

func gateMCPSourceRoot() string {
	if root := strings.TrimSpace(os.Getenv("LOOPFORGE_SOURCE_ROOT")); root != "" {
		if validSourceRoot(root) {
			return root
		}
		return ""
	}
	// A binary built into <checkout>/bin is often started from another
	// directory, so look upward from the executable as well as the cwd.
	var starts []string
	if cwd, err := os.Getwd(); err == nil {
		starts = append(starts, cwd)
	}
	if executable, err := os.Executable(); err == nil {
		starts = append(starts, filepath.Dir(executable))
	}
	for _, start := range starts {
		for current := start; ; current = filepath.Dir(current) {
			if validSourceRoot(current) {
				return current
			}
			parent := filepath.Dir(current)
			if parent == current {
				break
			}
		}
	}
	return ""
}

func validSourceRoot(root string) bool {
	return validExecutableFile(filepath.Join(root, "go.mod")) && dirExistsRuntime(filepath.Join(root, "cmd", "loopforge-gate-mcp"))
}

func dirExistsRuntime(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func buildArtifactFresh(cache, root string) bool {
	info, err := os.Stat(cache)
	if err != nil || info.IsDir() || info.Size() == 0 {
		return false
	}
	binaryTime := info.ModTime()
	fresh := true
	_ = filepath.Walk(root, func(path string, entry os.FileInfo, walkErr error) error {
		if walkErr != nil || !fresh {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			switch entry.Name() {
			case "vendor", ".git", "testdata", "web", "docs":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "go.mod") || strings.HasSuffix(path, "go.sum") {
			if entry.ModTime().After(binaryTime) {
				fresh = false
				return filepath.SkipDir
			}
		}
		return nil
	})
	return fresh
}

func writeMockGateResult(workspace, resultPath string, agent domain.AgentDefinition) error {
	target := filepath.Join(workspace, filepath.FromSlash(resultPath))
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	result := mockGateResult(agent, resultPath)
	raw, _ := json.Marshal(result)
	return os.WriteFile(target, raw, 0o600)
}

func mockGateResult(agent domain.AgentDefinition, resultPath string) domain.GateResult {
	decision := mockGateDecision(agent, resultPath)
	result := domain.GateResult{
		Decision:       decision,
		Reason:         mockDirective(agent.Instructions, "reason"),
		ActionRequired: mockDirective(agent.Instructions, "action"),
		Summary:        mockDirective(agent.Instructions, "summary"),
	}
	if result.Reason == "" {
		result.Reason = "mock executor called submit_gate_result with " + decision
	}
	if decision == "human" && result.ActionRequired == "" {
		result.ActionRequired = "请确认当前证据是否足以允许流程继续"
	}
	if decision == "rework" && result.ActionRequired == "" {
		result.ActionRequired = "请上游节点根据评审发现完成修复后重新提交"
	}
	for _, value := range mockDirectives(agent.Instructions, "finding") {
		parts := strings.SplitN(value, "|", 3)
		finding := domain.GateFinding{Title: strings.TrimSpace(parts[0])}
		if len(parts) > 1 {
			finding.Severity, finding.Title = finding.Title, strings.TrimSpace(parts[1])
		}
		if len(parts) > 2 {
			finding.Evidence = strings.TrimSpace(parts[2])
		}
		if finding.Title != "" {
			result.Findings = append(result.Findings, finding)
		}
	}
	return result
}

func mockDirective(instructions, name string) string {
	values := mockDirectives(instructions, name)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func mockDirectives(instructions, name string) []string {
	prefix := "[[mock:" + name + "="
	values := []string{}
	for {
		start := strings.Index(instructions, prefix)
		if start < 0 {
			return values
		}
		start += len(prefix)
		end := strings.Index(instructions[start:], "]]")
		if end < 0 {
			return values
		}
		values = append(values, strings.TrimSpace(instructions[start:start+end]))
		instructions = instructions[start+end+2:]
	}
}

func mockGateDecision(agent domain.AgentDefinition, resultPath string) string {
	const prefix = "[[mock:gate="
	instructions := agent.Instructions
	start := strings.Index(instructions, prefix)
	if start < 0 {
		return "pass"
	}
	start += len(prefix)
	end := strings.Index(instructions[start:], "]]")
	if end < 0 {
		return "pass"
	}
	sequence := strings.Split(instructions[start:start+end], ",")
	attempt := 1
	name := filepath.Base(filepath.FromSlash(resultPath))
	if value := strings.TrimSuffix(strings.TrimPrefix(name, "attempt-"), filepath.Ext(name)); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			attempt = parsed
		}
	}
	index := min(attempt-1, len(sequence)-1)
	if index < 0 {
		return "pass"
	}
	switch decision := strings.ToLower(strings.TrimSpace(sequence[index])); decision {
	case "pass", "rework", "fail", "human":
		return decision
	default:
		return "pass"
	}
}
