# 更新日志

本项目所有值得关注的变更都记录在此文件中，由 [git-cliff](https://git-cliff.org) 根据提交记录生成并人工整理。

## [0.6.1](https://github.com/palemoky/chinese-poetry-api/releases/tag/v0.6.1) - 2026-08-02

### ✨ 新功能

- **rest**: 校验查询参数，`/poems` 支持更多筛选条件 ([e045d6e](https://github.com/palemoky/chinese-poetry-api/commit/e045d6e9128a245761984f61c0bd2add153dd992))

### 🐛 问题修复

- **database**: REST 与 GraphQL 的诗词列表统一按 ID 升序排列 ([79828d7](https://github.com/palemoky/chinese-poetry-api/commit/79828d76ec9a69f2733fb95134879cb50c30b7be))
- **database**: 筛选条件支持重复的诗词类型名 ([8b2273e](https://github.com/palemoky/chinese-poetry-api/commit/8b2273e1deccb7d65d89ea2cd65a7c5322503228))
- **graph**: 拒绝非法的分页参数和格式错误的筛选 ID ([355138b](https://github.com/palemoky/chinese-poetry-api/commit/355138b6f8592c96e1f5ec06b68ebcb5f98fa811))
- **graph**: 修复 `lang` 参数未正确切换简繁体数据的问题 ([0a360f0](https://github.com/palemoky/chinese-poetry-api/commit/0a360f09e1c12203ba6568bb1eeeaa086eca933f))
- **api**: 回收空闲的限流器，分页顺序保持稳定 ([035412e](https://github.com/palemoky/chinese-poetry-api/commit/035412e18a0eca4c17b68ec6c3564c090e090b98))

### ⚡ 性能优化

- **author**: 改用 LEFT JOIN + GROUP BY 统计作者作品数 ([5f1b5be](https://github.com/palemoky/chinese-poetry-api/commit/5f1b5bea6cb2b0c369bfefa276b9a49926584e7f))

### ♻️ 重构

- 数据处理的工作池改用 errgroup 管理并发 ([30717da](https://github.com/palemoky/chinese-poetry-api/commit/30717da7307b81c7a09ce676852a7cd88b5424db))

### 📝 文档

- 移除 README 中的 goreportcard 徽章 ([7ba4794](https://github.com/palemoky/chinese-poetry-api/commit/7ba479404b2600c4d2f2d8932e9e0931f8356b72))

## [0.6.0](https://github.com/palemoky/chinese-poetry-api/releases/tag/v0.6.0) - 2026-07-04

### ✨ 新功能

- **search**: 新增基于 FTS5 trigram 索引的诗词全文搜索 ([1b7ac25](https://github.com/palemoky/chinese-poetry-api/commit/1b7ac25db7dbc1f190b6d1007dce3b603d77bd32))
- **api**: 新增飞花令玩法：按单字随机获取诗词 ([4fbb776](https://github.com/palemoky/chinese-poetry-api/commit/4fbb7767476fc7c78ab015974383968ecbdec862))

### 📝 文档

- **readme**: 使用本地 Logo，统一标点 ([c6cd4b6](https://github.com/palemoky/chinese-poetry-api/commit/c6cd4b64aa7db1facde959e310fd42f7f0b2a454))
- 更新图标链接 ([10fae18](https://github.com/palemoky/chinese-poetry-api/commit/10fae186ca9f5d0a3ed26ee4d0223c3620214d64))
- 更新 README ([88f81d7](https://github.com/palemoky/chinese-poetry-api/commit/88f81d75c66c046b04ae303ed35afd25996d4c7a))
- 新增赞助页面 ([09de5cf](https://github.com/palemoky/chinese-poetry-api/commit/09de5cf7b45ac8dcc98f0005791e6deb7b48753b))
- 新增印章风格 Logo ([5c942b6](https://github.com/palemoky/chinese-poetry-api/commit/5c942b65e73e64d3e4e017c912d3c17a158f7662))
- **github**: Issue 模板改为中文 ([69df351](https://github.com/palemoky/chinese-poetry-api/commit/69df351e892f65ed03a2d3b475298a7cb6942558))

## [0.5.0](https://github.com/palemoky/chinese-poetry-api/releases/tag/v0.5.0) - 2026-04-26

### ✨ 新功能

- 数据处理新增占位符检测，改进诗句规整 ([6a8213d](https://github.com/palemoky/chinese-poetry-api/commit/6a8213d42e8623edbe8e541815e67ebd6b522895))

## [0.4.3](https://github.com/palemoky/chinese-poetry-api/releases/tag/v0.4.3) - 2026-02-24

### ✨ 新功能

- **deps**: 启用 Dependabot 自动更新 Go 模块与 GitHub Actions ([ed686aa](https://github.com/palemoky/chinese-poetry-api/commit/ed686aa346faf96d5291b7250efdef94000e6f9a))

### ♻️ 重构

- 随机诗词改用 count + offset 选取，分布更均匀 ([1649777](https://github.com/palemoky/chinese-poetry-api/commit/1649777d5d01c98ec6a6a55113330e8c529d2751))

### 📝 文档

- 更新 README ([e73fb9a](https://github.com/palemoky/chinese-poetry-api/commit/e73fb9a949a42ba9f6b0b18b4d94a3e74b8ec2e7))

## [0.4.2](https://github.com/palemoky/chinese-poetry-api/releases/tag/v0.4.2) - 2025-12-15

### ⚡ 性能优化

- 新增 `(type_id, id)` 索引，随机诗词改为按 ID 选取，不再使用 OFFSET ([3dd51c8](https://github.com/palemoky/chinese-poetry-api/commit/3dd51c841404ba6e02b8b3e4813b66b3fbddb341))

## [0.4.1](https://github.com/palemoky/chinese-poetry-api/releases/tag/v0.4.1) - 2025-12-15

### ✨ 新功能

- 诗词类型 ID 支持批量查询与缓存 ([31fb663](https://github.com/palemoky/chinese-poetry-api/commit/31fb6639558de5ab37a4d955bdf52a76e00da5bf))

## [0.4.0](https://github.com/palemoky/chinese-poetry-api/releases/tag/v0.4.0) - 2025-12-14

### ✨ 新功能

- 宋词也按词牌名解析标题 ([2ff8dab](https://github.com/palemoky/chinese-poetry-api/commit/2ff8dab1baa777e07c987c951fc5e24906090bad))

## [0.3.3](https://github.com/palemoky/chinese-poetry-api/releases/tag/v0.3.3) - 2025-12-14

### ✨ 新功能

- k6 峰值压测目标从 1000 提高到 3000 并发用户 ([c81fe1d](https://github.com/palemoky/chinese-poetry-api/commit/c81fe1d22cbce1e39f003558f306ff0e0420ae61))
- 随机诗词改用密码学安全的随机数，并按 offset 选取 ([1a572b5](https://github.com/palemoky/chinese-poetry-api/commit/1a572b5225adc50c10db2fba73780cfe61c010b0))
- 随机诗词支持同时按多个诗词类型筛选 ([54a7b68](https://github.com/palemoky/chinese-poetry-api/commit/54a7b68e69c79a832d1fd0a1555999d68c494025))

## [0.3.2](https://github.com/palemoky/chinese-poetry-api/releases/tag/v0.3.2) - 2025-12-14

### ✨ 新功能

- 数据处理前预热朝代与作者缓存，避免 SQLite 锁竞争 ([239533f](https://github.com/palemoky/chinese-poetry-api/commit/239533f9aeae13aaae12f74a2be03fe813610181))

### ♻️ 重构

- Dockerfile 移除非 root 用户及显式属主设置 ([61fcc52](https://github.com/palemoky/chinese-poetry-api/commit/61fcc52eab3579461b062ee395e39b4dd4386882))

## [0.3.0](https://github.com/palemoky/chinese-poetry-api/releases/tag/v0.3.0) - 2025-12-14

### ♻️ 重构

- 精简启动脚本中的 curl 参数 ([c457831](https://github.com/palemoky/chinese-poetry-api/commit/c457831de0756f5d1d1670e67a3e953190e88658))

## [0.2.7](https://github.com/palemoky/chinese-poetry-api/releases/tag/v0.2.7) - 2025-12-14

### ♻️ 重构

- 精简 GitHub 工作流命名 ([b5d55da](https://github.com/palemoky/chinese-poetry-api/commit/b5d55da9c247b4947871475cef914cdde60a9add))
- 改进 Dockerfile 中应用目录的属主设置 ([11b86df](https://github.com/palemoky/chinese-poetry-api/commit/11b86df3b9bb13255a13f6c8f5373b37a9c5837e))

## [0.2.6](https://github.com/palemoky/chinese-poetry-api/releases/tag/v0.2.6) - 2025-12-13

### ♻️ 重构

- 优化 Dockerfile：提前 `USER` 指令，使用 `COPY --chmod` 设置权限 ([f4f1cb9](https://github.com/palemoky/chinese-poetry-api/commit/f4f1cb9c96d18716c49e4a5c80ef2cee60e54093))

## [0.2.5](https://github.com/palemoky/chinese-poetry-api/releases/tag/v0.2.5) - 2025-12-13

### ✨ 新功能

- 数据库下载增加校验和验证，改进更新检查 ([6f1a539](https://github.com/palemoky/chinese-poetry-api/commit/6f1a53976c05cb2f5ed8a215e786199d36831a9b))

### 🔒 安全

- 容器改为以非 root 用户运行 ([0339a8d](https://github.com/palemoky/chinese-poetry-api/commit/0339a8d6ab87999b568dee0370b6aaa77c140ccb))

### 📝 文档

- 更新 README ([cb0ff76](https://github.com/palemoky/chinese-poetry-api/commit/cb0ff76cc6f04883e24205bdea9b547f0b0dd5c2))

## [0.2.4](https://github.com/palemoky/chinese-poetry-api/releases/tag/v0.2.4) - 2025-12-13

### ♻️ 重构

- 移除独立的 search 包，抽取公共方法并补充集成测试 ([965e698](https://github.com/palemoky/chinese-poetry-api/commit/965e698230b0723ebd40681ff702e3e5eefdadb0))

## [0.2.3](https://github.com/palemoky/chinese-poetry-api/releases/tag/v0.2.3) - 2025-12-13

### ✨ 新功能

- 随机诗词改用 `ORDER BY RANDOM()` 实现 ([2260476](https://github.com/palemoky/chinese-poetry-api/commit/22604763ac195265fc04ebf88d9262fef536af96))
- 数据库仓储层按统计、写入、查询拆分模块 ([becbdf8](https://github.com/palemoky/chinese-poetry-api/commit/becbdf8759e71d4e2ff165650a6c0e15eac327e5))

### ⚡ 性能优化

- 调整 k6 压测的请求耗时阈值 ([fc3c00b](https://github.com/palemoky/chinese-poetry-api/commit/fc3c00ba818f868d0be50e148fc6c6eacb146e7b))

## [0.2.2](https://github.com/palemoky/chinese-poetry-api/releases/tag/v0.2.2) - 2025-12-13

### ✨ 新功能

- 新增常规负载压测，调整 k6 峰值与压力测试配置 ([30580ba](https://github.com/palemoky/chinese-poetry-api/commit/30580ba0ac7ca44cb83d9a7e48cc9215d7936e75))
- 诗词搜索支持指定搜索类型和分页 ([2e27e1c](https://github.com/palemoky/chinese-poetry-api/commit/2e27e1cd983ce6bbf1e5e4cfc2d20cf24b6af188))

### ♻️ 重构


## [0.2.1](https://github.com/palemoky/chinese-poetry-api/releases/tag/v0.2.1) - 2025-12-13

### ✨ 新功能

- 随机诗词支持按作者、类型、朝代筛选 ([c87a554](https://github.com/palemoky/chinese-poetry-api/commit/c87a55493a428d17736531bb23b62caa5744f4fe))
- 随机诗词的筛选条件不存在时返回 404 ([b59d53e](https://github.com/palemoky/chinese-poetry-api/commit/b59d53edbcf6cad774ae5750bd7eefc0aba9c84c))
- 数据库连接池大小根据 CPU 核数自适应 ([2b13c84](https://github.com/palemoky/chinese-poetry-api/commit/2b13c84225f84f844de4625071491b71d9faab44))
- 新增多场景的 k6 压测脚本及说明文档 ([7b2371a](https://github.com/palemoky/chinese-poetry-api/commit/7b2371a46a291b6b77840b49d04b3d326117f2a9))

### 📝 文档

- 补充克隆仓库的步骤 ([2a48e62](https://github.com/palemoky/chinese-poetry-api/commit/2a48e624eb9987bc6edb0c369433363c8c659a99))

## [0.2.0](https://github.com/palemoky/chinese-poetry-api/releases/tag/v0.2.0) - 2025-12-12

### ✨ 新功能

- 数据库文件统一存放在 `data/` 目录 ([f6df414](https://github.com/palemoky/chinese-poetry-api/commit/f6df414043e033bcdfabd2560779e09dd4858543))
- 新增分页的诗词列表接口，统计查询改用子查询 ([6d9f4e1](https://github.com/palemoky/chinese-poetry-api/commit/6d9f4e179fb1ff7a3464e45c8be49f5e66471782))
- 新增独立的 lint 工作流，测试工作流加入覆盖率与竞态检测 ([b6550c2](https://github.com/palemoky/chinese-poetry-api/commit/b6550c2dfe7f76ce90212027f63fc7b5b017a1f3))
- 构建 Docker 镜像前须先通过测试 ([dfb70aa](https://github.com/palemoky/chinese-poetry-api/commit/dfb70aaed926d96428f9cdc1ff6feb754d24db6b))
- 作者列表返回朝代名与拼音 ([4e0ab00](https://github.com/palemoky/chinese-poetry-api/commit/4e0ab00b6a370d6cb8e2f05248aca87336d0d28a))
- 作者详情返回拼音与朝代名 ([f7292c0](https://github.com/palemoky/chinese-poetry-api/commit/f7292c04e9b20794b501850587a2949ac0932c94))
- 作者作品列表以嵌套对象返回作者、朝代、类型信息 ([39bbccb](https://github.com/palemoky/chinese-poetry-api/commit/39bbccb182de6245c2456fb5911d5116207edfef))
- 诗词列表以嵌套对象返回关联信息 ([ea12838](https://github.com/palemoky/chinese-poetry-api/commit/ea12838652a54bf45867482ed7fc062f439a7ca0))
- 统一 API 响应格式，分页信息放入嵌套的 pagination 对象 ([4257f29](https://github.com/palemoky/chinese-poetry-api/commit/4257f2921ad86769f9375b1113701ff5163f2a5b))
- 作者与诗词 ID 由哈希改为自增序号 ([0f54b40](https://github.com/palemoky/chinese-poetry-api/commit/0f54b405183816094acb79cccee7c39528aeeadc))
- 过滤空白或只含标点的诗句 ([2d9e40b](https://github.com/palemoky/chinese-poetry-api/commit/2d9e40b18a5f0917f82893706f879428f52705d6))
- 按标题、作者、正文哈希对诗词去重 ([560c185](https://github.com/palemoky/chinese-poetry-api/commit/560c18519146146d6b4ac962e690e03cb4bed15e))
- 新增章节字段，按诗词类型解析标题 ([1f19201](https://github.com/palemoky/chinese-poetry-api/commit/1f192018b43e731e6037b0ee3c381a48ae043bd8))
- 跳过缺少作者或标题的诗词 ([9f32d29](https://github.com/palemoky/chinese-poetry-api/commit/9f32d29287a1621a1ad485da3e6b176fa71070dd))
- 明确「唐诗」「宋词」分类，无作者的诗词归为佚名，允许无标题 ([f96a3a0](https://github.com/palemoky/chinese-poetry-api/commit/f96a3a0d2eee621cf60b035cfc81bbbac54b02a6))
- 结合数据集来源改进诗词类型分类 ([afb7c99](https://github.com/palemoky/chinese-poetry-api/commit/afb7c99b757a72eae030706cb798ab0f69422e72))
- 为缺失作者的诗词设置默认作者，曹操诗集单独分类 ([ba2d4d5](https://github.com/palemoky/chinese-poetry-api/commit/ba2d4d56e915f128c7ef6c47e89f850504c30d86))
- 诗词类型分类时参考标题 ([81a89d0](https://github.com/palemoky/chinese-poetry-api/commit/81a89d09653b4672d87a897e0526c9233d9759e7))
- 跳过规整后正文为空的诗词 ([3496fd7](https://github.com/palemoky/chinese-poetry-api/commit/3496fd755422dda3aba7c704135b18d6aed99f7d))
- 更新分类器的类型定义 ([3329506](https://github.com/palemoky/chinese-poetry-api/commit/3329506913d8342644b2bdb4fe507f627986a706))
- 数据处理完成后以表格对比简繁体数据统计 ([2d95434](https://github.com/palemoky/chinese-poetry-api/commit/2d954345d90669790082dce44192af7379667936))
- 分页的诗词结果以嵌套对象返回 ([f73a57d](https://github.com/palemoky/chinese-poetry-api/commit/f73a57da82000397b5915de9ceb0046334b87a7d))
- 朝代与诗词类型接口不再返回 `created_at` ([d30f93a](https://github.com/palemoky/chinese-poetry-api/commit/d30f93a961e7fdc266f4fa0dd72c41e2d95af208))
- 修正作者朝代查询，补充 GraphQL 与 REST 的分页筛选测试 ([b8d052b](https://github.com/palemoky/chinese-poetry-api/commit/b8d052badab9b9c6ff73f62dd57fafeb26cb666d))
- 引入 zap 结构化日志，统一 API 错误处理 ([f53c014](https://github.com/palemoky/chinese-poetry-api/commit/f53c014130fe6cbddc009fcf6fe9603c5c1dc435))
- 朝代与诗词类型查询支持 `lang` 参数切换简繁体 ([f535360](https://github.com/palemoky/chinese-poetry-api/commit/f5353606d4a1de0b5b058e8a193c08a290d22a39))
- 简体与繁体数据合并为同一个数据库文件 ([4d29054](https://github.com/palemoky/chinese-poetry-api/commit/4d290542ad56dfcfcea1fefc23f7cb924638603f))

### 🐛 问题修复

- 修复批量写入时空诗词导致的异常 ([cdca880](https://github.com/palemoky/chinese-poetry-api/commit/cdca88064b3d55758226a69ba2567ff02247e0b8))

### ♻️ 重构

- 移除数据库路径配置，固定使用 `data/` 目录 ([ab99d38](https://github.com/palemoky/chinese-poetry-api/commit/ab99d382255a7d238489e5a649a4dba7320e771e))
- 抽取诗词格式化逻辑 ([16e6933](https://github.com/palemoky/chinese-poetry-api/commit/16e6933eaff2ae324af3373435066dac28351070))
- 移除拼音字段及拼音搜索 ([fea4e65](https://github.com/palemoky/chinese-poetry-api/commit/fea4e65ce2d45259654522c58d4c36ac5adaeed5))
- 更新朝代表结构定义 ([bce9bc5](https://github.com/palemoky/chinese-poetry-api/commit/bce9bc5d9179df9ceb2723b399f5d63210e1991c))
- 朝代按时间顺序排列 ([8eb4c10](https://github.com/palemoky/chinese-poetry-api/commit/8eb4c1068db144535ea2b698b044397fc5b128c0))
- 移除 `rhythmic` 字段，词牌名并入标题 ([8092be2](https://github.com/palemoky/chinese-poetry-api/commit/8092be2d7ea99acf9c584a2eca54ae1d5eb4b9b3))
- 更新分类器的类型定义 ([d4b1f23](https://github.com/palemoky/chinese-poetry-api/commit/d4b1f23af0edd55f184c40ef1e3b63c3c1a11c06))
- GraphQL：`Poem.paragraphs` 更名为 `content`，移除 `Poem.createdAt` 与 `Author.description` ([3f0eece](https://github.com/palemoky/chinese-poetry-api/commit/3f0eece575755b400d2051969aabcca3965fdf9b))
- 抽取公共的响应处理与格式化逻辑 ([02529cb](https://github.com/palemoky/chinese-poetry-api/commit/02529cb115902865c2ea028639eabcb94d82d888))
- 抽取分页、ID 解析等公共逻辑 ([6d0ea82](https://github.com/palemoky/chinese-poetry-api/commit/6d0ea82b17e94e599e38116b972c3febe7f0568c))
- 简繁体数据统一存储，按语言区分处理 ([5c1639e](https://github.com/palemoky/chinese-poetry-api/commit/5c1639e32b5783b9db791e537b300c1cecf1e2ad))
- 移除拼音与模糊搜索功能及相关配置 ([fcc8755](https://github.com/palemoky/chinese-poetry-api/commit/fcc8755be0059aa6fd8b809df600cd3d6bc881f8))

### 📝 文档

- 更新项目搭建与构建说明 ([ea3c69b](https://github.com/palemoky/chinese-poetry-api/commit/ea3c69b50b082c525faf70795f057a76238246e3))
- 更新 README ([b7ef550](https://github.com/palemoky/chinese-poetry-api/commit/b7ef5508a6abaff8563577a02ff46fb6e897da2b))

## [0.1.2](https://github.com/palemoky/chinese-poetry-api/releases/tag/v0.1.2) - 2025-12-10

### ✨ 新功能

- Docker 构建加入 Go 编译缓存，CI 按需选择构建平台 ([ea0f641](https://github.com/palemoky/chinese-poetry-api/commit/ea0f6418ff22ff1fae730fc7c8a2af6ae80c5ed6))

## [0.1.0](https://github.com/palemoky/chinese-poetry-api/releases/tag/v0.1.0) - 2025-12-10

### ✨ 新功能

- 以子模块方式引入 chinese-poetry 数据集 ([ccf79f2](https://github.com/palemoky/chinese-poetry-api/commit/ccf79f2d1e461fe3682b20cb0714d86614a713ce))
- 实现诗词数据处理 ([ac982e4](https://github.com/palemoky/chinese-poetry-api/commit/ac982e422a81ef69f9318cf04d61a1c0718f6c06))
- 实现 API 服务 ([d890a3d](https://github.com/palemoky/chinese-poetry-api/commit/d890a3d4f35dcd108c6f91eb6130c56a6276e530))
- 实现 GraphQL API ([85a8ab2](https://github.com/palemoky/chinese-poetry-api/commit/85a8ab2147f857ea3b51b6b33a926cceb9942a22))
- 支持 Docker 部署 ([f4b2abe](https://github.com/palemoky/chinese-poetry-api/commit/f4b2abe3004eabd40f045805143cddf286eb1c50))
- 新增 GitHub 工作流 ([94d9308](https://github.com/palemoky/chinese-poetry-api/commit/94d93081cb889afb20b625d748eb1814a4eefa42))
- 数据库访问改用 GORM ([6477770](https://github.com/palemoky/chinese-poetry-api/commit/64777703fb35b6c9bb3d1f25fa7ea77680fbc4d4))
- 优化数据处理 ([3405a4d](https://github.com/palemoky/chinese-poetry-api/commit/3405a4d86705c8c046c1cadb7fdff75d4890308c))
- GraphQL 支持分页 ([2086625](https://github.com/palemoky/chinese-poetry-api/commit/2086625cf221a1bbecfa1e0d38d68c87ae6e0aec))
- 缓存朝代与作者数据 ([67ab715](https://github.com/palemoky/chinese-poetry-api/commit/67ab715e663d3c5d0e2b7949712d8abfba351365))
- 仓储层加入缓存，统一错误上报 ([365126f](https://github.com/palemoky/chinese-poetry-api/commit/365126f2a26a7bdc7e5e8988747794ef102d4ba6))
- 实现 REST API ([b5ebb43](https://github.com/palemoky/chinese-poetry-api/commit/b5ebb43f12fcd2052e99f528178c68e57f161d50))

### 🐛 问题修复

- 解决 N+1 查询问题 ([587c12e](https://github.com/palemoky/chinese-poetry-api/commit/587c12ee442aa5dea992ca0fdc85366b7256ed3c))
- 修复写入事务时进度条显示错误 ([cfa22d5](https://github.com/palemoky/chinese-poetry-api/commit/cfa22d539b4de0dd14b11874d7aed7c579af5b4a))
- 修复逗号分隔符问题 ([126140e](https://github.com/palemoky/chinese-poetry-api/commit/126140e1ec8a357cd319968614ceb948f4eb5ae4))
- 修复朝代的繁体转换 ([70b5eb2](https://github.com/palemoky/chinese-poetry-api/commit/70b5eb29a143f834f553516ef1628e907d2a9281))
- 修复子模块路径 ([ab3918e](https://github.com/palemoky/chinese-poetry-api/commit/ab3918ec8b01668ae2c90cba2d07c0b510a4b906))
- 修正从 GitHub Releases 下载数据库的地址 ([89d1c6e](https://github.com/palemoky/chinese-poetry-api/commit/89d1c6ec56d92088453c2a2f5bb654672aa00db9))

### ⚡ 性能优化

- 数据处理耗时从 150 秒降至 60 秒 ([ac438d1](https://github.com/palemoky/chinese-poetry-api/commit/ac438d1c432615f1b62c944a902f682ae7ba335b))
- 减少内存分配 ([c915649](https://github.com/palemoky/chinese-poetry-api/commit/c915649764cfaa9b6aaf311c2eca7b718f978b18))

### ♻️ 重构

- 以 GORM 替换原生 SQL ([88b5f1a](https://github.com/palemoky/chinese-poetry-api/commit/88b5f1aa13b31d9e542fc9c9214014f9094bb04e))
- 移除 FTS5 ([ec51a6b](https://github.com/palemoky/chinese-poetry-api/commit/ec51a6b03abb000c1192caa2609244342baedc64))
- 重构诗词搜索 ([1606673](https://github.com/palemoky/chinese-poetry-api/commit/16066733500dae75bad3efef83f4ae0cb5443827))
- Go 版本升级至 1.25 ([a95fd35](https://github.com/palemoky/chinese-poetry-api/commit/a95fd354d2e2f2b6b4ffbde04433e21be33d795b))
- 默认端口由 1566 改为 1279（宋亡之年） ([2dc50e9](https://github.com/palemoky/chinese-poetry-api/commit/2dc50e95740f45e22c65641ec6b5ab367ae7d06f))
- `DATABASE_MODE` 改用数值配置 ([14c7d01](https://github.com/palemoky/chinese-poetry-api/commit/14c7d01156db99185d569f3891097e9e059009c0))
- 发布流程自动将最新版本标记为 latest ([418b4d9](https://github.com/palemoky/chinese-poetry-api/commit/418b4d94ad81a5cc6ba3d4ca3d099a528881f8c9))

### 📝 文档

- 更新 README ([e5ca1d5](https://github.com/palemoky/chinese-poetry-api/commit/e5ca1d513eaa51e3ac286202901aff56cf622aa0))


