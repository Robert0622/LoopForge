package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/Tencent/LoopForge/internal/domain"
	"github.com/Tencent/LoopForge/internal/repo"
	"github.com/gin-gonic/gin"
)

func assistantFilesTestUser(t *testing.T, st *repo.Store, login string) domain.User {
	t.Helper()
	if err := st.PutUser(domain.User{LoginName: login, CodeBuddyAPIKey: login + "-key"}); err != nil {
		t.Fatal(err)
	}
	u, ok := st.UserByLoginName(login)
	if !ok {
		t.Fatal("user")
	}
	return u
}

func assistantFilesRequest(method, target string, body *bytes.Buffer, contentType string, u domain.User) (*gin.Context, *httptest.ResponseRecorder) {
	if body == nil {
		body = &bytes.Buffer{}
	}
	req := httptest.NewRequest(method, target, body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req = req.WithContext(context.WithValue(req.Context(), requestUserContextKey{}, domainUserToAuth(u)))
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	return c, rec
}

func TestAssistantFileUploadIsPrivateAndSandboxed(t *testing.T) {
	t.Setenv("LOOPFORGE_ASSISTANT_SESSION_DIR", t.TempDir())
	st, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	owner := assistantFilesTestUser(t, st, "owner")
	other := assistantFilesTestUser(t, st, "other")
	h := &assistantHandlers{deps: &deps{repo: st}}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, _ := mw.CreateFormFile("files", "../../设计 图.svg")
	part.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`))
	mw.Close()
	c, rec := assistantFilesRequest("POST", "/api/assistant/files", &body, mw.FormDataContentType(), owner)
	h.uploadFiles(c)
	var out struct {
		Files []domain.AssistantAttachment `json:"files"`
	}
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &out) != nil || len(out.Files) != 1 {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body.String())
	}
	f := out.Files[0]
	if f.Name != "设计 图.svg" || f.Mime != "image/svg+xml" || f.Size == 0 {
		t.Fatalf("meta %+v", f)
	}

	c, rec = assistantFilesRequest("GET", "/api/assistant/files/"+f.ID, nil, "", owner)
	c.Params = gin.Params{{Key: "id", Value: f.ID}}
	h.file(c)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "<svg") {
		t.Fatalf("owner download: %d", rec.Code)
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.HasPrefix(csp, "sandbox") {
		t.Fatalf("uploaded files must be served sandboxed, got %q", csp)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("nosniff missing")
	}

	c, rec = assistantFilesRequest("GET", "/api/assistant/files/"+f.ID, nil, "", other)
	c.Params = gin.Params{{Key: "id", Value: f.ID}}
	h.file(c)
	if rec.Code != 404 {
		t.Fatalf("another user must not read the file, got %d", rec.Code)
	}
	c, rec = assistantFilesRequest("GET", "/api/assistant/files/..%2F..", nil, "", owner)
	c.Params = gin.Params{{Key: "id", Value: "../../meta"}}
	h.file(c)
	if rec.Code != 404 {
		t.Fatalf("path ids must be rejected, got %d", rec.Code)
	}
}

func TestSubmitAssistantReplyRichFields(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		valid     bool
	}{
		{"questions", `{"message":"要确认两点","drafts":[],"questions":[{"id":"q1","question":"范围？","options":["当前页","全部"],"multiSelect":false}]}`, true},
		{"single option", `{"message":"x","drafts":[],"questions":[{"id":"q1","question":"范围？","options":["当前页"],"multiSelect":false}]}`, false},
		{"launch new", `{"message":"可以开跑","drafts":[],"launch":{"title":"导出 CSV","description":"目标……","workflowId":"quick-fix"}}`, true},
		{"launch existing", `{"message":"跑它","drafts":[],"launch":{"issueId":"iss-1"}}`, true},
		{"launch empty", `{"message":"x","drafts":[],"launch":{"title":"导出 CSV"}}`, false},
		{"svg file", `{"message":"图在这","drafts":[],"files":[{"name":"flow.svg","content":"<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>"}]}`, true},
		{"binary ext", `{"message":"x","drafts":[],"files":[{"name":"a.exe","content":"MZ"}]}`, false},
		{"fake svg", `{"message":"x","drafts":[],"files":[{"name":"a.svg","content":"hello"}]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseSubmitAssistantReplyArgs(json.RawMessage(tc.raw), "p")
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
}

func TestAssistantHomeTurnCarriesAttachmentsRefsAndSOPs(t *testing.T) {
	t.Setenv("LOOPFORGE_ASSISTANT_SESSION_DIR", t.TempDir())
	t.Setenv("LOOPFORGE_ASSISTANT_SOURCE_DIR", t.TempDir())
	st, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	u := assistantFilesTestUser(t, st, "pm")
	if err := st.PutProject(domain.Project{ID: "p", Name: "Platform"}); err != nil {
		t.Fatal(err)
	}
	if err := st.PutWorkflow(domain.WorkflowDefinition{ID: "p-fix", Name: "Fix it", ProjectID: "p", SandboxProfileID: "local", Nodes: []domain.WorkflowNode{{ID: "dev", Name: "实现", Type: "approval"}}}); err != nil {
		t.Fatal(err)
	}
	if err := st.PutIssue(domain.Issue{ID: "iss-1", ProjectID: "p", Title: "登录页报错提示", Description: "登录失败时提示具体原因"}); err != nil {
		t.Fatal(err)
	}
	meta, err := storeAssistantFile(u.ID, "shot.png", "image/png", "upload", strings.NewReader("png-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	var prompt string
	h := &assistantHandlers{deps: &deps{repo: st}, generate: func(ctx context.Context, p string, user domain.User) (string, error) {
		if prompt == "" {
			prompt = p
		}
		return "收到，我看了截图和这条需求，建议按 Fix it 开跑。", nil
	}}
	body := `{"projectId":"p","scene":"home","messages":[{"role":"user","content":"看下这个","attachments":[{"id":"` + meta.ID + `"}],"refs":[{"type":"issue","id":"iss-1"},{"type":"issue","id":"missing"}]}]}`
	c, rec := assistantFilesRequest("POST", "/api/assistant/chat", bytes.NewBufferString(body), "application/json", u)
	h.chat(c)
	if rec.Code != 200 {
		t.Fatalf("chat: %d %s", rec.Code, rec.Body.String())
	}
	staged := assistantCLIWorkspaceForUser(u.ID) + "/attachments/" + meta.ID + "/shot.png"
	if raw, err := os.ReadFile(staged); err != nil || string(raw) != "png-bytes" {
		t.Fatalf("attachment not staged into the CLI workspace: %v", err)
	}
	for _, want := range []string{staged, "登录页报错提示", "登录失败时提示具体原因", `"id":"p-fix"`, "实现"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("turn prompt is missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "missing") && strings.Contains(prompt, `"id":"missing"`) {
		t.Fatal("unknown refs must be dropped")
	}

	c, rec = assistantFilesRequest("POST", "/api/assistant/chat", bytes.NewBufferString(`{"messages":[{"role":"user","content":"x","attachments":[{"id":"aaaaaaaaaaaaaaaaaaaaaaaa"}]}]}`), "application/json", u)
	h.chat(c)
	if rec.Code != 400 {
		t.Fatalf("unknown attachment must be rejected, got %d", rec.Code)
	}
}

func TestResolveAssistantLaunchDropsUnknownTargets(t *testing.T) {
	st, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	st.PutProject(domain.Project{ID: "p", Name: "P"})
	st.PutWorkflow(domain.WorkflowDefinition{ID: "p-fix", Name: "Fix", ProjectID: "p", SandboxProfileID: "local", Nodes: []domain.WorkflowNode{{ID: "a", Type: "approval"}}})
	st.PutIssue(domain.Issue{ID: "iss-1", ProjectID: "p", Title: "已有需求"})
	h := &assistantHandlers{deps: &deps{repo: st}}
	got := h.resolveAssistantLaunch("p", &assistantLaunch{IssueID: "iss-1", WorkflowID: "made-up"})
	if got == nil || got.Title != "已有需求" || got.WorkflowID != "" {
		t.Fatalf("%+v", got)
	}
	if got := h.resolveAssistantLaunch("p", &assistantLaunch{IssueID: "nope"}); got != nil {
		t.Fatalf("launch for an unknown issue without a fallback must be dropped: %+v", got)
	}
	if got := h.resolveAssistantLaunch("p", &assistantLaunch{Title: "T", Description: "D", WorkflowID: "p-fix"}); got == nil || got.WorkflowID != "p-fix" {
		t.Fatalf("%+v", got)
	}
}
