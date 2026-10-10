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

返回客户可见模型、厂商、真实协议/预算能力、安全服务状态和有效品牌终端价；不返回提供商、上游路由、管理人员或内部成本。

### `GET /v1/models/{model}`

返回有权单个模型详情、支持参数、媒体规格、安全服务状态和弃用状态。

公开请求禁止 provider、router、route、账号等上游控制项，包括嵌套和其他大小写/协议边界的变体；返回结构化 4xx。选路只由有权内部配置决定。

## 3. 文本/多模态推理

### `POST /v1/chat/completions`

兼容 OpenAI Chat Completions。必须明确处理 `stream`、tool calls、JSON schema、vision、reasoning 和 usage；不支持参数返回结构化 4xx。

### `POST /v1/responses`

实现 OpenAI Responses 子集；支持范围按实际模型 supported_endpoints 与文档生成，不能声称完整 Responses 或 Codex 兼容。

### `POST /v1/messages`

实现 Anthropic Messages 子集，已支持的输入/输出与流式类型按实际契约验证；未实现参数明确拒绝，不声称完整 Claude Code 兼容。

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

返回短期 **S3 预签名** URL（Compose/CI 无云 Key 时走 MinIO）；默认结果保留 7 天。响应带只读 `storage`（`source=s3|minio|unavailable`，`label` 为 `S3` 或 `存储不可用`）。缺桶或存储失败返回 `503 store_unavailable`，**禁止**回退本地盘并报告成功。`GET /healthz`、`GET /v1/me/media`、`GET /channel/brand` 同样回带 `storage`。

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
- `GET /v1/me`：当前会话用户。`login_methods` 只来自真实列（有 `password_hash` → `password`，有 `google_sub` → `google`），缺显示名/邮箱保持空串
- `PATCH /v1/me`：更新 `display_name` 与 `locale`（zh/en/ja）；不能改渠道归属
- `POST /v1/me/password`：校验当前密码后改密
- `POST /v1/auth/logout`：吊销当前会话令牌并清除 HttpOnly cookie
- `GET /v1/me/balance`：钱包视图。用户台顶栏余额钉 `balance.available`（可用 USD 字符串，对应 `available_minor`），失败不得写成假 `$0.00`
- `GET /v1/me/usage`：当前用户账本。查询 `api_key_id`、`public_model_id`、`state`（仅 `confirmed` / `pending_reconciliation` / `voided`）、`cursor`、`limit`。条目含 `api_key_id`、`prompt_tokens`、`completion_tokens`、`reasoning_tokens`、金额。另返回 `keys` / `models`（该用户按 API Key / 模型的 DimMoney 汇总）。用 API Key 鉴权时只返回这把 Key 的明细。不得把网关失败写成 `state=failed`。
- `GET /v1/me/requests`：当前用户可见的网关请求回单。查询 `result`（`succeeded` / `failed` / `started`）、`billing_state`（账务三态）、`api_key_id`、`public_model_id`、`from`、`to`、`limit`。`from`/`to` 为半开区间（`>= from` 且 `< to`）。RFC3339 按瞬间解析；仅 `YYYY-MM-DD` 时按 UTC 自然日，`to` 取次日 00:00（不含）。无效时间或起点不早于终点返回 400。任务用量接口支持明确time_zone；页面固定Asia/Shanghai，以展示时区解析日期，不依赖运行机器时区。条目分开展示 `result`、`billing_state`、`customer_amount_minor`、`error_code`。始终按会话用户（或 API Key 所属用户+该 Key）过滤，不能读他人请求。
- `GET /v1/me/usage/summary`：同一授权/日期/时区过滤下全量汇总，含total/daily/keys/models/facets，不按明细首屏估算。管理对应`/admin/usage/summary`与`/channel/usage/summary`；请求详情`.../requests/{id}`按原对象鉴权，普通面隐藏提供商/路由和成本。
- `GET /v1/me/ledger`
- `GET /v1/plans`：公共站已发布的平台套餐；
- `GET /v1/me/plans`：当前渠道可见的已发布套餐；
- `GET /v1/me/entitlements`：赠送与套餐权益账户；
- `POST /v1/me/subscriptions`
- `GET /v1/me/subscriptions`
- `POST /v1/me/subscriptions/{id}/cancel`
- `GET /v1/me/api-keys`：列表回带 `allowlist` 与 `rpm_limit`；完整 Key 仅创建者可见。
- `POST /v1/me/api-keys`：接受 `name`、`model_mode`、`allowlist`、`budget_limit_minor`、`expires_at`，RPM/并发为高级项。创建持久operation_id复用原操作。`model_mode=all`使用所有可用模型，selected必须有非空白名单；`PATCH /v1/me/api-keys/{id}`编辑限制，同样严格本人所有。USD累计预算原子预留/结算/释放/冲正，轮换不重置；有限额Key按合理费用预估准入，不因无法证明绝对上界而排除兼容模型；实际用量齐全就按实结算，可使Key超限及账户负余额，缺价格/计量仍明确拒绝。模型与品牌授权、有效期和账户余额独立生效。
- `POST /v1/me/api-keys/{id}/rotate`
- `POST /v1/me/api-keys/{id}/disable`
- `POST /v1/me/api-keys/{id}/copy`：复制完整 Key，只写审计不改密文

