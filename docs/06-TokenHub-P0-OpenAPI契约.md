# TokenHub P0 OpenAPI 契约骨架

本文定义 P0 对外 API 的稳定边界。具体 schema 可据此生成 OpenAPI 3.1 文件；所有接口统一返回 `request_id`，写操作支持 `Idempotency-Key`。

## 1. 通用约定

- API Base：`https://{api-domain}/v1`
- 鉴权：`Authorization: Bearer <tokenhub_api_key>`
- 请求头：`X-Request-ID` 可选；未提供时服务端生成。
- 写操作：`Idempotency-Key` 必须是客户端生成的稳定值，服务端保存 24 小时以上。文本网关把该键与请求体哈希写入 Redis（TTL 24h）；相同键不同请求体返回 `409 idempotency_conflict`。
- 分页：`limit`（默认 20，最大 100）+ `cursor`，返回 `next_cursor`。
- 错误结构：

```json
{
  "error": {
    "code": "insufficient_balance",
    "message": "余额不足",
    "param": null,
    "request_id": "req_01...",
    "retryable": false
  }
}
```

标准错误码：`invalid_request`、`authentication_error`、`permission_denied`、`model_not_allowed`、`insufficient_balance`、`rate_limit_exceeded`、`provider_unavailable`、`idempotency_conflict`、`usage_pending_reconciliation`、`media_job_not_ready`。

## 2. 模型与 Provider

### `GET /v1/models`

返回客户可见模型、厂商、能力、可用 Provider 状态和平台销售价；不返回上游密钥、内部成本和 Provider 独立价格。

### `GET /v1/models/{model}`

返回单个模型详情、支持参数、媒体规格、可用 Provider 和弃用状态。

请求可选 Provider 路由参数：

- `provider.only`: 仅允许指定 Provider 列表；
- `provider.ignore`: 排除指定 Provider；
- `provider.order`: 指定 Provider 优先级。

## 3. 文本/多模态推理

### `POST /v1/chat/completions`

兼容 OpenAI Chat Completions。必须明确处理 `stream`、tool calls、JSON schema、vision、reasoning 和 usage；不支持参数返回结构化 4xx。

### `POST /v1/responses`

兼容 OpenAI Responses，响应中保留 `id`、`model`、`usage`、`output` 和 `request_id`。

### `POST /v1/messages`

兼容 Anthropic Messages，支持 `stream`、system、tools、vision 和 usage 映射。

统一行为：

- 请求前预授权；余额不足不调用上游；
- 流式响应开始后不切 Provider；
- fallback 只在未向客户端返回正文前执行；
- 客户只产生一个最终扣费事件，但每次上游 attempt 都记录成本和错误。
- 沙箱头 `X-Tokenhub-Sandbox-Mode`：`fixed`（默认 8/4/12）、`content`（按 prompt/回复字符数，prompt 下限 8、completion 下限 4）、`reasoning`（content + `reasoning_tokens=3`）、`omit`（空 usage，进待对账）。`X-Tokenhub-Omit-Usage: 1` 等同 `omit`。Bifrost metadata 透传 `request_id` / `api_key_id` / `user_id` / `channel_org_id`。

## 4. 视频/图像异步任务

### `POST /v1/videos`

创建 Seedance 类视频任务，返回 `202 Accepted`：

```json
{
  "id": "vid_01...",
  "object": "video",
  "status": "queued",
  "model": "bytedance/seedance-1.0",
  "created_at": 1788000000,
  "status_url": "/v1/videos/vid_01..."
}
```

请求支持：文生、图生、首帧/首尾帧、参考图/视频/音频、时长、分辨率、宽高比、帧率、原生音频、编辑/延长（上游声明支持时）、`callback_url` 和客户端幂等键。

