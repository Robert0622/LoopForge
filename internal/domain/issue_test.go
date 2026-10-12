package domain

import (
	"strings"
	"testing"
)

func TestForgeClosed(t *testing.T) {
	closed := Issue{Source: "github", ExternalRef: &ExternalRef{ExternalState: "closed"}}
	if !closed.ForgeClosed() {
		t.Fatal("closed Forge issue must be ForgeClosed")
	}
	opened := Issue{Source: "github", ExternalRef: &ExternalRef{ExternalState: "opened"}}
	if opened.ForgeClosed() {
		t.Fatal("opened Forge issue must still be triggerable")
	}
	localDone := Issue{Source: "github", Status: "done"}
	if localDone.ForgeClosed() {
		t.Fatal("local status alone must not block trigger")
	}
	other := Issue{Source: "local", ExternalRef: &ExternalRef{ExternalState: "closed"}}
	if other.ForgeClosed() {
		t.Fatal("non-GitHub closed items are not ForgeClosed")
	}
}

func TestIssueCloseSource(t *testing.T) {
	manual := Issue{Status: "closed", CloseSource: "manual"}
	if !manual.ManuallyClosed() || manual.TrackerClosed() {
		t.Fatalf("manual close classification: %+v", manual)
	}
	legacy := Issue{Status: "closed"}
	if !legacy.ManuallyClosed() {
		t.Fatal("older manually closed issues must remain manual")
	}
	external := Issue{Source: "github", Status: "closed", CloseSource: "external", ExternalRef: &ExternalRef{ExternalState: "closed"}}
	if external.ManuallyClosed() || !external.TrackerClosed() {
		t.Fatalf("external close classification: %+v", external)
	}
	if !strings.Contains(external.ExecutionBlockedReason(), "外部系统") {
		t.Fatalf("external close block reason = %q", external.ExecutionBlockedReason())
	}
}

func TestMergedDeliveryBlocksUntilReopened(t *testing.T) {
	merged := Issue{Source: "github", Kind: "requirement", DeliveryState: "merged"}
	if !merged.DeliveryLocked() || merged.ExecutionBlockedReason() == "" {
		t.Fatal("merged delivery must block execution")
	}
	if !strings.Contains(merged.ExecutionBlockedReason(), "交付闭环") {
		t.Fatalf("expected delivery-closed wording without merged PR, got %q", merged.ExecutionBlockedReason())
	}
	if merged.DeliveryLockedLabel() != "交付闭环" {
		t.Fatalf("expected delivery-closed label without merged PR, got %q", merged.DeliveryLockedLabel())
	}
	merged.DeliveryMRURL = "https://github.com/org/repo/pull/1"
	merged.DeliveryMRState = "opened"
	if strings.Contains(merged.ExecutionBlockedReason(), "PR 已合入") {
		t.Fatalf("opened PR must not read as merged, got %q", merged.ExecutionBlockedReason())
	}
	merged.DeliveryMRState = "merged"
	if !strings.Contains(merged.ExecutionBlockedReason(), "PR 已合入") {
		t.Fatalf("expected PR merged wording, got %q", merged.ExecutionBlockedReason())
	}
	if merged.DeliveryLockedLabel() != "PR 已合入" {
		t.Fatalf("expected PR merged label, got %q", merged.DeliveryLockedLabel())
	}
	merged.DeliveryMRState = "integrated"
	if merged.DeliveryLockedLabel() != "已合入基线" {
		t.Fatalf("expected baseline label, got %q", merged.DeliveryLockedLabel())
	}
	if !strings.Contains(merged.ExecutionBlockedReason(), "已合入基线") {
		t.Fatalf("expected baseline block reason, got %q", merged.ExecutionBlockedReason())
	}
	merged.DeliveryMRState = "closed"
	if merged.DeliveryLockedLabel() != "PR 已关闭" {
		t.Fatalf("expected closed label, got %q", merged.DeliveryLockedLabel())
	}
	noLink := Issue{Status: "done", DeliveryState: "merged", DeliveryMRState: "closed"}
	if noLink.DeliveryLockedLabel() != "已关闭" || !strings.Contains(noLink.ExecutionBlockedReason(), "工作项已关闭") {
		t.Fatalf("closed PR state without a link is a manual close, label=%q reason=%q", noLink.DeliveryLockedLabel(), noLink.ExecutionBlockedReason())
	}
	if !strings.Contains(merged.ExecutionBlockedReason(), "PR 已关闭") {
		t.Fatalf("expected closed block reason, got %q", merged.ExecutionBlockedReason())
	}
	merged.Status = "closed"
	if merged.DeliveryLockedLabel() != "关闭" || !strings.Contains(merged.ExecutionBlockedReason(), "工作项已关闭") {
		t.Fatalf("persisted manual close must override PR state: label=%q reason=%q", merged.DeliveryLockedLabel(), merged.ExecutionBlockedReason())
	}
	merged.Status = "done"
	merged.DeliveryState = "reopened"
	if merged.DeliveryLocked() || merged.ExecutionBlockedReason() != "" {
		t.Fatal("explicitly reopened delivery must be triggerable")
	}
}
