package domain

import (
	"strings"
	"testing"
)

func TestPlatformRequirementKeyIsMetricsOnly(t *testing.T) {
	issue := Issue{ID: "platform-1", Source: "local", Kind: "requirement", TrackingID: " PROJ-1001 "}
	if got := issue.RequirementKey(); got != "PROJ-1001" {
		t.Fatalf("tracking ID = %q", got)
	}
	block := issue.IdentityBlock()
	if !strings.Contains(block, "统计关联 ID：PROJ-1001") || strings.Contains(block, "回写") {
		t.Fatalf("manual metrics link must not instruct write-back: %s", block)
	}
	issue.TrackingID = ""
	if issue.RequirementKey() != "" || !strings.Contains(issue.IdentityBlock(), "外部 ID：无") {
		t.Fatalf("unbound platform issue is valid: %+v", issue)
	}
	issue.Source = "github"
	issue.TrackingID = "PROJ-9999"
	issue.ExternalRef = &ExternalRef{Source: "github", ExternalID: "2002", ExternalURL: "https://github.com/acme/api/issues/7"}
	if got := issue.RequirementKey(); got != "https://github.com/acme/api/issues/7" {
		t.Fatalf("synced tracker issue must win: %q", got)
	}
}
