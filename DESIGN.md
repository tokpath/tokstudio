---
version: alpha
name: TokenHub-Clearing
description: TokenHub 的视觉宪法。隐喻是清算所——公布牌价、扣住预授权、写下不可改的账。Light 是纸，Dark 是碳，语法同一套；品牌只留一枚可替换的钴蓝签核章。
colors:
  brand: "#2150D6"
  brand-press: "#183CA8"
  brand-soft: "#DCE6FB"
  brand-emphasis-dark: "#8AA4FF"
  on-brand: "#FFFFFF"
  paper: "#F4F1EA"
  paper-raised: "#FFFDF8"
  paper-ink: "#141414"
  paper-ink-secondary: "#4A453C"
  paper-ink-mute: "#7A7368"
  paper-hairline: "#D9D3C7"
  carbon: "#161513"
  carbon-raised: "#1F1D1A"
  carbon-ink: "#EDEAE4"
  carbon-ink-secondary: "#C4BDB2"
  carbon-ink-mute: "#8A8378"
  carbon-hairline: "#2A2723"
  success: "#1F7A4D"
  hold: "#B45309"
  danger: "#C23B22"
  degraded: "#9A6700"
typography:
  display:
    fontFamily: "Inter, Geist, system-ui, sans-serif"
    fontSize: 40px
    fontWeight: 600
    lineHeight: 1.15
    letterSpacing: -0.6px
  title:
    fontFamily: "Inter, Geist, system-ui, sans-serif"
    fontSize: 24px
    fontWeight: 600
    lineHeight: 1.25
    letterSpacing: -0.2px
  body:
    fontFamily: "Inter, Geist, system-ui, sans-serif"
    fontSize: 16px
    fontWeight: 400
    lineHeight: 1.5
  caption:
    fontFamily: "Inter, Geist, system-ui, sans-serif"
    fontSize: 13px
    fontWeight: 400
    lineHeight: 1.45
  button:
    fontFamily: "Inter, Geist, system-ui, sans-serif"
    fontSize: 14px
    fontWeight: 500
    lineHeight: 1.2
  eyebrow:
    fontFamily: "ui-monospace, 'JetBrains Mono', monospace"
    fontSize: 11px
    fontWeight: 500
    lineHeight: 1.2
    letterSpacing: 0.08em
    textTransform: uppercase
  mono:
    fontFamily: "ui-monospace, 'JetBrains Mono', monospace"
    fontSize: 13px
    fontWeight: 400
    lineHeight: 1.5
rounded:
  stamp: 6px
  card: 6px
  control: 4px
spacing:
  xs: 4px
  sm: 8px
  md: 16px
  lg: 24px
  xl: 32px
  section: 64px
---

# TokenHub Clearing

这份文件是设计事实源。四个入口（公共站、用户控制台、渠道控制台、平台管理台）和 OEM 换皮都读这里，不要再从 M0 状态页的石板青推断品牌。

口令：**一张已发布的价目单，外加一枚签核章。**

## 1. 气质

TokenHub 是清算所，不是 AI 聊天玩具，也不是消费品牌。

第一竞争力来自需求文档：可解释路由、可复算的账、渠道/OEM 隔离。页面要看起来像凭证和牌价，而不是仪表盘皮肤。

- 公共站、文档、充值：让开发者和渠道合伙人觉得正规。
- 控制台：让财务、值班、代理商把表看清楚。
- OEM：只换章的颜色和 Logo，不换语法。

## 2. Light / Dark 是同一套语言

现代产品默认有两套模式。TokenHub **四个入口都要同时支持 Light 和 Dark**，不要把公共站锁浅色、控制台锁深色。

| 模式 | 表面名 | 画布 | 抬起一层 | 正文 | 细线 |
| --- | --- | --- | --- | --- | --- |
| Light | 纸 Paper | `{colors.paper}` `#F4F1EA` | `{colors.paper-raised}` `#FFFDF8` | `{colors.paper-ink}` `#141414` | `{colors.paper-hairline}` `#D9D3C7` |
| Dark | 碳 Carbon | `{colors.carbon}` `#161513` | `{colors.carbon-raised}` `#1F1D1A` | `{colors.carbon-ink}` `#EDEAE4` | `{colors.carbon-hairline}` `#2A2723` |

两套表面是同一张纸的正反面：略暖，中性黑，不偏 Tailwind 石板蓝，也不偏 Linear 发紫的纯黑。

### 2.1 用户怎么选

提供三档，不要只做开/关：

1. **浅色**：强制纸。
2. **深色**：强制碳。
3. **跟随系统**：读 `prefers-color-scheme`。

默认是 **跟随系统**。选择写入本地（`tokenhub-theme`），四个入口共用。未选择前不要闪白/闪黑：先按系统色画第一帧。

OEM 的 `theme_json` 可以建议默认档，但不能关掉其中一档。浅色/深色是可读性，不是品牌装饰。

### 2.2 实现约定

- 用 class 策略：`html.light` / `html.dark`。
- 颜色只走 CSS 变量，组件里不要写 `bg-slate-950`、`text-cyan-300` 这种具体色。
- Tailwind 映射语义名：`bg-canvas`、`text-ink`、`border-hairline`、`bg-brand`。
- 禁止用 `dark:bg-slate-*` 覆盖语义 token。模式切换必须只换变量，不换结构。

### 2.3 品牌色在两套模式下怎么用

签核章本身不换色：实心主按钮始终是 `{colors.brand}` `#2150D6`，字 `{colors.on-brand}` 白。

