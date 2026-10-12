<p align="center">
  <img src="web/images/loopforge-logo.png" alt="LoopForge logo" width="96">
</p>

<h1 align="center">LoopForge</h1>

<p align="center"><b>需求的全自动 Loop：从一张需求到一个可合并的 Pull Request。</b><br>编码 Agent 按团队 SOP 跑完全程，人在环上（Human on the Loop），而不是卡在环里。</p>

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

把一张需求交给 LoopForge，编码 Agent 会按团队的 SOP 把它走完：澄清、设计、实现、评审、测试、提 Pull Request，最后交回一个可以合并的结果。每一步是否真的通过由关卡判定，没通过就带着原因打回到对应步骤重做。哪里必须有人签字由你决定，其余环节自动跑下去，这就是 **Human on the Loop**。

LoopForge 源自腾讯团队的研发 SOP，腾讯内部已有多个项目团队在使用，包括 SkillHub，以及一些对质量要求很高的项目团队。

<p align="center">
  <img src="docs/images/run.png" alt="LoopForge 的一次运行：SOP 各步骤均已完成，附带产物、遥测和评测" width="100%">
</p>

## 三个核心亮点

### 1. 需求的全自动 Loop

- **需求自己进来**：GitHub Issue 定时同步，每张新需求按项目的澄清标准打分，不达标的退回澄清，不会直接交给编码 Agent。
- **派发不需要人**：定时触发任务扫描所有就绪需求并启动 SOP；集成分支冲突由对齐 Agent 解决；同一张需求不会被重复运行。
- **“做完”要验证，不靠自称**：每一步都以明确结论结束（通过、返工、失败、需要人工）；测试、Lint、脚本等命令门禁必须通过；打回会回到指定步骤，重置下游，并把原因带进下一次尝试。
- **每次运行一个独立容器**：每张需求都在一个全新的容器里运行，有自己的工作区，运行结束后删除；用 `docker compose` 部署即可开箱使用。不同运行之间看不到彼此的文件和进程。
- **交付自动闭环**：产出落到集成分支并提交 Pull Request，PR 合并后需求自动关闭。
- **人只在你需要的地方出现**：人工审批是 SOP 里的可选节点，可以只在方案设计处签字，也可以全程无人值守。

### 2. 可观测、可评测，面向自进化

- **每次运行实时可见**：模型活动、工具调用、Token 与成本、门禁结果和产物实时推送；遥测与执行器无关，并支持 OTLP 导出。
- **度量流程，而不只是 Token**：闭环率、一次通过率、返工率、各阶段耗时和每个节点的稳定性，告诉你该修的是哪一步。
- **每次运行都有评测**：评测给运行打分，并提出具体修改建议，比如调整某个 SOP 节点、增删门禁、收紧某个角色的指令；每条建议都可以一键应用。
- **规划中：自反馈闭环**：评测结果自动回流到角色和 SOP，让流水线从自己的运行中持续改进，见[规划](docs/roadmap.zh-CN.md)。

### 3. 上下文共享，专家经验沉淀为 SOP

- **SOP 承载专家的做法**：内置的后端、前端、产品交付 SOP 把设计、开发、评审、测试、产品之间怎么交接固化下来；发布你自己的 SOP，之后每张需求都按它走。
- **角色写一次，到处复用**：角色打包了指令、规则、技能、MCP 工具、自检循环和自己的完成标准；评审标准改一处，所有用到它的 SOP 都跟着变；不同步骤可以用不同的 CLI 和模型。
- **上下文共享，不用反复交代**：每一步交出声明过的产物（设计文档、补丁、测试报告），按内容哈希存储并保留全部尝试历史，下一个角色从真实交付物开始，而不是从聊天记录开始。
- **知识持续沉淀**：角色可以通过 MCP 检索团队知识库并引用；新发现先进入审核，再写回知识库。

### 其他能力

- **在你的机器上、用你的登录态执行**：一条命令绑定笔记本或云主机，直接用你已经登录的 CLI；任务结束后清理注入的密钥和 Git 令牌。沙箱支持 Docker、Linux bubblewrap、macOS Seatbelt。
- **从你自己的工具里驱动**：所有能力都提供 REST（OpenAPI 3.1）接口，也是一个 MCP Server，可接入 Cursor、Claude Code、Codex、CodeBuddy；内置 AI 助手可以通过对话起草角色和 SOP。
- **预览**：前端运行会发布构建后站点的在线预览，API 请求可转发到任意后端。
- **SOP 版本固定**：每次运行冻结在启动时的 SOP 版本上，修改画布不会改变正在进行的工作。

