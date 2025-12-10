# Chinese Poetry API

<p align="center">
  <a href="https://github.com/chinese-poetry/chinese-poetry">
      <img src="https://avatars3.githubusercontent.com/u/30764933?s=200&v=4" alt="chinese-poetry">
  </a>
</p>

<h2 align="center">中国古诗词 API 服务</h2>

---

[![Test Status](https://github.com/palemoky/chinese-poetry-api/actions/workflows/test.yml/badge.svg)](https://github.com/palemoky/chinese-poetry-api/actions/workflows/test.yml)
[![Docker Build](https://github.com/palemoky/chinese-poetry-api/actions/workflows/docker-build.yml/badge.svg)](https://github.com/palemoky/chinese-poetry-api/actions/workflows/docker-build.yml)
[![Docker Image](https://img.shields.io/docker/v/palemoky/chinese-poetry-api?sort=semver&label=docker)](https://hub.docker.com/r/palemoky/chinese-poetry-api)
[![Go Report Card](https://goreportcard.com/badge/github.com/palemoky/chinese-poetry-api)](https://goreportcard.com/report/github.com/palemoky/chinese-poetry-api)
[![Go Version](https://img.shields.io/github/go-mod/go-version/palemoky/chinese-poetry-api)](https://github.com/palemoky/chinese-poetry-api/blob/main/go.mod)
[![License](https://img.shields.io/github/license/palemoky/chinese-poetry-api)](https://github.com/palemoky/chinese-poetry-api/blob/main/LICENSE)

基于 Go 语言的高性能中国古诗词 API 服务，支持 REST 和 GraphQL 接口，提供简体/繁体中文、拼音搜索等功能。

## ✨ 特性

- 🚀 **高性能**: Go 语言编写，支持并发处理，性能优化（简繁转换 ~300ns/op）
- 📚 **海量数据**: 包含唐诗、宋词、元曲等数十万首诗词
- 🔍 **强大搜索**: 支持全文搜索、拼音搜索、模糊搜索
- 🌏 **双语支持**: 同时提供简体和繁体中文版本
- 🎯 **多种接口**: REST API 和 GraphQL 双接口支持
- 🛡️ **限流保护**: 内置 IP 限流，防止滥用
- 🐳 **容器化**: Docker 镜像开箱即用，支持多架构（amd64/arm64）
- 📊 **智能分类**: 按朝代、作者、诗词类型自动分类
- ✅ **高质量代码**: 完整的单元测试、性能测试、模糊测试

## 📖 数据集

本项目基于 [chinese-poetry](https://github.com/chinese-poetry/chinese-poetry) 数据集，包含:

- 唐诗 5.5万+ 首
- 宋诗 26万+ 首
- 宋词 2.1万+ 首
- 元曲、五代诗词、诗经、楚辞等

## 🚀 快速开始

### 使用 Docker（推荐）

```bash
docker run -d -p 1279:1279 palemoky/chinese-poetry-api:latest
```

完整配置参见 [docker-compose.yml](docker-compose.yml)。

### 使用 Makefile

```bash
make help          # 查看所有可用命令
make build         # 构建项目
make process-data  # 处理数据
make run-server    # 启动服务
```

所有命令详见 [Makefile](Makefile)。

## 📡 API 使用

### REST API

#### 健康检查
```bash
curl http://localhost:1279/api/v1/health
```

#### 搜索诗词
```bash
# 全文搜索
curl "http://localhost:1279/api/v1/poems/search?q=静夜思"

# 按标题搜索
curl "http://localhost:1279/api/v1/poems/search?q=静夜思&type=title"

# 按作者搜索
curl "http://localhost:1279/api/v1/poems/search?q=李白&type=author"

# 拼音搜索
curl "http://localhost:1279/api/v1/poems/search?q=jingye&type=pinyin"
```

#### 获取单首诗词
```bash
curl http://localhost:1279/api/v1/poems/12345678901234
```

#### 随机诗词
```bash
curl http://localhost:1279/api/v1/poems/random
```

#### 获取作者列表
```bash
curl "http://localhost:1279/api/v1/authors?page=1&page_size=20"
```

#### 获取朝代列表
```bash
curl http://localhost:1279/api/v1/dynasties
```

### GraphQL API

`http://localhost:1279/graphql`

#### 查询示例

#### 搜索诗词

```graphql
query {
  searchPoems(query: "静夜思", searchType: TITLE) {
    edges {
      node {
        title
        paragraphs
        author { name }
      }
    }
    totalCount
  }
}
```

#### 获取作者及其诗词

```graphql
query {
  author(id: "1") {
    name
    dynasty { name }
    poems(page: 1, pageSize: 10) {
      edges {
        node {
          title
          paragraphs
        }
      }
    }
  }
}
```

#### 获取统计信息

```graphql
query {
  statistics {
    totalPoems
    totalAuthors
    totalDynasties
    poemsByDynasty {
      dynasty { name }
      count
    }
  }
}
```

## 🔍 搜索功能

### 1. 全文搜索
支持标题、内容、作者名的 LIKE 模糊搜索

### 2. 拼音搜索
- 完整拼音: `jing ye si` → 静夜思
- 拼音缩写: `jys` → 静夜思
- 作者拼音: `libai` → 李白

### 3. 智能检测
自动识别查询是中文还是拼音（>50% ASCII 字母判定为拼音）

### 4. 搜索类型
- `all`: 全文搜索（默认）
- `title`: 标题搜索
- `content`: 内容搜索
- `author`: 作者搜索
- `pinyin`: 拼音搜索

## 🏷️ 诗词分类

### 按朝代
- 唐、宋、元、五代
- 先秦、两汉、魏晋、南北朝、隋、清

### 按类型（自动识别）
- **绝句**: 五言绝句（4行5字）、七言绝句（4行7字）
- **律诗**: 五言律诗（8行5字）、七言律诗（8行7字）
- **词**: 有词牌名（rhythmic 字段）
- **其他**: 不规则形式

## 🙏 致谢

- 数据来源: [chinese-poetry](https://github.com/chinese-poetry/chinese-poetry)
- 简繁转换: [gocc](https://github.com/liuzl/gocc)
- 拼音转换: [go-pinyin](https://github.com/mozillazg/go-pinyin)
- Web 框架: [Gin](https://github.com/gin-gonic/gin)
- GraphQL: [gqlgen](https://github.com/99designs/gqlgen)
- ORM: [GORM](https://gorm.io/)

## 📮 联系方式

如有问题或建议，欢迎提交 [Issue](https://github.com/palemoky/chinese-poetry-api/issues) 或 [Pull Request](https://github.com/palemoky/chinese-poetry-api/pulls)。