完整 API Key 只返回给创建者；查看、复制、轮换、禁用、过期均写审计日志。

## 6. 充值与支付

- `POST /v1/topups`：旧创建入口410；新充值使用持久原操作的Payment订单；
- `GET /v1/topups/{id}`：查询订单；
- `POST /v1/topups/{id}/refund`：旧退款入口410；管理topup读取也410，必须通过原支付订单校验收款主体和实际退款；
- `POST /v1/payments/orders`：创建钱包充值支付单。`channel_org_id` 由登录用户归属写入；可传 `pay_major` 由服务端按插件币种报价。
- `GET /v1/payments/checkout`：按用户 `channel_org_id` 返回已开通通道。空列表不要写成「支付功能未启用」。
- `GET /v1/payments/quote`：应付 / 手续费 / 钱包入账 / 发放额度。BPS 只读。
- `GET /v1/payments/orders/{id}`：查询支付单；
- `POST /v1/payments/{adapter}/webhook`：支付适配器回调。插件 `ParseWebhook` 验签；沙箱 HMAC 为 `event_id|order_id|status`，按 `external_event_id` 幂等。同一事件重放时若订单已 paid 但尚未 `fulfilled_at`，会再履约。
- 渠道收款：`GET /channel/payments/overview|adapters|instances|settings|orders`；`POST /channel/payments/instances`；`PATCH /channel/payments/instances/{id}`（改凭证需确认）；`POST .../test`、`POST .../go-live`（确认）。通道卡来自支付插件注册表，新增本地支付只需注册 Adapter。
- 平台：`GET /admin/channels/{id}/payments` 就绪灯（无密钥）；`POST .../disable` 紧急停用；`GET/PATCH /admin/payment-adapters` 插件总开关。
- `POST /v1/topups/redeem`：兑换码入账（M3 沙箱码 `THE2E` / `THCREDIT10`）。
- `POST /admin/topups/{id}/confirm`：旧入口410；通过原Payment线下订单登记实际收款。
- `POST /admin/refunds`：仅按原`request_id`消费账单冲正；`topup_id`兼容写410，充值退款必须原Payment订单。
- `POST /admin/usage/replay`：幂等回放 usage / 完成待对账。缺 `X-Tokenhub-Confirm` 返回 `409 confirm_required`，并写审计 `billing.usage.replay`。兼容入口只按原请求冻结事实恢复；明确真实用量核对在`/admin/requests/{id}/reconcile`，页面为原请求详情。OEM只读/报待核对，不新增释放预授权或补用量写权。
- `GET /admin/usage/pending`：待对账工作队列。`status`（默认 `pending_reconciliation`，`voided` / `all`）、`from` / `to`、以及 API Key / 模型 / 渠道筛选。管理页 `/admin/reconciliation`。
- `GET /v1/me/reconciliation`：用户台 W-meter ③。三桶（`available` / `reserved` / `commission_available`）+ 同一窗口 usage 合计 + 差异表。只问 TokenHub billing（Balance / QueryUsage / ListChargesByRequest / ListLedger）。
- `POST /v1/me/reconciliation/flag`：把差异行送进 `pending_reconciliation`。匹配行 `409`。缺 `X-Tokenhub-Confirm` 返回 `409 confirm_required`。**禁止估算扣款**。
- `GET /channel/reconciliation`、`POST /channel/reconciliation/flag`：渠道台同构页；桶来自 `ChannelQuota`（可提现无账本口径时为 0）。渠道管理员 / 财务可写 flag；运营只读。
- `GET /admin/usage/pending/{id}`：用量缺口详情（usage id 或 request_id）。
- `POST /admin/usage/pending/resolve`：单条或批量「标记已解」——作废 pending usage 并释放预授权，**禁止估算扣款**。缺 `X-Tokenhub-Confirm` 返回 `409 confirm_required`，写审计 `billing.usage.resolve`。已结算账单拒绝。
- `GET /admin/billing/report`：收入、成本、佣金负债、待对账数量。
- `GET /admin/margin`：管理台 W-meter ④「成本/毛利」。同一窗口 attempt 成本合计 / 售价合计 / 毛利合计；明细钉 `attempt_id`，成本源固定 `TokenHub`，四维单价只读快照。可用 `request_id` / `channel_id` / `public_model_id` / `from` / `to` 收窄窗口。缺 attempt 成本不进明细、计入 `pending_count`，禁止估算。只问 TokenHub billing（usage + `billing_cost_entries`）。
- `POST /admin/margin/corrections`：补成本 / 调毛利更正票。缺 `X-Tokenhub-Confirm` 返回 `409 confirm_required`。禁止带 `amount_minor` 估算写入成本。
- `GET /admin/billing/export`：对账 CSV（收入/成本/佣金/毛利/待对账）。
- `GET /admin/usage?format=csv`：用量明细导出，含 `api_key_id`、`user_id`、token 与金额。查询 `user_id`、`api_key_id`、`channel_id`、`public_model_id`、`state`、`from`、`to`。
- `POST /v1/me/api-keys/{id}/expire`：设置过期时间；过期后鉴权失败。
- `GET /v1/me/media`：当前用户媒体任务（kind/status 筛选，不含他人数据）。
- `POST /v1/videos` 与图像创建接口同时接受用户会话或 API Key，方便控制台直接提交任务。
- `GET /admin/media`：管理端媒体任务列表；`?format=csv` 导出且不含 prompt。
- `GET /v1/public/tls-check?domain=`：Caddy on-demand TLS 询问；仅已登记品牌域名返回 200。
- `GET /admin/brands`、`POST /admin/brands/{id}/tls/issue`：OEM CNAME 目标与证书状态（`tls_issuer`/`tls_directory`/`tls_expires_at`）；签发需二次确认。空目录或 `.localhost` 只标沙箱 `issued`。配齐 `TOKENHUB_CLOUDFLARE_API_TOKEN` + `ZONE_ID` 后，公网形态域名登记 Cloudflare Custom Hostname（`tls_issuer=cloudflare`，证书未 active 时 `tls_status=pending`）；失败 `502 provider_unavailable`。未登记域名 `GET /v1/public/tls-check` 仍 404。配置 `TOKENHUB_ACME_DIRECTORY` 后本地仍可对公网形态域名走 RFC 8555（Pebble）；**不自建公网 Let's Encrypt**。`GET /.well-known/acme-challenge/{token}` 承接 HTTP-01。管理页 `/admin/settings`「OEM 证书」可读取并签发。
- `POST /admin/brands`、`GET/PATCH /admin/brands/{id}`、`POST /admin/brands/{id}/assets`：创建/改品牌、上传 Logo 等资源。`GET/PATCH /channel/brand`、`POST /channel/brand/assets`：OEM自助品牌配置；渠道 `403 brand_not_customizable`。`GET /v1/public/brand-assets/{id}`：公开读当前资源，无签名、不过期。Logo ≤128KiB，短边 64–1024px；Favicon ≤64KiB 且 32/48 方图；超限 `400 asset_*`。管理页 `/admin/brands`，渠道页 `/channel/brand`。
- `POST /admin/commissions/recalc`：按价格快照重算佣金；管理页 `/admin/commission`「佣金重算」可操作。
- `POST /admin/price-books`：发布新价格版本，不影响历史账单。

