package tracker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Tencent/LoopForge/internal/domain"
)

type fakeProvider struct {
	mu       sync.Mutex
	token    string
	issues   map[int]RemoteIssue
	pulls    []PullRequest
	comments []string
	merged   []int
	created  []PullRequestInput
}

func (f *fakeProvider) Name() string { return "fake" }

func (f *fakeProvider) ListIssues(ctx context.Context, repo string) ([]RemoteIssue, error) {
	var out []RemoteIssue
	for _, it := range f.issues {
		out = append(out, it)
	}
	return out, nil
}

func (f *fakeProvider) GetIssue(ctx context.Context, repo string, number int) (RemoteIssue, error) {
	it, ok := f.issues[number]
	if !ok {
		return RemoteIssue{}, errors.New("not found")
	}
	return it, nil
}

func (f *fakeProvider) CreateIssue(ctx context.Context, repo string, in IssueInput) (RemoteIssue, error) {
	n := len(f.issues) + 100
	it := RemoteIssue{ID: int64(n * 10), Number: n, Title: in.Title, Body: in.Body, State: "opened", URL: fmt.Sprintf("https://github.com/%s/issues/%d", repo, n)}
	f.issues[n] = it
	return it, nil
}

func (f *fakeProvider) CommentIssue(ctx context.Context, repo string, number int, body string) error {
	f.comments = append(f.comments, fmt.Sprintf("%s#%d:%s", repo, number, body))
	return nil
}

func (f *fakeProvider) ListPullRequests(ctx context.Context, repo string, q PullRequestQuery) ([]PullRequest, error) {
	var out []PullRequest
	for _, pr := range f.pulls {
		if q.SourceBranch != "" && pr.SourceBranch != q.SourceBranch {
			continue
		}
		if q.TargetBranch != "" && pr.TargetBranch != q.TargetBranch {
			continue
		}
		if q.State == "opened" && pr.State != "opened" {
			continue
		}
		out = append(out, pr)
	}
	return out, nil
}

func (f *fakeProvider) GetPullRequest(ctx context.Context, repo string, number int) (PullRequest, error) {
	for _, pr := range f.pulls {
		if pr.Number == number {
			return pr, nil
		}
	}
	return PullRequest{}, errors.New("not found")
}

func (f *fakeProvider) CreatePullRequest(ctx context.Context, repo string, in PullRequestInput) (PullRequest, error) {
	f.created = append(f.created, in)
	pr := PullRequest{Number: 50 + len(f.pulls), State: "opened", SourceBranch: in.SourceBranch, TargetBranch: in.TargetBranch}
	f.pulls = append(f.pulls, pr)
	return pr, nil
}

func (f *fakeProvider) MergePullRequest(ctx context.Context, repo string, number int, title string) error {
	f.merged = append(f.merged, number)
	return nil
}

type memStore struct {
	issues   map[string]domain.Issue
	repos    []domain.Repository
	projects []domain.Project
	users    map[int64]domain.User
	cfg      domain.SyncConfig
}

func newMemStore() *memStore {
	return &memStore{
		issues:   map[string]domain.Issue{},
		repos:    []domain.Repository{{ID: "repo-1", ProjectID: "p1", Provider: "github", RepoPath: "acme/api", DefaultBranch: "main"}},
		projects: []domain.Project{{ID: "p1", Name: "P1", OwnerID: 1}},
		users:    map[int64]domain.User{1: {ID: 1, LoginName: "owner", GitToken: "owner-token"}, 2: {ID: 2, LoginName: "alice"}},
	}
}

