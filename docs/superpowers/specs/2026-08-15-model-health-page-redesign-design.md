# 模型健康度公开页重构设计（参考 check-cx）

- 日期：2026-08-15
- 状态：已确认
- 参考项目：[BingZi-233/check-cx](https://github.com/BingZi-233/check-cx)（AI 模型健康监测面板，UI 与信息架构参考）

## 1. 背景与现状

项目已有一套基于真实流量的模型健康度系统（迁移自 `CassiopeiaCode/new-api-radical`，见 `docs/model-health.md`）：

- **数据**：`model_health_slice_5m` 表按「5 分钟 × 模型」切片记录成功/失败（成功与错误日志异步打点，`model/model_health_writer.go`）。
- **API**：公开接口 `GET /api/public/model_health/hourly_last24h`（30 秒双层缓存）+ 管理员接口 `GET /api/model_health/hourly`。
- **前端**：公开页 `/model-health`（24 小时热力格）+ 管理员页 `/model-health-hourly`（VChart 面积图 + 表格）。

已知问题：

1. `model_health_slice_5m` 无保留期清理，表无限增长（对比 `perf_metrics` 有 `cleanupExpiredMetrics`）。
2. `BackfillModelHealthSlicesFromLogs` 同步阻塞在两个 API 的请求路径上（管理员接口最大 31 天范围且无缓存）。
3. 公开页与管理员页成功率分档标准不一致（95/80/60/20 vs 99/95/80/50）。
4. 页面无延迟指标；而 `perf_metrics` 表（`model × group × bucket`）已在采集总延迟、TTFT、生成耗时。

## 2. 已确认的产品决策

| 决策点 | 结论 |
|---|---|
| 重构范围 | 前端 + 后端 |
| 页面受众 | 公开状态页（未登录可见，沿用 `model_health` 模块开关与 requireAuth 配置） |
| 数据粒度 | 按模型聚合，不暴露渠道信息 |
| 数据来源 | 真实流量统计（不做主动探测） |
| 后端架构 | 双表联合：成功率/时间线用 `model_health_slice_5m`，延迟用 `perf_metrics` |
| 管理员页 | 本次不动（`/model-health-hourly` 及其接口保持现状） |
| 延迟指标 | 平均延迟 + TTFT 双指标 |

## 3. 非目标

- 不做主动探测子系统（check-cx 的 check_configs/轮询/选主机制不迁移）。
- 不做官方状态轮询（OpenAI/Anthropic status page）。
- 不做维护模式、系统通知横幅、分组拖拽排序。
- 不重构管理员分析页。
- 不做多天延迟历史（延迟只取近 24 小时均值）。
- 不删除旧公开端点 `hourly_last24h`（管理员页 `pickActiveModel` 依赖它）。

## 4. 数据口径

### 4.1 时间线

- 每张模型卡片的时间线固定为**最近 24 小时 × 24 格**（小时粒度，5 分钟切片聚合而来），从旧到新排列（Past → Now）。
- Hover 每格显示：小时时间、成功率、总请求、错误请求、成功 Token。
- 7/15/30 天周期切换**只作用于可用性百分比**，不改变时间线范围（与 check-cx 一致：时间线固定、可用性周期可切）。

### 4.2 可用性

- 口径：N 天窗口内 `SUM(success_qualified_requests) / SUM(total_requests)`，按请求加权。
- 「合格成功」沿用现有 `IsQualifiedSuccess`（`responseBytes > 1024 || completionTokens > 2 || assistantChars > 2`）。
- 配色阈值（沿 check-cx）：≥99% 绿、≥95% 黄、<95% 红。

### 4.3 状态判定

基于**最近 60 分钟滚动窗口**（`slice_start_ts >= now - 3600`，复用 5.2 的汇总查询，避免整点刚过时自然小时样本过小）的成功率：

| 状态 | 条件 |
|---|---|
| `operational` | 成功率 ≥ 95% |
| `degraded` | 80% ≤ 成功率 < 95% |
| `outage` | 成功率 < 80% |
| `no_data` | 最近 60 分钟无请求（灰色展示，不计入全局状态） |

全局徽章聚合：全部 `operational`/`no_data` → `OPERATIONAL`；存在 `degraded` → `DEGRADED`；存在 `outage` → `OUTAGE`（优先级 outage > degraded > operational）。

### 4.4 延迟

- 平均延迟 = 近 24h `SUM(total_latency_ms) / SUM(success_count)`（`perf_metrics` 跨 group 聚合）。
- TTFT = 近 24h `SUM(ttft_sum_ms) / SUM(ttft_count)`。
- 分母为 0（perf_metrics 停用、无流式数据、无流量）时返回 `null`，前端显示「—」。
- 只依赖 perf 表近 24h 数据，不受其 RetentionDays 配置影响。

### 4.5 Token

- 统计卡「总 Token」与卡片内 Token 均为 24h 口径，保留现有 quota_data 兜底逻辑（切片 `success_tokens` 为 0 时回退 `SUM(token_used)`）。
- N 天视图不展示 Token。

## 5. 后端设计

### 5.1 新公开端点

```
GET /api/public/model_health/overview?period=7d|15d|30d
```

- 中间件：`middleware.HeaderNavModuleAuth("model_health")`（与现有公开端点一致）。
- `period` 缺省 `7d`；非法值返回 400。
- 缓存：30 秒 Redis + 进程内双层缓存，按 period 分 key（沿用现有 `getPublicModelHealthCache` 模式）。

响应（`common.ApiSuccess` 包裹）：

```jsonc
{
  "updated_at": 1755244800,        // 服务端生成时间（秒）
  "period": "7d",
  "global_status": "operational",  // operational | degraded | outage
  "models": [
    {
      "model_name": "gpt-4o",
      "status": "operational",        // operational | degraded | outage | no_data
      "availability": 0.9987,          // N 天，无数据时 null
      "availability_success": 12345,   // N 天合格成功请求数
      "availability_total": 12360,     // N 天总请求数
      "avg_latency_ms": 2100,          // 近 24h，无数据 null
      "avg_ttft_ms": 380,              // 近 24h，无数据 null
      "success_tokens_24h": 123456789,
      "timeline": [                    // 恰好 24 项，旧 → 新
        {
          "hour_start_ts": 1755158400,
          "success_rate": 0.99,
          "total_requests": 100,
          "error_requests": 1,
          "success_tokens": 12345
        }
      ]
    }
  ]
}
```

模型排序：24h `success_tokens` 降序（沿现状）。模型集合 = 24h 切片表出现的模型 ∪ 24h quota_data 出现的模型（沿现状）。

### 5.2 数据查询

1. **24h 时间线**：复用 `GetAllModelsHealthHourlyStats`。
2. **N 天可用性汇总**：新增 `GetAllModelsHealthTotals(db, startTs, endTs)`——按模型 `SUM(success_qualified_requests), SUM(total_requests)`，不分时间桶（无方言分支需求，纯 GORM 聚合）。同一函数以 `startTs = now - 3600` 复调一次，得到状态判定所需的最近 60 分钟汇总。
3. **延迟**：扩展 `PerfMetricSummary` 与 `GetPerfMetricsSummaryAll` 增加 `TtftSumMs`/`TtftCount` 两列（对现有 `/api/perf-metrics/summary` 消费者是纯增量 JSON 字段，向后兼容）。
4. **Token 兜底**：复用 `getModelHealthQuotaAggRows`。

### 5.3 治理修复（随本次一并落地）

1. **切片表保留期清理**：新增 `modelHealthCleanupHandler` 注册进 `system_task` 框架（`controller/system_task_handlers.go`，DB 租约多实例去重）。每 24 小时删除 `slice_start_ts < now - 35 天` 的行（35 = 30 天视图 + 余量，代码常量；`Enabled()` 恒真）。新增 `model.DeleteModelHealthSlicesBefore(cutoffTs)`。
2. **回填移出请求路径**：删除两个现有 API 内的同步 `BackfillModelHealthSlicesFromLogs` 调用；改为 `main.go` 启动时 `gopool.Go` 异步回填一次最近 48 小时（保留「升级后立即有数据」的原始目的；`OnConflict DoNothing` 幂等，多实例重复执行安全）。
3. **状态分档常量统一**：新页面使用单一状态元数据源（见 6.2）；管理员页维持现状。

## 6. 前端设计

### 6.1 文件结构

```
web/src/features/model-health/
  public-page.tsx              — 重写：页面骨架 + 数据轮询（30s 轮询 + 下次刷新倒计时）
  components/
    model-health-card.tsx      — 模型卡片
    health-timeline.tsx        — 24 格时间线 + HoverCard 详情 + Past→Now 轴
  status.ts                    — STATUS_META 常量（状态→颜色/徽章样式/文案 key）+ 可用性配色函数
  api.ts                       — 新增 getPublicModelHealthOverview(period)
  types.ts                     — 新增 overview 响应类型
  utils.ts                     — 沿用 formatRate/formatTokens/hourLabel，新增延迟格式化
  hourly-page.tsx              — 不动
```

路由 `web/src/routes/model-health/index.tsx` 不变（仍渲染 `ModelHealthPublicPage`，`beforeLoad` 模块开关校验保留）。

### 6.2 页面布局（自上而下）

1. **Hero 区**（check-cx 编辑风排版）：大字标题 + 副标题 + 右侧全局状态徽章（绿点呼吸动画）+ 更新时间 + 下次刷新倒计时。
2. **控制行**：搜索框（模型名过滤，保留现状逻辑）+ 7/15/30 天 ToggleGroup。
3. **统计卡 ×4**：监控模型数 / 总体成功率(24h) / 总 Token(24h) / 健康模型数。视觉重做，保留骨架屏。
4. **模型卡片网格**（响应式 1/2/3 列），单卡结构（对应 check-cx ProviderCard）：
   - 头部：模型名（等宽字体 + truncate + Tooltip）+ 状态徽章
   - 指标格 ×2：平均延迟 / TTFT（muted 背景圆角小格，等宽大数字，null 显示「—」）
   - 可用性条：`可用性(N天)` 百分比（99/95 配色）+ `M/N 成功`
   - 时间线：24 竖条，hover HoverCard 详情，底部 Past→Now 轴标签
5. **空态 / 加载态**：无模型时空态插画文案；首次加载骨架屏。

### 6.3 交互与实现约束

- 30 秒静默轮询保留，新增倒计时显示（对应 check-cx「Next update in Xs」）。
- 深色模式：全部用 Tailwind `dark:` 变体，禁止硬编码只适配单主题的颜色。
- 使用现有 `@/components/ui/*`（badge、hover-card、tooltip、skeleton、toggle-group 等），不引入新依赖。
- 所有用户可见文案走 `useTranslation()`，英文原文为 key；7 个语言文件（en/zh/zh-TW/fr/ru/ja/vi）同步补齐，`bun run i18n:sync` 校验。
- 旧 `getRateLevel` 热力格分档随旧 UI 一并移除，新分档统一进 `status.ts`。

## 7. 错误处理

| 场景 | 行为 |
|---|---|
| `period` 非法 | 400，`success:false` 提示 |
| perf_metrics 停用/无数据 | `avg_latency_ms`/`avg_ttft_ms` 为 null，前端「—」 |
| 模型近 60 分钟无请求 | `status = no_data`，灰色徽章，不计入全局状态 |
| 全部模型无数据 | `global_status = operational`（无异常证据），页面正常渲染 |
| 接口失败 | 前端 toast + 保留上一次数据（沿现状） |
| 缓存读写失败 | 降级直查（沿现状模式） |

## 8. 测试计划

### 后端（testify require/assert）

1. `GetAllModelsHealthTotals` 聚合正确性（含跨天窗口、空窗口）。
2. 状态判定纯函数表驱动测试（95/80 边界、no_data、全局聚合优先级）。
3. `DeleteModelHealthSlicesBefore` 删除边界（等于/小于 cutoff）。
4. 扩展后的 `GetPerfMetricsSummaryAll` TTFT 列聚合。
5. overview 端点 `period` 参数校验。

### 前端

- `bun run build` 通过；`bun run i18n:sync` 无缺失 key。
- Playwright 视觉验证：浅色/深色/空数据三种状态。

### 回归

- 旧公开端点 `hourly_last24h` 与管理员端点行为不变（管理员页正常）。
- 回填移出后：重启服务，确认启动回填生效、请求路径无回填调用。

## 9. 风险与缓解

| 风险 | 缓解 |
|---|---|
| 双表口径差异（切片表「合格成功」 vs perf 表「成功」） | 可用性与状态只用切片表；perf 表仅供延迟均值，两者不混算 |
| perf_metrics BucketTime 可配为 minute/5min/hour | 聚合按时间范围 SUM，与桶粒度无关 |
| 35 天清理误删多天视图数据 | 35 > 30，含余量；清理任务测试覆盖边界 |
| 旧页面样式常量被其他模块引用 | 重构前 grep 确认 `getRateLevel`/`HealthCell` 仅 public-page 内部使用 |
| `web/default/src` 旧快照误改 | 只改 `web/src`（活跃构建源，`rsbuild.config.ts` 已确认） |
