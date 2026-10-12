package engine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tencent/LoopForge/internal/domain"
	"github.com/Tencent/LoopForge/internal/gitwork"
	"github.com/Tencent/LoopForge/internal/repo"
)

func TestGitCommandDirPrefersExistingWorkspace(t *testing.T) {
	root := t.TempDir()
	ws := filepath.Join(root, "persist-run")
	if err := os.Mkdir(ws, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := gitCommandDir(ws); got != ws {
		t.Fatalf("gitCommandDir existing = %q, want %q", got, ws)
	}
	missing := filepath.Join(root, "gone")
	if got := gitCommandDir(missing); got != root {
		t.Fatalf("gitCommandDir missing = %q, want parent %q", got, root)
	}
}

func TestWithEnvValueReplacesPWD(t *testing.T) {
	got := withEnvValue([]string{"PATH=/bin", "PWD=/old", "LANG=C"}, "PWD", "/ws")
	if strings.Join(got, ",") != "PATH=/bin,PWD=/ws,LANG=C" {
		t.Fatalf("withEnvValue = %v", got)
	}
}

func TestRewriteRemoteForGitToken(t *testing.T) {
	token := "tok_secret"
	got := gitwork.RewriteRemoteForToken("git@github.com:acme/widget.git", token)
	if got != "https://private:tok_secret@github.com/acme/widget.git" {
		t.Fatalf("ssh rewrite = %q", got)
	}
	got = gitwork.RewriteRemoteForToken("https://github.com/acme/widget.git", token)
	if got != "https://private:tok_secret@github.com/acme/widget.git" {
		t.Fatalf("https rewrite = %q", got)
	}
	got = gitwork.RewriteRemoteForToken("https://oauth2:already@github.com/acme/widget.git", token)
	if got != "https://private:tok_secret@github.com/acme/widget.git" {
		t.Fatalf("oauth2 user should be rewritten to private, got %q", got)
	}
	got = gitwork.RewriteRemoteForToken("https://private:already@github.com/acme/widget.git", token)
	if got != "https://private:already@github.com/acme/widget.git" {
		t.Fatalf("existing private user rewrite = %q", got)
	}
	if gitwork.RewriteRemoteForToken("/tmp/local-repo", token) != "/tmp/local-repo" {
		t.Fatalf("local path should stay unchanged")
	}
	if gitwork.RewriteRemoteForToken("git@github.com:acme/widget.git", "") != "git@github.com:acme/widget.git" {
		t.Fatalf("empty token should keep original remote")
	}
}

func TestRedactGitOutput(t *testing.T) {
	t.Setenv(gitTokenEnv, "tok_env")
	got := gitwork.Redact("fatal: https://private:tok_repo@github.com/x.git tok_env", "tok_repo")
	if strings.Contains(got, "tok_repo") || strings.Contains(got, "tok_env") {
		t.Fatalf("token leaked in git output: %q", got)
	}
	if !strings.Contains(got, "***") {
		t.Fatalf("expected redaction, got %q", got)
	}
}

func TestGitEnvAddsInsteadOfWhenTokenSet(t *testing.T) {
	t.Setenv(gitTokenEnv, "")
	env := gitEnv("tok_secret", "")
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "GIT_CONFIG_COUNT=") {
		t.Fatal("expected GIT_CONFIG_COUNT")
	}
	if !strings.Contains(joined, "git@github.com:") {
		t.Fatal("expected ssh insteadOf")
	}
	if !strings.Contains(joined, "https://private:tok_secret@github.com/") {
		t.Fatal("expected private user insteadOf")
	}
}

func TestPrepareClarifyWorkspaceRequiresGitToken(t *testing.T) {
	_, err := PrepareClarifyWorkspace(context.Background(), t.TempDir(), domain.Repository{
		ID: "repo", CloneURL: "https://github.com/a/b.git",
	}, "")
	if err == nil || !strings.Contains(err.Error(), "项目责任人未配置 GitHub Git Token") {
		t.Fatalf("empty token error = %v", err)
	}
}

