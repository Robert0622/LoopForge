package tracker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultGitHubAPIURL is the public GitHub REST endpoint. GitHub Enterprise
// uses https://<host>/api/v3.
const DefaultGitHubAPIURL = "https://api.github.com"

// GitHub implements Provider on the GitHub REST API v3.
type GitHub struct {
	BaseURL string
	Token   string
	Client  *http.Client
}

// NewGitHub returns a GitHub provider. An empty baseURL targets github.com.
func NewGitHub(baseURL, token string) *GitHub {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = DefaultGitHubAPIURL
	}
	return &GitHub{BaseURL: baseURL, Token: strings.TrimSpace(token), Client: &http.Client{Timeout: 30 * time.Second}}
}

// GitHubFactory returns a ProviderFactory for the given API base URL.
func GitHubFactory(baseURL string) ProviderFactory {
	return func(token string) Provider { return NewGitHub(baseURL, token) }
}

func (g *GitHub) Name() string { return "github" }

// APIError is a non-2xx GitHub response.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	msg := strings.TrimSpace(e.Message)
	if e.Status == http.StatusUnauthorized {
		return "GitHub authentication failed (HTTP 401): check the Git token of the project owner"
	}
	if msg == "" {
		msg = http.StatusText(e.Status)
	}
	return fmt.Sprintf("GitHub API HTTP %d: %s", e.Status, msg)
}

func (g *GitHub) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	u := g.BaseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "loopforge")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if g.Token != "" {
		req.Header.Set("Authorization", "Bearer "+g.Token)
	}
	client := g.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var e struct {
			Message string `json:"message"`
			Errors  []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}
		_ = json.Unmarshal(raw, &e)
		msg := e.Message
		for _, item := range e.Errors {
			if strings.TrimSpace(item.Message) != "" {
				msg += ": " + item.Message
			}
		}
		return &APIError{Status: resp.StatusCode, Message: msg}
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func repoAPIPath(repo string) (string, error) {
	repo = strings.Trim(strings.TrimSpace(repo), "/")
	parts := strings.Split(repo, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("invalid GitHub repository %q, expected owner/name", repo)
	}
	return "/repos/" + url.PathEscape(parts[0]) + "/" + url.PathEscape(parts[1]), nil
}

type ghUser struct {
	Login string `json:"login"`
}

