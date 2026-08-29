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

### `POST /v1/videos/{id}/cancel`

请求取消；已提交上游任务只执行可用的上游取消接口，不重复创建任务。

### 回调 `POST {callback_url}`

服务端使用签名头和事件 ID，支持幂等、指数退避重试；重复事件必须返回 2xx 且不得重复结算。

## 5. 用户、Key、套餐与余额

- `GET /v1/me`
- `GET /v1/me/balance`
- `GET /v1/me/usage`
- `GET /v1/me/ledger`
- `GET /v1/me/plans`
- `POST /v1/me/subscriptions`
- `POST /v1/me/subscriptions/{id}/cancel`
- `GET /v1/me/api-keys`
- `POST /v1/me/api-keys`
- `POST /v1/me/api-keys/{id}/rotate`
- `POST /v1/me/api-keys/{id}/disable`

完整 API Key 只返回给创建者；查看、复制、轮换、禁用、过期均写审计日志。

## 6. 充值与支付

- `POST /v1/topups`：创建充值订单；
- `GET /v1/topups/{id}`：查询订单；
- `POST /v1/topups/{id}/refund`：按权限申请退款；
- `POST /v1/payments/{adapter}/webhook`：支付适配器回调。

支付回调必须验签、记录原始事件、按外部事件 ID 幂等，并在确认 `paid` 后发放余额/权益。

## 7. 管理后台 API（P0）

- Provider：`GET/POST/PATCH /admin/providers`、健康检查和凭据轮换；
- 模型：`GET/POST/PATCH /admin/models`、同步、审核、发布、弃用；
- 路由：`GET/POST/PATCH /admin/routes`；
- 渠道/代理：`GET/POST/PATCH /admin/channels`、归因、额度和佣金策略；
- 套餐：`GET/POST/PATCH /admin/plans`、审核、发布、下架；
- 财务：充值、退款、额度调整、佣金结算和对账；
- 观测：`GET /admin/metrics`、`GET /admin/audit-logs`。

所有管理接口按角色授权；退款、手工加款、佣金调整、凭据修改、价格底线修改必须二次确认并记录 before/after 快照。