支付回调必须验签、记录原始事件、按外部事件 ID 幂等，并在确认 `paid` 后发放余额/权益。

## 7. 管理后台 API（P0）

- Provider：`GET/POST /admin/providers`、`GET/PATCH /admin/providers/{id}`（`id` 可为内部 id 或 slug）、`POST /admin/providers/{id}/health-check`（不会计费、不强制确认；管理列表 `/admin/providers` 每行可探测）。新建只需名称，省略 slug 时后端生成内部标识。`PATCH` 改状态/名称/适配器/上游地址/超时（二次确认；不要改种子 echo/gemini）。上游密钥统一在账号池中管理；管理列表只展示目录和新建，改状态、账号池、已接入模型在详情页 `/admin/providers/{id}`；
- 提供商支持模型：`GET/PUT /admin/providers/{id}/upstream-models`、`POST /admin/providers/{id}/upstream-models/discover`、`PATCH /admin/providers/{id}/upstream-models/status`。人工添加只填真实上游模型标识与成本价，省略显示名时使用该标识；状态接口提交 `upstream_model_id` 和 `enabled`。停用后路由与网关不再选用，重新探测或编辑报价不会自动启用；
- 上游账号池：`GET/POST /admin/providers/{id}/accounts`、`PATCH /admin/providers/{id}/accounts/{aid}`；列表只回指纹，不回密文；冷却/失效账号不参与路由；详情页「账号池」自动展示，并可添加、冷却、停用；
- 模型：`GET/POST /admin/models`、`GET/PATCH /admin/models/{id}`（`id` 为 public_id，可含斜杠，如 `tokenhub/echo-1`）、`POST /admin/models/attach` 挂载 Provider 映射；目录/路由组无 token 返回 `401 authentication_error`；出示了禁用/轮换/过期/无效 Key 或无权限仍是 `403`；未知模型 `404`，坏输入 `400`；`GET /v1/models/{id}` 与聊天对目录中不存在的模型返回 `404`，对存在但未授权的模型返回 `403 model_not_allowed`；`GET /admin/models?status=&sync_state=&q=` 先筛选再分页，`sync_state=draft` 为待审核队列（空 sync_state 且 `status=draft` 也算待审核）；模拟上游同步接口已移除；`POST /admin/models` 手工创建永远是 `draft`（请求里的 `status` 会被忽略）；`POST /admin/models/review|publish|deprecate`（body 带 `public_id`）分别通过/拒绝、发布、弃用，不删除历史映射和价格版本。发布要求 `sync_state=reviewed`，已拒绝不能发布；创建人（`created_by_user_id`）不能审核或发布自己建的模型（409）。`PATCH` 只改展示名、厂商和 `capabilities`（需二次确认），不能靠 PATCH 直接上架。管理页 `/admin/models`：Tab「目录 | 审核」；目录列表极简；审核分待审核 / 已通过待发布 / 已拒绝；挂载、弃用、发布销售价在 `/admin/models/{public_id}`；不要改 `tokenhub/echo-1`；
- 路由：`GET/POST/PATCH /admin/routes`；创建和改策略需二次确认；`public_model_id` 是本平台公开模型标识，不是上游模型名；候选项的 `provider_id` 请求可传内部 ID 或提供商 slug，响应同时返回 `provider_id` / `provider_slug`；“候选”即参与选路的提供商池成员，`priority=1` 是 priority 策略下的首选，不存在另一套“首发”字段；管理页 `/admin/routes` 按 vendor 折叠，可创建路由组并改 `priority`/`weight`/`price`/`health`；厂商默认以模板批量套用（见 `docs/13`）；
- 渠道/代理：`GET/POST /admin/channels`、`GET/PATCH /admin/channels/{id}`、`GET/PATCH /admin/channels/{id}/models`、`GET/PATCH /channel/models`；A/C 是各自品牌的平台，B 是继承上级品牌的渠道；A 可建直属 B/C，C 只建直属 B，B 无下属。C 创建时须指定未分配的品牌，B 自动继承上级品牌，类型和品牌归属创建后固定；创建和改状态需二次确认；新渠道没有默认模型授权。平台只管理直属 B/C，OEM 管理自己的直属 B，并可转授权或撤销模型；`PATCH /channel/models` 只切换本渠道已有授权模型的本地下架状态，不恢复上级撤权，需二次确认。平台模型全局下架、上级撤权或上级本地下架都会立即阻止下属调用。租户不能自建提供商或模型；`disabled` 后聊天/媒体返回 `403 channel_disabled`，`GET /v1/me/balance` 与 usage 仍可读；
- 用户治理：`GET /admin/users`、`POST /admin/users/{id}/ban|unban`、`POST /admin/users/{id}/attribution`；封禁后登录和旧 API Key 403，未结算佣金进入 `held`；改归因与封禁需二次确认并写审计；
- 管理员 2FA：`GET /admin/me` 回当前角色；`GET /admin/me/2fa`、`POST /admin/me/2fa/setup|enable|disable`；启用后敏感写操作还要 `X-Tokenhub-TOTP`；管理页 `/admin/settings` 可读取/绑定/启用/关闭；关闭需确认，启用后再关闭还要 TOTP；不要在共享管理员上留下 `enabled`；
- 用户 API Key（D38）：平台**不再**管理具体用户 Key。目标契约为渠道范围 `GET /channel/api-keys`（prefix/状态/最近使用等，无密文）、`POST /channel/api-keys/{id}/disable`（二次确认，仅本渠道用户）。终端用户仍用 `/v1/me/api-keys`。旧平台用户 Key 管理 API 已停用（410），管理页 `/admin/keys` 退役；本人 Key 读写严格 user_id 所有权，管理角色不跳过所有权；
- 推广：`GET/POST /admin/acquisition-roles`、`GET/PATCH /admin/acquisition-roles/{id}`、`GET/POST /admin/promotion-codes`、`GET/POST /channel/promotion-codes`；`type=agent|promoter` 分列表（历史 `kol` 仍可筛 `kol_l1`/`kol_l2`）；创建角色和推广码、改状态需二次确认并写审计；管理页 `/admin/channels` 分栏管理代理商/推广员，`/admin/promos` 管推广码；
- 邀请与收益：`GET /v1/me/referral` 返回本人邀请码/链接、邀请人数、积分与分佣资格进度、本人佣金和结算；普通用户在 `/app/referral` 使用同一账户。专业客户另按已有真实权限查询，不扩展普通用户下线财务读取；旧 `/partner` 页面重定向至本人收益；
- 佣金：`GET /admin/commissions`、`GET/PATCH /admin/commission-policy`（改 BPS/冻结天数需二次确认）、`GET/PATCH /admin/eligibility-rules`（平台达线：累计消费 / 单笔充值，需确认）、`GET/PATCH /channel/eligibility-rules`（仅 C 可写，B 读平台规则）、`POST /admin/commissions/recalc`（按价格快照重算需确认）、`POST /admin/commissions/unfreeze`（解冻需确认并写审计）、`POST /admin/commissions/settle`、`POST /admin/settlements/{id}/payout`；管理页 `/admin/commission` 可重算、手工解冻、生成结算单和人工打款；
- 渠道额度：`GET /channel/quota`、`GET /channel/allocations`、`GET /admin/channel-quotas/{channel_id}`、`POST /admin/channel-quotas/grant`（平台向 OEM 发放服务额度；渠道目标拒绝）；旧 `POST /channel/quotas/grant` 停用，不创建 OEM→渠道采购、`GET/PATCH /admin/channel-quotas/{channel_id}/issue-rule`；`quota` 含 `issued_minor`/`consumed_minor`/`allocation_count`/`issue_ratio_bps`；换算比默认 `10000` BPS = 1:1，平台/财务可改（需二次确认），渠道不能改换算比；用户入账从**原所属品牌 OEM 池**发放；OEM 品牌额度不足返回 `402 insufficient_quota`；
- 渠道运营：`GET /channel/users`、`POST /channel/users/{id}/ban|unban`（B/C 管理本渠道用户，C 也可管理直属 B 用户）、`GET /channel/subchannels/{id}/users`（仅 C 可读直属 B）；`GET/POST /channel/plans` 仅 C 可用，创建本品牌统一套餐，B 不创建套餐或设置终端价格；`PATCH /channel/model-prices` 仅 C 可设置整个品牌共用的模型客户价；`GET /channel/usage`（服务端完整搜索、cursor 明细）；`GET /channel/usage/summary`（同范围全量汇总与独立筛选 facets）；原请求详情再次范围鉴权，不含 prompt、`GET /channel/attribution`、`GET /channel/settlements`、`GET /channel/commissions`；渠道 API Key 列表与禁用见上条；
- 套餐：`GET/POST /admin/plans` 由 A/C 分别管理本品牌套餐，创建后待发布；B 无权管理。指定渠道时，`GET /admin/plans/eligible-channels` 只返回本品牌直属 B，可多选。公开与登录后的套餐列表按品牌隔离，B 用户仅见上级品牌套餐。`POST /admin/plans/{id}/review` 的 `approve/reject` 执行发布/拒绝，`PATCH /admin/plans/{id}` 下架已发布套餐。创建、发布、下架、拒绝原子写入含操作人和时间的审计。A/C 各自在本品牌后台发布、下架或拒绝套餐，平台不代 OEM 审核；管理页可按渠道、套餐名和购买方式筛选，显示额度和状态操作；一次性额度长期有效，周期套餐按期发放；`POST /admin/subscriptions/{id}/force-period-end` 与 `POST /admin/subscriptions/process-renewals` 仅供沙箱运维测试，常规续费由 Worker 执行；
- 价格书 API：`GET/POST /admin/price-books`（新版本不改历史账单；发布需 `X-Tokenhub-Confirm`；body 可带 `upstream_cost` / `wholesale` / `customer_sell` 及历史兼容 `channel_override`（不参与新渠道定价）；`GET ?format=csv` 含 `effective_at` 与四列单价；管理面改价入口在模型详情与 `/admin/prices`，走对象、金额、后果明确的二次确认）；
- 权益：`POST /admin/entitlements/bonus`（手工赠送需二次确认）；管理页 `/admin/billing` 可退消费账单、确认/退充值和赠送额度；
- 支付：`GET /admin/payments`、`POST /admin/payments/{id}/confirm`、`POST /admin/payments/{id}/refund`；
- 财务：充值、退款、额度调整、佣金结算和对账；
- 观测：`GET /admin/metrics`（`dimension=provider|model|channel|user|api_key`）、`GET /admin/metrics/series`、`GET /admin/metrics/daily?format=csv`、`GET /admin/ops/dashboard`（`dimensions` 含 model / api_key / channel / user / provider / agent）、`GET /admin/ops/alerts`、`POST /admin/ops/alerts/evaluate`、`GET/PATCH /admin/ops/thresholds`、`GET /admin/ops/runbooks`；看板 totals 含错误码分布、超时、Token/媒体用量、预授权失败和回调 P95；
- 加固：`POST /admin/ops/backup-drill`、`GET/POST /admin/ops/canary`、`POST /admin/ops/circuit/{id}`、`POST /admin/ops/drills/payment|media|tls`；TLS 演练验证已知 OEM 域名 200、未知域名 404、沙箱 `issued`（`.localhost` 不走 ACME）；`scripts/e2e_acme.sh` 对 Pebble 做 RFC 8555 真签发；公网 OEM 由 Cloudflare Custom Hostname 签发续期，不自建公网 Let's Encrypt，本仓库不把 Pebble 标成公网签发；健康探测、熔断、灰度、备份演练与支付/媒体/TLS 演练不强制二次确认（技术值班 e2e 不带头）；管理页 `/admin/settings`「运维开关」可探测、打开/复位熔断、读写灰度，「备份演练」记录 RPO 15 / RTO 60，「异常演练」可跑支付/媒体/TLS；
- 审计检索：`GET /admin/audit-logs?action=&resource_type=&q=`；`GET /admin/outbox/stats` 读 `pending`/`published`/`failed`；`POST /admin/audit-probes` 沙箱写 `audit.probe`（生产 403，不强制确认）；管理页 `/admin/audit` 可读取 Outbox 并写入探测。

