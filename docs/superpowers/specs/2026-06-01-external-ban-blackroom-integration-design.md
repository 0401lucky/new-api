# 外部 AI 封禁纳入小黑屋 — 设计文档

- 日期：2026-06-01
- 状态：已确认设计，待写实现计划
- 涉及仓库：
  - `new-api`（本仓库，主改动）
  - 外部工具 `new_api_tools`（fork，本地路径 `D:\code\AndroidStudioprojects\new_api_tools`）

## 1. 背景与问题

当前存在两套互相独立、互不可见的用户封禁机制：

**① new-api 内置「小黑屋」**
- 存储：独立表 `BlackroomBan`（`model/blackroom.go`），**不改 `users.status`**。
- 拦截：认证中间件查 `BlackroomBan` 记录拦截请求；被封用户的 `users.status` 仍是 `1`（启用）。
- 能力完整：列表、按状态/来源筛选、手动封禁、解封、扫描、阶梯封禁规则、多次临时封禁升级永久。
- 来源（`Source`）仅有 `auto`、`manual` 两种。

**② 外部工具的 AI 自动封禁（`new_api_tools`）**
- 封禁动作：`UserManagementService.BanUser()` 直接 `UPDATE users SET status=2` + `UPDATE tokens SET status=2`（`backend/internal/service/user_management.go:452`）。
- 没有独立封禁台账，审计仅写在它自己的 Redis。
- 没有临时/永久之分、没有解封时间、DB 里不存结构化封禁原因。

**核心问题**：被外部工具 AI 封禁的用户是 `status=2`，但小黑屋列表里**完全看不到**（没有对应 `BlackroomBan` 记录）。两套机制各管各的，无法统一查看与解封。

**附带 UI 问题**：小黑屋「配置」弹窗（`blackroom-setting-dialog.tsx`）的 `DialogContent` 只设了 `sm:max-w-2xl`，未设最大高度、表单区也无内部滚动，内容多时会撑爆视口。

## 2. 目标 / 非目标

**目标**
1. 让外部 AI 封禁的用户统一出现在 new-api 小黑屋台账中，来源标记为 `external`。
2. 外部封禁 → 联动 `users.status=2`；在小黑屋解封 → 联动 `users.status=1`，两边始终同步。
3. 外部封禁套用小黑屋的阶梯封禁规则与升级逻辑（规则只有一份，在 new-api）。
4. 存量已被外部封禁的用户（当前 `status=2` 非管理员）一次性导入小黑屋。
5. 修复配置弹窗过大、无内部滚动的 UI 问题。

**非目标**
- 不改变 `auto` / `manual` 两种来源的现有行为（它们仍不联动 `users.status`）。
- 不在外部工具里重新实现阶梯规则引擎（改为走 new-api 接口）。
- 不引入新的封禁判定算法；外部工具的 AI 判定逻辑保持不变，只改「执行封禁」这一步的落地方式。

## 3. 已确认的设计决策

| # | 决策点 | 结论 |
|---|--------|------|
| D1 | 纳入方式 | **方案 2**：改造外部工具，封禁动作改为调用 new-api 小黑屋 HTTP 接口；小黑屋成为唯一封禁台账。排除「外部工具直连 DB 写小黑屋表」（会导致规则引擎重复实现、走样）。 |
| D2 | 与 `users.status` 的联动 | `external` 来源**双向联动**：封禁时设 `status=2`，小黑屋解封时改回 `1`。原生「用户管理」页与小黑屋始终一致。`auto`/`manual` 保持现状不联动。 |
| D3 | 新外部封禁的时长 | **套用小黑屋阶梯规则**：用上报的 `ip_count` 匹配规则 + 升级逻辑。 |
| D4 | 存量导入 | 导入所有当前 `status=2` 的非管理员用户，**一律按永久**。有外部记录的理由就写入，没有则按小黑屋规则生成。 |

## 4. 整体数据流

```
外部工具 AI 判定要封 user X（已持有该用户 24h unique_ips、reason、evidence）
        │  HTTP POST /api/blackroom/external-ban
        │  Header: Authorization: <管理员 access_token>  +  New-Api-User: <管理员 user_id>
        ▼
new-api  ExternalBanBlackroomUser（AdminAuth 守卫）
        │  1) 校验 user 存在且非管理员
        │  2) 定时长：permanent > duration_hours > 阶梯规则(ip_count)+升级 > 永久兜底
        │  3) 理由：请求带了用请求的；没带按小黑屋规则生成
        │  4) UpsertActiveBlackroomBan(source=external)
        │  5) users.status = 2  +  失效用户/令牌/小黑屋缓存
        ▼
   小黑屋列表（来源=外部） + 原生用户管理页   均显示「已封禁」

管理员在小黑屋点「解封」  →  POST /api/blackroom/:id/release
        │  ReleaseBlackroomBan：置记录为 released
        │  若 source=external：users.status 改回 1
        ▼
   小黑屋记录已解封 + 原生用户管理页恢复「正常」（两边同步）
```

