package web

import (
	"os/exec"
	"testing"
)

func TestOnboardingAndQuickstart(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	for _, script := range []string{"onboarding_test.js", "quickstart_test.js"} {
		if out, err := exec.Command(node, script).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", script, err, out)
		}
	}
}
