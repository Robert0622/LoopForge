package domain

import (
	"strings"
	"testing"
)

func TestParseForgeMR(t *testing.T) {
	project, iid, url, ok := ParseForgeMR("PR URL: https://github.com/acme/widget/pull/138\nHEAD abc")
	if !ok || project != "acme/widget" || iid != 138 || !strings.Contains(url, "pull/138") {
		t.Fatalf("got project=%q iid=%d url=%q ok=%v", project, iid, url, ok)
	}
	project, iid, _, ok = ParseForgeMR("https://github.com/x/y/pull/1")
	if !ok || project != "x/y" || iid != 1 {
		t.Fatalf("short form project=%q iid=%d ok=%v", project, iid, ok)
	}
	if _, _, _, ok = ParseForgeMR("no mr here"); ok {
		t.Fatal("expected no match")
	}
}

func TestSameForgeMRIgnoresURLForm(t *testing.T) {
	a := "https://github.com/acme/widget/pull/248"
	b := "https://github.com/acme/widget/pull/248/files"
	if !SameForgeMR(a, b) {
		t.Fatal("expected PR sub pages to match the PR")
	}
	if SameForgeMR(a, "https://github.com/acme/widget/pull/183") {
		t.Fatal("different IIDs must not match")
	}
}

func TestParseForgeMRsKeepsOrderAndDedups(t *testing.T) {
	refs := ParseForgeMRs("-https://github.com/org/repo/pull/282\n+https://github.com/org/repo/pull/286\nhttps://github.com/org/repo/pull/282")
	if len(refs) != 2 || refs[0].IID != 282 || refs[1].IID != 286 {
		t.Fatalf("refs=%+v", refs)
	}
}

func TestMRArtifactPriorityIgnoresPatches(t *testing.T) {
	if got := MRArtifactPriority("repository-patch", "", "mr"); got != 0 {
		t.Fatalf("patch priority=%d", got)
	}
	if got := MRArtifactPriority("mr-body", "", "mr"); got <= MRArtifactPriority("closeout", "", "close") {
		t.Fatalf("mr-body should outrank closeout: %d", got)
	}
	if got := MRArtifactPriority("notes", "", "verify"); got != 0 {
		t.Fatalf("unrelated artifact priority=%d", got)
	}
}
