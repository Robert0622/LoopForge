package runtime

import "testing"

func TestParseDockerHostView(t *testing.T) {
	out := `[{"Type":"volume","Name":"loopforge_loopforge-data","Source":"/var/lib/docker/volumes/loopforge_loopforge-data/_data","Destination":"/data"},` +
		`{"Type":"bind","Source":"/var/run/docker.sock","Destination":"/var/run/docker.sock"},` +
		`{"Type":"bind","Source":"/srv/cache","Destination":"/data/cache"}]` + "\n" +
		`{"bridge":{},"loopforge_default":{"IPAddress":"172.18.0.2"}}`
	view, err := parseDockerHostView(out)
	if err != nil {
		t.Fatal(err)
	}
	if view.network != "loopforge_default" {
		t.Fatalf("network = %q", view.network)
	}
	cases := map[string]string{
		"/data":                       "/var/lib/docker/volumes/loopforge_loopforge-data/_data",
		"/data/workspaces/run-1":      "/var/lib/docker/volumes/loopforge_loopforge-data/_data/workspaces/run-1",
		"/data/home/.codebuddy/":      "/var/lib/docker/volumes/loopforge_loopforge-data/_data/home/.codebuddy",
		"/data/cache/go":              "/srv/cache/go",
		"/var/run/docker.sock":        "/var/run/docker.sock",
		"/data/../data/workspaces/r2": "/var/lib/docker/volumes/loopforge_loopforge-data/_data/workspaces/r2",
	}
	for in, want := range cases {
		got, err := view.hostPath(in)
		if err != nil || got != want {
			t.Errorf("hostPath(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"/root/.claude", "/database", "/"} {
		if got, err := view.hostPath(in); err == nil {
			t.Errorf("hostPath(%q) = %q, want error for a path outside shared mounts", in, got)
		}
	}
}

func TestParseDockerHostViewDefaultNetworkOnly(t *testing.T) {
	view, err := parseDockerHostView(`[]` + "\n" + `{"bridge":{}}`)
	if err != nil {
		t.Fatal(err)
	}
	if view.network != "" {
		t.Fatalf("network = %q, want none so Docker's default bridge is used", view.network)
	}
	if _, err := parseDockerHostView("not json"); err == nil {
		t.Fatal("want error for unparseable inspect output")
	}
}
