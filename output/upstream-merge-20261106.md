# 上游兼容合并记录

本次从本地 `1a813e4aa1455d2a9bffd8346b83c8dd8b74f44e` 完整合入上游 `2de51a19e35a602a0041d380b29be7c9bcd1e0e5` 的 185 个提交；共同基点为 `45094bdf4`。处理了 100 个冲突文件，保留完整上游提交历史。集成分支为 `integrate/upstream-20261106`，原状态备份为 `backup/pre-upstream-20261106`。

保留本地四叶草与手帐主题、用户/Token/分组限流及并发限制、限时额度分桶、原子退款、邀请码与随机额度兑换码、用户活跃统计与黑名单、日志用户 ID 与提示词检查筛选。保留 DeepSeek Responses `/v1` 路径修复、全部 anthropic-beta 请求头、CSV 前导空白公式转义，以及不自动归属且不删除的旧 Playground 缓存。

已接入上游鉴权缓存完整性与 Redis 跨槽修复、缓存写入计费、协议转换修复、渠道优先级拖动、任务插件原生路由/流式结算、Vertex/Gemini Omni、邮件通知配置、价格变更审核及同步汇率调整等改动。前端复用现有业务组件；价格审核使用 ConfirmDialog 与 StaticDataTable，复制使用 CopyButton，渠道布局继续使用 DataTablePage。

兼容修复包括：鉴权缓存写入并签名本地 Token 限流字段；消息及日志格式适配；移除重复导入、方法及重复回归用例；节点表兼容旧 `id` 主键和上游 `node_name` 主键；保留手动字体选择与默认 Sans，并支持上游 Auto 字体；补齐批量删除用户的访问令牌验证权限映射。

