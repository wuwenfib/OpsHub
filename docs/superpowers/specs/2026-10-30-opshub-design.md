# OpsHub 设计文档

日期：2026-10-30　范围：总体架构 + 阶段 1（监控链路）/ 阶段 2（后端核心）细化；阶段 3/4 只定接口边界。

需求背景、技术栈、端口见 `CLAUDE.md`；任务清单与验收标准见 `docs/PLAN.md`。本文只写"怎么做"。

---

## 1. 总体分层与告警数据流

### 1.1 容器

| 角色 | 容器 |
|---|---|
| 被管对象 | mock-gov-app、MySQL（被监控的"业务库"） |
| 工具层 | node_exporter、mysqld_exporter、blackbox_exporter、Prometheus、Alertmanager、Grafana |
| 管理平台 | opshub（Go 单体）、PostgreSQL（平台自己的库） |

MySQL 只是"被看的对象"，PostgreSQL 是"平台的账本"，两者互不相干。

### 1.2 后端分层

依赖方向只能自上而下：

```
api/       gin handler：解析请求 → 调 service → 错误映射为 HTTP 状态码
service/   用例编排：开事务 → 调纯逻辑 → 写库（AlertService / TicketService / StatsService / CMDBService）
cmdb/ ticket/ sla/ alert/   纯领域逻辑，零 IO，可单测
store/     sqlc 生成的查询 + 事务封装（pgx）
```

数据访问用 **sqlc + pgx**：手写 `.sql`，生成带类型的 Go 函数，不用 ORM。

### 1.3 告警 → 工单链路（`POST /api/alerts/webhook`）

1. `alert` 包把 Alertmanager payload 解析为 `[]Alert{Fingerprint, Status, Labels, Annotations, StartsAt}`，一次 webhook 可含多条，逐条处理。
2. 用 `labels.system` 查 `business_system.code`；查不到时工单 `system_id = NULL`，原始值存入 `system_label`。
3. 每条告警一个事务：
   - `firing` 且该 fingerprint 无未关闭工单 → 按"系统等级 × severity"查 `sla_policy` 算两个 deadline → 新建事件单 + 一条 `ticket_log`；
   - `firing` 且已有未关闭工单 → 只追加"重复告警"日志；
   - `resolved` → 有未关闭工单则追加"告警已恢复"日志，不改状态；无则忽略。
4. **并发去重靠数据库**：部分唯一索引 `UNIQUE (alert_fingerprint) WHERE status <> 'closed'`，插入用 `ON CONFLICT DO NOTHING`，冲突则退回"追加日志"分支。
5. 除 payload 无法解析（400）外，webhook 一律返回 200，避免 Alertmanager 重试风暴。单条告警处理失败记错误日志。

纯逻辑部分：`alert.Decide(a Alert, open *Ticket) Decision`，返回 `CreateTicket | AppendDuplicate | AppendRecovered | Ignore`。

### 1.4 已定口径

- `closed` 是终态。同 fingerprint 再次 firing → 新建工单。
- `reopened` 仅用于人工对 `resolved`（未关闭）的单子说"没修好"。

---

## 2. 数据模型与状态机

### 2.1 表

| 表 | 字段 | 约束 |
|---|---|---|
| `vendor` | id, name, contact, phone, contract_end (date) | name 唯一 |
| `business_system` | id, code, name, department, level, vendor_id, owner, description | code 唯一；level CHECK 1–3；vendor_id 可空 FK |
| `host` | id, ip, hostname, os, env | ip 唯一；env ∈ {prod, test} |
| `system_host` | system_id, host_id | 联合主键，级联删除 |
| `system_dependency` | system_id, depends_on_id | 联合主键；CHECK system_id <> depends_on_id |
| `sla_policy` | level, severity, response_minutes, resolve_minutes | 主键 (level, severity)，12 行 |
| `ticket` | id, type, title, system_id (可空), system_label, severity, status, source, alert_fingerprint (可空), assignee, created_at, responded_at, resolved_at, closed_at, response_deadline, resolve_deadline | 部分唯一索引见 1.3 |
| `ticket_log` | id, ticket_id, actor, action, from_status, to_status, note, created_at | |

- `type` ∈ {incident, problem, change}；`severity` ∈ {P1..P4}；`source` ∈ {alert, manual}。
- `ticket_log.actor`：`alertmanager` / `system` / 操作人姓名。`action`：`created`、`duplicate_alert`、`alert_resolved`、`ack`、`resolve`、`close`、`reopen`。
- 所有时间列 `timestamptz`，按 UTC 写入。

### 2.2 默认口径

