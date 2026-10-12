// Package repo is the gorm-backed persistence layer of LoopForge. SQLite (the
// single-node default) and MySQL (multi-instance) share the same gorm code
// paths; tests use an in-memory SQLite database.
package repo

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Tencent/LoopForge/internal/domain"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

var (
	ErrRunLeaseHeld      = errors.New("run lease is held by another instance")
	ErrRunLeaseLost      = errors.New("run lease ownership or generation changed")
	ErrRunWorkspaceBound = errors.New("run workspace is already bound to another host path")
)

// deps 是所有领域 Repo 共享的依赖（repo 层的共享依赖只有 gorm 句柄）。
type deps struct {
	DB *gorm.DB
}

func (d *deps) isMySQL() bool {
	return d.DB != nil && d.DB.Dialector.Name() == "mysql"
}

// sqlJSONText returns a portable "extract text at JSON path" expression.
// MySQL needs the JSON_UNQUOTE wrapper; sqlite's json_extract already returns
// unquoted text for string values.
func (d *deps) sqlJSONText(path string) string {
	if d.isMySQL() {
		return "JSON_UNQUOTE(JSON_EXTRACT(payload, ?))"
	}
	return "json_extract(payload, ?)"
}

// locking returns the query with a row-lock clause on MySQL. sqlite serializes
// writers already (single connection in tests), so no clause is needed there.
func (d *deps) locking(q *gorm.DB) *gorm.DB {
	if d.isMySQL() {
		return q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	return q
}

// CatalogRepo owns the reusable definitions: agents, rules, skills, MCP
// servers, executors, dockerfiles, environments, sandboxes, workflows,
// repositories, sync config, schedule jobs and definition revisions.
type CatalogRepo struct {
	*deps
	projects *ProjectsRepo
}

// ProjectsRepo owns projects and their members.
type BusinessDomainsRepo struct {
	*deps
}

type ProjectsRepo struct {
	*deps
}

// IssuesRepo owns issues and issue comments.
type IssuesRepo struct {
	*deps
	mu       sync.Mutex
	projects *ProjectsRepo
	users    *UsersRepo
	tasks    *TasksRepo
}

// TasksRepo owns tasks (the cross-instance idempotency records).
type TasksRepo struct {
	*deps
}

// UsersRepo owns users and their saved credentials.
type UsersRepo struct {
	*deps
}

// TokensRepo owns API tokens.
type TokensRepo struct {
	*deps
}

// AssistantRepo owns Studio AI assistant conversations and scheduled prompts.
type AssistantRepo struct {
	*deps
}

// RunsRepo owns runs, their children (nodes/events/telemetry/logs/artifacts),
// run leases and workflow metrics.
type RunsRepo struct {
	*deps
	issues  *IssuesRepo
	catalog *CatalogRepo
}

// RunnersRepo owns the CLI runner registry, runner tasks and runner events.
type RunnersRepo struct {
	*deps
}

// DeliveryRepo owns PR delivery state and the delivery locking that follows.
type DeliveryRepo struct {
	*deps
	issues  *IssuesRepo
	tasks   *TasksRepo
	catalog *CatalogRepo
	runs    *RunsRepo
}

// AnalyticsRepo owns the read-only operational analytics view.
type AnalyticsRepo struct {
	*deps
	runs     *RunsRepo
	catalog  *CatalogRepo
	delivery *DeliveryRepo
	issues   *IssuesRepo
	tasks    *TasksRepo
	projects *ProjectsRepo
	insights *InsightsRepo
}

// Store 组合各领域 Repo。领域 Repo 以匿名指针嵌入（未导出类型）：包外仅能
// 通过提升的方法调用，与现有调用形态完全兼容（方法提升对未导出类型同样生效）。
type Store struct {
	*deps
	*CatalogRepo
	*ProjectsRepo
	*BusinessDomainsRepo
	*IssuesRepo
	*TasksRepo
	*UsersRepo
	*TokensRepo
	*AssistantRepo
	*RunsRepo
	*RunnersRepo
	*DeliveryRepo
	*AnalyticsRepo
	*InsightsRepo
	*PreviewsRepo
	*EngineInstancesRepo
	*RoomsRepo
	*KnowledgeRepo
}

// newStore wires the shared deps and every cross-domain pointer. Domains have
// no cycles, so a single construction pass is enough.
func newStore(db *gorm.DB) *Store {
	d := &deps{DB: db}
	s := &Store{deps: d}
	s.ProjectsRepo = &ProjectsRepo{deps: d}
	s.BusinessDomainsRepo = &BusinessDomainsRepo{deps: d}
	s.UsersRepo = &UsersRepo{deps: d}
	s.TokensRepo = &TokensRepo{deps: d}
	s.AssistantRepo = &AssistantRepo{deps: d}
	s.TasksRepo = &TasksRepo{deps: d}
	s.CatalogRepo = &CatalogRepo{deps: d, projects: s.ProjectsRepo}
	s.IssuesRepo = &IssuesRepo{deps: d, projects: s.ProjectsRepo, users: s.UsersRepo, tasks: s.TasksRepo}
	s.RunsRepo = &RunsRepo{deps: d, issues: s.IssuesRepo, catalog: s.CatalogRepo}
	s.RunnersRepo = &RunnersRepo{deps: d}
	s.DeliveryRepo = &DeliveryRepo{deps: d, issues: s.IssuesRepo, tasks: s.TasksRepo, catalog: s.CatalogRepo, runs: s.RunsRepo}
	s.InsightsRepo = &InsightsRepo{deps: d}
	s.AnalyticsRepo = &AnalyticsRepo{deps: d, runs: s.RunsRepo, catalog: s.CatalogRepo, delivery: s.DeliveryRepo, issues: s.IssuesRepo, tasks: s.TasksRepo, projects: s.ProjectsRepo, insights: s.InsightsRepo}
	s.PreviewsRepo = &PreviewsRepo{deps: d}
	s.EngineInstancesRepo = &EngineInstancesRepo{deps: d}
	s.RoomsRepo = &RoomsRepo{deps: d}
	s.KnowledgeRepo = &KnowledgeRepo{deps: d}
	return s
}

// Open connects the store. dsn selects the driver:
//
//   - "sqlite://<path>" or a path ending in .db/.sqlite: embedded SQLite, the
//     zero-dependency default for single-node installs;
//   - "mysql://<go-sql-driver DSN>" or a bare go-sql-driver DSN
//     ("user:pass@tcp(host:3306)/db"): MySQL, for multi-instance deployments.
//
// An empty dsn falls back to LOOPFORGE_DATABASE_URL, then LOOPFORGE_MYSQL_DSN.
// Open never invents a location: the entrypoint resolves the default path.
func Open(dsn string) (*Store, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("LOOPFORGE_DATABASE_URL"))
	}
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("LOOPFORGE_MYSQL_DSN"))
	}
	if dsn == "" {
		return nil, errors.New("database is not configured: set LOOPFORGE_DATABASE_URL (sqlite://path or mysql://dsn)")
	}
	if path, ok := sqlitePath(dsn); ok {
		return openSQLite(path)
	}
	return openMySQL(strings.TrimPrefix(dsn, "mysql://"))
}

