package tracker

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Tencent/LoopForge/internal/domain"
)

// Source is the issue/source value written for forge-backed work items.
const Source = "github"

// Service orchestrates sync between the local store and the forge: issues are
// pulled and created through a Provider, clarifications are written back as
// comments, and delivery pull requests are created, merged and tracked.
type Service struct {
	cfg                   *syncConfigHolder
	newProvider           ProviderFactory
	authToken             string
	prCache               sync.Map
	logs                  []domain.SyncLog
	logsMu                sync.Mutex
	syncing               bool
	syncMu                sync.Mutex
	autoMu                sync.Mutex
	ticker                *time.Ticker
	stopCh                chan struct{}
	mrTicker              *time.Ticker
	mrStopCh              chan struct{}
	deliveryReconcileOnce sync.Once
	projectAuth           ProjectAuthStore
	OnRequirementImported func(issue domain.Issue)
}

type syncConfigHolder struct {
	mu   sync.RWMutex
	data domain.SyncConfig
}

func (h *syncConfigHolder) Get() domain.SyncConfig {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.data
}

func (h *syncConfigHolder) Set(cfg domain.SyncConfig) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.data = cfg
}

// IssueStoreAdapter abstracts the store operations the sync service needs.
type IssueStoreAdapter interface {
	Issues() []domain.Issue
	Issue(id string) (domain.Issue, bool)
	Repositories() []domain.Repository
	Repository(id string) (domain.Repository, bool)
	Projects() []domain.Project
	Project(id string) (domain.Project, bool)
	PutIssue(v domain.Issue) error
	DeleteIssue(id string) error
	PutSyncConfig(cfg domain.SyncConfig) error
	SyncConfig() domain.SyncConfig
}

// ProjectAuthStore provides the ownership and credential context needed to
// authenticate project-scoped forge calls.
type ProjectAuthStore interface {
	Projects() []domain.Project
	Project(id string) (domain.Project, bool)
	Repositories() []domain.Repository
	User(id int64) (domain.User, bool)
}

// DeliveryReconcileStore backfills delivery lock fields after external sync.
type DeliveryReconcileStore interface {
	ReconcileMergedMRDeliveries() error
	ReconcileDeliveryMRStates() error
}

type userDirectory interface {
	UserByLoginName(string) (domain.User, bool)
}

// NewService returns a service backed by GitHub. The API endpoint defaults to
// api.github.com and can be pointed at GitHub Enterprise with
// LOOPFORGE_GITHUB_API_URL or SyncConfig.APIBaseURL.
func NewService() *Service {
	s := &Service{cfg: &syncConfigHolder{}}
	s.newProvider = func(token string) Provider {
		return NewGitHub(s.apiBaseURL(), token)
	}
	return s
}

// NewServiceWithProvider returns a service using a custom provider factory.
func NewServiceWithProvider(factory ProviderFactory) *Service {
	s := NewService()
	if factory != nil {
		s.newProvider = factory
	}
	return s
}

// APIBaseURL returns the effective GitHub REST endpoint.
func (s *Service) APIBaseURL() string {
	if v := s.apiBaseURL(); v != "" {
		return strings.TrimRight(v, "/")
	}
	return DefaultGitHubAPIURL
}

func (s *Service) apiBaseURL() string {
	if v := strings.TrimSpace(s.cfg.Get().APIBaseURL); v != "" {
		return v
	}
	return strings.TrimSpace(os.Getenv("LOOPFORGE_GITHUB_API_URL"))
}

// Configure updates the sync config.
func (s *Service) Configure(cfg domain.SyncConfig) {
	s.cfg.Set(cfg)
}

// ConfigureWithStore enables project-aware authentication: forge calls use the
// Git token of the project owner (or of the issue assignee for write-back).
func (s *Service) ConfigureWithStore(cfg domain.SyncConfig, store ProjectAuthStore) {
	s.projectAuth = store
	s.Configure(cfg)
}

