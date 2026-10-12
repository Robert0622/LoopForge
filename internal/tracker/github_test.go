package tracker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGitHubListIssuesSkipsPullRequestsAndNormalizes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("authorization = %q", got)
		}
		if r.URL.Path != "/repos/acme/api/issues" || r.URL.Query().Get("state") != "all" {
			t.Errorf("unexpected request %s", r.URL)
		}
		_, _ = w.Write([]byte(`[
			{"id": 11, "number": 1, "title": "Bug", "state": "open", "html_url": "https://github.com/acme/api/issues/1", "labels": [{"name": "P1"}], "assignee": {"login": "alice"}},
			{"id": 12, "number": 2, "title": "PR", "state": "open", "pull_request": {}},
			{"id": 13, "number": 3, "title": "Done", "state": "closed"}
		]`))
	}))
	defer srv.Close()

	issues, err := NewGitHub(srv.URL, "tok").ListIssues(context.Background(), "acme/api")
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 2 {
		t.Fatalf("issues = %+v", issues)
	}
	if issues[0].State != "opened" || issues[0].Assignee != "alice" || labelPriority(issues[0].Labels) != "high" {
		t.Fatalf("first issue = %+v", issues[0])
	}
	if issues[1].State != "closed" {
		t.Fatalf("second issue = %+v", issues[1])
	}
}

func TestGitHubPullRequestStateAndHeadFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/api/pulls":
			if got := r.URL.Query().Get("head"); got != "acme:feature/x" {
				t.Errorf("head = %q", got)
			}
			_, _ = w.Write([]byte(`[
				{"number": 5, "state": "closed", "merged_at": "2026-01-02T03:04:05Z", "head": {"ref": "feature/x"}, "base": {"ref": "main"}},
				{"number": 6, "state": "open", "head": {"ref": "feature/x"}, "base": {"ref": "main"}}
			]`))
		case r.Method == http.MethodPut && r.URL.Path == "/repos/acme/api/pulls/6/merge":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["merge_method"] != "merge" {
				t.Errorf("merge body = %v", body)
			}
			_, _ = w.Write([]byte(`{"merged": true}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	gh := NewGitHub(srv.URL, "tok")
	prs, err := gh.ListPullRequests(context.Background(), "acme/api", PullRequestQuery{SourceBranch: "feature/x"})
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 2 || prs[0].State != "merged" || prs[1].State != "opened" || prs[1].TargetBranch != "main" {
		t.Fatalf("prs = %+v", prs)
	}
	if err := gh.MergePullRequest(context.Background(), "acme/api", 6, ""); err != nil {
		t.Fatal(err)
	}
}

func TestGitHubErrorsAreActionable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message": "Bad credentials"}`))
	}))
	defer srv.Close()
	_, err := NewGitHub(srv.URL, "bad").GetIssue(context.Background(), "acme/api", 1)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("err = %v", err)
	}
	if _, err := NewGitHub(srv.URL, "x").GetIssue(context.Background(), "not-a-repo", 1); err == nil {
		t.Fatal("invalid repository path must fail before calling the API")
	}
}

func TestParseIssueURL(t *testing.T) {
	ref, ok := ParseIssueURL("https://github.com/acme/api/issues/42#issuecomment-1")
	if !ok || ref.Repo != "acme/api" || ref.Number != 42 || ref.Host != "github.com" {
		t.Fatalf("ref = %+v ok=%v", ref, ok)
	}
	if _, ok := ParseIssueURL("https://ghe.example.com/team/svc/issues/7"); !ok {
		t.Fatal("GitHub Enterprise URLs must parse")
	}
	for _, bad := range []string{"", "acme/api#1", "https://github.com/acme/api/pull/3", "https://github.com/acme/api/issues/x"} {
		if _, ok := ParseIssueURL(bad); ok {
			t.Fatalf("%q must not parse", bad)
		}
	}
}
