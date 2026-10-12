package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Tencent/LoopForge/internal/domain"
	"github.com/Tencent/LoopForge/internal/repo"
)

func restoreMarket(t *testing.T) {
	t.Helper()
	items, defaults := builtinMCPMarket, defaultProjectMarketMCPAliases
	t.Cleanup(func() { builtinMCPMarket, defaultProjectMarketMCPAliases = items, defaults })
}

func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConfigureMCPMarketMergesCustomCatalog(t *testing.T) {
	restoreMarket(t)
	dir := t.TempDir()
	writeFile(t, dir, "team.json", `{"items":[
		{"id":"github","name":"GitHub Enterprise","url":"https://ghe.example.com/mcp/","authType":"personal-token"},
		{"id":"wiki","name":"Team Wiki","description":"internal docs","url":"https://wiki.example.com/mcp","aliases":["docs"]}
	]}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"id":"tickets","name":"Tickets","url":"https://tickets.example.com/mcp"}]`))
	}))
	defer srv.Close()
	before := len(builtinMCPMarket)
	n, err := ConfigureMCPMarket(context.Background(), []string{dir, srv.URL}, []string{"github", "docs"})
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 || len(builtinMCPMarket) != before+2 {
		t.Fatalf("custom=%d market=%d (built-in %d)", n, len(builtinMCPMarket), before)
	}
	gh, _ := marketItem("github")
	if gh.URL != "https://ghe.example.com/mcp/" || gh.Name != "GitHub Enterprise" {
		t.Fatalf("github entry not overridden: %+v", gh)
	}
	if wiki, ok := marketItem("docs"); !ok || wiki.ID != "wiki" || wiki.Transport != "http" || wiki.Category != "custom" {
		t.Fatalf("alias lookup / defaults: %+v ok=%v", wiki, ok)
	}

	st, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.PutProject(domain.Project{ID: "p1", Name: "P1"}); err != nil {
		t.Fatal(err)
	}
	if err := ensureDefaultProjectMCPs(st, "p1"); err != nil {
		t.Fatal(err)
	}
	installed := map[string]string{}
	for _, s := range st.MCPServers() {
		if s.ProjectID == "p1" {
			installed[s.MarketID] = s.URL
		}
	}
	if installed["github"] != "https://ghe.example.com/mcp/" || installed["wiki"] != "https://wiki.example.com/mcp" || len(installed) != 2 {
		t.Fatalf("default project MCPs = %v", installed)
	}
}

func TestConfigureMCPMarketRejectsBadInput(t *testing.T) {
	restoreMarket(t)
	dir := t.TempDir()
	for name, body := range map[string]string{
		"no-url":  `[{"id":"x","name":"X"}]`,
		"stdio":   `[{"id":"x","name":"X","transport":"stdio","url":"https://x"}]`,
		"auth":    `[{"id":"x","name":"X","url":"https://x","authType":"oauth"}]`,
		"no-name": `[{"id":"x","url":"https://x"}]`,
		"broken":  `[{`,
	} {
		path := writeFile(t, dir, name+".json", body)
		if _, err := ConfigureMCPMarket(context.Background(), []string{path}, nil); err == nil {
			t.Fatalf("%s: expected error", name)
		}
	}
	before := len(builtinMCPMarket)
	if _, err := ConfigureMCPMarket(context.Background(), nil, []string{"does-not-exist"}); err == nil || !strings.Contains(err.Error(), "does-not-exist") {
		t.Fatalf("unknown default error = %v", err)
	}
	if len(builtinMCPMarket) != before || len(defaultProjectMarketMCPAliases) != 1 {
		t.Fatal("failed configuration must leave the market unchanged")
	}
}

func TestWebPluginsAreServedAndInjected(t *testing.T) {
	t.Cleanup(func() { webPlugins = nil })
	dir := t.TempDir()
	writeFile(t, dir, "b dash.js", "window.x=1")
	writeFile(t, dir, "a.css", "body{}")
	writeFile(t, dir, "notes.md", "ignored")
	n, err := ConfigureWebPlugins(context.Background(), []string{dir})
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	static := fstest.MapFS{
		"index.html":  {Data: []byte("<html><head></head><body><script src=\"/app.js\"></script></body></html>")},
		"vscode.html": {Data: []byte("<html><head></head><body></body></html>")},
		"app.js":      {Data: []byte("1")},
	}
	h := studioFileServer(static)
	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec
	}
	index := get("/").Body.String()
	css := strings.Index(index, `<link rel="stylesheet" href="/plugins/01-a.css?v=`)
	js := strings.Index(index, `<script src="/plugins/02-b-dash.js?v=`)
	app := strings.Index(index, `/app.js`)
	if css < 0 || css > strings.Index(index, "</head>") || js < app || js > strings.Index(index, "</body>") {
		t.Fatalf("plugin tags misplaced:\n%s", index)
	}
	if strings.Contains(get("/vscode.html").Body.String(), "/plugins/") {
		t.Fatal("plugins must only load in the main UI")
	}
	rec := get("/plugins/02-b-dash.js")
	if rec.Code != http.StatusOK || rec.Body.String() != "window.x=1" || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/javascript") {
		t.Fatalf("plugin response %d %q %q", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
	if _, err := ConfigureWebPlugins(context.Background(), []string{filepath.Join(dir, "notes.md")}); err == nil {
		t.Fatal("an explicitly listed non-js/css file should fail")
	}
	if get("/plugins/nope.js").Code != http.StatusNotFound {
		t.Fatal("unknown plugin should 404")
	}
}