// SetAuthToken sets the fallback forge token used when a project owner has no
// personal Git token. The token must not include the "Bearer " prefix.
func (s *Service) SetAuthToken(token string) {
	s.authToken = strings.TrimSpace(token)
}

// Enabled reports whether any credential source is configured.
func (s *Service) Enabled() bool {
	return s.authToken != "" || s.projectAuth != nil
}

func (s *Service) providerForToken(token string) (Provider, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		token = s.authToken
	}
	if token == "" {
		return nil, ErrNotConfigured
	}
	return s.newProvider(token), nil
}

func (s *Service) providerForProject(projectID string) (Provider, error) {
	if s.projectAuth == nil {
		return s.providerForToken("")
	}
	project, ok := s.projectAuth.Project(strings.TrimSpace(projectID))
	if !ok {
		return nil, fmt.Errorf("project %s not found", projectID)
	}
	if project.OwnerID > 0 {
		if owner, ok := s.projectAuth.User(project.OwnerID); ok && strings.TrimSpace(owner.GitToken) != "" {
			return s.providerForToken(owner.GitToken)
		}
	}
	return s.providerForToken("")
}

func (s *Service) providerForWriteBack(projectID string, issue domain.Issue) (Provider, error) {
	if s.projectAuth != nil && issue.AssigneeUserID > 0 {
		if user, ok := s.projectAuth.User(issue.AssigneeUserID); ok && strings.TrimSpace(user.GitToken) != "" {
			return s.providerForToken(user.GitToken)
		}
	}
	return s.providerForProject(projectID)
}

func (s *Service) providerForRepoPath(repoPath string) (Provider, error) {
	repoPath = strings.TrimSpace(repoPath)
	if s.projectAuth != nil {
		for _, repo := range s.projectAuth.Repositories() {
			if strings.EqualFold(strings.TrimSpace(repo.RepoPath), repoPath) {
				return s.providerForProject(repo.ProjectID)
			}
		}
	}
	return s.providerForToken("")
}

// StartAutoSync launches a background goroutine that periodically pulls.
func (s *Service) StartAutoSync(store IssueStoreAdapter) {
	s.autoMu.Lock()
	defer s.autoMu.Unlock()
	s.stopAutoSyncLocked()
	cfg := s.cfg.Get()
	if !cfg.AutoSyncEnabled || cfg.AutoSyncIntervalSec <= 0 {
		return
	}
	interval := time.Duration(cfg.AutoSyncIntervalSec) * time.Second
	if interval < 30*time.Second {
		interval = 30 * time.Second
	}
	s.ticker = time.NewTicker(interval)
	s.stopCh = make(chan struct{})
	ticker, stopCh := s.ticker, s.stopCh
	go func() {
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				if err := s.PullAll(store); err != nil {
					log.Printf("auto-sync error: %v", err)
				}
			}
		}
	}()
}

// StopAutoSync halts the background ticker.
func (s *Service) StopAutoSync() {
	s.autoMu.Lock()
	defer s.autoMu.Unlock()
	s.stopAutoSyncLocked()
}

func (s *Service) stopAutoSyncLocked() {
	if s.ticker != nil {
		s.ticker.Stop()
		s.ticker = nil
	}
	if s.stopCh != nil {
		close(s.stopCh)
		s.stopCh = nil
	}
}

// Logs returns the recent sync log entries (most recent first).
func (s *Service) Logs() []domain.SyncLog {
	s.logsMu.Lock()
	defer s.logsMu.Unlock()
	out := make([]domain.SyncLog, 0, len(s.logs))
	out = append(out, s.logs...)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	if len(out) > 100 {
		out = out[:100]
	}
	return out
}

func (s *Service) addLog(l domain.SyncLog) {
	s.logsMu.Lock()
	defer s.logsMu.Unlock()
	if l.ID == "" {
		l.ID = fmt.Sprintf("sync-%d", l.StartedAt.UnixNano())
	}
	if l.Source == "" {
		l.Source = Source
	}
	if l.FinishedAt.IsZero() {
		l.FinishedAt = time.Now().UTC()
	}
	s.logs = append(s.logs, l)
	if len(s.logs) > 200 {
		s.logs = s.logs[len(s.logs)-200:]
	}
}

