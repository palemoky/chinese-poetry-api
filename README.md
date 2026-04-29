<div align="center">

<img src="https://avatars3.githubusercontent.com/u/30764933?s=200&v=4" alt="chinese-poetry" height="100px">

<h2>中国古诗词 API 服务</h2>

[![Docker Image](https://img.shields.io/docker/v/palemoky/chinese-poetry-api?sort=semver&label=docker)](https://hub.docker.com/r/palemoky/chinese-poetry-api)
[![Docker Image Size](https://img.shields.io/docker/image-size/palemoky/chinese-poetry-api/latest)](https://hub.docker.com/r/palemoky/chinese-poetry-api)
[![Go Report Card](https://goreportcard.com/badge/github.com/palemoky/chinese-poetry-api)](https://goreportcard.com/report/github.com/palemoky/chinese-poetry-api)
[![pre-commit](https://img.shields.io/badge/pre--commit-enabled-brightgreen?logo=pre-commit)](https://github.com/pre-commit/pre-commit)
[![License](https://img.shields.io/github/license/palemoky/chinese-poetry-api)](https://github.com/palemoky/chinese-poetry-api/blob/main/LICENSE)

基于 Go 语言的高性能中国古诗词 API 服务，支持 REST 和 GraphQL 接口，提供简体/繁体中文、爬虫练习场等功能。

</div>

## ✨ 特性

- 🚀 **高性能**: Go 语言编写，支持并发处理，性能优化（简繁转换 ~300ns/op）
- 📚 **海量数据**: 包含唐诗、宋词、元曲等近 40 万首诗词
- 🔍 **强大搜索**: 支持全文搜索、标题/内容/作者分类搜索
- 🌏 **双语支持**: 同一数据库同时存储简体和繁体中文，通过 `?lang=` 参数切换
- 🎯 **多种接口**: REST API 和 GraphQL 双接口支持
- 🛡️ **限流保护**: 内置 IP 限流，防止滥用
- 🐳 **容器化**: Docker 镜像开箱即用，支持多架构（amd64/arm64）
- 📊 **智能分类**: 按朝代、作者、诗词类型自动分类

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

### 克隆仓库

本项目使用 Git Submodules 管理诗词数据，推荐使用以下命令快速克隆：

```bash
# 完整克隆（包含 submodules）
git clone --recurse-submodules --depth=1 https://github.com/palemoky/chinese-poetry-api.git
```

如果已经克隆了仓库，可以单独更新 submodules：

```bash
git submodule update --init
```

## 📡 API 使用

### 多语言支持

所有接口支持 `lang` 参数切换简繁体：

|  参数值   |       说明       |
| :-------: | :--------------: |
| `zh-Hans` | 简体中文（默认） |
| `zh-Hant` |     繁体中文     |

### REST API

```bash
# 简体中文（默认）
curl "http://localhost:1279/api/v1/poems"

# 繁体中文
curl "http://localhost:1279/api/v1/poems?lang=zh-Hant"

# 搜索诗词
curl "http://localhost:1279/api/v1/poems/search?q=静夜思"

# 随机诗词
curl "http://localhost:1279/api/v1/poems/random"

# 随机诗词（带过滤）
curl "http://localhost:1279/api/v1/poems/random?author=李白"
curl "http://localhost:1279/api/v1/poems/random?type=五言绝句"
curl "http://localhost:1279/api/v1/poems/random?author=李白&type=五言绝句"
curl "http://localhost:1279/api/v1/poems/random?author=李白&type=五言绝句&dynasty=唐"
curl "http://localhost:1279/api/v1/poems/random?author=李白&dynasty=唐&type=五言绝句&type=七言绝句&type=五言律诗"

# 作者列表
curl "http://localhost:1279/api/v1/authors?page=1&page_size=20"

# 朝代列表
curl "http://localhost:1279/api/v1/dynasties"
```

### GraphQL API

端点: `http://localhost:1279/graphql`

```graphql
# 繁体中文查询
query {
  poems(lang: ZH_HANT, pageSize: 10) {
    edges {
      node {
        title
        content
        author {
          name
        }
      }
    }
    totalCount
  }
}

# 搜索诗词
query {
  searchPoems(query: "静夜思", searchType: TITLE) {
    edges {
      node {
        title
        author {
          name
        }
      }
    }
  }
}

# 统计信息
query {
  statistics {
    totalPoems
    totalAuthors
    poemsByDynasty {
      dynasty {
        name
      }
      count
    }
  }
}
```

## 🔍 搜索功能

|   类型    |       说明       |             示例             |
| :-------: | :--------------: | :--------------------------: |
|   `all`   | 全文搜索（默认） |           `?q=月`            |
|  `title`  |     标题搜索     |    `?q=静夜思&type=title`    |
| `content` |     内容搜索     | `?q=床前明月光&type=content` |
| `author`  |     作者搜索     |    `?q=李白&type=author`     |

## 📖 数据集

本项目基于 [chinese-poetry](https://github.com/chinese-poetry/chinese-poetry) 数据集，包含:

| 分类 | 数量 | 分类 | 数量 | 分类 | 数量 | 分类 | 数量 |
| :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: |
| 五言绝句 | 18,895 | 七言绝句 | 85,032 | 五言律诗 | 71,400 | 七言律诗 | 69,028 |
| 乐府诗   | 9,315  | 五代词   | 543    | 宋词     | 21,369 | 元曲     | 10,905 |
| 诗经     | 305    | 楚辞     | 65     | 论语     | 20     | 四书五经 | 14     |
| 其他     | 96,232 |         |        |          |        |        |        |

```mermaid
pie title 收录数据分布概览 (忽略极小值)
"七绝/七律" : 154060
"五绝/五律" : 90295
"宋词/五代词" : 21912
"元曲" : 10905
"乐府诗" : 9315
"其他" : 96232
```

## 🙏 致谢

- 数据来源: [chinese-poetry](https://github.com/chinese-poetry/chinese-poetry)
- 简繁转换: [gocc](https://github.com/liuzl/gocc)
