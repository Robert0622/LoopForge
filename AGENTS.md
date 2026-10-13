# AGENTS.md

本文件面向所有在仓库内工作的 AI agent 与人类贡献者，约定 repo 层与 handler 层的结构规则。
新增代码必须遵守；发现历史偏差时先归位再继续。

## repo 层（internal/repo）

每个领域一个 Repo，文件与方法一一归属：

| 领域 Repo | 拥有 |
|---|---|
| `CatalogRepo` | agents/rules/skills/MCP servers/executors/dockerfiles/environments/sandboxes/workflows/repositories/sync config/schedule jobs/definition revisions/generic templates |
| `ProjectsRepo` | projects、project members |
| `IssuesRepo` | issues、issue comments |
| `TasksRepo` | tasks（跨实例幂等记录） |
| `UsersRepo` | users、credentials、platform admins |
| `TokensRepo` | API tokens |
| `AssistantRepo` | Studio 助手会话、定时 prompts |
| `RunsRepo` | runs 及其子资源、run leases、workflow metrics、run evals |
| `RunnersRepo` | CLI runner 注册表、runner tasks/events |
| `DeliveryRepo` | MR delivery 状态与锁 |
| `AnalyticsRepo` | 只读运营分析视图 |
| `PreviewsRepo` | run previews |
| `EngineInstancesRepo` | engine instances |

规则：

1. **禁止在 `Store` 上新增领域方法。** `Store`（repo.go）只做两件事：
   - 组合各领域 Repo（匿名嵌入）与连接生命周期（`Open`/`Close`）；
   - 必须跨领域合并的视图（现已全部降级进领域 Repo，如
     `CatalogRepo.Workflow`/`Workflows` 合并 generic_templates）。
   - 合并视图的行为契约：`CatalogRepo.Workflows` 先按 `UpdatedAt` 排序
     workflows 表行，generic 模板恒追加在尾部（不得混入排序）；
     `Workflow(id)` generic 模板优先于同 ID 物理行；写路径
     （`PutWorkflow`/`DeleteWorkflow`）经 `storedWorkflow` 只看物理表。
2. 新实体先找所属领域 Repo；确实没有就新建一个 `XxxRepo`，并在 `newStore`
   中装配。领域间协作用 `newStore` 里的显式指针注入（如 `RunsRepo` 持有
   `issues`/`catalog`），不允许循环依赖。
3. 领域 Repo 的共享依赖只有 gorm 句柄（`deps`），不要往 `deps` 加别的。
4. 写一个文件挂多个领域 receiver 是坏味道；同一领域的方法可分布在多个文件
   （如 `runs.go`/`run_evals.go` 都属 `RunsRepo`），但文件不得为其他领域定义方法。

## 迁移与 seed（migrations/）

1. **代码不做数据迁移。** `repo.Open()` 只负责连接和 gorm
   AutoMigrate 的增量 DDL（补缺失的表/列/索引）。基线 schema、数据迁移
   （改数据、删行、删列）和内置默认资产之外的 seed 数据一律写成 `migrations/` 下的 SQL
   脚本，由运维**手动执行**，部署时不会自动跑。
2. 每个分支自带自己的迁移脚本，不做任何自动化生成（没有 seedgen 之类的
   工具）。脚本命名 `NNN_描述.sql`，按序号递增。
3. 禁止新增 `ensureXxx`/`migrateXxx`/`seedXxx` 之类的启动自愈逻辑写入
   生产路径。唯一例外是 `internal/repo/bootstrap.go` 的 `Bootstrap`：它只在
   缺失时插入内置默认资产（平台项目、执行器、沙箱、通用 SOP 模板等），
   从不覆盖运维修改，让空库开箱即用。新增默认资产放进它或
   `internal/repo/templates/*.json`，不要另起入口。测试专用的默认数据只
   存在于 `internal/repo/opentest.go`（`OpenTest`，仅测试调用，生产代码
   不得引用）。

## handler 层（internal/httpapi）

1. 每个领域一个 handler struct（`authHandlers`、`usersHandlers`、
   `catalogHandlers`、`issuesHandlers`、`tasksHandlers`、`runsHandlers`、
   `previewsHandlers`、`streamHandlers`、`runnersHandlers`、
   `scheduleHandlers`、`tokenHandlers`、`syncHandlers`、
   `deploymentMetricsHandlers`、`assistantHandlers`、`mcpHandlers`、
   `marketHandlers`…），在 `server.go` 的 `Server` 中以指针装配。
2. **新领域必须新建自己的 handler struct**，不要把方法挂到现有领域 handler
   或 `Server` 上。
3. 同一领域的多个文件允许（`run_eval.go`/`run_workspace.go` 属
   `runsHandlers`），但一个文件不得为别的领域 handler 定义方法。
   纯共享的包级 helper（无 receiver）可以放独立 helper 文件。
4. `Server` 只保留服务级端点与入口装配（`health`、`capabilities`、
   `openAPI`、`bootstrap`、`observabilitySpec`、`otel` 等）。
5. 跨领域协作走显式注入（如 `issuesHandlers` 持有 `tasks`/`stream`），
   与 repo 层同构。

## 本地运行与验证

- 体验（不需要 API Key）：`./start.sh -y`（缺 Docker 等会自动装），打开它打印的地址（默认 <http://localhost:8080>，被占用会自动换端口），第一个访问的人创建管理员账号；`./start.sh doctor` 做端到端自检。也可用 `curl -fsSL https://raw.githubusercontent.com/Tencent/LoopForge/main/install.sh | bash -s -- -y`。
- 默认的 `docker-compose.yml` 挂载宿主机 Docker socket，每次运行一个容器；这等同于宿主机 root 权限，只在专用或个人机器上使用。不需要时删掉 `user: root` 和 `docker.sock` 两行。
- 从源码运行：`make run`（数据在 `~/.loopforge`，监听 `:8080`），`make demo` 为演示模式。
- 提交前：`make fmt`、`make vet`、`make test`；改动 Web 或 API 时再跑 `make e2e`。
- 公开仓库不得出现内部主机、系统或地址：`scripts/check-internal-refs.sh` 必须通过。
