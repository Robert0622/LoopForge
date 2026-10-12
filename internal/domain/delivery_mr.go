package domain

import (
	"fmt"
	"sort"
	"strings"
)

// DeliveryMR is one merge request delivered for a work item. A multi-repository
// SOP delivers at most one PR per run repository.
type DeliveryMR struct {
	RepositoryID string `json:"repositoryId,omitempty"`
	Repository   string `json:"repository,omitempty"`
	Project      string `json:"project,omitempty"`
	IID          int    `json:"iid,omitempty"`
	URL          string `json:"url"`
	// State is the classified delivery state (merged, integrated, opened, closed, unknown).
	State        string `json:"state,omitempty"`
	TargetBranch string `json:"targetBranch,omitempty"`
	// ForgeState is the raw Forge state from a live lookup; empty when
	// the PR was only recovered from artifact text.
	ForgeState string `json:"-"`
}

// ForgeMRURL is the canonical web URL of a pull request.
func ForgeMRURL(project string, iid int) string {
	project = strings.Trim(strings.TrimSpace(project), "/")
	if project == "" || iid <= 0 {
		return ""
	}
	return fmt.Sprintf("https://%s/%s/pull/%d", ForgeRepoHost, project, iid)
}

// AggregateDeliveryMRState folds per-PR states into the work item label state.
// Closed PRs only win when every PR is closed; otherwise the item is merged
// when all live PRs reached the default branch, integrated when all reached at
// least the integrate baseline, and the first still-open state otherwise.
func AggregateDeliveryMRState(mrs []DeliveryMR) string {
	if len(mrs) == 0 {
		return ""
	}
	norm := func(s string) string {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			return "unknown"
		}
		return s
	}
	if len(mrs) == 1 {
		return norm(mrs[0].State)
	}
	live, merged, integrated := 0, 0, 0
	pending := ""
	for _, mr := range mrs {
		switch st := norm(mr.State); st {
		case "closed":
			continue
		case "merged":
			merged++
		case "integrated":
			integrated++
		default:
			if pending == "" || (pending == "unknown" && st != "unknown") {
				pending = st
			}
		}
		live++
	}
	switch {
	case live == 0:
		return "closed"
	case merged == live:
		return "merged"
	case merged+integrated == live:
		return "integrated"
	default:
		return pending
	}
}

// DeliveryMRList returns the work item's delivery PRs. Items written before
// multi-PR support only carry DeliveryMRURL / DeliveryMRState.
func (i Issue) DeliveryMRList() []DeliveryMR {
	if len(i.DeliveryMRs) > 0 {
		return append([]DeliveryMR(nil), i.DeliveryMRs...)
	}
	url := strings.TrimSpace(i.DeliveryMRURL)
	if url == "" {
		return nil
	}
	mr := DeliveryMR{URL: url, State: strings.TrimSpace(i.DeliveryMRState)}
	if project, iid, _, ok := ParseForgeMR(url); ok {
		mr.Project, mr.IID = project, iid
	}
	return []DeliveryMR{mr}
}

// HasDeliveryMR reports whether url is one of the work item's delivery PRs.
func (i Issue) HasDeliveryMR(url string) bool {
	for _, mr := range i.DeliveryMRList() {
		if SameForgeMR(mr.URL, url) {
			return true
		}
	}
	return false
}

// SetDeliveryMRs replaces the delivery PRs and keeps the legacy single-PR
// fields in sync: DeliveryMRURL is the first PR, DeliveryMRState the aggregate.
func (i *Issue) SetDeliveryMRs(mrs []DeliveryMR) {
	out := make([]DeliveryMR, 0, len(mrs))
	for _, mr := range mrs {
		mr.URL = strings.TrimSpace(mr.URL)
		if mr.URL == "" {
			continue
		}
		dup := false
		for _, cur := range out {
			if SameForgeMR(cur.URL, mr.URL) {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, mr)
		}
	}
	if len(out) == 0 {
		i.DeliveryMRs = nil
		i.DeliveryMRURL = ""
		i.DeliveryMRState = ""
		return
	}
	i.DeliveryMRs = out
	i.DeliveryMRURL = out[0].URL
	i.DeliveryMRState = AggregateDeliveryMRState(out)
}

