# 模型健康度功能来源与协议说明

本文档记录本仓库中“模型健康度”功能的来源、实现边界与开源协议处理方式，便于后续维护和分发时追溯。

## 功能来源

本功能参考了 `CassiopeiaCode/new-api-radical` 中的模型健康度页面与统计思路：

- 参考仓库：https://github.com/CassiopeiaCode/new-api-radical
- 参考功能：模型健康度公共页、管理员小时趋势页、按模型展示近 24 小时健康状态
- 迁移日期：2026-05-31

## 当前实现

当前仓库在保留原项目技术栈和工程约束的前提下，实现了等价功能：

- 后端使用 `model_health_request_5m` 5 分钟切片表，按客户端模型名记录请求最终成功、失败、内容阈值达标数和成功请求 token。
- 公共接口提供近 24 小时所有模型健康度数据。
- 管理员接口提供指定模型、指定小时范围的健康度数据。
- 前端在 `web/src/features/model-health` 中提供统计卡片、健康度热力格、搜索、小时趋势图和明细表。

成功率、可用性和状态统一使用 `成功请求数 / (成功请求数 + 失败请求数)`。HTTP 请求和 Responses WebSocket 的每个 `response.create` 在结束时采样一次，中间重试不增加请求数。流式错误、超时和缺失必要终止事件按共享的最终结果分类处理；客户端取消和业务策略拒绝不纳入分母。消费日志只提供使用量，不再直接证明请求成功，关闭日志或性能指标也不影响 HTTP/WebSocket 健康度采样。

内容阈值仍为响应字节数 > 1024、输出 token > 2 或文本长度 > 2，满足任一即可；它是独立的诊断计数，不影响成功率，正常短回复可以成功。成功请求 token 只来自最终成功请求的已记录使用量，不再以 `quota_data` 补入未知结果的历史消费量。任务在首次持久化的立即终态或轮询终态 CAS 成功后采样，提交成功但仍在运行的任务不提前计为成功。

### 升级与历史数据

旧表 `model_health_slice_5m` 包含渠道尝试、消费日志及不完整回填混合计数，无法仅从聚合值恢复最终请求结果。旧日志也可能缺少终止事件、取消原因或最终重试结果。因此保留旧表和数据供检查及回退，升级创建独立的新表，取消从旧日志自动回填；不会把旧计数重新解释为新口径。新版健康度历史从升级后实际采样开始累积，公开接口的 `observed_since` 和页面提示给出当前保留数据的最早切片时间。

所有节点应完成升级后再比较全站统计。新旧版本使用不同的健康度缓存键，滚动升级期间旧节点仍展示及写入旧口径。新版的 35 天清理任务仅处理新表；旧表不会被新版自动删除。回退旧程序可以继续使用旧表，但新版运行期间的请求不会自动补回旧统计。

### 定向验证

`go test ./model -run TestModelHealthDatabaseCompatibility -count=1 -v` 使用 `TEST_HEALTH_MYSQL_DSN` 和 `TEST_HEALTH_POSTGRES_DSN` 指向隔离测试库，覆盖 SQLite、MySQL、PostgreSQL 的新建、旧表共存升级、重复迁移、计数累加及唯一性约束。不要将这些测试变量指向生产数据库。

HTTP/WebSocket 的请求最终结果回归位于 `controller/responses_websocket_test.go`；前端成功率汇总、周期切换和时间线提示回归位于模型健康度模块的 `__tests__` 目录。

本次验证使用真实 SQLite 3.50.4、MySQL 5.7.44、PostgreSQL 9.6.24，以下模型测试在配置上述两个隔离测试库 DSN 后全部通过，包含新建和升级的重复迁移、旧数据保留、唯一性及 94.9999% 阈值精度回归：

```sh
go test ./model -run 'TestModelHealth|TestGetAllModelsHealthTotals|TestDeleteModelHealthSlicesBefore|TestErrorLogsDoNotFinalizeModelHealth' -count=1
```

另外，分别配置 `TEST_RESPONSES_SQL_DSN` 和 `TEST_RESPONSES_LOG_SQL_DSN` 为 MySQL、PostgreSQL 的独立主库与日志库后，以下真实 HTTP/WebSocket 集成测试均通过：

```sh
go test ./controller -run 'TestResponsesHTTPHealthCountsFinalResult|TestResponsesStreamOutcomesPreserveAccounting|TestResponsesInterruptedStreamHealth' -count=1
```

刷盘等待遵守关机上下文的超时，取消等待不会删除已经进入队列的事件；对应回归还覆盖入队后的事件快照。成功率由 Go 使用整数计数计算，避免数据库小数除法舍入改变状态阈值。

## 协议处理

当前仓库和参考仓库均使用 GNU Affero General Public License v3.0（AGPLv3）许可。

本次迁移遵循以下原则：

- 不删除、不替换当前项目已有的 `LICENSE`、版权声明、署名、NOTICE 或项目标识。
- 新增代码继续随当前项目以 AGPLv3 发布。
- 保留当前项目源文件中的 AGPLv3 许可头和版权信息。
- 通过本文档记录参考来源、修改日期和实现差异，便于后续分发时保持来源可追溯。

如果后续公开部署或分发修改后的版本，应继续遵守 AGPLv3 关于源代码可获取性的要求。
