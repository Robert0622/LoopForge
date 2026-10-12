<p align="center">
  <img src="web/images/loopforge-logo.png" alt="LoopForge logo" width="96">
</p>

<h1 align="center">LoopForge</h1>

<p align="center">面向编码 Agent 的 SOP 交付流水线，腾讯开源。</p>

<p align="center">
  <a href="./LICENSE"><img alt="License" src="https://img.shields.io/badge/license-Apache--2.0-FF5A1F?labelColor=16161A&color=FF5A1F&style=flat-square"></a>
  <a href="https://github.com/Tencent/LoopForge/actions/workflows/platform.yml"><img alt="platform" src="https://img.shields.io/github/actions/workflow/status/Tencent/LoopForge/platform.yml?branch=main&label=platform&labelColor=16161A&style=flat-square"></a>
  <a href="https://github.com/Tencent/LoopForge/actions/workflows/validate.yml"><img alt="cli" src="https://img.shields.io/github/actions/workflow/status/Tencent/LoopForge/validate.yml?branch=main&label=cli&labelColor=16161A&style=flat-square"></a>
  <a href="https://www.npmjs.com/package/loopforge-cli"><img alt="npm loopforge-cli" src="https://img.shields.io/npm/v/loopforge-cli?label=npm&labelColor=16161A&color=FF5A1F&style=flat-square"></a>
  <img alt="Go 1.25+" src="https://img.shields.io/badge/go-1.25%2B-00ADD8?labelColor=16161A&style=flat-square">
</p>

<p align="center">
  <a href="./README.md">English</a> · <b>简体中文</b>
</p>

**用 SOP 驱动编码 Agent 交付。** 发布一次流程，之后每张需求都按同样的角色、关卡和交接走完，最后交出一个可以评审的 Pull Request。

## LoopForge 的两种用法

