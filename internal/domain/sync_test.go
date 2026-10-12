package domain

import (
	"reflect"
	"testing"
)

func TestNormalizeIntegrateBranch(t *testing.T) {
	if got := NormalizeIntegrateBranch("  loopforge/integration  "); got != "loopforge/integration" {
		t.Fatalf("trim = %q", got)
	}
	if got := NormalizeIntegrateBranch(""); got != "" {
		t.Fatalf("empty must stay empty, got %q", got)
	}
}

func TestNormalizeScheduleIssueStatuses(t *testing.T) {
	got := NormalizeScheduleIssueStatuses([]string{" TODO ", "done", "backlog", "todo", "unknown", "blocked"})
	want := []string{"backlog", "todo", "blocked"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalize = %v, want %v", got, want)
	}
}

func TestScheduleJobTriggerIssueStatuses(t *testing.T) {
	empty := ScheduleJob{WorkflowID: "wf"}
	if got := empty.TriggerIssueStatuses(); !reflect.DeepEqual(got, ScheduleIssueStatusOptions) {
		t.Fatalf("empty default = %v", got)
	}
	if !empty.MatchesIssueStatus("todo") || !empty.MatchesIssueStatus("") || !empty.MatchesIssueStatus("in_progress") {
		t.Fatal("legacy empty job should match all open statuses")
	}
	custom := ScheduleJob{WorkflowID: "wf", IssueStatuses: []string{"in_progress", "blocked"}}
	if !custom.MatchesIssueStatus("blocked") || custom.MatchesIssueStatus("todo") {
		t.Fatal("custom statuses were not honored")
	}
	ai := ScheduleJob{SkipDispatch: true, IssueStatuses: []string{"todo"}}
	if len(ai.TriggerIssueStatuses()) != 0 || ai.MatchesIssueStatus("todo") {
		t.Fatal("AI jobs must not scan issue statuses")
	}
}