字段：`task_type`（或别名 `mode`）为 `t2v`（默认）/`i2v`/`first_frame`/`first_last_frame`/`reference`/`extend`/`edit`；`duration`、`resolution`、`aspect_ratio`、`fps`、`generate_audio`、`images`、`first_frame`、`last_frame`、`reference_video`、`reference_audio`、`source_job_id`。`i2v`/`first_frame` 要图；`first_last_frame` 要首+尾帧；`reference` 至少一种参考；`extend`/`edit` 要本用户已完成视频的 `source_job_id`。查询响应回带这些参数，不含 prompt。

### `GET /v1/videos/{id}`

返回 `queued`、`in_progress`、`completed`、`failed`、`cancelled`、`expired`、进度、失败原因、usage 摘要、最终 Provider，以及 `task_type` 与媒体参数。

### `GET /v1/videos/{id}/content`

返回短期签名 URL 或媒体流；默认结果保留 7 天。

### `POST /v1/images/generations` 与 `POST /v1/images/edits`

与视频共用媒体任务状态机、预授权和签名下载；返回 `202` 与任务 ID。`generations` 默认 `task_type=generate`；`edits` 默认 `edit` 且必须带 `images`。

### `POST /v1/videos/{id}/extend`

对调用方已完成的视频做延长；等价于 `POST /v1/videos` 且 `task_type=extend`、`source_job_id` 取路径中的 id。

### `POST /v1/videos/{id}/cancel`

请求取消；已提交上游任务只执行可用的上游取消接口，不重复创建任务。

### 回调 `POST {callback_url}` 与 `POST /v1/media/callbacks`

上游完成后可回调客户 `callback_url`，沙箱与自建上游使用平台入口 `POST /v1/media/callbacks`。出站与入站均校验/签发 `X-Tokenhub-Signature`（HMAC-SHA256，`event_id|job_id`）。入站按 `event_id` 幂等；重复事件必须返回 2xx 且不得重复结算。出站失败按退避重试，超过上限记死信，不阻断创建。未完成的任务在重试或状态轮询时会继续落状态和结算。

## 5. 用户、Key、套餐与余额

- `GET /v1/public/docs-context`：品牌 Base URL、模型白名单、curl/Python/Node/Messages/视频示例（占位 `$TOKENHUB_API_KEY`）以及错误码/限流/回调说明
- `GET /v1/public/models`：按域名品牌列出已发布模型卡片（id/vendor/display_name/capabilities/kind），不含 Provider 路由。查询参数 `vendor`、`kind`、`q`、`id`、`limit` 在服务端筛选；响应带 `total` 与 `facets.kinds` / `facets.vendors`（类型分面不含当前 kind，厂商分面不含当前 vendor）。前端目录不得再维护一份本地模型快照。
- `GET /v1/me`
- `PATCH /v1/me`：更新 `display_name` 与 `locale`（zh/en/ja）；不能改渠道归属
- `POST /v1/me/password`：校验当前密码后改密
- `GET /v1/me/balance`
- `GET /v1/me/usage`：当前用户账本。查询 `api_key_id`、`public_model_id`、`limit`。条目含 `api_key_id`、`prompt_tokens`、`completion_tokens`、`reasoning_tokens`、金额。另返回 `keys` / `models`（该用户按 API Key / 模型的 DimMoney 汇总）。用 API Key 鉴权时只返回这把 Key 的明细。
- `GET /v1/me/ledger`
- `GET /v1/plans`：公共站已发布的平台套餐；
- `GET /v1/me/plans`：当前渠道可见的已发布套餐；
- `GET /v1/me/entitlements`：赠送与套餐权益账户；
- `POST /v1/me/subscriptions`
- `GET /v1/me/subscriptions`
- `POST /v1/me/subscriptions/{id}/cancel`
- `GET /v1/me/api-keys`：列表回带 `allowlist` 与 `rpm_limit`；完整 Key 仅创建者可见。
- `POST /v1/me/api-keys`：接受 `name`、`allowlist`、`rpm_limit`、`concurrency_limit`。空白名单不限制模型；非空时聊天或列模型不在名单内返回 `403 model_not_allowed`。RPM 默认 60，并发默认 5；占满并发槽返回 `429 rate_limited`。
- `POST /v1/me/api-keys/{id}/rotate`
- `POST /v1/me/api-keys/{id}/disable`
- `POST /v1/me/api-keys/{id}/copy`：复制完整 Key，只写审计不改密文

