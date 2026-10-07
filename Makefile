.PHONY: help changelog build build-processor build-server clean test run-server run-processor process-data docker-build docker-run graphql-gen deps tidy fmt gofumpt lint install dev

# 默认目标
.DEFAULT_GOAL := help

# 变量定义
BINARY_DIR := build
PROCESSOR_BINARY := $(BINARY_DIR)/processor
SERVER_BINARY := $(BINARY_DIR)/server
DATA_DIR := data
POETRY_DATA_DIR := poetry-data
GO_BUILD_FLAGS := CGO_ENABLED=1
# sqlite_fts5 enables FTS5 (and the trigram tokenizer) in mattn/go-sqlite3, required
# by the full-text search index created in internal/database/migrate.go
GO_TAGS := sqlite_fts5

# 自动检测CPU核心数
NPROCS := $(shell sysctl -n hw.ncpu 2>/dev/null || nproc 2>/dev/null || echo 4)
WORKERS := $(NPROCS)

# 颜色输出
BLUE := \033[0;34m
GREEN := \033[0;32m
YELLOW := \033[0;33m
RED := \033[0;31m
CYAN := \033[0;36m
NC := \033[0m # No Color

## help: 显示帮助信息
help:
	@echo "$(BLUE)Chinese Poetry API - Makefile Commands$(NC)"
	@echo ""
	@echo "$(GREEN)构建命令:$(NC)"
	@echo "  make build              - 构建所有二进制文件"
	@echo "  make build-processor    - 只构建数据处理器"
	@echo "  make build-server       - 只构建API服务器"
	@echo "  make clean              - 清理构建产物"
	@echo ""
	@echo "$(GREEN)开发命令:$(NC)"
	@echo "  make dev                - 开发模式（自动重载）"
	@echo "  make run-server         - 运行API服务器"
	@echo "  make run-processor      - 运行数据处理器（交互式）"
	@echo "  make process-data       - 处理诗词数据生成数据库"
	@echo "  make rebuild-and-process - 重新构建并处理数据（开发用）"
	@echo "  make graphql-gen        - 生成GraphQL代码"
	@echo ""
	@echo "$(GREEN)测试命令:$(NC)"
	@echo "  make test               - 运行所有测试"
	@echo "  make test-verbose       - 运行测试（详细输出）"
	@echo "  make coverage           - 生成测试覆盖率报告"
	@echo "  make bench              - 运行基准测试"
	@echo "  make fuzz               - 运行模糊测试"
	@echo ""
	@echo "$(GREEN)代码质量:$(NC)"
	@echo "  make fmt                - 格式化代码"
	@echo "  make lint               - 运行linter"
	@echo "  make tidy               - 整理依赖"
	@echo "  make changelog          - 预览下个版本的 CHANGELOG"
	@echo ""
	@echo "$(GREEN)Docker命令:$(NC)"
	@echo "  make docker-build       - 构建Docker镜像"
	@echo "  make docker-run         - 运行Docker容器"
	@echo "  make docker-stop        - 停止Docker容器"
	@echo ""
	@echo "$(GREEN)其他命令:$(NC)"
	@echo "  make deps               - 安装依赖"
	@echo "  make install            - 安装到系统"
	@echo "  make stats              - 显示代码统计"
	@echo "  make db-stats           - 显示诗词数据库统计"
	@echo "  make info               - 显示系统信息"
	@echo ""
	@echo "$(GREEN)发布命令:$(NC)"
	@echo "  make release            - 更新 CHANGELOG，创建并推送版本标签"

## info: 显示系统信息
info:
	@echo "$(BLUE)系统信息:$(NC)"
	@echo "  CPU核心数: $(NPROCS)"
	@echo "  Workers数量: $(WORKERS)"
	@echo "  Go版本: $(shell go version)"
	@echo "  构建目录: $(BINARY_DIR)"
	@echo "  数据目录: $(DATA_DIR)"

## build: 构建所有二进制文件
build: build-processor build-server
	@echo "$(GREEN)✓ 构建完成$(NC)"

## build-processor: 构建数据处理器
build-processor:
	@echo "$(BLUE)构建数据处理器...$(NC)"
	@mkdir -p $(BINARY_DIR)
	@$(GO_BUILD_FLAGS) go build -tags $(GO_TAGS) -o $(PROCESSOR_BINARY) ./cmd/processor
	@echo "$(GREEN)✓ 处理器构建完成: $(PROCESSOR_BINARY)$(NC)"

## build-server: 构建API服务器
build-server:
	@echo "$(BLUE)构建API服务器...$(NC)"
	@mkdir -p $(BINARY_DIR)
	@$(GO_BUILD_FLAGS) go build -tags $(GO_TAGS) -o $(SERVER_BINARY) ./cmd/server
	@echo "$(GREEN)✓ 服务器构建完成: $(SERVER_BINARY)$(NC)"

## clean: 清理构建产物
clean:
	@echo "$(YELLOW)清理构建产物...$(NC)"
	@rm -rf $(BINARY_DIR)
	@rm -rf $(DATA_DIR)/*.db $(DATA_DIR)/*.db-shm $(DATA_DIR)/*.db-wal
	@rm -f *.db *.db-shm *.db-wal
	@echo "$(GREEN)✓ 清理完成$(NC)"

## deps: 安装依赖
deps:
	@echo "$(BLUE)安装依赖...$(NC)"
	@go mod download
	@go get github.com/99designs/gqlgen@latest
	@echo "$(GREEN)✓ 依赖安装完成$(NC)"

## tidy: 整理依赖
tidy:
	@echo "$(BLUE)整理依赖...$(NC)"
	@go mod tidy
	@echo "$(GREEN)✓ 依赖整理完成$(NC)"

## changelog: 预览上次发版以来、将写入 CHANGELOG 的内容（需安装 git-cliff）
changelog:
	@command -v git-cliff >/dev/null || { echo "$(RED)未找到 git-cliff，请先安装：brew install git-cliff$(NC)"; exit 1; }
	@git cliff --unreleased --strip header

## fmt: 格式化代码
# gofumpt，而非 go fmt：.golangci.yml 启用的格式化器是 gofumpt，它比 gofmt 严格，
# 只跑 go fmt 产出的代码仍会被 lint 判为未格式化。
fmt:
	@echo "$(BLUE)格式化代码...$(NC)"
	@$(MAKE) --no-print-directory gofumpt
	@echo "$(GREEN)✓ 代码格式化完成$(NC)"

# gofumpt: 对全仓库执行 gofumpt -w，未安装时退回 go fmt 并提示
gofumpt:
	@if command -v gofumpt >/dev/null 2>&1; then \
		gofumpt -w . ; \
	else \
		echo "$(YELLOW)gofumpt 未安装，退回 go fmt（结果可能仍不满足 lint）$(NC)"; \
		echo "安装: go install mvdan.cc/gofumpt@latest"; \
		go fmt ./... >/dev/null; \
	fi

## lint: 运行linter
lint:
	@echo "$(BLUE)运行linter...$(NC)"
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "$(YELLOW)golangci-lint 未安装，跳过linting$(NC)"; \
		echo "安装: brew install golangci-lint"; \
	fi

## test: 运行测试
test:
	@echo "$(BLUE)运行测试...$(NC)"
	@$(GO_BUILD_FLAGS) go test -tags $(GO_TAGS) -v ./...

## test-verbose: 运行测试（详细输出）
test-verbose:
	@echo "$(BLUE)运行测试（详细模式）...$(NC)"
	@$(GO_BUILD_FLAGS) go test -tags $(GO_TAGS) -v -race -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)✓ 测试完成，覆盖率报告: coverage.html$(NC)"

## coverage: 生成测试覆盖率报告
coverage:
	@echo "$(BLUE)生成测试覆盖率报告...$(NC)"
	@$(GO_BUILD_FLAGS) go test -tags $(GO_TAGS) -coverprofile=coverage.out $$(go list ./... | grep -v '/generated')
	@echo ""
	@echo "$(GREEN)📊 覆盖率详情:$(NC)"
	@go tool cover -func=coverage.out
	@echo ""
	@echo "$(GREEN)📈 总覆盖率:$(NC)"
	@go tool cover -func=coverage.out | grep "^total:" | awk '{print "  " $$3}'
	@go tool cover -html=coverage.out -o coverage.html
	@echo ""
	@echo "$(GREEN)✓ 覆盖率报告已生成: coverage.html$(NC)"
	@echo "$(YELLOW)注意: 已排除 generated 目录$(NC)"

## bench: 运行基准测试
bench:
	@echo "$(BLUE)运行基准测试...$(NC)"
	@$(GO_BUILD_FLAGS) go test -tags $(GO_TAGS) -bench=. -benchmem ./...

## fuzz: 运行模糊测试
fuzz:
	@echo "$(BLUE)运行模糊测试...$(NC)"
	@echo "$(YELLOW)测试 classifier 包...$(NC)"
	@$(GO_BUILD_FLAGS) go test -tags $(GO_TAGS) -fuzz='^FuzzToTraditional$$' -fuzztime=10s ./internal/classifier/ || true
	@$(GO_BUILD_FLAGS) go test -tags $(GO_TAGS) -fuzz='^FuzzToSimplified$$' -fuzztime=10s ./internal/classifier/ || true
	@$(GO_BUILD_FLAGS) go test -tags $(GO_TAGS) -fuzz='^FuzzClassifyPoetryType$$' -fuzztime=10s ./internal/classifier/ || true
	@echo "$(YELLOW)测试 search 包...$(NC)"
	@$(GO_BUILD_FLAGS) go test -tags $(GO_TAGS) -fuzz='^FuzzIsPinyinQuery$$' -fuzztime=10s ./internal/search/ || true
	@echo "$(GREEN)✓ 模糊测试完成$(NC)"

## graphql-gen: 生成GraphQL代码
# gqlgen 的输出不满足 gofumpt（例如把 resolver 类型写成多条独立的 type 声明，
# gofumpt 要求合并为一个 type 分组），所以生成后必须补一次格式化，否则 lint 会红。
# 注意 golangci-lint fmt 修不了这一条，必须用 gofumpt。
graphql-gen:
	@echo "$(BLUE)生成GraphQL代码...$(NC)"
	@go run github.com/99designs/gqlgen generate
	@$(MAKE) --no-print-directory gofumpt
	@echo "$(GREEN)✓ GraphQL代码生成完成$(NC)"

## run-server: 运行API服务器
run-server: build-server
	@echo "$(BLUE)启动API服务器...$(NC)"
	@$(SERVER_BINARY)

## run-processor: 运行数据处理器（交互式）
run-processor: build-processor
	@echo "$(BLUE)运行数据处理器...$(NC)"
	@$(PROCESSOR_BINARY) --help

## process-data: 处理诗词数据生成数据库
process-data: build-processor
	@echo "$(BLUE)处理诗词数据...$(NC)"
	@mkdir -p $(DATA_DIR)
	@$(PROCESSOR_BINARY) \
		--input $(POETRY_DATA_DIR) \
		--output $(DATA_DIR)/poetry.db \
		--workers $(WORKERS)
	@echo "$(GREEN)✓ 数据处理完成$(NC)"
	@echo "  统一数据库: $(DATA_DIR)/poetry.db (包含简体和繁体表)"

## rebuild-and-process: 重新构建并处理数据（开发时使用）
rebuild-and-process: clean build-processor
	@echo "$(BLUE)开始处理数据...$(NC)"
	@mkdir -p $(DATA_DIR)
	@$(PROCESSOR_BINARY) \
		--input $(POETRY_DATA_DIR) \
		--output $(DATA_DIR)/poetry.db \
		--workers $(WORKERS)
	@echo "$(GREEN)✓ 重新构建并处理完成$(NC)"
	@echo "  统一数据库: $(DATA_DIR)/poetry.db (包含简体和繁体表)"

## dev: 开发模式（需要安装 air）
dev:
	@if command -v air >/dev/null 2>&1; then \
		air; \
	else \
		echo "$(YELLOW)air 未安装，使用普通模式运行$(NC)"; \
		echo "安装 air: go install github.com/cosmtrek/air@latest"; \
		make run-server; \
	fi

## docker-build: 构建Docker镜像
docker-build:
	@echo "$(BLUE)构建Docker镜像...$(NC)"
	@docker build -t chinese-poetry-api:latest .
	@echo "$(GREEN)✓ Docker镜像构建完成$(NC)"

## docker-run: 运行Docker容器
docker-run:
	@echo "$(BLUE)启动Docker容器...$(NC)"
	@docker-compose up -d
	@echo "$(GREEN)✓ Docker容器已启动$(NC)"
	@echo "  API: http://localhost:1279"

## docker-stop: 停止Docker容器
docker-stop:
	@echo "$(YELLOW)停止Docker容器...$(NC)"
	@docker-compose down
	@echo "$(GREEN)✓ Docker容器已停止$(NC)"

## install: 安装到系统
install: build
	@echo "$(BLUE)安装到系统...$(NC)"
	@if [ -z "$$GOPATH" ]; then \
		echo "$(YELLOW)GOPATH 未设置，使用 go install...$(NC)"; \
		cd cmd/processor && go install; \
		cd ../server && go install; \
	else \
		cp $(PROCESSOR_BINARY) $$GOPATH/bin/poetry-processor; \
		cp $(SERVER_BINARY) $$GOPATH/bin/poetry-server; \
	fi
	@echo "$(GREEN)✓ 安装完成$(NC)"
	@echo "  poetry-processor - 数据处理器"
	@echo "  poetry-server - API服务器"

## stats: 显示代码统计
stats:
	@echo "$(BLUE)代码统计:$(NC)"
	@echo "Go文件数量:"
	@find . -name "*.go" -not -path "./vendor/*" -not -path "*/generated/*" | wc -l
	@echo "代码行数:"
	@find . -name "*.go" -not -path "./vendor/*" -not -path "*/generated/*" | xargs wc -l | tail -1
	@echo ""
	@echo "目录结构:"
	@tree -L 2 -I 'vendor|node_modules|.git|poetry-data' || ls -R | grep ":$$" | sed -e 's/:$$//' -e 's/[^-][^\/]*\//--/g' -e 's/^/   /' -e 's/-/|/'

## db-stats: 显示诗词数据库统计
db-stats:
	@echo "$(BLUE)诗词数据库统计:$(NC)"
	@if [ -f "$(DATA_DIR)/poetry.db" ]; then \
		echo ""; \
		echo "$(GREEN)简体中文 (zh-Hans):$(NC)"; \
		sqlite3 -header -column $(DATA_DIR)/poetry.db \
			"SELECT t.name AS 类型, COUNT(*) AS 数量 FROM poems_zh_hans AS p JOIN poetry_types_zh_hans AS t ON p.type_id = t.id GROUP BY type_id ORDER BY 数量 DESC;"; \
		echo ""; \
		echo "$(GREEN)繁体中文 (zh-Hant):$(NC)"; \
		sqlite3 -header -column $(DATA_DIR)/poetry.db \
			"SELECT t.name AS 類型, COUNT(*) AS 數量 FROM poems_zh_hant AS p JOIN poetry_types_zh_hant AS t ON p.type_id = t.id GROUP BY type_id ORDER BY 數量 DESC;"; \
	else \
		echo "$(YELLOW)数据库文件不存在: $(DATA_DIR)/poetry.db$(NC)"; \
		echo "请先运行: make process-data"; \
	fi

## release: 创建并推送版本标签
release:  ## Update CHANGELOG, create and push version tag
	@if ! command -v git-cliff >/dev/null; then \
		echo "$(RED)Error: git-cliff not found, install it with: brew install git-cliff$(NC)"; \
		exit 1; \
	fi; \
	if [ -n "$$(git status --porcelain)" ]; then \
		echo "$(RED)Error: Working directory has uncommitted changes$(NC)"; \
		echo "$(YELLOW)Please commit or stash your changes before releasing$(NC)"; \
		exit 1; \
	fi; \
	LATEST_TAG=$$(git describe --tags --abbrev=0 2>/dev/null || echo "none"); \
	echo "$(BLUE)════════════════════════════════════════$(NC)"; \
	echo "$(BLUE)         Release New Version$(NC)"; \
	echo "$(BLUE)════════════════════════════════════════$(NC)"; \
	echo "$(CYAN)Current latest tag: $(GREEN)$$LATEST_TAG$(NC)"; \
	echo "$(BLUE)════════════════════════════════════════$(NC)"; \
	printf "$(YELLOW)Enter new version: $(NC)"; \
	read -r VERSION; \
	if [ -z "$$VERSION" ]; then \
		echo "$(RED)Error: Version cannot be empty$(NC)"; \
		exit 1; \
	fi; \
	if ! echo "$$VERSION" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+$$'; then \
		echo "$(RED)Error: Invalid version format '$$VERSION'$(NC)"; \
		echo "$(YELLOW)Expected format: v1.0.0$(NC)"; \
		exit 1; \
	fi; \
	if git tag | grep -q "^$$VERSION$$"; then \
		echo "$(RED)Error: Tag $$VERSION already exists$(NC)"; \
		exit 1; \
	fi; \
	echo ""; \
	echo "$(YELLOW)About to create and push tag: $(GREEN)$$VERSION$(NC)"; \
	printf "$(YELLOW)Continue? [y/N] $(NC)"; \
	read -r CONFIRM; \
	if [ "$$CONFIRM" != "y" ] && [ "$$CONFIRM" != "Y" ]; then \
		echo "$(YELLOW)Aborted$(NC)"; \
		exit 1; \
	fi; \
	if [ "$$LATEST_TAG" != "none" ]; then \
		NEW_VER=$$(echo $$VERSION | sed 's/^v//'); \
		CUR_VER=$$(echo $$LATEST_TAG | sed 's/^v//'); \
		NEW_MAJOR=$$(echo $$NEW_VER | cut -d. -f1); \
		NEW_MINOR=$$(echo $$NEW_VER | cut -d. -f2); \
		NEW_PATCH=$$(echo $$NEW_VER | cut -d. -f3); \
		CUR_MAJOR=$$(echo $$CUR_VER | cut -d. -f1); \
		CUR_MINOR=$$(echo $$CUR_VER | cut -d. -f2); \
		CUR_PATCH=$$(echo $$CUR_VER | cut -d. -f3); \
		if [ $$NEW_MAJOR -lt $$CUR_MAJOR ] || \
		   ([ $$NEW_MAJOR -eq $$CUR_MAJOR ] && [ $$NEW_MINOR -lt $$CUR_MINOR ]) || \
		   ([ $$NEW_MAJOR -eq $$CUR_MAJOR ] && [ $$NEW_MINOR -eq $$CUR_MINOR ] && [ $$NEW_PATCH -le $$CUR_PATCH ]); then \
			echo "$(RED)Error: New version $$VERSION must be greater than $$LATEST_TAG$(NC)"; \
			exit 1; \
		fi; \
	fi; \
	echo "$(BLUE)Updating CHANGELOG.md...$(NC)"; \
	git cliff --unreleased --tag $$VERSION --prepend CHANGELOG.md 2>/dev/null || exit 1; \
	if git diff --quiet CHANGELOG.md; then \
		echo "$(YELLOW)⚠ No user-facing changes since $$LATEST_TAG, CHANGELOG.md unchanged$(NC)"; \
	else \
		git --no-pager diff --stat CHANGELOG.md; \
		echo "$(YELLOW)CHANGELOG.md updated. Review/edit it now (e.g. translate English entries), then press Enter to continue, or Ctrl-C to abort.$(NC)"; \
		read -r _; \
		git add CHANGELOG.md; \
		git commit -m "docs(changelog): release $$VERSION" || exit 1; \
		echo "$(BLUE)Pushing changelog commit...$(NC)"; \
		git push origin HEAD || exit 1; \
		echo "$(GREEN)✓ CHANGELOG.md committed$(NC)"; \
	fi; \
	if git config user.signingkey >/dev/null 2>&1 && command -v gpg >/dev/null 2>&1; then \
		echo "$(BLUE)Creating GPG signed tag $$VERSION...$(NC)"; \
		if git tag -s $$VERSION -m "Release $$VERSION" 2>/dev/null; then \
			echo "$(GREEN)✓ Signed tag $$VERSION created (Verified ✓)$(NC)"; \
		else \
			echo "$(YELLOW)⚠ GPG signing failed, using regular tag...$(NC)"; \
			git tag -a $$VERSION -m "Release $$VERSION"; \
			echo "$(GREEN)✓ Tag $$VERSION created$(NC)"; \
		fi \
	else \
		echo "$(BLUE)Creating tag $$VERSION...$(NC)"; \
		git tag -a $$VERSION -m "Release $$VERSION"; \
		echo "$(GREEN)✓ Tag $$VERSION created$(NC)"; \
		echo "$(YELLOW)💡 Tip: Configure GPG key to show Verified badge$(NC)"; \
	fi; \
	echo "$(BLUE)Pushing tag to remote...$(NC)"; \
	git push origin $$VERSION; \
	echo "$(GREEN)✓ Release $$VERSION completed$(NC)"
