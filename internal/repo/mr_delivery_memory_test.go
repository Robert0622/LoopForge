package repo

import (
	"testing"
	"time"

	"github.com/Tencent/LoopForge/internal/domain"
)

func TestRunMRTextPrefersMRBodyOverPatch(t *testing.T) {
	text := runMRText(domain.Run{Artifacts: []domain.Artifact{
		{Name: "repository-patch", NodeID: "mr", Content: "-https://github.com/org/repo/pull/282\n+https://github.com/org/repo/pull/286"},
		{Name: "mr-body", NodeID: "mr", Content: "https://github.com/org/repo/pull/286"},
		{Name: "closeout", NodeID: "close", Content: "https://github.com/org/repo/pull/286"},
	}})
	refs := domain.ParseForgeMRs(text)
	if len(refs) != 1 || refs[0].IID != 286 {
		t.Fatalf("expected only !286 from mr-body, got %+v text=%q", refs, text)
	}
}

func TestBetterAnalyticsMRPrefersIssueDelivery(t *testing.T) {
	older := mrRef{project: "org/repo", iid: 282, createdAt: time.Now().UTC(), sourceRank: 10}
	own := mrRef{project: "org/repo", iid: 286, createdAt: time.Now().UTC().Add(-time.Hour), sourceRank: 10}
	if !betterAnalyticsMR("org/repo", 286, older, "integrated", own, "integrated") {
		t.Fatal("issue delivery PR should win when both are on the baseline")
	}
	if betterAnalyticsMR("org/repo", 286, own, "integrated", older, "integrated") {
		t.Fatal("should not replace the preferred delivery PR")
	}
}

func TestApplyMRDeliveryStatesMemoryStore(t *testing.T) {
	s, err := OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	items := []domain.AnalyticsDelivery{{
		IssueID: "gf-1", MRProject: "org/repo", MRIID: 9,
		MRURL:   "https://github.com/org/repo/pull/9",
		MRState: "unknown", SOPSucceeded: true,
	}}
	issue, _ := s.Issue("gf-1")
	s.applyMRDeliveryStates(items, map[string][]mrRef{
		"gf-1": {{project: "org/repo", iid: 9, url: "https://github.com/org/repo/pull/9"}},
	}, map[string]domain.Issue{"gf-1": issue})
	if items[0].MRState != "unknown" || items[0].Delivered {
		t.Fatalf("memory store keeps unknown without db: %#v", items[0])
	}
}

