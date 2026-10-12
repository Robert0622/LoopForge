package httpapi

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tencent/LoopForge/internal/domain"
	"github.com/Tencent/LoopForge/internal/engine"
	"github.com/Tencent/LoopForge/internal/repo"
	"github.com/Tencent/LoopForge/internal/runtime"
	"github.com/Tencent/LoopForge/internal/tracker"
	webassets "github.com/Tencent/LoopForge/web"
	"github.com/gin-gonic/gin"
)

func assertProjectHasDefaultTrackerMCPs(t *testing.T, st *repo.Store, projectID string) {
	t.Helper()
	for _, server := range st.MCPServers() {
		if server.Scope == "project" && server.ProjectID == projectID && server.MarketID == "github" {
			return
		}
	}
	t.Fatalf("project %s missing default GitHub MCP", projectID)
}

func TestMCPInstallIDSeparatesScopeAndOwner(t *testing.T) {
	project := mcpInstallID("github", "project", "platform")
	personal := mcpInstallID("github", "personal", "2")
	if project == personal || project != "mcp-github-project-platform" || personal != "mcp-github-personal-2" {
		t.Fatalf("unexpected install ids: project=%q personal=%q", project, personal)
	}
}

func TestMCPMarketCatalogLoadsBuiltin(t *testing.T) {
	if len(builtinMCPMarket) == 0 {
		t.Fatal("expected builtin catalog")
	}
	item, ok := marketItem("forge")
	if !ok || item.ID != "github" || item.Name != "GitHub" {
		t.Fatalf("alias forge = %+v ok=%v", item, ok)
	}
	if item.AuthType != "personal-token" || item.Transport != "http" {
		t.Fatalf("github market item must use personal-token over http: %+v", item)
	}
	seen := map[string]bool{}
	for _, item := range builtinMCPMarket {
		if seen[item.ID] {
			t.Fatalf("duplicate market id %s", item.ID)
		}
		seen[item.ID] = true
		if !strings.HasPrefix(item.URL, "https://") {
			t.Fatalf("%s url must be https: %q", item.ID, item.URL)
		}
	}
}