| Token | Light | Dark | 用途 |
| --- | --- | --- | --- |
| `--brand` | `#2150D6` | `#2150D6` | 实心主按钮、当前导航条 |
| `--brand-press` | `#183CA8` | `#183CA8` | 按下 |
| `--brand-soft` | `#DCE6FB` | `#243056` | 淡章底（选中行、已发布标签） |
| `--brand-emphasis` | `#2150D6` | `#8AA4FF` | 链字、焦点环（保证深色对比） |

OEM 替换 `--brand` / `--brand-press` / `--brand-soft` / `--brand-emphasis`。纸/碳表面和语义色不给 OEM 改。

## 3. 颜色角色

### 品牌（可被 OEM 替换）

- **钴蓝章** `{colors.brand}`：每个可见区块最多一枚实心主按钮。
- 不要把钴蓝铺成大面积英雄底、渐变网格或光晕。

### 语义（不可被品牌色吞掉）

| 状态 | Token | Hex | 例子 |
| --- | --- | --- | --- |
| 成立 / 已结算 | `--success` | `#1F7A4D` | settled、paid、released |
| 扣住 / 冻结 | `--hold` | `#B45309` | preauthorized、frozen、pending |
| 冲正 / 失败 | `--danger` | `#C23B22` | reversed、refunded、denied |
| 降级 | `--degraded` | `#9A6700` | degraded、maintenance |
| 待对账 | `--hold` + 文案，或淡章 | — | `usage_pending_reconciliation` |

待对账是产品态，不要做成灰字「未知」。它要像还没盖章的回单。

颜色不表示「我们是 AI」，颜色表示「这一笔成立了」。

## 4. 字体

- 界面：Inter 或 Geist，中文回退系统黑体。展示 500–600，不要 300 特细，也不要 80px 英雄标题。
- 公共站主标题大约 40–48px。那句比字号大更重要：「一个 Key，可解释路由，账能复算。」
- 金额、单价、token、汇率：永远 `tabular-nums` / `tnum`。
- 模型 id、provider slug、request / attempt id、API 路径：等宽（JetBrains Mono / Geist Mono）。
- 小标题可以大写等宽：`PRICE BOOK`、`ATTEMPT`、`LEDGER`、`HOLD`。

## 5. 布局与形状

- 间距底数 8px。控制台比公共站更密，但 token 同一套。
- 圆角：按钮/卡片 `{rounded.stamp}` 6px，输入/分段控件 `{rounded.control}` 4px。不要胶囊主按钮。
- 层次靠纸/碳切换和 1px 细线，不要大投影、雾面光晕、网格渐变。
- 公共站内容宽约 1120–1200px；控制台表格可以吃满工作区。

## 6. 组件要点

- **主按钮**：钴蓝实心、6px、约 8×16 内边距；一区一枚。
- **次按钮**：透明底 + 细线，墨色字。
- **卡片**：抬起表面 + 1px 细线 + 6px，内边距 24px。
- **输入**：细线、4px、焦点用 `--brand-emphasis` 环，不要厚阴影。
- **表**：表头可用等宽小标题；金额列右对齐且等宽。
- **主题分段控件**：浅色 / 深色 / 系统，放在所有入口的顶栏。当前档用淡章底，不要做成开关隐喻「深色才是专业」。

## 7. 四个入口的主角

| 入口 | 页面英雄应该是 |
| --- | --- |
| 公共站 | 模型价目（输入/输出/媒体单位）+ 带品牌 Base URL 的 `curl` |
| 用户台 | 余额、预授权占用、Key、最近一张路由回单 |
| 渠道台 | 佣金冻结 → 可结算，额度用了多少。手机上先保证这张表 |
| 管理台 | 待对账、毛利、佣金负债、Provider 健康。不要大英雄区 |

路由回单示例：`public_model → provider → attempt 2 → 原因 429 → 客户只收一笔`。

密度可以不同，token 和组件语法必须相同。主题切换不能重排信息层级。

## 8. 能做 / 不能做

### 能做

- 四个入口都提供浅色、深色、跟随系统。
- 只把钴蓝当签核章。
- 金额等宽，ID 等宽。
- OEM 只换章和 Logo。
- 用产品截图/价目表/回单当视觉主角。

### 不能做

- 不要用电青、石板蓝或霓虹当品牌色（那是探活灯，不是清算台）。
- 不要用翠绿当品牌色（会和「已结算」打架）。
- 不要抄 Stripe 渐变网格、Together 三色缎带、Linear 营销站死深色。
- 不要把控制台锁死深色、公共站锁死浅色。
- 不要用 `dark:` 写一套完全不同的布局或另一套色相。
- 不要大面积投影、发光边、胶囊 CTA。
- 不要把语义色和品牌色混用。

## 9. 响应式

| 宽度 | 变化 |
| --- | --- |
| ≥ 1024px | 价目/卡片最多 3 列；顶栏完整展示主题分段 |
| 768–1023px | 2 列；主题分段可收成图标+当前档 |
| < 768px | 1 列；触控目标 ≥ 44px；主题三项仍要能点到 |

## 10. 给实现的短指令

1. 先定表面（纸或碳），再放组件。
2. 只引用 token 名，不要复制竞品 hex。
3. 金额用 `{typography.mono}` + `tabular-nums`。
4. 改品牌色时同时验收浅色和深色的对比度。
5. 每个入口、每种模式都要能走完：登录、看余额/表、切主题后结构不变。
