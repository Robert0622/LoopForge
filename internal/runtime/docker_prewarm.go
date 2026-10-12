package runtime

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/Tencent/LoopForge/internal/domain"
)

// DockerPrewarmStatus describes the most recent background sandbox image build.
type DockerPrewarmStatus struct {
	State      string     `json:"state"` // idle | building | ready | failed
	Image      string     `json:"image,omitempty"`
	Error      string     `json:"error,omitempty"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

var (
	dockerPrewarmMu    sync.Mutex
	dockerPrewarmState = DockerPrewarmStatus{State: "idle"}
)

// PrewarmDockerImage builds and caches the image for dockerfile so the first
// run that uses it does not wait for `docker build`.
func PrewarmDockerImage(ctx context.Context, dockerfile domain.DockerfileDefinition) (string, error) {
	dir, err := os.MkdirTemp("", "loopforge-prewarm-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	return ensureDockerImage(ctx, dir, dockerfile)
}

// StartDockerPrewarm runs PrewarmDockerImage in the background unless a build
// is already in progress. It reports whether a new build was started.
func StartDockerPrewarm(dockerfile domain.DockerfileDefinition, timeout time.Duration) bool {
	dockerPrewarmMu.Lock()
	if dockerPrewarmState.State == "building" {
		dockerPrewarmMu.Unlock()
		return false
	}
	now := time.Now().UTC()
	dockerPrewarmState = DockerPrewarmStatus{State: "building", Image: cachedDockerImageName(dockerfile), StartedAt: &now}
	dockerPrewarmMu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		image, err := PrewarmDockerImage(ctx, dockerfile)
		done := time.Now().UTC()
		dockerPrewarmMu.Lock()
		defer dockerPrewarmMu.Unlock()
		dockerPrewarmState.FinishedAt = &done
		if err != nil {
			dockerPrewarmState.State = "failed"
			dockerPrewarmState.Error = truncateForStatus(err.Error(), 600)
			return
		}
		dockerPrewarmState.State = "ready"
		dockerPrewarmState.Image = image
	}()
	return true
}

// DockerPrewarm returns the status of the latest background build.
func DockerPrewarm() DockerPrewarmStatus {
	dockerPrewarmMu.Lock()
	defer dockerPrewarmMu.Unlock()
	return dockerPrewarmState
}

func truncateForStatus(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return "…" + s[len(s)-max:]
}
