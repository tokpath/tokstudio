# TokenHub P0 数据模型与关键状态机

本文将已确认的产品规则落到可实施的数据结构。数据库建议使用 PostgreSQL；金额和额度不能使用浮点数，统一使用最小货币单位或定点小数。

## 1. 建模原则

- 所有余额、额度、成本、收入和佣金以不可变流水为准，汇总余额只能作为并发控制和查询缓存。
- 所有外部回调、消费结算和额度发放必须幂等。
- 异步领域事件先写入 Outbox，再由 Worker 或 Dapr Pub/Sub 投递；业务代码不直接依赖具体消息中间件。
- 模块按未来服务边界拥有数据；P0 即使共用 PostgreSQL，也禁止跨模块直接访问对方表或复用内部 ORM Model。
- 公开模型（`public_model`）与实际提供商模型（`provider_model_mapping`）分离，同一公开模型允许多个 Provider。
- 用户固定归属一个 `channel_org`；推广角色通过 `acquisition_attribution` 独立记录。
- 钱包现金余额与套餐/赠送权益分账，文本 Token、视频秒数、图片次数等不同计量单位不能混用。
- 删除采用软删除或状态下线，历史请求、账单、佣金和审计记录永不删除。

## 2. 核心实体

### 2.1 身份、组织与归因

| 表 | 关键字段 | 说明 |
|---|---|---|
| `user` | `id`, `email`, `password_hash`, `status`, `channel_org_id`, `brand_id`, `display_name`, `locale` | 普通用户是唯一终端用户类型；管理员是附加角色；`locale` 仅 zh/en/ja |
| `role` | `id`, `code` | `platform_admin`, `finance_admin`, `ops_admin`, `tech_admin`, `channel_admin`, `audit_readonly`, `end_user` |
| `user_role` | `user_id`, `role_id`, `scope_type`, `scope_id` | 管理角色按平台/渠道范围授权 |
| `channel_org` | `id`, `code`, `type`, `parent_id`, `status`, `brand_id` | A 官方、B 分销、C OEM；支持渠道层级 |
| `acquisition_role` | `id`, `channel_org_id`, `type`, `parent_id`, `level`, `status` | 代理商、1/2 级 KOL |
| `acquisition_attribution` | `user_id`, `channel_org_id`, `acquisition_role_id`, `source_code`, `attributed_at` | 唯一归因，注册完成后固化 |
| `role_member` | `user_id`, `acquisition_role_id` | 登录用户与代理商/KOL 主体绑定 |

P0 落地时推广角色物理表为 `identity_acquisition_roles`、`identity_role_members`。层级固定为 agent → kol_l1 → kol_l2。管理员 TOTP 物理表为 `identity_admin_totp`（密钥密文，`pending`/`enabled`）；未启用前敏感操作只要求二次确认，启用后还要 `X-Tokenhub-TOTP`。
| `brand` | `id`, `name`, `logo_url`, `primary_domain`, `api_domain`, `admin_domain`, `theme_json` | OEM 品牌和域名配置 |

### 2.2 Provider、模型与路由

| 表 | 关键字段 | 说明 |
|---|---|---|
| `provider` | `id`, `name`, `kind`, `adapter`, `base_url`, `credential_ref`, `region`, `status`, `health`, `priority`, `weight`, `timeout_ms`, `retry_max`, `rpm_limit`, `concurrency_limit`, `capability_tags` | 上游 Provider，不保存明文密钥 |
| `provider_credential` | `id`, `provider_id`, `ciphertext`, `key_hash`, `status`, `kind`, `label`, `model_tags`, `rpm_limit`, `concurrency_limit`, `last_success_at`, `last_error_at`, `last_error_code`, `cooldown_until`, `rotated_at` | 上游账号池：加密密文 + hash 指纹；状态 active/disabled/cooldown/invalid/exhausted/unknown/rotated |
| `public_model` | `id`, `public_id`, `vendor`, `display_name`, `capabilities_json`, `status` | 客户看到的模型，如 `openai/gpt-5.6` |
| `provider_model_mapping` | `id`, `public_model_id`, `provider_id`, `upstream_model_id`, `capabilities_json`, `sync_state` | 上游模型映射，自动同步先进入 draft |
| `price_version` | `id`, `public_model_id`, `provider_id`, `unit_prices_json`, `effective_at`, `status` | 成本、批发价、销售价版本化 |
| `route_group` | `id`, `public_model_id`, `strategy`, `fallback_policy`, `status` | 固定优先级/权重/价格/健康优先 |
| `route_candidate` | `route_group_id`, `provider_id`, `priority`, `weight`, `constraints_json` | provider.only/provider.ignore 等约束 |
| `channel_model_policy` | `channel_org_id`, `public_model_id`, `enabled`, `sell_price_override` | 渠道可见模型和销售价覆盖 |

