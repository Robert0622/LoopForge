# 侧边 AI 助手

顶栏「AI 助手」提供平台问答，并帮人把工作流从环境资产到 SOP 搭建起来。打开后作为右侧栏挤压主工作区。项目跟随顶栏当前选择；没有 CodeBuddy Key 时输入框禁用。描述需求后可以继续对话修改方案；**对话里**的平台写入先展示草稿，用户确认后通过公开 MCP 写入工具保存。

需求澄清、侧边 AI 助手和定时任务都会把**当前项目已安装的 HTTP MCP**以及**当前用户的个人 HTTP MCP**注入会话级 Session Gateway。模型在生成过程中直接 `tools/call`：查询用 `github__*` / `loopforge__*` 及其他已安装工具。互动助手结束时调用 `submit_assistant_reply`，澄清结束时调用 `submit_clarification`。第三方地址和 Token 仍只留在服务端，mcp.json 只指向 Session Gateway。互动写入仍只进入 `submit_assistant_reply` 的 drafts，用户确认后走现有 `/mcp`。定时任务没有终态提交工具，可直接读写平台和外部 MCP，写入立即生效；`DeferSOP` 继续挡住派发/开工。需求列表的平台 `schedule_jobs` 分两类：触发任务到点扫描需求并派发 SOP（基线冲突会暂停）；AI 定时任务到点用项目 Owner 的 CodeBuddy 跑用户提示词，不派发 SOP。侧栏不再提供独立的助手定时入口。

## 运行要求

- 服务端安装 CodeBuddy CLI，支持 `--mcp-config`、`--strict-mcp-config`、`--disallowedTools`、`--output-format stream-json`、`--include-partial-messages`。可通过 `LOOPFORGE_CODEBUDDY_BIN` 指定可执行文件。CodeBuddy 的 `--tools` 也会挡住 MCP 工具，不能传该参数，不能靠 `mcp__*` 通配来放行。
- 模型列表为内置目录（含 id 与 credits 倍率），默认 `glm-5.3-flash`。用户可在输入框旁切换，图标按厂商显示，选项右侧标注倍率。
- 用户登录后在「个人设置」填写自己的 CodeBuddy API Key 及网络环境。助手只读取当前用户的配置，不回退到项目、其他用户或进程环境中的 Key。未配置 Key 时不能发送。
- CLI 在每用户持久目录（默认 `~/.loopforge/assistant/<userId>/home` 与 `ws`）中运行。CLI 只加载会话级 `loopforge_session` MCP，模型在生成过程中直接对该 Gateway 做 `tools/call`。不传 `--tools` 放行单（该参数会把 MCP 从 function schema 里拿掉），用 `--disallowedTools` / settings deny 关闭 Bash、Write、Edit 等危险内置工具。凭据仍留在服务端，mcp.json 不含 Authorization，stdio MCP 不会启动。该目录只是 CodeBuddy `--resume` 用的运行缓存。对话里的平台资产草稿仍由用户在界面确认后走现有 `/mcp`；定时任务直接调用平台和外部写入工具立即落库。

## 接口

`POST /api/assistant/chat`，要求登录 Cookie 或有效个人 Token。浏览器发送 `Accept: text/event-stream` 时返回思考过程（`thought`，`kind=thinking/tool/status`）与最终回答的 token 增量（`kind=answer`），最后发 `result`；否则返回单个 JSON。请求可带 `sessionId`、`model`、`approvalMode`。

**审批模式**（`approvalMode`，前端在输入框旁切换并记在浏览器本地）：

- `confirm`（默认，每次确认）：平台写入一律进入草稿；每轮最多 1 个草稿，用户确认保存后**前端自动续跑下一轮**继续计划中的下一步（连续续跑上限 30 轮，手动发送消息会重置；随时可点停止或发消息接管）。
- `assisted`（帮我批准）：保存类等低风险草稿由平台自动执行（回执形式展示），删除/发布/派发/启动等危险操作以及存在依赖冲突的草稿仍逐个确认。
- `auto`（自动执行）：全部草稿由平台直接执行，不弹审批卡片，只展示执行回执。

**依赖预检**：草稿下发前服务端会复刻 catalog 删除端点的引用检查（角色被 SOP 引用、共享资产、SOP 绑定未关闭 Issue、内置资产等），并校验 `workflows_save` 草稿的节点完整性（agent 节点必须绑定已存在的角色、script 节点必须有脚本），把执行必然失败或残缺的原因以 `warnings` 随草稿返回并在卡片上高亮。带警告的草稿即使在 auto 模式也不会自动执行，降级为带警告的确认卡片。系统提示词同时要求模型在提交删除类草稿前先用只读工具自行检查引用关系。