func TestMCPMarketSearch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &marketHandlers{}
	req := httptest.NewRequest(http.MethodGet, "/api/mcp-market?q=pull", nil)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	h.mcpMarket(c)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Source string          `json:"source"`
		Total  int             `json:"total"`
		Items  []mcpMarketItem `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Source != "loopforge-builtin" || payload.Total != len(builtinMCPMarket) {
		t.Fatalf("catalog meta = %+v", payload)
	}
	if len(payload.Items) == 0 {
		t.Fatal("expected search hits for pull")
	}
	found := false
	for _, item := range payload.Items {
		if item.ID == "github" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("search pull missed github: %+v", payload.Items)
	}

	all := httptest.NewRecorder()
	allCtx, _ := gin.CreateTestContext(all)
	allCtx.Request = httptest.NewRequest(http.MethodGet, "/api/mcp-market", nil)
	h.mcpMarket(allCtx)
	var allPayload struct {
		Items []mcpMarketItem `json:"items"`
	}
	if err := json.Unmarshal(all.Body.Bytes(), &allPayload); err != nil {
		t.Fatal(err)
	}
	if len(allPayload.Items) != len(builtinMCPMarket) {
		t.Fatalf("unfiltered items=%d catalog=%d", len(allPayload.Items), len(builtinMCPMarket))
	}
}

func TestMCPMarketInstallAliasStoresCanonicalID(t *testing.T) {
	st, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.PutProject(domain.Project{ID: "platform", Name: "Platform"}); err != nil {
		t.Fatal(err)
	}
	h := &marketHandlers{deps: &deps{repo: st}}
	newInstallRecorder := func(body string) (*gin.Context, *httptest.ResponseRecorder) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/mcp-market/install", bytes.NewBufferString(body))
		return c, rec
	}
	c, rec := newInstallRecorder(`{"marketId":"forge","scope":"project","projectId":"platform"}`)
	h.mcpMarketInstall(c)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var saved domain.MCPServerDefinition
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.MarketID != "github" || saved.Name != "GitHub" {
		t.Fatalf("saved = %+v", saved)
	}
	conflictCtx, conflictRec := newInstallRecorder(`{"marketId":"github","scope":"project","projectId":"platform"}`)
	h.mcpMarketInstall(conflictCtx)
	if conflictRec.Code != http.StatusConflict {
		t.Fatalf("expected conflict for alias/canonical, status=%d body=%s", conflictRec.Code, conflictRec.Body.String())
	}
}

func TestEnsureDefaultProjectMCPsIsIdempotent(t *testing.T) {
	st, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.PutProject(domain.Project{ID: "onboard", Name: "Onboard"}); err != nil {
		t.Fatal(err)
	}
	if err := ensureDefaultProjectMCPs(st, "onboard"); err != nil {
		t.Fatal(err)
	}
	assertProjectHasDefaultTrackerMCPs(t, st, "onboard")
	before := len(st.MCPServers())
	if err := ensureDefaultProjectMCPs(st, "onboard"); err != nil {
		t.Fatal(err)
	}
	if got := len(st.MCPServers()); got != before {
		t.Fatalf("second ensure created duplicates: %d -> %d", before, got)
	}
}

func TestNewBackfillsDefaultMCPsOnExistingProjects(t *testing.T) {
	st, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.PutProject(domain.Project{ID: "legacy", Name: "Legacy"}); err != nil {
		t.Fatal(err)
	}
	New(st, engine.New(st, runtime.NewManager(t.TempDir())), tracker.NewService(), "", "")
	assertProjectHasDefaultTrackerMCPs(t, st, "legacy")
	assertProjectHasDefaultTrackerMCPs(t, st, "platform")
}

func TestMarketMCPRejectsPersonalScope(t *testing.T) {
	st, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{deps: &deps{repo: st}}
	s.market = &marketHandlers{deps: s.deps}
	g := gin.New()
	g.POST("/api/mcp-market/install", s.market.mcpMarketInstall)
	req := httptest.NewRequest(http.MethodPost, "/api/mcp-market/install", bytes.NewBufferString(`{"marketId":"github","scope":"personal"}`))
	rec := httptest.NewRecorder()
	g.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestVisibleMCPServersForUserHidesOtherPersonalItems(t *testing.T) {
	items := []domain.MCPServerDefinition{
		{ID: "project", Scope: "project", ProjectID: "p1"},
		{ID: "mine", Scope: "personal", OwnerUserID: 2},
		{ID: "other", Scope: "personal", OwnerUserID: 3},
	}
	visible := visibleMCPServersForUser(items, 2)
	if len(visible) != 2 || visible[0].ID != "project" || visible[1].ID != "mine" {
		t.Fatalf("visible items = %#v", visible)
	}
}

func TestUpdateCustomMCPKeepsScopeAndClearsIcon(t *testing.T) {
	st, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	original := domain.MCPServerDefinition{ID: "custom", Name: "Old", Scope: "project", ProjectID: "shop", IconURL: "https://broken.example/icon.svg", Transport: "http", URL: "https://old.example/mcp"}
	if err := st.PutMCPServer(original); err != nil {
		t.Fatal(err)
	}
	s := &Server{deps: &deps{repo: st}}
	s.mcp = &mcpHandlers{deps: s.deps}
	g := gin.New()
	g.PUT("/api/mcp-servers", s.mcp.mcpServers)
	req := httptest.NewRequest(http.MethodPut, "/api/mcp-servers", bytes.NewBufferString(`{"id":"custom","name":"New","scope":"personal","ownerUserId":99,"projectId":"other","iconUrl":"https://bad.example/icon.svg","transport":"http","url":"https://new.example/mcp"}`))
	rec := httptest.NewRecorder()
	g.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var saved domain.MCPServerDefinition
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Name != "New" || saved.Scope != "project" || saved.ProjectID != "shop" || saved.OwnerUserID != 0 || saved.IconURL != "" {
		t.Fatalf("saved MCP = %+v", saved)
	}
}

func TestUpdateMarketMCPIsRejected(t *testing.T) {
	st, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.PutMCPServer(domain.MCPServerDefinition{ID: "market", Name: "Market", MarketID: "github", Scope: "project", ProjectID: "platform", Transport: "http", URL: "https://example.com/mcp"}); err != nil {
		t.Fatal(err)
	}
	s := &Server{deps: &deps{repo: st}}
	s.mcp = &mcpHandlers{deps: s.deps}
	g := gin.New()
	g.PUT("/api/mcp-servers", s.mcp.mcpServers)
	req := httptest.NewRequest(http.MethodPut, "/api/mcp-servers", bytes.NewBufferString(`{"id":"market","name":"Changed","transport":"http","url":"https://changed.example/mcp"}`))
	rec := httptest.NewRecorder()
	g.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestMCPMarketIconsAreLocalAndEmbedded(t *testing.T) {
	static, err := fs.Sub(webassets.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	var withIcon int
	for _, item := range builtinMCPMarket {
		if strings.TrimSpace(item.IconURL) == "" {
			continue
		}
		withIcon++
		if !strings.HasPrefix(item.IconURL, "/mcp-icons/") {
			t.Fatalf("%s iconUrl = %q, want local /mcp-icons/", item.ID, item.IconURL)
		}
		name := strings.TrimPrefix(item.IconURL, "/mcp-icons/")
		if _, err := fs.ReadFile(static, "mcp-icons/"+name); err != nil {
			t.Fatalf("embedded icon %s: %v", name, err)
		}
	}
	if withIcon != len(builtinMCPMarket) {
		t.Fatalf("expected local icons, got %d", withIcon)
	}
}
