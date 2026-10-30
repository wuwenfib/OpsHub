COMPOSE := docker compose -f deploy/docker-compose.yml -p opshub
MOCK    := http://localhost:8081

.PHONY: up down logs ps test chaos-slow chaos-error chaos-down chaos-reset

up:            ## 构建并启动全部容器
	$(COMPOSE) up -d --build

down:          ## 停止并删除容器（保留数据卷）
	$(COMPOSE) down

logs:          ## 看日志：make logs s=prometheus
	$(COMPOSE) logs -f --tail=100 $(s)

ps:
	$(COMPOSE) ps

test:          ## 单元测试
	cd mock/mock-gov-app && go test -race ./...

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