func sqlitePath(dsn string) (string, bool) {
	switch {
	case strings.HasPrefix(dsn, "sqlite://"):
		return strings.TrimPrefix(dsn, "sqlite://"), true
	case strings.HasPrefix(dsn, "sqlite:"):
		return strings.TrimPrefix(dsn, "sqlite:"), true
	case strings.HasSuffix(dsn, ".db"), strings.HasSuffix(dsn, ".sqlite"), strings.HasSuffix(dsn, ".sqlite3"):
		return dsn, !strings.Contains(dsn, "@tcp(")
	}
	return "", false
}

func openSQLite(path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("sqlite path is empty")
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("sqlite data dir: %w", err)
		}
	}
	dsn := "file:" + path + "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(0)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{TranslateError: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("sqlite open: %w", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		// SQLite serializes writers; one connection avoids SQLITE_BUSY between
		// a transaction and a sibling query issued outside it.
		sqlDB.SetMaxOpenConns(1)
	}
	s := newStore(db)
	if err := db.AutoMigrate(allModels()...); err != nil {
		s.Close()
		return nil, fmt.Errorf("sqlite migrate: %w", err)
	}
	return s, nil
}

func openMySQL(dsn string) (*Store, error) {
	if !strings.Contains(dsn, "charset=") {
		if strings.Contains(dsn, "?") {
			dsn += "&charset=utf8mb4&collation=utf8mb4_unicode_ci"
		} else {
			dsn += "?charset=utf8mb4&collation=utf8mb4_unicode_ci"
		}
	}
	if !strings.Contains(dsn, "parseTime=") {
		dsn += "&parseTime=true&loc=UTC"
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{TranslateError: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("mysql open: %w", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(32)
		sqlDB.SetMaxIdleConns(8)
		sqlDB.SetConnMaxLifetime(30 * time.Minute)
		if err := sqlDB.Ping(); err != nil {
			return nil, fmt.Errorf("mysql ping: %w", err)
		}
	}
	s := newStore(db)
	// Additive DDL only (missing tables/columns/indexes). Data migrations and
	// MySQL-only optimizations live in migrations/ and are applied manually.
	if err := autoMigrateIncremental(s.DB); err != nil {
		s.Close()
		return nil, fmt.Errorf("auto migrate: %w", err)
	}
	return s, nil
}

// Close releases the underlying connection pool. The gorm handle is kept so
// in-flight background goroutines fail gracefully ("sql: database is closed")
// instead of panicking on a nil handle.
func (s *Store) Close() {
	if s.DB == nil {
		return
	}
	if sqlDB, err := s.DB.DB(); err == nil {
		_ = sqlDB.Close()
	}
}

func (s *Store) StorageInfo() map[string]any {
	return map[string]any{
		"driver":   s.DB.Dialector.Name(),
		"database": true,
		"note":     "All state (projects, issues, SOPs, runs, assistant sessions, schedules) lives in this database.",
	}
}

func IssueBoundToTracker(issue domain.Issue) bool {
	if issue.Source != "github" {
		return false
	}
	if issue.ExternalRef == nil {
		return false
	}
	return strings.TrimSpace(issue.ExternalRef.ExternalID) != "" || issue.ExternalRef.ExternalIID != 0
}

// RestoreTrackerBinding fills Source/ExternalRef from a still-bound snapshot, or
// from a stable local id (gh-<repo>-<number>) after a save wiped the pointer.
func RestoreTrackerBinding(issue *domain.Issue, existing domain.Issue) {
	if issue == nil || IssueBoundToTracker(*issue) {
		return
	}
	if IssueBoundToTracker(existing) {
		issue.Source = existing.Source
		issue.ExternalRef = existing.ExternalRef
		if strings.TrimSpace(issue.ID) == "" {
			issue.ID = existing.ID
		}
		return
	}
	id := strings.TrimSpace(issue.ID)
	if id == "" {
		id = strings.TrimSpace(existing.ID)
		issue.ID = id
	}
	if _, iid := parseStoredForgeIssueID(id); iid > 0 {
		issue.Source = "github"
		if existing.ExternalRef != nil {
			issue.ExternalRef = existing.ExternalRef
			return
		}
		issue.ExternalRef = &domain.ExternalRef{Source: "github", ExternalIID: iid, ExternalType: "issue"}
	}
}

func parseStoredForgeIssueID(id string) (repo string, iid int) {
	body := strings.TrimPrefix(strings.TrimSpace(id), "gh-")
	if body == "" || body == strings.TrimSpace(id) {
		return "", 0
	}
	i := strings.LastIndex(body, "-")
	if i <= 0 {
		return "", 0
	}
	n := 0
	for _, ch := range body[i+1:] {
		if ch < '0' || ch > '9' {
			return "", 0
		}
		n = n*10 + int(ch-'0')
	}
	if n <= 0 {
		return "", 0
	}
	return body[:i], n
}

func cloneRunNodes(nodes map[string]*domain.NodeRun) map[string]*domain.NodeRun {
	if len(nodes) == 0 {
		return nil
	}
	out := make(map[string]*domain.NodeRun, len(nodes))
	for id, node := range nodes {
		if node == nil {
			continue
		}
		cp := *node
		out[id] = &cp
	}
	return out
}

func cloneRun(run domain.Run) domain.Run {
	run.Nodes = cloneRunNodes(run.Nodes)
	run.Events = append([]domain.RunEvent(nil), run.Events...)
	run.Telemetry = append([]domain.TelemetryEvent(nil), run.Telemetry...)
	run.Logs = append([]domain.LogRecord(nil), run.Logs...)
	run.Artifacts = append([]domain.Artifact(nil), run.Artifacts...)
	return run
}

func normalizeRefs(values []string, prefix string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimPrefix(v, prefix)
		v = strings.TrimSuffix(v, ".md")
		out = append(out, v)
	}
	return out
}

func sortSlice[T any](values []T, less func(a, b T) bool) {
	sort.Slice(values, func(i, j int) bool { return less(values[i], values[j]) })
}

func mustJSON(v any) []byte {
	raw, err := json.Marshal(v)
	if err != nil || len(raw) == 0 {
		return []byte("{}")
	}
	return raw
}
