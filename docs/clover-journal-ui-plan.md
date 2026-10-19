# 四叶草手帐 UI 改造计划

## 文档信息

- 文档版本：v2.0
- 文档状态：P7 全部完成，项目收尾
- 创建日期：2026-10-17
- 最后更新：2026-10-19
- 涉及模块：`web/` 前端主题、布局、基础组件、页面视觉
- 目标主题：`clover-journal`（四叶草手帐）
- 范围边界：仅做 UI；不改后端、API、计费、权限、路由语义

> 进度维护约定：每完成一项，将任务前的 `- [ ]` 改为 `- [x]`，并同步更新「进度总览」与「变更记录」。

## 一、背景与目标

现有 `lucky-clover` 预设已经提供绿色配色、圆角卡片、胶囊按钮和柔和阴影，但整体仍是「清爽配色皮肤」，与参考图的纸品手帐视觉语言存在明显差距。

本次目标：

1. 新增独立主题预设 `clover-journal`，保留现有 `lucky-clover`，可随时切回。
2. 以「四叶草植物手帐」为视觉语言，重构全站布局与基础组件外观。
3. 按页面类型分级控制装饰强度，避免管理表格被贴纸、旋转和手写批注拖累可用性。
4. 同步设计「夜间手帐」暗色模式。
5. 所有改动限定在前端 UI 层，不改变任何业务行为。

## 二、已确认决策

| 决策项 | 结论 |
| --- | --- |
| 主题落地方式 | 新增独立预设 `clover-journal`，保留 `lucky-clover` |
| 改造深度 | 全站布局重构；上游合并冲突后续单独解决 |
| 覆盖范围 | 全站覆盖 + A/B/C 三级装饰强度 |
| 暗色模式 | 设计「夜间手帐」暗色体系，不做简单反色 |
| 默认预设 | 待定；建议 P1 起临时设为默认便于验收，P7 由项目所有者确认是否保留 |
| 业务边界 | 仅 UI，不改 API、数据逻辑、计费、权限和路由语义 |

## 三、设计规格

> 方向值已在 P0-2 定稿：精确令牌见 3.7，页面构图见 3.8。

### 3.1 视觉基调

- 奶油纸张画布，叠加极细纸纹与绿色水彩晕染。
- 纸白卡片浮在纸面之上，用纸胶带、回形针、便签和相纸制造手帐层次。
- 植物插画以手绘线条为主，避免写实照片感。
- 数据区域保持克制：不旋转、不贴胶带、不压纹理，优先保证可读性。

### 3.2 色彩方向

| 角色 | 方向 |
| --- | --- |
| 纸张底色 | 奶油白、暖米色 |
| 主色 | 四叶草绿 |
| 深色强调 | 深林绿 |
| 辅助色 | 嫩叶绿、暖金、陶土红、雾蓝 |
| 文字 | 绿黑色，避免纯黑 |
| 边框 | 鼠尾草绿细线 |
| 暗色画布 | 深绿夜纸，非纯黑 |
| 暗色文字 | 奶油白 |

### 3.3 字体

| 用途 | 方案 |
| --- | --- |
| 正文与数据 | 沿用 Public Sans，保证密集数据可读性 |
| 标题 | Lora / 宋体感处理，与手帐气质协调 |
| 手写批注 | Caveat（拉丁）+ 系统楷体（CJK 回退） |
| 等宽数据 | 沿用现有等宽字体与 `tabular-nums` |

不引入全量中文手写字体，避免数 MB 的字体体积；如后续需要统一中文手写效果，再单独评估字体子集化。

### 3.4 形状、纹理与装饰

- 不规则圆角：卡片四角使用不同半径，模拟手工裁纸。
- 纸胶带：半透明条纹渐变 + 轻微旋转，只出现在卡片角落或标题上方。
- 回形针、便签、相纸：内联 SVG 或 CSS 实现，全部 `aria-hidden`、`pointer-events: none`。
- 纸纹：单张 128×128 纹理全局复用，不在每张卡片上运行动态滤镜。
- 撕边与打孔线：用 `mask-image` 或伪元素实现，不切割真实内容与焦点轮廓。
- 装饰旋转只作用于装饰层，交互卡片本身保持水平。

### 3.5 分级强度

| 级别 | 适用页面 | 装饰内容 |
| --- | --- | --- |
| `immersive` | 首页、登录/注册、定价/模型广场、排行、关于、仪表盘、钱包、订阅 | 纸张、胶带、贴纸、手绘、手写批注、相纸 |
| `medium` | 日志、密钥、任务、个人资料、邀请、安全、模型健康、聊天、Playground | 纸感、配色、少量胶带/角标 |
| `calm` | 渠道、用户、系统设置、模型管理、动态倍率、黑名单、指纹、限流 | 只保留纸张质感、配色与圆角，不加贴纸/旋转/批注 |

实现方式：页面容器携带 `data-journal-level`，主题 CSS 按级别启用装饰；未标记的页面默认按 `calm` 处理。

### 3.6 组件规格

| 组件 | 手帐化方向 |
| --- | --- |
| Card | 纸白卡面、不规则圆角、纸影、可选胶带/回形针装饰 |
| Button | 胶囊或叶片形；主按钮为绿色墨章，次按钮为纸片 |
| Tabs | 叶片形标签，选中态为贴纸纸片 |
| Table | 纸面行、虚线分隔、便签式表头；数据区不旋转 |
| Dialog/Sheet | 纸张弹层、顶部胶带、贴纸关闭按钮 |
| Input/Select | 纸面内嵌、缝线或虚线下边、绿色高亮焦点 |
| Badge | 印章式标签；表格内保持水平 |
| Chart | 植物色板、点线网格、圆角柱、便签式 tooltip |
| Empty/Loading/Error | 手绘涂鸦 + 便签说明；骨架屏为纸条 |

### 3.7 定稿令牌（P0-2）

#### 浅色令牌

| 令牌 | 定稿值 |
| --- | --- |
| `--background` | `oklch(0.965 0.018 95)` |
| `--foreground` | `oklch(0.32 0.04 155)` |
| `--card` | `oklch(0.992 0.01 95)` |
| `--card-foreground` | `oklch(0.32 0.04 155)` |
| `--popover` | `oklch(0.995 0.008 95)` |
| `--primary` | `oklch(0.46 0.09 158)` |
| `--primary-foreground` | `oklch(0.985 0.012 95)` |
| `--secondary` | `oklch(0.92 0.035 145)` |
| `--secondary-foreground` | `oklch(0.36 0.055 150)` |
| `--muted` | `oklch(0.945 0.018 100)` |
| `--muted-foreground` | `oklch(0.50 0.025 140)` |
| `--accent` | `oklch(0.90 0.06 135)` |
| `--accent-foreground` | `oklch(0.30 0.05 150)` |
| `--destructive` | `oklch(0.56 0.15 35)` |
| `--success` | `oklch(0.55 0.13 155)` |
| `--warning` | `oklch(0.76 0.12 82)` |
| `--warning-foreground` | `oklch(0.26 0.04 80)` |
| `--info` | `oklch(0.58 0.07 210)` |
| `--border` | `oklch(0.87 0.035 140)` |
| `--input` | `oklch(0.88 0.035 140)` |
| `--ring` | `oklch(0.62 0.12 150)` |
| `--sidebar` | `oklch(0.952 0.022 100)` |
| `--sidebar-accent` | `oklch(0.90 0.055 135)` |
| `--sidebar-accent-foreground` | `oklch(0.28 0.05 150)` |
| `--table-header` | `oklch(0.945 0.03 115)` |
| `--table-header-hover` | `oklch(0.93 0.038 115)` |
| `--chart-1` | `oklch(0.50 0.10 155)` |
| `--chart-2` | `oklch(0.68 0.13 145)` |
| `--chart-3` | `oklch(0.76 0.12 82)` |
| `--chart-4` | `oklch(0.62 0.07 205)` |
| `--chart-5` | `oklch(0.56 0.15 35)` |
| `--radius` | `1.25rem` |

#### 暗色令牌（夜间手帐）

| 令牌 | 定稿值 |
| --- | --- |
| `--background` | `oklch(0.22 0.025 155)` |
| `--foreground` | `oklch(0.91 0.02 95)` |
| `--card` | `oklch(0.27 0.028 155)` |
| `--card-foreground` | `oklch(0.91 0.02 95)` |
| `--popover` | `oklch(0.29 0.03 155)` |
| `--primary` | `oklch(0.72 0.12 150)` |
| `--primary-foreground` | `oklch(0.18 0.03 155)` |
| `--secondary` | `oklch(0.32 0.03 150)` |
| `--secondary-foreground` | `oklch(0.90 0.02 95)` |
| `--muted` | `oklch(0.29 0.022 155)` |
| `--muted-foreground` | `oklch(0.74 0.02 110)` |
| `--accent` | `oklch(0.34 0.045 145)` |
| `--accent-foreground` | `oklch(0.93 0.02 95)` |
| `--destructive` | `oklch(0.68 0.16 30)` |
| `--success` | `oklch(0.74 0.13 150)` |
| `--warning` | `oklch(0.82 0.12 82)` |
| `--info` | `oklch(0.70 0.07 210)` |
| `--border` | `oklch(0.80 0.05 150 / 14%)` |
| `--input` | `oklch(0.85 0.05 150 / 20%)` |
| `--ring` | `oklch(0.72 0.12 150)` |
| `--sidebar` | `oklch(0.20 0.022 155)` |
| `--sidebar-accent` | `oklch(0.34 0.045 145)` |
| `--chart-1` | `oklch(0.72 0.12 150)` |
| `--chart-2` | `oklch(0.68 0.11 135)` |
| `--chart-3` | `oklch(0.82 0.12 82)` |
| `--chart-4` | `oklch(0.70 0.07 205)` |
| `--chart-5` | `oklch(0.72 0.12 35)` |

#### 字体与排版