func (s *Service) beginSync() bool {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	if s.syncing {
		return false
	}
	s.syncing = true
	return true
}

func (s *Service) endSync() {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	s.syncing = false
}

func (s *Service) IsSyncing() bool {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	return s.syncing
}

// PullAll pulls forge issues of every bound repository into the store.
func (s *Service) PullAll(store IssueStoreAdapter) error {
	if !s.beginSync() {
		return fmt.Errorf("sync already in progress")
	}
	defer s.endSync()

	_, lastErr := s.pullRepositories(store, "")

	pruneDuplicateExternalIssues(store)
	if rs, ok := store.(DeliveryReconcileStore); ok {
		if err := rs.ReconcileMergedMRDeliveries(); err != nil && lastErr == nil {
			lastErr = fmt.Errorf("delivery reconcile: %w", err)
		}
		_ = rs.ReconcileDeliveryMRStates()
	}

	cfg := s.cfg.Get()
	cfg.LastSyncAt = time.Now().UTC()
	if lastErr != nil {
		cfg.LastSyncStatus = "failed"
		cfg.LastSyncError = lastErr.Error()
	} else {
		cfg.LastSyncStatus = "success"
		cfg.LastSyncError = ""
	}
	s.cfg.Set(cfg)
	_ = store.PutSyncConfig(cfg)
	return lastErr
}

// PullScheduleJob pulls the issues a scheduled job consumes.
func (s *Service) PullScheduleJob(store IssueStoreAdapter, job domain.ScheduleJob) (int, []string, error) {
	if !s.beginSync() {
		return 0, nil, fmt.Errorf("sync already in progress")
	}
	defer s.endSync()
	switch strings.ToLower(strings.TrimSpace(job.Source)) {
	case Source, "":
		count, err := s.pullRepositories(store, job.ProjectID)
		return count, nil, err
	default:
		return 0, nil, fmt.Errorf("unknown schedule source %q", job.Source)
	}
}

func (s *Service) pullRepositories(store IssueStoreAdapter, projectID string) (int, error) {
	count := 0
	var lastErr error
	for _, repo := range store.Repositories() {
		if projectID != "" && repo.ProjectID != projectID {
			continue
		}
		if !isForgeRepository(repo) {
			continue
		}
		provider, err := s.providerForProject(repo.ProjectID)
		if err != nil {
			lastErr = err
			continue
		}
		n, err := s.pullIssues(provider, store, repo)
		count += n
		if err != nil {
			lastErr = err
		}
	}
	return count, lastErr
}

func isForgeRepository(repo domain.Repository) bool {
	return strings.EqualFold(strings.TrimSpace(repo.Provider), Source) && strings.TrimSpace(repo.RepoPath) != ""
}

func (s *Service) pullIssues(provider Provider, store IssueStoreAdapter, repo domain.Repository) (int, error) {
	started := time.Now().UTC()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	remote, err := provider.ListIssues(ctx, repo.RepoPath)
	if err != nil {
		s.addLog(domain.SyncLog{Direction: "pull", Action: "sync_issues", Status: "failed", Message: err.Error(), ProjectID: repo.ProjectID, StartedAt: started})
		return 0, err
	}
	count := 0
	for _, ri := range remote {
		issue := remoteToIssue(repo, ri, "issue")
		if existing, found := findForgeIssue(store, repo, ri.Number); found {
			applyExistingIssue(&issue, existing)
		} else {
			issue.Number = ri.Number
			issue.CreatedAt = ri.CreatedAt
		}
		issue.UpdatedAt = time.Now().UTC()
		if err := store.PutIssue(issue); err != nil {
			log.Printf("sync issue %s#%d: %v", repo.RepoPath, ri.Number, err)
			continue
		}
		count++
	}
	s.addLog(domain.SyncLog{Direction: "pull", Action: "sync_issues", Status: "success", Count: count,
		Message: fmt.Sprintf("synced %d issues from %s", count, repo.RepoPath), ProjectID: repo.ProjectID, StartedAt: started})
	return count, nil
}

