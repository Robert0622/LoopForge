package preview

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tencent/LoopForge/internal/repo"
)

func TestSanitizeRelPathRejectsTraversal(t *testing.T) {
	for _, raw := range []string{"", "/abs", "..", "foo/../bar", "foo\\bar", "foo/%2e%2e/bar", "foo\x00bar"} {
		if _, err := SanitizeRelPath(raw); err == nil {
			t.Fatalf("expected rejection for %q", raw)
		}
	}
	got, err := SanitizeRelPath("frontend/dist")
	if err != nil || got != "frontend/dist" {
		t.Fatalf("got %q %v", got, err)
	}
}

func TestArchiveDirRejectsSymlinkAndMissingIndex(t *testing.T) {
	workspace := t.TempDir()
	dist := filepath.Join(workspace, "dist")
	if err := os.MkdirAll(dist, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "readme.txt"), []byte("no index"), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := ArchiveDir(workspace, "dist", "index.html", Limits{MaxFiles: 10, MaxCompressed: 1 << 20, MaxExpanded: 1 << 20}, &buf); err == nil {
		t.Fatal("expected missing index")
	}
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dist, "link.js")); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	if _, err := ArchiveDir(workspace, "dist", "index.html", Limits{MaxFiles: 10, MaxCompressed: 1 << 20, MaxExpanded: 1 << 20}, &buf); err == nil {
		t.Fatal("expected symlink rejection")
	}
}

