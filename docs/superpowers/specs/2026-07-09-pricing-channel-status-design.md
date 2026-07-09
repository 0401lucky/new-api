# 设计：模型定价表格「渠道状态」统一视图

日期：2026-07-09
状态：已获用户确认

## 背景

用户此前在「系统设置 → 模型定价」自行实现过「只配置渠道有的模型」功能，合并上游前端重构（提交 `13551f11`）时被整体覆盖丢失。本次重做，目标覆盖两个用途：

1. **筛选**：只看渠道实际有的模型，集中配置有用的定价条目。
2. **发现**：找出渠道有、但尚未配置定价的模型，避免漏配。

范围限定：仅「模型定价」主可视化表格（`model-ratio-visual-editor.tsx`）；分层计费编辑器、JSON 编辑器不动。
「渠道有的模型」= 所有渠道（含禁用渠道）的模型并集。

## 方案（已选：状态列统一视图）

表格行 = 已定价模型 ∪ 渠道模型，新增「渠道」状态列 + 工具栏筛选，未定价模型以合成行内联展示、一键补定价。

落选备选：B（筛选开关 + 顶部未定价提示条，体验割裂）、C（独立对账弹窗，多一层入口）。

## 1. 后端

`GET /api/channel/models_enabled` 增加可选查询参数 `scope`：

- 缺省：现状不变，返回启用渠道的去重模型（`abilities` 表 `enabled = true`）。
- `scope=all`：返回所有渠道（含禁用）的去重模型名。

实现：`model/ability.go` 新增 `GetAllChannelModels()`，即 `GetEnabledModels()` 去掉 `enabled` 条件的 distinct 查询；`controller/model.go` 的 `EnabledListModels` 按参数分发。纯 GORM，SQLite/MySQL/PostgreSQL 天然兼容。不新增路由，权限沿用渠道 `read`（`RequireAdminPermission("channel", "read")`）。

## 2. 前端数据流

`model-ratio-visual-editor.tsx`：

- 挂载时用 react-query 拉取 `/api/channel/models_enabled?scope=all`，得到 `string[]`，构建 `Set<string>`（下称 channelModelSet）。
- 行构建（现有 `models` useMemo 处扩展）：
  - **已定价行**：现有快照逻辑不变；每行标记 `channelStatus`：模型名在 channelModelSet 中 → `in_channel`，否则 → `not_in_channel`。
  - **未定价合成行**：channelModelSet 中存在、但定价配置中没有的模型名，生成 `channelStatus = 'unpriced'` 的合成行，倍率等数值字段为空。
- `ModelRow` 类型（`model-pricing-snapshots.ts`）增加可选字段 `channelStatus` 与 `isUnpriced`。

## 3. 表格交互

- **新列「渠道」**（columnId: `channelStatus`）：徽标三态——渠道有（绿）/ 未定价（琥珀）/ 渠道无（灰）。参与现有列显隐持久化（localStorage）。
- **工具栏筛选**：`DataTableToolbar` 的 `filters` 数组追加 `channelStatus` 多选项（与现有 `billingMode` 筛选同款），选项带计数。
- **未定价行**：
  - 各倍率列显示「—」；
  - 操作列仅「添加定价」按钮，点击复用现有添加/编辑面板并预填模型名，保存后自然转为正常行；
  - 不可勾选（排除在行选择/批量删除之外）；
  - 计费模式筛选激活时不匹配任何模式（无 mode 值，被过滤掉属预期）。
- **优雅降级**：渠道模型列表加载中或失败时，不渲染「渠道」列徽标与筛选项，表格行为与现状完全一致。

## 4. 边界与错误处理

- 定价条目与渠道模型均可能数百条，客户端合并 + 现有客户端分页，无性能问题。
- 渠道模型接口失败不阻塞定价编辑（降级），不弹错误打断。
- 合成行不进入保存 payload：保存逻辑仅序列化真实配置行（现状机制天然保证——合成行不在配置 JSON 中）。
- i18n：新文案使用英文 key + `t()`，中文翻译写入 `zh.json`，跑 `bun run i18n:sync` 同步其余语言文件。

## 5. 验证

- 后端：`go build ./...`；`GetAllChannelModels` 行为由控制器参数分发的简单性保证，配合现有路由测试。
- 前端：`tsgo -b` 类型检查、改动文件 oxlint、`rsbuild build`。
- 手动路径：表格出现三态徽标与筛选；勾「渠道无」能看到陈旧条目；未定价行点「添加定价」预填名称、保存后转正。