| 令牌/用途 | 定稿值 |
| --- | --- |
| 正文 `--font-body` | `var(--font-sans)`，沿用 Public Sans |
| 标题 `--font-display` | `var(--font-serif)`，Lora + 中文宋体回退 |
| 手写 `--font-hand` | `'Caveat Variable', 'Caveat', 'KaiTi', 'STKaiti', 'Kaiti SC', '楷体', cursive` |
| 手写使用范围 | 仅装饰性批注、页脚落款、空状态提示，不用于正文和数据 |
| 数字 | 沿用 `tabular-nums`，表格与统计卡保持等宽对齐 |

#### 形状、阴影与纹理

| 令牌 | 定稿值 |
| --- | --- |
| `--journal-card-radius` | `1.35rem 1.6rem 1.3rem 1.55rem / 1.5rem 1.3rem 1.6rem 1.35rem` |
| `--journal-paper-shadow` | `0 1px 0 oklch(0.45 0.05 150 / 0.08), 0 10px 24px -12px oklch(0.35 0.05 150 / 0.28)` |
| `--journal-paper-shadow-hover` | `0 2px 0 oklch(0.45 0.05 150 / 0.1), 0 16px 32px -14px oklch(0.35 0.05 150 / 0.34)` |
| 暗色纸影 | `0 1px 0 oklch(0.10 0.02 155 / 0.5), 0 12px 28px -12px oklch(0.08 0.02 155 / 0.6)` |
| 纸纹 | 128×128 `feTurbulence` SVG，浅色 `multiply`、暗色 `screen`，透明度 0.035 / 0.05 |
| 水彩晕染 | 左上叶绿、右上暖金两组径向渐变，`background-repeat: no-repeat` |
| 胶带 | 宽 5–8rem、高 1.2–1.5rem、旋转 -3°–2°、透明度 0.65–0.8 |
| 回形针 | 1.5rem 内联 SVG，右上角，旋转 8°，`currentColor` |
| 便签 | 暖金纸色、旋转 -2°、折角、纸影 |
| 涂鸦 | 1.5px 手绘线条，`currentColor`，透明度 0.55 |
| 旋转上限 | 装饰层 ±4°，交互元素与表格文字 0° |

#### 装饰组件定稿

| 组件 | 规格 |
| --- | --- |
| `CloverDoodle` | 四叶草、单叶、枝叶、小花四种变体；尺寸 `sm/md/lg`；`currentColor` |
| `WashiTape` | 绿、金、陶土三种色；可设旋转角与宽度；半透明条纹 |
| `PaperClip` | 银色或绿色描边；右上角定位辅助 |
| `StickyNote` | 便签纸形、折角、可选手写文案 |
| `HandNote` | 手写字体 + 可选箭头/爱心涂鸦 |
| `PaperGrain` | 纯装饰纹理层，`pointer-events: none` |

#### 组件定稿

| 组件 | 定稿规则 |
| --- | --- |
| Card | 纸白卡面、1px 鼠尾草边框、`--journal-card-radius`、纸影；A 级可选胶带/回形针 |
| Button | 主按钮深林绿胶囊；次按钮纸片；ghost 高亮笔触；危险操作陶土红 |
| Tabs | 叶片形标签；选中态纸片 + 轻微胶带；C 级保持水平无胶带 |
| Table | 表头纸带、虚线分隔、悬停高亮笔触；数据区不旋转、不贴纸 |
| Dialog/Sheet | 纸张弹层、顶部胶带、贴纸关闭按钮 |
| Input/Select | 纸面内嵌、鼠尾草描边、绿色高亮焦点 |
| Badge | 印章式标签；表格内保持水平 |
| Chart | 植物色板、点线网格、圆角柱、便签式 tooltip |
| Empty/Loading/Error | 手绘涂鸦 + 便签说明；骨架屏为纸条 |

### 3.8 页面构图定稿

| 页面 | 构图 |
| --- | --- |
| 登录/注册 | 桌面左右分栏，左侧手帐封面 + 拍立得 + 手写批注，右侧纸白表单卡；移动端表单优先堆叠 |
| 首页 | 手帐拼贴首屏、相纸截图、胶带分区、手写批注 |
| 定价/模型广场 | 左侧活页索引筛选栏，右侧模型卡片网格；搜索条为纸面胶囊 |
| 仪表盘概览 | 顶部纸带导航、左侧笔记本侧边栏、手帐式概览卡片 |
| 仪表盘模型分析 | 标题 + 叶片标签页 + 回形针统计卡 + 性能健康卡 + 模型成功率列表 + 纸面明细表 |
| 管理表格 | 保留纸感与配色，隐藏贴纸、旋转和手写批注 |

## 四、页面分级清单

### A 级（immersive）

| 路由 | 功能模块 |
| --- | --- |
| `/` | `features/home` |
| `/(auth)/*` | `features/auth` |
| `/pricing`、`/pricing/$modelId` | `features/pricing` |
| `/rankings` | `features/rankings` |
| `/about` | `features/about` |
| `/_authenticated/dashboard/*` | `features/dashboard` |
| `/_authenticated/wallet` | `features/wallet` |
| `/_authenticated/subscriptions` | `features/subscriptions` |

### B 级（medium）

| 路由 | 功能模块 |
| --- | --- |
| `/_authenticated/keys` | `features/keys` |
| `/_authenticated/usage-logs/*` | `features/usage-logs` |
| `/_authenticated/active-tasks` | `features/active-tasks` |
| `/_authenticated/recent-calls` | `features/recent-calls` |
| `/_authenticated/profile` | `features/profile` |
| `/_authenticated/invite-codes` | `features/invite-codes` |
| `/_authenticated/security` | `features/security` |
| `/_authenticated/model-health-hourly` | `features/model-health` |
| `/_authenticated/playground` | `features/playground` |
| `/_authenticated/chat/*` | `features/chat` |
| `/_authenticated/task-plugins` | `features/task-plugins` |
| `/_authenticated/redemption-codes` | `features/redemption-codes` |
| `/_authenticated/donations/*` | `features/donations` |
| `/model-health` | `features/model-health` |
| `/console/log`、`/console/topup` | `features` 控制台页面 |
| 错误页 401/403/404/500/503 | `features/errors` |

### C 级（calm）

| 路由 | 功能模块 |
| --- | --- |
| `/_authenticated/channels` | `features/channels` |
| `/_authenticated/users` | `features/users` |
| `/_authenticated/models/*` | `features/models` |
| `/_authenticated/system-settings/*` | `features/system-settings` |
| `/_authenticated/dynamic-ratio` | `features/dynamic-ratio` |
| `/_authenticated/blackroom` | `features/blackroom` |
| `/_authenticated/fingerprints` | `features/fingerprints` |
| `/_authenticated/rate-limit` | `features/rate-limit` |
| `/_authenticated/system-info` | `features/system-info` |
| `/_authenticated/system-update` | `features/system-update` |
| `/setup` | `features/setup` |

## 五、任务看板

### P0 设计确认（编码前）

- [x] **P0-1 生成概念图**
  - 目标：用生图 skill 生成登录页、仪表盘模型分析、定价页三张概念图，作为视觉基准。
  - 产物：概念图存本地临时目录 `tmp/clover-journal/`，不提交仓库。
  - 实际产出：
    - 登录页：`tmp/clover-journal/20261017_175422.png`
    - 定价/模型广场：`tmp/clover-journal/20261017_175631.png`
    - 仪表盘模型分析：`tmp/clover-journal/20261017_175754.png`
  - 验收：三张图已生成，浅色方向与参考图一致；暗色方向留到 P1-3/P6-1 用令牌实现，不再单独出图。

- [x] **P0-2 固化设计规格**
  - 目标：把色彩令牌、字体、圆角、纹理、装饰用法、组件规格和页面分级写回本文档。
  - 验收：本文档 3.7 与 3.8 已给出精确令牌、组件规则和页面构图，无“待定”项。

### P0 概念图结论

三张概念图确认以下语言可落地：

| 页面 | 可复用结论 |
| --- | --- |
| 登录页 | 桌面左右分栏成立：左侧手帐封面 + 拍立得 + 手写批注，右侧纸白表单卡 + 和纸胶带 + 回形针；表单控件保持现代清晰，主按钮深林绿 |
| 仪表盘 | 顶部纸带导航 + 左侧笔记本侧边栏；叶片形标签页；统计卡用回形针和纸胶带；性能健康卡与模型成功率列表在同一纸面；数据区不旋转 |
| 定价/模型广场 | 筛选栏做成活页索引；搜索条为纸面胶囊；模型卡片用胶带/相纸；空状态用手绘放大镜与四叶草 |
| 共同语言 | 奶油纸、细纸纹、水彩晕染、四叶草绿/深林绿/暖金、纸影、胶带、回形针、便签、手写批注、不规则圆角 |

实现注意：

- 概念图中的部分文字细节不作为实现依据，所有文案仍走 i18n。
- 中文手写效果用系统楷体回退，不追求与概念图完全一致。
- 装饰只在 A/B 级页面出现；概念图中的贴纸密度在 C 级管理页需要明显降低。

### P1 主题基础

- [x] **P1-1 注册预设 `clover-journal`**
  - 文件：`web/src/lib/theme-customization.ts`、`web/src/components/config-drawer.tsx`、`web/src/i18n/locales/*.json`
  - 内容：预设条目、色卡预览、`preset.clover-journal` 七语言文案、可选默认字体映射。
  - 验收：配置抽屉出现「四叶草手帐」，切换后其他预设零变化。

- [x] **P1-2 浅色令牌、纸纹与晕染**
  - 文件：`web/src/styles/theme-presets.css`
  - 内容：`[data-theme-preset='clover-journal']` 完整令牌、纸张纹理、角落晕染、纸影。
  - 验收：浅色下卡片、画布、边框、图表色均使用新令牌。

- [x] **P1-3 夜间手帐暗色令牌**
  - 文件：`web/src/styles/theme-presets.css`
  - 内容：`.dark [data-theme-preset='clover-journal']` 令牌、低饱和装饰、深色纸影。
  - 验收：暗色下无刺眼纯白、无低对比文字，装饰不发光。

