package domain

import (
	"strings"
	"testing"
)

func TestIssueIdentityBlockGitHub(t *testing.T) {
	issue := Issue{
		ID:     "gh-repo-1-42",
		Source: "github",
		ExternalRef: &ExternalRef{
			Source:       "github",
			ExternalID:   "1987654321",
			ExternalIID:  42,
			ExternalType: "issue",
			ExternalURL:  "https://github.com/acme/api/issues/42",
		},
	}
	got := issue.IdentityBlock()
	for _, want := range []string{
		"需求身份（平台权威，禁止忽略）",
		"平台工作项：gh-repo-1-42",
		"来源：GitHub",
		"类型：issue",
		"https://github.com/acme/api/issues/42",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("identity block missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "回写") {
		t.Fatalf("identity block must not instruct write-back:\n%s", got)
	}
}

func TestIssueIdentityBlockLocal(t *testing.T) {
	got := Issue{ID: "iss-local"}.IdentityBlock()
	if !strings.Contains(got, "来源：本地") || !strings.Contains(got, "外部 ID：无") || strings.Contains(got, "回写") {
		t.Fatalf("local identity:\n%s", got)
	}
}