### 2.3 API Key、套餐与权益

| 表 | 关键字段 | 说明 |
|---|---|---|
| `api_key` | `id`, `user_id`, `name`, `prefix`, `secret_ciphertext`, `secret_hash`, `expires_at`, `last_used_at`, `status`, `rpm_limit`, `concurrency_limit` | 完整 Key 可长期查看；轮换改密文不改主键；禁用/过期后鉴权失败，操作写审计 |
| `api_key_model_policy` | `api_key_id`, `public_model_id`, `allowed` | Key 级模型白名单 |
| `product_plan` | `id`, `owner_type`, `owner_id`, `name`, `currency`, `price`, `billing_period`, `status`, `policy_version` | 平台和渠道均可创建 |
| `plan_item` | `plan_id`, `public_model_id`, `unit_type`, `included_amount`, `overage_price`, `expires_in` | token/video_second/image_count/request_count/usd_credit |
| `subscription` | `id`, `user_id`, `plan_id`, `status`, `current_period_start`, `current_period_end`, `renewal_policy`, `payment_method_ref` | active/past_due/cancelled 等 |
| `entitlement_account` | `id`, `user_id`, `source_type`, `source_id`, `unit_type`, `granted`, `consumed`, `expires_at`, `status` | 套餐和赠送额度独立账户 |
| `entitlement_ledger` | `id`, `account_id`, `event_type`, `amount`, `request_id`, `occurred_at` | 发放、消费、过期、回收、冲正 |

P0 落地时套餐实体由独立 `plans` 模块拥有，物理表为 `plans_product_plans`、`plans_plan_items`、`plans_subscriptions`、`plans_entitlement_accounts`、`plans_entitlement_ledger`。金额与 `usd_credit` 使用 micro-USD。billing 只能通过 `AvailableUSD` / `ConsumeUSD` / `ReverseKeep` 接口覆盖预授权，禁止直连套餐表。

### 2.4 钱包、充值、用量与账务

| 表 | 关键字段 | 说明 |
|---|---|---|
| `wallet_account` | `id`, `user_id`, `currency`, `available_minor`, `reserved_minor`, `version` | 现金钱包，乐观锁/行锁控制并发 |
| `wallet_ledger` | `id`, `wallet_id`, `event_type`, `amount_minor`, `reference_type`, `reference_id`, `idempotency_key` | 充值、预授权、结算、释放、退款 |
| `topup_order` | `id`, `user_id`, `channel_org_id`, `amount_minor`, `currency`, `payment_method`, `status`, `provider_trade_id` | pending/paid/failed/expired/refunded/partially_refunded |
| `payment_event` | `id`, `adapter`, `external_event_id`, `signature_valid`, `payload_json`, `processed_at` | webhook 原文与幂等 |

P0 支付实体由独立 `payment` 模块拥有，物理表为 `payment_orders`、`payment_events`。适配器为 `stripe` / `alipay` / `wechat` / `manual`；只有 Stripe 声明自动续费能力。billing 预授权增加 `wallet_reserved_minor`：权益覆盖后钱包只冻结差额。
| `request` | `id`, `request_id`, `user_id`, `api_key_id`, `channel_org_id`, `public_model_id`, `protocol`, `status`, `started_at`, `ended_at` | 一次客户请求 |
| `attempt` | `id`, `request_id`, `provider_id`, `upstream_model_id`, `status`, `error_code`, `latency_ms`, `started_at`, `ended_at` | 一次上游尝试；fallback 不重复客户收费 |
| `usage_event` | `id`, `request_id`, `attempt_id`, `unit_usage_json`, `unit_prices_json`, `customer_amount`, `upstream_cost`, `currency`, `state`, `idempotency_key` | confirmed/pending_reconciliation/voided |
| `customer_charge` | `id`, `request_id`, `usage_event_id`, `amount_minor`, `price_version_id`, `status` | 每个请求最多一个最终客户扣费事件 |
| `commission_ledger` | `id`, `usage_event_id`, `channel_org_id`, `acquisition_role_id`, `policy_version`, `amount_minor`, `status` | frozen/available/paid/reversed |

P0 佣金明细由独立 `commission` 模块拥有：`commission_policies`、`commission_entries`、`commission_settlements`、`commission_payouts`。默认 7 天冻结、35% 单笔上限、按团队→渠道→管理奖励→直接佣金缩减。billing 只通过 `AccrueUsage`/`ReverseUsage` 接口通知，不直连佣金表。

P0 落地时这些实体由 `billing` 模块拥有，物理表带 `billing_` 前缀（如 `billing_wallets`、`billing_usage_events`）。金额使用 micro-USD（`1 USD = 1_000_000`）。其他模块只能通过账务服务接口读写，禁止直连表。