定价核对使用 [OpenAI Docs 的官方价目表](https://developers.openai.com/api/docs/pricing)：GPT-6 Astra 的 12 小时缓存写入在不超过 272K 输入时为 25 USD/百万 tokens，长上下文为 50 USD/百万 tokens。DeepSeek Search/Fetch 依据[官方价目表](https://api-docs.deepseek.com/quick_start/pricing/)设置为 0.75 USD/千次成功请求。DeepSeek Token 默认价继续使用管理员配置：[V4.1 公告](https://api-docs.deepseek.com/news/news260910)与 Pro 价目表对路由和价格的描述不一致，且仅靠星期无法覆盖官方节假日例外。未覆盖已有管理员报价。

七种前端语言通过项目 i18n 脚本完成合并与同步，并为本地接口兼容消息增加翻译。所有语言缺键数为零；产品名称和协议名称保留原文。

## 验证

后端在一次性 Docker 环境使用 Go 1.26.10，命令均设置 `GOWORK=off`；前端使用任务目录中的 Bun 1.4.4。Docker 镜像、缓存和数据库的实际存储在 D 盘，已确认 Docker 的 C 盘 WSL 路径仍是指向 `D:/Docker/wsl` 的 junction。

| 数据库 | 实测版本 | 结果 |
| --- | --- | --- |
| SQLite | 3.50.4 | 新库、rc.48 升级、重复迁移、已有数据/唯一性/日志索引；鉴权、计费与日志专项通过 |
| MySQL | 5.7.44 | 同上，通过 |
| PostgreSQL | 9.6.24 | 同上，通过 |
| 独立 ClickHouse 日志库 | 25.3.14.14 | 日志导出与访问隔离专项通过 |

新库和升级库各启动合并版本两次，升级基库由上游 `v1.0.0-rc.48` 二进制建立。验证保留用户余额/用量、Token 权限与用量、缓存统计、旧日志及管理员定价；敏感字段以摘要比较，验证 Token 唯一性和日志排序索引。节点表另测新库、本地旧主键、上游旧主键三种布局，三种数据库均通过。

实际运行的主要命令：

```text
GOWORK=off go build ./...
GOWORK=off go vet ./...
GOWORK=off go test -p 2 ./... -count=1
cd relaykit; GOWORK=off go build ./...; GOWORK=off go test ./... -count=1; GOWORK=off go vet ./...
cd tokenkit; GOWORK=off go build ./...; GOWORK=off go test ./... -count=1; GOWORK=off go vet ./...
bun install --frozen-lockfile
bun run typecheck
bun run lint
bun run i18n:sync
bun run build
git diff --check HEAD
python tmp/upstream-merge-20261106/run_migrations.py
./tmp/upstream-merge-20261106/run_database_tests.ps1
```

数据库专项设置了 `TEST_MYSQL_DSN`、`TEST_POSTGRES_DSN`、对应的独立日志 DSN，以及 `AUDIT_*`、`TEST_CACHE_*`、`TEST_WALLET_*`、`TEST_FIXED_*`、`TEST_HEALTH_*`、`TEST_RANKINGS_*` 的一次性数据库 DSN；执行：

```text
go test -p 1 ./model ./service ./controller -run 'DatabaseMatrix|TestMigrationSchemaStability|TestMigrate(TokenKey|PrefillGroup)Uniqueness|TestTokenMigrationFromChar48ToVarchar128|TestUserSessionPreviousRefreshHashMigration|TestWalletFundingRepeatedPreConsumeRefundPreservesAllBuckets' -count=1 -v
go test ./model -run '^TestSubscriptionPurchaseLimit$' -count=1 -v
go test ./model -run '^TestSystemInstanceSchemaCompatibilityDatabaseMatrix$' -count=1 -v
CGO_ENABLED=1 go test -race -p 2 ./model ./controller ./relay/channel/task/jsplugin ./middleware -run 'TestAuthCacheIntegrityDatabaseMatrix/sqlite|TestRelayKeepAlive|TestRelayStreamErrorAfterKeepAlive|TestTaskQueryRedirects|TestModelRateLimitCountsConfiguredGroupsSeparately|TestModelMemoryRateLimitReservesConcurrentSuccessAdmission' -count=1
```

订阅专项使用单独的空数据库，保留测试的安全限制。需要回环数据库的测试通过一次性容器内部回环转发运行，未放宽测试限制或开放宿主机端口。主专项、订阅专项、节点表专项及竞态专项均通过。

前端完整首轮为 256 个文件、2771 个用例，253 个文件/2763 个用例通过；修复剩余问题后，涉及的主题、错误页、价格审核、选择与主题颜色回归共 6 个文件/51 个用例通过。类型检查在 1590 个文件中通过，lint 无 error，保留既有 warning。涉及文件已格式化，并保留原版权头。前端生产构建通过。

任务插件元数据描述已按 Plugin API v1 复核：数值字段描述为计费对象与单价，单位单列；布尔字段描述对应肯定状态，未将技术约束或数字价格写入描述。

安全参考：[ASVS 5.0.0](https://owasp.org/www-project-application-security-verification-standard/)、[Authentication](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)、[Session Management](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)、MFA、OAuth2、CSRF 指南。涉及控制包括 v5.0.0-6.3.1、6.3.4、6.5.1、7.2.1、7.4.1、7.4.2、7.5.1。重点验证缓存伪造/过期/修复、权限撤销、失败计数与重放边界；这不是对整个项目的 ASVS 等级合规声明。

## 本地验收与边界

原 Docker SQLite 数据库的一致性备份位于 `D:/Docker/new-api-local/backups/upstream-merge-20261106/one-api.db`，完整性检查为 `ok`，基线为 4 个用户、0 个 Token。未使用仓库根目录或预览环境的数据库覆盖 Docker 数据。

本地 Compose 已重建并通过健康检查，应用与 Redis 均为 healthy。`/api/status`、`/api/setup`、`/api/pricing`、`/rate-limit`、`/sign-in` 均返回 200；启动日志显示迁移及已有初始化状态正常。数据库完整性为 `ok`，原有 4 个用户、0 个 Token 均保留，节点记录可读取。Docker Hub/软件源连接受本机网络影响；使用任务进程代理重试，运行镜像的 APT 软件源改用 HTTPS，沿用受信任 CA 并增加重试，未修改全局系统代理或关闭证书校验。

成功的本地构建与启动命令：

```text
docker compose -f docker-compose.local.yml build --build-arg HTTP_PROXY= --build-arg HTTPS_PROXY= --build-arg NO_PROXY=localhost,127.0.0.1,deb.debian.org
docker compose -f docker-compose.local.yml up -d --wait
docker compose -f docker-compose.local.yml ps
```

镜像清单为 `sha256:8045369b186067746c09d9b346d3d0a24919dfbdc9acba983e2d25e46fdf4f17`。常规 `up -d --build --wait` 的初次尝试因网络/软件源失败，最终采用上述构建参数完成；未用旧二进制或旧前端替代构建。

内置 Browser 无法连接，未完成浏览器视觉验收。已完成组件行为测试与前端构建；不发起真实供应商付费请求。不涉及生产服务器部署、凭据修改或外部发布。

完整验证日志和一次性验收脚本保留在 `tmp/upstream-merge-20261106/`。用户原有未跟踪截图、历史部署记录和临时文件均保留。
