package domain

import "testing"

func TestAggregateDeliveryMRState(t *testing.T) {
	mr := func(states ...string) []DeliveryMR {
		out := make([]DeliveryMR, len(states))
		for i, s := range states {
			out[i] = DeliveryMR{URL: ForgeMRURL("g/r", i+1), State: s}
		}
		return out
	}
	cases := []struct {
		name string
		mrs  []DeliveryMR
		want string
	}{
		{"none", nil, ""},
		{"single empty is unknown", mr(""), "unknown"},
		{"single closed", mr("closed"), "closed"},
		{"all merged", mr("merged", "merged"), "merged"},
		{"merged and integrated", mr("merged", "integrated"), "integrated"},
		{"one still open", mr("merged", "opened"), "opened"},
		{"open beats unknown", mr("unknown", "opened"), "opened"},
		{"closed ignored when others live", mr("closed", "merged"), "merged"},
		{"all closed", mr("closed", "closed"), "closed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := AggregateDeliveryMRState(tc.mrs); got != tc.want {
				t.Fatalf("AggregateDeliveryMRState() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIssueDeliveryMRsKeepLegacyFields(t *testing.T) {
	legacy := Issue{DeliveryMRURL: "https://github.com/g/a/pull/3", DeliveryMRState: "opened"}
	if list := legacy.DeliveryMRList(); len(list) != 1 || list[0].Project != "g/a" || list[0].IID != 3 || list[0].State != "opened" {
		t.Fatalf("legacy list = %+v", list)
	}
	issue := Issue{}
	issue.SetDeliveryMRs([]DeliveryMR{
		{URL: "https://github.com/g/a/pull/3", State: "merged"},
		{URL: "https://github.com/g/a/pull/3", State: "opened"},
		{URL: "https://github.com/g/b/pull/9", State: "opened"},
	})
	if len(issue.DeliveryMRs) != 2 || issue.DeliveryMRURL != "https://github.com/g/a/pull/3" || issue.DeliveryMRState != "opened" {
		t.Fatalf("SetDeliveryMRs = %+v", issue)
	}
	if !issue.SetDeliveryMRState("https://github.com/g/b/pull/9", "merged") || issue.DeliveryMRState != "merged" {
		t.Fatalf("state update not aggregated: %+v", issue)
	}
	if issue.SetDeliveryMRState("https://github.com/g/c/pull/1", "merged") {
		t.Fatal("foreign PR must not join the delivery list")
	}
}

func TestResolveRunDeliveryMRsPerRepository(t *testing.T) {
	run := Run{
		Repositories: []RepositoryContext{
			{RepositoryID: "repo-a", Name: "a", RepoPath: "g/a", WorkBranch: "feature/x"},
			{RepositoryID: "repo-b", Name: "b", RepoPath: "g/b", WorkBranch: "feature/x"},
			{RepositoryID: "repo-c", Name: "c", RepoPath: "g/c", WorkBranch: "feature/x"},
			{RepositoryID: "repo-d", Name: "d", RepoPath: "g/d", AlignOnly: true},
		},
		Artifacts: []Artifact{
			// Patches quote unrelated PRs and must never be used.
			{Name: "repository-patch", Content: "https://github.com/g/c/pull/77"},
			{Name: "design", Content: "https://github.com/g/c/pull/78"},
			{Name: "mr-body", NodeID: "mr", Content: "https://github.com/g/a/pull/1 https://github.com/g/b/pull/5 https://github.com/g/d/pull/2"},
			{Name: "closeout", Content: "https://github.com/g/b/pull/4"},
		},
	}
	byBranch := func(rc RepositoryContext) (DeliveryMR, bool) {
		if rc.RepoPath == "g/a" {
			return DeliveryMR{IID: 12, ForgeState: "opened", TargetBranch: "master"}, true
		}
		return DeliveryMR{}, false
	}
	got := ResolveRunDeliveryMRs(run, byBranch)
	if len(got) != 2 {
		t.Fatalf("want PRs for repo-a and repo-b only, got %+v", got)
	}
	if got[0].RepositoryID != "repo-a" || got[0].IID != 12 || got[0].URL != "https://github.com/g/a/pull/12" || got[0].Repository != "a" {
		t.Fatalf("branch lookup must win for repo-a: %+v", got[0])
	}
	if got[1].RepositoryID != "repo-b" || got[1].IID != 5 {
		t.Fatalf("repo-b must fall back to the mr-body PR of its own project: %+v", got[1])
	}
}

func TestResolveRunDeliveryMRsWithoutForgeRepository(t *testing.T) {
	run := Run{
		Artifacts: []Artifact{
			{Name: "repository-patch", Content: "https://github.com/x/y/pull/1"},
			{Name: "e2e-report", Content: "https://github.com/x/y/pull/2"},
			{Name: "mr-body", Content: "https://github.com/x/y/pull/3"},
		},
	}
	got := ResolveRunDeliveryMRs(run, nil)
	if len(got) != 1 || got[0].IID != 3 {
		t.Fatalf("want highest-priority artifact PR, got %+v", got)
	}
	run = Run{Telemetry: []TelemetryEvent{{Attributes: map[string]any{"url": "https://github.com/x/y/pull/8"}}}}
	if got := ResolveRunDeliveryMRs(run, nil); len(got) != 1 || got[0].IID != 8 {
		t.Fatalf("want telemetry fallback, got %+v", got)
	}
}
