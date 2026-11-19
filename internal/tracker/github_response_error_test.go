package tracker

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGitHubRequestExplainsUpstreamErrors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		headers map[string]string
		message string
		want    string
	}{
		{"primary limit", 403, map[string]string{"X-RateLimit-Remaining": "0", "X-RateLimit-Reset": "1700000000"}, "", "2023-11-14 22:13:20 UTC"},
		{"secondary limit with delay", 403, map[string]string{"Retry-After": "60"}, "", "60 seconds"},
		{"secondary limit in message", 403, nil, "You have exceeded a secondary rate limit", "rate limit"},
		{"too many requests", 429, nil, "", "rate limit"},
		{"invalid delay", 429, map[string]string{"Retry-After": "-1"}, "", "wait before retrying"},
		{"bad credentials", 401, nil, "", "credentials"},
		{"forbidden", 403, map[string]string{"X-RateLimit-Remaining": "13"}, "Resource not accessible", "permissions"},
		{"not found", 404, nil, "", "resource exists"},
		{"invalid input", 422, nil, "", "parameters"},
		{"service unavailable", 503, nil, "", "try again later"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const privateText = "private-diagnostic-text"
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for key, value := range tc.headers {
					w.Header().Set(key, value)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_ = json.NewEncoder(w).Encode(map[string]string{"message": tc.message + " " + privateText})
			}))
			t.Cleanup(srv.Close)
			client := githubClient{http: srv.Client(), base: srv.URL, path: "example/orders"}
			err := client.request(http.MethodGet, "/issues", nil, nil)
			if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("HTTP %d", tc.status)) ||
				!strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), privateText) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
