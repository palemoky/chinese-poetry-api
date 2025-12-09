# Chinese Poetry API

<p align="center">
  <a href="https://github.com/chinese-poetry/chinese-poetry">
      <img src="https://avatars3.githubusercontent.com/u/30764933?s=200&v=4" alt="chinese-poetry">
  </a>
</p>

<h2 align="center">中国古诗词 API 服务</h2>

<p align="center">
  基于 Go 语言的高性能中国古诗词 API 服务，支持 REST 和 GraphQL 接口，提供简体/繁体中文、拼音搜索等功能。
</p>

## ✨ 特性

- 🚀 **高性能**: Go 语言编写，支持并发处理
- 📚 **海量数据**: 包含唐诗、宋词、元曲等数十万首诗词
- 🔍 **强大搜索**: 支持全文搜索、拼音搜索、模糊搜索
- 🌏 **双语支持**: 同时提供简体和繁体中文版本
- 🎯 **多种接口**: REST API 和 GraphQL 双接口支持
- 🛡️ **限流保护**: 内置 IP 限流，防止滥用
- 🐳 **容器化**: Docker 镜像开箱即用
- 📊 **智能分类**: 按朝代、作者、诗词类型分类

## 📖 数据集

本项目基于 [chinese-poetry](https://github.com/chinese-poetry/chinese-poetry) 数据集，包含:

- 唐诗 5.5万+ 首
- 宋诗 26万+ 首
- 宋词 2.1万+ 首
- 元曲、五代诗词、诗经、楚辞等

## 🚀 快速开始

### 使用 Makefile（推荐）

```bash
# 查看所有可用命令
make help

# 构建项目
make build

# 处理数据生成数据库
make process-data

# 启动API服务器
make run-server
```

### Docker 部署

```bash
docker run -d -p 8080:8080 \
  -e GITHUB_REPO=your-username/chinese-poetry-api \
  -e RELEASE_VERSION=v0.1.0 \
  -e DB_TYPE=simplified \
  your-dockerhub-username/chinese-poetry-api:latest
```

### Docker Compose

```yaml
version: '3.8'

services:
  poetry-api:
    image: your-dockerhub-username/chinese-poetry-api:latest
    ports:
      - "8080:8080"
    environment:
      - DB_TYPE=simplified
      - GITHUB_REPO=your-username/chinese-poetry-api
      - RELEASE_VERSION=latest
      - RATE_LIMIT_RPS=10
      - RATE_LIMIT_BURST=20
    volumes:
      - ./data:/app
    restart: unless-stopped
```

### 本地开发

```bash
# 克隆仓库
git clone https://github.com/your-username/chinese-poetry-api.git
cd chinese-poetry-api

# 安装依赖
make deps

# 构建项目
make build

# 处理数据 (首次运行)
make process-data

# 启动服务器
make run-server
```

访问:
- REST API: http://localhost:8080/api/v1
- GraphQL: http://localhost:8080/graphql
- Health Check: http://localhost:8080/api/v1/health

## 📡 API 使用

### REST API

#### 获取诗词详情
```bash
curl http://localhost:8080/api/v1/poems/{id}
```

#### 搜索诗词 (中文)
```bash
curl "http://localhost:8080/api/v1/poems/search?q=春江花月夜"
```

#### 搜索诗词 (拼音)
```bash
# 完整拼音
curl "http://localhost:8080/api/v1/poems/search?q=jing+ye+si&type=pinyin"

# 拼音缩写
curl "http://localhost:8080/api/v1/poems/search?q=jys&type=pinyin"
```

#### 按作者搜索
```bash
curl "http://localhost:8080/api/v1/poems/search?q=李白&type=author"
```

#### 随机诗词
```bash
curl http://localhost:8080/api/v1/random
```

#### 统计信息
```bash
curl http://localhost:8080/api/v1/stats
```

### GraphQL API

```graphql
query {
  poems(page: 1, pageSize: 10) {
    edges {
      node {
        id
        title
        paragraphs
        author {
          name
          dynasty {
            name
          }
        }
      }
    }
    pageInfo {
      hasNextPage
      totalCount
    }
  }
}
```

## 🔧 配置选项

### 环境变量

| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| `DB_TYPE` | 数据库类型 (simplified/traditional) | simplified |
| `GITHUB_REPO` | GitHub 仓库 (用于下载数据库) | - |
| `RELEASE_VERSION` | Release 版本 | latest |
| `PORT` | 服务端口 | 8080 |
| `GIN_MODE` | Gin 模式 (debug/release) | release |
| `RATE_LIMIT_ENABLED` | 是否启用限流 | true |
| `RATE_LIMIT_RPS` | 每秒请求数 | 10 |
| `RATE_LIMIT_BURST` | 突发请求数 | 20 |
| `GRAPHQL_PLAYGROUND` | 是否启用 GraphQL Playground | false |

### 配置文件 (config.yaml)

```yaml
server:
  port: 8080
  mode: release

database:
  type: simplified
  path: poetry-simplified.db

rate_limit:
  enabled: true
  requests_per_second: 10
  burst: 20

search:
  enable_pinyin: true
  enable_fuzzy: true
```

## 🏗️ 项目结构

```
chinese-poetry-api/
├── cmd/
│   ├── processor/      # 数据处理工具
│   └── server/         # API 服务器
├── internal/
│   ├── api/            # API 层
│   ├── classifier/     # 分类器
│   ├── config/         # 配置管理
│   ├── database/       # 数据库层
│   ├── loader/         # 数据加载器
│   ├── processor/      # 数据处理器
│   └── search/         # 搜索引擎
├── scripts/            # 脚本
├── .github/workflows/  # CI/CD
├── Dockerfile
├── docker-compose.yml
└── README.md
```

## 🔍 搜索功能

### 1. 全文搜索 (FTS5)
使用 SQLite FTS5 进行高效全文搜索

### 2. 拼音搜索
- 完整拼音: `jing ye si` → 静夜思
- 拼音缩写: `jys` → 静夜思

### 3. 模糊搜索
支持 LIKE 查询，可搜索标题、内容、作者

### 4. 智能检测
自动识别查询是中文还是拼音

## 🏷️ 诗词分类

### 按朝代
- 唐、宋、元、五代
- 先秦、两汉、魏晋、南北朝、隋、清

### 按类型
- **绝句**: 五言绝句、七言绝句
- **律诗**: 五言律诗、七言律诗
- **古诗**: 五言古诗、七言古诗
- **词**: 按词牌名分类
- **曲**: 元曲
- **其他**: 不规则形式

## 📦 数据处理

### 处理流程

1. **加载数据**: 从 JSON 文件加载诗词数据
2. **分类**: 按朝代、类型自动分类
3. **转换**: 生成拼音、简繁体转换
4. **并发处理**: 使用 Worker Pool 并发处理
5. **存储**: 存入 SQLite 数据库
6. **索引**: 创建 FTS5 全文索引

### 手动处理数据

```bash
go run cmd/processor/main.go \
  --input . \
  --output-simplified poetry-simplified.db \
  --output-traditional poetry-traditional.db \
  --workers 8 \
  --config loader/datas.json
```

## 🚢 发布流程

### 版本策略

- **主次版本** (v0.1.0, v0.2.0, v1.0.0): 触发数据处理 + Docker 构建
- **补丁版本** (v0.1.1, v0.1.2): 仅触发 Docker 构建

### GitHub Actions

#### 数据处理工作流
- 触发条件: `v*.*.0` 标签
- 生成简体和繁体数据库
- 压缩并上传到 GitHub Release

#### Docker 构建工作流
- 触发条件: 所有 `v*` 标签
- 多架构构建 (amd64, arm64)
- 推送到 Docker Hub

## 🛠️ 开发

### 依赖

- Go 1.23+
- SQLite 3
- Docker (可选)

### 构建

```bash
# 构建处理器
go build -o processor ./cmd/processor

# 构建服务器
go build -o server ./cmd/server
```

### 测试

```bash
# 运行测试
go test ./...

# 测试数据处理
./processor --input . --output test.db --workers 4

# 测试 API
./server
curl http://localhost:8080/api/v1/health
```

## 📄 License

[MIT](LICENSE)

## 🙏 致谢

- 数据来源: [chinese-poetry](https://github.com/chinese-poetry/chinese-poetry)
- 简繁转换: [gocc](https://github.com/liuzl/gocc)
- 拼音转换: [go-pinyin](https://github.com/mozillazg/go-pinyin)

## 📮 联系方式

如有问题或建议，欢迎提交 Issue 或 Pull Request。
