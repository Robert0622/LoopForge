package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVerifyGitHubToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("Authorization") {
		case "Bearer classic-full":
			w.Header().Set("X-OAuth-Scopes", "repo, workflow")
		case "Bearer classic-read":
			w.Header().Set("X-OAuth-Scopes", "read:user")
		case "Bearer fine-grained":
		default:
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"login":"octocat"}`))
	}))
	t.Cleanup(srv.Close)
	ctx := context.Background()
	if got := verifyGitHubToken(ctx, srv.URL, "classic-full"); !got.Valid || got.Login != "octocat" || len(got.Scopes) != 2 {
		t.Fatalf("classic full = %+v", got)
	}
	if got := verifyGitHubToken(ctx, srv.URL, "classic-read"); got.Valid || len(got.Missing) != 1 || got.Missing[0] != "repo" {
		t.Fatalf("classic read = %+v", got)
	}
	if got := verifyGitHubToken(ctx, srv.URL, "fine-grained"); !got.Valid || len(got.Scopes) != 0 {
		t.Fatalf("fine-grained = %+v", got)
	}
	if got := verifyGitHubToken(ctx, srv.URL, "bad"); got.Valid || got.Error == "" {
		t.Fatalf("bad token = %+v", got)
	}
}
