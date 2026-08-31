---
version: alpha.2
name: TokenHub-Clearing
description: TokenHub 的视觉宪法。隐喻仍是清算所——公布牌价、扣住预授权、写下不可改的账。公共站的版式手艺参考 Ofox（留白、无框统计、双 CTA、价目作高潮），不搬橙色、狐狸标或促销墙。结构对齐 Stitch DESIGN.md，内容只服务本产品的四个入口、账本和 OEM。
colors:
  brand: "#2150D6"
  brand-press: "#183CA8"
  brand-soft: "#DCE6FB"
  brand-soft-dark: "#243056"
  brand-emphasis: "#2150D6"
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
  scrim-light: "rgba(20, 20, 20, 0.40)"
  scrim-dark: "rgba(20, 20, 20, 0.64)"
typography:
  display:
    fontFamily: "Inter, Geist, 'PingFang SC', 'Hiragino Sans', 'Noto Sans SC', 'Noto Sans JP', system-ui, sans-serif"
    fontSize: 56px
    fontWeight: 600
    lineHeight: 1.08
    letterSpacing: -0.8px
  display-sm:
    fontFamily: "Inter, Geist, 'PingFang SC', 'Hiragino Sans', 'Noto Sans SC', 'Noto Sans JP', system-ui, sans-serif"
    fontSize: 40px
    fontWeight: 600
    lineHeight: 1.15
    letterSpacing: -0.6px
  title:
    fontFamily: "Inter, Geist, 'PingFang SC', 'Hiragino Sans', 'Noto Sans SC', 'Noto Sans JP', system-ui, sans-serif"
    fontSize: 24px
    fontWeight: 600
    lineHeight: 1.25
    letterSpacing: -0.2px
  heading:
    fontFamily: "Inter, Geist, 'PingFang SC', 'Hiragino Sans', 'Noto Sans SC', 'Noto Sans JP', system-ui, sans-serif"
    fontSize: 18px
    fontWeight: 600
    lineHeight: 1.3
  body:
    fontFamily: "Inter, Geist, 'PingFang SC', 'Hiragino Sans', 'Noto Sans SC', 'Noto Sans JP', system-ui, sans-serif"
    fontSize: 16px
    fontWeight: 400
    lineHeight: 1.65
  body-sm:
    fontFamily: "Inter, Geist, 'PingFang SC', 'Hiragino Sans', 'Noto Sans SC', 'Noto Sans JP', system-ui, sans-serif"
    fontSize: 14px
    fontWeight: 400
    lineHeight: 1.5
  caption:
    fontFamily: "Inter, Geist, 'PingFang SC', 'Hiragino Sans', 'Noto Sans SC', 'Noto Sans JP', system-ui, sans-serif"
    fontSize: 13px
    fontWeight: 400
    lineHeight: 1.45
  button:
    fontFamily: "Inter, Geist, 'PingFang SC', 'Hiragino Sans', 'Noto Sans SC', 'Noto Sans JP', system-ui, sans-serif"
    fontSize: 14px
    fontWeight: 500
    lineHeight: 1.2
  eyebrow:
    fontFamily: "ui-monospace, 'JetBrains Mono', 'Geist Mono', monospace"
    fontSize: 11px
    fontWeight: 500
    lineHeight: 1.2
    letterSpacing: 0.08em
    textTransform: uppercase
  mono:
    fontFamily: "ui-monospace, 'JetBrains Mono', 'Geist Mono', monospace"
    fontSize: 13px
    fontWeight: 400
    lineHeight: 1.5
  tabular:
    fontFamily: "ui-monospace, 'JetBrains Mono', 'Geist Mono', monospace"
    fontSize: 14px
    fontWeight: 500
    lineHeight: 1.4
    fontFeature: tnum
  stat:
    fontFamily: "ui-monospace, 'JetBrains Mono', 'Geist Mono', monospace"
    fontSize: 32px
    fontWeight: 500
    lineHeight: 1.1
    fontFeature: tnum
rounded:
  stamp: 10px
  card: 12px
  control: 8px
spacing:
  xs: 4px
  sm: 8px
  md: 16px
  lg: 24px
  xl: 32px
  section: 96px
  section-sm: 64px