type ghIssue struct {
	ID          int64     `json:"id"`
	Number      int       `json:"number"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	State       string    `json:"state"`
	HTMLURL     string    `json:"html_url"`
	User        *ghUser   `json:"user"`
	Assignee    *ghUser   `json:"assignee"`
	PullRequest *struct{} `json:"pull_request"`
	Labels      []struct {
		Name string `json:"name"`
	} `json:"labels"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (i ghIssue) remote() RemoteIssue {
	out := RemoteIssue{
		ID: i.ID, Number: i.Number, Title: i.Title, Body: i.Body,
		State: normalizeIssueState(i.State), URL: i.HTMLURL,
		CreatedAt: i.CreatedAt, UpdatedAt: i.UpdatedAt,
	}
	if i.User != nil {
		out.Author = i.User.Login
	}
	if i.Assignee != nil {
		out.Assignee = i.Assignee.Login
	}
	for _, l := range i.Labels {
		out.Labels = append(out.Labels, l.Name)
	}
	return out
}

func normalizeIssueState(state string) string {
	if strings.EqualFold(strings.TrimSpace(state), "closed") {
		return "closed"
	}
	return "opened"
}

const maxIssuePages = 20

func (g *GitHub) ListIssues(ctx context.Context, repo string) ([]RemoteIssue, error) {
	base, err := repoAPIPath(repo)
	if err != nil {
		return nil, err
	}
	var out []RemoteIssue
	for page := 1; page <= maxIssuePages; page++ {
		q := url.Values{"state": {"all"}, "per_page": {"100"}, "page": {strconv.Itoa(page)}, "sort": {"updated"}, "direction": {"desc"}}
		var items []ghIssue
		if err := g.do(ctx, http.MethodGet, base+"/issues", q, nil, &items); err != nil {
			return nil, err
		}
		for _, it := range items {
			if it.PullRequest != nil {
				continue
			}
			out = append(out, it.remote())
		}
		if len(items) < 100 {
			break
		}
	}
	return out, nil
}

func (g *GitHub) GetIssue(ctx context.Context, repo string, number int) (RemoteIssue, error) {
	base, err := repoAPIPath(repo)
	if err != nil {
		return RemoteIssue{}, err
	}
	var it ghIssue
	if err := g.do(ctx, http.MethodGet, base+"/issues/"+strconv.Itoa(number), nil, nil, &it); err != nil {
		return RemoteIssue{}, err
	}
	if it.PullRequest != nil {
		return RemoteIssue{}, fmt.Errorf("%s#%d is a pull request, not an issue", repo, number)
	}
	return it.remote(), nil
}

func (g *GitHub) CreateIssue(ctx context.Context, repo string, in IssueInput) (RemoteIssue, error) {
	base, err := repoAPIPath(repo)
	if err != nil {
		return RemoteIssue{}, err
	}
	body := map[string]any{"title": in.Title, "body": in.Body}
	if len(in.Labels) > 0 {
		body["labels"] = in.Labels
	}
	var it ghIssue
	if err := g.do(ctx, http.MethodPost, base+"/issues", nil, body, &it); err != nil {
		return RemoteIssue{}, err
	}
	return it.remote(), nil
}

func (g *GitHub) CommentIssue(ctx context.Context, repo string, number int, body string) error {
	base, err := repoAPIPath(repo)
	if err != nil {
		return err
	}
	return g.do(ctx, http.MethodPost, base+"/issues/"+strconv.Itoa(number)+"/comments", nil, map[string]any{"body": body}, nil)
}

type ghPull struct {
	Number   int        `json:"number"`
	State    string     `json:"state"`
	Title    string     `json:"title"`
	HTMLURL  string     `json:"html_url"`
	MergedAt *time.Time `json:"merged_at"`
	Updated  time.Time  `json:"updated_at"`
	Head     struct {
		Ref string `json:"ref"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
}

func (p ghPull) remote() PullRequest {
	out := PullRequest{
		Number: p.Number, Title: p.Title, URL: p.HTMLURL,
		SourceBranch: p.Head.Ref, TargetBranch: p.Base.Ref, UpdatedAt: p.Updated,
	}
	switch {
	case p.MergedAt != nil && !p.MergedAt.IsZero():
		out.State = "merged"
		out.MergedAt = *p.MergedAt
	case strings.EqualFold(p.State, "closed"):
		out.State = "closed"
	default:
		out.State = "opened"
	}
	return out
}

func (g *GitHub) ListPullRequests(ctx context.Context, repo string, q PullRequestQuery) ([]PullRequest, error) {
	base, err := repoAPIPath(repo)
	if err != nil {
		return nil, err
	}
	limit := q.Limit
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	state := "all"
	switch strings.ToLower(strings.TrimSpace(q.State)) {
	case "opened", "open":
		state = "open"
	case "closed", "merged":
		state = "closed"
	}
	values := url.Values{"state": {state}, "per_page": {strconv.Itoa(limit)}, "sort": {"updated"}, "direction": {"desc"}}
	if src := strings.TrimSpace(q.SourceBranch); src != "" {
		owner := strings.SplitN(strings.Trim(repo, "/"), "/", 2)[0]
		values.Set("head", owner+":"+src)
	}
	if tgt := strings.TrimSpace(q.TargetBranch); tgt != "" {
		values.Set("base", tgt)
	}
	var items []ghPull
	if err := g.do(ctx, http.MethodGet, base+"/pulls", values, nil, &items); err != nil {
		return nil, err
	}
	out := make([]PullRequest, 0, len(items))
	for _, it := range items {
		pr := it.remote()
		if strings.EqualFold(q.State, "merged") && pr.State != "merged" {
			continue
		}
		out = append(out, pr)
	}
	return out, nil
}

func (g *GitHub) GetPullRequest(ctx context.Context, repo string, number int) (PullRequest, error) {
	base, err := repoAPIPath(repo)
	if err != nil {
		return PullRequest{}, err
	}
	var it ghPull
	if err := g.do(ctx, http.MethodGet, base+"/pulls/"+strconv.Itoa(number), nil, nil, &it); err != nil {
		return PullRequest{}, err
	}
	return it.remote(), nil
}

func (g *GitHub) CreatePullRequest(ctx context.Context, repo string, in PullRequestInput) (PullRequest, error) {
	base, err := repoAPIPath(repo)
	if err != nil {
		return PullRequest{}, err
	}
	var it ghPull
	body := map[string]any{"title": in.Title, "body": in.Body, "head": in.SourceBranch, "base": in.TargetBranch}
	if err := g.do(ctx, http.MethodPost, base+"/pulls", nil, body, &it); err != nil {
		return PullRequest{}, err
	}
	return it.remote(), nil
}

func (g *GitHub) MergePullRequest(ctx context.Context, repo string, number int, commitTitle string) error {
	base, err := repoAPIPath(repo)
	if err != nil {
		return err
	}
	body := map[string]any{"merge_method": "merge"}
	if t := strings.TrimSpace(commitTitle); t != "" {
		body["commit_title"] = t
	}
	return g.do(ctx, http.MethodPut, base+"/pulls/"+strconv.Itoa(number)+"/merge", nil, body, nil)
}