可行性已验证：外部工具在封禁执行点（`ai_auto_ban_runtime.go` 的 `executeAutoBanIfNeeded`）可通过 `findAIBanWindowSummary(analysis, "24h")["unique_ips"]` 取到每用户 24 小时不同 IP 数，窗口与 new-api 默认 24h lookback 一致，阶梯规则能真正生效。理由可通过现有 `detailReason(detail)` 取到。

## 5. 详细设计 — new-api 后端

### 5.1 新增来源常量
`model/blackroom.go`：在现有 `BlackroomBanSourceAuto`/`BlackroomBanSourceManual` 旁新增：
```go
BlackroomBanSourceExternal = "external"
```

### 5.2 抽出「时长/升级」决策为共享函数
现状：阶梯规则匹配 + 升级判定内联在 `service/blackroom_task.go` 的 `handleBlackroomCandidate`（约 165-186 行）。规则只能有一份，外部封禁必须复用。

抽出到 `service` 包（`handleBlackroomCandidate` 同包，可访问 `model`），形如：
```go
type blackroomBanDecision struct {
    Matched         bool                          // 是否命中某条阶梯规则
    Permanent       bool
    DurationSeconds int64
    BannedUntil     int64
    Escalated       bool
    Rule            operation_setting.BlackroomRule
}

// allowEscalation 仅在「新建封禁（当前无生效记录）」时为 true，与自动扫描行为一致。
func resolveBlackroomBanDecision(
    setting *operation_setting.BlackroomSetting,
    userID, ipCount int, now int64, allowEscalation bool,
) (blackroomBanDecision, error)
```
- 内部：`operation_setting.MatchBlackroomRule(setting, ipCount)` 匹配；未命中 → `Matched=false`。
- 升级：命中临时规则且 `allowEscalation` 时，按 `EscalationWindowDays` 窗口 `model.CountRecentTemporaryBlackroomBans` 计数，达到 `EscalationTemporaryBanCount` 则升级永久（逻辑照搬现状）。
- `handleBlackroomCandidate` 改为调用此函数，`allowEscalation = (existingErr != nil)`，**行为不变**（重构等价，由现有 `model/blackroom_test.go` 兜底）。

### 5.3 新增服务函数 `CreateExternalBlackroomBan`
放在 `service`（新建 `service/blackroom_ban.go` 或并入 `blackroom_task.go`）：
```go
func CreateExternalBlackroomBan(userID, ipCount int, reason, evidence string,
    permanent bool, durationHours int) (*model.BlackroomBan, error)
```
逻辑：
1. `model.GetUserById` 校验存在；`user.Role >= common.RoleAdminUser` → 返回错误（不封管理员，与手动封禁一致）。
2. 定时长优先级：
   - `permanent==true` → 永久；
   - 否则 `durationHours > 0` → 固定时长 `now + durationHours*3600`；
   - 否则查当前是否有生效封禁，调 `resolveBlackroomBanDecision(setting, userID, ipCount, now, allowEscalation=无生效封禁)`；命中则用其结果；
   - 都没有（未命中规则且无显式时长）→ **永久兜底**。
3. 理由：`reason` 非空用之；否则生成默认值（`ip_count>0` → 形如「外部风控检测到 %d 个不同 IP」；否则「外部 AI 风控封禁」）。
4. `evidence`：请求带的原样存入；为空则存一段结构化 JSON（标注 `source=external`、`ip_count`、`permanent` 等）。
5. `model.UpsertActiveBlackroomBan(BlackroomBanInput{Source: external, ...})` 写台账。
6. `users.status = common.UserStatusDisabled` 的定向更新（`DB.Model(&User{}).Where("id=?").Update("status", ...)`），随后 `model.InvalidateBlackroomUserAuthCache(userID)`（一次性失效小黑屋/用户/令牌缓存，与原生禁用一致；**不动 token 状态**，靠认证查用户状态拦截）。

注意 `UpsertActiveBlackroomBan` 现有 `updateActiveBlackroomBan` 逻辑里 `manual` 优先级高于 `auto`；`external` 与 `auto` 同级处理即可（不需要让 external 覆盖 manual）。