完整 API Key 只返回给创建者；查看、复制、轮换、禁用、过期均写审计日志。

## 6. 充值与支付

- `POST /v1/topups`：创建充值订单；
- `GET /v1/topups/{id}`：查询订单；
- `POST /v1/topups/{id}/refund`：按权限申请退款；
- `POST /v1/payments/orders`：创建钱包充值支付单。`channel_org_id` 由登录用户归属写入；可传 `pay_major` 由服务端按插件币种报价。
- `GET /v1/payments/checkout`：按用户 `channel_org_id` 返回已开通通道。空列表不要写成「支付功能未启用」。
- `GET /v1/payments/quote`：应付 / 手续费 / 钱包入账 / 发放额度。BPS 只读。
- `GET /v1/payments/orders/{id}`：查询支付单；
- `POST /v1/payments/{adapter}/webhook`：支付适配器回调。插件 `ParseWebhook` 验签；沙箱 HMAC 为 `event_id|order_id|status`，按 `external_event_id` 幂等。
- 渠道收款：`GET /channel/payments/overview|adapters|instances|settings|orders`；`POST /channel/payments/instances`；`PATCH /channel/payments/instances/{id}`（改凭证需确认）；`POST .../test`、`POST .../go-live`（确认）。通道卡来自支付插件注册表，新增本地支付只需注册 Adapter。
- 平台：`GET /admin/channels/{id}/payments` 就绪灯（无密钥）；`POST .../disable` 紧急停用；`GET/PATCH /admin/payment-adapters` 插件总开关。
- `POST /v1/topups/redeem`：兑换码入账（M3 沙箱码 `THE2E` / `THCREDIT10`）。
- `POST /admin/topups/{id}/confirm`：财务确认人工充值。
- `POST /admin/refunds`：按 `request_id` 或 `topup_id` 退款并冲正佣金。
- `POST /admin/usage/replay`：幂等回放 usage / 完成待对账。缺 `X-Tokenhub-Confirm` 返回 `409 confirm_required`，并写审计 `billing.usage.replay`。管理页 `/admin/usage` 可按 request_id 补真实 Token。
- `GET /admin/billing/report`：收入、成本、佣金负债、待对账数量。
- `GET /admin/billing/export`：对账 CSV（收入/成本/佣金/毛利/待对账）。
- `GET /admin/usage?format=csv`：用量明细导出，含 `api_key_id`、`user_id`、token 与金额。查询 `user_id`、`api_key_id`、`channel_id`、`public_model_id`。
- `POST /v1/me/api-keys/{id}/expire`：设置过期时间；过期后鉴权失败。
- `GET /v1/me/media`：当前用户媒体任务（kind/status 筛选，不含他人数据）。
- `POST /v1/videos` 与图像创建接口同时接受用户会话或 API Key，方便控制台直接提交任务。
- `GET /admin/media`：管理端媒体任务列表；`?format=csv` 导出且不含 prompt。
- `GET /v1/public/tls-check?domain=`：Caddy on-demand TLS 询问；仅已登记品牌域名返回 200。
- `GET /admin/brands`、`POST /admin/brands/{id}/tls/issue`：OEM CNAME 目标与证书状态（`tls_issuer`/`tls_directory`/`tls_expires_at`）；签发需二次确认。空目录或 `.localhost` 只标沙箱 `issued`。配置 `TOKENHUB_ACME_DIRECTORY` 后，公网形态域名走 RFC 8555（本地用 Pebble）；失败 `502 provider_unavailable`。`GET /.well-known/acme-challenge/{token}` 承接 HTTP-01。公网 Let's Encrypt 仍要真实 DNS 与边缘节点。管理页 `/admin/settings`「OEM 证书」可读取并签发。
- `POST /admin/brands`、`GET/PATCH /admin/brands/{id}`、`POST /admin/brands/{id}/assets`：创建/改品牌、上传 Logo 等资源。`GET/PATCH /channel/brand`、`POST /channel/brand/assets`：C 渠道自助换皮；B 渠道 `403 brand_not_customizable`。`GET /v1/public/brand-assets/{id}`：公开读当前资源，无签名、不过期。Logo ≤128KiB，短边 64–1024px；Favicon ≤64KiB 且 32/48 方图；超限 `400 asset_*`。管理页 `/admin/brands`，渠道页 `/channel/brand`。
- `POST /admin/commissions/recalc`：按价格快照重算佣金；管理页 `/admin/commission`「佣金重算」可操作。
- `POST /admin/price-books`：发布新价格版本，不影响历史账单。