func forgeIssueID(repoID string, number int) string {
	return fmt.Sprintf("gh-%s-%d", repoID, number)
}

func remoteToIssue(repo domain.Repository, ri RemoteIssue, kind string) domain.Issue {
	status := issueStateToStatus(ri.State)
	return domain.Issue{
		ID:           forgeIssueID(repo.ID, ri.Number),
		ProjectID:    repo.ProjectID,
		RepositoryID: repo.ID,
		BaseBranch:   repo.DefaultBranch,
		Kind:         kind,
		Title:        ri.Title,
		Description:  ri.Body,
		Priority:     labelPriority(ri.Labels),
		Status:       status,
		CloseSource:  closeSourceForStatus(status),
		Source:       Source,
		ExternalRef: &domain.ExternalRef{
			Source:        Source,
			ExternalID:    fmt.Sprintf("%d", ri.ID),
			ExternalIID:   ri.Number,
			ExternalURL:   issueWebURL(repo, ri),
			ExternalState: ri.State,
			ExternalType:  "issue",
			SyncedAt:      time.Now().UTC(),
		},
	}
}

func issueWebURL(repo domain.Repository, ri RemoteIssue) string {
	if strings.TrimSpace(ri.URL) != "" {
		return ri.URL
	}
	web := strings.TrimSuffix(strings.TrimSpace(repo.WebURL), "/")
	if web == "" {
		web = "https://" + domain.ForgeRepoHost + "/" + repo.RepoPath
	}
	return fmt.Sprintf("%s/issues/%d", web, ri.Number)
}