### 2.5 媒体任务与审计

| 表 | 关键字段 | 说明 |
|---|---|---|
| `media_job` | `id`, `user_id`, `request_id`, `public_model_id`, `provider_id`, `upstream_job_id`, `status`, `progress`, `callback_url`, `expires_at` | queued/in_progress/completed/failed/cancelled/expired |
| `media_asset` | `id`, `media_job_id`, `kind`, `object_key`, `content_type`, `size_bytes`, `sha256`, `expires_at` | 受控对象存储，签名 URL 下载 |

P0 落地时媒体实体由 `media` 模块拥有，物理表为 `media_jobs`、`media_assets`、`media_callback_events`。拿到 `upstream_job_id` 后禁止再次 Create；回调按 `event_id` 幂等；结果默认 7 天后清理。

P0 运营实体由独立 `ops` 模块拥有：`ops_alerts`、`ops_runbooks`、`ops_backup_drills`、`ops_canary`、`ops_alert_thresholds`。看板数字通过 gateway/billing 公开接口聚合，ops 不直连它们的表。预授权失败由 `billing_preauth_failures` 在事务外落库，供看板统计 `preauth_failed`。限流、熔断与写操作 `Idempotency-Key` 计数只存在 Redis（幂等记录 TTL 24h）。目录管理通过 catalog 公开接口做 Provider/模型/路由 CRUD，不直连表；`AttachProvider` 写 mapping 并追加 route candidate。P0 文本模型含 `tokenhub/echo-1` 与 `google/gemini-flash`（无 Gemini Base URL 时走沙箱适配器）。Bifrost 数据面默认走独立 sidecar，未配置 URL 时该 Adapter 不作为成功候选。过期预授权由 `billing.ReapExpired` 回收。
| `audit_log` | `id`, `actor_user_id`, `action`, `resource_type`, `resource_id`, `before_json`, `after_json`, `ip`, `created_at` | 不可删除，敏感操作二次确认 |
| `outbox_event` | `id`, `event_type`, `aggregate_type`, `aggregate_id`, `payload_json`, `status`, `attempts`, `available_at`, `published_at` | 可靠投递；可由本地 Worker 或 Dapr Pub/Sub 消费 |

## 3. 关键不变量

1. `available_minor + reserved_minor` 不得为负；预授权、释放和结算必须在同一账务事务中完成。
2. 同一 `idempotency_key` 在同一业务域只能成功一次。
3. `usage_event` 的客户金额按价格快照计算，价格变更不影响历史账单。
4. B/C 用户消费只扣自己的权益/钱包；渠道额度用于风险上限和分配校验，不对同一请求重复扣款。
5. 佣金基于已确认 usage 和渠道批发价产生，退款或人工冲正必须生成反向流水。
6. 媒体任务拿到 `upstream_job_id` 后禁止自动重复提交；未知状态进入待确认。
7. 跨模块一致性通过 Outbox 事件和补偿流水实现，不使用跨服务分布式事务；每个事件必须有版本和幂等消费记录。

## 4. 关键状态机

### 4.1 预授权与结算

`none -> reserved -> settled`

`reserved -> released`（上游明确失败且无费用）

`reserved -> pending_reconciliation`（成功但缺 usage）

`settled -> reversed`（退款/冲正，仅退未消费部分或生成负向流水）

### 4.2 Provider 健康

`maintenance -> available`

`available -> degraded`（错误率/延迟超过阈值）

`degraded -> unavailable`（连续失败触发熔断）

`unavailable -> degraded -> available`（连续成功探测或真实请求恢复）

### 4.3 订阅

`pending -> active -> past_due -> active`

`past_due -> cancelled`（7 天宽限期结束仍未支付）

`active -> cancel_at_period_end -> cancelled`

### 4.4 充值订单

`pending -> paid`

`pending -> failed | expired`

`paid -> refunded | partially_refunded`

### 4.5 媒体任务

`queued -> in_progress -> completed`

`queued/in_progress -> cancelled | failed | expired`

上游已返回任务 ID 后，重试只允许查询/回调处理，不允许再次创建同一任务。

### 4.6 佣金

`frozen -> available -> paid`

任一阶段均可因退款、冲正或风控进入 `reversed`，但不得删除原流水。

### 4.7 异步事件投递

业务事务提交时同时写入 `outbox_event`；投递器发布 CloudEvents 后标记 `published`。失败事件按指数退避重试，超过阈值进入死信/人工处理。推荐事件主题：`media.job.created`、`media.job.poll`、`payment.webhook.received`、`usage.reconciliation.requested`、`commission.settlement.requested`。事件消费者必须按事件 ID 幂等。