支付回调必须验签、记录原始事件、按外部事件 ID 幂等，并在确认 `paid` 后发放余额/权益。

## 7. 管理后台 API（P0）

- Provider：`GET/POST /admin/providers`、`GET/PATCH /admin/providers/{id}`（`id` 可为内部 id 或 slug）、`POST /admin/providers/{id}/health-check`（不会计费、不强制确认；管理列表 `/admin/providers` 每行可探测）。`PATCH` 改状态/名称/适配器/上游地址/超时（二次确认；不要改种子 echo/gemini）。`POST /admin/providers/{id}/credentials` 凭据轮换（二次确认，响应只回 `credential_ref`，明文不回显）。管理列表只展示目录和新建；改状态、凭据轮换、账号池、已挂模型在详情页 `/admin/providers/{id}`；
- 上游账号池：`GET/POST /admin/providers/{id}/accounts`、`PATCH /admin/providers/{id}/accounts/{aid}`；列表只回指纹，不回密文；冷却/失效账号不参与路由；详情页「账号池」可读取、添加、冷却、停用；
- 模型：`GET/POST /admin/models`、`GET/PATCH /admin/models/{id}`（`id` 为 public_id，可含斜杠，如 `tokenhub/echo-1`）、`POST /admin/models/attach` 挂载 Provider 映射；`GET /admin/models?status=&sync_state=&q=` 先筛选再分页，`sync_state=draft` 为待审核队列（空 sync_state 且 `status=draft` 也算待审核）；`POST /admin/providers/{id}/sync` 同步结果只进入 `draft`，不自动审核（同步入口仅提供商详情，见 `docs/13`）；`POST /admin/models` 手工创建永远是 `draft`（请求里的 `status` 会被忽略）；`POST /admin/models/review|publish|deprecate`（body 带 `public_id`）分别通过/拒绝、发布、弃用，不删除历史映射和价格版本。发布要求 `sync_state=reviewed`，已拒绝不能发布；创建人（`created_by_user_id`）不能审核或发布自己建的模型（409）。`PATCH` 只改展示名、厂商和 `capabilities`（需二次确认），不能靠 PATCH 直接上架。管理页 `/admin/models`：Tab「目录 | 审核」；目录列表极简；审核分待审核 / 已通过待发布 / 已拒绝；挂载、弃用、发布销售价在 `/admin/models/{public_id}`；不要改 `tokenhub/echo-1`；
- 路由：`GET/POST/PATCH /admin/routes`；创建和改策略需二次确认；管理页 `/admin/routes` 按 vendor 折叠、主键仍是公开模型，可创建路由组并改 `priority`/`weight`/`price`/`health`；厂商默认以模板批量套用（见 `docs/13`）；
- 渠道/代理：`GET/POST /admin/channels`、`GET/PATCH /admin/channels/{id}`、`GET/PATCH /admin/channels/{id}/models`、`GET /channel/models`；渠道组织是租户边界（A/B/C），代理商和个人推广员是租户内推广角色；A 可建 B/C，B 无下属，C 只建 B；创建和改状态需二次确认；管理页 `/admin/channels` 先列表（渠道/代理商/推广员分栏），点进详情可编辑保存；列表提供新建渠道；新建渠道会复制官方已启用模型白名单；平台可从目录勾选授权/撤销租户模型（`PATCH` 需二次确认，未知 `public_id` 返回 400）；租户不能自建提供商或模型，渠道控制台只读本渠道已授权模型；`disabled` 后聊天/媒体返回 `403 channel_disabled`，`GET /v1/me/balance` 与 usage 仍可读；
- 用户治理：`GET /admin/users`、`POST /admin/users/{id}/ban|unban`、`POST /admin/users/{id}/attribution`；封禁后登录和旧 API Key 403，未结算佣金进入 `held`；改归因与封禁需二次确认并写审计；
- 管理员 2FA：`GET /admin/me` 回当前角色；`GET /admin/me/2fa`、`POST /admin/me/2fa/setup|enable|disable`；启用后敏感写操作还要 `X-Tokenhub-TOTP`；管理页 `/admin/settings` 可读取/绑定/启用/关闭；关闭需确认，启用后再关闭还要 TOTP；不要在共享管理员上留下 `enabled`；
- 用户 API Key（D38）：平台**不再**管理具体用户 Key。目标契约为渠道范围 `GET /channel/api-keys`（prefix/状态/最近使用等，无密文）、`POST /channel/api-keys/{id}/disable`（二次确认，仅本渠道用户）。终端用户仍用 `/v1/me/api-keys`。既有 `GET /admin/api-keys` 与 `POST /admin/api-keys/{id}/disable`、管理页 `/admin/keys` 视为待下线兼容面，实现 PR 删除前勿用于新产品验收；
- 推广：`GET/POST /admin/acquisition-roles`、`GET/PATCH /admin/acquisition-roles/{id}`、`GET/POST /admin/promotion-codes`、`GET/POST /channel/promotion-codes`；`type=agent|promoter` 分列表（历史 `kol` 仍可筛 `kol_l1`/`kol_l2`）；创建角色和推广码、改状态需二次确认并写审计；管理页 `/admin/channels` 分栏管理代理商/推广员，`/admin/promos` 管推广码；
- 分销只读：`GET /v1/partner/me|users|commissions|settlements|export`（按邀请链过滤，邮箱脱敏，不含 prompt）；代理商看授权范围内用户，个人推广员看自己发展的下线；计佣两跳见 `docs/15`；
- 佣金：`GET /admin/commissions`、`GET/PATCH /admin/commission-policy`（改 BPS/冻结天数需二次确认）、`GET/PATCH /admin/eligibility-rules`（平台达线：累计消费 / 单笔充值，需确认）、`GET/PATCH /channel/eligibility-rules`（仅 C 可写，B 读平台规则）、`POST /admin/commissions/recalc`（按价格快照重算需确认）、`POST /admin/commissions/unfreeze`（解冻需确认并写审计）、`POST /admin/commissions/settle`、`POST /admin/settlements/{id}/payout`；管理页 `/admin/commission` 可重算、手工解冻、生成结算单和人工打款；
- 渠道额度：`GET /channel/quota`、`GET /channel/allocations`、`GET /admin/channel-quotas/{channel_id}`、`POST /admin/channel-quotas/grant`（平台向 B/C 进货）、`POST /channel/quotas/grant`（仅 C 向其下属 B 划拨，扣 C 加 B，需确认）、`GET/PATCH /admin/channel-quotas/{channel_id}/issue-rule`；`quota` 含 `issued_minor`/`consumed_minor`/`allocation_count`/`issue_ratio_bps`；换算比默认 `10000` BPS = 1:1，平台/财务可改（需二次确认），B/C 不能改换算比；用户充值从**所属渠道自己的池**发放；渠道额度不足返回 `402 insufficient_quota`；
- 渠道运营：`GET /channel/users`、`GET/POST /channel/plans`（渠道自建套餐，归属强制为本渠道；低于 1 USD 进 `pending_review`；渠道控制台「创建渠道套餐」）、`GET /channel/usage`（合计 + `keys`/`models`/`items`，可按 `api_key_id` 筛，不含 prompt）、`GET /channel/attribution`、`GET /channel/settlements`、`GET /channel/commissions`；渠道 API Key 列表与禁用见上条；
- 套餐：`GET/POST/PATCH /admin/plans`、`POST /admin/plans/{id}/review`、发布、下架；管理页 `/admin/plans` 可审核、创建平台套餐，并用 `PATCH` 把套餐标成 `archived`（不要下架 `pln_echo_month`）；`POST /admin/subscriptions/{id}/force-period-end` 与 `POST /admin/subscriptions/process-renewals` 只在沙箱拨时钟/扫续费（生产禁止；管理页「续费扫描」）；
- 价格书 API：`GET/POST /admin/price-books`（新版本不改历史账单；管理面改价入口在模型详情）；
- 权益：`POST /admin/entitlements/bonus`（手工赠送需二次确认）；管理页 `/admin/billing` 可退消费账单、确认/退充值和赠送额度；
- 支付：`GET /admin/payments`、`POST /admin/payments/{id}/confirm`、`POST /admin/payments/{id}/refund`；
- 财务：充值、退款、额度调整、佣金结算和对账；
- 观测：`GET /admin/metrics`（`dimension=provider|model|channel|user|api_key`）、`GET /admin/metrics/series`、`GET /admin/metrics/daily?format=csv`、`GET /admin/ops/dashboard`（`dimensions` 含 model / api_key / channel / user / provider / agent）、`GET /admin/ops/alerts`、`POST /admin/ops/alerts/evaluate`、`GET/PATCH /admin/ops/thresholds`、`GET /admin/ops/runbooks`；看板 totals 含错误码分布、超时、Token/媒体用量、预授权失败和回调 P95；
- 加固：`POST /admin/ops/backup-drill`、`GET/POST /admin/ops/canary`、`POST /admin/ops/circuit/{id}`、`POST /admin/ops/drills/payment|media|tls`；TLS 演练验证已知 OEM 域名 200、未知域名 404、沙箱 `issued`（`.localhost` 不走 ACME）；`scripts/e2e_acme.sh` 对 Pebble 做 RFC 8555 真签发；公网 Let's Encrypt 仍由边缘节点对真实 DNS 签发，本仓库不把「已对公网签发」标完成；健康探测、熔断、灰度、备份演练与支付/媒体/TLS 演练不强制二次确认（技术值班 e2e 不带头）；管理页 `/admin/settings`「运维开关」可探测、打开/复位熔断、读写灰度，「备份演练」记录 RPO 15 / RTO 60，「异常演练」可跑支付/媒体/TLS；
- 审计检索：`GET /admin/audit-logs?action=&resource_type=&q=`；`GET /admin/outbox/stats` 读 `pending`/`published`/`failed`；`POST /admin/audit-probes` 沙箱写 `audit.probe`（生产 403，不强制确认）；管理页 `/admin/audit` 可读取 Outbox 并写入探测。

所有管理接口按角色授权；退款、手工加款、佣金调整、凭据修改、价格底线修改、usage 回放必须二次确认：请求头 `X-Tokenhub-Confirm: 1`（或 `confirm=1`），并记录 before/after 快照。缺少确认返回 `409 confirm_required`。健康探测、熔断与灰度设置不要求确认头。
