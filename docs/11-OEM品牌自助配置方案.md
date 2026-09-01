# OEM 品牌自助配置方案（待审批）

状态：**方案待审批，未开工实现。**  
关联：D12（`docs/04`）、`brand` 表（`docs/05`）、`DESIGN.md` §2.4、飞书目录「四类入口与 OEM 品牌配置」。

本文回答两个问题：

1. 现在能不能让「每个租户」自定义 Logo、品牌名、配色？
2. 若不够，按现有产品边界补到什么程度、怎么补？

---

## 1. 结论（先看这里）

**读路径已经能换皮，写路径还不能自助改。**

| 能力 | 现在 | 说明 |
| --- | --- | --- |
| 按域名识别品牌 | 能 | `GET /v1/public/brand` 用 Host 查 `identity_brands` |
| 顶栏显示品牌名 | 能 | 公共站 / 控制台读 `brand.name`，缺省写 TokenHub |
| 顶栏显示 Logo | 半成品 | 前端会渲染 `logo_url`，但没有上传、也没有写接口 |
| 换主色（章的颜色） | 半成品 | 前端把 `theme.brand` / `theme.primary` 写成 CSS 变量；种子数据仍是旧 key |
| OEM 自己改名 / 改色 / 换 Logo | **不能** | 没有 Create/Update Brand；管理页只能读品牌、签发证书 |
| 每个终端用户各自一套皮肤 | **不做** | 与已确认的 A/B/C 模型冲突 |

「租户」在本产品里**不是**每个开发者账号，而是 **OEM 渠道（类型 C）绑定的一条 `brand` 记录**。B 分销商继续用平台品牌。终端用户只继承所属渠道的品牌。

请在文末「审批选项」勾选后再开工。

---

## 2. 先对齐三个词

用一个小例子：

- **平台（渠道 A）**：官网叫 TokenHub，域名 `localhost` / 未来的公网主站。所有官方用户看到钴蓝章。
- **分销商（渠道 B）**：帮平台卖额度，自己没有独立网站。用户打开的仍是 TokenHub 官网。
- **OEM（渠道 C）**：比如种子数据里的「Aurora OEM」。它有自己的充值站、API 域名、管理后台域名。用户打开 `oem.localhost`，顶栏应写 Aurora OEM，章可以换成琥珀金。

`user.display_name` 是用户自己的昵称（Alice），**不是**站点品牌名。改昵称已经有 `PATCH /v1/me`，和换皮无关。

所以「每个租户自定义品牌」= **每个 C 渠道有一条 `identity_brands`，按访问域名生效**。不是给每个钱包、每个 API Key 做皮肤。

---

## 3. 现状（对照代码）

### 3.1 已经落地

- 表 `identity_brands`：`name`、`logo_url`、三套域名、`theme_json`、CNAME/TLS。
- 渠道 `identity_channel_orgs.brand_id` 指向品牌；用户注册时固化 `brand_id`。
- 公开接口 `GET /v1/public/brand`：按 Host 命中，未命中回落到官方品牌。
- 文档/模型目录也会带上当前品牌的 Base URL。
- 前端 `themeStyle()` 把主色写到 `--brand` / `--brand-press` / `--brand-emphasis`。
- 种子：官方 `brd_official` + OEM `brd_oem`（Aurora OEM）。

### 3.2 缺口（所以还不能说「已经支持」）

1. **没有写品牌的 API。** 现有只有 `GET /admin/brands` 和 `POST /admin/brands/{id}/tls/issue`。
2. **没有 Logo / Favicon 上传。** `logo_url` 是文本列，管理页也没有填。
3. **`theme_json` 契约和实现不一致。**  
   `DESIGN.md` 规定只能写章的颜色（`brand`、`brand_press`、`brand_soft`…）。  
   种子却写成了旧探活灯：`{"primary":"#22d3ee","background":"#020617"}`。  
   前端只吃 `brand`/`primary` 等少数 key，**纸/碳表面、成功/冻结/危险色故意不开放**（这是对的）。
4. **渠道管理员没有品牌页。** `/channel` 看不到自己的 Logo/配色。
5. **浏览器标题写死 TokenHub**（`web/app/layout.tsx` 的 `metadata.title`），OEM 域名打开标签页仍是平台名。
6. **模型/文档按品牌找渠道时写死了 `brd_oem`。** 新建第三条 OEM 品牌时，目录和文档会错绑到官方渠道。这是本次必须顺手修的隐藏债。

### 3.3 产品已经拍过的边界（不要推翻）

摘自 D12 与 `DESIGN.md` §2.4：

- OEM 可以配：站点名、Logo、Favicon、默认浅/深/跟随系统、**一枚章的颜色**。
- OEM **不能**改：纸/碳背景、圆角、字号、渐变、成功/冻结/危险/降级色。
- B 渠道用平台品牌，不写自己的 `theme_json`。
- 浅色/深色/跟随系统三档必须保留，OEM 只能建议默认档，不能关掉其中一档。
- 换主色必须过对比度：正文/链字/章上白字不达标则拒绝发布。

---

## 4. 推荐方案（请批「做 / 不做」）

