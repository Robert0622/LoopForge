package runtime

import (
	"os/exec"
	goruntime "runtime"
)

// DetectTemplateSandbox picks the strongest sandbox profile that can actually
// start on this host, so built-in SOP templates run out of the box: Docker
// when a daemon is reachable (including the host's, through a mounted socket),
// then bubblewrap or Seatbelt, then local (for example inside the LoopForge
// container without a socket, which is itself the boundary).
func DetectTemplateSandbox() string {
	if docker, err := exec.LookPath("docker"); err == nil && exec.Command(docker, "info", "--format", "{{.ServerVersion}}").Run() == nil {
		return "generic-docker"
	}
	switch goruntime.GOOS {
	case "linux":
		if binary, err := exec.LookPath("bwrap"); err == nil && bwrapUsable(binary) == nil {
			return "linux-bwrap"
		}
	case "darwin":
		if _, err := exec.LookPath("sandbox-exec"); err == nil {
			return "macos-seatbelt"
		}
	}
	return "local"
}
