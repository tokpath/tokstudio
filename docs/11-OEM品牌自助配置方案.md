# OEM 品牌自助配置方案（待审批）

状态：**已批 E 并实现。** Logo ≤128 KiB，短边最多 1024 px。  
关联：D12（`docs/04`）、`brand` 表（`docs/05`）、`DESIGN.md` §2.4 / §7 / §9、飞书目录「四类入口与 OEM 品牌配置」。

本文回答两个问题：

1. 现在能不能让「每个租户」自定义 Logo、品牌名、配色？
2. 若不够，按现有产品边界补到什么程度、怎么补？

已吸收的审批意见：

- **必须能上传 Logo 等品牌资源**，不能只填外链。
- **大小必须有约定**：文件体积、像素、宽高比、页面展示尺寸四套数字都写死，服务端校验，超限拒绝。

---

## 1. 结论（先看这里）

**读路径已经能换皮，写路径还不能自助改。上传和尺寸门禁都还没有。**

| 能力 | 现在 | 说明 |
| --- | --- | --- |
| 按域名识别品牌 | 能 | `GET /v1/public/brand` 用 Host 查 `identity_brands` |
| 顶栏显示品牌名 | 能 | 公共站 / 控制台读 `brand.name`，缺省写 TokenHub |
| 顶栏显示 Logo | 半成品 | 前端会渲染 `logo_url`，但没有上传、也没有写接口 |
| 换主色（章的颜色） | 半成品 | 前端把 `theme.brand` / `theme.primary` 写成 CSS 变量；种子数据仍是旧 key |
| OEM 自己改名 / 改色 / 换 Logo | **不能** | 没有 Create/Update Brand；管理页只能读品牌、签发证书 |
| 上传 Logo / Favicon 等资源 | **不能** | 媒体库是任务产物 + 签名过期 URL，不能当品牌资源 |
| 每个终端用户各自一套皮肤 | **不做** | 与已确认的 A/B/C 模型冲突 |

「租户」在本产品里**不是**每个开发者账号，而是 **OEM 渠道（类型 C）绑定的一条 `brand` 记录**。B 分销商继续用平台品牌。终端用户只继承所属渠道的品牌。

审批：**E**。Logo / `logo_dark` 文件体积 **≤ 128 KiB**，短边最多 **1024 px**；其余 kind 仍按下表。

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
- 媒体模块已有本地目录对象存储（`media.Store`），但对象走 HMAC 签名 URL，默认 7 天过期，**不能直接当 Logo**。

### 3.2 缺口（所以还不能说「已经支持」）

1. **没有写品牌的 API。** 现有只有 `GET /admin/brands` 和 `POST /admin/brands/{id}/tls/issue`。
2. **没有品牌资源上传。** `logo_url` 是文本列；没有 kind、像素、体积校验；管理页也没有上传控件。
3. **`theme_json` 契约和实现不一致。**  
   `DESIGN.md` 规定只能写章的颜色（`brand`、`brand_press`、`brand_soft`…）。  
   种子却写成了旧探活灯：`{"primary":"#22d3ee","background":"#020617"}`。  
   前端只吃 `brand`/`primary` 等少数 key，**纸/碳表面、成功/冻结/危险色故意不开放**（这是对的）。
4. **渠道管理员没有品牌页。** `/channel` 看不到自己的 Logo/配色。
5. **浏览器标题写死 TokenHub**（`web/app/layout.tsx` 的 `metadata.title`），OEM 域名打开标签页仍是平台名。
6. **模型/文档按品牌找渠道时写死了 `brd_oem`。** 新建第三条 OEM 品牌时，目录和文档会错绑到官方渠道。这是本次必须顺手修的隐藏债。

### 3.3 产品已经拍过的边界（不要推翻）

摘自 D12 与 `DESIGN.md` §2.4 / §7 / §9：

- OEM 可以配：站点名、Logo、Favicon、默认浅/深/跟随系统、**一枚章的颜色**。
- OEM **不能**改：纸/碳背景、圆角、字号、渐变、成功/冻结/危险/降级色。
- B 渠道用平台品牌，不写自己的 `theme_json`。
- 浅色/深色/跟随系统三档必须保留，OEM 只能建议默认档，不能关掉其中一档。
- 换主色必须过对比度：正文/链字/章上白字不达标则拒绝发布。
- 页面上 Logo 固定高 **24px**（字标旁），标记 20–24px，Favicon 同标记。上传源图必须更大，供视网膜屏，但**展示尺寸不随源图变大**。

---

