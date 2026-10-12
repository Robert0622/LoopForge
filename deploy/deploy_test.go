package deploy

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestBindCLIBracesVariablesBeforeNonASCIIText(t *testing.T) {
	script, err := os.ReadFile("bind-cli.sh")
	if err != nil {
		t.Fatal(err)
	}
	unbraced := regexp.MustCompile(`\$[A-Za-z_][A-Za-z0-9_]*[^\x00-\x7F]`)
	if match := unbraced.Find(script); match != nil {
		t.Fatalf("shell variable adjacent to non-ASCII text must be braced for Bash 3.2 locale compatibility: %q", match)
	}
}

func TestBindCLIFallsBackToHTTPOnHTTPSTransportFailure(t *testing.T) {
	home := t.TempDir()
	fakeBin := filepath.Join(home, "fake-bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeCurl := `#!/bin/sh
case "$*" in
  *https://isolated.example/runners/health*) exit 7 ;;
  *http://isolated.example/runners/health*) exit 0 ;;
  *http://isolated.example/runners/loopforge-gate-mcp*)
    previous=""
    for argument in "$@"; do
      if [ "$previous" = "-o" ]; then printf gate-helper > "$argument"; exit 0; fi
      previous="$argument"
    done
    exit 2
    ;;
esac
exit 22
`
	if err := os.WriteFile(filepath.Join(fakeBin, "curl"), []byte(fakeCurl), 0o755); err != nil {
		t.Fatal(err)
	}
	runner := filepath.Join(home, "runner-source")
	if err := os.WriteFile(runner, []byte("runner"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("/bin/bash", "bind-cli.sh",
		"--server", "https://isolated.example",
		"--token", "bind-test",
		"--from", runner,
		"--bin-dir", filepath.Join(home, "bin"),
		"--no-start",
	)
	cmd.Env = append(os.Environ(), "HOME="+home, "PATH="+fakeBin+":/usr/bin:/bin")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bind script failed: %v\n%s", err, out)
	}
	env, err := os.ReadFile(filepath.Join(home, ".loopforge-runner", "env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(env), "LOOPFORGE_SERVER_URL=http://isolated.example\n") {
		t.Fatalf("resolved server missing from env:\n%s", env)
	}
	if !strings.Contains(string(env), "LOOPFORGE_GATE_MCP_BINARY=") {
		t.Fatalf("gate MCP binary pattern missing from env:\n%s", env)
	}
	if !strings.Contains(string(out), "已自动降级到明文 HTTP") {
		t.Fatalf("downgrade warning missing:\n%s", out)
	}
}

func TestBindCLIStrictHTTPSRefusesFallback(t *testing.T) {
	home := t.TempDir()
	fakeBin := filepath.Join(home, "fake-bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fakeBin, "curl"), []byte("#!/bin/sh\nexit 7\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("/bin/bash", "bind-cli.sh", "--server", "isolated.example", "--token", "bind-test", "--strict-https")
	cmd.Env = append(os.Environ(), "HOME="+home, "PATH="+fakeBin+":/usr/bin:/bin")
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "已启用 --strict-https") {
		t.Fatalf("strict HTTPS should reject fallback: err=%v\n%s", err, out)
	}
}

func TestBindCLIUpgradePreservesBindingAndRestartsService(t *testing.T) {
	home := t.TempDir()
	binDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	runnerHome := filepath.Join(home, ".loopforge-runner")
	if err := os.MkdirAll(runnerHome, 0o700); err != nil {
		t.Fatal(err)
	}
	state := []byte(`{"runnerId":"runner-1","sessionToken":"session-1"}`)
	for path, body := range map[string][]byte{
		filepath.Join(runnerHome, "env"):          []byte("LOOPFORGE_SERVER_URL=http://isolated.example\n"),
		filepath.Join(runnerHome, "state.json"):   state,
		filepath.Join(binDir, "loopforge-runner"): []byte("old runner"),
	} {
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var serviceFile string
	if runtime.GOOS == "darwin" {
		serviceFile = filepath.Join(home, "Library/LaunchAgents/com.loopforge.runner.plist")
	} else {
		serviceFile = filepath.Join(home, ".config/systemd/user/loopforge-runner.service")
	}
	if err := os.MkdirAll(filepath.Dir(serviceFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(serviceFile, []byte("existing service"), 0o600); err != nil {
		t.Fatal(err)
	}
	fakeBin := filepath.Join(home, "fake-bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, script := range map[string]string{
		"curl":      "#!/bin/sh\nwhile [ \"$#\" -gt 0 ]; do if [ \"$1\" = -o ]; then printf 'new runner' > \"$2\"; exit 0; fi; shift; done\nexit 1\n",
		"launchctl": "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$TEST_SERVICE_LOG\"\n",
		"systemctl": "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$TEST_SERVICE_LOG\"\n",
	} {
		if err := os.WriteFile(filepath.Join(fakeBin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	serviceLog := filepath.Join(home, "service.log")
	cmd := exec.Command("/bin/bash", "bind-cli.sh", "--upgrade", "--bin-dir", binDir)
	cmd.Env = append(os.Environ(), "HOME="+home, "PATH="+fakeBin+":/usr/bin:/bin", "TEST_SERVICE_LOG="+serviceLog)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("upgrade failed: %v\n%s", err, out)
	}
	gotState, err := os.ReadFile(filepath.Join(runnerHome, "state.json"))
	if err != nil || string(gotState) != string(state) {
		t.Fatalf("binding changed: %q, %v", gotState, err)
	}
	gotBinary, err := os.ReadFile(filepath.Join(binDir, "loopforge-runner"))
	if err != nil || string(gotBinary) != "new runner" {
		t.Fatalf("binary not upgraded: %q, %v", gotBinary, err)
	}
	log, err := os.ReadFile(serviceLog)
	if err != nil || !strings.Contains(string(log), "restart") && !strings.Contains(string(log), "load") {
		t.Fatalf("service not restarted: %q, %v", log, err)
	}
}
