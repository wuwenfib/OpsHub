COMPOSE := docker compose -f deploy/docker-compose.yml -p opshub
MOCK    := http://localhost:8081

PROMTOOL := docker run --rm -v $(CURDIR)/deploy/prometheus:/p:ro -w /p/tests --entrypoint promtool prom/prometheus:v3.15.1

# Grafana 端口：deploy/.env 里设置了 GRAFANA_PORT 就用它，否则 3000（与 docker-compose.yml 一致）
GRAFANA_PORT := $(or $(shell grep -s '^GRAFANA_PORT=' deploy/.env | cut -d= -f2),3000)

.PHONY: up down logs ps test test-rules test-integration tools sqlc seed web reload demo-check demo-reset chaos-slow chaos-error chaos-down chaos-reset

up:            ## 构建并启动全部容器
	$(COMPOSE) up -d --build

down:          ## 停止并删除容器（保留数据卷）
	$(COMPOSE) down

logs:          ## 看日志：make logs s=prometheus
	$(COMPOSE) logs -f --tail=100 $(s)

ps:
	$(COMPOSE) ps

test: test-rules  ## 单元测试（不需要数据库）
	cd mock/mock-gov-app && go test -race ./...
	cd backend && go test -race ./...

test-integration:  ## 集成测试：连 compose 里 PostgreSQL 的 opshub_test 库，需先 make up
	cd backend && go test -race -count=1 -p 1 -tags integration ./...

test-rules:    ## 告警规则语法检查 + 单元测试
	$(PROMTOOL) check rules /p/rules/gov-app.yml
	$(PROMTOOL) test rules gov-app_test.yml

tools:         ## 安装开发工具（sqlc 含 C 代码，需要 gcc）
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1

sqlc:          ## 改了 migrations 或 queries 后重新生成 store 代码
	cd backend && sqlc generate

seed:          ## 导入演示数据：台账 + 过去 30 天的历史工单（可重复执行，已存在的跳过）
	docker exec -i opshub-postgres-1 psql -U opshub -d opshub -v ON_ERROR_STOP=1 -q < backend/seed/demo.sql
	docker exec -i opshub-postgres-1 psql -U opshub -d opshub -v ON_ERROR_STOP=1 -q < backend/seed/history.sql

web:           ## 启动前端开发服务器 :5173（首次自动安装依赖），/api 代理到后端 :8080
	cd frontend && [ -d node_modules ] || npm install
	cd frontend && npm run dev

reload:        ## 改完规则后热加载 Prometheus，无需重启
	curl -fsS -X POST localhost:9090/-/reload

demo-check:    ## 演示前自检：各服务是否在线、当前有无残留告警，并列出要打开的页面
	@for s in "mock-gov-app|$(MOCK)/healthz" "Prometheus|http://localhost:9090/-/ready" \
	          "Alertmanager|http://localhost:9093/-/ready" "Grafana|http://localhost:$(GRAFANA_PORT)/api/health" \
	          "OpsHub 后端|http://localhost:8080/healthz" "OpsHub 前端|http://localhost:5173/"; do \
	  name=$${s%%|*}; url=$${s#*|}; \
	  if curl -fsS -o /dev/null -m 3 $$url; then echo "  ✓ $$name"; else echo "  ✗ $$name  ($$url)"; fi; \
	done
	@echo "  当前 firing 告警数：$$(curl -s localhost:9090/api/v1/alerts | grep -o '"state":"firing"' | wc -l)（演示前应为 0）"
	@echo "  故障注入模式：$$(curl -s $(MOCK)/metrics | awk '/^gov_app_chaos_mode/{print $$2}')（0 = 正常）"
	@echo ""
	@echo "  OpsHub      http://localhost:5173"
	@echo "  Grafana     http://localhost:$(GRAFANA_PORT)/d/opshub-system-health"
	@echo "  Prometheus  http://localhost:9090/alerts"
	@echo "  Alertmanager http://localhost:9093"

demo-reset:    ## 清空所有工单与流水后重新导入历史工单（台账、SLA 策略保留）。会删除数据！
	@read -p "将删除全部工单与流水（台账保留），确认请输入 yes：" a && [ "$$a" = yes ]
	docker exec opshub-postgres-1 psql -U opshub -d opshub -c "TRUNCATE ticket RESTART IDENTITY CASCADE"
	docker exec -i opshub-postgres-1 psql -U opshub -d opshub -v ON_ERROR_STOP=1 -q < backend/seed/history.sql

chaos-slow:    ## mock-gov-app 变慢，触发延迟告警
	curl -fsS -X POST $(MOCK)/chaos/slow; echo

chaos-error:   ## mock-gov-app 30% 返回 500，触发错误率告警
	curl -fsS -X POST $(MOCK)/chaos/error; echo

chaos-down:    ## 停掉 mock-gov-app，触发探活失败告警
	$(COMPOSE) stop mock-gov-app

chaos-reset:   ## 恢复正常
	$(COMPOSE) start mock-gov-app
	@until curl -fsS $(MOCK)/healthz >/dev/null 2>&1; do sleep 1; done
	curl -fsS -X POST $(MOCK)/chaos/reset; echo
