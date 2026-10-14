package httpapi

import (
	"strings"
	"testing"

	"github.com/Tencent/LoopForge/internal/domain"
)

func TestCrewPlanRulesSplitsRolesByName(t *testing.T) {
	roster := []domain.AgentDefinition{
		{ID: "demo-developer", Name: "演示 · 开发实现", Description: "演示执行器：模拟按方案完成实现", ExecutorID: "mock"},
		{ID: "demo-analyst", Name: "演示 · 需求分析", Description: "演示执行器：模拟拆解需求并给出方案", ExecutorID: "mock"},
		{ID: "reviewer", Name: "Reviewer", Description: "Independently reviews the change", ExecutorID: "claude"},
	}
	tasks := crewPlanRules("调研负责人筛选", roster)
	got := []string{}
	for _, task := range tasks {
		got = append(got, task.AgentID)
	}
	want := []string{"demo-analyst", "demo-developer", "reviewer"}
	if len(got) != len(want) {
		t.Fatalf("plan = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("plan = %v, want %v", got, want)
		}
	}
	if tasks[0].PlanReason == "" || !strings.Contains(tasks[0].PlanReason, "拆解需求") {
		t.Fatalf("plan reason = %q", tasks[0].PlanReason)
	}
}

func TestCrewQuestionFindsAsk(t *testing.T) {
	if q := crewQuestion("结论\n\n需要确认：用哪个字段当负责人？"); q != "用哪个字段当负责人？" {
		t.Fatalf("question = %q", q)
	}
	if q := crewQuestion("全部完成"); q != "" {
		t.Fatalf("unexpected question %q", q)
	}
}

func TestCrewWantsAnswerSplitsQuestionsFromWork(t *testing.T) {
	for text, want := range map[string]bool{
		"现在进度怎么样":        true,
		"这个方案为什么要拆两步？":   true,
		"总结一下刚才的讨论":      true,
		"你好":             true,
		"调研一下需求列表加负责人筛选": false,
		"帮我修复登录页白屏的问题吗？": false,
		"给导出功能写一份发布公告":   false,
		"整理一下导出的已知问题":    false,
		"":               false,
	} {
		if got := crewWantsAnswer(text); got != want {
			t.Errorf("crewWantsAnswer(%q) = %v, want %v", text, got, want)
		}
	}
}
