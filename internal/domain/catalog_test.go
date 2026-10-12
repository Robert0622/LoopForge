package domain

import "testing"

func TestCatalogVisibility(t *testing.T) {
	if !CatalogOwned("shop", "shop") {
		t.Fatal("owned item should match its project")
	}
	if CatalogOwned("", "shop") {
		t.Fatal("legacy global item is not owned by a project")
	}
	if !CatalogSharedWith("", false, "shop") {
		t.Fatal("legacy global item should appear as shared")
	}
	if CatalogSharedWith("shop", true, "shop") {
		t.Fatal("an owned item stays in the project tab even when shared")
	}
	if !CatalogSharedWith("shop", true, "igtm") {
		t.Fatal("shared item should appear in another project's shared tab")
	}
	if CatalogSharedWith("shop", false, "igtm") {
		t.Fatal("private item must not leak to another project")
	}
	if !CatalogVisible("shop", false, "shop") || CatalogVisible("shop", false, "igtm") {
		t.Fatal("private catalog visibility is wrong")
	}
	if !AssetVisible("", "shop") || !AssetVisible("shop", "shop") || AssetVisible("igtm", "shop") {
		t.Fatal("asset visibility is wrong")
	}
	if got := RunProjectID(Run{ExecutionContext: TelemetryContext{ProjectID: "shop"}}, "other"); got != "shop" {
		t.Fatalf("run project = %q", got)
	}
	if got := RunProjectID(Run{}, "shop"); got != "shop" {
		t.Fatalf("issue-linked run project = %q", got)
	}
	if got := KeepCatalogProjectID("", "shop"); got != "shop" {
		t.Fatalf("empty write should keep existing project: %q", got)
	}
	if got := KeepCatalogProjectID("igtm", "shop"); got != "igtm" {
		t.Fatalf("explicit project should win: %q", got)
	}
}
