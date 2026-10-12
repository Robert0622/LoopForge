package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Tencent/LoopForge/internal/config"
	"github.com/Tencent/LoopForge/internal/domain"
	"github.com/Tencent/LoopForge/internal/engine"
	"github.com/Tencent/LoopForge/internal/httpapi"
	"github.com/Tencent/LoopForge/internal/managedcli"
	"github.com/Tencent/LoopForge/internal/preview"
	"github.com/Tencent/LoopForge/internal/repo"
	"github.com/Tencent/LoopForge/internal/runtime"
	"github.com/Tencent/LoopForge/internal/tracker"
	"github.com/gin-gonic/gin"
)

// loadDotEnv loads KEY=VALUE pairs from the first .env file found. Search
// order: the current working directory and up to three parents (so running
// from a subdirectory still finds the project root), then next to the
// executable. Existing environment variables take precedence and are never
// overwritten.
func loadDotEnv() {
	var candidates []string
	if cwd, err := os.Getwd(); err == nil {
		dir := cwd
		for i := 0; i < 4; i++ {
			candidates = append(candidates, filepath.Join(dir, ".env"))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), ".env"))
	}
	for _, p := range candidates {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			line = strings.TrimPrefix(line, "export ")
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			k = strings.TrimSpace(k)
			v = strings.Trim(strings.TrimSpace(v), `"'`)
			if k != "" && os.Getenv(k) == "" {
				os.Setenv(k, v)
			}
		}
		return
	}
}