- [x] **P1-4 手写字体轴 `--font-hand`**
  - 文件：`web/src/styles/theme.css`、`web/src/styles/theme-presets.css`、`web/package.json`
  - 内容：Caveat 字体依赖或本地字体方案、CJK 楷体回退栈。
  - 验收：拉丁手写效果生效，中文回退到楷体，无字体加载阻塞。

- [x] **P1-5 装饰组件集**
  - 文件：`web/src/components/clover-journal/`
  - 内容：`CloverDoodle`、`WashiTape`、`PaperClip`、`StickyNote`、`HandNote`、`PaperGrain`。
  - 验收：全部内联 SVG/CSS，支持 `currentColor` 与尺寸变体，`aria-hidden`、`pointer-events: none`，有单元测试。

- [x] **P1-6 分级容器与 `data-journal-level`**
  - 文件：`web/src/components/clover-journal/`、`web/src/components/layout/components/section-page-layout.tsx`
  - 内容：`JournalPage`/`JournalSection` 容器与级别属性；CSS 按级别启用装饰。
  - 验收：未标记页面按 `calm` 渲染；A/B 级页面可独立开关装饰。

### P1 实施记录

- 预设注册：`web/src/lib/theme-customization.ts`、7 个语言包、配置抽屉色卡。
- 令牌：`web/src/styles/theme-presets.css` 新增 `[data-theme-preset='clover-journal']` 与 `.dark` 块。
- 字体：新增 `@fontsource-variable/caveat`，`--font-hand` 与 `--font-display` 落在 `theme.css`。
- 装饰组件：`web/src/components/clover-journal/` 下六个组件与 `__tests__/decorations.test.tsx`。
- 验证结果：
  - `bun run typecheck` 通过。
  - `bunx oxlint -c .oxlintrc.json ...` 通过。
  - `bun run test src/components/clover-journal/__tests__/decorations.test.tsx` 4/4 通过。
  - `bun run test src/context/__tests__/theme-preferences.test.tsx` 9/9 通过。
  - 浅色/暗色登录页截图：`tmp/clover-journal/verify-signin-light-2.png`、`verify-signin-dark-2.png`。
- 已知遗留：`SectionPageLayout` 尚未接入 `journalLevel` 属性，应用外壳仍是旧布局，留到 P2 处理。

### P2 应用外壳（全站布局重构）

- [x] **P2-1 顶栏手帐化**
  - 文件：`web/src/components/layout/components/header.tsx`、`app-header.tsx`、`public-header.tsx`
  - 内容：纸带导航、印章式 Logo、缝线图标按钮、撕边或虚线底边。
  - 验收：导航、搜索、通知、语言、配置、头像功能与键盘顺序不变。

- [x] **P2-2 侧边栏笔记本化**
  - 文件：`web/src/components/layout/components/app-sidebar.tsx`、`sidebar-view-header.tsx`、`nav-group.tsx`、`nav-link-item.tsx`
  - 内容：笔记本脊线、手写分组标题、荧光笔选中态、折叠态纸签。
  - 验收：展开/折叠、视图切换、移动端抽屉、选中态与焦点均正常。

- [x] **P2-3 页面标题区与内容区**
  - 文件：`web/src/components/layout/components/section-page-layout.tsx`、`main.tsx`、`page-footer.tsx`、`footer.tsx`
  - 内容：手绘下划线标题、页码/日期、纸片操作按钮、页脚手写落款。
  - 验收：标题截断、操作换行、固定内容区与滚动内容区行为不变。

- [x] **P2-4 公共站外壳**
  - 文件：`web/src/components/layout/components/public-layout.tsx`、`public-header.tsx`、`public-navigation.tsx`
  - 内容：纸带页头、植物角落插画、手帐式页脚。
  - 验收：首页、定价、排行、关于页面共用外壳正常，移动端导航可用。

- [x] **P2-5 登录/注册外壳**
  - 文件：`web/src/features/auth/auth-layout.tsx`、`web/src/features/auth/sign-in/`、`sign-up/`
  - 内容：桌面左右分栏（手帐封面 + 表单纸页），移动端堆叠；保留现有表单逻辑。
  - 验收：登录、注册、找回密码、OTP、OAuth 流程不变；表单校验与错误提示正常。

### P2 实施记录

- 顶栏：`header.tsx`、`public-header.tsx`、`top-nav.tsx`、`system-brand.tsx` 补充 `data-slot` 锚点；主题 CSS 提供纸带底纹、虚线底边、印章式 Logo、导航悬停波浪下划线；滚动后的公共页头变成漂浮纸带。
- 侧边栏：复用现有 `sidebar-*` 槽位，在主题 CSS 中实现纸纹、虚线脊线、手写分组标题、荧光笔选中态和页脚四叶草水印，未改动侧边栏交互代码。
- 页面标题：`section-page-layout.tsx` 补充 `data-slot` 锚点，并新增可选 `journalLevel` 属性（输出 `data-journal-level`，未传时由 CSS 按 `calm` 处理）；标题使用衬线字体与手绘虚线底边。
- 公共站：`public-layout.tsx`、`footer.tsx` 补充槽位；页脚使用纸纹与虚线分隔。
- 登录/注册：`auth-layout.tsx` 增加桌面手帐封面（拍立得、和纸胶带、回形针、便签）与表单纸卡；所有装饰带 `aria-hidden` 且 `pointer-events: none`。分栏布局、封面与纸卡样式只在 `clover-journal` 下生效，其他预设保持原来的居中单列表单。
- 验证结果：
  - `bun run typecheck` 通过。
  - 改动文件 `oxlint` 无 error（全仓 lint 仍有与本次无关的历史 error，例如 `dynamic-ratio`、`active-tasks`、`ai-elements`）。
  - `bun run test` 相关 4 个文件 16/16 通过，包含新增的 `section-page-layout.test.tsx` 与 `auth-layout.test.tsx`。
  - Playwright 验收截图：`tmp/clover-journal/verify-home-light-2.png`、`verify-auth-light-3.png`、`verify-signup-light.png`、`verify-auth-mobile-light-2.png`、`verify-dashboard-light.png`、`verify-dashboard-dark.png`、`verify-dashboard-mobile-light.png`、`verify-channels-light.png`、`verify-channels-dark.png`、`verify-usage-logs-light.png`、`verify-pricing-light.png`、`verify-pricing-mobile-light.png`、`verify-footer-light.png`、`verify-sidebar-collapsed.png`、`verify-mobile-drawer.png`。
  - 侧边栏折叠 `208px → 60px` 正常，移动端抽屉可打开并可 Esc 关闭；375px 下仪表盘、定价页与登录页均无横向溢出。
  - 其他预设回归：`lucky-clover`、`default`、`anthropic` 下标题无手绘底边、页头无虚线/纸纹、侧边栏无手写字体，登录页保持居中单列。
  - 公共导航悬停样式验证：`text-decoration: underline wavy 1px`，颜色切换到 `--primary`。
- 已知遗留：
  - `journalLevel` 属性已就绪，但各页面尚未按第四节的 A/B/C 清单接入；留到 P4/P5 逐页处理。
  - 页脚手写落款与公共站植物角落插画未单独添加：页脚是分页/工具栏的 portal 容器，直接加装饰会挤占控件空间，留到 P4/P5 页面级装饰时一并评估。
  - 公共页头在滚动收缩状态下会截断导航文字（例如「模型广...」）；该行为在 `lucky-clover` 等既有预设中同样存在，属于上游既有表现，本次未改动。

### P3 基础组件

- [x] **P3-1 表单与操作控件**
  - 文件：`web/src/components/ui/card.tsx`、`button.tsx`、`tabs.tsx`、`badge.tsx`、`input.tsx`、`select.tsx`、`checkbox.tsx`、`radio-group.tsx`
  - 内容：卡片纸面、按钮叶片/墨章、叶片标签页、印章徽章、纸面输入框。
  - 验收：所有变体与尺寸在浅色/暗色下可读，焦点环清晰，禁用态可辨。

- [x] **P3-2 列表与数据组件**
  - 文件：`web/src/components/ui/table.tsx`、`web/src/components/data-table/`、`pagination.tsx`、`empty.tsx`、`spinner.tsx`、`skeleton.tsx`
  - 内容：纸面表格、虚线分隔、便签式表头、纸条骨架、手绘空状态。
  - 验收：密集表格不出现旋转或贴纸；排序、分页、选择、虚拟滚动行为不变。

- [x] **P3-3 弹层与反馈组件**
  - 文件：`web/src/components/ui/dialog.tsx`、`sheet.tsx`、`drawer.tsx`、`popover.tsx`、`dropdown-menu.tsx`、`tooltip.tsx`、`sonner.tsx`、`web/src/components/confirm-dialog.tsx`
  - 内容：纸张弹层、顶部胶带、贴纸关闭按钮、便签 toast。
  - 验收：焦点陷阱、Esc 关闭、滚动锁定、移动端安全区行为不变。

- [x] **P3-4 图表主题**
  - 文件：`web/src/components/ui/chart.tsx`、`web/src/features/dashboard/` 图表组件
  - 内容：植物色板、点线网格、圆角柱、便签式 tooltip、暗色图表令牌。
  - 验收：图表在浅色/暗色下对比度足够，图例与 tooltip 可读。

### P3 实施记录

- 落地方式：P3 以 `theme-presets.css` 的 `clover-journal` 作用域覆盖为主，复用既有 `data-slot` / `data-variant` 锚点，不改 UI 基础组件的结构与交互。组件侧只新增了少量稳定锚点与图表色板参数：
  - `web/src/components/ui/chart.tsx`：为 tooltip 与 legend 增加 `data-slot='chart-tooltip'`、`data-slot='chart-legend'`。
  - `web/src/components/data-table/core/pagination.tsx`：桌面/紧凑分页容器增加 `data-slot='data-table-pagination'`，让当前页贴纸样式覆盖 data-table 自己的分页按钮。
  - `web/src/features/dashboard/lib/charts.ts`：新增 `getJournalChartColors()` 与植物色板常量；`processChartData` / `processUserChartData` 接受可选 `colorPalette`，仅在传入时启用圆角柱与便签 tooltip，默认行为不变。
  - `model-charts.tsx`、`consumption-distribution-chart.tsx`、`user-charts.tsx`、`flow-charts.tsx`：仅在 `preset === 'clover-journal'` 时传入按浅/暗主题选择的色板。
