package domain

import (
	"testing"
	"time"
)

func TestDefaultWorkBranch(t *testing.T) {
	now := time.Date(2026, 8, 31, 15, 30, 0, 0, time.Local)

	tests := []struct {
		name  string
		issue Issue
		want  string
	}{
		{
			name: "forge issue uses external iid",
			issue: Issue{
				ID: "gh-repo-18", Source: "github", Kind: "issue", Number: 7,
				ExternalRef: &ExternalRef{ExternalIID: 18},
			},
			want: "feature/fix-issue-18-20260831-1530",
		},
		{
			name: "local requirement uses requirement prefix",
			issue: Issue{
				ID: "issue-1", ProjectID: "platform", Kind: "requirement", Number: 5,
			},
			want: "feature/requirement-5-20260831-1530",
		},
		{
			name: "unknown source with number keeps legacy fix-issue prefix",
			issue: Issue{
				ID: "custom-1", Source: "custom", Number: 3,
			},
			want: "feature/fix-issue-3-20260831-1530",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := DefaultWorkBranch(tc.issue, now); got != tc.want {
				t.Fatalf("DefaultWorkBranch() = %q, want %q", got, tc.want)
			}
		})
	}
}
