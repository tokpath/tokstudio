# TokenHub P0 OpenAPI 契约骨架

本文定义 P0 对外 API 的稳定边界。具体 schema 可据此生成 OpenAPI 3.1 文件；所有接口统一返回 `request_id`，写操作支持 `Idempotency-Key`。

## 1. 通用约定

- API Base：`https://{api-domain}/v1`
- 鉴权：`Authorization: Bearer <tokenhub_api_key>`
- 请求头：`X-Request-ID` 可选；未提供时服务端生成。
- 写操作：`Idempotency-Key` 必须是客户端生成的稳定值，服务端保存 24 小时以上。
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

### `GET /v1/videos/{id}`

返回 `queued`、`in_progress`、`completed`、`failed`、`cancelled`、`expired`、进度、失败原因、usage 摘要和最终 Provider。

### `GET /v1/videos/{id}/content`

返回短期签名 URL 或媒体流；默认结果保留 7 天。

### `POST /v1/images/generations` 与 `POST /v1/images/edits`

与视频共用媒体任务状态机、预授权和签名下载；返回 `202` 与任务 ID。

### `POST /v1/videos/{id}/cancel`

请求取消；已提交上游任务只执行可用的上游取消接口，不重复创建任务。

### 回调 `POST {callback_url}` 与 `POST /v1/media/callbacks`

上游完成后可回调客户 `callback_url`，沙箱与自建上游使用平台入口 `POST /v1/media/callbacks`。服务端校验 `X-Tokenhub-Signature`（HMAC-SHA256，`event_id|job_id`），按 `event_id` 幂等；重复事件必须返回 2xx 且不得重复结算。未完成的任务在重试时会继续落状态和结算。

## 5. 用户、Key、套餐与余额

- `GET /v1/me`
- `GET /v1/me/balance`
- `GET /v1/me/usage`
- `GET /v1/me/ledger`
- `GET /v1/plans`：公共站已发布的平台套餐；
- `GET /v1/me/plans`：当前渠道可见的已发布套餐；
- `GET /v1/me/entitlements`：赠送与套餐权益账户；
- `POST /v1/me/subscriptions`
- `GET /v1/me/subscriptions`
- `POST /v1/me/subscriptions/{id}/cancel`
- `GET /v1/me/api-keys`
- `POST /v1/me/api-keys`
- `POST /v1/me/api-keys/{id}/rotate`
- `POST /v1/me/api-keys/{id}/disable`
- `POST /v1/me/api-keys/{id}/copy`：复制完整 Key，只写审计不改密文

完整 API Key 只返回给创建者；查看、复制、轮换、禁用、过期均写审计日志。

## 6. 充值与支付

- `POST /v1/topups`：创建充值订单；
- `GET /v1/topups/{id}`：查询订单；
- `POST /v1/topups/{id}/refund`：按权限申请退款；
- `POST /v1/payments/orders`：创建钱包充值支付单（沙箱适配器）；
- `GET /v1/payments/orders/{id}`：查询支付单；
- `POST /v1/payments/{adapter}/webhook`：支付适配器回调。`X-Tokenhub-Payment-Signature` 为 HMAC-SHA256(`event_id|order_id|status`)，按 `external_event_id` 幂等。
- `POST /v1/topups/redeem`：兑换码入账（M3 沙箱码 `THE2E` / `THCREDIT10`）。
- `POST /admin/topups/{id}/confirm`：财务确认人工充值。
- `POST /admin/refunds`：按 `request_id` 或 `topup_id` 退款并冲正佣金。
- `POST /admin/usage/replay`：幂等回放 usage / 完成待对账。
- `GET /admin/billing/report`：收入、成本、佣金负债、待对账数量。
- `GET /admin/billing/export`：对账 CSV（收入/成本/佣金/毛利/待对账）。
- `GET /admin/usage?format=csv`：用量明细导出。
- `POST /v1/me/api-keys/{id}/expire`：设置过期时间；过期后鉴权失败。
- `GET /v1/me/media`：当前用户媒体任务（kind/status 筛选，不含他人数据）。
- `GET /admin/media`：管理端媒体任务列表；`?format=csv` 导出且不含 prompt。
- `GET /v1/public/tls-check?domain=`：Caddy on-demand TLS 询问；仅已登记品牌域名返回 200。
- `GET /admin/brands`、`POST /admin/brands/{id}/tls/issue`：OEM CNAME 目标与证书状态。
- `POST /admin/commissions/recalc`：按价格快照重算佣金。
- `POST /admin/price-books`：发布新价格版本，不影响历史账单。

支付回调必须验签、记录原始事件、按外部事件 ID 幂等，并在确认 `paid` 后发放余额/权益。

## 7. 管理后台 API（P0）

- Provider：`GET/POST/PATCH /admin/providers`、`POST /admin/providers/{id}/health-check`、`POST /admin/providers/{id}/credentials` 凭据轮换（不回显明文）；
- 模型：`GET/POST/PATCH /admin/models`、同步、审核、发布、弃用；
- 路由：`GET/POST/PATCH /admin/routes`；
- 渠道/代理：`GET/POST/PATCH /admin/channels`、归因、额度和佣金策略；
- 管理员 2FA：`GET /admin/me/2fa`、`POST /admin/me/2fa/setup|enable|disable`；启用后敏感写操作还要 `X-Tokenhub-TOTP`；
- API Key 摘要：`GET /admin/api-keys`（只有 prefix，无完整密钥）；
- 推广：`GET/POST /admin/acquisition-roles`、`POST /admin/promotion-codes`；
- 分销只读：`GET /v1/partner/users|commissions|settlements|export`（按角色树过滤，邮箱脱敏，不含 prompt）；
- 佣金：`GET /admin/commissions`、`POST /admin/commissions/unfreeze`、`POST /admin/commissions/settle`、`POST /admin/settlements/{id}/payout`；
- 渠道额度：`GET /channel/quota`、`POST /admin/channel-quotas/grant`；
- 套餐：`GET/POST/PATCH /admin/plans`、`POST /admin/plans/{id}/review`、发布、下架；
- 权益：`POST /admin/entitlements/bonus`；
- 支付：`POST /admin/payments/{id}/confirm`、`POST /admin/payments/{id}/refund`；
- 财务：充值、退款、额度调整、佣金结算和对账；
- 观测：`GET /admin/metrics`、`GET /admin/ops/dashboard`、`GET /admin/ops/alerts`、`POST /admin/ops/alerts/evaluate`、`GET /admin/ops/runbooks`；
- 加固：`POST /admin/ops/backup-drill`、`GET/POST /admin/ops/canary`、`POST /admin/ops/circuit/{id}`、`POST /admin/ops/drills/payment|media`；
- 审计检索：`GET /admin/audit-logs?action=&resource_type=&q=`。

所有管理接口按角色授权；退款、手工加款、佣金调整、凭据修改、价格底线修改必须二次确认：请求头 `X-Tokenhub-Confirm: 1`（或 `confirm=1`），并记录 before/after 快照。缺少确认返回 `409 confirm_required`。
