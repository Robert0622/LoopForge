package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tencent/LoopForge/internal/gitwork"
	"github.com/Tencent/LoopForge/internal/repo"
)

const gitTokenEnv = gitwork.TokenEnv

// HTTPS Git hosts (GitHub included) accept a personal access token with any
// non-empty username; some reject the "oauth2" username, so use a neutral one.
const gitHTTPUser = "private"

type gitTokenCtxKey struct{}
type gitIdentityCtxKey struct{}

type gitIdentity struct {
	name  string
	email string
}

func withGitToken(ctx context.Context, token string) context.Context {
	return gitwork.WithToken(ctx, token)
}

func gitTokenFrom(ctx context.Context) string { return gitwork.TokenFrom(ctx) }

func withGitIdentity(ctx context.Context, name, email string) context.Context {
	identity := gitIdentity{name: strings.TrimSpace(name), email: strings.TrimSpace(email)}
	if identity.name == "" || identity.email == "" {
		return ctx
	}
	return context.WithValue(ctx, gitIdentityCtxKey{}, identity)
}

func gitIdentityFrom(ctx context.Context) gitIdentity {
	if ctx != nil {
		if identity, ok := ctx.Value(gitIdentityCtxKey{}).(gitIdentity); ok {
			return identity
		}
	}
	return gitIdentity{}
}

func resolveGitIdentity(s *repo.Store, userID int64) gitIdentity {
	if s == nil || userID <= 0 {
		return gitIdentity{}
	}
	user, ok := s.User(userID)
	if !ok {
		return gitIdentity{}
	}
	login := strings.TrimSpace(user.LoginName)
	if login == "" {
		return gitIdentity{}
	}
	return gitIdentity{name: login, email: login + "@" + gitEmailDomain()}
}

// gitEmailDomain is the domain appended to login names for commit authorship;
// LOOPFORGE_GIT_EMAIL_DOMAIN lets deployments map logins to their mail domain.
func gitEmailDomain() string {
	if domain := strings.Trim(strings.TrimSpace(os.Getenv("LOOPFORGE_GIT_EMAIL_DOMAIN")), "@"); domain != "" {
		return domain
	}
	return "users.noreply.loopforge.dev"
}

func applyWorkspaceGitIdentity(ctx context.Context, workspace string) error {
	identity := gitIdentityFrom(ctx)
	if identity.name == "" || identity.email == "" {
		return nil
	}
	for _, item := range []struct {
		key, value string
	}{
		{key: "user.name", value: identity.name},
		{key: "user.email", value: identity.email},
	} {
		if _, err := gitC(ctx, workspace, "config", "--local", item.key, item.value); err != nil {
			return fmt.Errorf("configure git %s: %w", item.key, err)
		}
	}
	return nil
}

func resolveGitToken(s *repo.Store, userID int64) string {
	if s != nil && userID > 0 {
		if user, ok := s.User(userID); ok {
			return strings.TrimSpace(user.GitToken)
		}
	}
	return ""
}

// forgeInsteadOfPrefixes are rewritten to HTTPS + private:token. Same list
// for command-line -c, GIT_CONFIG_COUNT, and the workspace include file so
// clone/fetch/push and CodeBuddy's own git all see one rewrite.
var forgeInsteadOfPrefixes = []string{
	"git@github.com:",
	"ssh://git@github.com/",
	"https://github.com/",
	"http://github.com/",
}

const gitAuthIncludeFile = "loopforge.auth"
const gitAskPassFile = "loopforge-askpass"
const gitAskPassSecretFile = "loopforge-askpass-secret"

func forgeAuthedBase(token string) string {
	return "https://" + gitHTTPUser + ":" + strings.TrimSpace(token) + "@github.com/"
}

func gitSandboxEnv(token, workspace string) map[string]string {
	extra := map[string]string{"GIT_TERMINAL_PROMPT": "0"}
	token = strings.TrimSpace(token)
	if token == "" {
		return extra
	}
	authed := forgeAuthedBase(token)
	extra["GIT_CONFIG_COUNT"] = fmt.Sprintf("%d", len(forgeInsteadOfPrefixes))
	for i, prefix := range forgeInsteadOfPrefixes {
		extra[fmt.Sprintf("GIT_CONFIG_KEY_%d", i)] = "url." + authed + ".insteadof"
		extra[fmt.Sprintf("GIT_CONFIG_VALUE_%d", i)] = prefix
	}
	return extra
}

func gitEnv(token, workspace string) []string {
	var env []string
	for _, kv := range os.Environ() {
		switch {
		case strings.HasPrefix(kv, "GIT_CONFIG_COUNT="),
			strings.HasPrefix(kv, "GIT_CONFIG_KEY_"),
			strings.HasPrefix(kv, "GIT_CONFIG_VALUE_"),
			strings.HasPrefix(kv, "GIT_ASKPASS="),
			strings.HasPrefix(kv, "SSH_ASKPASS="):
			continue
		}
		env = append(env, kv)
	}
	for k, v := range gitSandboxEnv(token, workspace) {
		env = append(env, k+"="+v)
	}
	return env
}

func gitCommandDir(workspace string) string {
	workspace = strings.TrimSpace(workspace)
	if workspace != "" {
		if st, err := os.Stat(workspace); err == nil && st.IsDir() {
			return workspace
		}
		parent := filepath.Dir(workspace)
		if st, err := os.Stat(parent); err == nil && st.IsDir() {
			return parent
		}
	}
	if wd, err := os.Getwd(); err == nil && wd != "" {
		return wd
	}
	return os.TempDir()
}

func withEnvValue(env []string, key, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	found := false
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			if !found {
				out = append(out, prefix+value)
				found = true
			}
			continue
		}
		out = append(out, kv)
	}
	if !found {
		out = append(out, prefix+value)
	}
	return out
}

func gitCredentialHelper(token string) string {
	token = strings.TrimSpace(token)
	return fmt.Sprintf(`!f() { test "$1" = get || exit 0; printf "username=%s\npassword=%s\n"; }; f`, gitHTTPUser, token)
}

func gitAuthConfigArgs(token string) []string {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	args := []string{"-c", "credential.helper=", "-c", "credential.helper=" + gitCredentialHelper(token)}
	authed := forgeAuthedBase(token)
	for _, prefix := range forgeInsteadOfPrefixes {
		args = append(args, "-c", "url."+authed+".insteadof="+prefix)
	}
	return args
}

// applyWorkspaceGitAuth removes credentials persisted by older LoopForge
// versions. Git authentication is now supplied only to the relevant child
// process, so user tokens never become part of the run workspace.
func applyWorkspaceGitAuth(ctx context.Context, workspace string) error {
	gitDir := filepath.Join(workspace, ".git")
	info, err := os.Stat(gitDir)
	if err != nil || !info.IsDir() {
		return nil
	}
	for _, name := range []string{gitAuthIncludeFile, gitAskPassFile, gitAskPassSecretFile} {
		if err := os.Remove(filepath.Join(gitDir, name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	configured, _ := gitC(ctx, workspace, "config", "--local", "--get-all", "include.path")
	_, _ = gitC(ctx, workspace, "config", "--local", "--unset-all", "include.path")
	for _, include := range strings.Split(configured, "\n") {
		include = strings.TrimSpace(include)
		if include == "" || include == gitAuthIncludeFile {
			continue
		}
		if _, err := gitC(ctx, workspace, "config", "--local", "--add", "include.path", include); err != nil {
			return err
		}
	}
	return nil
}
