package httpapi

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	goruntime "runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Tencent/LoopForge/internal/domain"
	"github.com/Tencent/LoopForge/internal/repo"
	"github.com/Tencent/LoopForge/internal/runtime"
	"github.com/gin-gonic/gin"
)

// doctorCheck is one readiness item. Status is ok, warn, error, or info;
// Action names a UI affordance that fixes it (setup-wizard, prewarm-sandbox,
// git-token).
type doctorCheck struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Action string `json:"action,omitempty"`
}

type doctorReport struct {
	Ready   bool                        `json:"ready"`
	Sandbox string                      `json:"sandbox"`
	Prewarm runtime.DockerPrewarmStatus `json:"prewarm"`
	Checks  []doctorCheck               `json:"checks"`
}

type doctorHandlers struct {
	*deps
}

// 路由: g.GET("/api/doctor", h.report)
func (h *doctorHandlers) report(c *gin.Context) {
	current, _ := h.requestUser(c.Request, true)
	var user domain.User
	if current != nil && h.repo != nil {
		user, _ = h.repo.User(current.ID)
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	c.JSON(http.StatusOK, h.build(ctx, user))
}

// 路由: g.POST("/api/doctor/prewarm-sandbox", h.prewarm)
func (h *doctorHandlers) prewarm(c *gin.Context) {
	if !h.isPlatformAdmin(c.Request) {
		c.JSON(http.StatusForbidden, map[string]string{"error": "只有平台管理员可以预构建沙箱镜像"})
		return
	}
	if !dockerReachable(c.Request.Context()) {
		c.JSON(http.StatusConflict, map[string]string{"error": "当前环境连不上 Docker，无法构建沙箱镜像"})
		return
	}
	started := runtime.StartDockerPrewarm(h.sandboxDockerfile(), 45*time.Minute)
	c.JSON(http.StatusAccepted, map[string]any{"started": started, "prewarm": runtime.DockerPrewarm()})
}

func (h *doctorHandlers) sandboxDockerfile() domain.DockerfileDefinition {
	if h.repo != nil {
		if stored, ok := h.repo.Dockerfile(repo.GenericBackendDockerfileID); ok && strings.TrimSpace(stored.Content) != "" {
			return stored
		}
	}
	return repo.GenericBackendDockerfile()
}

func (h *doctorHandlers) build(ctx context.Context, user domain.User) doctorReport {
	sandbox := repo.TemplateSandbox()
	if sandbox == "" {
		sandbox = repo.GenericDockerSandboxID
	}
	report := doctorReport{Sandbox: sandbox, Prewarm: runtime.DockerPrewarm()}
	add := func(check doctorCheck) { report.Checks = append(report.Checks, check) }

	docker := dockerReachable(ctx)
	switch {
	case sandbox == "generic-docker" && docker:
		add(doctorCheck{ID: "sandbox", Label: "运行隔离", Status: "ok", Detail: "每个 Run 在独立 Docker 容器里执行，结束即销毁。"})
	case sandbox == "generic-docker":
		add(doctorCheck{ID: "sandbox", Label: "运行隔离", Status: "error", Detail: "内置 SOP 配置为 Docker 沙箱，但当前连不上 Docker：检查 /var/run/docker.sock 是否挂载、Docker 是否在运行。"})
	case sandbox == "local":
		add(doctorCheck{ID: "sandbox", Label: "运行隔离", Status: "warn", Detail: "Run 以进程方式直接执行（无容器隔离）。需要每个 Run 独立容器时，用 docker compose 启动并挂载 docker.sock。"})
	default:
		add(doctorCheck{ID: "sandbox", Label: "运行隔离", Status: "ok", Detail: "内置 SOP 使用 " + sandbox + " 沙箱。"})
	}

	if sandbox == "generic-docker" && docker {
		image := doctorCheck{ID: "sandbox-image", Label: "沙箱镜像"}
		switch {
		case runtime.DockerImageCached(ctx, h.sandboxDockerfile()):
			image.Status, image.Detail = "ok", "已构建，真实 Run 可以直接启动。"
		case report.Prewarm.State == "building":
			image.Status, image.Detail = "info", "正在后台构建（通常需要几分钟），完成后真实 Run 可以直接启动。"
		case report.Prewarm.State == "failed":
			image.Status, image.Detail, image.Action = "warn", "上次预构建失败："+report.Prewarm.Error, "prewarm-sandbox"
		default:
			image.Status, image.Detail, image.Action = "warn", "尚未构建：第一次真实 Run 会先构建镜像，需要几分钟。可以现在预构建。", "prewarm-sandbox"
		}
		add(image)
	}

	gate := doctorCheck{ID: "gate-helper", Label: "审批节点组件", Status: "ok", Detail: "带审批节点的 Agent Run 可以正常挂起与恢复。"}
	targets := []string{"linux"}
	if goruntime.GOOS != "linux" {
		targets = append(targets, goruntime.GOOS)
	}
	for _, targetOS := range targets {
		if err := runtime.CheckGateMCPBinary(targetOS, goruntime.GOARCH); err != nil {
			gate.Status, gate.Detail = "error", err.Error()
			break
		}
	}
	add(gate)

	modelKeys := []string{}
	if strings.TrimSpace(user.ClaudeCodeAPIKey) != "" {
		modelKeys = append(modelKeys, "Claude Code")
	}
	if strings.TrimSpace(user.CodexAPIKey) != "" {
		modelKeys = append(modelKeys, "Codex")
	}
	if strings.TrimSpace(user.CodeBuddyAPIKey) != "" {
		modelKeys = append(modelKeys, "CodeBuddy")
	}
	if len(modelKeys) > 0 {
		add(doctorCheck{ID: "model-key", Label: "模型 Key", Status: "ok", Detail: "已配置：" + strings.Join(modelKeys, "、") + "。"})
	} else {
		add(doctorCheck{ID: "model-key", Label: "模型 Key", Status: "warn", Detail: "还没配置任何 Agent 的 Key：一键演示可以直接跑，真实需求需要至少一个 Key。", Action: "setup-wizard"})
	}

	if strings.TrimSpace(user.GitToken) != "" {
		add(doctorCheck{ID: "git-token", Label: "Git 凭据", Status: "ok", Detail: "已配置，交付节点可以推分支、开 PR。"})
	} else {
		add(doctorCheck{ID: "git-token", Label: "Git 凭据", Status: "info", Detail: "未配置：可以先跑不需要推代码的 SOP；要自动开 PR 时再到「账号与凭据」填写。", Action: "git-token"})
	}

	if sandbox != "generic-docker" {
		var found []string
		for _, bin := range []string{"claude", "codex", "codebuddy"} {
			if _, err := exec.LookPath(bin); err == nil {
				found = append(found, bin)
			}
		}
		if len(found) > 0 {
			add(doctorCheck{ID: "agent-cli", Label: "Agent CLI", Status: "ok", Detail: "本机可用：" + strings.Join(found, "、") + "。"})
		} else {
			add(doctorCheck{ID: "agent-cli", Label: "Agent CLI", Status: "warn", Detail: "本机没找到 claude / codex / codebuddy：真实 Run 需要至少安装其中一个，或改用 Docker 启动（镜像内已预装）。"})
		}
	}

	if demo, _ := strconv.ParseBool(strings.TrimSpace(os.Getenv("LOOPFORGE_DEMO_MODE"))); demo {
		add(doctorCheck{ID: "demo", Label: "演示模式", Status: "info", Detail: "已开启：「一键演示」SOP 不需要任何 Key 就能跑完整条流程。"})
	}
	if strings.TrimSpace(os.Getenv("LOOPFORGE_NOTIFY_WEBHOOK")) == "" {
		add(doctorCheck{ID: "notify", Label: "待审批通知", Status: "info", Detail: "未配置 LOOPFORGE_NOTIFY_WEBHOOK：待审批与失败只在页面里提示，不推送到群聊。"})
	} else {
		add(doctorCheck{ID: "notify", Label: "待审批通知", Status: "ok", Detail: "待审批与失败会推送到群聊机器人。"})
	}

	report.Ready = true
	for _, check := range report.Checks {
		if check.Status == "error" {
			report.Ready = false
		}
	}
	return report
}

func dockerReachable(ctx context.Context) bool {
	bin, err := exec.LookPath("docker")
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, bin, "info", "--format", "{{.ServerVersion}}").Run() == nil
}