func main() {
	if runtime.RunMCPCallCLI(os.Args[1:]) {
		return
	}
	addr := flag.String("addr", "", "HTTP listen address; overrides LOOPFORGE_HTTP_PORT")
	workspaces := flag.String("workspaces", "", "sandbox workspace root")
	workerConcurrency := flag.Int("worker-concurrency", 8, "maximum concurrent node sandboxes for this worker")
	basicUser := flag.String("basic-user", "", "optional HTTP Basic Auth username")
	basicPass := flag.String("basic-pass", "", "optional HTTP Basic Auth password")
	githubToken := flag.String("github-token", "", "fallback GitHub token for issue sync and PR delivery when the project owner has none (or set LOOPFORGE_GITHUB_TOKEN / GITHUB_TOKEN)")
	tlsCert := flag.String("tls-cert", "", "optional TLS certificate path")
	tlsKey := flag.String("tls-key", "", "optional TLS private key path")
	database := flag.String("database", "", "database URL: sqlite://path or mysql://dsn (default: SQLite in the data dir)")
	dataDir := flag.String("data-dir", "", "directory for the SQLite database and session secret (default ~/.loopforge)")
	roleFlag := flag.String("role", "", "process role: api, worker, or all (default all)")
	cliAction := flag.String("cli-action", "", "local managed CLI administration: status, update, or rollback; exits without starting services")
	cliProvider := flag.String("cli-provider", "", "provider for CLI rollback: codex, codebuddy, or claude-code")
	cliTimeout := flag.Duration("cli-timeout", 80*time.Minute, "total managed CLI operation timeout")
	prewarmSandbox := flag.Bool("prewarm-sandbox", false, "build the default Docker sandbox image used by built-in SOPs, then exit")
	selfTest := flag.Bool("self-test", false, "check a running instance end to end (health, login, doctor, demo run with share-link approval), then exit")
	selfTestURL := flag.String("self-test-url", "", "instance URL for --self-test (default http://127.0.0.1:$LOOPFORGE_HTTP_PORT)")
	selfTestUser := flag.String("self-test-user", "", "admin login for --self-test (default $LOOPFORGE_ADMIN_USER or admin)")
	selfTestPassword := flag.String("self-test-password", "", "admin password for --self-test (default $LOOPFORGE_ADMIN_PASSWORD, or the demo password in demo mode)")
	selfTestTimeout := flag.Duration("self-test-timeout", 3*time.Minute, "overall --self-test timeout")
	flag.Parse()
	loadDotEnv()
	if *selfTest {
		opts := selfTestOptions{baseURL: *selfTestURL, user: *selfTestUser, password: *selfTestPassword, timeout: *selfTestTimeout}
		if opts.baseURL == "" {
			opts.baseURL = defaultSelfTestURL()
		}
		if opts.user == "" {
			opts.user = strings.TrimSpace(os.Getenv("LOOPFORGE_ADMIN_USER"))
		}
		if opts.user == "" {
			opts.user = "admin"
		}
		if opts.password == "" {
			opts.password = defaultSelfTestPassword()
		}
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		err := runSelfTest(ctx, opts, os.Stdout)
		stop()
		if err != nil {
			fmt.Fprintf(os.Stderr, "自检失败：%v\n", err)
			os.Exit(1)
		}
		fmt.Println("自检通过")
		return
	}
	if *prewarmSandbox {
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		log.Printf("building the default sandbox image; this can take several minutes")
		image, err := runtime.PrewarmDockerImage(ctx, repo.GenericBackendDockerfile())
		stop()
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("sandbox image ready: %s\n", image)
		return
	}
	if *cliAction != "" || *cliProvider != "" {
		if flag.NArg() != 0 {
			log.Fatal("CLI administration does not accept positional arguments")
		}
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		err := runCLI(ctx, *cliAction, *cliProvider, *cliTimeout, os.Stdout)
		stop()
		if err != nil {
			log.Fatal(err)
		}
		return
	}
	role, err := resolveRole(*roleFlag)
	if err != nil {
		log.Fatal(err)
	}
	root, err := resolveWorkspaceRoot(*workspaces)
	if err != nil {
		log.Fatal(err)
	}
	dir, err := resolveDataDir(*dataDir)
	if err != nil {
		log.Fatal(err)
	}
	if strings.TrimSpace(os.Getenv("LOOPFORGE_AUTH_REQUIRED")) == "" {
		_ = os.Setenv("LOOPFORGE_AUTH_REQUIRED", "true")
	}
	ensureSessionSecret(dir)
	s, err := repo.Open(resolveDatabase(*database, dir))
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("storage driver=%v role=%s", s.StorageInfo()["driver"], role)
	if demo, _ := strconv.ParseBool(strings.TrimSpace(os.Getenv("LOOPFORGE_DEMO_MODE"))); demo {
		repo.SetDemoMode(true)
		log.Printf("demo mode: demo executor and the %q SOP are available", repo.DemoWorkflowID)
	}
	if role != "worker" {
		sandbox := strings.TrimSpace(os.Getenv("LOOPFORGE_TEMPLATE_SANDBOX"))
		if sandbox == "" {
			sandbox = runtime.DetectTemplateSandbox()
		}
		repo.SetTemplateSandbox(sandbox)
		log.Printf("built-in SOP templates use sandbox %q (override with LOOPFORGE_TEMPLATE_SANDBOX)", sandbox)
		warnMissingGateMCP()
		configureExtensions()
		ownerID, err := ensureAdmin(s)
		if err != nil {
			log.Fatal(err)
		}
		if err := repo.Bootstrap(s, ownerID); err != nil {
			log.Fatalf("bootstrap: %v", err)
		}
		if prewarm, _ := strconv.ParseBool(strings.TrimSpace(os.Getenv("LOOPFORGE_PREWARM_SANDBOX"))); prewarm && sandbox == repo.GenericDockerSandboxID {
			dockerfile, ok := s.Dockerfile(repo.GenericBackendDockerfileID)
			if !ok || strings.TrimSpace(dockerfile.Content) == "" {
				dockerfile = repo.GenericBackendDockerfile()
			}
			if !runtime.DockerImageCached(context.Background(), dockerfile) {
				runtime.StartDockerPrewarm(dockerfile, 45*time.Minute)
				log.Printf("building the default sandbox image in the background (LOOPFORGE_PREWARM_SANDBOX=true)")
			}
		}
	}
	e := engine.New(s, runtime.NewManagerWithLimit(root, *workerConcurrency))
	host, _ := os.Hostname()
	instanceID := strings.TrimSpace(os.Getenv("LOOPFORGE_INSTANCE_ID"))
	if instanceID == "" {
		instanceID = fmt.Sprintf("%s-%s-%d", role, host, os.Getpid())
	}
	leaseTTL := 30 * time.Second
	if raw := strings.TrimSpace(os.Getenv("LOOPFORGE_RUN_LEASE_TTL")); raw != "" {
		parsed, parseErr := time.ParseDuration(raw)
		if parseErr != nil || parsed <= 0 {
			log.Fatalf("invalid LOOPFORGE_RUN_LEASE_TTL %q", raw)
		}
		leaseTTL = parsed
	}
	executeRuns := role != "api"
	if role == "all" {
		if raw := strings.TrimSpace(os.Getenv("LOOPFORGE_EXECUTE_RUNS")); raw != "" {
			parsed, parseErr := strconv.ParseBool(raw)
			if parseErr != nil {
				log.Fatalf("invalid LOOPFORGE_EXECUTE_RUNS %q", raw)
			}
			executeRuns = parsed
		}
	}
	e.ConfigureExecution(instanceID, host, leaseTTL, executeRuns)
	e.ConfigureCloudRunnerPool(engine.CloudRunnerPool{
		Label:    os.Getenv("LOOPFORGE_CLOUD_RUNNER_LABEL"),
		Projects: engine.ParseCloudRunnerProjects(os.Getenv("LOOPFORGE_CLOUD_RUNNER_PROJECTS")),
		// LOOPFORGE_LOCAL_EXECUTION=false keeps this host from running nodes
		// of pool projects: they wait for, or move to, a pool runner.
		Required: strings.EqualFold(strings.TrimSpace(os.Getenv("LOOPFORGE_LOCAL_EXECUTION")), "false"),
	})
	attachPreviewStore(e, s)
	log.Printf("engine instance=%s role=%s execute_runs=%t run_lease_ttl=%s", instanceID, role, executeRuns, leaseTTL)
	claimCtx, stopClaim := context.WithCancel(context.Background())
	defer stopClaim()
	e.StartMaintenance(claimCtx)
	if executeRuns {
		e.StartClaimLoop(claimCtx)
	}

	endpoints, err := config.LoadEndpoints(role, strings.TrimSpace(*addr))
	if err != nil {
		log.Fatal(err)
	}
	config.SetCurrent(endpoints)
	e.ConfigurePlatformMCP(endpoints.PlatformBase, endpoints.PlatformMCPToken)
	if endpoints.PlatformBase == "" {
		log.Printf("platform MCP: no sandbox-reachable platform URL; cloud runs skip it (set LOOPFORGE_PLATFORM_MCP_URL)")
	} else {
		log.Printf("platform MCP base=%s", endpoints.PlatformBase)
	}

	var srv *http.Server
	var syncSvc *tracker.Service
	switch role {
	case "worker":
		e.ConfigureControl(engine.ControlConfig{Role: role, URL: endpoints.ControlURL, Token: endpoints.ControlToken})
		e.StartInstanceHeartbeat(claimCtx)
		srv = &http.Server{Addr: endpoints.ListenAddr, Handler: workerHandler(e), ReadHeaderTimeout: 10 * time.Second}
	default:
		e.ConfigureControl(engine.ControlConfig{Token: endpoints.ControlToken, FallbackURL: endpoints.WorkerFallbackURL})
		listenAddr := endpoints.ListenAddr
		if host := strings.TrimSpace(os.Getenv("LOOPFORGE_GITHUB_HOST")); host != "" {
			domain.ForgeRepoHost = host
		}
		syncSvc = tracker.NewService()
		token := firstNonEmpty(*githubToken, os.Getenv("LOOPFORGE_GITHUB_TOKEN"), os.Getenv("GITHUB_TOKEN"))
		if token != "" {
			syncSvc.SetAuthToken(token)
		}
		gin.SetMode(gin.ReleaseMode)
		syncSvc.ConfigureWithStore(s.SyncConfig(), s)
		syncSvc.StartAutoSync(s)
		syncSvc.StartMRStateSync(s)
		srv = &http.Server{Addr: listenAddr, Handler: httpapi.New(s, e, syncSvc, *basicUser, *basicPass), ReadHeaderTimeout: 10 * time.Second}
	}

	stopCLIUpdates := startManagedCLIUpdates(claimCtx, role, executeRuns)
	defer stopCLIUpdates()
	stopped := make(chan struct{})
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		log.Printf("shutting down %s: stop claiming, drain local runs, then stop HTTP...", role)
		stopClaim()
		stopCLIUpdates() // Cancel updater subprocesses and join before draining.
		drainTimeout := 30 * time.Minute
		if role == "api" {
			drainTimeout = 15 * time.Second
		}
		if raw := strings.TrimSpace(os.Getenv("LOOPFORGE_DRAIN_TIMEOUT")); raw != "" {
			if parsed, parseErr := time.ParseDuration(raw); parseErr == nil && parsed > 0 {
				drainTimeout = parsed
			}
		}
		drainCtx, cancelDrain := context.WithTimeout(context.Background(), drainTimeout)
		defer cancelDrain()
		if err := e.Drain(drainCtx); err != nil {
			log.Printf("drain timeout with %d active run(s): %v", e.ActiveRunCount(), err)
		} else {
			log.Printf("all local runs drained")
		}
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelShutdown()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("http shutdown: %v", err)
		}
		if syncSvc != nil {
			syncSvc.StopMRStateSync()
			syncSvc.StopAutoSync()
		}
		s.Close()
		close(stopped)
	}()
	displayAddr := srv.Addr
	if strings.HasPrefix(displayAddr, ":") {
		displayAddr = "localhost" + displayAddr
	}
	serve := srv.ListenAndServe
	if role != "worker" && *tlsCert != "" && *tlsKey != "" {
		log.Printf("LoopForge %s listening on https://%s", role, displayAddr)
		serve = func() error { return srv.ListenAndServeTLS(*tlsCert, *tlsKey) }
	} else {
		log.Printf("LoopForge %s listening on http://%s", role, displayAddr)
	}
	if err := serve(); err != nil && err != http.ErrServerClosed {
		stopClaim()
		stopCLIUpdates() // log.Fatal bypasses deferred cleanup.
		log.Fatal(err)
	}
	<-stopped
}