## 4. 推荐方案（请批「做 / 不做」）

### 4.1 做：把 OEM 换皮补成可运营能力（含上传）

目标：平台管理员能创建品牌并绑到 C 渠道；该渠道的 `channel_admin` 能改自己的名、配色，并**上传** Logo / Favicon（以及下方白名单里的其它品牌资源）；用户打开 OEM 域名立刻看到新皮。

品牌记录仍用 `identity_brands` + `channel_org.brand_id`。品牌资源**另建一张表**，不塞进媒体任务表。

```
访问 Host
    │
    ▼
identity_brands（name / theme_json / 三域名 / 当前资源指针）
    │
    ├── identity_brand_assets（kind / 体积 / 像素 / sha256 / 公开 URL）
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
| B 分销商独立 Logo/配色 | **不做** | B 用平台品牌；要换皮应升级为 C。见 `docs/15` |
| 整站换背景、换字体、换圆角 | 不做 | 破坏清算台气质和可访问性 |
| 一个渠道绑多个品牌、A/B 测试皮肤 | 不做 | P0 无需求 |
| 只填外链、平台不存文件 | **不做** | 已要求必须能上传；外链仅作可选项见 5.5 |
| 任意尺寸、任意文件当 Logo | **不做** | 大小已约定，超限 400 |
| 把品牌文件写进 `media_assets` | 不做 | 媒体默认 7 天过期 + 签名 URL，Logo 必须长期公开 |

---

## 5. 契约与校验

### 5.1 `theme_json` 只允许这些 key

与 `DESIGN.md` §2.4 对齐，写入时丢掉未知字段（不要静默存 `background`）：

```json
{
  "display_name": "Acme API",
  "logo_url": "/v1/public/brand-assets/bas_xxx",
  "favicon_url": "/v1/public/brand-assets/bas_yyy",
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

说明：

- `display_name` 与列 `name`：对外展示用 `name`；`theme_json.display_name` 若传入则同步写 `name`，避免两处打架。
- `logo_url` / `favicon_url`：权威值是品牌表上的列（或由当前 `logo` / `favicon` 资源推导）。JSON 里的副本只为文档契约完整。上传成功后由服务端回填**本站公开路径**，不要让 OEM 手填一个会过期的签名 URL。
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

### 5.3 先分清三种「大小」（给审批人）

上传时容易把三件事混在一起。本方案把它们拆开，**每一项都有硬上限**：

| 名称 | 它量的是什么 | 谁校验 | 超了怎样 |
| --- | --- | --- | --- |
| **文件体积** | 磁盘上有多少字节 | 服务端看 `Content-Length` + 实际读入字节 | `400 asset_too_large` |
| **像素尺寸** | 图片宽×高，单位 px | 服务端解码图片头（SVG 看 viewBox） | `400 asset_dimension` |
| **展示尺寸** | 页面上画多大 | 前端写死，不跟源图走 | 源图再大也只显示约定高度 |

原则：源图给视网膜屏（2×～3×），页面仍按清算台的 24px 标记来画。大图不是为了把顶栏撑高。

### 5.4 品牌资源种类与大小约定（硬门禁）

只允许下面四个 `kind`。没有列出来的（英雄图、背景、自定义字体、视频）一律 `400 asset_kind`。

| kind | 用途 | 允许格式 | 文件体积 | 像素（栅格） | 宽高比 | 页面展示 |
| --- | --- | --- | --- | --- | --- | --- |
| `logo` | 顶栏 / 侧栏标记 | `image/png`、`image/webp`、`image/svg+xml` | **≤ 128 KiB** | 短边 **64–1024 px**；推荐 **256×256** | **1:1 到 4:1**（宽/高） | 高 **24px**，宽随比例、上限 96px；`object-contain`；圆角 6px |
| `logo_dark` | 可选。碳面（深色）下若浅底 Logo 看不清才传 | 同 `logo` | **≤ 128 KiB** | 同 `logo` | 同 `logo` | 同 `logo`；仅 `html.dark` 替换 |
| `favicon` | 浏览器标签、收藏夹 | `image/png`、`image/x-icon`、`image/vnd.microsoft.icon` | **≤ 64 KiB** | **恰好 32×32 或 48×48**（ICO 须含 32×32） | **必须 1:1** | 浏览器默认 16–32px，平台不另画 |
| `og_image` | 可选。链接预览（飞书/iMessage 拉卡片） | `image/png`、`image/jpeg`、`image/webp` | **≤ 512 KiB** | **恰好 1200×630** | **1200:630** | 不进顶栏 |

补充约定（全部服务端执行，前端只是提前提示）：

1. **请求体上限**：整个 multipart 表单 **≤ 600 KiB**（略大于最大单文件，防止旁路字段把内存撑爆）。先看 `Content-Length`，再 `io.LimitReader`，超了立刻断。
2. **扩展名必须和 MIME 一致。** `logo.png` 却声称 `image/svg+xml` → `400 asset_type`。以**解码后的真实格式**为准，不信客户端报的 type。
3. **JPEG 不能当 `logo` / `favicon`。** 有损、无透明，顶栏纸/碳面上会露出脏边。
4. **GIF / APNG / 视频不做。** 标记不能闪。
5. **SVG 额外规则：** 禁止 `<script>`、事件属性（`onclick` 等）、外部实体、`foreignObject`、外部 `xlink:href` / `href`。没有 `viewBox` 或 viewBox 宽高比超出该 kind 的比例 → 拒绝。SVG **不按像素短边检查**（矢量没有 px），但仍受该 kind 的 KiB 上限（Logo 128 KiB）。
6. **不自动缩放、不自动裁切。** 超了请 OEM 自己改好再传。服务端不是修图工具。
7. **每个品牌每个 kind 只保留「当前生效」一份。** 新上传成功后，旧对象删掉（或标记 superseded），避免对象存储堆积。历史以审计 `before_json` 为准，不提供资源回收站。
8. **每个品牌每小时最多上传 20 次。** 防刷。超了 `429 rate_limited`。
9. **一张图只服务一个 kind。** 不能把 1200×630 的 `og_image` 复用成顶栏 Logo。

推荐给 OEM 的出图规格（写在上传控件旁，不当建议、当验收样张）：

- Logo：256×256 PNG，透明底，实心图形，四周留 16px 安全边。
- Favicon：48×48 PNG，透明底，中心图形不要细于 2px。
- 深色 Logo：仅当浅色 Logo 在碳面上对比度不够时才传。
- 分享图：1200×630，纸面底 + 字标 + 一句定位，不要霓虹。

### 5.5 外链还收不收

**主路径是上传。** `logo_url` / `favicon_url` 默认指向本站 `/v1/public/brand-assets/{id}`。

外链只作为**兼容阀**，默认关掉，避免 OEM 指到会过期或带追踪参数的地址：

- 平台管理员可在创建品牌时临时填 `https://` 外链（本地 e2e 允许 `http://localhost`）。
- 渠道管理员界面**不提供外链输入框**，只提供上传。
- 一旦该 kind 上传成功，外链被本站公开 URL 覆盖。
- 若另批「允许外链」，外链仍要过 URL allowlist（禁止 `javascript:`、data URI、私网），且**不豁免尺寸约定**：服务端拉取后须通过同一套体积/像素门禁，失败则拒绝保存。一期不实现外链拉取。

### 5.6 存储：公开前缀，不走媒体签名 URL

媒体任务的对象 7 天过期、下载要 HMAC，适合视频结果，不适合 Logo。

品牌资源：

- 对象 key：`brand/{brand_id}/{kind}/{sha256}.{ext}`
- 复用 `media.Store` 的本地磁盘（P0）或同一 S3 桶的**公开前缀**（生产）；读接口**不验签名、不设过期**。
- 元数据进 `identity_brand_assets`，不进 `media_assets`。
- 公开读：`GET /v1/public/brand-assets/{id}`，按 `Content-Type` 输出字节；缓存头 `Cache-Control: public, max-age=86400, immutable`（key 含 sha256，换图即换 URL）。
- 响应安全头：`X-Content-Type-Options: nosniff`；SVG 再加 `Content-Security-Policy: default-src 'none'; img-src 'self'`，降低 XSS。

建议表结构（实现时用 migration，禁止 AutoMigrate）：

| 列 | 说明 |
| --- | --- |
| `id` | `bas_...` |
| `brand_id` | 外键 |
| `kind` | `logo` / `logo_dark` / `favicon` / `og_image` |
| `object_key` | 存储路径 |
| `content_type` | 解码后的真实 MIME |
| `size_bytes` | 实际字节 |
| `width_px` / `height_px` | 栅格解码结果；SVG 为 viewBox 整数 |
| `sha256` | 去重与 cache bust |
| `status` | `active` / `superseded` |
| `created_by` / `created_at` | 审计 |

`identity_brands` 增加 `favicon_url` 列（现在只有 `logo_url`）。两列由当前 `active` 资源回填。`theme_json` 不再作为文件权威源。

---

## 6. API（建议补这些，不动现有读接口语义）

权限原则：前端藏按钮不是安全边界。渠道管理员只能改**自己渠道的 `brand_id`**。平台管理员可以创建品牌、改任何品牌、把品牌绑到 C 渠道。

| 方法 | 路径 | 谁 | 作用 |
| --- | --- | --- | --- |
| `GET` | `/v1/public/brand` | 匿名 | **已有。** 按 Host 返回公开品牌（含当前 logo/favicon/og URL） |
| `GET` | `/v1/public/brand-assets/{id}` | 匿名 | **新增。** 输出资源字节；未发布/已替换返回 404 |
| `GET` | `/admin/brands` | 平台/运营/技术 | **已有。** 列表 |
| `POST` | `/admin/brands` | `platform_admin` | **新增。** 创建品牌（名、三域名、可选 theme）。需二次确认 |
| `PATCH` | `/admin/brands/{id}` | `platform_admin` | **新增。** 改名/域名/theme。改域名需二次确认 |
| `POST` | `/admin/brands/{id}/assets` | `platform_admin` | **新增。** multipart 上传；字段 `kind` + `file` |
| `GET` | `/channel/brand` | `channel_admin` | **新增。** 读本渠道品牌 + 当前资源 |
| `PATCH` | `/channel/brand` | `channel_admin` 且 type=C | **新增。** 改 name / theme_json。B 渠道 `403 brand_not_customizable` |
| `POST` | `/channel/brand/assets` | `channel_admin` 且 type=C | **新增。** 与管理端同一套体积/像素门禁 |
| `POST` | `/admin/brands/{id}/tls/issue` | 已有 | 不变 |

上传请求：`multipart/form-data`，字段：

- `kind`：必填，四选一
- `file`：必填，单个文件

成功响应示例：

```json
{
  "item": {
    "id": "bas_01",
    "kind": "logo",
    "content_type": "image/png",
    "size_bytes": 18120,
    "width_px": 256,
    "height_px": 256,
    "url": "/v1/public/brand-assets/bas_01"
  }
}
```

失败用稳定错误码，方便表单贴在控件下：

| HTTP | code | 何时 |
| --- | --- | --- |
| 400 | `asset_kind` | kind 不在白名单 |
| 400 | `asset_type` | MIME / 扩展名 / 实解码不一致，或 JPEG 当 Logo |
| 400 | `asset_too_large` | 超过该 kind 的 KiB 上限或表单 600 KiB |
| 400 | `asset_dimension` | 像素或宽高比不在表内 |
| 400 | `asset_svg` | SVG 含脚本/外链/无 viewBox |
| 403 | `brand_not_customizable` | B 渠道或改别人的品牌 |
| 429 | `rate_limited` | 每品牌每小时超过 20 次 |

`POST /admin/channels` 已能带 `brand_id`。创建 C 渠道时：若没带品牌，平台应先 `POST /admin/brands`，再把新 `brand_id` 写进渠道。不要静默复用官方品牌，否则 OEM 用户会看到 TokenHub。

写操作一律：

- 校验 theme 契约 + 对比度；
- 校验资源 kind / 体积 / 像素 / SVG；
- 写 `audit_log`（`brand.update` / `brand.create` / `brand.asset.upload`），带 before/after（after 含 size_bytes 与宽高）；
- 敏感改动（域名）要 `X-Tokenhub-Confirm`。上传本身不二次确认（可回传覆盖），但写审计。

---

## 7. 前端怎么吃

四个入口共用同一套壳，已经按 Host 拉品牌。补齐即可，不要再做第二套皮肤系统。

1. **`themeStyle()` 对齐契约。** 写出 `--brand`、`--brand-press`、`--brand-soft`、`--brand-emphasis`、`--on-brand`；深色模式用 `brand_*_dark`。丢掉对 `background` 的幻想。
2. **`<html>` 上设 favicon 和 `<title>`。** 用 `brand.name`，不要写死 TokenHub。
3. **顶栏 Logo。** 公共站已有；控制台侧栏/顶栏 presently 只用 `BrandMark` 色块，改为：有 `logo_url` 就画图（高 24px），深色优先 `logo_dark`。没有资源时退回钴方标记。
4. **上传控件。** 渠道台 `/channel/brand` 与管理台品牌页：每个 kind 一块「选择文件 / 看预览 / 看当前体积与像素」。控件旁写死数字，例如「Logo：PNG/WebP/SVG，≤128KiB，短边 64–1024px，宽高比 1:1～4:1，页上高 24px」。选文件后**先在浏览器做一遍同样的检查**，不通过不发请求；服务端再查一次。
5. **不做裁切器、不做在线压缩。** 不合格就告诉差多少（「现在 800×200，宽高比 4.0 已到上限；请改成不超过 4:1」）。
6. **平台管理**还可创建品牌、改域名、看对比度失败原因。
7. **B 渠道**品牌页只读：「使用平台品牌」。
8. **种子 theme 迁移。** 官方改为 DESIGN 默认钴蓝；Aurora OEM 改为对比度合格的琥珀金，去掉 `background`。

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
- C 渠道管理员可 PATCH 自己的品牌并上传 `logo`；B 渠道 403；其它渠道的管理员改不到这条品牌。
- `theme_json` 含 `paper` / `success` / 非法 hex → 400。
- 白字叠在浅黄主色上对比度不够 → 400。
- **体积：** 129 KiB 的文件当 `logo` → `400 asset_too_large`；65 KiB 当 `favicon` → 同样拒绝。
- **像素：** 32×32 PNG 当 `logo`（短边小于 64）→ `400 asset_dimension`；1025 px 短边 → 拒绝；800×100（比 8:1）→ 拒绝；33×33 当 `favicon` → 拒绝；1200×629 当 `og_image` → 拒绝。
- **类型：** JPEG 当 `logo` → `400 asset_type`；带 `<script>` 的 SVG → `400 asset_svg`。
- 上传成功后 `GET /v1/public/brand-assets/{id}` 匿名 200，`Content-Type` 正确，无签名参数。
- 同一 kind 二次上传后旧 id 404，品牌 `logo_url` 指向新 id。
- 改品牌 / 上传写审计，after 含 `size_bytes`、宽高。

前端：

- `oem.localhost` 顶栏、标题、favicon、主按钮色来自该品牌；Logo 视觉高度 24px，不因源图 512px 而撑高顶栏。
- 官方域名不受 OEM 改色/换图影响。
- 浅色/深色切换后，链字仍用 `brand_emphasis` / `brand_emphasis_dark`；若有 `logo_dark` 则深色换图。
- 上传控件在超体积、超像素时本地拦截，文案带出约定数字。
- Playwright：渠道品牌页上传合格 Logo 并保存后，公共站刷新可见。

---

## 10. 实现切片（批准后的开工顺序）

不估日历，只按依赖切开，每一刀可单独验收：

1. **契约与校验库**：theme key 白名单、hex、对比度；**资源 kind / KiB / 像素 / 比例 / SVG 消毒**；种子 theme 改成合法值；前端 `themeStyle` 对齐。
2. **存储 + 表**：`identity_brand_assets` migration；`identity_brands.favicon_url`；公开前缀读（不签名）。
3. **写 API**：品牌 CRUD + `POST .../assets` + 审计 + 确认头 + 每小时 20 次。
4. **按品牌找渠道**：修公开模型/文档的硬编码。
5. **管理面 + 渠道面**：表单、上传控件（旁注写死尺寸）、标题/favicon、控制台也吃 Logo。
6. **e2e / Playwright**（含超体积/超像素拒绝）；更新 `docs/06`、`docs/09`、飞书目录该条为「已实现待验收」。

公网 Custom Hostname（飞书目录独立条目）仍另批，不绑在这次换皮里。

---

## 11. 审批选项

请直接回复编号，或改条件后批：

| 编号 | 选项 | 建议 |
| --- | --- | --- |
| **A** | C 渠道可改名 / **上传** Logo 等白名单资源 / 章颜色；尺寸按 §5.4 硬门禁；B 不换皮；终端用户不换皮 | **推荐（已含上传）** |
| **B** | 只要平台管理员能改和上传，渠道自己不能改 | 能上线，但 OEM 每次换 Logo 都要工单 |
| **C** | 还要给 B 渠道独立品牌 | B 跟 C 的池与支付，品牌仍按 C；见 `docs/15` |
| **D** | 还要每个终端用户/工作区换皮 | 不建议 |
| **E** | 改 §5.4 的数字（例如 Logo 改成 ≤128KiB，或允许 1024px） | 可以，请直接给出新数字 |
| **F** | 暂缓 | 不能支撑真实 OEM 签约 |
| **G** | 一期仍允许渠道手填外链（上传同时保留输入框） | 不推荐；和外链拉取/SSRF 搅在一起 |

已批 **E**：按 A 的范围实现，尺寸用 §5.4（Logo ≤128 KiB、短边 ≤1024 px）。
