package tracker

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Tencent/LoopForge/internal/domain"
)

func findForgeIssue(store IssueStoreAdapter, repo domain.Repository, number int) (domain.Issue, bool) {
	var matches []domain.Issue
	seen := map[string]bool{}
	add := func(issue domain.Issue) {
		if issue.ID == "" || seen[issue.ID] || !sameForgeRepo(issue, repo, number) {
			return
		}
		seen[issue.ID] = true
		matches = append(matches, issue)
	}
	if repo.ID != "" {
		if issue, ok := store.Issue(forgeIssueID(repo.ID, number)); ok {
			add(issue)
		}
	}
	for _, issue := range store.Issues() {
		if issue.Source != Source {
			continue
		}
		n := 0
		if issue.ExternalRef != nil {
			n = issue.ExternalRef.ExternalIID
		}
		if n == 0 {
			_, n = parseForgeIssueID(issue.ID)
		}
		if n != number {
			continue
		}
		add(issue)
	}
	if len(matches) == 0 {
		return domain.Issue{}, false
	}
	return pickIssueKeeper(matches), true
}

func sameForgeRepo(issue domain.Issue, repo domain.Repository, number int) bool {
	if strings.TrimSpace(repo.ID) == "" {
		return false
	}
	if repoID := strings.TrimSpace(issue.RepositoryID); repoID != "" {
		return repoID == repo.ID
	}
	if issue.ExternalRef != nil {
		if ref, ok := ParseIssueURL(issue.ExternalRef.ExternalURL); ok {
			return strings.EqualFold(ref.Repo, strings.TrimSpace(repo.RepoPath))
		}
	}
	if parsedRepo, n := parseForgeIssueID(issue.ID); n == number && parsedRepo != "" {
		return parsedRepo == repo.ID
	}
	return false
}

// applyExistingIssue merges a freshly pulled remote issue onto the stored
// copy: local ownership, workflow, delivery and clarification state win, and
// local execution status is kept unless the remote side closed the issue.
func applyExistingIssue(dst *domain.Issue, existing domain.Issue) {
	previousExternalClosed := existing.TrackerClosed()
	remoteClosed := dst.Status == "closed"
	dst.ID = existing.ID
	dst.CreatedAt = existing.CreatedAt
	if existing.Number > 0 {
		dst.Number = existing.Number
	}
	dst.AssigneeType = existing.AssigneeType
	dst.AssigneeID = existing.AssigneeID
	dst.AssigneeUserID = existing.AssigneeUserID
	dst.WorkflowID = existing.WorkflowID
	dst.LatestTaskID = existing.LatestTaskID
	dst.AutomationState = existing.AutomationState
	if existing.ManuallyClosed() {
		dst.CloseSource = "manual"
	} else if previousExternalClosed && !remoteClosed && !existing.DeliveryLocked() {
		// The remote issue reopened; scheduled execution may resume.
		dst.AutomationState = "eligible"
	}
	dst.DeliveryState = existing.DeliveryState
	dst.DeliveryMRURL = existing.DeliveryMRURL
	dst.DeliveryMRState = existing.DeliveryMRState
	dst.DeliveryMRs = existing.DeliveryMRs
	dst.DeliveryUpdatedAt = existing.DeliveryUpdatedAt
	if existing.Kind != "" {
		dst.Kind = existing.Kind
	}
	dst.Clarification = existing.Clarification
	if existing.ExternalRef != nil && existing.ExternalRef.Imported {
		if dst.ExternalRef == nil {
			ref := *existing.ExternalRef
			dst.ExternalRef = &ref
		} else {
			dst.ExternalRef.Imported = true
		}
	}
	if existing.ProjectID != "" {
		dst.ProjectID = existing.ProjectID
	}
	if existing.RepositoryID != "" {
		dst.RepositoryID = existing.RepositoryID
	}
	if len(existing.RepositoryIDs) > 0 {
		// Remote issues are single-repository; keep the local multi-repo
		// binding intact so scheduled SOPs keep touching every repository.
		dst.RepositoryIDs = append([]string(nil), existing.RepositoryIDs...)
		if dst.RepositoryID == "" {
			dst.RepositoryID = existing.RepositoryIDs[0]
		}
	}
	if existing.BaseBranch != "" {
		dst.BaseBranch = existing.BaseBranch
	}
	if keepLocalExecutionStatus(existing, dst.Status) {
		dst.Status = existing.Status
	} else if existing.CloseSource == "external" && !remoteClosed && existing.DeliveryLocked() {
		// A completed local delivery remains done after the remote reopens.
		dst.Status = "done"
	}
	if dst.Status != "closed" {
		dst.CloseSource = ""
	}
}