func startManagedCLIUpdates(ctx context.Context, role string, executeRuns bool) func() {
	root := strings.TrimSpace(os.Getenv("LOOPFORGE_MANAGED_CLI_ROOT"))
	// Never infer a root or opt personal runners/API-only processes into updates.
	if !executeRuns || (role != "worker" && role != "all") || root == "" {
		return func() {}
	}
	return managedcli.Start(ctx, root)
}

func workerHandler(e *engine.Engine) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = fmt.Fprintf(w, `{"status":"ok","role":"worker","activeRuns":%d}`, e.ActiveRunCount())
	})
	mux.HandleFunc("POST /internal/runs/{id}/interrupt", e.HandleRunInterrupt)
	return mux
}

func resolveRole(override string) (string, error) {
	role := strings.ToLower(strings.TrimSpace(override))
	if role == "" {
		role = strings.ToLower(strings.TrimSpace(os.Getenv("LOOPFORGE_ROLE")))
	}
	if role == "" {
		role = "all"
	}
	switch role {
	case "api", "worker", "all":
		return role, nil
	default:
		return "", fmt.Errorf("invalid role %q (want api, worker, or all)", role)
	}
}

func resolveWorkspaceRoot(override string) (string, error) {
	if root := strings.TrimSpace(override); root != "" {
		return root, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve default sandbox workspace root: %w", err)
	}
	if strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("resolve default sandbox workspace root: user home is empty")
	}
	return filepath.Join(home, ".loopforge", "workspace"), nil
}

func attachPreviewStore(e *engine.Engine, s *repo.Store) {
	cfg := preview.LoadConfig()
	e.SetPreview(preview.NewService(s, preview.NewDBStore(s), cfg))
	if cfg.PublicHost != "" {
		log.Printf("preview gateway path=/preview/{token}/ public=%s://%s", cfg.PublicScheme, cfg.PublicHost)
	} else {
		log.Printf("preview gateway path=/preview/{token}/")
	}
}

// warnMissingGateMCP surfaces a broken install at startup instead of on the
// first real run: every Agent node with a gate needs the gate MCP helper for
// the host and, for Docker sandboxes, for linux.
func warnMissingGateMCP() {
	targets := []string{goruntime.GOOS}
	if goruntime.GOOS != "linux" {
		targets = append(targets, "linux")
	}
	for _, targetOS := range targets {
		if err := runtime.CheckGateMCPBinary(targetOS, goruntime.GOARCH); err != nil {
			log.Printf("WARNING: Agent runs with gates will fail: %v", err)
		}
	}
}
