package docs

import (
	_ "embed"
	"fmt"
	"strings"
	"testing"

	"github.com/Tencent/LoopForge/internal/application"
)

//go:embed mcp.md
var mcpGuide string

func TestMCPGuideListsEveryPublicTool(t *testing.T) {
	for _, op := range application.PublicOperations() {
		if !strings.Contains(mcpGuide, "`"+op.Name+"`") {
			t.Errorf("MCP guide does not list tool %s", op.Name)
		}
	}
	want := fmt.Sprintf("（%d 个工具）", len(application.PublicOperations()))
	if !strings.Contains(mcpGuide, want) {
		t.Errorf("MCP guide does not contain current tool count %s", want)
	}
}