func TestResolveGitTokenUsesResponsibleUserOnly(t *testing.T) {
	t.Setenv(gitTokenEnv, "tok_env")
	s, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.PutUser(domain.User{ID: 22, LoginName: "responsible", GitToken: "tok_user"}); err != nil {
		t.Fatal(err)
	}
	got := resolveGitToken(s, 22)
	if got != "tok_user" {
		t.Fatalf("resolveGitToken = %q, want tok_user", got)
	}
	if got := resolveGitToken(s, 999); got != "" {
		t.Fatalf("missing user must not fall back to host env, got %q", got)
	}
}

func TestResolveGitIdentityUsesResponsibleUserLogin(t *testing.T) {
	s, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.PutUser(domain.User{ID: 22, LoginName: " devuser ", DisplayName: "王有为"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOOPFORGE_GIT_EMAIL_DOMAIN", "example.com")
	got := resolveGitIdentity(s, 22)
	if got.name != "devuser" || got.email != "devuser@example.com" {
		t.Fatalf("resolveGitIdentity = %q <%s>", got.name, got.email)
	}
	if got := resolveGitIdentity(s, 999); got != (gitIdentity{}) {
		t.Fatalf("missing user identity = %+v", got)
	}
}

func TestProjectAndRepositoryTokensAreIgnored(t *testing.T) {
	t.Setenv(gitTokenEnv, "tok_env")
	s, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	repo := domain.Repository{ID: "repo-token", ProjectID: "platform", Name: "widget", RepoPath: "g/widget", GitToken: "tok_repo"}
	if err := s.PutRepository(repo); err != nil {
		t.Fatal(err)
	}
	if got := resolveGitToken(s, 1); got != "" {
		t.Fatalf("project/repository/env tokens must be ignored, got %q", got)
	}
}

func TestGitTokenFromContextOverridesEnv(t *testing.T) {
	t.Setenv(gitTokenEnv, "tok_env")
	ctx := withGitToken(context.Background(), "tok_repo")
	if got := gitTokenFrom(ctx); got != "tok_repo" {
		t.Fatalf("gitTokenFrom = %q, want tok_repo", got)
	}
	if os.Getenv(gitTokenEnv) != "tok_env" {
		t.Fatal("env token should remain set")
	}
}

func TestGitAuthConfigArgs(t *testing.T) {
	if gitAuthConfigArgs("") != nil {
		t.Fatal("empty token should not add -c")
	}
	args := gitAuthConfigArgs("tok_secret")
	joined := strings.Join(args, "\n")
	if !strings.Contains(joined, "credential.helper=") {
		t.Fatalf("missing credential.helper: %q", args)
	}
	if !strings.Contains(joined, "url.https://private:tok_secret@github.com/.insteadof=git@github.com:") {
		t.Fatalf("missing ssh insteadOf: %q", args)
	}
}

func TestGitCIncludesOutputOnFailure(t *testing.T) {
	_, err := gitC(context.Background(), t.TempDir(), "status")
	if err == nil {
		t.Fatal("expected git error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "exit status") {
		t.Fatalf("missing exit status: %v", err)
	}
	if !strings.Contains(msg, "fatal") && !strings.Contains(msg, "not a git repository") {
		t.Fatalf("git stderr discarded: %v", err)
	}
}

func TestApplyWorkspaceGitAuthRemovesPersistedCredentials(t *testing.T) {
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", "-b", "master", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	gitDir := filepath.Join(dir, ".git")
	for _, name := range []string{gitAuthIncludeFile, gitAskPassFile, gitAskPassSecretFile} {
		if err := os.WriteFile(filepath.Join(gitDir, name), []byte("tok_secret"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := exec.Command("git", "-C", dir, "config", "--local", "include.path", gitAuthIncludeFile).CombinedOutput(); err != nil {
		t.Fatalf("configure legacy include: %v: %s", err, out)
	}
	ctx := withGitToken(context.Background(), "tok_secret")
	if err := applyWorkspaceGitAuth(ctx, dir); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", dir, "config", "--local", "--get", "include.path").CombinedOutput(); err == nil {
		t.Fatalf("legacy include survived: %s", out)
	}
	for _, name := range []string{gitAuthIncludeFile, gitAskPassFile, gitAskPassSecretFile} {
		if _, err := os.Stat(filepath.Join(gitDir, name)); !os.IsNotExist(err) {
			t.Fatalf("credential file %s survived: %v", name, err)
		}
	}
}