### 5.4 解封联动 `users.status`
`model/blackroom.go` 的 `ReleaseBlackroomBan`：在置记录为 `released` 成功后、现有 `InvalidateBlackroomUserAuthCache` 调用之前，加判断：
```go
if ban.Source == BlackroomBanSourceExternal {
    // users.status 改回 UserStatusEnabled 的定向更新
}
```
`auto`/`manual` 来源不进此分支，行为不变。解封统一在 new-api 小黑屋完成。

### 5.5 控制器与路由
- `controller/blackroom.go` 新增 `ExternalBanBlackroomUser(c *gin.Context)`：解析请求体 → 调 `service.CreateExternalBlackroomBan` → `common.ApiSuccess(c, ban)`。请求 DTO：
  ```go
  type blackroomExternalBanRequest struct {
      UserID        int    `json:"user_id"`        // 必填 > 0
      IpCount       int    `json:"ip_count"`       // 选填，阶梯规则匹配用
      Reason        string `json:"reason"`         // 选填
      Evidence      string `json:"evidence"`       // 选填
      Permanent     bool   `json:"permanent"`      // 选填，强制永久
      DurationHours int    `json:"duration_hours"` // 选填，强制固定时长
  }
  ```
  说明：此端点是内部管理 API，非 relay/convert 路径，**不适用 Rule 6**；非指针标量在此处语义正确——`permanent=false`/`duration_hours=0` 即「未指定，按规则走」，无需区分「显式零值」与「缺省」。
- `router/api-router.go` 在 `blackroomRoute` 组（已 `middleware.AdminAuth()`，约 338-348 行）加：
  ```go
  blackroomRoute.POST("/external-ban", controller.ExternalBanBlackroomUser)
  ```

### 5.6 鉴权
`middleware.AdminAuth()` 接受管理员的访问令牌（`Authorization` 头）+ `New-Api-User` 头。外部工具用其 `NEWAPI_API_KEY`（配为某管理员的 access_token）+ 新增的 `NEWAPI_ADMIN_USER_ID`（作 `New-Api-User` 头）即可调用，无需新增鉴权方式。

## 6. 详细设计 — new-api 前端

### 6.1 修复配置弹窗（UI 问题）
`web/default/src/features/blackroom/components/blackroom-setting-dialog.tsx:101`：
- `DialogContent` 加最大高度并改为纵向 flex：`className='sm:max-w-2xl max-h-[85vh] flex flex-col'`。
- 中间 `<form>` 区加 `overflow-y-auto flex-1 min-h-0`，使表单内容滚动、Header/Footer 固定。
- 仅动这一个组件，外科手术式改动；如手动封禁弹窗（`manual-ban-dialog.tsx`）也有同样隐患，本次仅在确认溢出时一并修，否则只提及不改。

### 6.2 来源新增 `external`
`web/default/src/features/blackroom/constants.ts`：
- `BLACKROOM_SOURCE_VALUES` 增加 `'external'`（驱动筛选下拉，`blackroom-table.tsx` 的 `getBlackroomSourceOptions` 自动带出）。
- `BLACKROOM_SOURCES` 增加 `external: { labelKey: 'External', variant: 'danger' }`（列标签，`blackroom-columns.tsx` 自动渲染）。
- `types.ts` 的 `BlackroomSource` 联合类型补 `'external'`。

### 6.3 i18n
新增键补到全部 6 个语言文件 `web/default/src/i18n/locales/{zh,en,fr,ru,ja,vi}.json`（en 为基准、其余翻译）：
- `External`（来源标签）
- 弹窗修复不引入新文案，无需新键。

## 7. 详细设计 — 外部工具（`new_api_tools`）

### 7.1 新增 new-api 管理客户端
现状：`config.go:47-48,88-89` 已有 `NewAPIBaseURL`（`NEWAPI_BASEURL`）、`NewAPIKey`（`NEWAPI_API_KEY`），但**未用于调用管理 API**（现有 `Authorization: Bearer` 是调大模型的）。
- 新增 env `NEWAPI_ADMIN_USER_ID`（int），作 `New-Api-User` 头。
- 新建极小客户端（如 `backend/internal/service/newapi_admin_client.go`），方法：
  - `BlackroomExternalBan(userID, ipCount int64, reason, evidence string) error` → `POST {base}/api/blackroom/external-ban`，头带 `Authorization: <NEWAPI_API_KEY>`、`New-Api-User: <NEWAPI_ADMIN_USER_ID>`。
  - `BlackroomRelease(...)`（若保留外部解封入口，见 7.4）。

