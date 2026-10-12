package httpapi

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tencent/LoopForge/internal/engine"
	"github.com/Tencent/LoopForge/internal/repo"
	"github.com/Tencent/LoopForge/internal/runtime"
	"github.com/Tencent/LoopForge/internal/tracker"
)

func TestOTLPCollectorAcceptsLoopbackWithoutSessionWhenLoginRequired(t *testing.T) {
	t.Setenv("LOOPFORGE_AUTH_REQUIRED", "true")
	t.Setenv("LOOPFORGE_SESSION_SECRET", "otel-auth-secret")
	s, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	h := New(s, engine.New(s, runtime.NewManager(filepath.Join(t.TempDir(), "ws"))), tracker.NewService(), "", "")
	for remote, want := range map[string]int{"127.0.0.1:4000": http.StatusOK, "203.0.113.9:4000": http.StatusForbidden} {
		req := httptest.NewRequest(http.MethodPost, "/api/otel/v1/traces", strings.NewReader(""))
		req.RemoteAddr = remote
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("%s: status=%d body=%s, want %d", remote, rec.Code, rec.Body.String(), want)
		}
	}
}