func keepLocalExecutionStatus(existing domain.Issue, remote string) bool {
	if existing.ManuallyClosed() {
		return true
	}
	if remote == "closed" || existing.CloseSource == "external" {
		return false
	}
	if existing.DeliveryLocked() {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(existing.DeliveryState), "reopened") {
		return true
	}
	switch existing.Status {
	case "in_progress", "blocked", "in_review":
		return remote != "done" && remote != "cancelled"
	default:
		return false
	}
}

func pruneDuplicateExternalIssues(store IssueStoreAdapter) {
	groups := map[string][]domain.Issue{}
	for _, issue := range store.Issues() {
		key := externalIssueKey(issue)
		if key == "" {
			continue
		}
		groups[key] = append(groups[key], issue)
	}
	for _, group := range groups {
		if len(group) < 2 {
			continue
		}
		keeper := pickIssueKeeper(group)
		for _, extra := range group {
			if extra.ID == keeper.ID {
				continue
			}
			keeper = mergeIssueKeep(keeper, extra)
			_ = store.DeleteIssue(extra.ID)
		}
		_ = store.PutIssue(keeper)
	}
}

func externalIssueKey(issue domain.Issue) string {
	if issue.Source != Source {
		return ""
	}
	n := 0
	if issue.ExternalRef != nil {
		n = issue.ExternalRef.ExternalIID
	}
	if n == 0 {
		_, n = parseForgeIssueID(issue.ID)
	}
	if n == 0 {
		return ""
	}
	scope := forgeIssueScope(issue)
	if scope == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s:%d", Source, scope, n)
}

func pickIssueKeeper(issues []domain.Issue) domain.Issue {
	best := issues[0]
	for _, issue := range issues[1:] {
		if betterIssue(issue, best) {
			best = issue
		}
	}
	return best
}

func betterIssue(a, b domain.Issue) bool {
	if (a.LatestTaskID != "") != (b.LatestTaskID != "") {
		return a.LatestTaskID != ""
	}
	if (a.WorkflowID != "") != (b.WorkflowID != "") {
		return a.WorkflowID != ""
	}
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.Before(b.CreatedAt)
	}
	if len(a.ID) != len(b.ID) {
		return len(a.ID) < len(b.ID)
	}
	return a.ID < b.ID
}

// parseForgeIssueID splits a "gh-<repositoryID>-<number>" issue ID.
func parseForgeIssueID(id string) (repo string, number int) {
	body := strings.TrimPrefix(strings.TrimSpace(id), "gh-")
	if body == "" || body == strings.TrimSpace(id) {
		return "", 0
	}
	i := strings.LastIndex(body, "-")
	if i <= 0 {
		return "", 0
	}
	n, err := strconv.Atoi(body[i+1:])
	if err != nil || n <= 0 {
		return "", 0
	}
	return body[:i], n
}

func forgeIssueScope(issue domain.Issue) string {
	if repo := strings.TrimSpace(issue.RepositoryID); repo != "" {
		return repo
	}
	if issue.ExternalRef != nil {
		if ref, ok := ParseIssueURL(issue.ExternalRef.ExternalURL); ok {
			return strings.ToLower(ref.Repo)
		}
	}
	if repo, _ := parseForgeIssueID(issue.ID); repo != "" {
		return repo
	}
	return ""
}

func mergeIssueKeep(keeper, extra domain.Issue) domain.Issue {
	if keeper.LatestTaskID == "" {
		keeper.LatestTaskID = extra.LatestTaskID
	}
	if keeper.WorkflowID == "" {
		keeper.WorkflowID = extra.WorkflowID
	}
	if keeper.AssigneeID == "" {
		keeper.AssigneeID = extra.AssigneeID
		keeper.AssigneeType = extra.AssigneeType
	}
	if keeper.RepositoryID == "" {
		keeper.RepositoryID = extra.RepositoryID
	}
	if keeper.Clarification.Score == 0 && extra.Clarification.Score > 0 {
		keeper.Clarification = extra.Clarification
	}
	if strings.TrimSpace(extra.DeliveryState) != "" {
		if strings.TrimSpace(keeper.DeliveryState) == "" || extra.DeliveryUpdatedAt.After(keeper.DeliveryUpdatedAt) {
			keeper.DeliveryState = extra.DeliveryState
			keeper.DeliveryMRURL = extra.DeliveryMRURL
			keeper.DeliveryMRState = extra.DeliveryMRState
			keeper.DeliveryMRs = extra.DeliveryMRs
			keeper.DeliveryUpdatedAt = extra.DeliveryUpdatedAt
		}
	}
	return keeper
}
