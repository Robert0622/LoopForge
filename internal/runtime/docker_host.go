package runtime

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// When LoopForge itself runs in a container and drives the host's Docker
// daemon through a mounted socket, bind mount sources are resolved by the
// daemon on the host, not in this container. Paths are therefore translated
// through this container's own mounts, and sandboxes join its network so they
// can reach the platform API by service name.

type dockerHostMount struct {
	destination string
	source      string
}

type dockerHostView struct {
	mounts  []dockerHostMount // longest destination first
	network string
}

var (
	dockerHostOnce   sync.Once
	dockerHostState  dockerHostView
	dockerHostActive bool
)

func currentDockerHostView() (dockerHostView, bool) {
	dockerHostOnce.Do(func() {
		self := strings.TrimSpace(os.Getenv("LOOPFORGE_DOCKER_SELF"))
		if self == "" {
			if _, err := os.Stat("/.dockerenv"); err != nil {
				return
			}
			self, _ = os.Hostname()
		}
		if self == "" || !dockerAvailable() {
			return
		}
		out, err := exec.Command("docker", "inspect", "-f", "{{json .Mounts}}\n{{json .NetworkSettings.Networks}}", self).Output()
		if err != nil {
			log.Printf("docker sandboxes: cannot inspect own container %q (%v); bind sources are used as-is (set LOOPFORGE_DOCKER_SELF to the container name)", self, err)
			return
		}
		view, err := parseDockerHostView(string(out))
		if err != nil {
			log.Printf("docker sandboxes: %v; bind sources are used as-is", err)
			return
		}
		if network := strings.TrimSpace(os.Getenv("LOOPFORGE_DOCKER_NETWORK")); network != "" {
			view.network = network
		}
		dockerHostState, dockerHostActive = view, true
		log.Printf("docker sandboxes: LoopForge runs in container %s; bind sources map through its %d mounts, sandboxes join network %q", self, len(view.mounts), view.network)
	})
	return dockerHostState, dockerHostActive
}

func parseDockerHostView(out string) (dockerHostView, error) {
	mountsJSON, networksJSON, _ := strings.Cut(strings.TrimSpace(out), "\n")
	var mounts []struct {
		Source      string
		Destination string
	}
	if err := json.Unmarshal([]byte(mountsJSON), &mounts); err != nil {
		return dockerHostView{}, fmt.Errorf("parse own container mounts: %w", err)
	}
	var view dockerHostView
	for _, m := range mounts {
		if m.Source == "" || !filepath.IsAbs(m.Destination) {
			continue
		}
		view.mounts = append(view.mounts, dockerHostMount{destination: filepath.Clean(m.Destination), source: m.Source})
	}
	sort.SliceStable(view.mounts, func(i, j int) bool {
		return len(view.mounts[i].destination) > len(view.mounts[j].destination)
	})
	var networks map[string]json.RawMessage
	if strings.TrimSpace(networksJSON) != "" && json.Unmarshal([]byte(networksJSON), &networks) == nil {
		names := make([]string, 0, len(networks))
		for name := range networks {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if name != "bridge" && name != "none" {
				view.network = name
				break
			}
		}
	}
	return view, nil
}

func (v dockerHostView) hostPath(path string) (string, error) {
	clean := filepath.Clean(path)
	for _, m := range v.mounts {
		if clean == m.destination {
			return m.source, nil
		}
		if strings.HasPrefix(clean, m.destination+string(filepath.Separator)) {
			return m.source + clean[len(m.destination):], nil
		}
	}
	return "", fmt.Errorf("%s is not on a volume shared with the Docker host; keep LOOPFORGE_DATA_DIR and HOME on a mounted volume", path)
}

// dockerHostPath returns the path the Docker daemon sees for a local path.
func dockerHostPath(path string) (string, error) {
	view, active := currentDockerHostView()
	if !active {
		return path, nil
	}
	return view.hostPath(path)
}

// dockerSandboxNetworkArgs attaches sandboxes to LoopForge's own network when
// it runs in a container; otherwise Docker's default network is used.
func dockerSandboxNetworkArgs() []string {
	if network := strings.TrimSpace(os.Getenv("LOOPFORGE_DOCKER_NETWORK")); network != "" {
		return []string{"--network", network}
	}
	if view, active := currentDockerHostView(); active && view.network != "" {
		return []string{"--network", view.network}
	}
	return nil
}