| | **LoopForge 平台**（仓库根目录） | **LoopForge CLI**（[`cli/`](cli/)） |
| --- | --- | --- |
| 是什么 | 自托管服务：Web 工作台、SOP 引擎、执行机器、REST 与 MCP 接口 | 安装进单个编码 Agent 的工作流 |
| 在哪里执行 | 你的服务器和绑定的机器，在沙箱里运行，团队共用 | 你本机的 Agent 会话里 |
| 适合 | 希望每张需求都按同一条发布过的流程走，并且需要审批、效能指标和 PR 交付的团队 | 想在自己的 Agent 里走「澄清 → 设计 → 实现 → 评审 → 测试」的个人开发者 |
| 开始 | `docker compose up -d`（见[快速开始](#快速开始)） | `npx -y loopforge-cli@latest install claude`（见 [cli/README.zh-CN.md](cli/README.zh-CN.md)） |

两者理念一致：先确认需求，再分角色推进，每个阶段都留下可检查的证据。两者相互独立，可以只用其中一个。本页后文介绍平台。

Claude Code、Codex、CodeBuddy 这类编码 Agent 做单个任务已经很好用。但团队里总有人在当**调度员**：开一个会话做分析，再开一个写代码，把结果贴到第三个会话里评审，最后还得自己确认测试是不是真过了、PR 有没有提。

LoopForge 平台把这套日常做法变成一条**发布过的 SOP**：由角色组成的 DAG，每一步声明要交出什么、哪些检查必须通过、没过打回到哪一步。你派发一张需求，流程在你自己的机器上、用你自己的 CLI 跑完；你只在选定的关口批准，最后评审 PR。

```
GitHub Issue ──► 需求定型 ──► 方案设计 ──► 实现 ──► 独立评审 ──► 测试 ──► 发起 PR ──► 交付汇总
                                  ▲          │         │          │
                                  └──── 带原因打回 ────┴──────────┘   （下游重置、重试）
```

## 亮点

**1. 派一次，流程自己往下走。** 依赖、并行、失败重试和打回目标都写在 SOP 里，不靠人记。每次运行锁定发布时的 SOP 快照，别人改画布不会让在途的单中途改道。

**2. 下一位拿到的是交付物，不是聊天记录。** 每个角色事先声明要交出的文件（方案、补丁、测试报告）。通过后按内容哈希（SHA-256）留存并保留全部历史尝试，下游在自己的输入目录里直接读到。打回重做不会覆盖上一轮证据。

**3. 没做完就是没做完。** Agent 结束时必须给出明确结论（通过 / 返工 / 失败 / 需要人处理）和原因。测试、检查脚本等命令门禁先跑，人才能批准。打回会回到指定步骤、清掉下游进度，并把原因带进下一轮。

**4. 需求和结果留在团队原来的地方。** GitHub Issue 双向同步（拉取、推送、定时自动同步），交付走集成分支和 Pull Request，PR 合入后自动闭环。前端项目还能在平台上打开构建出的**预览页面**，查看时把接口指向任意后端。

**5. 角色写一次，多条流程共用。** 角色上放着职责、规则、技能、MCP 工具、自检循环和本角色的完成标准；SOP 只选择哪一步用哪个角色。评审标准改一处，所有引用它的流程一起生效；不同步骤可以用不同的 CLI 和模型。

**6. 在自己的机器上跑，用自己的登录。** 一条命令绑定笔记本或云主机，你负责的运行都在那台机器上执行，可以用本机已登录的 CLI，也可以用你自己保存的 Key。任务结束清除注入的密钥和 Git 令牌。沙箱支持 Docker、Linux bubblewrap、macOS Seatbelt。

**7. 在原来的工具里派发和放行。** 所有能力同时提供 REST（OpenAPI 3.1）和 **MCP Server**，Cursor、Claude Code、Codex、CodeBuddy 可以直接建项目、写角色、发布 SOP、派发需求、审批运行。内置助手能按对话起草角色和流程，保存前请人确认。

**8. 看得懂流程健不健康，而不只是 Token 用量。** 单次运行能看模型在做什么、调用了哪些工具、Token 与成本、门禁结果和产物（与模型厂商无关的 SSE 遥测，支持 OTLP 导出）。汇总能看闭环率、一次通过率、返工率、各阶段耗时和每个节点的可靠度：该改的是哪一步，而不是再写一段更长的提示词。

## 和其他项目的区别

LoopForge 平台**不是**又一个编码 Agent，也不是通用自动化工具。它在 Agent 之上一层，让团队的交付流程可重复。

| | LoopForge 平台 | Agent 同事看板（如 [Multica](https://github.com/multica-ai/multica)） | 编码 Agent 控制台（如 [OpenHands](https://github.com/OpenHands/OpenHands)） | LLM 应用 / 自动化平台（如 [Dify](https://github.com/langgenius/dify)、[Langflow](https://github.com/langflow-ai/langflow)、[n8n](https://github.com/n8n-io/n8n)） | 多 Agent 框架（如 [CrewAI](https://github.com/crewAIInc/crewAI)、[AutoGen](https://github.com/microsoft/autogen)、[MetaGPT](https://github.com/FoundationAgents/MetaGPT)） |
| --- | --- | --- | --- | --- | --- |
| 工作单元 | 每张需求都按同一条发布过的 SOP 走 | 把 Issue 派给一个 Agent 或一个小队 | 把任务交给一个 Agent | 聊天应用或集成流程 | 一段 Python 程序 |
| 谁决定下一步给谁 | DAG（确定、可版本化） | 队长 Agent 每次用评论 @ 下一位 | 你自己 | 通用节点 | 你的代码 |
| 步骤之间怎么交接 | 声明过、带哈希的产物 | Issue 评论 | 无 | 节点输出 | 内存消息 |
| 什么叫做完 | 命令门禁 + 明确结论 + 指定关口人工批准；打回重置下游 | 运行结束，由人关单 | Agent 说做完了 | 流程跑完 | 程序返回 |
| 最后评审什么 | PR、产物、预览页面、交付阶段 | Issue 时间线和 diff | diff / PR | 聊天回答 / API 调用 | 生成的文件 |
| 在哪里执行 | 你的机器和 CLI，带沙箱 | 你的机器 | 托管或自托管运行时 | 服务端 | 你运行 Python 的地方 |
| 适合的单子 | 可重复的路径：修 bug、功能交付、评审、发布 | 临时、事先不知道该派给谁的任务 | 一次性的编码会话 | 业务自动化、RAG 应用 | 研究与自定义 Agent 程序 |

它们可以叠在一起用：零散任务交给 Agent 看板，路径已经说得清的交付交给 LoopForge 平台。平台刻意**不**自研编码 Agent、知识库或连接器市场：执行用你已有的 CLI，对接其他系统走 MCP。

## 快速开始

### Docker

```bash
git clone https://github.com/Tencent/LoopForge.git && cd LoopForge
docker compose pull    # 预构建镜像（linux/amd64、linux/arm64）
docker compose up -d
docker compose logs loopforge | grep "Created admin account"   # 首次启动打印的管理员密码
```

打开 <http://localhost:8080> 登录（端口被占用时：`LOOPFORGE_PORT=9090 docker compose up -d`）。镜像内置 Claude Code、Codex、CodeBuddy CLI，在「个人设置」里填上你的 API Key 即可。

不执行 `docker compose pull`、或者镜像拉不下来时，`up` 会改为从源码构建，需要几分钟。`docker compose up -d --build` 总是从源码构建；`LOOPFORGE_VERSION=0.1.0` 可以固定版本。

### 没有 API Key 也能先体验

```bash
LOOPFORGE_DEMO_MODE=true docker compose up -d    # 或者：make demo
```

演示模式会写入一个脚本化执行器和「演示交付流程」SOP（分析 → 人工审批 → 实现 → 报告），并设为默认项目 SOP。在工作总览新建需求、触发、审批通过，然后在「产物」里打开生成的 `delivery-report.md`。全程不调用任何模型。内置模板依然可用，但需要真实的 API Key。

### 从源码运行

需要 Go 1.25+ 和 Git。

```bash
make run        # SQLite 存在 ~/.loopforge，监听 :8080
```

首次启动会创建 `admin` 账号（密码只在日志里打印一次，也可以用 `LOOPFORGE_ADMIN_PASSWORD` 指定）、一个默认项目、Claude Code / Codex / CodeBuddy 执行器、local / bubblewrap / Seatbelt / Docker 沙箱，以及三套内置 SOP 模板。模板使用主机上可用的最强沙箱（依次尝试 Docker、bubblewrap、Seatbelt、本地进程；启动日志会打印选择结果，可用 `LOOPFORGE_TEMPLATE_SANDBOX` 覆盖）：

| 模板 | 步骤 |
| --- | --- |
| 通用后端交付 | 需求定型 → 方案设计 → 实现与自测 → 独立评审 → 测试验证 → 发起 PR → 交付汇总 |
| 通用前端交付 | 需求定型 → 方案设计 → 实现 → 独立评审 → 验证 → 发起 PR → 归档汇总 |
| 通用产品交付 | 产品定义 → 交互 Demo → 研发交接 → 产品验收 → Issue 回写 → 交付汇总 |

### 接入 GitHub

1. 在「个人设置」里填 GitHub Token（classic 需 `repo` 权限；fine-grained 需 *Contents*、*Issues*、*Pull requests* 读写），使用 GitHub MCP 工具还需要填个人 MCP 令牌。
2. 在项目里绑定仓库（粘贴仓库地址即可）。
3. 按 URL 导入 Issue 或开启自动同步，然后派发到 SOP。

GitHub Enterprise Server：设置 `LOOPFORGE_GITHUB_API_URL=https://<host>/api/v3` 和 `LOOPFORGE_GITHUB_HOST=<host>`。

## 核心概念

| 概念 | 说明 |
| --- | --- |
| **项目** | 需求、仓库、成员和默认 SOP 的边界。 |
| **需求 / Issue** | 本地创建或从 GitHub 同步；新建和导入的需求会按项目澄清策略打分后再派发。 |
| **SOP（流程）** | 由 Agent、脚本、人工审批节点组成的版本化 DAG，带门禁、重试和打回目标。 |
| **角色** | 与执行器无关的定义：指令、规则、技能、MCP、Hook、自检循环、产物和门禁。 |
| **运行** | 一次冻结 SOP 快照的执行，共享一个沙箱/工作区，带遥测和产物。 |
| **执行机器** | 绑定的机器，执行其主人负责的运行。 |

## 架构

单个 Go 二进制内嵌 Web 界面（原生 JavaScript，无构建步骤）。默认用 SQLite，多实例部署用 MySQL，两者走同一套 gorm 代码。

```
cmd/loopforge          服务端（API + 界面 + 调度 + Worker；--role api|worker|all）
cmd/loopforge-runner   执行机器守护进程
cmd/loopforge-mcp      连接 REST API 的 stdio MCP 桥
cmd/loopforge-gate-mcp 注入 Agent 会话的门禁结果工具
internal/engine        DAG 调度、门禁、打回、交付、集成分支
internal/runtime       执行器、沙箱、MCP 物化、遥测适配
internal/tracker       GitHub Issue / Pull Request 适配
internal/httpapi       REST、认证、MCP 网关
internal/repo          存储（SQLite / MySQL）、内置模板
web/                   内嵌的 Studio 界面
```

更多文档：[架构](docs/architecture.md) · [REST API](docs/api/README.md) · [OpenAPI](docs/api/openapi.yaml) · [MCP Server](docs/mcp.md) · [可观测性](docs/observability.md) · [AI 助手](docs/assistant.md)。

## 配置

全部可选，完整列表见 [.env.example](.env.example)。常用项：

| 变量 | 作用 |
| --- | --- |
| `LOOPFORGE_DATA_DIR` | 数据目录（SQLite、会话密钥、工作区），默认 `~/.loopforge`。 |
| `LOOPFORGE_DATABASE_URL` | `sqlite:///path/loopforge.db` 或 `mysql://user:pass@tcp(host:3306)/loopforge`。 |
| `LOOPFORGE_ADMIN_USER` / `LOOPFORGE_ADMIN_PASSWORD` | 首次启动创建的管理员。 |
| `LOOPFORGE_GITHUB_TOKEN` | 服务端兜底令牌，用于 Issue 同步和 PR 交付。 |
| `LOOPFORGE_TRUST_IDENTITY_HEADERS` | 信任反向代理（oauth2-proxy、Authelia 等）注入的 `X-Forwarded-User`。 |
| `LOOPFORGE_ROLE` | `all`（默认）、`api` 或 `worker`，用于拆分部署。 |
| `LOOPFORGE_DEMO_MODE` | 写入演示执行器和演示 SOP（见「没有 API Key 也能先体验」）。 |
| `LOOPFORGE_TEMPLATE_SANDBOX` | 内置模板使用的沙箱：`generic-docker`、`linux-bwrap`、`macos-seatbelt` 或 `local`。不设置时自动检测。 |
| `LOOPFORGE_MCP_MARKET` / `LOOPFORGE_TEMPLATES` / `LOOPFORGE_WEB_PLUGINS` | 从文件、目录或 URL 加载自己的 MCP 市场条目、SOP 模板和界面扩展页面，见 [docs/extending.md](docs/extending.md)。 |

默认使用本地账号（bcrypt）登录；需要 SSO 时放在身份感知的反向代理后面。REST 和 MCP 客户端使用在「接入指南 → 我的 Token」申请的个人令牌（`Authorization: Bearer afp_...`）。

## 执行机器：在自己的电脑上跑

在「个人 → 执行机器」复制绑定命令，到目标机器上执行：

```bash
curl -fsSL https://loopforge.example.com/runners/install.sh | bash -s -- \
  --server loopforge.example.com --token bind-xxxx --service
```

安装脚本从你的服务端下载 Runner（不需要 Go），自动发现已安装的 CLI 并注册机器。设为默认执行机器后，你负责的运行的每个节点都在那台机器上执行：检出仓库、沙箱、Agent CLI 和门禁。任务状态存在数据库里，休眠、崩溃、断网都不会丢；租约到期按策略重新排队。Runner 空闲时自动升级。

## 安全

平台会让 Agent 在真实仓库上执行命令。`local` 沙箱只是开发便利，不是隔离边界；Seatbelt、bubblewrap 和 Docker 适合在可信主机上无人值守运行，但不是对抗性的多租户隔离。部署建议和漏洞报告方式见 [SECURITY.md](SECURITY.md)。

## 现状与规划

已知限制：

- Studio 界面目前是中文，英文界面在规划中。
- SOP 运行之外的 AI 功能（需求澄清、内置助手、运行评估）使用 CodeBuddy，需要在「个人设置」里配置 CodeBuddy API Key。没有 Key 时，新建需求请选「跳过分析并建单」。
- 在 Docker 镜像内，模板会回退到 `local` 沙箱（容器里没有嵌套 Docker，也无法创建 user namespace）。需要隔离时，绑定一台装有 Docker 或 bubblewrap 的执行机器并在「流程设置」里切换沙箱，或者直接在这样的主机上运行二进制。
- 同一个数据库只应有一个实例执行运行（只读副本设置 `LOOPFORGE_EXECUTE_RUNS=false`）。
- 执行器目前支持 Claude Code、Codex、CodeBuddy；更多 CLI（包括 ACP 兼容 Agent）在规划中。
- 产物存在数据库里；对象存储后端在规划中。
- 需求系统目前是 GitHub；GitLab、Gitea、Jira 需要在本仓库内实现 Provider（见 [docs/extending.md](docs/extending.md#issue-trackers)），欢迎贡献。

## 参与贡献

欢迎贡献！请先阅读 [CONTRIBUTING.md](CONTRIBUTING.md) 和 [AGENTS.md](AGENTS.md)（代码分层约定），并遵守 [行为准则](CODE_OF_CONDUCT.md)。修改 CLI 及其工作流请遵循 [cli/CONTRIBUTING.zh-CN.md](cli/CONTRIBUTING.zh-CN.md) 和 [cli/AGENTS.md](cli/AGENTS.md)。

## 许可证

Apache License 2.0。Copyright (C) 2026 Tencent。`cli/` 下的 CLI 仍采用 [MIT 许可证](cli/LICENSE)。随附的第三方软件见 [LICENSE](LICENSE) 和 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。
