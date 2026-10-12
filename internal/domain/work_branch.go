package domain

import (
	"fmt"
	"strings"
	"time"
)

// DefaultWorkBranch names the git work branch for an issue run. Forge issues
// use fix-issue-{number}; requirements (local or imported) use
// requirement-{number}. The suffix is a local timestamp that keeps branch names
// readable while separating dispatches started in different minutes.
func DefaultWorkBranch(issue Issue, now time.Time) string {
	suffix := now.In(time.Local).Format("20060102-1504")
	source := strings.ToLower(strings.TrimSpace(issue.Source))
	kind := strings.ToLower(strings.TrimSpace(issue.Kind))

	if (source == "github" && kind != "requirement") || kind == "issue" {
		if n := forgeIssueNumber(issue); n > 0 {
			return fmt.Sprintf("feature/fix-issue-%d-%s", n, suffix)
		}
	}
	if kind == "requirement" || source == "" || source == "local" {
		if issue.Number > 0 {
			return fmt.Sprintf("feature/requirement-%d-%s", issue.Number, suffix)
		}
	}
	if issue.Number > 0 {
		return fmt.Sprintf("feature/fix-issue-%d-%s", issue.Number, suffix)
	}
	if issue.ID != "" {
		return "feature/" + workBranchSegment(issue.ID) + "-" + suffix
	}
	return "feature/fix-" + suffix
}

func forgeIssueNumber(issue Issue) int {
	if issue.ExternalRef != nil && issue.ExternalRef.ExternalIID > 0 {
		return issue.ExternalRef.ExternalIID
	}
	return issue.Number
}

func workBranchSegment(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else if b.Len() > 0 {
			b.WriteByte('-')
		}
		if b.Len() >= 64 {
			break
		}
	}
	result := strings.Trim(b.String(), "-.")
	if result == "" {
		return "run"
	}
	return result
}