func TestArchiveDirIsDeterministicAndRoundTrips(t *testing.T) {
	workspace := t.TempDir()
	dist := filepath.Join(workspace, "dist")
	if err := os.MkdirAll(filepath.Join(dist, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte("<html><script src=/assets/app.js></script></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "assets", "app.js"), []byte("console.log('ok')"), 0o644); err != nil {
		t.Fatal(err)
	}
	limits := Limits{MaxFiles: 20, MaxCompressed: 1 << 20, MaxExpanded: 1 << 20}
	var first, second bytes.Buffer
	a, err := ArchiveDir(workspace, "dist", "index.html", limits, &first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ArchiveDir(workspace, "dist", "index.html", limits, &second)
	if err != nil {
		t.Fatal(err)
	}
	if a.SHA256 == "" || a.SHA256 != b.SHA256 || first.Len() != second.Len() {
		t.Fatalf("archive was not deterministic: %+v %+v", a, b)
	}
	inspected, err := InspectTarZst(bytes.NewReader(first.Bytes()), "index.html", limits)
	if err != nil {
		t.Fatal(err)
	}
	if inspected.FileCount != 2 || inspected.SHA256 != a.SHA256 {
		t.Fatalf("inspect mismatch: %+v vs %+v", inspected, a)
	}
	dest := filepath.Join(t.TempDir(), "expanded")
	if err := ExtractTarZst(bytes.NewReader(first.Bytes()), dest, limits); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dest, "index.html"))
	if err != nil || !bytes.Contains(body, []byte("/assets/app.js")) {
		t.Fatalf("extracted index: %s %v", body, err)
	}
	member, err := ReadTarZstMember(bytes.NewReader(first.Bytes()), "assets/app.js", limits)
	if err != nil || !bytes.Contains(member, []byte("console.log")) {
		t.Fatalf("member extract: %s %v", member, err)
	}
}

func TestDBStoreRoundTrip(t *testing.T) {
	db, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	store := NewDBStore(db)
	payload := []byte("hello-preview")
	sum := sha256.Sum256(payload)
	id := hex.EncodeToString(sum[:])
	if err := store.Put(t.Context(), id, bytes.NewReader(payload), int64(len(payload)), id); err != nil {
		t.Fatal(err)
	}
	obj, meta, err := store.Open(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	defer obj.Close()
	if meta.Size != int64(len(payload)) {
		t.Fatalf("size %d", meta.Size)
	}
}

func TestParsePreviewPath(t *testing.T) {
	token := strings.Repeat("ab", 16)
	gotToken, rel, ok := ParsePreviewPath("/preview/" + token + "/assets/app.js")
	if !ok || gotToken != token || rel != "assets/app.js" {
		t.Fatalf("got %q %q %v", gotToken, rel, ok)
	}
	if _, _, ok := ParsePreviewPath("/api/runs/x/preview"); ok {
		t.Fatal("api preview path must not match gateway")
	}
	rewritten := rewriteRootRelativeRefs(`<script src="/assets/app.js"></script>`, "/preview/"+token)
	if !strings.Contains(rewritten, `/preview/`+token+`/assets/app.js`) {
		t.Fatalf("rewrite: %s", rewritten)
	}
}

func TestRewriteJSRootURLsPrefixesQuotedAssets(t *testing.T) {
	prefix := "/preview/" + strings.Repeat("ab", 16)
	src := `import("/assets/chunk.js");const a="/static/logo.svg";const b='/api/runs';const c="./assets/ok.js"`
	got := rewriteJSRootURLs(src, prefix)
	if !strings.Contains(got, `import("`+prefix+`/assets/chunk.js")`) {
		t.Fatalf("dynamic import not rewritten: %s", got)
	}
	if !strings.Contains(got, `"`+prefix+`/static/logo.svg"`) {
		t.Fatalf("static asset not rewritten: %s", got)
	}
	if !strings.Contains(got, `'/api/runs'`) {
		t.Fatalf("non-asset path must stay: %s", got)
	}
	if !strings.Contains(got, `"./assets/ok.js"`) {
		t.Fatalf("relative path must stay: %s", got)
	}
	again := rewriteJSRootURLs(got, prefix)
	if strings.Count(again, prefix+prefix) != 0 {
		t.Fatalf("rewrite must be idempotent: %s", again)
	}
}

func TestRewritePreviewHTMLFillsViewport(t *testing.T) {
	got := string(rewritePreviewHTML([]byte(`<html><head></head><body>hi</body></html>`), "/preview/tok", ""))
	if !strings.Contains(got, `data-af-preview-viewport`) || !strings.Contains(got, "min-height:100%") {
		t.Fatalf("viewport css missing: %s", got)
	}
	again := string(rewritePreviewHTML([]byte(got), "/preview/tok", ""))
	if strings.Count(again, `data-af-preview-viewport`) != 1 {
		t.Fatalf("viewport css should be idempotent: %s", again)
	}
}

func TestRewritePreviewHTMLInjectsBackend(t *testing.T) {
	got := string(rewritePreviewHTML([]byte(`<html><head></head><body>hi</body></html>`), "/preview/tok", "https://api.example.com/v1"))
	if !strings.Contains(got, `data-af-preview-backend`) || !strings.Contains(got, `window.__LOOPFORGE_PREVIEW_BACKEND__="https://api.example.com/v1"`) {
		t.Fatalf("backend inject missing: %s", got)
	}
	if !strings.Contains(got, `window.fetch = function`) || !strings.Contains(got, `XMLHttpRequest.prototype.open`) || !strings.Contains(got, `navigator.sendBeacon`) {
		t.Fatalf("backend intercept missing: %s", got)
	}
	if !strings.Contains(got, `var prefix = "/preview/tok";`) {
		t.Fatalf("preview prefix missing from intercept: %s", got)
	}
	if idx := strings.Index(got, `data-af-preview-backend`); idx < 0 || !strings.Contains(got[idx:], `function retarget`) {
		t.Fatalf("intercept must retarget cross-origin requests: %s", got)
	}
	again := string(rewritePreviewHTML([]byte(got), "/preview/tok", "https://evil.example"))
	if strings.Count(again, `data-af-preview-backend`) != 1 {
		t.Fatalf("backend inject should be idempotent: %s", again)
	}
	plain := string(rewritePreviewHTML([]byte(`<html><head></head><body>hi</body></html>`), "/preview/tok", ""))
	if strings.Contains(plain, `__LOOPFORGE_PREVIEW_BACKEND__`) {
		t.Fatalf("empty backend must not inject: %s", plain)
	}
}