components:
  button-primary:
    backgroundColor: "{colors.brand}"
    textColor: "{colors.on-brand}"
    typography: "{typography.button}"
    rounded: "{rounded.stamp}"
    padding: 10px 20px
  button-primary-press:
    backgroundColor: "{colors.brand-press}"
    textColor: "{colors.on-brand}"
    typography: "{typography.button}"
    rounded: "{rounded.stamp}"
  button-secondary:
    backgroundColor: transparent
    textColor: "{colors.paper-ink}"
    borderColor: "{colors.paper-hairline}"
    typography: "{typography.button}"
    rounded: "{rounded.stamp}"
    padding: 10px 20px
  button-danger:
    backgroundColor: "{colors.danger}"
    textColor: "{colors.on-brand}"
    typography: "{typography.button}"
    rounded: "{rounded.stamp}"
    padding: 10px 20px
  button-ghost:
    backgroundColor: transparent
    textColor: "{colors.paper-ink-secondary}"
    typography: "{typography.button}"
    rounded: "{rounded.stamp}"
  text-input:
    backgroundColor: "{colors.paper-raised}"
    textColor: "{colors.paper-ink}"
    typography: "{typography.body}"
    rounded: "{rounded.control}"
    padding: 8px 12px
    borderColor: "{colors.paper-hairline}"
  text-input-focus:
    borderColor: "{colors.brand-emphasis}"
  card-raised:
    backgroundColor: "{colors.paper-raised}"
    textColor: "{colors.paper-ink}"
    rounded: "{rounded.card}"
    padding: 24px
    borderColor: "{colors.paper-hairline}"
  top-nav:
    backgroundColor: "{colors.paper}"
    textColor: "{colors.paper-ink}"
    typography: "{typography.body-sm}"
    height: 64px
  sidebar-row:
    backgroundColor: "{colors.paper}"
    textColor: "{colors.paper-ink}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.control}"
    padding: 8px 12px
  sidebar-row-active:
    backgroundColor: "{colors.brand-soft}"
    textColor: "{colors.brand-emphasis}"
    activeIndicator: "{colors.brand}"
  theme-toggle:
    backgroundColor: "{colors.paper-raised}"
    textColor: "{colors.paper-ink-mute}"
    typography: "{typography.caption}"
    rounded: "{rounded.control}"
  badge-ready:
    textColor: "{colors.success}"
    typography: "{typography.eyebrow}"
  badge-hold:
    textColor: "{colors.hold}"
    typography: "{typography.eyebrow}"
  badge-danger:
    textColor: "{colors.danger}"
    typography: "{typography.eyebrow}"
  badge-degraded:
    textColor: "{colors.degraded}"
    typography: "{typography.eyebrow}"
  price-book-row:
    backgroundColor: "{colors.paper-raised}"
    textColor: "{colors.paper-ink}"
    typography: "{typography.body-sm}"
    borderColor: "{colors.paper-hairline}"
    padding: 16px 20px
  ledger-row:
    backgroundColor: "{colors.paper-raised}"
    textColor: "{colors.paper-ink}"
    typography: "{typography.tabular}"
    borderColor: "{colors.paper-hairline}"
    padding: 12px 16px
  routing-receipt:
    backgroundColor: "{colors.paper-raised}"
    textColor: "{colors.paper-ink}"
    typography: "{typography.mono}"
    rounded: "{rounded.card}"
    padding: 16px
    borderColor: "{colors.paper-hairline}"
  api-key-card:
    backgroundColor: "{colors.paper-raised}"
    textColor: "{colors.paper-ink}"
    typography: "{typography.mono}"
    rounded: "{rounded.card}"
    padding: 16px
  auth-card:
    backgroundColor: "{colors.paper-raised}"
    textColor: "{colors.paper-ink}"
    rounded: "{rounded.card}"
    padding: 32px
    borderColor: "{colors.paper-hairline}"
  confirm-dialog:
    backgroundColor: "{colors.paper-raised}"
    textColor: "{colors.paper-ink}"
    rounded: "{rounded.card}"
    padding: 24px
  empty-state:
    backgroundColor: "{colors.paper-raised}"
    textColor: "{colors.paper-ink-mute}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.card}"
    padding: 48px
  toast:
    backgroundColor: "{colors.paper-raised}"
    textColor: "{colors.paper-ink}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.control}"
    padding: 12px 16px
  code-block:
    backgroundColor: "{colors.carbon-raised}"
    textColor: "{colors.carbon-ink}"
    typography: "{typography.mono}"
    rounded: "{rounded.control}"
    padding: 16px
  docs-nav:
    backgroundColor: "{colors.paper}"
    textColor: "{colors.paper-ink-secondary}"
    typography: "{typography.body-sm}"
  chart-tooltip:
    backgroundColor: "{colors.paper-raised}"
    textColor: "{colors.paper-ink}"
    borderColor: "{colors.paper-hairline}"
    typography: "{typography.caption}"
  hero-stat:
    backgroundColor: transparent
    textColor: "{colors.paper-ink}"
    typography: "{typography.stat}"
    padding: 0
  vendor-strip:
    backgroundColor: transparent
    textColor: "{colors.paper-ink-secondary}"
    typography: "{typography.body-sm}"
    borderColor: "{colors.paper-hairline}"
    padding: 24px 0
  split-demo:
    backgroundColor: transparent
    rounded: "{rounded.card}"
  closing-cta:
    backgroundColor: transparent
    textColor: "{colors.paper-ink}"
    padding: 64px 0
  site-footer:
    backgroundColor: "{colors.paper}"
    textColor: "{colors.paper-ink-mute}"
    typography: "{typography.caption}"
    padding: 48px 0 24px