- P3-1：卡片标题改用衬线、页脚虚线接缝；主按钮/危险按钮加纸纹，ghost 悬停为荧光笔高亮，link 悬停为波浪下划线；叶片形 Tabs（选中态纸片 + 虚线边，line 变体为虚线手绘下划线）；印章式 Badge（不规则角、outline 虚线、表格内保持水平）；输入框/文本域/Select/原生 Select 为纸面内衬 + 缝线底边，禁用态回退到 muted 纸面；Checkbox 为小圆角墨章，Radio 保持圆形纸面。
- P3-2：表头纸带、行分隔虚线、悬停/选中绿色荧光笔；分页当前页为纸片贴纸；Empty 保留纸纹与衬线标题，medium/immersive 级别才显示四叶草水印与手写说明；Skeleton 为纸条质感；Spinner 使用四叶草绿。
- P3-3：弹层遮罩改为暖墨色；Dialog/AlertDialog/Sheet 顶部和纸胶带、贴纸式关闭按钮；Popover/Dropdown/Select 为纸张面板与虚线分隔；Tooltip 反转为墨色便签（暗色下为奶油便签）；Sonner toast 为虚线描边纸片。
- P3-4：VChart 仪表盘图表使用浅色 `#2a7449 / #60ad64 / #d7a94f / #4e939a / #bc4c31` 与暗色 `#69ba7c / #7aa761 / #ebbc63 / #67abb3 / #e58870` 植物色板；柱图为圆角，tooltip 为奶油便签；Recharts `chart.tsx` 增加纸张 tooltip 与点线网格样式。
- 验证结果：
  - `cd web && bun run typecheck` 通过。
  - `bunx oxlint -c .oxlintrc.json` 覆盖全部改动 TS/TSX 文件，无 error。
  - `bun run test src/features/dashboard/lib/__tests__/charts.test.ts src/components/data-table/core/__tests__/pagination.test.tsx`：2 个文件 16/16 通过（新增 4 个色板回归用例）。
  - `bun run test src/components/ui`：5 个文件 23/23 通过。
  - 其他预设回归：切到 `default` 后按钮/输入/表头/Empty 均无手帐背景图与虚线输入边，`data-theme-preset` 属性被移除；再切回 `lucky-clover` 恢复原状。
- 截图（`tmp/clover-journal/`，不提交）：
  - 表单与操作：`p3-signin-light.png`、`p3-signin-dark.png`、`p3-filter-dialog-light.png`、`p3-radio-light.png`、`p3-users-light.png`、`p3-users-dark.png`、`p3-users-selected-light.png`。
  - 列表与数据：`p3-pricing-table-light.png`、`p3-channels-table-light.png`、`p3-pagination-dark-clean.png`、`p3-empty-medium-light.png`、`p3-empty-medium-dark-2.png`、`p3-skeleton-spinner-light.png`。
  - 弹层与反馈：`p3-channel-dialog-light.png`、`p3-confirm-dialog-light.png`、`p3-confirm-dialog-dark.png`、`p3-popover-dark.png`、`p3-dropdown-dark.png`、`p3-select-dark.png`、`p3-tooltip-dark.png`、`p3-toast-preview-light-2.png`。
  - 图表：`p3-dashboard-model-chart-light.png`、`p3-dashboard-chart-dark.png`、`p3-dashboard-chart-tooltip-light.png`、`p3-dashboard-chart-tooltip-dark.png`、`p3-dashboard-area-chart-light.png`、`p3-dashboard-pie-dark.png`、`p3-dashboard-rank-bar-dark.png`。
  - 预设隔离：`p3-regression-default-light.png`。
- 已知说明：
  - 本地数据库没有真实用量数据，图表截图通过 Playwright 路由拦截注入当天假数据完成，未写入数据库。
  - Toast 没有可安全触发的真实业务入口（会改数据），因此用真实 Sonner DOM 结构做视觉预览，并核对了虚线边框、纸片圆角与纸纹背景均已命中。
  - VChart 维度 tooltip 在图表底部边缘会被图表卡片/图例裁切，这是既有 VChart 定位行为；mark tooltip 与暗色奶油便签均完整可读。
  - Flow Sankey 的节点标签仍沿用上游固定的深灰填充，夜间手帐下的对比度统一校准留到 P6-1。
  - `journalLevel` 逐页接入仍留到 P4/P5。

### P4 核心页面

- [x] **P4-1 登录/注册/找回密码/OTP**
  - 文件：`web/src/features/auth/**`
  - 内容：手帐封面、表单纸页、植物装饰、手写欢迎语。
  - 验收：所有认证流程、错误提示、OAuth、Passkey、TOTP 行为不变。

- [x] **P4-2 首页**
  - 文件：`web/src/features/home/**`
  - 内容：手帐拼贴首屏、相纸截图、胶带分区、手写批注。
  - 验收：CTA、登录入口、公告、数据展示正常；移动端首屏不溢出。

- [x] **P4-3 定价/模型广场/模型详情**
  - 文件：`web/src/features/pricing/**`
  - 内容：纸质模型卡片、索引式筛选栏、印章价格标签、胶带分组。
  - 验收：筛选、搜索、排序、视图切换、模型详情跳转正常。

- [x] **P4-4 仪表盘概览**
  - 文件：`web/src/features/dashboard/components/overview/**`
  - 内容：手帐式概览卡片、便签公告、植物装饰、手绘分隔线。
  - 验收：引导步骤、API 信息、公告、额度、性能健康面板功能不变。

- [x] **P4-5 仪表盘模型分析（对齐参考图）**
  - 文件：`web/src/features/dashboard/components/models/**`
  - 内容：统计纸卡、性能健康卡、模型成功率列表、纸张图表。
  - 验收：筛选、图表切换、模型用量明细、分页与导出行为不变。

- [x] **P4-6 钱包/订阅/兑换**
  - 文件：`web/src/features/wallet/**`、`web/src/features/subscriptions/**`、`web/src/features/redemption-codes/**`
  - 内容：存折式余额卡、手帐账单、印章状态标签。
  - 验收：充值、兑换、订阅、账单分页与状态展示正常。

### P4 实施记录（P4-1～P4-3）

- 落地方式：页面逻辑保持不变，只新增稳定的 `data-slot` / `data-journal-level` 锚点并复用 P1 装饰组件；视觉规则继续集中在 `web/src/styles/theme-presets.css`，全部限定在 `[data-theme-preset='clover-journal']` 下。其他预设不受影响。
- P4-1 认证页：
  - `auth-layout.tsx` 在表单纸卡内加入手写欢迎语 `A little luck for your journey`，在移动端堆叠布局补充四叶草角落装饰，并在纸卡右下角加入小型枝叶；装饰层继续 `aria-hidden` + `pointer-events:none`。
  - 登录、注册、找回密码、OTP、重置密码共用同一 `AuthLayout`，表单结构与提交逻辑未改动。
- P4-2 首页：
  - `home/index.tsx` 默认首页容器接入 `data-journal-level='immersive'`，自定义首页（URL/HTML/Markdown）保持原样。
  - `hero.tsx`：蓝色光晕改为通过 `--home-hero-aura` 切换为植物水彩；网格改为点线纸格；标题渐变改为深林绿墨色；补充和纸胶带、手写批注与四叶草涂鸦；终端演示包进相纸框，下方增加手写说明。
  - `stats.tsx`、`features.tsx`、`how-it-works.tsx`、`cta.tsx`：统计条、Bento 单元、步骤图标与结尾 CTA 增加纸张、虚线、胶带锚点，标题保留原有语义结构。
- P4-3 定价/模型广场/模型详情：
  - `pricing/index.tsx` 接入 `data-journal-level='immersive'`；隐藏蓝色网格光晕，改用植物水彩晕染；标题增加衬线字体与手绘虚线底边，搜索条为纸面胶囊。
  - `pricing-sidebar.tsx` 成为活页索引纸：桌面端在左侧绘制打孔，移动端筛选抽屉通过 `embedded` 属性复用同一组件但不重复叠加纸张。
  - `pricing-toolbar.tsx` 为纸带工具条，顶部贴和纸胶带；`model-card.tsx` 的卡片增加胶带与虚线描边，价格数字使用印章标签；`empty-state.tsx` 增加纸张、手写标题与四叶草涂鸦。
  - `model-details.tsx` 的内容根节点与独立详情页接入沉浸级别，独立页使用纸张卡片包裹；详情抽屉沿用 P3 的纸张弹层。
- i18n：新增两条装饰文案 `A little luck for your journey`、`Better models start here`，已同步 en / zh / zh-TW / fr / ru / ja / vi 七个语言包。
- 新增测试：
  - `web/src/features/auth/__tests__/auth-layout.test.tsx`：沉浸级别、移动端植物层、手写欢迎语的可访问性。
  - `web/src/features/home/__tests__/home-journal.test.tsx`：沉浸级别、CTA 链接、相纸框、装饰可访问性、统计与分区锚点。
  - `web/src/features/pricing/__tests__/pricing-journal.test.tsx`：定价页沉浸级别、搜索/筛选/空状态与手写装饰。
  - `model-cards.test.tsx`、`pricing-controls.test.tsx`：模型卡印章价格、筛选栏与工具条锚点。
