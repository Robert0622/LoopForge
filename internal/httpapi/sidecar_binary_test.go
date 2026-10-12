package httpapi

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSidecarBinaryNextToExecutable(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(executable)
	write := func(name string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("bin"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Remove(path) })
		return path
	}
	if got := sidecarBinary("af-sidecar-test", "linux", "arm64"); got != "" {
		t.Fatalf("no sidecar yet, got %q", got)
	}
	native := write("af-sidecar-test")
	if got := sidecarBinary("af-sidecar-test", runtime.GOOS, runtime.GOARCH); got != native {
		t.Fatalf("native target = %q, want %q", got, native)
	}
	other := "arm64"
	if runtime.GOARCH == "arm64" {
		other = "amd64"
	}
	if got := sidecarBinary("af-sidecar-test", runtime.GOOS, other); got != "" {
		t.Fatalf("the bare binary must not be served for another arch, got %q", got)
	}
	cross := write("af-sidecar-test-" + runtime.GOOS + "-" + other)
	if got := sidecarBinary("af-sidecar-test", runtime.GOOS, other); got != cross {
		t.Fatalf("cross target = %q, want %q", got, cross)
	}
}