---

# TokenHub Clearing

这份文件是设计事实源。结构对齐 [Stitch DESIGN.md](https://stitch.withgoogle.com/docs/design-md/specification/) 与 awesome-design-md 的九段写法；**内容只服务 TokenHub 的产品定位**。

四个入口（公共/充值站、用户控制台、渠道/代理控制台、平台管理台）和 OEM 换皮都读这里。不要再从 M0 脚手架的石板青推断品牌。

口令：**一张已发布的价目单，外加一枚签核章。**

`alpha.2` 把公共站的**版式手艺**写清楚：大标题、留白、无框统计、双 CTA、价目作高潮。隐喻、OEM 契约、语义色、二次确认不变。等四个入口都按这套节奏压过真数据后再升 `1.0`。

## 1. Visual Theme & Atmosphere

TokenHub 是面向开发者和渠道商的**多模型 API 中转与分销清算台**。第一竞争力不是模型墙，而是：

1. 同一请求走了哪家、为什么 fallback，每次 attempt 留得住。
2. 客户收费、上游成本、渠道佣金是三条账，重试和退款不能打乱。
3. A/B/C/OEM 能独立运营，平台仍管住额度、底价和佣金上限。

所以气质是**官方、可引用、可对账**。公共站要让支付宝/微信用户和 KOL 觉得正规，也要让第一次来的开发者觉得这是一个被设计过的产品，而不是一张内部表单。控制台要让财务和值班把表看清楚。

**Key Characteristics:**

- Light 是纸，Dark 是碳；四个入口都提供浅色 / 深色 / 跟随系统。
- 品牌只留一枚可替换钴蓝章。颜色表示「这一笔成立了」，不表示「我们是 AI」。
- 主角是价目表、路由回单、账本行、佣金冻结，不是摄影或插画。
- 高风险操作（退款、加款、改佣金、改凭据、改价格底线）必须二次确认，视觉上要像盖章，不像轻点。
- 公共站用编辑级节奏（大标题 + 空气 + 数字），控制台用账本级密度。两套节奏共用同一套 token，不换色相。

### 1.1 向 Ofox 学什么

参考 [ofox.ai/zh](https://ofox.ai/zh) 的**手艺**，不搬它的身份。Ofox 让人觉得「有设计感」，通常不是因为橙色或狐狸，而是：

| 学 | 落到 TokenHub |
| --- | --- |
| 标题够大，副文够短，第一屏先承诺再证明 | 公共站 `{typography.display}` 56px；一句承诺里最多一个钴蓝强调词 |
| 统计数字直接坐在留白里，不装箱 | `hero-stat`：大号等宽数字 + 下方 mute 标签 |
| 主按钮实心、次按钮细线，永远成对出现 | 开始使用 / 看价目；获取 Key / 看文档 |
| 供应商名做成静音跑马灯，建立「这是真目录」的信任 | `vendor-strip`：只列已发布公开模型的厂商字，不要假 Logo 墙 |
| 价目表才是页面高潮，不是英雄插画 | `price-book-row` 吃满内容宽；数字右对齐 |
| 左代码、右结果 | 左 curl（品牌 Base URL），右 `routing-receipt` 示例 |
| 页脚分层：产品 / 文档 / 账户 / 状态 | `site-footer` 四栏，底行状态字 |
| 圆角当代、仍是方章 | 按钮 10px、卡片 12px、输入 8px；禁止胶囊 |

**明确不学：**

- 不学 `#ff5000` / `#fc5404` 橙。TokenHub 的章是钴，OEM 也只换这一枚章。
- 不学狐狸标、充值倒计时条、折扣角标、图像/视频砌体墙。
- 不学把「3 分钟接入世界模型」这类口号当视觉系统。我们的承诺是价目已发布、attempt 留得住、账能复算。
- 不学巨量 mega-nav。公共站顶栏仍是 Logo、文档/模型/定价、主题、登录、一枚主章。

一句话：**Ofox 证明开发者站可以又好用又好看；TokenHub 用同样的留白和信息层级，讲清算所的故事。**

## 2. Color Palette & Roles

### 2.1 Light / Dark

| 模式 | 表面 | 画布 | 抬起 | 正文 | 细线 |
| --- | --- | --- | --- | --- | --- |
| Light | 纸 | `{colors.paper}` `#F4F1EA` | `{colors.paper-raised}` `#FFFDF8` | `{colors.paper-ink}` `#141414` | `{colors.paper-hairline}` `#D9D3C7` |
| Dark | 碳 | `{colors.carbon}` `#161513` | `{colors.carbon-raised}` `#1F1D1A` | `{colors.carbon-ink}` `#EDEAE4` | `{colors.carbon-hairline}` `#2A2723` |

实现时映射为 `--canvas` / `--canvas-raised` / `--ink` / `--ink-secondary` / `--ink-mute` / `--hairline`。YAML 里的 `paper-*` 是 Light 真值；Dark 换碳的对应值。组件不要写死 `paper-ink`，要写语义变量。

纸保持微暖，不要改成 Ofox 的冷灰 `#f4f4f4`。暖纸是清算所的触感；冷灰会让四个入口看起来像又一个模型聚合站。

三档：浅色、深色、跟随系统。默认跟随系统，键名 `tokenhub-theme`。OEM 可建议默认档，不能关掉其中一档。

| Token | Light | Dark | 用途 |
| --- | --- | --- | --- |
| `--brand` | `#2150D6` | `#2150D6` | 实心主按钮、当前导航指示 |
| `--brand-press` | `#183CA8` | `#183CA8` | 按下 |
| `--brand-soft` | `#DCE6FB` | `#243056` | 选中行、已发布淡章 |
| `--brand-emphasis` | `#2150D6` | `#8AA4FF` | 链字、焦点环、标题里的一个强调词 |
| `--on-brand` | `#FFFFFF` | `#FFFFFF` | 章上的字 |

钴蓝在一屏里只出现这些地方：一枚主章、至多一个标题强调词、当前导航指示、已发布淡章。其余全是墨与纸。这和 Ofox 把橙色用得很省是同一条纪律。

### 2.2 语义色（OEM 不得改）

| 状态 | Token | Hex | 产品例子 |
| --- | --- | --- | --- |
| 成立 / 已结算 | `--success` | `#1F7A4D` | `settled`、`paid`、`released`、`available` |
| 扣住 / 冻结 | `--hold` | `#B45309` | `preauthorized`、`frozen`、`pending`、`usage_pending_reconciliation` |
| 冲正 / 失败 | `--danger` | `#C23B22` | `reversed`、`refunded`、`denied`、`unavailable` |
| 降级 | `--degraded` | `#9A6700` | `degraded`、`maintenance`、`past_due` |

待对账不要做成灰字「未知」。它是还没盖章的回单，用 `--hold` + 文案 `HOLD` / `RECONCILE`。

健康态 `available` / `degraded` / `unavailable` / `maintenance` 走语义色，并同时写字，不能只靠色点。

### 2.3 对比度

- 正文与画布 ≥ 4.5:1；大标题 ≥ 3:1。
- 实心章：白字配 `#2150D6`，两套模式都成立。
- 深色链字必须用 `--brand-emphasis` `#8AA4FF`，不要用实心钴当正文色。
- OEM 换主色时，浅色和深色都要重测链字/焦点环。不达标就拒绝发布该 `theme_json`。
- 状态不能只靠颜色：徽章必须带 `READY` / `HOLD` / `REVERSED` 这类字。

### 2.4 `brand.theme_json` 契约

对应 `docs/05` 的 `brand.theme_json`。OEM（渠道 C）只能写这些 key：

```json
{
  "display_name": "Acme API",
  "logo_url": "https://...",
  "favicon_url": "https://...",
  "default_theme": "system",
  "brand": "#2150D6",
  "brand_press": "#183CA8",
  "brand_soft": "#DCE6FB",
  "brand_soft_dark": "#243056",
  "brand_emphasis": "#2150D6",
  "brand_emphasis_dark": "#8AA4FF",
  "on_brand": "#FFFFFF"
}
```

禁止出现：`paper`、`carbon`、`success`、`hold`、`danger`、`degraded`、圆角、字号、渐变、背景图。B 渠道用平台品牌，不写自己的 theme。域名、Logo、站点名按 `docs/04` D12；配色只换章。

## 3. Typography Rules

正文字体实现用 **Inter**（拉丁）+ 中日文回退。DESIGN.md 长期把 Inter 写在第一位；不要用更「AI 感」的展示体替换它。

| Token | Size | Weight | 用途 |
| --- | --- | --- | --- |
| `{typography.display}` | 56px | 600 | 公共站首页主标题。手机落到 36–40px。不要超过 64px |
| `{typography.display-sm}` | 40px | 600 | 文档、登录、次级公共页标题 |
| `{typography.title}` | 24px | 600 | 控制台页标题、公共站区块标题 |
| `{typography.heading}` | 18px | 600 | 卡片标题 |
| `{typography.body}` | 16px / 1.65 | 400 | 说明、文档正文。公共站副文可以再松一点 |
| `{typography.body-sm}` | 14px | 400 | 表内文案、侧栏 |
| `{typography.caption}` | 13px | 400 | 辅助、页脚 |
| `{typography.button}` | 14px | 500 | 按钮 |
| `{typography.eyebrow}` | 11px | 500 | `PRICE BOOK`、`ATTEMPT`、`LEDGER` |
| `{typography.mono}` | 13px | 400 | 模型 id、request/attempt id、API 路径、curl |
| `{typography.tabular}` | 14px | 500 | 金额、token、单价、汇率 |
| `{typography.stat}` | 32px | 500 | 英雄区无框统计数字 |

原则：

- 负字距只加在拉丁展示字。中文/日文不要 `letter-spacing: -0.8px`。
- 中文回退 `PingFang SC`、`Noto Sans SC`；日文回退 `Hiragino Sans`、`Noto Sans JP`。三语（next-intl）共用布局，不为某一语言换皮肤。
- 金额、用量永远 `tnum` / `tabular-nums`，右对齐。
- 完整 API Key 默认掩码；用户主动复制才露出。文档示例禁止内嵌真实 Key。
- 公共站主标题里最多**一个**强调词，用 `--brand-emphasis`。不要整句上色，不要渐变字。

## 4. Component Stylings

组件对齐 shadcn/Radix，但皮肤只准用上面的 token。下面每条都对应真实产品面，不写通用电商件。

### 4.1 按钮

**`button-primary`**：钴蓝实心章，10px 圆角，内边距 10×20。每个可见区块最多一枚。用于「充值」「创建 Key」「发布套餐」「确认结算」「开始使用」。

**`button-secondary`**：细线 + 墨字，同样 10px。用于「导出」「查看文档」「看价目」「Google 登录」。

**`button-danger`**：朱红实心。只用于退款、冲正、禁用 Key、封禁。必须出现在二次确认里，不要单独躺在表行里当主操作。

**`button-ghost`**：无边，次要导航或「取消」「登录」（顶栏里主章旁边的那一个）。

按下走 `{colors.brand-press}`。禁用：40% 透明，不可点。触控高 ≥ 40px，手机 ≥ 44px。

不要做成 `border-radius: 999px` 的胶囊。10–12px 已经是当代产品的方章，再圆就变成促销按钮。

### 4.2 输入

**`text-input`**：抬起表面、8px、1px 细线。焦点 2px `--brand-emphasis` 环，不要厚阴影。

校验失败：细线改 `--danger`，下方 caption 用危险色写原因（余额不足 `402`、限流 `429` 要用产品文案，不要只画红框）。

密钥、密码：等宽掩码；「查看」写审计。

### 4.3 导航与壳

**公共站 `top-nav`**：64px，纸/碳画布，底 1px 细线。左 Logo+字标，中文档/模型/定价（字重 400，当前项淡章底，不要整段灌钴），右 主题分段 + ⌘K + 登录（ghost 或 secondary）+ 一枚主章（开始使用/充值）。链接之间要有空气，不要挤成工具栏。

**控制台侧栏 `sidebar-row`**：宽 240px。当前项：左 2px 钴蓝条 + `{colors.brand-soft}` 底。不要整行灌满钴蓝。

菜单按门户裁剪（D33），隐藏不是安全边界：

| 入口 | 侧栏只出现 |
| --- | --- |
| 用户台 | 总览、余额/充值、套餐、API Key、用量/账单、媒体任务、文档、设置 |
| 渠道台 | 本渠道用户、套餐、推广、额度、用量、佣金/结算 |
| 管理台 | 总览、提供商、模型、路由组、价格、用户、余额、用量、渠道、套餐审核、佣金策略、指标、审计、设置 |

用户台不得出现 Provider 密钥、平台成本、他人数据、分销佣金。渠道台不得出现其他渠道和 prompt/completion。财务看账务，默认不看上游密钥。技术管凭据，不能退款或改佣金。

**`theme-toggle`**：浅色 / 深色 / 系统。所有入口顶栏都要有。当前档用淡章底。不要用「深色=专业」的开关隐喻。

### 4.4 产品卡片与行

**`price-book-row`**：公开模型价目，公共站的视觉高潮。列：`public_model_id`（等宽）、厂商、输入/输出/媒体单价（tabular，这是行里最重的字）、能力、Provider 状态字。不展示内部成本价、上游 Key、敏感故障。历史价是快照，现价变更不得改写旧行的数字颜色以外的「已改」动画。

公开 payload 还没有单价时，可以先用目录卡（id + 厂商 + 能力），但卡上不要发明价格，也不要用插画填空。单价一到，立刻回到表。

**`ledger-row`**：钱包/权益/佣金流水。金额右对齐等宽；方向用语义字 + 色（入账 success、扣住 hold、冲正 danger）。客户收费、上游成本、佣金分三列或三个视图，禁止一个绿数字打天下。

**`routing-receipt`**：用户台英雄组件，也是公共站 split-demo 的右侧。一行讲清：`public_model → provider → attempt N → 原因 429 → 客户只收一笔`。attempt 可展开，每次上游尝试单独一行。这是「可解释路由」的视觉对应物。公共站示例必须标明 EXAMPLE，禁止写真实 Key 或平台成本。

**`api-key-card`**：名称、前缀、状态、RPM/并发、模型白名单。完整 Key 掩码；复制/轮换/禁用都是次按钮 + 审计。过期用 `--hold` 或 `--danger` 字标。

**`card-raised`**：套餐、媒体任务、渠道额度等通用容器。媒体任务显示 `queued / in_progress / completed / failed`，进度用细线而不是发光环。

**`hero-stat`**：无框。上方或下方 mute 标签，中间 `{typography.stat}`。不要放进带边框的小卡片里——装箱会把证明变成装饰。

**`vendor-strip`**：已发布公开模型的厂商名，单行或轻量换行，字色 `--ink-secondary`，上下 1px 细线。可以慢速平移，不要彩色 Logo 墙，不要把未发布厂商写上去。

### 4.5 表（TanStack Table）

表头用 `{typography.eyebrow}`。行 1px 底边。悬停：淡章 40%，不要投影。选中：`--brand-soft`。

金额列 `{typography.tabular}` 右对齐。分页、筛选、导出是次按钮，不是主章。空表用 `empty-state`，不要骨架闪烁假装有数。

公共站价目表行高可以比控制台松一档（16–20px 内边距），让单价先被看见。

### 4.6 登录与高风险确认

**`auth-card`**：居中抬起卡片。邮箱密码、验证码、Google。推广码不展示为可改归属；注册后渠道不可自助切换。没有渐变英雄，没有社交插画。登录页可以左右分栏（左一句承诺 + 右表单），但左栏仍然是纸上的字，不是摄影。

**`confirm-dialog`**：退款、手工加款、佣金调整、凭据修改、价格底线必须走这层。标题说清后果（会冲正佣金 / 会停止新请求）。主操作在危险场景用 `button-danger`，普通确认用 `button-primary`。蒙层 `{colors.scrim-light}` / `{colors.scrim-dark}`。

### 4.7 空状态、Toast、徽章

**`empty-state`**：一句人话 + 一个次按钮。例如「还没有用量事件」「还没有冻结佣金」。不要机器人插画。

**`toast`**：抬起 + 细线 + 左侧 2px 语义条。成功/冻结/失败对应 success/hold/danger。自动消失不用于高风险结果；退款成功应留在页内回单。

**徽章**：只用字 + 语义色，背景透明或淡。`READY` `HOLD` `REVERSED` `DEGRADED` `MAINTENANCE`。

### 4.8 文档与代码（D34）

**`docs-nav`**：左栏目录。正文栏约 720px，阅读优先。

**`code-block`**：即使公共站是纸，代码块也落在碳面（回单/终端）。内嵌当前品牌 Base URL 和模型白名单；Key 用 `sk-...xxxx` 占位。OEM 文档不出现平台成本、内部路由细节。旧版本文档保留，废弃接口用 `--hold` 眉题 `DEPRECATED`。

**`split-demo`**：公共站专用。左 `code-block`（curl），右 `routing-receipt` 示例。两栏等宽，中间空气，不要把预览做成视频或生成图。

语言切换（中/英/日）放文档顶栏，不要另做一套皮肤。

### 4.9 图表（ECharts，管理/渠道总览）

管理台总览要能看：成功率、P50/P95、上游错误、平台成本、客户收入、毛利、余额风险、佣金负债、渠道消耗、待对账。

- 轴和标注：`--ink-mute`，数字 tabular。
- 网格：`--hairline`，不要渐变面积。
- 主序列：`--brand`；对比序列：`--ink-secondary`。
- 账本类堆叠：入账 `--success`，占用 `--hold`，冲正 `--danger`。毛利不要用彩虹。
- Tooltip：`chart-tooltip`，纸/碳抬起 + 细线。
- 无数据：`empty-state`，不要画一条假的平滑曲线。

### 4.10 页脚与收束 CTA

**`closing-cta`**：公共站在价目和充值之后、页脚之前，重复一句承诺 + 双按钮。不要第三枚主章，不要倒计时。

**`site-footer`**：四栏（产品入口、文档、账户、状态/法务）+ 底行版权与主题口令。链接用 caption，不要再放一枚主按钮。OEM 用 `display_name` 替换站点名。

## 5. Layout Principles

- 间距底数 8px。公共站段落用 `{spacing.section}` 96px；文档和登录用 `{spacing.section-sm}` 64px；控制台表区用 16–24px。
- 公共站内容宽 1120–1200px。控制台：侧栏 240px + 表吃满。文档正文 720px。
- 公共站没有侧栏。三个控制台同一套壳，菜单不同。
- 主题切换不得重排信息层级，不得换另一套色相。
- 公共站第一屏：标题 → 一句副文 → 三个 `hero-stat` → 双 CTA。统计不要抢主章的位置。

四个入口的英雄（不是装饰，是当页主信息）：

| 入口 | 英雄 |
| --- | --- |
| 公共站 | 大标题 + 无框统计 + 价目表 + 带品牌 Base URL 的 curl / 回单对页 |
| 用户台 | 余额、预授权占用、Key、最近一张路由回单 |
| 渠道台 | 佣金冻结 → 可结算；额度用了多少。手机先保证这张表 |
| 管理台 | 待对账、毛利、佣金负债、Provider 健康。不要大英雄区 |

公共站推荐纵向节奏（可按数据有无折叠，不可换成砌体墙）：

1. 顶栏
2. 英雄（承诺 + 统计 + 双 CTA）
3. `vendor-strip`（有已发布厂商才出现）
4. `split-demo`（curl + 示例回单）
5. 价目 / 模型目录（高潮）
6. 套餐与充值（真按钮，文案保持产品验收用词）
7. `closing-cta`
8. 四栏页脚

## 6. Depth & Elevation

| 层级 | 做法 | 用途 |
| --- | --- | --- |
| 0 | 画布，无影 | 页面底、侧栏底 |
| 1 | 抬起表面 + 1px 细线 | 卡片、表、回单、输入 |
| 2 | 同样细线；可选 `rgba(20,20,20,0.06) 0 1px 2px` | Toast、下拉 |
| 3 | 蒙层 + 抬起卡片 | 确认对话框 |

不要大投影、雾面光晕、网格渐变。层次靠纸/碳，不靠光。Ofox 几乎不用阴影；TokenHub 同样靠细线和抬起，不靠光晕假装高级。

## 7. Logo、字标与动效

**字标**：站点名用 `{typography.title}` 墨色，不要发光。平台默认写 TokenHub；OEM 用 `display_name`。

**标记**：可选 20–24px 几何章（10px 圆角的钴方，或一条分叉折线表示路由）。不要霓虹、不要抽象大脑、不要狐狸。Favicon 同标记。

**动效**：120–180ms，ease-out。主题切换关闭过渡（已用 `disableTransitionOnChange`）。对话框：淡入 + 上移 4px。Toast：自下而上 8px。表行不要闪。`vendor-strip` 平移必须可暂停（`prefers-reduced-motion` 时静止）。不要视差、不要页面级渐变扫光。

## 8. Do's and Don'ts

### Do

- 四个入口都提供浅色 / 深色 / 系统。
- 把路由回单、价目、账本行当主角。
- 公共站用大标题、留白、无框统计、双 CTA；控制台保持账本密度。
- 金额等宽，ID 等宽，状态写字。
- 高风险操作走确认对话框并写审计。
- OEM 只换 `theme_json` 允许的章和 Logo。
- 敏感字段按门户藏：前端藏了，后端仍要拒。
- 学 Ofox 的节奏：证明用数字，高潮用价目，收束用一对按钮。

### Don't

- 不要用电青、石板蓝、霓虹当品牌（探活灯，不是清算台）。
- 不要用翠绿当品牌（和已结算打架）。
- 不要用 Ofox 橙、狐狸标、充值倒计时、折扣角标、生成图砌体墙。
- 不要把纸改成冷灰营销底，来「更像 Ofox」。
- 不要抄 Stripe 网格、Together 缎带、Linear 死深色营销站。
- 不要把控制台锁深色、公共站锁浅色。
- 不要用 `dark:` 写另一套布局。
- 不要胶囊 CTA、大光晕、机器人空状态。
- 不要一个数字同时表示收入、成本和佣金。
- 不要在文档或示例里写真实完整 Key。
- 不要为「好看」展示平台成本或他人渠道数据。
- 不要从 awesome-design-md 或竞品整页抄皮肤；只许借用本文件已写进「能做」的原则。

## 9. Responsive Behavior

| 宽度 | 变化 |
| --- | --- |
| ≥ 1024px | 价目最多 3 列或宽表；侧栏展开；主题三段完整；split-demo 两栏；页脚四栏 |
| 768–1023px | 2 列；侧栏可收；主题可收成当前档；split-demo 上下叠；页脚两栏 |
| < 768px | 1 列；侧栏变抽屉；触控 ≥ 44px；display 落到 36–40px；渠道佣金表横向滚动而不是把金额折行到看不清；顶栏主章保留，「登录」可收进菜单 |

图片/Logo：字标旁 Logo 高 24px。文档代码块横向滚动，不要缩小到无法复制。

## 10. Agent Prompt Guide

实现或改 UI 时按这个顺序：

1. 先问：这是四个入口里的哪一个？当前用户能看见成本/佣金/密钥吗？
2. 先定表面（纸或碳），再放组件。只引用 token 名。
3. 钱和用量用 `{typography.tabular}`。模型 id、request id 用 `{typography.mono}`。公共站大标题用 `{typography.display}`。
4. 需要「可解释」就用 `routing-receipt`，不要用一张状态饼图代替。
5. 需要「账能平」就用 `ledger-row`，客户账/成本/佣金分开。
6. 公共站先排英雄 → 统计 → 双 CTA → 价目，再考虑其它模块。
7. 改品牌色时浅色和深色都验对比度。
8. 高风险动词必须进 `confirm-dialog`。
9. 不要从 Ofox 或 awesome-design-md 某站整页抄皮肤；只许借用本文件已写进「能做」的原则。

可用短指令：

> 按 DESIGN.md 的 TokenHub Clearing alpha.2：纸/碳双主题，一枚钴蓝章。公共站用 56px 标题、无框统计、双 CTA、curl 和路由回单对页、价目作高潮。圆角 10/12/8。不要橙，不要胶囊，不要生成图墙。

> 按 DESIGN.md 的 TokenHub Clearing：纸/碳双主题，一枚钴蓝章。做用户台：余额、预授权、API Key 卡、最近一张路由回单。金额等宽。不要渐变，不要青蓝。

> 按 DESIGN.md 做管理台总览：待对账、毛利、佣金负债、Provider 健康。ECharts 用 brand + hairline，不要彩虹面积图。

## 11. 已知仍待真页面压测

- `alpha.2` 只锁定公共站节奏和圆角/字号；登录、价目、账本、渠道佣金、管理总览仍要用真数据看一遍疏密，不得改隐喻和 OEM 契约。
- 图表色在色觉障碍下的成对组合，要在 M7 看板里用真实数列复验。
- 具体 Logo 路径图形（折线 vs 钴方）可在 M1 品牌资源里二选一，不得改成发光标或动物标。
- 公开模型若迟迟不带单价，目录卡只是过渡，不能变成产品的默认英雄。
