# GitHub 同步 API

认证和通用状态码见 [README](README.md)。同步服务通过 GitHub REST API 读写 Issue 与 Pull Request：服务端令牌来自 `LOOPFORGE_GITHUB_TOKEN`（或 `GITHUB_TOKEN`），项目内的仓库绑定决定拉取范围。GitHub Enterprise Server 通过 `apiBaseUrl` 或 `LOOPFORGE_GITHUB_API_URL` 指向 `https://<host>/api/v3`。

## 同步配置

`SyncConfig` 字段：

```json
{
  "playbookSeedRev":1,
  "repoPath":"acme/widget",
  "apiBaseUrl":"",
  "autoSyncEnabled":true,
  "autoSyncIntervalSec":300,
  "lastSyncAt":"2026-09-02T12:00:00Z",
  "lastSyncStatus":"success",
  "lastSyncError":""
}
```

`apiBaseUrl` 为空时使用 `https://api.github.com`。

### `GET /api/sync/config`

返回当前配置，`200`。

### `POST|PUT /api/sync/config`

两种方法语义相同。保存配置并重新配置同步服务、启动/刷新自动同步。`autoSyncIntervalSec<=0` 时设为 `300`；客户端提交的 `lastSyncAt`、`lastSyncStatus`、`lastSyncError` 会被忽略并保留服务端当前值。

成功 `200` 返回保存后的 `SyncConfig`；JSON/存储错误 `400`。

```bash
curl -u "$LOOPFORGE_USER:$LOOPFORGE_PASS" -X PUT \
  -H 'Content-Type: application/json' \
  -d '{"autoSyncEnabled":true,"autoSyncIntervalSec":300}' \
  "$LOOPFORGE_URL/api/sync/config"
```

## 手动拉取

### `POST /api/sync/now`

无请求体。在后台对所有已绑定仓库启动一次全量 pull，并立即返回：

```json
{"status":"started","syncing":true}
```

正常状态码 `200`。它是异步长任务启动接口：`200` 只表示任务已启动，不表示同步成功，也不返回任务 ID。用 `/api/sync/status` 轮询 `syncing` 和配置中的最近结果；也可查看 `/api/sync/logs`。拉取结束后会顺带对账已合并的 Pull Request，把对应需求标记为交付闭环。

```bash
curl -u "$LOOPFORGE_USER:$LOOPFORGE_PASS" -X POST \
  "$LOOPFORGE_URL/api/sync/now"

curl -u "$LOOPFORGE_USER:$LOOPFORGE_PASS" \
  "$LOOPFORGE_URL/api/sync/status"
```

同一时刻只允许一次同步；同步服务内部负责并发互斥。

## 按 URL 导入单个 Issue

### `POST /api/issues/import`

```json
{"projectId":"platform","url":"https://github.com/acme/widget/issues/42","repositoryIds":["repo-widget"],"workflowId":""}
```

`projectId`、`url` 和至少一个仓库必填。导入后按标题与描述立即做澄清评分；未达项目准入线的需求会自动排队澄清。成功 `200` 返回保存后的 Issue。

### `POST /api/issues/{issueId}/resync`

从 GitHub 重新拉取单个已关联 Issue 的标题、描述、状态和标签。

## 状态与日志

### `GET /api/sync/status`

返回 `200`：

```json
{
  "syncing":false,
  "config":{"lastSyncStatus":"success","lastSyncAt":"..."},
  "logs":[]
}
```

`logs` 为最近同步记录，格式同下。

### `GET /api/sync/logs`

返回最近最多 100 条 `SyncLog[]`，按新到旧排列，不提供游标分页：

```json
[
  {
    "id":"sync-...",
    "direction":"pull",
    "source":"github",
    "action":"sync_issues",
    "status":"success",
    "count":12,
    "message":"...",
    "startedAt":"...",
    "finishedAt":"..."
  }
]
```

正常状态码 `200`。服务内部最多保留 200 条，但 API 只暴露最近 100 条。

## 将已有需求推送为 GitHub Issue

### `POST /api/sync/create`

请求：

```json
{"issueId":"issue-1","target":"github"}
```

- `issueId`、`target` 必填，`target` 只能为 `github`。
- Issue 未绑定仓库时继承项目默认仓库；最终必须有可用的 GitHub 仓库。
- 已手动填写统计关联 ID 的平台需求返回 `409`，需先清空关联 ID。

成功 `200` 返回带 `source` 和 `externalRef` 的已保存 Issue；字段错误/外部创建失败通常 `400`；Issue 不存在 `404`。

```bash
curl -u "$LOOPFORGE_USER:$LOOPFORGE_PASS" -X POST \
  -H 'Content-Type: application/json' \
  -d '{"issueId":"issue-1","target":"github"}' \
  "$LOOPFORGE_URL/api/sync/create"
```

新建需求时也可以在 `POST /api/issues` 中直接传 `syncTarget`（`local` 或 `github`），详见 [协作与需求 API](collaboration.md)。

## 与定时任务的关系

自动同步配置控制周期性 pull；`/api/schedule-jobs` 则从本地需求池挑选候选项并派发指定 SOP，两者不是同一任务模型。定时任务接口见 [编排与运行 API](orchestration.md)。

同步接口没有 SSE、文件下载或分页端点。内部 Worker signals、OTLP、实例 lease 和沙箱回收均不属于本页公共 API。
