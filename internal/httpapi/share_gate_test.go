package httpapi

import (
	"strings"
	"testing"
	"time"

	"github.com/Tencent/LoopForge/internal/domain"
)

func TestShareGateSignRoundTrip(t *testing.T) {
	secret := "test-share-secret"
	token, err := signShareGate(secret, shareGatePayload{
		RunID: "run-1", NodeID: "approve", Exp: time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	p, err := parseShareGate(secret, token)
	if err != nil {
		t.Fatal(err)
	}
	if p.RunID != "run-1" || p.NodeID != "approve" {
		t.Fatalf("unexpected payload %#v", p)
	}
}

func TestShareGateRejectsTamperAndExpiry(t *testing.T) {
	secret := "test-share-secret"
	token, err := signShareGate(secret, shareGatePayload{
		RunID: "run-1", NodeID: "approve", Exp: time.Now().Add(-time.Minute).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseShareGate(secret, token); err == nil {
		t.Fatal("expected expiry error")
	}
	fresh, err := signShareGate(secret, shareGatePayload{
		RunID: "run-1", NodeID: "approve", Exp: time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseShareGate("other-secret", fresh); err == nil {
		t.Fatal("expected signature error")
	}
}

func TestShareGatePageShowsRequirementAndUpstreamResult(t *testing.T) {
	run := domain.Run{
		WorkflowSnapshot: domain.WorkflowDefinition{
			Name:  "演示交付流程",
			Nodes: []domain.WorkflowNode{{ID: "analyze", Name: "需求分析"}, {ID: "approve", Name: "确认方案"}},
			Edges: []domain.WorkflowEdge{{Source: "analyze", Target: "approve"}},
		},
		Nodes: map[string]*domain.NodeRun{"analyze": {Output: "方案：<导出> CSV"}},
	}
	up := shareGateUpstream(run, "approve")
	if len(up) != 1 || up[0].Name != "需求分析" {
		t.Fatalf("upstream = %#v", up)
	}
	html := shareGateHTML(shareGateView{Title: "需求列表支持导出 CSV", Workflow: "演示交付流程", NodeName: "确认方案", Token: "t", Waiting: true, Upstream: up})
	for _, want := range []string{"需求列表支持导出 CSV", "流程：<b>演示交付流程</b>", "上游「需求分析」的结果", "方案：&lt;导出&gt; CSV"} {
		if !strings.Contains(html, want) {
			t.Fatalf("share page misses %q", want)
		}
	}
}
