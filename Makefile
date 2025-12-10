.PHONY: help build build-processor build-server clean test run-server run-processor process-data docker-build docker-run graphql-gen deps tidy fmt lint install dev

# 默认目标
.DEFAULT_GOAL := help

# 变量定义
BINARY_DIR := build
PROCESSOR_BINARY := $(BINARY_DIR)/processor
SERVER_BINARY := $(BINARY_DIR)/server
DATA_DIR := data
POETRY_DATA_DIR := poetry-data
GO_BUILD_FLAGS := CGO_ENABLED=1

# 自动检测CPU核心数
NPROCS := $(shell sysctl -n hw.ncpu 2>/dev/null || nproc 2>/dev/null || echo 4)
WORKERS := $(NPROCS)

# 颜色输出
BLUE := \033[0;34m
GREEN := \033[0;32m
YELLOW := \033[0;33m
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
	@echo "  make bench              - 运行基准测试"
	@echo ""
	@echo "$(GREEN)代码质量:$(NC)"
	@echo "  make fmt                - 格式化代码"
	@echo "  make lint               - 运行linter"
	@echo "  make tidy               - 整理依赖"
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
	@echo "  make info               - 显示系统信息"

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
	@$(GO_BUILD_FLAGS) go build -o $(PROCESSOR_BINARY) ./cmd/processor
	@echo "$(GREEN)✓ 处理器构建完成: $(PROCESSOR_BINARY)$(NC)"

## build-server: 构建API服务器
build-server:
	@echo "$(BLUE)构建API服务器...$(NC)"
	@mkdir -p $(BINARY_DIR)
	@$(GO_BUILD_FLAGS) go build -o $(SERVER_BINARY) ./cmd/server
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

## fmt: 格式化代码
fmt:
	@echo "$(BLUE)格式化代码...$(NC)"
	@go fmt ./...
	@echo "$(GREEN)✓ 代码格式化完成$(NC)"

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
	@$(GO_BUILD_FLAGS) go test -v ./...

## test-verbose: 运行测试（详细输出）
test-verbose:
	@echo "$(BLUE)运行测试（详细模式）...$(NC)"
	@$(GO_BUILD_FLAGS) go test -v -race -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)✓ 测试完成，覆盖率报告: coverage.html$(NC)"

## bench: 运行基准测试
bench:
	@echo "$(BLUE)运行基准测试...$(NC)"
	@$(GO_BUILD_FLAGS) go test -bench=. -benchmem ./...

## graphql-gen: 生成GraphQL代码
graphql-gen:
	@echo "$(BLUE)生成GraphQL代码...$(NC)"
	@go run github.com/99designs/gqlgen generate
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
		--output-simplified $(DATA_DIR)/poetry-simplified.db \
		--output-traditional $(DATA_DIR)/poetry-traditional.db \
		--workers $(WORKERS)
	@echo "$(GREEN)✓ 数据处理完成$(NC)"
	@echo "  简体数据库: $(DATA_DIR)/poetry-simplified.db"
	@echo "  繁体数据库: $(DATA_DIR)/poetry-traditional.db"

## rebuild-and-process: 重新构建并处理数据（开发时使用）
rebuild-and-process: clean build-processor
	@echo "$(BLUE)开始处理数据...$(NC)"
	@mkdir -p $(DATA_DIR)
	@$(PROCESSOR_BINARY) \
		--input $(POETRY_DATA_DIR) \
		--output-simplified $(DATA_DIR)/poetry-simplified.db \
		--output-traditional $(DATA_DIR)/poetry-traditional.db \
		--workers $(WORKERS)
	@echo "$(GREEN)✓ 重新构建并处理完成$(NC)"

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
	@echo "  API: http://localhost:8080"

## docker-stop: 停止Docker容器
docker-stop:
	@echo "$(YELLOW)停止Docker容器...$(NC)"
	@docker-compose down
	@echo "$(GREEN)✓ Docker容器已停止$(NC)"

## install: 安装到系统
install: build
	@echo "$(BLUE)安装到系统...$(NC)"
	@cp $(PROCESSOR_BINARY) $(GOPATH)/bin/poetry-processor
	@cp $(SERVER_BINARY) $(GOPATH)/bin/poetry-server
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