### 7.2 替换封禁执行点
`backend/internal/service/ai_auto_ban_runtime.go` 的 `executeAutoBanIfNeeded`（约 1579 行）：
- 把 `(&UserManagementService{db: s.db}).BanUser(userID, true)` 替换为调用 `BlackroomExternalBan(userID, ipCount, reason, evidence)`。
- `ipCount` 来自 `findAIBanWindowSummary(analysis, "24h")["unique_ips"]`；`reason` 用 `detailReason(detail)`；`evidence` 可序列化 `detail` 的 assessment。
- 接口失败：记日志、`detail["action"]="error"`，**不再静默直改 DB**，下轮扫描自然重试（与现有错误分支一致）。

### 7.3 存量导入（一次性）
- 新增一次性触发动作（手动触发，非常驻）：查询当前 `status=2` 且非管理员的用户，逐个调 `BlackroomExternalBan(userID, ipCount, reason, "")`，强制 `permanent=true`。
- 理由：能从外部审计记录匹配到的带上，否则留空交由 new-api 端生成默认理由。
- 幂等：重复执行经 `UpsertActiveBlackroomBan` 更新生效记录，安全。
- 取舍（已确认）：会纳入所有 `status=2` 非管理员用户（含管理员手动禁用的），它们本就是禁用态，纳入后可在小黑屋统一解封。

### 7.4 外部解封入口（保持一致）
若保留外部工具自带的「解封」按钮（`UnbanUser`），改为调用 new-api `POST /api/blackroom/:id/release`，避免两边脱节；或在文档中明确「解封统一在 new-api 小黑屋操作」，停用外部解封按钮。**实现计划阶段二选一**（默认：改为调 new-api 解封，保持单一入口）。

## 8. 边界与取舍

1. **来源无法 100% 区分**：存量导入把所有 `status=2` 非管理员用户都纳入（含手动禁用），不可避免。已确认接受。
2. **仅 `external` 联动 `status`**：`auto`/`manual` 保持现状不联动，最小改动；若日后要三种来源行为统一，再单独立项。
3. **时长口径**：外部上报的 `ip_count` 用其 24h 窗口统计，与 new-api 默认 24h lookback 一致；若管理员把小黑屋 lookback 改成非 24h，两者口径会有偏差（外部仍按 24h 上报）。可接受，必要时后续让外部窗口跟随设置。
4. **新外部封禁可能比旧「一律永久」宽松**：已选「套用阶梯规则」，符合预期。
5. **token 状态**：external 联动只改 `users.status`，不改 `tokens.status`（与 new-api 原生禁用一致，靠认证查用户状态拦截）。这与外部工具旧行为（同时禁 token）略有差异，但更贴合 new-api 语义。

## 9. 测试策略

**new-api 后端（Go，table-driven，覆盖 SQLite，参考 `model/blackroom_test.go`）**
- `resolveBlackroomBanDecision`：各 `ip_count` 命中/未命中、升级触发/不触发、`allowEscalation` 开关。
- 重构等价：`handleBlackroomCandidate` 现有测试仍全绿。
- `CreateExternalBlackroomBan`：permanent / duration_hours / 阶梯命中 / 永久兜底四条路径；写入 `source=external`；`users.status` 变 2；拒绝管理员。
- `ReleaseBlackroomBan`：`external` 解封后 `users.status` 回 1；`auto`/`manual` 解封不动 `status`。
- 控制器：缺 `user_id`、非法参数的错误返回。

**new-api 前端**
- 构建通过（`bun run build`）；配置弹窗内容超长时出现内部滚动、不溢出（手动/截图验证）；列表来源筛选与标签出现「外部」。

**外部工具**
- 客户端：请求头、URL、错误处理的单测（可对 mock server）。
- 替换点：`executeAutoBanIfNeeded` 在 ban 分支调用新客户端、失败时进 error 分支（现有 `ai_auto_ban_test.go` 风格）。

## 10. 验收标准

1. 外部工具触发一次 AI 封禁后：该用户出现在 new-api 小黑屋列表，来源「外部」，时长符合阶梯规则；原生用户管理页显示「已封禁」。
2. 在 new-api 小黑屋点该记录「解封」：记录变已解封，原生用户管理页恢复「正常」。
3. 运行一次存量导入：所有原 `status=2` 非管理员用户出现在小黑屋，永久封禁，原有理由（若有）保留。
4. 配置弹窗在小屏/内容超长时不撑爆视口，表单区可滚动。
5. new-api 全部新增/既有相关测试通过；前端构建通过。

## 11. 受保护信息

涉及 `QuantumNous`/`new-api` 的品牌、版权、模块路径等受保护标识一律不改（项目 Rule 5）。本设计不触及这些内容。
