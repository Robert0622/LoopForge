.PHONY: run demo test e2e fmt vet test-mysql clean-mysql build build-gate-mcp web-deps docker

GOCACHE ?= /tmp/loopforge-gocache
# Use the local toolchain; go.mod requires go >= 1.25.

# Point go-run at the prebuilt helpers so Agent/demo gates work even when
# the temporary go-run binary lives outside the checkout.
export LOOPFORGE_GATE_MCP_BINARY ?= $(CURDIR)/bin/loopforge-gate-mcp-{os}-{arch}
export LOOPFORGE_RUNNER_BINARY ?= $(CURDIR)/bin/loopforge-runner
export LOOPFORGE_SOURCE_ROOT ?= $(CURDIR)

run: build-gate-mcp
	GOCACHE=$(GOCACHE) go run ./cmd/loopforge

# demo runs with the demo executor so the full SOP flow works without any
# model API key.
demo: build-gate-mcp
	LOOPFORGE_DEMO_MODE=true GOCACHE=$(GOCACHE) go run ./cmd/loopforge

test:
	GOCACHE=$(GOCACHE) go test -p=1 ./... -count=1 -timeout=300s

# e2e starts a real server twice: once for the API suite (script SOP, approval,
# schedule, rooms, runners) and once for the browser suite.
e2e: build
	GOCACHE=$(GOCACHE) go test -tags e2e -count=1 -timeout 180s ./e2e
	cd e2e && npm ci --no-audit --no-fund && npx playwright install chromium && npx playwright test

fmt:
	gofmt -w cmd internal web docs deploy e2e

vet:
	GOCACHE=$(GOCACHE) go vet ./...

# web-deps fetches vscode-web (Code - OSS web build, MIT) for the in-browser
# workspace editor. The server works without it; only /vscode-web/ is missing.
web-deps:
	sh scripts/fetch-vscode-web.sh

docker:
	docker build -t loopforge:local .

# test-mysql runs the gorm integration tests against a throwaway local MySQL
# container. It never touches any externally configured database: the DSN is
# bound to 127.0.0.1 and the container is removed afterwards.
MYSQL_CONTAINER ?= loopforge-test-mysql
MYSQL_PORT ?= 33306
MYSQL_TEST_DB ?= loopforge_it
MYSQL_IMAGE ?= mysql:8.0

test-mysql:
	@docker rm -f $(MYSQL_CONTAINER) >/dev/null 2>&1 || true
	docker run -d --name $(MYSQL_CONTAINER) -p 127.0.0.1:$(MYSQL_PORT):3306 \
		-e MYSQL_ROOT_PASSWORD=root -e MYSQL_DATABASE=$(MYSQL_TEST_DB) $(MYSQL_IMAGE)
	@echo "waiting for MySQL to become ready..."
	@until docker exec $(MYSQL_CONTAINER) mysqladmin ping -uroot -proot --silent >/dev/null 2>&1; do sleep 1; done
	@until docker exec $(MYSQL_CONTAINER) mysql -uroot -proot -e "SELECT 1" >/dev/null 2>&1; do sleep 1; done
	@sleep 3
	LOOPFORGE_TEST_MYSQL_DSN="root:root@tcp(127.0.0.1:$(MYSQL_PORT))/$(MYSQL_TEST_DB)" \
		GOCACHE=$(GOCACHE) go test -p=1 ./internal/repo/ -count=1 -timeout=180s
	$(MAKE) clean-mysql

clean-mysql:
	-@docker rm -f $(MYSQL_CONTAINER) >/dev/null 2>&1 || true

build:
	mkdir -p bin
	GOCACHE=$(GOCACHE) go build -o bin/loopforge ./cmd/loopforge
	GOCACHE=$(GOCACHE) go build -o bin/loopforge-mcp ./cmd/loopforge-mcp
	GOCACHE=$(GOCACHE) go build -o bin/loopforge-runner ./cmd/loopforge-runner
	$(MAKE) build-gate-mcp

build-gate-mcp:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 GOCACHE=$(GOCACHE) go build -o bin/loopforge-gate-mcp-darwin-amd64 ./cmd/loopforge-gate-mcp
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 GOCACHE=$(GOCACHE) go build -o bin/loopforge-gate-mcp-darwin-arm64 ./cmd/loopforge-gate-mcp
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOCACHE=$(GOCACHE) go build -o bin/loopforge-gate-mcp-linux-amd64 ./cmd/loopforge-gate-mcp
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 GOCACHE=$(GOCACHE) go build -o bin/loopforge-gate-mcp-linux-arm64 ./cmd/loopforge-gate-mcp
