COMPOSE := docker compose -f deploy/docker-compose.yml -p opshub
MOCK    := http://localhost:8081

PROMTOOL := docker run --rm -v $(CURDIR)/deploy/prometheus:/p:ro -w /p/tests --entrypoint promtool prom/prometheus:v3.15.1

.PHONY: up down logs ps test test-rules reload chaos-slow chaos-error chaos-down chaos-reset

up:            ## 构建并启动全部容器
	$(COMPOSE) up -d --build

down:          ## 停止并删除容器（保留数据卷）
	$(COMPOSE) down

logs:          ## 看日志：make logs s=prometheus
	$(COMPOSE) logs -f --tail=100 $(s)

ps:
	$(COMPOSE) ps

test: test-rules  ## 单元测试
	cd mock/mock-gov-app && go test -race ./...

test-rules:    ## 告警规则语法检查 + 单元测试
	$(PROMTOOL) check rules /p/rules/gov-app.yml
	$(PROMTOOL) test rules gov-app_test.yml

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
