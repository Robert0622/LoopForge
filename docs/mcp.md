# LoopForge MCP 接入文档

本文件描述 **LoopForge 平台自身作为 MCP Server** 对外提供的能力：Streamable HTTP `/mcp` 与 stdio 两种传输、个人 Token 鉴权、客户端配置、工具命名与全部用户可见业务能力的映射、长任务的 start/status/cancel 用法、artifact 与日志的资源 URI，以及错误语义。

实现以 `internal/mcpserver/server.go`（协议层）与 `internal/application/operations.go`（能力清单）为准；HTTP 接口细节见 [docs/api](api/README.md)。

> **必读：不要把本文档和平台已有的「外部 MCP 消费链路」混为一谈。**
> LoopForge 同时扮演 MCP 的**服务端**和**客户端**，两条链路方向相反、凭据不同、互不通用。见 [第 1 节](#1-两条-mcp-链路必读)。

---

## 1. 两条 MCP 链路（必读）

| | 入站：LoopForge 是 MCP Server（本文档） | 出站：LoopForge 是 MCP Client（平台既有能力） |
| --- | --- | --- |
| 谁连谁 | Cursor / Claude / CodeBuddy / Codex 等客户端连 LoopForge | LoopForge 里的 Agent / 助手连 GitHub 等外部 MCP |
| 入口 | `POST /mcp`（与 Studio、`/api/*` 同端口）、`loopforge-mcp` stdio 进程 | 外部 MCP URL，来自 MCP Catalog 资产（内置市场或自定义） |
| 鉴权 | **个人 Bearer Token**（Studio「接入指南 → 我的 Token」申请）；浏览器继续使用登录 Cookie，兼容部署级 Basic Auth | Catalog Headers；`authType=personal-token` 时注入需求责任人的个人 MCP 令牌 |
| 工具 | 本文第 5 节的 LoopForge 业务工具 | 外部 MCP 自带的工具，例如 GitHub MCP 的 `create_pull_request` / `pull_request_read` / `add_issue_comment` |
| 代码 | `internal/mcpserver/`、`internal/application/operations.go`、`cmd/loopforge-mcp/` | `internal/runtime/materialize.go`、`internal/runtime/mcp_call.go`、`internal/httpapi/mcp_market.go` |

### 1.1 出站链路：MCP Catalog

Studio 的「能力资产 → MCP 管理」是一份**被消费的外部 MCP 资产目录**（`MCPServerDefinition`），通过 `GET/PUT /api/mcp-servers` 与 `POST /api/mcp-servers/import` 维护，字段为 `transport`（`stdio` | `http`）、`command` / `args` / `environment`、`url` / `headers`、`authType`、`timeoutSec`。`authType=personal-token` 的 HTTP MCP 会在运行时使用需求责任人的个人 MCP 令牌生成 Authorization 请求头，不在共享资产中保存个人 Token。它服务于两类消费者：

- **Agent 运行时**：被引用的条目物化为 `.loopforge/mcp/<server-id>.json`，再通过 `.loopforge/<agent-id>/mcp` 软链视图注入执行器；仓库自带的 `.mcp.json` 会被沙箱屏蔽，不能覆盖平台选择的资产。Headers 中的密钥用占位符 `${secret:ENV_NAME}` 表示，仅在物化运行时从控制面进程环境解析，不落库明文。
- **Issue 同步**：GitHub Issue / Pull Request 同步不走 MCP，直接调用 GitHub REST API（`internal/tracker/`），令牌来自 `LOOPFORGE_GITHUB_TOKEN` 或用户的 Git Token。

### 1.2 凭据不互通

出站凭据（Catalog Headers、个人 MCP 令牌）**不能**用来访问 LoopForge 的 `/mcp`；入站个人 Token 也不会被转发给任何外部 MCP。两者分别配置、分别轮换。

---

## 2. 传输方式

### 2.1 Streamable HTTP：`/mcp`

`/mcp` 与 Studio、`/api/*` 共用同一个监听端口（`LOOPFORGE_HTTP_PORT`，默认 `8080`），因此复用同一套用户鉴权、同源 CORS 与安全响应头。它是**无状态**实现：不签发也不要求 `Mcp-Session-Id`，每个 POST 自成一次完整的请求-响应。

| 项目 | 约定 |
| --- | --- |
| 客户端 → 服务端 | `POST /mcp`，body 为**单条** JSON-RPC 2.0 消息（不支持批量数组） |
| 请求头 | `Content-Type: application/json` |
| 响应 | `application/json`，HTTP `200` 携带 JSON-RPC 响应；响应头回带协商后的 `MCP-Protocol-Version` |
| 通知 | `notifications/*` 无响应体，返回 HTTP `202` |
| 解析失败 | HTTP `400` + JSON-RPC `-32700` |
| `GET /mcp` | `405`，响应 `Allow: POST, DELETE`（无服务端主动推送通道） |
| `DELETE /mcp` | `204`，无状态实现下为空操作，仅为兼容客户端的会话关闭流程 |
| 协议版本 | 接受 `2024-11-05`、`2025-03-26`、`2025-06-18`；未声明或不识别时按 `2025-06-18` 协商 |
| 请求体上限 | `/mcp` 4 MiB；被转发的 `/api/*` 请求体上限 2 MiB |

支持的方法：`initialize`、`ping`、`tools/list`、`tools/call`、`resources/list`、`resources/templates/list`、`resources/read`。其余方法返回 `-32601`。

服务端在 `initialize` 中声明的能力：

```json
{
  "protocolVersion": "2025-06-18",
  "capabilities": {
    "tools": { "listChanged": false },
    "resources": { "subscribe": false, "listChanged": false }
  },
  "serverInfo": { "name": "loopforge-platform", "version": "1.0.0" }
}
```

即：工具列表不会动态变更，**资源不支持订阅**，需要实时进度请轮询（见第 6 节）。

`/mcp` 在进程内直接调用同一个 HTTP 路由表来执行业务操作，不额外发起网络请求，因此不会引入第二套业务语义或第二次鉴权。

### 2.2 stdio：`loopforge-mcp`

`loopforge-mcp` 是独立的**瘦桥接进程**：从 stdin 逐行读取 JSON-RPC，转发到目标 LoopForge 实例的 HTTP API，再把响应逐行写回 stdout。它不连 MySQL、不执行 Run，适合把凭据留在本机或客户端不便直连服务的场景。

```bash
make build   # 产出 bin/loopforge 和 bin/loopforge-mcp

export LOOPFORGE_API_URL=https://loopforge.example.com
export LOOPFORGE_API_TOKEN='<personal-token>'
bin/loopforge-mcp
```

| 环境变量 | 说明 |
| --- | --- |
| `LOOPFORGE_API_URL` | **必填**，目标实例基址（不含 `/mcp`）。未设置时进程会向 stderr 打印提示 |
| `LOOPFORGE_API_TOKEN` | Studio「接入方式」申请的个人 Token，优先于 Basic Auth |
| `LOOPFORGE_API_USER` / `LOOPFORGE_API_PASS` | 兼容旧部署的 Basic Auth 凭据 |

约束：一行一条 JSON-RPC 消息，单行上限 4 MiB；stdout 是协议通道，诊断信息一律走 stderr；远端响应读取上限 32 MiB。

### 2.3 两种传输的差异

工具集、参数结构、返回结构、错误语义完全一致。差异只有：HTTP 模式在服务进程内直调路由，stdio 模式经由网络回调 HTTP API（因此多一跳网络与一层 Bearer 鉴权）；stdio 的进程生命周期就是会话，HTTP 无会话概念。两者都不推送事件，长任务一律靠轮询。

### 2.4 能力自描述

`GET /api/capabilities` 返回权威的操作清单与两种传输入口，可用于客户端自检或代码生成：

```json
{
  "operations": [ { "name": "runs_start", "method": "POST", "path": "/api/runs", "readOnly": false } ],
  "mcp": { "streamableHttp": "/mcp", "stdioCommand": "loopforge-mcp" }
}
```

---

## 3. 鉴权与安全

### 3.1 用户登录与个人 Bearer Token

Studio 使用企业登录 Cookie。登录用户在「接入指南 → 我的 Token」创建个人 Token 后，REST API 与 `/mcp` 使用：

```
Authorization: Bearer afp_<secret>
```

- Token 明文只在创建时显示一次，数据库仅保存 SHA-256 哈希；支持 1–365 天有效期、最近使用时间和即时撤销。
- 设置 `LOOPFORGE_AUTH_REQUIRED=true` 后，公共 `/api/*` 与 `/mcp` 必须使用有效登录 Cookie 或个人 Token；失败返回 `401` + `WWW-Authenticate: Bearer realm="LoopForge"`。
- `GET/POST /api/auth/tokens` 与 `DELETE /api/auth/tokens/{id}` 只接受浏览器登录 Cookie，不接受 Token 自行增殖。
- `-basic-user` / `-basic-pass` 仍作为兼容的部署级外围 Basic Auth；新接入应使用个人 Token。
- 鉴权发生在 JSON-RPC 之前，属于传输层错误，**不会**包装成 JSON-RPC 错误对象；客户端必须按 HTTP 状态码处理。
- 唯一豁免是回环地址上的 `/api/otel/`（遥测回收），与 MCP 无关。

### 3.2 部署要求

- 这是明文 HTTP 的开发 / 内网形态。对外暴露必须在前面放 TLS 与身份感知反向代理，或用 `-tls-cert` / `-tls-key` 直接启用 TLS。
- CORS 只允许同源，异源请求返回 `403`；浏览器内的 MCP 客户端需与 LoopForge 同源部署。
- 只读副本（`LOOPFORGE_EXECUTE_RUNS=false`）保留全部只读工具，但会触发执行的工具返回 `403`。
- 建议每个 MCP 客户端单独申请一个个人 Token，便于识别、轮换与吊销。

---

## 4. 客户端配置

客户端配置格式并不相同。Cursor / CodeBuddy 使用 JSON；Claude Code 推荐通过 `claude mcp add` 维护配置；Codex 使用 TOML。修改时只合并 `loopforge` 条目，不要覆盖已有配置和其他 MCP 服务。

### 4.1 Cursor / CodeBuddy：Streamable HTTP JSON

```json
{
  "mcpServers": {
    "loopforge": {
      "type": "streamable-http",
      "url": "https://loopforge.example.com/mcp",
      "headers": {
        "Authorization": "Bearer afp_<personal-token>"
      }
    }
  }
}
```

- 少数客户端不认 `type`，只按有无 `url` 判断传输类型；删掉 `"type"` 即可，其余不变。
- `Authorization` 的值是创建个人 Token 时一次性显示的 `Bearer afp_...`。请优先使用客户端自身的密钥引用机制，不要把明文提交进仓库。
- `${secret:ENV_NAME}` 是 **LoopForge MCP Catalog 内部**的占位符约定（见 1.1），第三方客户端不认识，配置外部客户端时用该客户端自己的变量语法。

### 4.2 Claude Code

推荐用 CLI 创建用户级配置（由 Claude Code 写入 `~/.claude.json`）：

```bash
claude mcp add --transport http --scope user loopforge https://loopforge.example.com/mcp \
  --header "Authorization: Bearer afp_<personal-token>"
```

用 `claude mcp get loopforge` 和 `claude mcp list` 验证。项目共享配置应使用项目根目录 `.mcp.json`，不要把个人 Token 提交进仓库。

### 4.3 Codex

Codex 默认读取 `~/.codex/config.toml`；项目级配置为受信任项目中的 `.codex/config.toml`。HTTP MCP 配置示例：

```toml
[mcp_servers.loopforge]
url = "https://loopforge.example.com/mcp"
http_headers = { Authorization = "Bearer afp_<personal-token>" }
```

用 `codex mcp list` 验证。生产环境更建议把 Token 放入环境变量，再用 `bearer_token_env_var` 引用。

### 4.4 stdio

```json
{
  "mcpServers": {
    "loopforge": {
      "command": "/usr/local/bin/loopforge-mcp",
      "env": {
        "LOOPFORGE_API_URL": "https://loopforge.example.com",
        "LOOPFORGE_API_TOKEN": "afp_<personal-token>"
      }
    }
  }
}
```

### 4.5 连通性自检

```bash
curl -sS -H 'Authorization: Bearer afp_<personal-token>' \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' \
  https://loopforge.example.com/mcp
```

`401` 说明凭据不对；`403` 说明命中同源校验（检查是否误带 `Origin`）；拿到工具列表即接入成功。

---

## 5. 工具

### 5.1 命名规范

- 形如 `<资源域>_<动作>`，全小写 snake_case，匹配 `^[a-z][a-z0-9_]*$`，长度 ≤ 64，不含点号或冒号，兼容所有主流客户端的名称校验。
- 资源域用复数名词（`runs`、`issues`、`workflows`），单例能力用单数（`health_get`、`analytics_get`）。
- 动作动词固定：`list` / `get` / `save` / `delete` / `start` / `cancel` / `import` / `restore` / `publish` / `clone` / `run` / `download` / `add`。
- `save` 表示按 `id` 覆盖写入的幂等写（对应 HTTP `PUT`）；平台没有独立的 create/update 语义，创建与更新是同一次写入。
- 名称不带 `loopforge_` 前缀，命名空间由客户端侧的服务器名提供（在会展平工具名的客户端里显示为 `mcp__loopforge__runs_start`）。
- 工具名一经发布只增不改；必须改名时旧名保留为别名至少一个大版本。

### 5.2 统一参数结构

所有工具共用一个透传式 `inputSchema`，字段最多三个，`additionalProperties: false`：

```json
{
  "path":  { "id": "run-1730000000" },
  "query": { "nodeId": "review", "limit": "100" },
  "body":  { "workflowId": "sop-fix", "input": "修复登录超时" }
}
```

- `path`：URL 路径参数，工具路径含 `{name}` 时必填，缺失返回 `path.<name> is required`。
- `query`：查询参数，值一律用字符串（含数字与布尔）。
- `body`：JSON 请求体，仅 `POST` / `PUT` 工具提供该字段。

每个工具的 `description` 末尾附带其映射的 `METHOD /path`，`annotations` 提供 `readOnlyHint`、`destructiveHint`（`DELETE` 或名称含 `reject` / `cancel`）、`idempotentHint`（只读、`PUT` 或 `DELETE`）。

### 5.3 返回结构

`tools/call` 返回：

```json
{
  "content": [{ "type": "text", "text": "<LoopForge API 原始响应体>" }],
  "structuredContent": { "status": 201, "data": { "id": "run-1730000000", "status": "running" } },
  "isError": false
}
```

- `content[0].text` 是 API 的原始响应体（JSON 或纯文本）。
- `structuredContent` 仅在响应体是合法 JSON 时出现，`status` 是 HTTP 状态码，`data` 是解析后的响应。
- `isError` 在 HTTP 状态码不在 2xx 时为 `true`。

### 5.4 能力映射总表（106 个工具）

覆盖平台全部用户可见业务能力。「只读」列对应 `readOnlyHint`。

#### 平台与目录

| 工具 | 语义 | 映射接口 | 只读 |
| --- | --- | --- | --- |
| `health_get` | 存活探针 | `GET /api/health` | 是 |
| `capabilities_get` | 公共 HTTP / MCP 操作清单与传输入口 | `GET /api/capabilities` | 是 |
| `openapi_get` | OpenAPI 3.1 机器可读契约 | `GET /api/openapi.yaml` | 是 |
| `bootstrap_get` | 一次性拉取项目、仓库、角色、资产、SOP、同步配置、定时任务与存储信息 | `GET /api/bootstrap` | 是 |
| `observability_spec_get` | 遥测事件契约（类别、字段、适配器） | `GET /api/observability/spec` | 是 |

#### 项目与仓库（项目成员）

| 工具 | 语义 | 映射接口 | 只读 |
| --- | --- | --- | --- |
| `business_domains_list` | 业务域目录与调用者管理／查看权限 | `GET /api/business-domains` | 是 |
| `business_domains_save` | 平台管理员维护分类与管理员；业务域管理员维护信息和查看者 | `PUT /api/business-domains` | 否 |
| `business_domain_report` | 按当前项目归属查看已授权业务汇总，不返回个人或运行明细 | `GET /api/business-domain-report` | 是 |
| `project_business_domain_assign` | 设置主业务域，携带 expectedDomainId 和 reason；Owner 仅能归类未分类项目 | `PUT /api/projects/{id}/business-domain` | 否 |
| `project_business_domain_history` | Owner／平台管理员查看最近 100 条归属变更 | `GET /api/projects/{id}/business-domain/history` | 是 |
| `projects_list` | 项目列表，凭据脱敏为 `*Configured` 布尔位 | `GET /api/projects` | 是 |
| `projects_get` | 单个项目 | `GET /api/projects/{id}` | 是 |
| `projects_save` | 新建或更新项目：澄清策略、默认仓库、默认 SOP、凭据 | `PUT /api/projects` | 否 |
| `repositories_list` | 仓库列表（凭据脱敏） | `GET /api/repositories` | 是 |
| `repositories_get` | 单个仓库 | `GET /api/repositories/{id}` | 是 |
| `repositories_save` | 新建或更新仓库绑定（`provider` 目前仅 `github`） | `PUT /api/repositories` | 否 |
| `repositories_delete` | 删除仓库并清理引用它的项目默认仓库；仓库 ID 走 `query.id` | `DELETE /api/repositories` | 否 |

`personalMcpToken` / `gitToken` / `codeBuddyApiKey` 留空表示保留原值，需显式传对应的 `*Clear=true` 才清空。

#### 需求与 Issue（需求列表）

| 工具 | 语义 | 映射接口 | 只读 |
| --- | --- | --- | --- |
| `issues_list` | 需求 / Issue 列表。`query` 支持 `projectId`、`status`（可逗号分隔）、`source`、`q`（标题/编号/外部 ID）、`limit`、`offset`。不传 `limit` 时返回 `Issue[]`；传入后返回 `{items,total,offset,limit,hasMore}`（`limit` 最大 200） | `GET /api/issues` | 是 |
| `issues_get` | 单条需求详情 + 评论 | `GET /api/issues/{id}` | 是 |
| `issues_delete` | 删除本地需求、评论和澄清记录，并清理失败／取消的历史任务、运行、日志、产物和执行环境。已有成功历史或活动任务时返回 `409`；保留外部原单，后续同步可重新导入 | `DELETE /api/issues/{id}` | 否 |
| `issues_save` | 新建或更新需求。`body.syncTarget` 取 `local` 或 `github`（`github` 会同步创建 GitHub Issue，要求已绑定仓库）；更新时未传字段保留原值 | `PUT /api/issues` | 否 |
| `issues_comments_add` | 追加评论 | `POST /api/issues/{id}/comments` | 否 |
| `issues_runs_list` | 该需求的历史 Run 摘要（倒序） | `GET /api/issues/{id}/runs` | 是 |
| `issues_clarify` | 按项目澄清策略对答案打分，返回分数、维度得分、缺失维度与是否达标 | `POST /api/issues/{id}/clarify` | 否 |
| `issues_chat_clarify` | 对已有需求跑 AI 澄清（服务端任务，关网页不中断；GET 同路径可接回 SSE） | `POST /api/issues/{id}/chat-clarify` | 否 |
| `issues_chat_clarify_stream` | 接入正在进行的澄清 SSE，回放思考过程 | `GET /api/issues/{id}/chat-clarify` | 是 |
| `issues_chat_clarify_cancel` | 取消一张需求正在进行的服务端澄清 | `POST /api/issues/{id}/chat-clarify/cancel` | 否 |
| `issues_clarification_save` | 保存已有工作项的多轮澄清；完成时幂等写回对应 GitHub Issue 评论 | `POST /api/issues/{id}/clarification` | 否 |
| `clarification_chat` | 需求还不存在时的 AI 澄清分析 | `POST /api/chat-clarify` | 否 |
| `clarification_cancel` | 取消进行中的 AI 澄清 | `POST /api/chat-clarify/cancel` | 否 |
| `issues_reopen` | 重新打开已交付闭环（PR 已合入）的需求，解除执行锁 | `POST /api/issues/{id}/reopen` | 否 |
| `issues_dispatch` | 派发需求到 SOP：创建 Task 并启动 Run，`body.rerun` 控制重跑 | `POST /api/issues/{id}/dispatch` | 否 |
| `issues_import` | 从 GitHub Issue 链接导入一条需求到当前项目并按澄清策略打分（`body`：`projectId`、`url`，以及至少一个 `repositoryId` / `repositoryIds`） | `POST /api/issues/import` | 否 |
| `tasks_list` | 执行任务记录列表 | `GET /api/tasks` | 是 |
| `tasks_get` | 单条执行任务 | `GET /api/tasks/{id}` | 是 |

需求状态：`backlog`、`todo`、`in_progress`、`in_review`、`done`、`blocked`、`cancelled`。任务状态：`queued`、`dispatched`、`running`、`completed`、`failed`、`cancelled`；`dispatchMode` 为 `primary` / `scheduled` / `rerun`，`triggerOrigin` 为 `manual` / `timer` / `manual_run`。

「待我确认」清单没有独立工具：用 `runs_list` 找出 `status=waiting` 的运行，再用 `runs_get` 定位 `waiting_approval` 节点。

AI 澄清工具使用接口的阻塞 JSON 形式，因此调用时长等于分析时长（服务端上限 10 分钟，超时返回 `504`）；需要取消时可由另一客户端调用 `clarification_cancel`。

#### SOP / 流程（流程模板、流程设计）

| 工具 | 语义 | 映射接口 | 只读 |
| --- | --- | --- | --- |
| `workflows_list` | SOP 列表 | `GET /api/workflows` | 是 |
| `workflows_get` | 单个 SOP 完整定义（节点、连线、校验门、沙箱） | `GET /api/workflows/{id}` | 是 |
| `workflows_save` | 保存 SOP。`sandboxProfileId` 必填；带 `revision` 做乐观并发，落后写入返回 `409` | `PUT /api/workflows` | 否 |
| `workflows_delete` | 删除 SOP；存在进行中 Run 或被未关闭需求绑定时返回 `409` | `DELETE /api/workflows/{id}` | 否 |
| `workflows_publish` | 发布：状态置 `published` 并递增版本号 | `POST /api/workflows/{id}/publish` | 否 |
| `workflows_clone` | 克隆为新的草稿 SOP（`body` 需 `id`、`name`、`owner`） | `POST /api/workflows/{id}/clone` | 否 |
| `workflows_revisions_list` | 历史版本列表（含 `payload` 快照） | `GET /api/workflows/{id}/revisions` | 是 |
| `workflows_revision_get` | 单个历史版本 | `GET /api/workflows/{id}/revisions/{revisionId}` | 是 |
| `workflows_revision_restore` | 回滚到指定历史版本 | `POST /api/workflows/{id}/revisions/{revisionId}/restore` | 否 |
| `workflow_metrics_get` | SOP 运营指标：闭环率、成功率、返工率、时长、Token、工具调用、产物数、近期 Run、节点可靠性 | `GET /api/workflow-metrics` | 是 |

#### 角色 Agent（角色定义）

| 工具 | 语义 | 映射接口 | 只读 |
| --- | --- | --- | --- |
| `agents_list` | 角色目录（轻量视图：执行器、模型、引用的规则/技能/MCP、循环策略） | `GET /api/agents` | 是 |
| `agents_get` | 角色完整定义（指令、任务书、干预点、产物契约、校验门） | `GET /api/agents/{id}` | 是 |
| `agents_save` | 保存角色定义 | `PUT /api/agents` | 否 |
| `agents_revisions_list` | 角色历史版本（含 `payload` 快照） | `GET /api/agents/{id}/revisions` | 是 |
| `agents_revision_get` | 单个角色历史版本 | `GET /api/agents/{id}/revisions/{revisionId}` | 是 |
| `agents_revision_restore` | 回滚角色到历史版本 | `POST /api/agents/{id}/revisions/{revisionId}/restore` | 否 |
| `agents_delete` | 删除未被流程引用且未共享的角色 | `DELETE /api/agents/{id}` | 否 |

#### 能力资产与环境资产

列表、覆盖写与删除。删除要求未共享且无引用；技能删除会级联清理二进制文件；内置沙箱 `local` 不能删。写入即校验。

| 工具 | 语义 | 映射接口 |
| --- | --- | --- |
| `rules_list` / `rules_save` / `rules_delete` | 规则（Rules），按优先级注入角色 | `GET/PUT/DELETE /api/rules` |
| `skills_list` / `skills_save` / `skills_delete` | 技能（Skills），含附带文件 | `GET/PUT/DELETE /api/skills` |
| `mcp_servers_list` / `mcp_servers_save` | **外部** MCP 服务器资产目录（stdio/http） | `GET/PUT /api/mcp-servers` |
| `mcp_servers_import` | 批量导入标准 MCP JSON（校验 `transport` / `url` / `command`），导入后立即重绑同步客户端 | `POST /api/mcp-servers/import` |
| `executors_list` | 只读的内置 CLI（`codebuddy` / `codex` / `claude`） | `GET /api/executors` |
| `dockerfiles_list` / `dockerfiles_save` / `dockerfiles_delete` | Dockerfile 资产（内容与构建上下文存库，沙箱初始化时临时构建） | `GET/PUT/DELETE /api/dockerfiles` |
| `environments_list` / `environments_save` / `environments_delete` | 环境安装脚本（`install.sh`、变量、PATH 条目） | `GET/PUT/DELETE /api/environments` |
| `sandboxes_list` / `sandboxes_save` / `sandboxes_delete` | 沙箱档案（`local` / `mac-seatbelt` / `linux-bwrap` / Docker） | `GET/PUT/DELETE /api/sandboxes` |

`mcp_servers_*` 管理的是**被消费的外部 MCP 服务器**，与本文档描述的 LoopForge 自身 MCP Server 无关。

#### 运行 Run（运行记录）

| 工具 | 语义 | 映射接口 | 只读 |
| --- | --- | --- | --- |
| `runs_start` | 按 SOP 启动一次运行（`body`：`workflowId`、`input`、`projectId`） | `POST /api/runs` | 否 |
| `runs_list` | 运行摘要列表（不含遥测、日志与产物正文） | `GET /api/runs` | 是 |
| `runs_get` | 单次运行详情：整体状态、各节点状态与尝试次数、冻结的 SOP 快照、用量、产物元数据、最近事件 | `GET /api/runs/{id}` | 是 |
| `runs_cancel` | 取消运行 | `POST /api/runs/{id}/cancel` | 否 |
| `runs_pause` | 中断当前节点并保留工作区，转入人工补充信息状态 | `POST /api/runs/{id}/pause` | 否 |
| `runs_resume` | 继续暂停的运行（`body.context` 可选，会注入当前节点） | `POST /api/runs/{id}/resume` | 否 |
| `runs_refresh_sop` | 暂停期间把冻结流程更新为最新 SOP，保留已完成同名节点及当前环境 | `POST /api/runs/{id}/refresh-sop` | 否 |
| `runs_delete` | 删除运行记录及其历史 | `DELETE /api/runs/{id}` | 否 |
| `runs_approve` | 通过人工确认门（`body.nodeId`） | `POST /api/runs/{id}/approve` | 否 |
| `runs_pass` | 将失败的**脚本**节点视为通过并继续下游（`body.nodeId`），不重新执行；AI 节点应改用 `runs_retry` 并附带 `context` | `POST /api/runs/{id}/pass` | 否 |
| `runs_reject` | 打回（`body`：`nodeId`、`targetId`、`reason`）：重置目标节点及下游，返工计数 +1，理由注入下次尝试 | `POST /api/runs/{id}/reject` | 否 |
| `runs_retry` | 重试指定节点（`body`：`nodeId`、`reason`） | `POST /api/runs/{id}/retry` | 否 |
| `runs_decline_retry` | 放弃重试，把等待中的节点判为失败（`body.nodeId`） | `POST /api/runs/{id}/decline-retry` | 否 |
| `runs_eval_start` | 手动评测一次已结束的 SOP run（`body.model` 可选，使用当前用户 CodeBuddy Key 与助手模型目录） | `POST /api/runs/{id}/eval` | 否 |
| `runs_eval_get` | 读取该 run 最近一次评测结果 | `GET /api/runs/{id}/eval` | 是 |
| `runs_eval_apply` | 用户确认后把评测建议写回当前 SOP/角色（`body`：`evalId`、`recommendationIds`） | `POST /api/runs/{id}/eval/apply` | 否 |
| `runs_telemetry_list` | 分页查询遥测事件（`query`：`nodeId`、`adapter`、`after`、`before`、`limit`） | `GET /api/runs/{id}/telemetry` | 是 |
| `runs_logs_list` | 分页查询 worker 日志（`query`：`nodeId`、`after`、`before`、`limit`） | `GET /api/runs/{id}/logs` | 是 |
| `runs_logs_download` | 导出全量纯文本日志（`query.nodeId` 可选） | `GET /api/runs/{id}/logs.txt` | 是 |
| `runs_artifact_get` | 读取单个不可变产物正文（`path`：`id`、`artifactId`） | `GET /api/runs/{id}/artifacts/{artifactId}` | 是 |

运行状态：`running`、`waiting`、`succeeded`、`failed`、`cancelled`。节点状态：`pending`、`running`、`waiting_approval`、`succeeded`、`failed`、`cancelled`。

**结果裁剪**：为控制上下文占用，运行详情会裁剪长文本——摘要列表清空节点输出、遥测与日志；详情里节点 `output` 截断至 2500 字符、`error` 1200、`feedback` 800、`activity` 300；遥测 `message` 800、日志 `body` 2000；产物只返回元数据。完整正文一律通过第 7 节的资源 URI 或 `runs_artifact_get` / `runs_logs_download` 获取。

**实例绑定**：Run 的工作区绑定在特定实例上，`runs_approve` / `runs_pass` / `runs_reject` / `runs_retry` / `runs_resume` 必须发到绑定实例，否则返回带绑定主机信息的错误。

#### 用量分析（用量分析）

| 工具 | 语义 | 映射接口 | 只读 |
| --- | --- | --- | --- |
| `analytics_get` | 运营快照：运行数与结果分布、完成率、一次通过率、P50/P95/P99 时长、Token 与成本、工具调用、产物、趋势、维度拆解、失败归因、遥测覆盖率、交付统计。`query`：`projectId`、`workflowId`、`userId`、`issueSource`、`grain`（day/week/month/all）、`from`、`to`（RFC3339） | `GET /api/analytics` | 是 |

#### 外部同步（GitHub）

这些工具**操作出站链路**，本身通过入站 MCP 暴露。

| 工具 | 语义 | 映射接口 | 只读 |
| --- | --- | --- | --- |
| `sync_config_get` | 同步配置：GitHub API 地址、自动同步开关与间隔、上次结果 | `GET /api/sync/config` | 是 |
| `sync_config_save` | 保存同步配置并重启自动同步（间隔缺省 300 秒、下限 30 秒） | `PUT /api/sync/config` | 否 |
| `sync_start` | 立即触发一次全量拉取，**异步执行、立刻返回** | `POST /api/sync/now` | 否 |
| `sync_status_get` | 是否在同步 + 配置 + 最近日志 | `GET /api/sync/status` | 是 |
| `sync_logs_list` | 同步日志（方向、来源、动作、状态、条数、错误） | `GET /api/sync/logs` | 是 |
| `sync_issue_create` | 把本地需求推送为 GitHub Issue 并回写 `externalRef`（`body`：`issueId`、`target`） | `POST /api/sync/create` | 否 |

#### 定时任务

| 工具 | 语义 | 映射接口 | 只读 |
| --- | --- | --- | --- |
| `schedule_jobs_list` | 定时任务列表（含上次运行时间、状态、候选与派发计数） | `GET /api/schedule-jobs` | 是 |
| `schedule_jobs_save` | 新建或更新定时任务；`name` 必填。调度为常用间隔 `intervalSec` 或 5 段 `cron`（东八区）。触发任务 `skipDispatch=false` 需 `workflowId`，到点扫描需求、基线门禁并派发 SOP；AI 定时任务 `skipDispatch=true` 需 `assistantPrompt` / `assistantModel`，只用 CodeBuddy 跑提示词 | `PUT /api/schedule-jobs` | 否 |
| `schedule_jobs_delete` | 删除定时任务 | `DELETE /api/schedule-jobs/{id}` | 否 |
| `schedule_jobs_run` | 在服务端立即启动一次（`lastStatus=running` 后马上返回）；关客户端不会取消。实例调度器关闭或已有任务在跑时返回 `409` | `POST /api/schedule-jobs/{id}/run` | 否 |
| `assistant_tasks_list` | 遗留：当前用户的助手定时任务（可用 `query.projectId` 过滤）；Studio 不再调度 | `GET /api/assistant/tasks` | 是 |
| `assistant_tasks_save` | 遗留：新建或更新助手定时任务；`intervalSec`（≥60）或 `cron`（5 段，东八区）二选一，可指定 `model`。进程默认不会到点执行，请用平台 `schedule_jobs` 的 `assistantPrompt` | `PUT /api/assistant/tasks` | 否 |
| `assistant_tasks_delete` | 删除助手定时任务 | `DELETE /api/assistant/tasks/{id}` | 否 |
| `assistant_tasks_run` | 立即执行一次助手定时任务 | `POST /api/assistant/tasks/{id}/run` | 否 |

定时派发使用集成分支（默认 `loopforge/integration`），按仓库串行化最终集成，冲突回流到 SOP 的修复与校验节点，最多三次。平台 `schedule_jobs_*` 可只跑提示词，或再扫描当前项目已同步的需求并派发。`assistant_tasks_*` 仅保留 API。

#### 不对外暴露

`POST /api/runs/{id}/signals`（worker 信号回传）、`POST /api/otel/v1/traces`（回环遥测采集）、运行租约与沙箱维护属于数据面内部通道，不在 MCP 清单中。SSE 流 `GET /api/runs/{id}/stream` 也不暴露为工具——MCP 侧统一用轮询。

---

## 6. 长任务：start / status / cancel

SOP 运行可能持续数分钟到数小时，远超单次 MCP 请求的合理时限。平台的做法是**启动即返回句柄**，后续靠轮询。服务端不推送、不支持资源订阅，客户端必须自己轮询。

| 场景 | start | status | cancel |
| --- | --- | --- | --- |
| SOP 运行 | `runs_start`（返回 `201` + Run，含 `id`、`status`） | `runs_get`（`path.id` = Run ID） | `runs_cancel` |
| 需求派发 | `issues_dispatch`（返回 `201` + Task，含 `runId`） | `runs_get`（`path.id` = Task 的 `runId`） | `runs_cancel` |
| 外部同步 | `sync_start`（返回 `{"status":"started","syncing":true}`） | `sync_status_get` / `sync_logs_list` | 不支持取消（单次拉取有界） |
| 定时任务立即执行 | `schedule_jobs_run` | `schedule_jobs_list`（看 `lastStatus` / `lastPulled` / `lastDispatched`）；派发出的 Run 用 `runs_get` | `runs_cancel`（针对派发出的 Run） |

### 6.1 轮询节奏

`runs_get` 返回完整运行状态。建议轮询间隔 3–5 秒，长时间运行可退避到 15–30 秒。要跟踪细粒度进展，用 `runs_get` 返回的 `telemetrySequence` / `logSequence` 作为游标，配合 `runs_telemetry_list` / `runs_logs_list` 的 `query.after` 增量拉取，避免重复搬运历史事件。

终止条件：`status` 进入 `succeeded` / `failed` / `cancelled`。`running` 与 `waiting` 都是非终态。

### 6.2 人工确认

`status=waiting` 且存在 `waiting_approval` 节点时需要人来决策，Agent 不应自行推进。可用动作：`runs_approve` 通过、`runs_reject` 打回（重置目标节点及下游、返工 +1、理由注入下次尝试）、`runs_retry` 重试、`runs_decline_retry` 放弃并判失败。等待期间不占用执行并发槽。

`status=failed` 且失败节点是脚本时（停下来或打回轮次用尽），可用 `runs_pass` 将该节点视为成功并继续下游，无需重新执行。硬脚本卡住时用这个，而不是反复 `runs_retry`。失败的 AI 节点应调用 `runs_retry`，并把补充说明放进 `body.context`。

### 6.3 取消语义

`runs_cancel` 将取消意图持久化到数据库，再由持有租约的 worker 中断节点和 Runner 任务；API 与 worker 分进程部署时同样有效。已结算或因进程重启而失去调度者的运行会被清出活动计数。对已进入终态的运行重复取消不会破坏状态。

`runs_pause` 使用相同的持久化控制通道，但不会销毁工作区。节点收敛到人工确认状态后，可以先调用 `runs_refresh_sop` 更新流程图，再通过 `runs_resume` 附带补充信息继续；补充内容同时进入 `human-context.md` 和当前节点反馈。流程更新不会在运行中更换已经冻结的沙箱环境。

### 6.4 派发幂等

`issues_dispatch` 的首次任务 ID 由 `issueId` 哈希派生，重复派发不会产生重复 Task：已跑过的需求返回 `409`，必须显式 `body.rerun=true` 才会新建运行；对从未运行过的需求传 `rerun=true` 返回 `400`。

---

## 7. 资源（Resources）

大体积、不可变或纯文本的内容通过 `resources/read` 读取，不塞进工具返回值。

### 7.1 URI 模板

`resources/templates/list` 返回：

| URI 模板 | 内容 | mimeType |
| --- | --- | --- |
| `loopforge://runs/{runId}/artifacts/{artifactId}` | 不可变产物正文 | `text/markdown`（实际以 API 响应的 `Content-Type` 为准） |
| `loopforge://runs/{runId}/logs` | 该 Run 的全量 worker 日志文本导出 | `text/plain` |

`resources/list` 返回空数组：产物与日志按需读取，不做全量枚举；`resources/subscribe` 不受支持（`initialize` 中已声明 `subscribe: false`）。URI 不匹配上述两种模板时返回 `-32602`。

读取结果形如：

```json
{ "contents": [ { "uri": "loopforge://runs/run-1730000000/logs", "mimeType": "text/plain", "text": "..." } ] }
```

### 7.2 产物的不可变性

产物按「节点 + 尝试次数」快照，历史尝试永久保留以便审计，下游节点消费最新一次成功版本。产物元数据带 `sha256`，HTTP 层同时返回同值的 `ETag`；同一 `artifactId` 的内容永不变化，可安全长期缓存。产物 ID 从 `runs_get` 的产物元数据中获取。

### 7.3 与工具的关系

`loopforge://runs/{runId}/artifacts/{artifactId}` 与 `runs_artifact_get`、`loopforge://runs/{runId}/logs` 与 `runs_logs_download` 是同一份数据的两种取法。不支持资源读取的客户端直接用工具；需要按节点过滤日志时也用工具（资源 URI 不带 `nodeId` 参数）。

---

## 8. 错误语义

### 8.1 三层错误

**第一层：传输层。** 登录 Cookie / Bearer Token 鉴权失败返回 HTTP `401` + `WWW-Authenticate`，异源请求返回 `403`，请求体解析失败返回 HTTP `400` + JSON-RPC `-32700`。这些都不是 JSON-RPC 成功响应，客户端必须先看 HTTP 状态码。

**第二层：协议层。** JSON-RPC 错误对象，用于协议与参数问题，不应重试：

| code | 触发场景 |
| --- | --- |
| `-32700` | 请求体不是合法 JSON |
| `-32601` | 方法不存在（`initialize` / `ping` / `tools/*` / `resources/*` 之外） |
| `-32602` | 工具名未知、`arguments` 结构非法、缺少必填路径参数、资源 URI 不受支持 |

**第三层：业务层。** `tools/call` 正常返回，`isError: true`，`structuredContent.status` 为下游 HTTP 状态码，`content[0].text` 与 `structuredContent.data` 为 API 原始错误体（形如 `{"error":"..."}`）。桥接进程本身的失败（如 `LOOPFORGE_API_URL` 未配置、网络不可达）也返回 `isError: true`，`structuredContent` 为 `{"error":"..."}` 且无 `status`。

### 8.2 业务状态码对照

| status | 含义 | 典型场景 | 处理方式 |
| --- | --- | --- | --- |
| `400` | 参数或业务校验失败 | 必填字段缺失、引用无效、时间戳格式错误、请求体超 2 MiB、对从未运行的需求传 `rerun=true`；底层存储与引擎错误当前也映射为 `400` | 改参数后重试 |
| `401` | 登录或个人 Token 无效 / 已过期 / 已撤销 | — | 重新登录或申请新 Token |
| `403` | 拒绝执行 | 实例只读（`LOOPFORGE_EXECUTE_RUNS=false`）或正在优雅停机时启动运行；异源请求 | 改到执行实例 |
| `404` | 资源不存在 | Run / 需求 / SOP / 产物 / 子路径不存在 | 先列表确认 ID |
| `405` | 方法不支持 | 路径存在但方法不对（正常使用不应出现） | 检查工具用法 |
| `409` | 状态冲突 | SOP 乐观并发失败、需求已派发过、需求交付闭环或 GitHub Issue 已关闭、澄清未达标、SOP 有进行中运行或被需求绑定、调度器已关闭 | 按下表恢复 |
| `500` | 内部错误 | 流式输出不受支持、澄清执行器失败 | 退避后重试 |
| `504` | 超时 | AI 澄清超过 10 分钟 | 简化需求描述后重试 |

`409` 的恢复路径：乐观并发冲突 → 重新 `workflows_get` 后基于最新 `revision` 保存；已派发 → 显式 `rerun=true`；交付闭环 → 先 `issues_reopen`；GitHub Issue 已关闭 → 在 GitHub 侧重开；澄清未达标 → 先补答案并 `issues_clarify`；SOP 被占用 → 先终止运行或改绑需求；调度器关闭 → 改到启用调度的实例。

### 8.3 客户端处理约定

- 只对 `500` 和网络层错误做指数退避重试；`409` 必须按具体原因走恢复路径，盲目重试会持续失败。
- 判断成败以 `isError` 和 `structuredContent.status` 为准，不要解析中文错误文案。
- 传输层的 `401` / `403` 不会伪装成工具成功，务必单独处理。

---

## 9. 兼容与变更策略

- 工具名、参数结构（`path` / `query` / `body`）、资源 URI 模板属于公开契约，只增不改；废弃需保留别名至少一个大版本。
- 工具入参只新增可选字段；新增必填字段等同破坏性变更。
- 返回结构可新增字段，客户端必须容忍未知字段。
- 权威清单是 `GET /api/capabilities` 与 `internal/application/operations.go`；本文与其冲突时以代码为准，并应同步修订本文。

## 10. 相关文档

- [docs/api/README.md](api/README.md)：HTTP API 详解与 OpenAPI 定义，MCP 工具的参数细节都在这里。
- [architecture.md](architecture.md)：SOP / 角色 / 执行器 / 沙箱 / 产物的分层契约与执行路径。
- [observability.md](observability.md)：遥测事件契约，对应 `runs_telemetry_list` 与 `observability_spec_get`。
- [../README.md](../README.md)：启动参数、账号登录与 GitHub 同步配置。
