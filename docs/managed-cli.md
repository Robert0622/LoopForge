# 托管 Agent CLI 自动更新

托管更新只管理平台内置 runner 使用的 Codex、CodeBuddy、Claude Code，三者默认自动更新，没有更新策略配置。个人 [bind-cli 安装器](../deploy/bind-cli.sh)、用户机器和 Docker 任务不参与更新；CLI 可运行不等于模型账号已授权。

## 启用条件

- 只有 `role=worker` 或 `role=all`、会执行运行（`executeRuns=true`），并且显式设置了绝对路径 `LOOPFORGE_MANAGED_CLI_ROOT`，才会在 worker 进程内启动更新循环。API 进程、个人 runner 和未设置 root 的实例都不会启动。root 是托管身份与共享目录，不是更新策略；没有额外服务、HTTP 管理接口或数据库种子数据。
- worker 与内置 runner 必须使用同一个 root，并且共享同一台执行机器、文件系统和运行用户；容器部署时需挂载同一个卷。程序不会推断默认 root，例如可以使用 `/srv/loopforge/managed-cli`。
- 核心逻辑按需以 0700 权限创建 root 与 updater-home，拒绝目录软链接；已有目录的权限不会被覆盖。部署过程本身不需要执行 CLI 更新。
- API 进程也可以设置同一个 `LOOPFORGE_MANAGED_CLI_ROOT`，此时它只读取 worker 发布的当前清单，不安装也不更新。AI 助手、澄清与 SOP 汇报通过共享的 CodeBuddy 查找逻辑使用托管版本：每次新调用读取当前版本，已经开始的调用继续使用选定的不可变版本路径。显式设置的 `LOOPFORGE_CODEBUDDY_BIN` 优先；未配置托管 root 的本地环境继续按 PATH 和常见安装路径查找。配置了 root 后，如果清单损坏或 CodeBuddy 尚未安装，会明确报错，不回退到旧的全局 CLI。
- Node/npm 从服务的 `PATH` 中发现，要求 Node >= 22。更新器不安装、更新或删除全局 CLI。
- 首次激活前仍可使用已发现的主机 CLI；一台没有任何 CLI 的新机器需要等待自动检查，或由管理员手动更新。部署不执行首次更新，也不以更新结果作为健康检查条件。
- worker 与内置 runner 需同步升级。三把永久锁 `updater.lock`、`maintenance-intent.lock`、`usage.lock` 不得手动删除。领取任务时先经过短暂的 intent 入口取得 usage 租约，租约覆盖领取、执行和完成；维护流程先独占 intent 拒绝新的领取，再独占 usage 等待已有请求和任务结束。已经发出的长轮询也算在途请求，允许按原版本正常完成。

## 固定调度

固定使用官方 npm 源 `https://registry.npmjs.org`，候选版本为不超过正式 latest、且发布至少 24 小时的稳定版本；不支持覆盖 registry、禁用某个 provider、固定版本或自定义验证命令。维护等待最多 30 分钟，单个外部命令最多 10 分钟。

调度固定为每天 UTC+8 04:00，加 0 到 10 分钟的确定性散列抖动，与主机时区无关。启动时如果已错过当天时刻会补查一次，未到则等待当天窗口，不追补历史天数；检查结束后只设一个定时器等待下一日窗口，不做每分钟轮询。

`Manager.Check` 在共用的 `updater.lock` 内先把当天的领取写入 `State.LastCheck`，避免并发或崩溃重启导致重复执行。失败或中断同样消耗当天的领取，通常次日重试；管理员可以随时手动更新。手动 update/rollback 不受每日限额约束，也不影响自动调度。

旧的试验配置文件和 `LOOPFORGE_MANAGED_CLI_CONFIG` 不再读取，无需迁移。要停用整个托管功能，从 API、worker 和内置 runner 三者的环境中移除 `LOOPFORGE_MANAGED_CLI_ROOT` 并重启即可，不需要删除托管数据。

## 管理命令

使用 loopforge 主程序，与 worker 使用同一个 root。命令加载 `.env` 后直接执行并退出，不连接数据库，也不启动 HTTP 或引擎。管理员 shell 同样需要正确的 Node/npm PATH。

```sh
export LOOPFORGE_MANAGED_CLI_ROOT=/srv/loopforge/managed-cli
loopforge -cli-action=status
loopforge -cli-action=update -cli-timeout=80m
loopforge -cli-action=rollback -cli-provider=codex
```

stdout 输出 JSON，失败时返回非零退出码。status 只读状态，不创建 root，也不要求 Node/npm。update 默认处理全部三个 provider；rollback 必须指定 `codex`、`codebuddy` 或 `claude-code`，`-cli-provider` 只用于 rollback。没有 pin 或 force 参数。`-cli-timeout` 默认 80 分钟且必须为正数，SIGINT/SIGTERM 会取消操作和外部命令。

rollback 需要上一份托管版本，旧的全局 CLI 不算。回滚会把当前版本记为 rejected，之后自动和手动 update 都不会再选它，直到出现更新的正式版本。回滚只影响新的运行；已有运行（含挂起后恢复的）保持各自持久化的执行版本。不要删除仍被运行中或挂起运行引用的版本目录，旧目录不会被自动清理。

## 权限与验证

更新器使用独立的 updater-home、npm 配置和缓存，并通过白名单净化环境变量，不继承模型凭据、平台 token、任务 API key、runner 登录 HOME、`NODE_OPTIONS` 或任意 npm 配置；代理和 CA 只做最小化显式传递。

这些措施不是操作系统级沙箱：npm 生命周期脚本与 CLI 探测以 worker 的系统权限运行，不能认为它们只能访问托管 root。

安装在托管版本目录中使用非全局 npm：先 `npm install --ignore-scripts` 并校验版本、SHA512 SRI 和 lock 文件，再执行必要的生命周期脚本、复查 lock 并验证 CLI；不覆盖全局 CLI。完整性校验不等于代码可信，仍需信任 CLI 发布方。

验证只检查版本和参数兼容性，不调用模型，也不保证模型授权。排查问题时查看 worker 的服务日志。

不产生部署副作用的聚焦测试：

```sh
go test ./cmd/loopforge ./internal/managedcli ./internal/runner -run 'TestManagedCLI|TestCLIAdmin|TestResolveRole' -count=1
```
