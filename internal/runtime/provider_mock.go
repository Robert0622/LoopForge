package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Tencent/LoopForge/internal/domain"
)

type mockProvider struct{ baseProvider }

func (mockProvider) Kind() string { return "mock" }

func (mockProvider) Protocol(ex domain.ExecutorDefinition, sink EventSink) *protocolAdapter {
	return makeProtocolAdapter(ex, sink, protocolGeneric, "generic-json-v1")
}

func (mockProvider) RunBuiltin(ctx context.Context, agent domain.AgentDefinition, prompt string, sink EventSink) (string, bool, error) {
	sessionID := "mock-" + agent.ID
	for _, msg := range []string{"读取 Agent 定义与技能清单", "分析任务和约束", "生成结果并提交验证"} {
		select {
		case <-ctx.Done():
			return "", true, ctx.Err()
		case <-time.After(240 * time.Millisecond):
		}
		if sink != nil {
			sink(domain.TelemetryEvent{Category: "lifecycle", Name: "agent.progress", Phase: "progress", Provider: "mock", SessionID: sessionID, Message: msg})
		}
	}
	duty := strings.TrimSpace(agent.Instructions)
	if duty == "" {
		duty = strings.TrimSpace(agent.Description)
	}
	if duty == "" {
		duty = "按 SOP 完成本节点。"
	}
	return fmt.Sprintf("# %s（演示结果）\n\n> 这是演示执行器生成的示例内容，没有调用模型，也没有改动代码。配好模型 API Key 后换用真实 SOP（例如「快速修复」），Agent 会在你的仓库里真正执行。\n\n## 本节点职责\n\n%s\n\n## 示例过程\n\n- 读取 Agent 定义与技能清单\n- 分析任务和约束\n- 生成结果并提交验证\n\n## 平台下发给 Agent 的提示词（供参考）\n\n```text\n%s\n```\n", agent.Name, duty, strings.TrimSpace(prompt)), true, nil
}