- 验证结果：
  - `cd web && bun run typecheck` 通过。
  - 改动文件 `oxlint` 无新增 error。`hero-terminal-demo.tsx` 仍有上游既有的 `react/button-has-type` 与 `react/no-array-index-key` error，本次只增加了一个 `data-slot`，未顺手修改。
  - `bun run test src/features/auth src/features/pricing src/features/home`：23 个测试文件、350 个用例全部通过。
  - Playwright 交互验收：搜索 `gemini-3.5` 后模型卡 13 → 2，清除后恢复 13；筛选芯片可将结果缩至 0，重置后恢复；排序菜单 3 项，可切换到「价格：从低到高」；表格视图渲染 14 行后切回卡片视图；点击「详情」打开模型详情抽屉，Esc 可关闭；`/pricing/gemini-3-flash-preview` 独立详情页正常。
  - 宽度检查：认证页、首页、定价页在 1440px 与 375px 下横向溢出均为 0。
  - 其他预设回归（`lucky-clover`）：认证页欢迎语容器 `display:none`，表单卡顶部间距保持 32px；首页装饰 `display:none`、终端仍居中、CTA 内边距为 0；定价页筛选栏为实线边框、无打孔伪元素，搜索框与模型卡保持原有圆角与描边。
- 截图（`tmp/clover-journal/`，不提交）：
  - 认证：`p4-signin-light.png`、`p4-signin-dark.png`、`p4-signin-mobile-light.png`、`p4-signin-mobile-dark.png`、`p4-signup-light.png`、`p4-forgot-password-light.png`。
  - 首页：`p4-home-light.png`、`p4-home-dark.png`、`p4-home-mobile-light.png`、`p4-home-mobile-dark.png`、`p4-home-stats-light.png`、`p4-home-features-light.png`、`p4-home-how-it-works-light.png`。
  - 定价：`p4-pricing-light.png`、`p4-pricing-dark.png`、`p4-pricing-mobile-light.png`、`p4-pricing-mobile-dark.png`、`p4-pricing-details-drawer-light.png`、`p4-pricing-details-page-light.png`。
  - 预设回归：`p4-regression-auth-lucky-clover.png`、`p4-regression-home-lucky-clover.png`、`p4-regression-pricing-lucky-clover.png`。
- 已知说明：
  - 定价页模型详情目前通过抽屉打开，独立 `/pricing/$modelId` 路由仅用于直接访问；两者的手帐样式均已接入。
  - 本地后端未启动时 `/api/status` 返回 504，首页会弹出错误 toast 且定价路由因 fail-closed 跳回首页；本次已按仓库约定启动 Docker Desktop 与 `docker-compose.local.yml` 后完成验收。
  - `journalLevel` 逐页接入仍只覆盖 P4-1～P4-3，其余页面留到 P5。

### P4 实施记录（P4-4～P4-6）

- 落地方式：页面逻辑与数据请求保持不变，只补充稳定的 `data-slot` / `data-journal-level` 锚点并复用 P1 装饰组件；视觉规则集中追加在 `web/src/styles/theme-presets.css` 的 P4 段落，全部限定在 `[data-theme-preset='clover-journal']` 下。其他预设不受影响。
- P4-4 仪表盘概览：
  - `overview-dashboard.tsx`：`SectionPageLayout` 接入 `journalLevel='immersive'`，内容根节点新增 `data-slot='dashboard-overview'`。
  - `summary-cards.tsx`：概览卡成为夹着和纸胶带、回形针的纸质汇总页；三个统计格改为虚线纸片，余额栏为可撕下的存根（虚线接缝 + 纸纹），标题右侧增加手写批注 `Steady progress, keep going!`。
  - `announcements-panel.tsx`：面板标题右侧增加便签 `Fresh notes`，公告条目增加 `data-slot='overview-announcement'`，悬停为荧光笔；点击仍打开原有详情弹窗。
  - `performance-health-panel.tsx`：卡片纸面化，指标格改为缝线纸片，表头改为虚线接缝。
  - `panel-wrapper.tsx`：新增 `overview-panel`、`overview-panel-header`、`overview-panel-title` 锚点，API 信息 / 公告 / FAQ / 运行时间共用纸张面板与角落四叶草水印。
  - 装饰文案 `Steady progress, keep going!`、`Fresh notes` 已同步 7 个语言包。
- P4-5 仪表盘模型分析：
  - `dashboard/index.tsx`：非概览分区接入 `journalLevel='immersive'`，工具栏新增 `data-slot='dashboard-section-toolbar'`。
  - `log-stat-cards.tsx`：五项 KPI 合并为一张带胶带与回形针的统计纸卡，每个指标格新增 `data-slot='dashboard-stat-cell'`。
  - `performance-overview.tsx`：性能健康行改为纸质卡片 + 金色胶带；模型成功率徽章改为透明下划线索引列表，仍保持水平与等宽数字。
  - `model-usage-table.tsx`、`model-charts.tsx`、`consumption-distribution-chart.tsx`：分别新增 `dashboard-usage-table` / `dashboard-chart-card` 锚点，表格与图表卡片纸面化，图表头部虚线接缝。
  - 图表继续复用 P3-4 的 `getJournalChartColors()` 与 `colorPalette` 参数，未复制新色板。
- P4-6 钱包 / 订阅 / 兑换：
  - `wallet/index.tsx` 接入 `journalLevel='immersive'`；`wallet-stats-card.tsx` 成为存折式余额卡（左侧打孔装订、虚线分栏、胶带与回形针），四个格子新增 `data-slot='wallet-stat'`。
  - `recharge-form-card.tsx`、`subscription-plans-card.tsx` 增加 `wallet-recharge-card` / `wallet-plans-card` 锚点，表头改为收据式虚线接缝，充值卡顶部贴纸胶带。
  - `billing-history-dialog.tsx`：账单列表与每条记录新增 `billing-history-list` / `billing-record` 锚点，记录纸成为虚线纸片收据。
  - `subscriptions/index.tsx` 接入 `journalLevel='immersive'`；`user-subscriptions-table.tsx` 的筛选区、纸张表格与分页保留原行为，新增 `user-subscriptions-table` / `user-subscriptions-table-scroll` 锚点，表头纸带、虚线行、悬停荧光。
  - `redemption-codes/index.tsx` 按 B 级接入 `journalLevel='medium'`，未添加贴纸、旋转或手写批注。
  - `status-badge.tsx` 增加稳定的 `data-type` 属性；主题 CSS 把 `data-type='badge'` 的状态标签渲染为印章式纸片（虚线边、不规则角、纸面填充），表格内保持水平。
- 新增测试：
  - `web/src/features/dashboard/components/overview/__tests__/journal-overview.test.tsx`：沉浸级别、概览/余额锚点、4 个装饰元素的 `aria-hidden` 与 `pointer-events:none`、公告便签可点击打开详情。
  - `web/src/features/dashboard/components/models/__tests__/journal-models.test.tsx`：五项 KPI 与装饰可访问性、性能指标与模型成功率可读性、用量明细表锚点与数据。
  - `web/src/features/wallet/__tests__/journal-wallet.test.tsx`：存折四项指标与装饰可访问性、账单收据锚点与印章状态。
  - `web/src/features/subscriptions/components/__tests__/journal-subscriptions.test.tsx`：订阅页沉浸级别、套餐表与用户订阅表的纸张锚点、状态印章 `data-type` 与用量百分比。
- 验证结果：
  - `cd web && bun run typecheck` 通过（`tsgo -b` 无输出）。
  - `bunx oxlint -c .oxlintrc.json` 覆盖本轮 22 个 TS/TSX 文件：20 个无 error；`user-subscriptions-table.tsx` 仅有 2 个历史 `no-nested-ternary` error（`git show HEAD` 中已存在，且不在本次改动行内），按「不顺手修无关历史问题」保留。
  - `bun run test src/features/dashboard/components/overview src/features/dashboard/components/models src/features/wallet src/features/subscriptions src/features/redemption-codes src/components/clover-journal`：15 个测试文件、67 个用例全部通过。
  - Playwright 实测（路由拦截假数据，未写数据库、未使用真实凭据）：
    - 仪表盘概览：`main[data-journal-level='immersive']`；公告条目可点击并打开详情；设置引导、额度、API 信息、性能健康、运行时间面板正常。
    - 模型分析：筛选与偏好设置弹窗可打开并由 Esc 关闭；消耗分布从柱状图切换到面积图成功；模型用量明细仍可搜索/排序；`dashboard-stat-strip` 显示 5 个指标格。
    - 钱包 / 订阅：钱包与订阅页均为 `immersive`；钱包存折显示 4 项指标；账单弹窗显示 3 条收据、状态印章与「显示第 1-3 条，共 3」分页文案，Esc 可关闭；切换到「用户订阅」后显示 2 行记录与 2 个印章状态。
    - 兑换码：`main[data-journal-level='medium']`，3 行数据与 8 个印章标签正常，无沉浸级装饰。
    - 预设回归：`lucky-clover` 下概览、模型分析、钱包均保持原有卡片、标题、表格与按钮样式，无纸张胶带、回形针、手写批注、打孔或印章边。
- 截图（`tmp/clover-journal/`，不提交）：
  - 仪表盘概览：`p4-overview-desktop-light.png`、`p4-overview-desktop-dark.png`、`p4-overview-mobile-light.png`、`p4-overview-mobile-dark.png`。
  - 模型分析：`p4-models-desktop-light.png`、`p4-models-desktop-dark.png`、`p4-models-mobile-light.png`、`p4-models-mobile-dark.png`、`p4-models-charts-desktop-light.png`、`p4-models-charts-desktop-dark.png`。
  - 钱包：`p4-wallet-desktop-light.png`、`p4-wallet-desktop-dark.png`、`p4-wallet-mobile-light.png`、`p4-wallet-mobile-dark.png`、`p4-wallet-billing-light.png`。
  - 订阅：`p4-subscriptions-desktop-light.png`、`p4-subscriptions-desktop-dark.png`、`p4-subscriptions-mobile-light.png`、`p4-subscriptions-mobile-dark.png`、`p4-subscriptions-instances-light.png`。
  - 兑换码：`p4-redemptions-desktop-light.png`、`p4-redemptions-desktop-dark.png`、`p4-redemptions-mobile-light.png`、`p4-redemptions-mobile-dark.png`。
  - 预设回归：`p4-regression-overview-lucky-clover.png`、`p4-regression-models-lucky-clover.png`、`p4-regression-wallet-lucky-clover.png`。
