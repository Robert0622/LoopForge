# 业务域第一版

## 产品口径

一个项目最多归属一个主业务域，空 ID 表示未分类。业务域为单层、受管理的业务汇报维度，不改变项目的执行、仓库或成员边界。

看板和重新导出的报告全部按项目当前归属汇总：项目从 A 移到 B 后，过去时间段的执行也跟随迁移；全局总量不变。已下载的 HTML 文件是生成时的静态副本。统计时间复用现有 Analytics 的运行创建时间口径。关联需求是统计期内关联执行的去重需求，不等同于期间新增需求；交付采用现有交付判定。

项目配置在默认代码仓库旁提供主业务域选择，随「保存项目设置」统一保存；新项目也可一并设置首次归属。左侧「平台」分组提供独立的「业务域管理」入口，维护分类及授权；业务域页面仅提供汇总报告。业务域管理员只能访问自己获授权的业务域管理，「平台管理」入口仅平台管理员可见。

页面支持日期范围、全部时间、业务分布、运行趋势、项目贡献、HTML 报告导出。Token 占比的分母为当前可查看范围，不宣称代表人力投入。成本未识别会明确标记；没有数据时不会用演示快照填充。

## 权限

部署时需启用 `LOOPFORGE_AUTH_REQUIRED=true`；未启用认证时遵循现有开发模式，允许配置和查看。

| 能力 | 平台管理员 | 业务域管理员 | 汇报查看者 | 项目 Owner |
| --- | --- | --- | --- | --- |
| 创建、启停业务域，配置管理员 | 是 | 否 | 否 | 否 |
| 维护名称、说明、查看者 | 是 | 本域 | 否 | 否 |
| 设置未分类项目归属 | 是 | 否 | 否 | 本项目 |
| 迁移项目或清除归属 | 是 | 否 | 否 | 否 |
| 查看业务汇总／导出 | 全部 | 本域 | 授权域 | 仅自己拥有的未分类项目；其他域需授权 |
| 查看归属变更历史 | 是 | 否 | 否 | 本项目 |

停用保留原有项目和统计，禁止新的项目归入；可以将项目迁出。没有物理删除入口。支持多个管理员及多个查看者，授权用户须存在且不能重复。业务域汇总不返回个人使用明细、需求文本、Run 或日志。

普通项目写入不能直接改归属；生产认证模式下，现有项目设置仅允许 Owner 或平台管理员修改，新建项目的 Owner 默认为当前用户（平台管理员可指定）。数据库事务内重新校验首次归属权限，compare-and-swap 防止并发迁移覆盖，配置更新也校验事务内版本。归属调整原因可选，记录操作者和前后业务域。普通项目保存即使携带旧值也不会覆盖归属。

以上为新增业务域链路的权限；既有其他项目读取、分析和执行接口的完整隔离未在本版本重构，不应将业务域授权当作全平台的数据隔离机制。

## API / MCP

接口与工具清单见 `docs/api/openapi.yaml`、`docs/mcp.md`。

- `GET /api/business-domains`：分类、canAdmin、各域 canManage/canView。
- `PUT /api/business-domains`：保存业务域；管理员列表／启停仅平台管理员可改。
- `GET /api/business-domain-report?businessDomainId=all|unclassified|ID&from=RFC3339&to=RFC3339&grain=day|week|month`：汇总数据；all 始终收敛为调用者的授权范围。
- `PUT /api/projects/:id/business-domain`：`{businessDomainId, expectedDomainId, reason}`；空目标为清除归属。
- `GET /api/projects/:id/business-domain/history`：最近 100 条变更。

汇总率和分位数复用完整样本计算，不平均各项目结果。服务端项目范围 `nil` 表示原有不限定范围，非 nil 空列表表示零项目，不能退化为全平台查询。旧执行只有 issueId 的情况仍通过需求查找项目。

## 数据与部署

业务域配置由 `BusinessDomainsRepo` 管理；归属字段、调整及历史由 `ProjectsRepo` 管理；汇总由 `AnalyticsRepo` 管理。对应独立 businessDomainsHandlers 和 projectsHandlers，项目 handler 从 catalog 中归位。

启动时由现有 GORM 增量 DDL 自动补齐两张新表、projects.business_domain_id 字段及索引，无需手动执行 SQL 脚本。已有项目默认未分类，不重写历史 Run／需求，也不运行数据迁移或 seed。

## 本地验证

必须使用隔离 Docker MySQL，不能使用实际环境 DSN。以下示例创建新容器及全新测试数据库，容器名／端口如有占用须调整：

```sh
docker run -d --name business-domains-test-mysql -p 127.0.0.1:33078:3306 \
  -e MYSQL_ROOT_PASSWORD=local-test-password -e MYSQL_DATABASE=business_domains_test mysql:8.4
# 待 mysqladmin ping 确认就绪后执行；行为测试目前要求全新测试数据库。
LOOPFORGE_BUSINESS_TEST_MYSQL_DSN='root:local-test-password@tcp(127.0.0.1:33078)/business_domains_test?parseTime=true&loc=UTC' \
  go test ./internal/httpapi -run TestBusinessDomains -count=1
go test ./internal/httpapi ./internal/repo ./internal/domain ./internal/application ./internal/mcpserver ./docs/... ./web
```

本地服务以显式 DSN、`-role api`、`-addr 127.0.0.1:18088` 启动；`LOOPFORGE_SCHEDULER_ENABLED=false` 避免调度。若需浏览器验收认证，手动在隔离数据库建立测试用户，设置 `LOOPFORGE_LOCAL_LOGIN_USER` 和本地会话密钥后使用本地验收登录。登录仍由已有接口限制为直接 loopback 且同源请求；新增启动入口只在 loopback、认证开启且本地验收登录显式启用时出现。

管理页采用左侧可点击列表、右侧编辑表单；切换时暂存未保存内容，刷新页面会清除草稿。新建使用 `PUT /api/business-domains?createOnly=true`，已有标识返回 409，避免覆盖原有配置。

用户授权采用姓名或账号搜索，候选最多展示 20 条，已选用户显示为可移除标签；跨角色重复选择会禁用。搜索使用现有 bootstrap 用户目录，不一次渲染完整目录。

看板在全部业务范围展示跨业务分布；选定单一业务域（含未分类）后改为域内项目用量分布，隐藏重复的业务分布表和项目表的主业务域列。导出报告采用相同展示规则。
