package domain

import (
	"fmt"
	"strings"
	"time"
)

type Project struct {
	BusinessDomainID string `json:"businessDomainId,omitempty"`
	ID               string `json:"id"`
	Key              string `json:"key"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	Color            string `json:"color"`
	WorkflowID       string `json:"workflowId,omitempty"`
	// SOPPlaybooks are named requirement combos (e.g. 完整需求 / 仅后端).
	// A requirement picks one; it does not inherit a single project-wide chain.
	SOPPlaybooks []SOPPlaybook `json:"sopPlaybooks,omitempty"`
	// SOPBindings is the legacy single playbook. Empty SOPPlaybooks still
	// expose it as the default combo so old projects keep working.
	SOPBindings         []IssueSOPBinding   `json:"sopBindings,omitempty"`
	DefaultRepositoryID string              `json:"defaultRepositoryId,omitempty"`
	OwnerID             int64               `json:"ownerId,omitempty"`
	Members             []ProjectMember     `json:"members,omitempty"`
	Clarification       ClarificationPolicy `json:"clarification"`
	RunEval             RunEvalPolicy       `json:"runEval,omitempty"`
	// Deprecated project-level credential fields are kept only so old payloads
	// can be decoded and scrubbed by the schema migration.
	GitToken                     string    `json:"-"`
	GitTokenConfigured           bool      `json:"-"`
	CodeBuddyAPIKey              string    `json:"-"`
	CodeBuddyAPIKeyConfigured    bool      `json:"-"`
	CodeBuddyInternetEnvironment string    `json:"-"`
	CreatedAt                    time.Time `json:"createdAt"`
	UpdatedAt                    time.Time `json:"updatedAt"`
}

func PublicProject(p Project) Project {
	p.GitToken = ""
	p.GitTokenConfigured = false
	p.CodeBuddyAPIKey = ""
	p.CodeBuddyAPIKeyConfigured = false
	return p
}

func PublicProjects(items []Project) []Project {
	out := make([]Project, len(items))
	for i, p := range items {
		out[i] = PublicProject(p)
	}
	return out
}

type ProjectMember struct {
	UserID int64  `json:"userId"`
	Role   string `json:"role"` // maintainer, member, viewer
}

type ClarificationDimension struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Question  string `json:"question"`
	Weight    int    `json:"weight"`
	MinLength int    `json:"minLength"`
}

type ClarificationPolicy struct {
	Enabled    bool                     `json:"enabled"`
	SkillID    string                   `json:"skillId,omitempty"`
	Threshold  int                      `json:"threshold"`
	Dimensions []ClarificationDimension `json:"dimensions,omitempty"`
}

type Repository struct {
	ID                 string    `json:"id"`
	ProjectID          string    `json:"projectId"`
	Provider           string    `json:"provider"`
	Name               string    `json:"name"`
	RepoPath           string    `json:"repoPath"`
	WebURL             string    `json:"webUrl,omitempty"`
	CloneURL           string    `json:"cloneUrl,omitempty"`
	DefaultBranch      string    `json:"defaultBranch"`
	LocalPath          string    `json:"localPath,omitempty"`
	GitToken           string    `json:"-"` // deprecated; credentials belong to users
	GitTokenConfigured bool      `json:"-"`
	PipelineURLPattern string    `json:"pipelineUrlPattern,omitempty"`
	IntegrateBranch    string    `json:"integrateBranch,omitempty"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

// PublicRepository strips the git token for API responses. Clone/push still
// reads the live Studio record; run snapshots never copy GitToken.
func PublicRepository(r Repository) Repository {
	r.GitTokenConfigured = false
	r.GitToken = ""
	return r
}

func PublicRepositories(items []Repository) []Repository {
	out := make([]Repository, len(items))
	for i, r := range items {
		out[i] = PublicRepository(r)
	}
	return out
}

type RepositoryContext struct {
	RepositoryID       string `json:"repositoryId"`
	Provider           string `json:"provider"`
	Name               string `json:"name"`
	RepoPath           string `json:"repoPath,omitempty"`
	WebURL             string `json:"webUrl,omitempty"`
	CloneURL           string `json:"cloneUrl,omitempty"`
	BaseBranch         string `json:"baseBranch"`
	WorkBranch         string `json:"workBranch,omitempty"`
	DefaultBranch      string `json:"defaultBranch,omitempty"`
	IntegrateBranch    string `json:"integrateBranch,omitempty"`
	IntegrationReworks int    `json:"integrationReworks,omitempty"`
	LocalPath          string `json:"localPath,omitempty"`
	PipelineURLPattern string `json:"pipelineUrlPattern,omitempty"`
	AlignOnly          bool   `json:"alignOnly,omitempty"`
	// WorkspaceDir is the first-level sub-directory of the run workspace this
	// repository is checked out into (slash-separated, relative). Empty is
	// only kept for persisted legacy runs that already checked out at the
	// workspace root; new runs always get a sub-directory so SOP deliverables
	// at the run root cannot pollute the git checkout.
	WorkspaceDir string `json:"workspaceDir,omitempty"`
}

// ForgeRepoHost is the Git hosting domain repository links are normalized
// against: github.com, or a GitHub Enterprise host set via
// LOOPFORGE_GITHUB_HOST at startup.
var ForgeRepoHost = "github.com"

// ForgeRepoInfo carries the repository identity parsed out of a pasted
// Forge link, so a repository can be configured with a single URL.
type ForgeRepoInfo struct {
	Project  string // group/repository, i.e. the RepoPath
	Name     string // repository (last path segment)
	WebURL   string // https://github.com/group/repository
	CloneURL string // https://github.com/group/repository.git
}

// forgeRepoSubpages are URL suffixes that point at a sub page of a
// repository (tree view, PR list, issue, ...). Everything before the first
// marker is the project path.
var forgeRepoSubpages = []string{
	"/-/", "/tree/", "/blob/", "/commit/", "/commits/", "/pull/", "/pulls",
	"/issues/", "/snippets/", "/wikis/", "/releases/", "/tags/",
}

// ParseForgeRepoURL parses a GitHub (or GitHub Enterprise) repository reference into the
// project path plus normalized URLs. Accepted inputs:
//   - https://github.com/group/repository
//   - https://github.com/group/repository/-/tree/master
//   - https://github.com/owner/repository/pull/12
//   - git@github.com:group/repository.git
//   - ssh://git@github.com:22/group/repository.git
//   - github.com/group/repository
//   - group/repository (bare project path, no URL decoration)
func ParseForgeRepoURL(raw string) (ForgeRepoInfo, bool) {
	s := strings.Trim(strings.TrimSpace(raw), `"'`)
	if len(s) >= 10 && strings.EqualFold(s[:10], "git clone ") {
		s = strings.Trim(strings.TrimSpace(s[10:]), `"'`)
		if i := strings.IndexAny(s, " \t"); i >= 0 {
			s = s[:i]
		}
	}
	if s == "" {
		return ForgeRepoInfo{}, false
	}
	if strings.ContainsAny(s, " \t\"'<>()") {
		return ForgeRepoInfo{}, false
	}
	sshClone := strings.HasPrefix(strings.ToLower(s), "git@") || strings.HasPrefix(strings.ToLower(s), "ssh://")
	s = strings.TrimSuffix(s, "/")
	// Strip scheme and userinfo so ssh / https / scp forms converge.
	pathSource := s
	if i := strings.Index(pathSource, "://"); i >= 0 {
		pathSource = pathSource[i+3:]
	}
	if i := strings.LastIndex(pathSource, "@"); i >= 0 {
		pathSource = pathSource[i+1:]
	}
	var path string
	if !strings.Contains(s, "://") && !strings.Contains(s, "@") && !strings.Contains(pathSource, ":") {
		// Bare project path (group/repository) pasted without URL decoration.
		path = pathSource
	} else if strings.Contains(pathSource, "/") {
		host := pathSource[:strings.IndexAny(pathSource, "/:")]
		if !strings.EqualFold(host, ForgeRepoHost) {
			return ForgeRepoInfo{}, false
		}
		if pathSource[len(host)] == ':' {
			// github.com:group/repo.git (scp) or github.com:22/group/repo.git
			path = pathSource[len(host)+1:]
			if j := strings.Index(path, "/"); j >= 0 && isAllDigits(path[:j]) {
				path = path[j+1:]
			}
		} else {
			path = pathSource[len(host)+1:]
		}
	} else {
		// bare host without path is not a repository reference
		return ForgeRepoInfo{}, false
	}
	project := normalizeForgeProjectPath(path)
	if project == "" {
		return ForgeRepoInfo{}, false
	}
	info := ForgeRepoInfo{Project: project}
	info.Name = project[strings.LastIndex(project, "/")+1:]
	info.WebURL = "https://" + ForgeRepoHost + "/" + project
	info.CloneURL = info.WebURL + ".git"
	if sshClone {
		info.CloneURL = "git@" + ForgeRepoHost + ":" + project + ".git"
	}
	return info, true
}

// normalizeForgeProjectPath strips clone suffixes and sub page markers from
// a repository path and validates the remaining "group/repository" shape.
func normalizeForgeProjectPath(path string) string {
	p := strings.TrimSpace(path)
	if strings.HasPrefix(p, "/") || strings.Contains(p, "..") {
		// absolute local paths are never repository paths
		return ""
	}
	p = strings.TrimPrefix(p, ForgeRepoHost+"/")
	if strings.HasSuffix(strings.ToLower(p), ".git") {
		p = p[:len(p)-4]
	}
	for _, marker := range forgeRepoSubpages {
		if i := strings.Index(p, marker); i >= 0 {
			p = p[:i]
		}
	}
	p = strings.Trim(p, "/")
	if p == "" || !strings.Contains(p, "/") {
		// must look like group/repository (at least two segments)
		return ""
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || strings.ContainsAny(seg, " \t\\?#") {
			return ""
		}
	}
	return p
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// IsSafeRepoPath reports whether s is a safe, single-segment legacy
// Forge project identifier. Repository paths and links must instead pass
// ParseForgeRepoURL; this compatibility path is intentionally limited so
// malformed paths cannot bypass that parser.
func IsSafeRepoPath(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || s == "." || s == ".." || strings.Contains(s, "..") {
		return false
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

// IsZero reports whether the context carries no usable repository at all.
func (rc RepositoryContext) IsZero() bool {
	return strings.TrimSpace(rc.RepositoryID) == "" &&
		strings.TrimSpace(rc.CloneURL) == "" &&
		strings.TrimSpace(rc.RepoPath) == "" &&
		strings.TrimSpace(rc.LocalPath) == ""
}

// NormalizeRepositoryDirName converts a repository name into a safe,
// single-level directory name used as the checkout under the run workspace.
func NormalizeRepositoryDirName(rc RepositoryContext) string {
	name := strings.TrimSpace(rc.Name)
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = strings.TrimSpace(rc.RepositoryID)
	}
	if out == "" {
		out = "repo"
	}
	return out
}

// AssignRepositoryDirs fills WorkspaceDir so every bound repository is
// checked out into a unique first-level sub-directory of the run workspace.
// SOP deliverables such as fix-plan.md stay at the run root and never enter
// a git checkout.
func AssignRepositoryDirs(repositories []RepositoryContext) {
	used := map[string]bool{}
	for i := range repositories {
		base := NormalizeRepositoryDirName(repositories[i])
		dir := base
		for n := 2; used[dir]; n++ {
			dir = fmt.Sprintf("%s-%d", base, n)
		}
		used[dir] = true
		repositories[i].WorkspaceDir = dir
	}
}
