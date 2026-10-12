package tracker

import (
	"context"
	"errors"
	"time"
)

// Provider is the forge abstraction the tracker service talks to. The first
// implementation is GitHub (github.com or GitHub Enterprise); GitLab, Gitea or
// Jira-backed providers only need to satisfy this interface.
//
// Repository paths are "owner/name". Pull request states are normalized to
// "opened", "merged" or "closed".
type Provider interface {
	Name() string

	ListIssues(ctx context.Context, repo string) ([]RemoteIssue, error)
	GetIssue(ctx context.Context, repo string, number int) (RemoteIssue, error)
	CreateIssue(ctx context.Context, repo string, in IssueInput) (RemoteIssue, error)
	CommentIssue(ctx context.Context, repo string, number int, body string) error

	ListPullRequests(ctx context.Context, repo string, q PullRequestQuery) ([]PullRequest, error)
	GetPullRequest(ctx context.Context, repo string, number int) (PullRequest, error)
	CreatePullRequest(ctx context.Context, repo string, in PullRequestInput) (PullRequest, error)
	MergePullRequest(ctx context.Context, repo string, number int, commitTitle string) error
}

// ProviderFactory builds a provider bound to one credential.
type ProviderFactory func(token string) Provider

// RemoteIssue is a forge issue normalized for local import.
type RemoteIssue struct {
	ID        int64
	Number    int
	Title     string
	Body      string
	State     string // opened, closed
	Labels    []string
	Author    string
	Assignee  string
	URL       string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// IssueInput creates a forge issue.
type IssueInput struct {
	Title  string
	Body   string
	Labels []string
}

// PullRequest is a forge pull request normalized for delivery tracking.
type PullRequest struct {
	Number       int
	State        string // opened, merged, closed
	SourceBranch string
	TargetBranch string
	Title        string
	URL          string
	MergedAt     time.Time
	UpdatedAt    time.Time
}

// PullRequestQuery filters ListPullRequests. Empty fields are not filtered.
type PullRequestQuery struct {
	SourceBranch string
	TargetBranch string
	State        string // opened, closed, all (default all)
	Limit        int
}

// PullRequestInput creates a pull request.
type PullRequestInput struct {
	Title        string
	Body         string
	SourceBranch string
	TargetBranch string
}

// ErrNoOpenMR reports that no open pull request exists for the requested
// source branch. Callers treat it as a normal outcome (the SOP may deliver
// without a pull request).
var ErrNoOpenMR = errors.New("no open pull request for source branch")

// ErrNotConfigured reports that no forge credential is available.
var ErrNotConfigured = errors.New("tracker is not configured: set a Git token on the project owner or LOOPFORGE_GITHUB_TOKEN")