// CreateForgeIssue pushes a local issue to the forge as a new issue.
func (s *Service) CreateForgeIssue(store IssueStoreAdapter, issue domain.Issue) (domain.Issue, error) {
	started := time.Now().UTC()
	repo, ok := store.Repository(issue.RepositoryID)
	if !ok || !isForgeRepository(repo) {
		return issue, fmt.Errorf("issue must bind a GitHub repository")
	}
	provider, err := s.providerForProject(repo.ProjectID)
	if err != nil {
		return issue, err
	}
	in := IssueInput{Title: issue.Title, Body: issue.Description}
	if issue.Priority != "" {
		in.Labels = []string{"priority: " + issue.Priority}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ri, err := provider.CreateIssue(ctx, repo.RepoPath, in)
	if err != nil {
		s.addLog(domain.SyncLog{Direction: "push", Action: "create_issue", Status: "failed", Message: err.Error(), ProjectID: issue.ProjectID, StartedAt: started})
		return issue, err
	}
	issue.Source = Source
	issue.ExternalRef = &domain.ExternalRef{
		Source:        Source,
		ExternalID:    fmt.Sprintf("%d", ri.ID),
		ExternalIID:   ri.Number,
		ExternalURL:   issueWebURL(repo, ri),
		ExternalState: ri.State,
		ExternalType:  "issue",
		SyncedAt:      time.Now().UTC(),
		Imported:      true,
	}
	if ri.Number > 0 {
		issue.Number = ri.Number
		stableID := forgeIssueID(repo.ID, ri.Number)
		oldID := issue.ID
		issue.ID = stableID
		if oldID != "" && oldID != stableID && !strings.HasPrefix(oldID, "gh-") {
			_ = store.DeleteIssue(oldID)
		}
	}
	_ = store.PutIssue(issue)
	s.addLog(domain.SyncLog{Direction: "push", Action: "create_issue", Status: "success", Count: 1,
		Message: fmt.Sprintf("created %s#%d: %s", repo.RepoPath, ri.Number, ri.Title), ProjectID: issue.ProjectID, StartedAt: started})
	return issue, nil
}

// WriteBackClarification appends a clarification summary to the linked forge
// issue as a comment, so later pulls cannot erase it.
func (s *Service) WriteBackClarification(store IssueStoreAdapter, issue domain.Issue, body string) error {
	body = strings.TrimSpace(body)
	if body == "" {
		return fmt.Errorf("clarification summary is empty")
	}
	if issue.ExternalRef == nil || !strings.EqualFold(strings.TrimSpace(issue.Source), Source) {
		return nil
	}
	repoPath, number := issueRemoteRef(store, issue)
	if repoPath == "" || number <= 0 {
		return fmt.Errorf("issue is not linked to a GitHub issue")
	}
	provider, err := s.providerForWriteBack(issue.ProjectID, issue)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return provider.CommentIssue(ctx, repoPath, number, body)
}

// issueRemoteRef resolves the forge repository path and issue number.
func issueRemoteRef(store IssueStoreAdapter, issue domain.Issue) (string, int) {
	if issue.ExternalRef != nil {
		if ref, ok := ParseIssueURL(issue.ExternalRef.ExternalURL); ok {
			return ref.Repo, ref.Number
		}
	}
	number := 0
	if issue.ExternalRef != nil {
		number = issue.ExternalRef.ExternalIID
	}
	if repo, ok := store.Repository(issue.RepositoryID); ok && isForgeRepository(repo) {
		return repo.RepoPath, number
	}
	return "", number
}

// ImportIssueURL imports one forge issue by URL as a requirement of the
// project. The issue repository must be bound to the project.
func (s *Service) ImportIssueURL(store IssueStoreAdapter, projectID, rawURL string, fallbackUserID int64) (domain.Issue, error) {
	started := time.Now().UTC()
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return domain.Issue{}, fmt.Errorf("projectId is required")
	}
	if _, ok := store.Project(projectID); !ok {
		return domain.Issue{}, fmt.Errorf("project %s not found", projectID)
	}
	ref, ok := ParseIssueURL(rawURL)
	if !ok {
		return domain.Issue{}, fmt.Errorf("not a GitHub issue URL: %s", strings.TrimSpace(rawURL))
	}
	repo, ok := projectRepositoryByPath(store, projectID, ref.Repo)
	if !ok {
		return domain.Issue{}, fmt.Errorf("repository %s is not bound to project %s", ref.Repo, projectID)
	}
	issue, err := s.refreshIssue(store, repo, ref.Number, "requirement", fallbackUserID, true)
	if err != nil {
		s.addLog(domain.SyncLog{Direction: "pull", Action: "import_issue", Status: "failed", Message: err.Error(), ProjectID: projectID, StartedAt: started})
		return domain.Issue{}, err
	}
	s.addLog(domain.SyncLog{Direction: "pull", Action: "import_issue", Status: "success", Count: 1,
		Message: fmt.Sprintf("imported %s#%d: %s", ref.Repo, ref.Number, issue.Title), ProjectID: projectID, StartedAt: started})
	return issue, nil
}

// ResyncIssue re-pulls the linked forge issue and refreshes title,
// description, status and priority. Local assignee, clarification and
// repository bindings are preserved; a manual resync never re-enters the
// clarification gate.
func (s *Service) ResyncIssue(store IssueStoreAdapter, issue domain.Issue) (domain.Issue, error) {
	started := time.Now().UTC()
	if !strings.EqualFold(issue.Source, Source) || issue.ExternalRef == nil {
		return domain.Issue{}, fmt.Errorf("only GitHub-linked issues can be resynced")
	}
	repoPath, number := issueRemoteRef(store, issue)
	repo, ok := projectRepositoryByPath(store, issue.ProjectID, repoPath)
	if !ok || number <= 0 {
		return domain.Issue{}, fmt.Errorf("cannot resolve the GitHub issue of %s", issue.ID)
	}
	kind := issue.Kind
	if kind == "" {
		kind = "issue"
	}
	updated, err := s.refreshIssue(store, repo, number, kind, 0, false)
	if err != nil {
		s.addLog(domain.SyncLog{Direction: "pull", Action: "resync", Status: "failed", Message: err.Error(), ProjectID: issue.ProjectID, StartedAt: started})
		return domain.Issue{}, err
	}
	s.addLog(domain.SyncLog{Direction: "pull", Action: "resync", Status: "success", Count: 1,
		Message: fmt.Sprintf("resynced %s#%d", repoPath, number), ProjectID: issue.ProjectID, StartedAt: started})
	return updated, nil
}