func (m *memStore) Issues() []domain.Issue {
	out := make([]domain.Issue, 0, len(m.issues))
	for _, v := range m.issues {
		out = append(out, v)
	}
	return out
}
func (m *memStore) Issue(id string) (domain.Issue, bool) { v, ok := m.issues[id]; return v, ok }
func (m *memStore) Repositories() []domain.Repository    { return m.repos }
func (m *memStore) Repository(id string) (domain.Repository, bool) {
	for _, r := range m.repos {
		if r.ID == id {
			return r, true
		}
	}
	return domain.Repository{}, false
}
func (m *memStore) Projects() []domain.Project { return m.projects }
func (m *memStore) Project(id string) (domain.Project, bool) {
	for _, p := range m.projects {
		if p.ID == id {
			return p, true
		}
	}
	return domain.Project{}, false
}
func (m *memStore) PutIssue(v domain.Issue) error             { m.issues[v.ID] = v; return nil }
func (m *memStore) DeleteIssue(id string) error               { delete(m.issues, id); return nil }
func (m *memStore) PutSyncConfig(cfg domain.SyncConfig) error { m.cfg = cfg; return nil }
func (m *memStore) SyncConfig() domain.SyncConfig             { return m.cfg }
func (m *memStore) User(id int64) (domain.User, bool)         { u, ok := m.users[id]; return u, ok }
func (m *memStore) UserByLoginName(login string) (domain.User, bool) {
	for _, u := range m.users {
		if u.LoginName == login {
			return u, true
		}
	}
	return domain.User{}, false
}

func newTestService(store *memStore, fp *fakeProvider) *Service {
	s := NewServiceWithProvider(func(token string) Provider {
		fp.mu.Lock()
		fp.token = token
		fp.mu.Unlock()
		return fp
	})
	s.ConfigureWithStore(domain.SyncConfig{}, store)
	return s
}

func TestPullAllUsesOwnerTokenAndKeepsLocalState(t *testing.T) {
	store := newMemStore()
	fp := &fakeProvider{issues: map[int]RemoteIssue{
		7: {ID: 70, Number: 7, Title: "Crash on start", State: "opened", CreatedAt: time.Now()},
	}}
	s := newTestService(store, fp)
	if err := s.PullAll(store); err != nil {
		t.Fatal(err)
	}
	if fp.token != "owner-token" {
		t.Fatalf("token = %q, want project owner token", fp.token)
	}
	issue, ok := store.Issue("gh-repo-1-7")
	if !ok || issue.Source != Source || issue.Status != "todo" || issue.ExternalRef.ExternalURL != "https://github.com/acme/api/issues/7" {
		t.Fatalf("issue = %+v ok=%v", issue, ok)
	}

	issue.Status = "in_progress"
	issue.WorkflowID = "wf-1"
	_ = store.PutIssue(issue)
	fp.issues[7] = RemoteIssue{ID: 70, Number: 7, Title: "Crash on start (edited)", State: "opened"}
	if err := s.PullAll(store); err != nil {
		t.Fatal(err)
	}
	issue, _ = store.Issue("gh-repo-1-7")
	if issue.Status != "in_progress" || issue.WorkflowID != "wf-1" || issue.Title != "Crash on start (edited)" {
		t.Fatalf("local execution state must survive a pull: %+v", issue)
	}

	fp.issues[7] = RemoteIssue{ID: 70, Number: 7, Title: "Crash on start (edited)", State: "closed"}
	_ = s.PullAll(store)
	issue, _ = store.Issue("gh-repo-1-7")
	if issue.Status != "closed" || issue.CloseSource != "external" {
		t.Fatalf("remote close must win: %+v", issue)
	}
}

func TestPullFallsBackToServiceToken(t *testing.T) {
	store := newMemStore()
	store.users[1] = domain.User{ID: 1, LoginName: "owner"}
	fp := &fakeProvider{issues: map[int]RemoteIssue{}}
	s := newTestService(store, fp)
	if err := s.PullAll(store); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
	s.SetAuthToken("env-token")
	if err := s.PullAll(store); err != nil {
		t.Fatal(err)
	}
	if fp.token != "env-token" {
		t.Fatalf("token = %q", fp.token)
	}
}

func TestImportIssueURLCreatesGatedRequirement(t *testing.T) {
	store := newMemStore()
	fp := &fakeProvider{issues: map[int]RemoteIssue{
		9: {ID: 90, Number: 9, Title: "Add SSO", Body: "We need SSO", State: "opened", Assignee: "alice"},
	}}
	s := newTestService(store, fp)
	var notified []string
	s.OnRequirementImported = func(issue domain.Issue) { notified = append(notified, issue.ID) }

	issue, err := s.ImportIssueURL(store, "p1", "https://github.com/acme/api/issues/9", 1)
	if err != nil {
		t.Fatal(err)
	}
	if issue.Kind != "requirement" || !issue.ExternalRef.Imported || issue.AssigneeUserID != 2 || issue.RepositoryID != "repo-1" {
		t.Fatalf("issue = %+v", issue)
	}
	if _, err := s.ImportIssueURL(store, "p1", "https://github.com/other/repo/issues/9", 1); err == nil {
		t.Fatal("repositories not bound to the project must be rejected")
	}
	_ = notified
}