- 已知说明：
  - 仪表盘模型分析页本身没有导出操作；「分页与导出行为不变」中的导出项本轮无对应入口，分页、搜索与排序已由既有测试覆盖。
  - 截图中的支付金额计算接口未纳入本次路由拦截，`待支付金额` 显示为 `-`；这是假数据缺口，充值计算逻辑未改动。
  - 兑换码状态标签在控制台出现 `Unused` / `Used` 缺少翻译的提示，属既有 i18n 缺口，与本次主题改造无关。
  - `journalLevel` 已覆盖 P4 全部 A/B 级页面；其余页面仍留到 P5 接入。

### P5 其余页面

- [x] **P5-1 B 级页面批量适配**
  - 文件：`web/src/features/keys/`、`usage-logs/`、`active-tasks/`、`recent-calls/`、`profile/`、`invite-codes/`、`security/`、`playground/`、`chat/`、`task-plugins/`、`donations/`
  - 内容：套用 medium 级别的纸感、配色与少量装饰。
  - 验收：各页面主要操作路径与表格行为不变。

- [x] **P5-2 C 级管理页面批量适配**
  - 文件：`web/src/features/channels/`、`users/`、`models/`、`system-settings/`、`dynamic-ratio/`、`blackroom/`、`fingerprints/`、`rate-limit/`、`system-info/`、`system-update/`
  - 内容：只应用 calm 级别令牌与纸感，不加贴纸、旋转、手写批注。
  - 验收：密集表格可读性不下降；表单、权限、批量操作行为不变。

- [x] **P5-3 错误页与空状态**
  - 文件：`web/src/features/errors/**`、`web/src/components/empty-state.tsx`、`error-state.tsx`、`loading-state.tsx`
  - 内容：手绘涂鸦、便签说明、纸片按钮。
  - 验收：错误码、返回操作、重试行为不变。

### P5 实施记录

- 落地方式：本轮只新增页面级别锚点、medium 级装饰与模型健康页配色覆写；业务逻辑、数据请求、权限、路由语义均未改动。所有样式继续限定在 `[data-theme-preset='clover-journal']` 下。
- P5-1 B 级页面：
  - `keys`、`usage-logs`（含 `audit`）、`active-tasks`、`recent-calls`、`invite-codes`、`security`、`task-plugins`、`donations` 的 `SectionPageLayout` 接入 `journalLevel='medium'`。
  - `profile` 的自定义 `Main`、`playground` 根容器、`model-health` 小时页根容器、`/model-health` 公开页内容容器、`chat/$chatId` 的加载/错误/回退分支与 iframe 包裹层均标记 `data-journal-level='medium'`。
  - `theme-presets.css` 新增 P5 medium 段落：在 `section-page-header` 顶部内边距内绘制一条 4.5rem 绿色和纸胶带，`pointer-events: none`，不覆盖标题、操作区或数据；calm 页面不命中。
  - `model-health/hourly-page.tsx`：标题、查询按钮、八张统计卡和两个图表面板增加稳定锚点；内联渐变改为 `var(--model-health-stat-bg, 原渐变)` / `var(--model-health-query-bg, 原渐变)`，主题内替换为纸张色，其他预设回退原渐变。统计卡白色光斑在主题内隐藏，图表面板补纸张圆角与纸纹。
  - `model-health/components/`：公开页统计卡与模型卡增加纸张锚点；时间线色条增加 `data-journal-slot='model-health-timeline-bar'` 与 `data-status`，主题内改用 `--chart-2/3/5` 植物色板。
  - 兑换码页面保持 P4-6 的 medium 接入，仅回归：`level=medium`、3 行记录、8 个印章标签。
  - `/console/log`、`/console/topup` 分别重定向到 `/usage-logs` 与 `/wallet`，没有独立页面需要适配。
- P5-2 C 级管理页面：
  - 未新增 feature 代码。未标记页面继续按 calm 默认渲染；本轮用 Playwright 逐页确认 `mainLevel=null`、页头伪元素 `content='none'`，且数据区无可见 `.journal-tape`、`.journal-doodle`、`.journal-hand`、`.journal-sticky-note`。
  - 覆盖 `channels`、`users`、`models`、`system-settings/site`、`dynamic-ratio`、`blackroom`、`fingerprints`、`rate-limit`、`system-info`、`system-update`（见下方说明）与 `setup`。
  - 交互回归：渠道创建弹层与批量开关、用户添加弹层与行选择、模型添加弹层、动态倍率新建规则弹层、小黑屋配置弹层、指纹全部记录标签切换均正常；全部弹层可用 Esc 关闭。
- P5-3 错误页与共享状态：
  - 401/403/404/500/503 五个错误页根节点增加 `data-slot='error-page' data-journal-level='medium'`，说明段落增加 `data-slot='error-note'`。
  - 主题 CSS 为错误码增加高亮下划线与衬线墨色，为说明增加带和纸胶带的纸张便签，为页面增加右上角四叶草水印；文字保持水平，仅装饰胶带轻微旋转。
  - `LoadingState` 改用共享 `Spinner`，复用 P3 的绿色 spinner 与 `role='status'`；`EmptyState` / `ErrorState` 未重写，P3 的纸张、手写说明与四叶草水印仍按 medium/immersive 级别生效，calm 页面不显示。
- 新增/扩展测试（13 个新用例）：
  - `web/src/features/keys/__tests__/journal-level.test.tsx`：medium 级别、标题、创建按钮与表格占位。
  - `web/src/features/profile/__tests__/journal-level.test.tsx`：自定义 `Main` 的 medium 级别与卡片保留。
  - `web/src/features/donations/__tests__/journal-layout.test.tsx`：捐献导航、正文与 medium 级别。
  - `web/src/features/errors/__tests__/error-pages.test.tsx`：五个错误码、medium 纸张便签、404 返回/返回主页、429 文案。
  - `web/src/components/__tests__/state-components.test.tsx`：EmptyState 内容、ErrorState 重试回调、LoadingState 的 `role='status'`。
  - `web/src/features/model-health/components/__tests__/health-timeline.test.tsx`：保留原有 10 个用例，新增时间线色条的 `data-status` 与无障碍名称断言。
  - `stat-card.test.tsx`：补充公开页统计卡纸张锚点断言。
- 验证结果：
  - `cd web && bun run typecheck`：`tsgo -b` 无输出，通过。
  - `bunx oxlint -c .oxlintrc.json` 覆盖本轮全部 TS/TSX 文件：新增/修改行无 error。仍存在的历史 error 均不在本次改动行内：`active-tasks/index.tsx` 3 个 `no-nested-ternary`、`recent-calls/index.tsx` 3 个 `no-nested-ternary` + 1 个 `exhaustive-deps`、`chat/$chatId.tsx` 既有 iframe 缺少 `sandbox`（`git diff` 已确认这些行未改动）。
  - `bun run test src/features/keys src/features/usage-logs src/features/profile src/features/donations src/features/errors src/features/model-health src/components/__tests__ src/components/layout/__tests__`：50 个文件、590 个用例，589 通过；`model-badge.test.tsx` 在全量并发下出现一次图标异步加载抖动，单独重跑该文件 183/183 通过，与本次改动无关。
  - 新增测试单独运行：5 个文件 12/12 通过；恢复并扩展后的 `health-timeline` + `stat-card` 2 个文件 14/14 通过。
  - Playwright 交互输出：keys/recent-calls/security/channels/users/models/dynamic-ratio/blackroom 弹层均可打开并由 Esc 关闭；task-plugins 插件市场 `aria-selected=true`；channels 批量开关 `false -> true`；users 首行选择 `true`；donations 活动选项显示 `Community keys`；fingerprints 全部记录 `aria-selected=true`；404 返回主页跳到 `/`；setup 下一步按钮可见且启用。
  - 其他预设回归（`lucky-clover`）：keys/渠道页头伪元素 `content='none'` 且无 `.journal-tape`；404 无四叶草水印、说明边框非虚线；模型健康小时页统计卡仍为原紫蓝渐变、标题仍为渐变文字。
- 截图（`tmp/clover-journal/`，不提交）：
  - B 级：`p5-keys-*`、`p5-usage-logs-*`、`p5-invite-codes-*`、`p5-active-tasks-*`、`p5-recent-calls-*`、`p5-task-plugins-*`、`p5-profile-*`、`p5-security-*`、`p5-donations-*`、`p5-playground-*`、`p5-model-health-hourly-*`、`p5-model-health-public-*`、`p5-chat-*`，每页含 `desktop/mobile × light/dark` 四张。
  - C 级：`p5-channels-*`、`p5-users-*`、`p5-models-*`、`p5-system-settings-*`、`p5-dynamic-ratio-*`、`p5-blackroom-*`、`p5-fingerprints-*`、`p5-rate-limit-*`、`p5-system-info-*`、`p5-system-update-*`、`p5-setup-*`。
  - 错误页：`p5-error-401-*`、`p5-error-403-*`、`p5-error-404-*`、`p5-error-500-*`、`p5-error-503-*`。
  - 预设回归：`p5-regression-keys-*-lucky-clover.png`、`p5-regression-error-404-*-lucky-clover.png`、`p5-regression-model-health-hourly-*-lucky-clover.png`、`p5-regression-channels-*-lucky-clover.png`。