`GET /api/assistant/models` 返回进程内缓存的模型目录。

`GET /api/assistant/sessions`、`GET|PUT|DELETE /api/assistant/sessions/:id` 读取、回写或删除当前用户的对话。对话存在 MySQL `assistant_sessions`（消息在 `payload`）。删除时同时去掉该用户 PrivateHome 下 CodeBuddy transcript（`home/.codebuddy/projects/<cwd>/<sessionId>.jsonl`），避免 `--resume` 读到已删对话。保存草稿后会回写会话，恢复历史时保留「已保存 / 打开」状态。

`GET|PUT /api/assistant/tasks`、`DELETE /api/assistant/tasks/:id`、`POST /api/assistant/tasks/:id/run` 仍保留给 MCP/旧草稿，Studio 侧栏不再管理，进程也不再自动到点执行。需要定时派发 SOP 时用需求列表的触发任务；需要定时跑提示词时用 AI 定时任务（`skipDispatch`）。侧栏宽度可拖左边框调节，并记在浏览器本地。

请求：

```json
{"projectId":"project-id","sessionId":"af…","model":"glm-5.3-flash","messages":[{"role":"user","content":"帮我构建代码评审角色"}]}
```

返回：

```json
{"sessionId":"af…","message":"已生成评审角色草稿，请预览后保存。","drafts":[{"tool":"agents_save","body":{"id":"project-reviewer","name":"代码评审","projectId":"project-id","executorId":"codebuddy","instructions":"检查正确性、安全性与测试覆盖"}}]}
```

仅接受 user / assistant 消息，请求体遵循普通 `/api` 的 2 MiB 上限；单用户同时最多一次生成，超时 30 分钟，客户端停止会取消请求。错误使用 400（缺少 Key/输入非法）、401（未登录）、404（项目不存在）、429（已有生成）、502（模型失败或格式无效）、504（超时）。不向客户端返回 CLI 原始错误或密钥。

多轮对话只把**本轮用户输入**放进 `-p`。总提示词只通过 `--system-prompt` 注入，不写入每一轮 user 内容。CodeBuddy 侧使用 `--session-id` / `--resume`。GitHub/平台查询由 CodeBuddy 对 Session MCP 连续 `tools/call` 完成，不再把查询打回 HTTP 再开一轮。CLI 没有无 ID 的非交互 resume；交互式 `--resume` 只适用于 TUI。历史列表以平台自己的 JSON 为准。若 CLI transcript 丢失，会回退为不 resume，并把完整对话（仍不含总提示词）放进 `-p` 继续。

## 验证

常规回归：`go test ./...`。

真实模型验收需要可用的 CodeBuddy API Key：登录后在「个人设置」填写 Key，打开侧边助手生成一个角色和一条 SOP，确认草稿经 MCP 保存成功。

## 根据代码回答问题与拦截写操作

部署时把与运行版本一致的源码快照（例如 `git archive HEAD` 导出的 `cmd/internal/web/docs/migrations`、README 与 Go 模块文件）放到只读目录，保证问答源码对应构建版本。快照不应包含 `.git`、生产环境的 `.env`、部署凭据或 TLS 私钥。

通过 `LOOPFORGE_ASSISTANT_SOURCE_DIR` 指定源码位置。平台与外部 MCP 查询走 Session MCP 的 `tools/call`。返回 `sources` 包含实际检索的文件、行号和片段，侧栏可展开查看。服务端使用 `os.Root` 限定根目录，拒绝绝对路径、路径穿越、隐藏文件、符号链接、非文本扩展、非常规文件和超大文件；限制结果大小与总上下文。

CLI 工作区是每用户持久的可写目录，而不是源码快照。PrivateHome 不再每次清空，以便 `--resume` 读到先前 transcript。未配置源码时仍可问答与构建，但助手会明确不能声称已阅读代码。

拦截不是关键词黑名单。CLI 只绑定当前用户 CodeBuddy Key，不绑定 Git 凭据或遥测，也不把第三方 MCP 地址写进 mcp.json。模型必须直接 `tools/call` Session Gateway：澄清只读；互动写入不能直接调平台写工具，只能经 `submit_assistant_reply` 进入 drafts；定时任务可直接调平台和外部 MCP 写入。Gateway 用当前用户凭据访问外部 MCP 与平台，并做项目范围检查和结果脱敏。stdio MCP 拒绝。Bash/Write 等未知工具仍被拒绝。带 `projectId` 的草稿必须属于当前项目；`projects_save` / `repositories_save` 会剥掉 token 字段。对话里的平台资产草稿仍由用户确认后走现有 `/mcp`；定时任务会把同一批写入工具立即执行。源码快照保持只读。
