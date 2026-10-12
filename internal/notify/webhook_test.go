package notify

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHumanWaitPostsRobotShape(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("LOOPFORGE_NOTIFY_WEBHOOK", srv.URL)
	HumanWait("run-1", "确认改动", "加导出", "http://localhost/#approvals")
	if got["msgtype"] != "text" {
		t.Fatalf("msgtype=%v body=%v", got["msgtype"], got)
	}
	text, _ := got["text"].(map[string]any)
	content, _ := text["content"].(string)
	for _, part := range []string{"待你确认", "加导出", "确认改动"} {
		if !strings.Contains(content, part) {
			t.Fatalf("content missing %q: %q", part, content)
		}
	}
}