- 工单 severity 取告警 `severity` 标签；缺失或非法按 P3，并在日志注明。
- 未登记系统按**二级系统**的 SLA 计时。
- 无登录：请求体 `operator` 字段为操作人；ack 时写入 `assignee`。
- 只实现 incident 的状态机；problem/change 可手工建单但不能流转（返回 409）。

### 2.3 SLA 策略种子

| 等级 \ 级别 | P1 | P2 | P3 | P4 |
|---|---|---|---|---|
| 一级 | 15m / 2h | 30m / 4h | 1h / 8h | 4h / 24h |
| 二级 | 30m / 4h | 1h / 8h | 2h / 24h | 8h / 48h |
| 三级 | 1h / 8h | 2h / 24h | 4h / 72h | 24h / 120h |

格式：响应时限 / 恢复时限，按自然时间计算。

### 2.4 事件单状态机

| 当前 | 动作 | 下一状态 | 附带效果 |
|---|---|---|---|
| new | ack | acknowledged | 设 responded_at、assignee |
| acknowledged | resolve | resolved | 设 resolved_at |
| reopened | resolve | resolved | 设 resolved_at |
| resolved | close | closed | 设 closed_at（释放去重锁） |
| resolved | reopen | reopened | 清空 resolved_at（历史留在 log） |

表外组合一律 `ErrInvalidTransition` → 409。纯函数 `ticket.Transition(cur Status, act Action) (Status, error)`。

### 2.5 SLA 指标（查询时计算，无定时任务）

- 响应达标：`responded_at <= response_deadline`
- 恢复达标：最后一次 `resolved_at <= resolve_deadline`
- 当前超时：`responded_at IS NULL AND now > response_deadline`，或 `resolved_at IS NULL AND status <> 'closed' AND now > resolve_deadline`
- MTTR：已恢复工单 `resolved_at - created_at` 的平均
- 未登记系统在按系统统计中单独成"未登记"分组

---

## 3. 监控侧（阶段 1）

### 3.1 mock-gov-app（:8081）

| 接口 | 行为 |
|---|---|
| `GET /api/apply` | 模拟办件，正常耗时 20–100ms |
| `GET /healthz` | 200 ok |
| `GET /metrics` | Prometheus 指标 |
| `POST /chaos/slow` | 之后每个请求额外 1.5–2s |
| `POST /chaos/error` | 之后 30% 请求返回 500 |
| `POST /chaos/reset` | 恢复正常 |

- 故障状态：`atomic.Int32`（0 正常 / 1 慢 / 2 错误）。
- 指标：`http_requests_total{handler,code}`（Counter）、`http_request_duration_seconds{handler}`（Histogram，桶 0.05–5s，覆盖 1s 附近）、`gov_app_chaos_mode`（Gauge）。
- 内置流量发生器：后台 goroutine 每秒对自身 `/api/apply` 发 `LOADGEN_RPS`（默认 5）次真实 HTTP 请求。
- 配置走环境变量：`LISTEN_ADDR`（默认 `:8081`）、`LOADGEN_RPS`。

### 3.2 system 标签

在 Prometheus 抓取配置的 `static_configs.labels` 里打 `system: GOV-APPLY`，不写进应用代码。同系统的 mock-gov-app、node_exporter、mysqld_exporter、blackbox 探测目标都打同一个值。CMDB 预置 5 个系统，只有 `GOV-APPLY`（一网通办，一级）真正被监控。

### 3.3 告警规则（scrape / evaluation 间隔 15s）

| 告警 | 表达式 | for | severity |
|---|---|---|---|
| GovAppHighLatency | `histogram_quantile(0.95, sum by (system, le)(rate(http_request_duration_seconds_bucket[1m]))) > 1` | 1m | P2 |
| GovAppHighErrorRate | `sum by (system)(rate(http_requests_total{code=~"5.."}[1m])) / sum by (system)(rate(http_requests_total[1m])) > 0.05` | 1m | P2 |
| GovAppProbeDown | `probe_success == 0` | 30s | P1 |
| HostHighCPU | `1 - avg by (system, instance)(rate(node_cpu_seconds_total{mode="idle"}[5m])) > 0.85` | 5m | P3 |

每条规则都带 `system`（来自表达式聚合保留的标签）和 `severity` 标签，以及 `summary` 注解（作为工单标题）。

### 3.4 Alertmanager

- `group_by: [alertname, system]`，`group_wait: 10s`，`group_interval: 1m`，`repeat_interval: 4h`。
- 唯一 receiver：webhook `http://opshub:8080/api/alerts/webhook`，`send_resolved: true`。
- 抑制：同 system 的 `GovAppProbeDown` firing 时，抑制 `GovAppHighLatency` / `GovAppHighErrorRate`。