func TestMarkAndReopenDeliveredIssue(t *testing.T) {
	s, err := OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	issue := domain.Issue{ID: "req-1", ProjectID: "platform", Source: "local", Status: "in_review", AutomationState: "eligible"}
	if err := s.PutIssue(issue); err != nil {
		t.Fatal(err)
	}
	mergedAt := time.Now().UTC().Add(-time.Minute)
	if err := s.MarkIssueDelivered(issue.ID, "https://github.com/org/repo/pull/1", mergedAt); err != nil {
		t.Fatal(err)
	}
	locked, _ := s.Issue(issue.ID)
	if !locked.DeliveryLocked() || locked.Status != "done" || locked.AutomationState != "ignored" {
		t.Fatalf("merged issue not locked: %+v", locked)
	}
	reopened, err := s.ReopenDeliveredIssue(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.DeliveryState != "reopened" || reopened.Status != "todo" || reopened.AutomationState != "eligible" {
		t.Fatalf("issue not reopened: %+v", reopened)
	}
	if err := s.MarkIssueDelivered(issue.ID, locked.DeliveryMRURL, mergedAt); err != nil {
		t.Fatal(err)
	}
	stillOpen, _ := s.Issue(issue.ID)
	if stillOpen.DeliveryState != "reopened" {
		t.Fatalf("old merged PR relocked reopened issue: %+v", stillOpen)
	}
	if err := s.MarkIssueDelivered(issue.ID, locked.DeliveryMRURL, time.Now().UTC().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	stillOpen, _ = s.Issue(issue.ID)
	if stillOpen.DeliveryState != "reopened" {
		t.Fatalf("newer PR check must not relock a reopened issue: %+v", stillOpen)
	}
	if err := s.MarkIssueDeliveredFromSOP(issue.ID, "https://github.com/org/repo/pull/2", time.Now().UTC().Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	lockedAgain, _ := s.Issue(issue.ID)
	if !lockedAgain.DeliveryLocked() {
		t.Fatalf("new SOP success should lock a reopened issue: %+v", lockedAgain)
	}
}

func TestExternalClosedIssueKeepsStatusDuringDelivery(t *testing.T) {
	s, err := OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	issue := domain.Issue{
		ID: "gf-closed", ProjectID: "platform", Source: "github",
		Status: "closed", CloseSource: "external",
		ExternalRef: &domain.ExternalRef{Source: "github", ExternalState: "closed"},
	}
	if err := s.PutIssue(issue); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkIssueDelivered(issue.ID, "https://github.com/org/repo/pull/1", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Issue(issue.ID)
	if got.Status != "closed" || got.CloseSource != "external" || !got.DeliveryLocked() {
		t.Fatalf("PR delivery must not turn external closure into done: %+v", got)
	}
	if _, err := s.ReopenDeliveredIssue(issue.ID); err == nil {
		t.Fatal("externally closed issue must be reopened in its tracker first")
	}
}

func TestReconcileCompletedDeliveries(t *testing.T) {
	s, err := OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	issue := domain.Issue{ID: "req-done", ProjectID: "platform", Source: "local", LatestTaskID: "task-1"}
	if err := s.PutIssue(issue); err != nil {
		t.Fatal(err)
	}
	if err := s.PutTask(domain.Task{ID: "task-1", IssueID: issue.ID, RunID: "run-1", Status: "completed"}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutRun(domain.Run{ID: "run-1", Status: "succeeded", UpdatedAt: time.Now().UTC(), ExecutionContext: domain.TelemetryContext{IssueID: issue.ID}}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReconcileCompletedDeliveries(); err != nil {
		t.Fatal(err)
	}
	locked, _ := s.Issue(issue.ID)
	if !locked.DeliveryLocked() {
		t.Fatalf("expected succeeded SOP to lock issue: %+v", locked)
	}
}

func TestClassifyMRDeliveryUsesRepoBranches(t *testing.T) {
	s, err := OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	if err := s.PutRepository(domain.Repository{
		ID: "repo-widget", ProjectID: "platform", Name: "widget",
		RepoPath:        "acme/widget",
		DefaultBranch:   "master",
		IntegrateBranch: "loopforge/integration",
	}); err != nil {
		t.Fatal(err)
	}
	baseline := s.classifyMRDelivery(domain.MRDeliveryState{
		MRProject: "acme/widget", State: "merged", TargetBranch: "loopforge/integration",
	})
	if baseline != "integrated" {
		t.Fatalf("baseline merge classified as %q", baseline)
	}
	mainline := s.classifyMRDelivery(domain.MRDeliveryState{
		MRProject: "acme/widget", State: "merged", TargetBranch: "master",
	})
	if mainline != "merged" {
		t.Fatalf("mainline merge classified as %q", mainline)
	}
	pending := s.classifyMRDelivery(domain.MRDeliveryState{
		MRProject: "acme/widget", State: "merged", TargetBranch: "",
	})
	if pending != "unknown" {
		t.Fatalf("merged without target classified as %q", pending)
	}
}

func TestPromoteIssueToMainlineKeepsWorkMR(t *testing.T) {
	s, err := OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	issue := domain.Issue{
		ID: "gh-widget-32", ProjectID: "platform", Source: "github",
		Status: "done", DeliveryState: "merged", DeliveryMRState: "integrated",
		DeliveryMRURL:     "https://github.com/acme/widget/pull/293",
		DeliveryUpdatedAt: time.Now().UTC().Add(-time.Hour),
	}
	if err := s.PutIssue(issue); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC()
	if err := s.PromoteIssueToMainline(issue.ID, issue.DeliveryMRURL, at); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Issue(issue.ID)
	if got.DeliveryMRState != "merged" {
		t.Fatalf("expected mainline label, got %+v", got)
	}
	if got.DeliveryMRURL != issue.DeliveryMRURL {
		t.Fatalf("work PR URL changed: %s", got.DeliveryMRURL)
	}
	if err := s.ReconcileDeliveryMRStates(); err != nil {
		t.Fatal(err)
	}
	still, _ := s.Issue(issue.ID)
	if still.DeliveryMRState != "merged" {
		t.Fatalf("reconcile downgraded mainline label: %+v", still)
	}
}

func TestPromoteSkipsWorkAfterPromote(t *testing.T) {
	s, err := OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	promoteAt := time.Now().UTC().Add(-time.Hour)
	old := domain.Issue{
		ID: "gf-old", ProjectID: "platform",
		DeliveryState: "merged", DeliveryMRState: "integrated",
		DeliveryMRURL:     "https://github.com/org/repo/pull/10",
		DeliveryUpdatedAt: promoteAt.Add(-time.Hour),
	}
	newer := domain.Issue{
		ID: "gf-new", ProjectID: "platform",
		DeliveryState: "merged", DeliveryMRState: "integrated",
		DeliveryMRURL:     "https://github.com/org/repo/pull/20",
		DeliveryUpdatedAt: promoteAt.Add(time.Hour),
	}
	if err := s.PutIssue(old); err != nil {
		t.Fatal(err)
	}
	if err := s.PutIssue(newer); err != nil {
		t.Fatal(err)
	}
	if err := s.promoteIntegratedIssues("org/repo", promoteAt); err != nil {
		t.Fatal(err)
	}
	gotOld, _ := s.Issue(old.ID)
	gotNew, _ := s.Issue(newer.ID)
	if gotOld.DeliveryMRState != "merged" {
		t.Fatalf("older baseline PR should promote: %+v", gotOld)
	}
	if gotNew.DeliveryMRState != "integrated" {
		t.Fatalf("newer baseline PR must stay integrated: %+v", gotNew)
	}
}

func TestApplyMRDeliveryStatesHonorsPromotedIssue(t *testing.T) {
	s, err := OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	if err := s.PutIssue(domain.Issue{
		ID: "gf-1", DeliveryState: "merged", DeliveryMRState: "merged",
		DeliveryMRURL: "https://github.com/org/repo/pull/9",
	}); err != nil {
		t.Fatal(err)
	}
	items := []domain.AnalyticsDelivery{{
		IssueID: "gf-1", MRProject: "org/repo", MRIID: 9,
		MRURL:   "https://github.com/org/repo/pull/9",
		MRState: "integrated", SOPSucceeded: true,
	}}
	issue, _ := s.Issue("gf-1")
	s.applyMRDeliveryStates(items, map[string][]mrRef{
		"gf-1": {{project: "org/repo", iid: 9, url: items[0].MRURL}},
	}, map[string]domain.Issue{"gf-1": issue})
	if items[0].MRState != "merged" || !items[0].Delivered {
		t.Fatalf("promoted issue should count as delivered: %#v", items[0])
	}
}

func TestFinalizeAnalyticsDeliveryMerged(t *testing.T) {
	item := domain.AnalyticsDelivery{SOPSucceeded: true, MRURL: "http://x", MRState: "merged"}
	domain.FinalizeAnalyticsDelivery(&item)
	if !item.Delivered {
		t.Fatal("expected delivered")
	}
}

func TestStaleOpenedMRDoesNotOverwriteCanonicalDelivery(t *testing.T) {
	s, err := OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	canonical := "https://github.com/acme/widget/pull/248"
	stale := "https://github.com/acme/widget/pull/183"
	issue := domain.Issue{
		ID: "gf-19", ProjectID: "widget", Source: "github",
		Status: "done", DeliveryState: "merged", DeliveryMRState: "merged",
		DeliveryMRURL:     canonical,
		DeliveryUpdatedAt: time.Now().UTC().Add(-time.Hour),
	}
	if err := s.PutIssue(issue); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureMRDeliveryState("acme/widget", 248, canonical, issue.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateMRDeliveryState("acme/widget", 248, "merged", "master", time.Now().UTC().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureMRDeliveryState("acme/widget", 183, stale, issue.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateMRDeliveryState("acme/widget", 183, "opened", "master", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Issue(issue.ID)
	if got.DeliveryMRURL != canonical {
		t.Fatalf("canonical PR URL changed: %s", got.DeliveryMRURL)
	}
	if got.DeliveryMRState != "merged" {
		t.Fatalf("stale opened PR overwrote delivery label: %+v", got)
	}
}

func TestCanonicalMRPollStillUpdatesDeliveryLabel(t *testing.T) {
	s, err := OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	canonical := "https://github.com/org/repo/pull/9"
	issue := domain.Issue{
		ID: "gf-1", Status: "done", DeliveryState: "merged", DeliveryMRState: "opened",
		DeliveryMRURL:     "https://github.com/org/repo/pull/9",
		DeliveryUpdatedAt: time.Now().UTC().Add(-time.Hour),
	}
	if err := s.PutIssue(issue); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureMRDeliveryState("org/repo", 9, canonical, issue.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateMRDeliveryState("org/repo", 9, "merged", "master", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Issue(issue.ID)
	if got.DeliveryMRState != "merged" {
		t.Fatalf("canonical merged poll should set PR 已合入, got %+v", got)
	}
}

func TestMultiRepositoryDeliveryMRs(t *testing.T) {
	s, err := OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	issue := domain.Issue{ID: "req-multi", ProjectID: "platform", Source: "local", Status: "in_review"}
	if err := s.PutIssue(issue); err != nil {
		t.Fatal(err)
	}
	urlA := "https://github.com/org/a/pull/1"
	urlB := "https://github.com/org/b/pull/2"
	at := time.Now().UTC().Add(-time.Hour)
	if err := s.MarkIssueDeliveredFromSOPMRs(issue.ID, []domain.DeliveryMR{
		{RepositoryID: "a", Repository: "a", Project: "org/a", IID: 1, URL: urlA, ForgeState: "merged", TargetBranch: "master"},
		{RepositoryID: "b", Repository: "b", Project: "org/b", IID: 2, URL: urlB, ForgeState: "opened", TargetBranch: "master"},
	}, at); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Issue(issue.ID)
	if !got.DeliveryLocked() || len(got.DeliveryMRs) != 2 || got.DeliveryMRURL != urlA || got.DeliveryMRState != "opened" {
		t.Fatalf("multi-repo delivery not recorded: %+v", got)
	}
	if got.DeliveryMRs[0].State != "merged" || got.DeliveryMRs[1].State != "opened" || got.DeliveryLockedLabel() != "交付闭环" {
		t.Fatalf("per-PR states / label wrong: %+v label=%s", got.DeliveryMRs, got.DeliveryLockedLabel())
	}
	// Both PRs are registered for polling; the second merge lands the item.
	if err := s.UpdateMRDeliveryState("org/b", 2, "merged", "master", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Issue(issue.ID)
	if got.DeliveryMRState != "merged" || got.DeliveryLockedLabel() != "PR 已合入" || got.DeliveryMRURL != urlA {
		t.Fatalf("all PRs merged must read PR 已合入: %+v", got)
	}
	// A leftover PR from an earlier run must not join or relabel the item.
	if err := s.EnsureMRDeliveryState("org/a", 9, "https://github.com/org/a/pull/9", issue.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateMRDeliveryState("org/a", 9, "closed", "master", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Issue(issue.ID)
	if len(got.DeliveryMRs) != 2 || got.DeliveryMRState != "merged" {
		t.Fatalf("leftover PR changed delivery PRs: %+v", got)
	}
}

func TestReconcileDeliveryMRStatesMigratesLegacySingleMR(t *testing.T) {
	s, err := OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	url := "https://github.com/org/a/pull/3"
	issue := domain.Issue{ID: "req-legacy", ProjectID: "platform", Status: "done", DeliveryState: "merged", DeliveryMRURL: url, DeliveryMRState: "opened", DeliveryUpdatedAt: time.Now().UTC()}
	if err := s.PutIssue(issue); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureMRDeliveryState("org/a", 3, url, issue.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.recordMRState("org/a", 3, "merged", "master", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := s.ReconcileDeliveryMRStates(); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Issue(issue.ID)
	if len(got.DeliveryMRs) != 1 || got.DeliveryMRs[0].URL != url || got.DeliveryMRState != "merged" {
		t.Fatalf("legacy PR not migrated into deliveryMrs: %+v", got)
	}
}