- 已知说明：
  - `features/system-update/` 没有独立的 `/_authenticated/system-update` 路由，更新检查实际位于 `/system-settings/operations/update-checker`；本轮在该区块完成 calm 与“检查更新”按钮验证。
  - `/console/log` 与 `/console/topup` 是重定向路由，没有独立 UI。
  - 公开模型健康页在开发态 StrictMode 下首次查询可能显示 `canceled`，切换 7/15/30 天或等待刷新后正常加载；这是本次 Playwright 验收脚本中的开发态行为，未改动业务代码。
  - 小黑屋页的 `Auto ban` / `On` 在 zh 语言包缺少翻译，属既有 i18n 缺口，未扩大处理范围。
  - Setup 页 375px 下 SQLite 徽标右侧略有裁切，属既有响应式表现，留到 P6-2 统一处理。
  - P3 的弹层顶部和纸胶带是全局共享弹层样式，经 portal 渲染后不受页面级别影响；C 级页面的弹窗仍会显示该胶带。本轮按“共享组件不重写”保留，若需严格 calm 可在 P6/P7 评估是否把级别传递到 portal。

### P6 暗色、移动端、可访问性

- [x] **P6-1 夜间手帐逐页校准**
  - 内容：关键页与共享组件的暗色对比度、纸影、装饰透明度。
  - 验收：暗色下无纯白刺眼、无装饰发光、图表与表格可读。

- [x] **P6-2 移动端适配**
  - 内容：侧边栏抽屉、顶栏、表格横向滚动、卡片堆叠、弹层安全区。
  - 验收：375px、768px、1024px 三档宽度无横向溢出。

- [x] **P6-3 可访问性**
  - 内容：正文对比度 ≥ 4.5:1、焦点可见、键盘顺序、装饰 `aria-hidden`、`prefers-reduced-motion`。
  - 验收：键盘可完成主要流程；读屏不朗读装饰；减少动效时无旋转/漂浮动画。

### P6 实施记录

- 落地方式：P6 以主题令牌校准、作用域内焦点/状态色规则和三处响应式布局修复为主，未改动数据请求、权限、路由与计费逻辑；新增样式继续限定在 `[data-theme-preset='clover-journal']` 下。
- P6-1 夜间手帐逐页校准：
  - 新增临时审计脚本 `tmp/clover-journal/.scripts/audit-p6.py`，覆盖 sign-in、home、pricing、dashboard 概览/模型分析、usage-logs、keys、channels、users、wallet、subscriptions、model-health、profile、system-settings、404 共 15 页 × 浅色/暗色。
  - 审计输出：全部页面 `violations=0`、`whites=0`、`glows=0`，即无低于 4.5:1 的可见正文、无纯白大色块、无白色发光阴影。
  - 令牌调整：浅色 `--muted-foreground` 由 `0.50` 收紧到 `0.46`，浅色 `--success` 由 `0.55` 收紧到 `0.52`；新增 `--journal-warning-ink`、`--journal-danger-ink`、`--journal-success-ink`、`--journal-info-ink`，深色模式单独取值。
  - 规则调整：`text-muted-foreground/*` 统一回到可读的 muted 墨色；`text-foreground/*` 设 80% 下限；中性状态徽章使用页面墨色；success/warning/destructive/info 的硬编码状态色映射到植物墨色；头像回退由随机亮色 HSL 改为四叶草纸印章；未选中标签与侧栏分组标题提升对比度；destructive 徽章与 alert 说明使用校准后的 danger ink。
  - 暗色装饰：纸胶带使用暗色 alpha 令牌，涂鸦透明度 0.42；审计未发现白色发光阴影，夜间纸影保持深色。
- P6-2 移动端适配：
  - `audit-p6.py mobile` 覆盖 15 页 × 375/768/1024：所有页面 `overflow=0`。仅剩一个被应用外壳裁切的离屏装饰 `div`（left 为负、页面无滚动），属装饰层误报。
  - `SectionPageLayout` 操作区新增 `data-slot='section-page-actions'`；journal 下限制 `max-width: 100%` 并允许内部换行。768px 渠道页头不再裁切“创建渠道”，开关组自动换行。
  - 渠道卡片网格由 `lg:grid-cols-3` 调整为 `lg:grid-cols-2 xl:grid-cols-3`；1024px 下卡片名称不再被压成“Op...”。
  - Setup 数据库卡片内容区改为 `min-w-0 flex-1`，状态徽章加 `shrink-0`；375px 下 SQLite 徽章完整显示。
  - 交互实测：375px 侧栏抽屉 `272×812`、页面溢出 0、Esc 关闭；keys 创建弹层动画结束后 `375×812`、页面溢出 0、Esc 关闭；768px 用户表格滚动容器 `1400 / 518`，页面溢出 0。
- P6-3 可访问性：
  - 装饰审计：15 页中所有 `.journal-tape`、`.journal-doodle`、`.journal-sticky-note`、`.journal-hand`、`.journal-paper-clip`、`.journal-grain` 均为 `aria-hidden='true'` 且 `pointer-events: none`，`bad=0`。
  - `prefers-reduced-motion: reduce` 下首页 landing/terminal 动画的 `animationName` 均为 `none`，活跃动画列表为空。
  - 键盘/焦点：sign-in 14 个 Tab 停靠点、keys 18 个停靠点全部有可见焦点指示；登录用户名/密码/提交按钮均为实心 2px 轮廓；分页下一页按钮、弹层自动聚焦的关闭按钮同样为实心 2px 轮廓；Esc 可关闭弹层。
  - 新增 journal 作用域 `:focus-visible` 轮廓规则（button/a/input/textarea/select 及常见 ARIA 控件），并为弹层关闭按钮补 `:focus` 轮廓，避免透明 ring 槽位导致焦点不可见。
- 新增/扩展测试：
  - `web/src/components/layout/__tests__/section-page-layout.test.tsx`：新增操作区 `data-slot` 锚点断言，保护窄屏换行入口。
  - `web/src/features/setup/__tests__/database-step.test.tsx`：SQLite 名称、说明、持久化提示可见，内容区保持 `min-w-0 flex-1`、徽章保持 `shrink-0`。
- 验证结果：
  - `cd web && bun run typecheck`：`tsgo -b` 无输出，通过。
  - `bunx oxlint -c .oxlintrc.json` 覆盖 P6 改动文件：无 error。
  - `bun run test src/components/layout/__tests__ src/features/setup src/features/channels src/components/__tests__ src/context/__tests__/theme-preferences.test.tsx`：31 个文件、319 个用例全部通过。
  - 其他预设回归（`lucky-clover`）：keys/渠道页头伪元素 `content='none'`、无 `.journal-tape`；404 无四叶草水印、便签边框非虚线；模型健康小时页统计卡仍为原紫蓝渐变、标题仍为渐变文字。
- 截图（`tmp/clover-journal/`，不提交）：
  - 暗色/浅色关键页：`p6-keys-light/dark.png`、`p6-usage-logs-light/dark.png`、`p6-wallet-light/dark.png`、`p6-home-light/dark.png`、`p6-model-health-light/dark.png`、`p6-error-404-light/dark.png`。
  - 移动端：`p6-channels-375.png`、`p6-channels-768-after.png`、`p6-channels-1024-after.png`、`p6-setup-375-after.png`。
  - 完整页面矩阵可复用 P5 的 `p5-<page>-<desktop|mobile>-<light|dark>.png`。
- 已知说明：
  - 对比度审计基于可见文本节点与有效背景色估算，纹理/渐变背景按基础背景色近似；结果为 0 可见违规，但仍建议在 P7 的人工截图复核中抽查。
  - 渠道卡片网格断点与 setup 卡片 flex 修复属于响应式布局修正，会影响其他预设的对应宽度；`lucky-clover` 回归仍通过。
  - P3 的 portal 弹层纸胶带在 C 级页面弹窗中仍会出现，本轮未改变共享弹层级别传递，留到 P7 评估。
  - P5 已知的 `model-badge` 全量并发抖动、小黑屋 `Auto ban`/`On` 翻译缺口、公开模型健康页开发态首次查询 `canceled` 仍未处理。

### P7 验证与收尾

- [x] **P7-1 静态检查与测试**
  - 命令：`cd web && bun run typecheck && bun run lint && bun run test`
  - 验收：类型检查、lint、受影响测试全部通过；新增测试覆盖主题切换与装饰组件。

- [x] **P7-2 生产构建**
  - 命令：`cd web && bun run build`
  - 验收：构建成功；产物体积无明显膨胀；字体与纹理资源按预期加载。

- [x] **P7-3 截图矩阵**
  - 内容：关键页 × 浅色/暗色 × 桌面/移动端，与 P0 概念图对比。
  - 验收：视觉方向一致；无布局错位、遮挡、溢出。

- [x] **P7-4 全站回归**
  - 内容：主题切换、语言切换、权限页、表格分页、弹窗、表单、图表。
  - 验收：无功能回归；其他 10 个预设保持原样。

- [x] **P7-5 默认预设确认**
  - 内容：由项目所有者决定是否把 `DEFAULT_THEME_CUSTOMIZATION.preset` 与 `web/index.html` 切到 `clover-journal`。
  - 验收：确认后更新默认值、`index.html` 与主题测试；未确认则保持 `lucky-clover`。

### P7 实施记录

- P7-1 静态检查与测试：
  - `cd web && bun run typecheck`：`tsgo -b` 无输出，通过。
  - 全量测试：`bunx vitest run --maxWorkers=2`（为降低内存占用限制 worker）→ **225 个测试文件、2421 个用例全部通过**，耗时 1372.98s。
  - 全仓 lint：`bun run lint` 返回 185 个 error、61 个 warning，全部为既有问题，分布在不相关的功能模块与 `web/default` 副本中；主题相关改动文件无 error。按项目所有者决定，本轮**记录为既有遗留，不顺手修复**。
  - 主题切换与装饰组件测试：`theme-preferences.test.tsx` + `decorations.test.tsx` 共 13 个用例通过。
- P7-2 生产构建：
  - `cd web && bun run build` 成功，`rsbuild` 19.0s 完成。
  - 产物：`dist` 约 69MB，入口 CSS 553KB；Caveat 字体四个子集约 210KB，Lora、Public Sans 与 KaTeX 字体均正常产出；未发现主题资源缺失或异常膨胀。
