# OpsHub —— 小型 IT 运维管理平台（学习项目）

## 项目背景与目的

这是一个**学习型 Demo**，用来理解"政企 IT 运维管理体系"是怎么运转的：
200 个左右的业务系统从各个厂商手里划转过来以后，要怎么建台账、监控告警、开工单、用 SLA 考核外包厂商。

目标不是做成生产级产品，而是：
1. 亲手把"告警 → 工单 → SLA 考核"这条链路跑通；
2. 能用 docker-compose 一键拉起，现场演示；
3. 面试时能讲清楚每一层为什么这样设计。

作者背景：资深 C++ / Linux 系统工程师，熟悉分布式与性能调优，**Go、Web 前端、Prometheus 生态不熟**。

## 给 Claude Code 的协作要求（重要）

- **每一步先说明目的**：这一步要验证什么、用到的组件/接口/命令是干什么的、看到什么结果说明成功了。只给命令让人照着敲是没用的。
- 讲新概念时先给整体，再讲细节；遇到 Go 与 C++ 差异大的地方（goroutine、interface、error 处理、包管理）顺带用 C++ 类比解释一句。
- 每完成 `docs/PLAN.md` 里的一个任务：跑测试 / 手动验证 → 在 PLAN.md 里打勾 → 用 3～5 句话总结"这一步对应运维体系里的哪个概念"。
- 优先小步提交，一个任务一个 commit。
- 不要过度设计：不做多租户、不做复杂权限、不做微服务拆分。单体后端 + 一个数据库就够了。

## 整体架构

```
被管对象层   mock-gov-app (模拟政务系统, 暴露 /metrics, 可注入故障) + MySQL
     │ 指标
工具层       node_exporter / mysqld_exporter / blackbox_exporter
             → Prometheus (存储+告警规则) → Alertmanager (分组/去重/路由)
             → Grafana (看图)
     │ webhook
流程层       OpsHub 后端 (Go)：CMDB、告警接入、工单状态机、SLA 计时
     │ REST API
展示层       OpsHub 前端 (Vue3)：台账、工单、SLA 看板
```

原则：**采集、存储、告警计算全部用开源组件，自己只写"管理"那部分**（CMDB / 工单 / SLA）。

## 技术栈

| 部分 | 选型 | 说明 |
|---|---|---|
| 后端 | Go 1.22+，gin，pgx 或 sqlc，golang-migrate | 单体服务 |
| 数据库 | PostgreSQL 16 | |
| 前端 | Vue3 + Vite + Element Plus + ECharts | 第三阶段再做，之前用 curl/httpie 验证 |
| 监控 | Prometheus、Alertmanager、Grafana、node_exporter、mysqld_exporter、blackbox_exporter | 均用官方镜像 |
| 模拟系统 | Go 写的 mock-gov-app | 带故障注入开关 |
| 部署 | docker-compose | `make up` 一键启动 |

## 目录结构

```
opshub/
├── CLAUDE.md
├── Makefile                  # up / down / logs / test / chaos-*
├── docs/
│   └── PLAN.md               # 分阶段任务清单与验收标准
├── deploy/
│   ├── docker-compose.yml
│   ├── prometheus/           # prometheus.yml, rules/*.yml
│   ├── alertmanager/         # alertmanager.yml（webhook 指向 opshub）
│   └── grafana/              # provisioning: 数据源 + 预置看板
├── mock/
│   └── mock-gov-app/         # 模拟业务系统
├── backend/
│   ├── cmd/opshub/main.go
│   ├── internal/
│   │   ├── api/              # gin 路由与 handler，错误 → HTTP 状态码
│   │   ├── service/          # 用例编排：校验 → 事务 → 读写库
│   │   ├── apperr/           # 跨层共用的哨兵错误
│   │   ├── cmdb/             # 台账领域逻辑（纯校验，零 IO）
│   │   ├── alert/            # Alertmanager webhook 解析、去重、关联
│   │   ├── ticket/           # 工单与状态机
│   │   ├── sla/              # SLA 策略与计时
│   │   └── store/            # 数据库访问：queries/*.sql → sqlc 生成代码
│   ├── migrations/           # 版本化迁移，go:embed 进二进制，启动时自动执行
│   └── seed/                 # 演示数据（make seed）
└── frontend/                 # 第三阶段
```

## 核心数据模型（初版，可调整）

**CMDB**
- `vendor`：厂商（名称、联系人、电话、合同到期日）
- `business_system`：业务系统（编码 `code`、名称、所属部门、**重要等级 level 1～3**、厂商、负责人、描述）
- `host`：主机（IP、主机名、操作系统、环境）
- `system_host`：系统部署在哪些主机上（多对多）
- `system_dependency`：系统之间的依赖（A 依赖 B）

**工单**
- `ticket`：类型 `incident | problem | change`，标题，关联系统，**严重级别 P1～P4**，状态，来源 `alert | manual`，告警指纹 `alert_fingerprint`，处理人，`created_at / responded_at / resolved_at / closed_at`，`response_deadline / resolve_deadline`
- `ticket_log`：工单流水（谁、什么时候、做了什么、状态从哪变到哪）
- `sla_policy`：按"系统等级 × 严重级别"定义响应时限和恢复时限（分钟）

**工单状态机（事件单）**
`new → acknowledged → resolved → closed`，另有 `reopened`。非法跳转返回 409。

## 关键约定

- **告警与 CMDB 的关联**：Prometheus 告警规则里必须带 `system` 标签，值等于 `business_system.code`。关联不上的告警归到"未登记系统"，并在看板上提示（这本身就说明台账不全）。
- **告警去重**：同一个 `fingerprint` 已有未关闭的事件单时，不新建工单，只追加一条 `ticket_log`。
- **告警恢复**：收到 `status=resolved` 时，**不自动关单**，只记一条"告警已恢复"日志，由人工确认后关单（这是事件管理的常见做法）。
- **SLA 计时**：工单创建时按 `sla_policy` 写入两个 deadline；超时由查询时计算，不需要定时任务。
- 时间统一存 UTC，展示时转 Asia/Shanghai。

## 常用命令

```bash
make up            # 启动全部容器
make down          # 停止
make logs s=opshub # 看某个服务日志
make test          # 单元测试（含告警规则测试，不需要数据库）
make test-integration  # 集成测试，连 compose 里的 opshub_test 库，需先 make up
make seed          # 导入演示数据（可重复执行）
make tools         # 安装 sqlc（需要 gcc）
make sqlc          # 改了 migrations 或 queries 后重新生成 store 代码
make web           # 前端开发服务器 :5173，/api 经 Vite 代理到后端 :8080
make chaos-slow    # 让 mock-gov-app 变慢，触发延迟告警
make chaos-error   # 让 mock-gov-app 大量 500，触发错误率告警
make chaos-down    # 停掉 mock-gov-app，触发探活失败告警
make chaos-reset   # 恢复正常
```

## 端口规划

| 服务 | 端口 |
|---|---|
| opshub 后端 | 8080 |
| 前端 dev | 5173 |
| Prometheus | 9090 |
| Alertmanager | 9093 |
| Grafana | 3000（本机冲突时在 `deploy/.env` 设 `GRAFANA_PORT`） |
| mock-gov-app | 8081 |
| PostgreSQL | 5432 |
