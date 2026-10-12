package runtime

import (
	"context"
	"fmt"
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
	return fmt.Sprintf("%s 已完成任务：%s", agent.Name, prompt), true, nil
}