- P7-3 截图矩阵：
  - 生成 7 个关键页 × 浅色/暗色 × 桌面/移动端共 28 张截图，另补仪表盘模型分析图表近景 2 张；文件名已写入 7.3 表格。
  - 人工复核：登录页拍立得/表单纸卡、首页移动端堆叠、定价移动端筛选条、仪表盘概览卡片、模型分析图表、日志移动卡片、系统设置表单均无布局错位、遮挡或横向溢出；图表浅/暗色轴标签与柱图可读。
- P7-4 全站回归：
  - 主题隔离：`clover-journal` 概览页装饰可见、keys 页头 medium 胶带存在；`lucky-clover`、`default`、`anthropic` 的概览与 keys 页均无胶带、涂鸦、手写、便签泄漏。
  - 回归中发现并修复的真实问题：`default` 预设会移除 `data-theme-preset` 属性，原有隔离规则只匹配“存在属性且非 clover-journal”，导致手帐装饰在默认预设下可见。改为 `:is(...):not([data-theme-preset='clover-journal'] *)` 后，默认预设与其余预设均不再渲染装饰。
  - 语言切换：`/keys` 标题 `API 密钥` → `API Keys` → `API 密钥`，界面语言切换与回切正常。
  - 权限：普通用户访问 `/channels` 被重定向到 `/403`，管理端导航不显示。
  - 表格分页：日志页每页 10 条，键盘聚焦“下一页”并 Enter 后请求 `p=2`，行数 10 → 2，首行时间变化。
  - 弹层：keys 创建弹层打开并由 Esc 关闭。
  - 表单：个人资料通知阈值保存成功，出现“设置更新成功”toast。
  - 图表：模型分析页渲染 2 个 canvas、2 个图表面板。
- P7-5 默认预设确认：
  - 项目所有者决定保持 `lucky-clover`；`DEFAULT_THEME_CUSTOMIZATION.preset`、`web/index.html` 与主题测试均未改动。
- 已知说明：
  - 全仓 lint 的 185 个 error / 61 个 warning 为既有问题，按决定未修复；`web/default` 是工作区中的副本目录，也会被 lint 扫描。
  - P3 的 portal 弹层纸胶带在 C 级页面弹窗中仍会出现，未改变共享弹层级别传递。
  - P5 已知的 `model-badge` 全量并发抖动、小黑屋 `Auto ban`/`On` 翻译缺口、公开模型健康页开发态首次查询 `canceled` 仍未处理。
  - P7-4 的默认预设装饰隔离修复为 CSS-only，发生在全量测试之后；主题切换与装饰组件测试已重跑通过。
  - 截图与全站回归使用 Playwright 路由拦截假数据，未写入数据库、未使用真实凭据。

## 六、进度总览

| 阶段 | 任务数 | 状态 | 完成数 | 备注 |
| --- | ---: | --- | ---: | --- |
| P0 设计确认 | 2 | 已完成 | 2 | 概念图与定稿令牌已确认 |
| P1 主题基础 | 6 | 已完成 | 6 | 预设、令牌、字体、装饰组件、分级容器 |
| P2 应用外壳 | 5 | 已完成 | 5 | 顶栏、侧边栏、标题区、公共站、登录外壳 |
| P3 基础组件 | 4 | 已完成 | 4 | 表单控件、数据组件、弹层、图表主题 |
| P4 核心页面 | 6 | 已完成 | 6 | 认证页、首页、定价、仪表盘概览、模型分析、钱包/订阅/兑换 |
| P5 其余页面 | 3 | 已完成 | 3 | B 级 medium、C 级 calm 回归、错误页与共享状态 |
| P6 暗色/移动端/可访问性 | 3 | 已完成 | 3 | 对比度、三档宽度、键盘/焦点、reduced-motion |
| P7 验证与收尾 | 5 | 已完成 | 5 | 静态检查、构建、截图矩阵、全站回归、默认预设确认 |
| **合计** | **34** | **已完成** | **34** | |

## 七、验证基线

### 7.1 静态检查与测试

```bash
cd web
bun run typecheck
bun run lint
bun run test
```

### 7.2 构建

```bash
cd web
bun run build
```

### 7.3 截图矩阵

| 页面 | 浅色桌面 | 浅色移动 | 暗色桌面 | 暗色移动 |
| --- | --- | --- | --- | --- |
| 登录 | `p7-sign-in-desktop-light.png` | `p7-sign-in-mobile-light.png` | `p7-sign-in-desktop-dark.png` | `p7-sign-in-mobile-dark.png` |
| 首页 | `p7-home-desktop-light.png` | `p7-home-mobile-light.png` | `p7-home-desktop-dark.png` | `p7-home-mobile-dark.png` |
| 定价/模型广场 | `p7-pricing-desktop-light.png` | `p7-pricing-mobile-light.png` | `p7-pricing-desktop-dark.png` | `p7-pricing-mobile-dark.png` |
| 仪表盘概览 | `p7-dashboard-overview-desktop-light.png` | `p7-dashboard-overview-mobile-light.png` | `p7-dashboard-overview-desktop-dark.png` | `p7-dashboard-overview-mobile-dark.png` |
| 仪表盘模型分析 | `p7-dashboard-models-desktop-light.png` | `p7-dashboard-models-mobile-light.png` | `p7-dashboard-models-desktop-dark.png` | `p7-dashboard-models-mobile-dark.png` |
| 日志表格 | `p7-usage-logs-desktop-light.png` | `p7-usage-logs-mobile-light.png` | `p7-usage-logs-desktop-dark.png` | `p7-usage-logs-mobile-dark.png` |
| 系统设置 | `p7-system-settings-desktop-light.png` | `p7-system-settings-mobile-light.png` | `p7-system-settings-desktop-dark.png` | `p7-system-settings-mobile-dark.png` |

补充图表近景：`p7-dashboard-models-charts-light.png`、`p7-dashboard-models-charts-dark.png`。

### 7.4 可访问性检查

- 正文对比度 ≥ 4.5:1。
- 键盘可完成登录、导航、表格分页、弹窗关闭。
- 装饰元素全部 `aria-hidden` 且 `pointer-events: none`。
- `prefers-reduced-motion` 下关闭旋转、漂浮和纸片飞入动画。

## 八、风险与对策

| 风险 | 影响 | 对策 |
| --- | --- | --- |
| 上游合并冲突 | 布局与组件改动面大 | 预设隔离、附加式组件 API、主题 CSS 集中管理；冲突单独提交解决 |
| 装饰拖累可读性 | 密集表格与管理页难用 | A/B/C 分级；数据区禁用旋转、胶带和纹理 |
| 纹理与 SVG 性能 | 首屏变慢 | 单张纹理复用、避免逐卡片滤镜、装饰只在 A/B 级渲染 |
| 中文手写字体体积 | 包体增大、加载变慢 | 不引入全量字体，使用楷体回退；必要时再子集化 |
| i18n 翻译负担 | 七语言维护成本 | 装饰文案限定 8–12 条，其余用纯图形 |
| 暗色观感割裂 | 暗色用户体验下降 | 独立夜间手帐令牌，逐页校准 |
| 其他预设被污染 | 影响现有用户 | 所有样式限定在 `[data-theme-preset='clover-journal']` 下；切换回归测试 |

## 九、变更记录

| 日期 | 版本 | 变更 |
| --- | --- | --- |
| 2026-10-17 | v1.0 | 创建规划文档，锁定四项决策并拆分 34 项任务 |
| 2026-10-17 | v1.1 | P0-1 完成：生成登录、定价、仪表盘三张概念图，补充 P0 概念图结论 |
| 2026-10-17 | v1.2 | P0-2 完成：固化浅色/暗色令牌、字体、形状、装饰、组件与页面构图 |
| 2026-10-17 | v1.3 | P1 完成：预设注册、令牌、字体轴、装饰组件、分级容器与验证记录 |
| 2026-10-17 | v1.4 | P2 完成：顶栏、侧边栏、页面标题区、公共站与登录外壳；补齐其他预设的样式隔离与新增回归测试 |
| 2026-10-18 | v1.5 | P3 完成：表单与操作控件、列表与数据组件、弹层与反馈组件、图表植物色板；补充 P3 实施记录、测试与截图 |
| 2026-10-18 | v1.6 | P4-1～P4-3 完成：认证页手写欢迎语与移动端植物装饰、首页手帐拼贴/相纸/胶带分区、定价页活页索引/印章价格/纸质卡片；补充实施记录、交互验收与截图 |
| 2026-10-18 | v1.7 | P4-4～P4-6 完成：仪表盘概览纸张汇总页/便签公告/性能健康、模型分析统计纸卡/成功率索引/纸张图表、钱包存折余额卡/手帐账单/印章状态、订阅纸张表格与兑换码 B 级接入；补充实施记录、67 项测试、交互验收与截图 |
| 2026-10-19 | v1.8 | P5 完成：B 级 medium 页面批量接入与页头纸胶带、模型健康页纸张配色、C 级 calm 逐页回归、错误页涂鸦/便签/纸片按钮与共享 Loading 复用；补充 13 个新测试、交互验收、其他预设回归与截图路径 |
| 2026-10-19 | v1.9 | P6 完成：夜间手帐对比度/纸影/装饰透明度校准，375/768/1024 移动端溢出与抽屉/弹层/表格滚动修复，键盘焦点轮廓、装饰可访问性与 reduced-motion 验证；补充 P6 实施记录、319 项测试与截图 |
| 2026-10-19 | v2.0 | P7 完成：全量类型检查与 2421 项测试、生产构建、28 张关键页截图矩阵、主题/语言/权限/分页/弹层/表单/图表全站回归；修复 default 预设装饰泄漏；默认预设保持 `lucky-clover` |

## 十、下一步

1. 四叶草手帐主题改造已完成（34/34）；默认预设按项目所有者决定保持 `lucky-clover`，未改动 `DEFAULT_THEME_CUSTOMIZATION.preset` 与 `web/index.html`。
2. 可选后续清理：全仓既有 lint error/warning、C 级弹窗的 portal 纸胶带级别传递、P5 已知的 `model-badge` 抖动与小黑屋翻译缺口。