func TestWriteBackClarificationComments(t *testing.T) {
	store := newMemStore()
	fp := &fakeProvider{issues: map[int]RemoteIssue{}}
	s := newTestService(store, fp)
	issue := domain.Issue{ID: "gh-repo-1-3", ProjectID: "p1", RepositoryID: "repo-1", Source: Source,
		ExternalRef: &domain.ExternalRef{Source: Source, ExternalIID: 3, ExternalURL: "https://github.com/acme/api/issues/3"}}
	if err := s.WriteBackClarification(store, issue, "summary"); err != nil {
		t.Fatal(err)
	}
	if len(fp.comments) != 1 || fp.comments[0] != "acme/api#3:summary" {
		t.Fatalf("comments = %v", fp.comments)
	}
	if err := s.WriteBackClarification(store, domain.Issue{ID: "local"}, "summary"); err != nil {
		t.Fatalf("local issues are a no-op: %v", err)
	}
}

func TestEnsureAndMergeBranchCreatesThenMerges(t *testing.T) {
	store := newMemStore()
	fp := &fakeProvider{issues: map[int]RemoteIssue{}}
	s := newTestService(store, fp)

	ref, err := s.EnsureAndMergeBranchMR("acme/api", "loopforge/integration", "main", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(fp.created) != 1 || len(fp.merged) != 1 || ref != fmt.Sprintf("#%d", fp.merged[0]) {
		t.Fatalf("created=%v merged=%v ref=%q", fp.created, fp.merged, ref)
	}
	if _, err := s.EnsureAndMergeBranchMR("acme/api", "feature/none", "", "", ""); !errors.Is(err, ErrNoOpenMR) {
		t.Fatalf("err = %v, want ErrNoOpenMR", err)
	}
	if ref, err := s.EnsureAndMergeBranchMR("acme/api", "main", "main", "", ""); err != nil || ref != "" {
		t.Fatalf("same branch must be a no-op: %q %v", ref, err)
	}
}

func TestFindBranchMRPrefersLiveOverClosed(t *testing.T) {
	store := newMemStore()
	fp := &fakeProvider{issues: map[int]RemoteIssue{}, pulls: []PullRequest{
		{Number: 3, State: "closed", SourceBranch: "feature/x", TargetBranch: "main"},
		{Number: 2, State: "merged", SourceBranch: "feature/x", TargetBranch: "main"},
		{Number: 4, State: "closed", SourceBranch: "feature/x", TargetBranch: "main"},
	}}
	s := newTestService(store, fp)
	mr, ok, err := s.FindBranchMR("acme/api", "feature/x")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if mr.IID != 2 || mr.ForgeState != "merged" || mr.URL != "https://github.com/acme/api/pull/2" {
		t.Fatalf("mr = %+v", mr)
	}
}

func TestPruneDuplicateExternalIssuesKeepsWorkedCopy(t *testing.T) {
	store := newMemStore()
	ref := func() *domain.ExternalRef {
		return &domain.ExternalRef{Source: Source, ExternalIID: 5, ExternalURL: "https://github.com/acme/api/issues/5"}
	}
	_ = store.PutIssue(domain.Issue{ID: "gh-repo-1-5", RepositoryID: "repo-1", Source: Source, ExternalRef: ref()})
	_ = store.PutIssue(domain.Issue{ID: "issue-legacy", RepositoryID: "repo-1", Source: Source, LatestTaskID: "task-1", ExternalRef: ref()})
	pruneDuplicateExternalIssues(store)
	if len(store.issues) != 1 {
		t.Fatalf("issues = %v", store.issues)
	}
	if _, ok := store.issues["issue-legacy"]; !ok {
		t.Fatal("the copy with task history must be kept")
	}
}