所有管理接口按角色授权；退款、手工加款、佣金调整、凭据修改、价格底线修改、usage 回放必须二次确认：请求头 `X-Tokenhub-Confirm: 1`（或 `confirm=1`），并记录 before/after 快照。缺少确认返回 `409 confirm_required`。健康探测、熔断与灰度设置不要求确认头。

### 财务赠送额度验收补充

- `GET /admin/billing/users?q=`：仅平台管理员、财务和运营可搜索收款用户；至少 2 个字符，最多返回 20 人。返回 `id/email/display_name/channel_code/status`，不开放用户管理或登录资料。
- `POST /admin/entitlements/bonus`：除二次确认外，必须传 `Idempotency-Key`（最多 128 字节）。同一操作重试须复用原编号与参数；并发重试只创建一个权益账户和一笔权益流水，改变参数返回 409。收款用户必须存在且处于启用状态。
- 手工赠送记录在限时权益账户，与钱包余额分开。用户在钱包权益页查看剩余额度、原始发放额和到期时间。

### 财务订单操作验收补充

- `GET /admin/payments?q=` 支持邮箱、姓名和订单编号，先过滤再截取最新匹配记录，保留原有 `limit/cursor` 分页契约。管理端 JSON 补充 `user_email/user_name/channel_code`；渠道名按订单历史渠道读取。
- 支付页从具体订单发起确认和退款，明确展示支付币种、实付金额、对应充值额度及入账状态；失败原因保留在确认框内，操作成功与列表刷新失败分开反馈。
- 退款先校验订单状态和额度回收条件；本地退款状态与额度冲正在同一事务提交。已退款订单重试不再调用渠道。Stripe 退款使用订单固定幂等键（支付宝、微信原有固定退款编号保留）。外部渠道网络结果不确定时仍需以渠道状态与对账结果核实，不能将本地事务等同于跨支付渠道事务。
- 线下订单的“登记退款”只回收平台额度并登记状态，不执行真实转账。消费账单退款及佣金冲正是独立链路，不能与充值/支付退款混称。