func (s *Service) refreshIssue(store IssueStoreAdapter, repo domain.Repository, number int, kind string, fallbackUserID int64, imported bool) (domain.Issue, error) {
	provider, err := s.providerForProject(repo.ProjectID)
	if err != nil {
		return domain.Issue{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ri, err := provider.GetIssue(ctx, repo.RepoPath, number)
	if err != nil {
		return domain.Issue{}, err
	}
	issue := remoteToIssue(repo, ri, kind)
	issue.ExternalRef.Imported = imported
	existing, found := findForgeIssue(store, repo, ri.Number)
	if found {
		applyExistingIssue(&issue, existing)
	} else {
		issue.Number = ri.Number
		issue.CreatedAt = ri.CreatedAt
	}
	if issue.AssigneeUserID == 0 {
		issue.AssigneeUserID = resolveAssignee(store, ri.Assignee, fallbackUserID)
	}
	if imported {
		scoreStoredRequirement(store, &issue)
	}
	issue.UpdatedAt = time.Now().UTC()
	if err := store.PutIssue(issue); err != nil {
		return domain.Issue{}, err
	}
	if stored, ok := store.Issue(issue.ID); ok {
		issue = stored
	}
	if imported && !found {
		s.notifyRequirementImported(issue)
	}
	return issue, nil
}

func projectRepositoryByPath(store IssueStoreAdapter, projectID, repoPath string) (domain.Repository, bool) {
	repoPath = strings.Trim(strings.TrimSpace(repoPath), "/")
	if repoPath == "" {
		return domain.Repository{}, false
	}
	for _, repo := range store.Repositories() {
		if repo.ProjectID == projectID && strings.EqualFold(strings.TrimSpace(repo.RepoPath), repoPath) {
			return repo, true
		}
	}
	return domain.Repository{}, false
}

func resolveAssignee(store IssueStoreAdapter, login string, fallbackUserID int64) int64 {
	login = strings.TrimSpace(login)
	if dir, ok := store.(userDirectory); ok && login != "" {
		if u, ok := dir.UserByLoginName(login); ok {
			return u.ID
		}
	}
	return fallbackUserID
}

func (s *Service) notifyRequirementImported(issue domain.Issue) {
	if s == nil || s.OnRequirementImported == nil {
		return
	}
	if !domain.ShouldAutoStartClarification(issue) {
		return
	}
	s.OnRequirementImported(issue)
}

func scoreStoredRequirement(store IssueStoreAdapter, issue *domain.Issue) {
	if issue == nil || !strings.EqualFold(issue.Kind, "requirement") {
		return
	}
	policy := domain.DefaultClarificationPolicy()
	if p, ok := store.Project(issue.ProjectID); ok {
		policy = domain.EffectiveClarificationPolicy(p.Clarification)
	}
	issue.Clarification = domain.ScoreRequirementIssue(*issue, policy)
}

func issueStateToStatus(state string) string {
	switch state {
	case "opened":
		return "todo"
	case "closed":
		return "closed"
	default:
		return "backlog"
	}
}

// labelPriority maps common GitHub priority labels ("P0", "priority: high",
// "priority/critical") onto local priorities.
func labelPriority(labels []string) string {
	for _, raw := range labels {
		l := strings.ToLower(strings.TrimSpace(raw))
		l = strings.TrimPrefix(l, "priority")
		l = strings.TrimLeft(l, ":/- ")
		switch l {
		case "p0", "critical", "urgent", "blocker":
			return "urgent"
		case "p1", "high":
			return "high"
		case "p2", "medium", "normal":
			return "medium"
		case "p3", "p4", "low", "minor":
			return "low"
		}
	}
	return "medium"
}

func closeSourceForStatus(status string) string {
	if status == "closed" {
		return "external"
	}
	return ""
}