### 4.1 做：把 OEM 换皮补成可运营能力

目标：平台管理员能创建品牌并绑到 C 渠道；该渠道的 `channel_admin` 能改自己的名、Logo、Favicon、章颜色；用户打开 OEM 域名立刻看到新皮。

**不新增表。** 继续用 `identity_brands` + `channel_org.brand_id`。品牌是换皮单位，渠道是账务/隔离单位，一对一即可。

```
访问 Host
    │
    ▼
identity_brands（name / logo / theme_json / 三域名）
    ▲
    │ brand_id
identity_channel_orgs（type=C）
    ▲
    │ channel_org_id
终端用户（只继承，不能改皮）
```

### 4.2 明确不做（除非另批）

| 选项 | 建议 | 原因 |
| --- | --- | --- |
| 每个终端用户 / 每个 API Key 一套皮肤 | 不做 | 没有独立站点，也没有产品入口 |
| B 分销商独立 Logo/配色 | 不做 | `docs/02` 写明 B 用平台品牌；要换皮应升级为 C |
| 整站换背景、换字体、换圆角 | 不做 | 破坏清算台气质和可访问性 |
| 一个渠道绑多个品牌、A/B 测试皮肤 | 不做 | P0 无需求 |
| 公网对象存储上传 Logo | 二期 | 见 5.3；一期先收 HTTPS URL |

---

## 5. 契约与校验

### 5.1 `theme_json` 只允许这些 key

与 `DESIGN.md` §2.4 对齐，写入时丢掉未知字段（不要静默存 `background`）：

```json
{
  "display_name": "Acme API",
  "logo_url": "https://cdn.example.com/acme-logo.png",
  "favicon_url": "https://cdn.example.com/acme-favicon.ico",
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

说明给审批人：

- `display_name` 与列 `name`：对外展示用 `name`；`theme_json.display_name` 若传入则同步写 `name`，避免两处打架。
- `logo_url` / `favicon_url` 同样：列上的 `logo_url` 是权威值；JSON 里的副本只为文档契约完整。
- 颜色必须是 `#RRGGBB`。缺省回落到官方钴蓝。
- `default_theme` 只能是 `light` / `dark` / `system`。

### 5.2 发布前对比度门禁

OEM 换色不是「随便填一个好看的」。章是实心按钮，链字是可点文字。不达标的主题拒绝保存（`400 theme_contrast`），不要上线后再说看不清。

最低线（WCAG AA）：

| 对 | 最低对比度 | 不达标时 |
| --- | --- | --- |
| `on_brand` 叠在 `brand` 上 | 4.5:1 | 拒绝 |
| 浅色模式 `brand_emphasis` 叠在纸面 `#F4F1EA` 上 | 4.5:1 | 拒绝 |
| 深色模式 `brand_emphasis_dark` 叠在碳面 `#161513` 上 | 4.5:1 | 拒绝 |

`brand_press`、`brand_soft*` 由服务端按主色推导默认值，OEM 可不填。若手填，软底只要求能分辨选中行，不强制 4.5:1。

### 5.3 Logo / Favicon（一期 URL，二期上传）

**一期（建议先做）：**

- 只收 `https://` URL（本地/e2e 允许 `http://localhost`）。
- 拒绝 `javascript:`、data URI、私网地址（复用 catalog 已有 URL allowlist）。
- 建议：Logo 正方形或宽不大于 4:1；展示高度 24px；Favicon 32×32 或 ICO。
- 不代理、不转存。坏链由 OEM 自己负责；预览页用 `<img>` 当场看。

**二期（另批）：**

- `POST /channel/brand/assets` 上传 PNG/SVG/ICO，写入受控对象存储的**公开前缀**（媒体任务默认是签名过期 URL，不适合 Logo）。
- 限制：≤ 512KB，MIME 白名单，SVG 去脚本。

一期不阻塞换皮上线。

---

## 6. API（建议补这些，不动现有读接口语义）

权限原则：前端藏按钮不是安全边界。渠道管理员只能改**自己渠道的 `brand_id`**。平台管理员可以创建品牌、改任何品牌、把品牌绑到 C 渠道。

| 方法 | 路径 | 谁 | 作用 |
| --- | --- | --- | --- |
| `GET` | `/v1/public/brand` | 匿名 | **已有。** 按 Host 返回公开品牌（不含内部 TLS 私钥） |
| `GET` | `/admin/brands` | 平台/运营/技术 | **已有。** 列表 |
| `POST` | `/admin/brands` | `platform_admin` | **新增。** 创建品牌（名、三域名、可选 theme/logo）。需二次确认 |
| `PATCH` | `/admin/brands/{id}` | `platform_admin` | **新增。** 改名/域名/theme/logo。改域名需二次确认 |
| `GET` | `/channel/brand` | `channel_admin` | **新增。** 读本渠道品牌 |
| `PATCH` | `/channel/brand` | `channel_admin` 且渠道 type=C | **新增。** 改 name / logo_url / favicon_url / theme_json。B 渠道固定 `403 brand_not_customizable` |
| `POST` | `/admin/brands/{id}/tls/issue` | 已有 | 不变 |

