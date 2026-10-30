COMPOSE := docker compose -f deploy/docker-compose.yml -p opshub
MOCK    := http://localhost:8081

PROMTOOL := docker run --rm -v $(CURDIR)/deploy/prometheus:/p:ro -w /p/tests --entrypoint promtool prom/prometheus:v3.15.1

.PHONY: up down logs ps test test-rules test-integration tools sqlc seed reload chaos-slow chaos-error chaos-down chaos-reset

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

seed:          ## 导入演示数据（可重复执行）
	docker exec -i opshub-postgres-1 psql -U opshub -d opshub -v ON_ERROR_STOP=1 -q < backend/seed/demo.sql

reload:        ## 改完规则后热加载 Prometheus，无需重启
	curl -fsS -X POST localhost:9090/-/reload

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
