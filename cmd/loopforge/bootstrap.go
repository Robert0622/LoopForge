package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Tencent/LoopForge/internal/auth"
	"github.com/Tencent/LoopForge/internal/httpapi"
	"github.com/Tencent/LoopForge/internal/repo"
)

// resolveDataDir returns the directory for the SQLite database and the
// persisted session secret: --data-dir, LOOPFORGE_DATA_DIR, ~/.loopforge.
func resolveDataDir(override string) (string, error) {
	for _, candidate := range []string{override, os.Getenv("LOOPFORGE_DATA_DIR")} {
		if dir := strings.TrimSpace(candidate); dir != "" {
			return dir, nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("resolve data dir: set LOOPFORGE_DATA_DIR")
	}
	return filepath.Join(home, ".loopforge"), nil
}

// resolveDatabase picks the database URL: --database, LOOPFORGE_DATABASE_URL,
// LOOPFORGE_MYSQL_DSN, then SQLite inside the data dir.
func resolveDatabase(override, dataDir string) string {
	for _, candidate := range []string{override, os.Getenv("LOOPFORGE_DATABASE_URL"), os.Getenv("LOOPFORGE_MYSQL_DSN")} {
		if dsn := strings.TrimSpace(candidate); dsn != "" {
			return dsn
		}
	}
	return "sqlite://" + filepath.Join(dataDir, "loopforge.db")
}

// ensureSessionSecret persists a random session secret in the data dir when
// LOOPFORGE_SESSION_SECRET is unset, so restarts keep users signed in.
// Multi-instance deployments must set the variable explicitly.
func ensureSessionSecret(dataDir string) {
	if strings.TrimSpace(os.Getenv("LOOPFORGE_SESSION_SECRET")) != "" {
		return
	}
	path := filepath.Join(dataDir, "session.secret")
	if raw, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(raw))) >= 32 {
		_ = os.Setenv("LOOPFORGE_SESSION_SECRET", strings.TrimSpace(string(raw)))
		return
	}
	secret := auth.RandomSecret()
	if err := os.MkdirAll(dataDir, 0o700); err == nil {
		if err := os.WriteFile(path, []byte(secret+"\n"), 0o600); err != nil {
			log.Printf("[auth] could not persist session secret: %v", err)
		}
	}
	_ = os.Setenv("LOOPFORGE_SESSION_SECRET", secret)
}

// ensureAdmin guarantees a password account exists. LOOPFORGE_ADMIN_PASSWORD,
// when set, is applied to LOOPFORGE_ADMIN_USER (default "admin") on every
// start. Otherwise, if no password account exists yet, "admin" is created
// with a random password printed once to the log.
func ensureAdmin(s *repo.Store) (int64, error) {
	login := strings.TrimSpace(os.Getenv("LOOPFORGE_ADMIN_USER"))
	if login == "" {
		login = "admin"
	}
	password := os.Getenv("LOOPFORGE_ADMIN_PASSWORD")
	if password == "" {
		var fallback int64
		for _, u := range s.Users() {
			if stored, ok := s.User(u.ID); ok && stored.PasswordHash != "" {
				if stored.PlatformRole == "admin" {
					return stored.ID, nil
				}
				if fallback == 0 {
					fallback = stored.ID
				}
			}
		}
		if fallback != 0 {
			return fallback, nil
		}
	}
	generated := password == ""
	demoMode, _ := strconv.ParseBool(strings.TrimSpace(os.Getenv("LOOPFORGE_DEMO_MODE")))
	if generated {
		if demoMode {
			// Match Grasp's fixed local-demo login so README and teammates
			// can sign in without scraping compose logs.
			password = "demo1234"
		} else {
			password = auth.RandomSecret()[:20]
		}
	}
	user, err := httpapi.EnsureLocalUser(s, login, "Administrator", password, true)
	if err != nil {
		return 0, fmt.Errorf("bootstrap admin: %w", err)
	}
	if generated {
		log.Printf("============================================================")
		log.Printf(" Created admin account  login: %s  password: %s", login, password)
		if demoMode {
			log.Printf(" Demo mode: this password is fixed. Change it before any shared deploy.")
		} else {
			log.Printf(" Change it after signing in (Settings -> Password).")
		}
		log.Printf("============================================================")
	} else {
		log.Printf("admin account %q is ready", login)
	}
	return user.ID, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}