`POST /admin/channels` 已能带 `brand_id`。创建 C 渠道时：若没带品牌，平台应先 `POST /admin/brands`，再把新 `brand_id` 写进渠道。不要静默复用官方品牌，否则 OEM 用户会看到 TokenHub。

写操作一律：

- 校验 theme 契约 + 对比度；
- 写 `audit_log`（`brand.update` / `brand.create`），带 before/after；
- 敏感改动（域名）要 `X-Tokenhub-Confirm`。

公开 `GET /v1/public/brand` 的响应建议补齐前端已在用的字段，并增加 `favicon_url`、`default_theme`。现有字段不改名。

---

## 7. 前端怎么吃

四个入口共用同一套壳，已经按 Host 拉品牌。补齐即可，不要再做第二套皮肤系统。

1. **`themeStyle()` 对齐契约。** 写出 `--brand`、`--brand-press`、`--brand-soft`、`--brand-emphasis`、`--on-brand`；深色模式用 `brand_*_dark`。丢掉对 `background` 的幻想。
2. **`<html>` 上设 favicon 和 `<title>`。** 用 `brand.name`，不要写死 TokenHub。
3. **顶栏。** 已有 Logo + 字标；控制台侧栏同样用 `brand.name`。
4. **平台管理 `/admin/brands`（或设置页一节）。** 列表、创建、改域名、预览对比度失败原因。
5. **渠道台 `/channel/brand`。** C 渠道：表单（名称、Logo URL、Favicon URL、主色取色器、默认主题档）+ 即时预览条（一枚主按钮 + 一条链字 + 选中行）。B 渠道：只读说明「使用平台品牌」。
6. **种子 theme 迁移。** 官方改为 DESIGN 默认钴蓝；Aurora OEM 改为对比度合格的琥珀金（或其它已算过的色），去掉 `background`。

`next-themes` 继续管纸/碳。OEM 的 `default_theme` 只在用户**从未选过**时生效，不能覆盖用户已保存在 `tokenhub-theme` 里的选择。

---

## 8. 必须顺手修的隐藏债

`publicModels` / `docsContext` 现在用 `if brand.ID == brd_oem` 决定渠道。第三条 OEM 会错。

改为：`channel_org` 按 `brand_id` 查找（一个品牌只绑一个 C 渠道）。公开模型、文档示例、注册落地页都走这条查找，禁止再写死种子 ID。

---

## 9. 验收（批准后才写测试）

后端：

- 创建第二条 OEM（不是种子 `brd_oem`），Host 命中后 `GET /v1/public/brand` 返回新 name/logo/theme。
- 该 Host 的 `/v1/public/models` 与 `/v1/public/docs-context` 绑到新渠道，不泄漏官方模型策略。
- C 渠道管理员可 PATCH 自己的品牌；B 渠道 403；其它渠道的管理员改不到这条品牌。
- `theme_json` 含 `paper` / `success` / 非法 hex → 400。
- 白字叠在浅黄主色上对比度不够 → 400。
- 改品牌写审计。

前端：

- `oem.localhost` 顶栏、标题、favicon、主按钮色来自该品牌。
- 官方域名不受 OEM 改色影响。
- 浅色/深色切换后，链字仍用 `brand_emphasis` / `brand_emphasis_dark`。
- Playwright：渠道品牌页保存后，公共站刷新可见。

---

## 10. 实现切片（批准后的开工顺序）

不估日历，只按依赖切开，每一刀可单独验收：

1. **契约与校验库**：theme key 白名单、hex、对比度；种子 theme 改成合法值；前端 `themeStyle` 对齐。
2. **写 API**：`POST/PATCH /admin/brands`、`GET/PATCH /channel/brand` + 审计 + 确认头。
3. **按品牌找渠道**：修公开模型/文档的硬编码。
4. **管理面 + 渠道面表单**；文档标题/favicon。
5. **e2e / Playwright**；更新 `docs/06`、`docs/09`、飞书目录该条为「已实现待验收」。

二期（另批）：Logo 上传、公网 Custom Hostname（飞书目录里已有独立条目）。

---

## 11. 审批选项

请直接回复编号，或改条件后批：

| 编号 | 选项 | 建议 |
| --- | --- | --- |
| **A** | 按本文做：C 渠道可改名 / Logo URL / Favicon URL / 章颜色；B 不换皮；终端用户不换皮 | **推荐** |
| **B** | 只要平台管理员能改，渠道自己不能改 | 能上线，但 OEM 每次改 Logo 都要工单 |
| **C** | 还要给 B 渠道独立品牌 | 需先改 `docs/02`，相当于把 B 升级成「无独立域名的弱 OEM」 |
| **D** | 还要每个终端用户/工作区换皮 | 不建议；要新实体，和四类入口模型冲突 |
| **E** | 一期就要对象存储上传 Logo，不要外链 | 可做，但依赖公开 ACL 前缀，工作量大于 URL |
| **F** | 暂缓，P0 继续用种子品牌手工改库 | 文档现状可以支撑演示，不能支撑真实 OEM 签约 |

默认按 **A + 切片 1→5** 开工。你批 A（或 A+E / B）后，再开实现 PR。