### 2026-10-10 业务补充：实际结算与 OEM 采购

- 请求开始前用原品牌价格预估，账户适用余额与 Key 累计限额均原子占用。可靠实际消费可超过预估与 Key 限额，并使现金余额为负；缺少真实用量仍待核对。新请求继续检查扣除欠额后的可用余额，不把欠额按零计算；充值不重置 Key 已用。无法取得有效计费估算时返回 `price_estimate_unavailable`，不再以供应商绝对费用上界白名单决定有限额 Key 是否可用。
- `GET /admin/oem-purchases`：平台/财务/运营/审计读取，支持 `oem_channel_id`、`limit`、`cursor`；游标必须属于当前查询范围。`GET /channel/oem-purchases` 仅 OEM 读取自己的采购，裁去平台内部备注与操作人员字段。
- `POST /admin/oem-purchases`：平台管理员/财务确认实际到账并划入服务额度。必填 `operation_id`、`oem_channel_org_id`、`cash_amount_minor`、`cash_currency`、`sale_amount_minor`、`quota_amount_minor`、`occurred_at`、`confirmed=true`，需现有确认头。`external_reference` 与 `note` 可为空。
- 金额单位：USD 现金、协议 USD 销售金额及服务额度均为 microUSD（1 USD = 1,000,000）；CNY 现金为分（1 CNY = 100）。USD 现金金额须等于协议 USD 销售金额；CNY 的协议 USD 金额由双方明确填写，不推测汇率。销售金额与服务额度分别保存，不能自动互相代替。
- 原操作编号绑定登记人、OEM 与完整参数；重试不重复发放，参数不同返回 `409 operation_conflict`。真实外部交易号非空时按平台收款主体防重，重复返回 `external_reference_conflict` 与原记录。`GET /admin/oem-purchases/operations?oem_channel_id=&operation_id=` 供原登记人核对未知结果；`GET /admin/oem-purchases/{id}` 供有权平台岗位查看原单。
- `POST /admin/oem-purchases/{id}/reverse`：平台管理员/财务登记误录撤销，提交新的稳定 `operation_id` 与 `reason` 并确认。原 OEM 可用服务额度足额时原子收回、冲减销售并写审计；额度不足返回 `409 quota_not_recoverable`，不改变客户余额。不执行外部退款，不删除原采购，重复请求沿同一原操作核对。
- 平台经营收入是自营品牌终端消费与已完成 OEM 服务额度销售；不汇总 OEM 终端售价。OEM API 毛利是其终端实际计费减原平台结算价对应消耗。采购现金、未消费额度、真实上游成本和营销支出分别呈现，详情见 docs/28、docs/30。