// SetDeliveryMRState updates one delivery PR's state. A work item without
// any PR adopts url as its only PR. It reports false when url belongs to a
// different PR than the ones already recorded.
func (i *Issue) SetDeliveryMRState(url, state string) bool {
	url = strings.TrimSpace(url)
	if url == "" {
		return false
	}
	list := i.DeliveryMRList()
	if len(list) == 0 {
		mr := DeliveryMR{URL: url, State: state}
		if project, iid, _, ok := ParseForgeMR(url); ok {
			mr.Project, mr.IID = project, iid
		}
		i.SetDeliveryMRs([]DeliveryMR{mr})
		return true
	}
	for idx := range list {
		if SameForgeMR(list[idx].URL, url) {
			list[idx].State = state
			i.SetDeliveryMRs(list)
			return true
		}
	}
	return false
}

// ResolveRunDeliveryMRs picks the delivery PR of every Forge repository in
// the run. byBranch is the authoritative lookup (Forge PR whose source is
// the repository work branch); PR-bearing artifacts are only a fallback and
// must belong to the same Forge project. Runs without any Forge
// repository fall back to the best PR-bearing artifact, then telemetry URLs.
func ResolveRunDeliveryMRs(run Run, byBranch func(RepositoryContext) (DeliveryMR, bool)) []DeliveryMR {
	candidates := runArtifactMRCandidates(run)
	repos := []RepositoryContext{}
	for _, rc := range run.RepositoryContexts() {
		if strings.TrimSpace(rc.RepoPath) != "" && !rc.AlignOnly {
			repos = append(repos, rc)
		}
	}
	if len(repos) == 0 {
		if len(candidates) > 0 {
			return []DeliveryMR{candidates[0].mr}
		}
		for _, obs := range run.Telemetry {
			if url, ok := obs.Attributes["url"].(string); ok {
				if project, iid, parsed, ok := ParseForgeMR(url); ok {
					return []DeliveryMR{{Project: project, IID: iid, URL: parsed}}
				}
			}
		}
		return nil
	}
	out := []DeliveryMR{}
	for _, rc := range repos {
		project := strings.TrimSpace(rc.RepoPath)
		var picked DeliveryMR
		found := false
		if byBranch != nil && strings.TrimSpace(rc.WorkBranch) != "" {
			picked, found = byBranch(rc)
		}
		if !found {
			for _, c := range candidates {
				if strings.EqualFold(c.mr.Project, project) {
					picked, found = c.mr, true
					break
				}
			}
		}
		if !found {
			continue
		}
		if picked.Project == "" {
			picked.Project = project
		}
		if picked.URL == "" {
			picked.URL = ForgeMRURL(picked.Project, picked.IID)
		}
		picked.RepositoryID = rc.RepositoryID
		picked.Repository = rc.Name
		out = append(out, picked)
	}
	return out
}

type artifactMRCandidate struct {
	mr       DeliveryMR
	priority int
	order    int
}

// runArtifactMRCandidates lists PRs quoted by PR-bearing artifacts, best first:
// higher MRArtifactPriority, then later artifacts (a rework's newest PR).
func runArtifactMRCandidates(run Run) []artifactMRCandidate {
	best := map[string]artifactMRCandidate{}
	for idx, art := range run.Artifacts {
		pri := MRArtifactPriority(art.Name, art.Path, art.NodeID)
		if pri <= 0 {
			continue
		}
		for _, ref := range ParseForgeMRs(art.Content) {
			key := fmt.Sprintf("%s#%d", strings.ToLower(ref.Project), ref.IID)
			if cur, ok := best[key]; ok && cur.priority >= pri {
				continue
			}
			best[key] = artifactMRCandidate{mr: DeliveryMR{Project: ref.Project, IID: ref.IID, URL: ref.URL}, priority: pri, order: idx}
		}
	}
	out := make([]artifactMRCandidate, 0, len(best))
	for _, c := range best {
		out = append(out, c)
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].priority != out[b].priority {
			return out[a].priority > out[b].priority
		}
		if out[a].order != out[b].order {
			return out[a].order > out[b].order
		}
		return out[a].mr.IID > out[b].mr.IID
	})
	return out
}