```
GitHub Issue ──► 澄清 ──► 设计 ──► 实现 ──► 评审 ──► 测试 ──► 提 PR ──► 总结
                           ▲         │        │        │
                           └──── 带原因打回 ──┴────────┘   （重置下游，重试）
```

## LoopForge 的两种用法

| | **LoopForge 平台**（仓库根目录） | **LoopForge CLI**（[`cli/`](cli/)） |
| --- | --- | --- |
| 是什么 | 自托管服务：Web 工作台、SOP 引擎、执行机器、REST 与 MCP 接口 | 安装进单个编码 Agent 的工作流 |
| 在哪里执行 | 你的服务器和绑定的机器，在沙箱里运行，团队共用 | 你本机的 Agent 会话里 |
| 适合 | 希望每张需求都按同一条发布过的流程走，并且需要审批、效能指标和 PR 交付的团队 | 想在自己的 Agent 里走「澄清 → 设计 → 实现 → 评审 → 测试」的个人开发者 |
| 开始 | `docker compose up -d`（见[快速开始](#快速开始)） | `npx -y loopforge-cli@latest install claude`（见 [cli/README.zh-CN.md](cli/README.zh-CN.md)） |

平台是 CLI 工作流的部署版。两者理念相同：先确认需求、分离角色、每个阶段都留下可评审的证据。CLI 在单个开发者的 Agent 里运行，不需要部署；平台把同一套流程交给整个团队，无人值守地跑。两者都可以单独使用。本页后面介绍的是平台。

## 和其他项目的区别

LoopForge **不是**又一个编码 Agent，也不是通用自动化工具。它在 Agent 之上一层，让团队的交付流程可重复。

| | LoopForge 平台 | Agent 同事看板（如 [Multica](https://github.com/multica-ai/multica)） | 编码 Agent 控制台（如 [OpenHands](https://github.com/OpenHands/OpenHands)） | LLM 应用 / 自动化平台（如 [Dify](https://github.com/langgenius/dify)、[Langflow](https://github.com/langflow-ai/langflow)、[n8n](https://github.com/n8n-io/n8n)） | 多 Agent 框架（如 [CrewAI](https://github.com/crewAIInc/crewAI)、[AutoGen](https://github.com/microsoft/autogen)、[MetaGPT](https://github.com/FoundationAgents/MetaGPT)） |
| --- | --- | --- | --- | --- | --- |
| 工作单元 | 每张需求都按同一条发布过的 SOP 走 | 把 Issue 派给一个 Agent 或一个小队 | 把任务交给一个 Agent | 聊天应用或集成流程 | 一段 Python 程序 |
| 谁决定下一步给谁 | DAG（确定、可版本化） | 队长 Agent 每次用评论 @ 下一位 | 你自己 | 通用节点 | 你的代码 |
| 步骤之间怎么交接 | 声明过、带哈希的产物 | Issue 评论 | 无 | 节点输出 | 内存消息 |
| 什么叫做完 | 命令门禁 + 明确结论 + 指定关口人工批准；打回重置下游 | 运行结束，由人关单 | Agent 说做完了 | 流程跑完 | 程序返回 |
| 最后评审什么 | PR、产物、预览页面、交付阶段 | Issue 时间线和 diff | diff / PR | 聊天回答 / API 调用 | 生成的文件 |
| 在哪里执行 | 你的机器和 CLI，带沙箱 | 你的机器 | 托管或自托管运行时 | 服务端 | 你运行 Python 的地方 |
| 适合的单子 | 可重复的路径：修 bug、功能交付、评审、发布 | 临时、事先不知道该派给谁的任务 | 一次性的编码会话 | 业务自动化、RAG 应用 | 研究与自定义 Agent 程序 |

它们可以叠在一起用：零散任务交给 Agent 看板，路径已经说得清的交付交给 LoopForge。LoopForge 刻意**不**自研编码 Agent 或连接器市场：执行用你已有的 CLI，对接其他系统走 MCP。

## 快速开始

### 五分钟体验，不需要 API Key

```bash
git clone https://github.com/Tencent/LoopForge.git && cd LoopForge
docker compose pull    # 预构建镜像（linux/amd64、linux/arm64）
LOOPFORGE_DEMO_MODE=true docker compose up -d
docker compose logs loopforge | grep "Created admin account"   # 一次性管理员密码
```

打开 <http://localhost:8080> 登录（端口被占用时设置 `LOOPFORGE_PORT=9090`）。演示模式会加入一个脚本执行器和「演示交付流程」SOP：分析 → 人工审批 → 实现 → 报告。在工作总览里新建需求、触发运行、通过审批，然后在「产物」里打开生成的 `delivery-report.md`。整个过程不调用模型。

<p align="center">
  <img src="docs/images/dashboard.png" alt="创建几张演示需求后的 LoopForge 工作总览" width="100%">
</p>

### 跑真实需求

去掉 `LOOPFORGE_DEMO_MODE` 启动，然后：

1. 在「个人设置」里添加 Claude Code、Codex 或 CodeBuddy 的 API Key（镜像里三个 CLI 都已安装），以及 GitHub 令牌：经典令牌勾选 `repo`，或细粒度令牌授予 *Contents*、*Issues*、*Pull requests* 读写权限。
2. 粘贴仓库地址，把仓库绑定到项目。
3. 通过链接导入 Issue 或打开自动同步，再派发到内置 SOP：

| SOP | 步骤 |
| --- | --- |
| 后端交付 | 需求定型 → 方案设计 → 实现与自测 → 独立评审 → 测试验证 → 发起 PR → 交付汇总 |
| 前端交付 | 需求定型 → 方案设计 → 实现 → 独立评审 → 验证 → 发起 PR → 归档汇总 |
| 产品交付 | 产品定义 → 交互 Demo → 研发交接 → 产品验收 → Issue 回写 → 交付汇总 |

第一次跑真实需求时会构建默认的沙箱镜像（Node、Agent CLI、Go、Git），只需构建一次，耗时几分钟。一张后端需求从头到尾一般需要 30 到 75 分钟，大部分时间是 Agent 在七个会话里干活。小修小改可以发布一条更短的 SOP，比如 实现 → 测试 → 提 PR。

GitHub Enterprise Server：设置 `LOOPFORGE_GITHUB_API_URL=https://<host>/api/v3` 和 `LOOPFORGE_GITHUB_HOST=<host>`。

### 其他启动方式

- `docker compose up -d --build` 总是从源码构建镜像；没有先 `docker compose pull` 时，`up` 也会自动构建，需要几分钟。`LOOPFORGE_VERSION=0.1.0` 可固定版本。
- 从源码运行（需要 Go 1.25+ 和 Git）：`make run` 把 SQLite 放在 `~/.loopforge` 并监听 `:8080`；`make demo` 启动演示模式。要单独运行二进制，请用 `make build`，并让 `bin/loopforge-gate-mcp-*` 这几个辅助程序和 `bin/loopforge` 放在一起；Agent 运行要用到它们，缺失时服务启动日志会给出警告。

## 运行在哪里执行

每次运行都有自己独立的环境。用 `docker compose` 部署时，LoopForge 通过挂载进来的 Docker socket 使用宿主机的 Docker，**每次运行启动一个容器**：同一次运行的各个步骤共用这个容器和工作区，其他运行看不到它，运行结束后删除。容器去掉了全部 Linux capability，只加入 LoopForge 的网络，用来向平台回报结果。

| 部署方式 | 隔离 |
| --- | --- |
| `docker compose up -d`（默认） | 在宿主机 Docker 上每次运行一个容器 |
| 在装有 Docker 的主机上直接运行二进制，或绑定一台装有 Docker 的执行机器 | 每次运行一个容器 |
| 没有 Docker 的 Linux（有 bubblewrap）或 macOS 主机 | 每次运行一个 bubblewrap 或 Seatbelt 沙箱 |
| 不挂 Docker socket 的 `docker compose` | 运行是 LoopForge 容器里的进程，各自有独立的工作区目录 |

挂载 Docker socket 等于让 LoopForge 拥有宿主机的 root 权限，和直接装在宿主机上一样。如果不能接受，删掉 `docker-compose.yml` 里的 `user: root` 和 `docker.sock` 两行，再绑定一台单独的、装有 Docker 的执行机器来跑需要隔离的运行。

沙箱在启动时自动选择（依次尝试 Docker、bubblewrap 或 Seatbelt、local）并写入日志；可以在「流程设置」里按 SOP 修改，或用 `LOOPFORGE_TEMPLATE_SANDBOX` 全局指定。在其他机器上执行见[执行机器](#执行机器在自己的电脑上跑)。

## 核心概念

| 概念 | 说明 |
| --- | --- |
| **项目** | 需求、仓库、成员和默认 SOP 的边界。 |
| **需求 / Issue** | 本地创建或从 GitHub 同步；派发前按项目澄清标准打分。 |
| **SOP（流程）** | 由 Agent、脚本、人工审批节点组成的版本化 DAG，带门禁、重试和打回目标。 |
| **角色** | 与执行器无关的定义：指令、规则、技能、MCP、Hook、自检循环、产物和门禁。 |
| **运行** | 一次冻结 SOP 快照的执行，共享一个沙箱/工作区，带遥测、产物和评测。 |
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
internal/runeval       运行评测及其建议的应用
internal/tracker       GitHub Issue / Pull Request 适配
internal/httpapi       REST、认证、MCP 网关
internal/repo          存储（SQLite / MySQL）、内置模板
web/                   内嵌的 Studio 界面
```

更多文档：[架构](docs/architecture.md) · [REST API](docs/api/README.md) · [OpenAPI](docs/api/openapi.yaml) · [MCP Server](docs/mcp.md) · [可观测性](docs/observability.md) · [AI 助手](docs/assistant.md) · [扩展](docs/extending.md) · [规划](docs/roadmap.zh-CN.md)。

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
| `LOOPFORGE_DEMO_MODE` | 写入演示执行器和演示 SOP。 |
| `LOOPFORGE_TEMPLATE_SANDBOX` | 内置 SOP 使用的沙箱：`generic-docker`、`linux-bwrap`、`macos-seatbelt` 或 `local`。不设置时自动检测。 |
| `LOOPFORGE_MCP_MARKET` / `LOOPFORGE_TEMPLATES` / `LOOPFORGE_WEB_PLUGINS` | 从文件、目录或 URL 加载自己的 MCP 市场条目、SOP 模板和界面扩展页面，见 [docs/extending.md](docs/extending.md)。 |

**MySQL**：启动时会自动建表、补列。另外请执行一次 [`migrations/001_run_obs_generated_columns.sql`](migrations/001_run_obs_generated_columns.sql)，MySQL 上的遥测筛选和分析统计依赖它添加的生成列；`002` 可选。SQLite 不需要任何操作。

默认使用本地账号（bcrypt）登录；需要 SSO 时放在身份感知的反向代理后面。REST 和 MCP 客户端使用在「接入指南 → 我的 Token」申请的个人令牌（`Authorization: Bearer afp_...`）。

## 执行机器：在自己的电脑上跑

在「个人 → 执行机器」复制绑定命令，到目标机器上执行：

```bash
curl -fsSL https://loopforge.example.com/runners/install.sh | bash -s -- \
  --server loopforge.example.com --token bind-xxxx --service
```

安装脚本从你的服务端下载 Runner（不需要 Go），自动发现已安装的 CLI 并注册机器。设为默认执行机器后，你负责的运行的每个节点都在那台机器上执行：检出仓库、沙箱、Agent CLI 和门禁。任务状态存在数据库里，休眠、崩溃、断网都不会丢；租约到期按策略重新排队。Runner 空闲时自动升级。

## 安全

LoopForge 会让 Agent 在真实仓库上执行命令。`local` 沙箱只是便利，不是隔离边界；Seatbelt、bubblewrap 和 Docker 适合在可信主机上无人值守运行，但不是对抗性的多租户隔离。默认的 `docker compose` 部署挂载了 Docker socket，等同于这台主机的 root 权限，请部署在专门给 LoopForge 用的机器上。部署建议和漏洞报告方式见 [SECURITY.md](SECURITY.md)。

## 规划与已知限制

接下来的重点：自反馈闭环、对话式入口（一句话说清需求，结果以回复的形式返回）、更精简的 Studio 导航、英文界面。完整列表见 [docs/roadmap.zh-CN.md](docs/roadmap.zh-CN.md)。

目前的已知限制：

- Studio 界面目前是中文。
- SOP 运行之外的 AI 功能（需求澄清、内置助手、运行评测）使用 CodeBuddy，需要在「个人设置」里配置 CodeBuddy API Key。没有 Key 时，新建需求请选「跳过分析并建单」。
- 同一个数据库只应有一个实例执行运行（只读副本设置 `LOOPFORGE_EXECUTE_RUNS=false`）。
- 执行器支持 Claude Code、Codex、CodeBuddy；需求系统支持 GitHub；产物存在数据库里。

## 参与贡献

欢迎贡献！请先阅读 [CONTRIBUTING.md](CONTRIBUTING.md) 和 [AGENTS.md](AGENTS.md)（代码分层约定），并遵守 [行为准则](CODE_OF_CONDUCT.md)。修改 CLI 及其工作流请遵循 [cli/CONTRIBUTING.zh-CN.md](cli/CONTRIBUTING.zh-CN.md) 和 [cli/AGENTS.md](cli/AGENTS.md)。问题和想法欢迎提到 [GitHub Issues](https://github.com/Tencent/LoopForge/issues)。

## 许可证

Apache License 2.0。Copyright (C) 2026 Tencent。`cli/` 下的 CLI 仍采用 [MIT 许可证](cli/LICENSE)。随附的第三方软件见 [LICENSE](LICENSE) 和 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。