### 3.5 Grafana

Provisioning 自动配置 Prometheus 数据源 + "系统健康总览"看板（QPS、错误率、P95、CPU/内存、故障状态）。

### 3.6 Makefile 故障注入

- `chaos-slow` / `chaos-error`：`curl -X POST :8081/chaos/...`
- `chaos-down`：`docker compose stop mock-gov-app`
- `chaos-reset`：`docker compose start mock-gov-app` + `curl -X POST :8081/chaos/reset`

刻意不做：CPU 故障注入、多个 mock 实例。

---

## 4. API、错误处理、测试、目录

### 4.1 REST API

统一 `/api` 前缀，JSON，时间 RFC3339 UTC。

| 方法与路径 | 说明 |
|---|---|
| `GET/POST /api/vendors`，`GET/PUT/DELETE /api/vendors/:id` | 厂商 |
| `GET/POST /api/systems`，`GET/PUT/DELETE /api/systems/:code` | 系统；详情含厂商、主机、depends_on、depended_by |
| `PUT /api/systems/:code/hosts` | 整体替换部署主机列表（host id 数组） |
| `PUT /api/systems/:code/dependencies` | 整体替换依赖列表（system code 数组），不做环检测 |
| `GET/POST /api/hosts`，`PUT/DELETE /api/hosts/:id` | 主机 |
| `GET /api/tickets?status=&system=&overdue=true` | 工单列表 |
| `POST /api/tickets` | 手工建单 |
| `GET /api/tickets/:id` | 详情 + 流水 + 计算的 SLA 状态 |
| `POST /api/tickets/:id/{ack,resolve,close,reopen}` | 状态操作，body `{operator, note}` |
| `POST /api/alerts/webhook` | Alertmanager 接入 |
| `GET /api/stats/sla?group_by=system\|vendor&from=&to=` | 故障数、MTTR、响应/恢复达标率、当前超时数 |
| `GET /healthz` | 存活检查 |

### 4.2 错误处理

领域层哨兵错误，handler 层用 `errors.Is` 统一映射：

| 错误 | HTTP |
|---|---|
| `ErrNotFound` | 404 |
| `ErrInvalidTransition` | 409 |
| `ErrValidation` | 400 |
| 唯一约束冲突 `ErrConflict` | 409 |
| 其他 | 500（记日志） |

错误体 `{"error": "...", "code": "..."}`。状态变更在事务内 `SELECT … FOR UPDATE` 锁工单行。

### 4.3 时间注入

service 依赖 `Clock` 接口（`Now() time.Time`），生产用真实时钟，测试用固定时钟。

### 4.4 测试策略

| 层 | 方式 | 覆盖 |
|---|---|---|
| 纯逻辑 | 表驱动单测，`make test`，无 DB | 状态机全组合；deadline 计算；`alert.Decide`；真实 Alertmanager payload 样本解析 |
| service + store | 集成测试，compose 中 PostgreSQL 的 `opshub_test` 库，`make test-integration` | 建单、去重、状态流转写库；20 goroutine 并发推同一 fingerprint → 恰好 1 张工单 |
| 端到端 | 手动，按 PLAN 验收标准 | `make chaos-error` → 自动出 P2 单，关联正确系统与厂商 |

### 4.5 数据

- `sla_policy` 是参考数据，放在 migration；opshub 启动时用 golang-migrate 自动迁移（迁移文件 `go:embed` 进二进制）。
- 5 个模拟系统、厂商、主机是演示数据，`make seed` 导入。

### 4.6 目录（相对 CLAUDE.md 的调整）

```
backend/
├── go.mod                     # 独立 module
├── sqlc.yaml
├── Dockerfile                 # 多阶段构建
├── cmd/opshub/main.go
├── internal/{api,service,cmdb,alert,ticket,sla,store}/
├── internal/store/queries/*.sql
└── migrations/
mock/mock-gov-app/
├── go.mod                     # 独立 module
├── Dockerfile
└── main.go 等
```

### 4.7 阶段 3/4 边界

- 前端只消费 4.1 的 REST API；开发期用 Vite proxy，后端不处理 CORS。
- 阶段 4 需要 pgvector 时把 PostgreSQL 镜像换成 `pgvector/pgvector:pg16`，现在不预留。

### 4.8 开发环境

本机安装 Go 1.22+ 与 docker compose 插件；compose 只用于运行服务，编译和测试在本机。
