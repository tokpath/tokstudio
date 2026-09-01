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
| `user` | `id`, `email`, `password_hash`, `status`, `channel_org_id`, `brand_id`, `display_name`, `locale` | 普通用户是唯一终端用户类型；管理员是附加角色；`locale` 仅 zh/en/ja；`status` 为 `active`/`banned`，封禁后会话与 API Key 失效 |
| `role` | `id`, `code` | `platform_admin`, `finance_admin`, `ops_admin`, `tech_admin`, `channel_admin`, `audit_readonly`, `end_user` |
| `user_role` | `user_id`, `role_id`, `scope_type`, `scope_id` | 管理角色按平台/渠道范围授权 |
| `channel_org` | `id`, `code`, `type`, `parent_id`, `status`, `brand_id` | A 官方、B 分销、C OEM；支持渠道层级；`disabled` 冻结新消费（聊天/媒体 403），余额和历史保留 |
| `acquisition_role` | `id`, `channel_org_id`, `type`, `parent_id`, `level`, `status` | 代理商、1/2 级 KOL |
| `acquisition_attribution` | `user_id`, `channel_org_id`, `acquisition_role_id`, `source_code`, `attributed_at` | 唯一归因，注册完成后固化 |
| `role_member` | `user_id`, `acquisition_role_id` | 登录用户与代理商/KOL 主体绑定 |

P0 落地时推广角色物理表为 `identity_acquisition_roles`、`identity_role_members`。层级固定为 agent → kol_l1 → kol_l2。管理员 TOTP 物理表为 `identity_admin_totp`（密钥密文，`pending`/`enabled`）；未启用前敏感操作只要求二次确认，启用后还要 `X-Tokenhub-TOTP`。
| `brand` | `id`, `name`, `logo_url`, `logo_dark_url`, `favicon_url`, `primary_domain`, `api_domain`, `admin_domain`, `theme_json`, `cname_target`, `tls_status`, `tls_issuer`, `tls_directory`, `tls_expires_at` | OEM 品牌和域名；`tls_issuer` 为 `sandbox` 或 `acme`；空 ACME 目录或 `.localhost` 只标沙箱 `issued`，不假装公网 Let's Encrypt |
| `identity_brand_assets` | `id`, `brand_id`, `kind`, `object_key`, `content_type`, `size_bytes`, `width_px`, `height_px`, `sha256`, `status` | 品牌公开资源；`kind` 为 `logo` / `logo_dark` / `favicon` / `og_image`。Logo ≤128KiB、短边 64–1024px。不走媒体 7 天签名 URL |

### 2.2 Provider、模型与路由

| 表 | 关键字段 | 说明 |
|---|---|---|
| `provider` | `id`, `name`, `kind`, `adapter`, `base_url`, `credential_ref`, `region`, `status`, `health`, `priority`, `weight`, `timeout_ms`, `retry_max`, `rpm_limit`, `concurrency_limit`, `capability_tags` | 上游 Provider，不保存明文密钥 |
| `provider_credential` | `id`, `provider_id`, `ciphertext`, `key_hash`, `status`, `kind`, `label`, `model_tags`, `rpm_limit`, `concurrency_limit`, `last_success_at`, `last_error_at`, `last_error_code`, `cooldown_until`, `rotated_at` | 上游账号池：加密密文 + hash 指纹；状态 active/disabled/cooldown/invalid/exhausted/unknown/rotated |
| `public_model` | `id`, `public_id`, `vendor`, `display_name`, `capabilities_json`, `status`, `sync_state`, `created_by_user_id`, `reviewed_by_user_id` | 客户看到的模型，如 `openai/gpt-5.6`；手工创建/同步为 `draft`，审核通过为 `reviewed`，拒绝为 `rejected`，发布后 `status=published` 且 `sync_state=published`。创建人不能审核或发布自己建的模型。ofox 公开目录快照由独立命令 `cmd/catalog-seed` 预置为已审核已发布（`created_by`/`reviewed_by` 为空，系统目录不做创建人互斥） |
| `provider_model_mapping` | `id`, `public_model_id`, `provider_id`, `upstream_model_id`, `capabilities_json`, `sync_state` | 上游模型映射，自动同步先进入 draft |
| `price_version` | `id`, `public_model_id`, `provider_id`, `unit_prices_json`, `effective_at`, `status` | 成本、批发价、销售价版本化 |
| `route_group` | `id`, `public_model_id`, `strategy`, `fallback_policy`, `status` | 固定优先级/权重/价格/健康优先 |
| `route_candidate` | `route_group_id`, `provider_id`, `priority`, `weight`, `constraints_json` | provider.only/provider.ignore 等约束 |
| `channel_model_policy` | `channel_org_id`, `public_model_id`, `enabled`, `sell_price_override` | 平台从目录授权给租户的可见模型；租户不能自建提供商或模型 |

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

P0 支付实体由独立 `payment` 模块拥有，物理表为 `payment_orders`（含 `channel_org_id`、`credit_minor`）、`payment_events`、`payment_provider_instances`（按渠道加密凭证）、`payment_channel_settings`、`payment_adapter_flags`。适配器是可插拔插件（`Adapter` 接口 + `Registry`）：内置 `stripe` / `alipay` / `wechat` / `manual`；后续本地支付或聚合网关实现同一接口并 Register 即可。只有声明 `AutoRenew` 的插件（当前 Stripe）可代扣。billing 预授权增加 `wallet_reserved_minor`：权益覆盖后钱包只冻结差额。
| `request` | `id`, `request_id`, `user_id`, `api_key_id`, `channel_org_id`, `public_model_id`, `protocol`, `status`, `started_at`, `ended_at` | 一次客户请求 |
| `attempt` | `id`, `request_id`, `provider_id`, `upstream_model_id`, `status`, `error_code`, `latency_ms`, `started_at`, `ended_at` | 一次上游尝试；fallback 不重复客户收费 |
| `usage_event` | `id`, `request_id`, `attempt_id`, `unit_usage_json`, `unit_prices_json`, `customer_amount`, `upstream_cost`, `currency`, `state`, `idempotency_key` | confirmed/pending_reconciliation/voided |
| `customer_charge` | `id`, `request_id`, `usage_event_id`, `amount_minor`, `price_version_id`, `status` | 每个请求最多一个最终客户扣费事件 |
| `commission_ledger` | `id`, `usage_event_id`, `channel_org_id`, `acquisition_role_id`, `policy_version`, `amount_minor`, `status` | frozen/held/available/paid/reversed；封禁把未结算标 `held` |

P0 佣金明细由独立 `commission` 模块拥有：`commission_policies`、`commission_entries`、`commission_settlements`、`commission_payouts`。默认 7 天冻结、35% 单笔上限、按团队→渠道→管理奖励→直接佣金缩减。billing 只通过 `AccrueUsage`/`ReverseUsage` 接口通知，不直连佣金表。

P0 落地时这些实体由 `billing` 模块拥有，物理表带 `billing_` 前缀（如 `billing_wallets`、`billing_usage_events`、`billing_quota_allocations`、`billing_quota_issue_rules`）。金额使用 micro-USD（`1 USD = 1_000_000`）。其他模块只能通过账务服务接口读写，禁止直连表。

| 表 | 关键字段 | 说明 |
|---|---|---|
| `billing_quota_issue_rules` | `id`, `channel_org_id`, `issue_ratio_bps`, `version`, `updated_at` | 平台按渠道配置“充值金额 → 服务额度”换算比；`10000` BPS = 1.0（默认 1:1）；合法范围 `1000`–`100000`；无行按 1:1；B/C 代理商不能改 |

B/C 额度发放：用户充值入账后按渠道 `issue_ratio_bps`（默认 1:1）写入 `billing_quota_allocations.granted_minor`，并从渠道 `billing_quota_accounts.available_minor` 扣减发放额（`quota_issue`）。请求结算只增加 `consumed_minor` 并记 `billing_quota_consumes`，不再二次扣渠道。渠道额度不足时兑换/确认入账返回 `402 insufficient_quota`。官方渠道不发放。未消费部分退充值时 `quota_reclaim` 退回渠道。代理商实际充值、平台授予额度、用户充值、用户额度、终端消费、渠道批发成本和佣金基数分别记账。

### 2.5 媒体任务与审计

| 表 | 关键字段 | 说明 |
|---|---|---|
| `media_job` | `id`, `user_id`, `request_id`, `public_model_id`, `provider_id`, `upstream_job_id`, `task_type`, `duration_seconds`, `resolution`, `aspect_ratio`, `fps`, `generate_audio`, `first_frame`, `last_frame`, `reference_video`, `reference_audio`, `source_job_id`, `images_json`, `status`, `progress`, `callback_url`, `expires_at` | queued/in_progress/completed/failed/cancelled/expired；`task_type` 为 t2v/i2v/first_frame/first_last_frame/reference/extend/edit/generate |
| `media_asset` | `id`, `media_job_id`, `kind`, `object_key`, `content_type`, `size_bytes`, `sha256`, `expires_at` | 受控对象存储，签名 URL 下载 |

P0 落地时媒体实体由 `media` 模块拥有，物理表为 `media_jobs`、`media_assets`、`media_callback_events`。D3.2 用 `0002_d32_task_modes.sql` 补任务模式和参考素材列，禁止 AutoMigrate。拿到 `upstream_job_id` 后禁止再次 Create；回调按 `event_id` 幂等；结果默认 7 天后清理。图生/首帧要图，首尾帧要两帧，参考模式至少一种参考，延长/编辑要本用户已完成视频的 `source_job_id`，图像 edit 要 `images`。独立音频/转写/视频理解/复杂时间线仍是 P1。

P0 运营实体由独立 `ops` 模块拥有：`ops_alerts`、`ops_runbooks`、`ops_backup_drills`、`ops_canary`、`ops_alert_thresholds`。看板数字通过 gateway/billing 公开接口聚合，ops 不直连它们的表。预授权失败由 `billing_preauth_failures` 在事务外落库，供看板统计 `preauth_failed`。限流、熔断与写操作 `Idempotency-Key` 计数只存在 Redis（幂等记录 TTL 24h）。目录管理通过 catalog 公开接口做 Provider/模型/路由 CRUD，不直连表；`AttachProvider` 写 mapping 并追加 route candidate。P0 文本模型含 `tokenhub/echo-1` 与 `google/gemini-flash`（无 Gemini Base URL 时走沙箱适配器）。Bifrost 数据面默认嵌入 API 进程（`github.com/maximhq/bifrost/core`），`TOKENHUB_BIFROST_SANDBOX=true` 时用 plugin 回声；未成功 Init 时该 Adapter 返回 `provider_unavailable`。过期预授权由 `billing.ReapExpired` 回收。
| `audit_log` | `id`, `actor_user_id`, `action`, `resource_type`, `resource_id`, `before_json`, `after_json`, `ip`, `created_at` | 不可删除，敏感操作二次确认 |
| `outbox_event` | `id`, `event_type`, `aggregate_type`, `aggregate_id`, `payload_json`, `status`, `attempts`, `available_at`, `published_at` | 可靠投递；可由本地 Worker 或 Dapr Pub/Sub 消费 |

## 3. 关键不变量

1. `available_minor + reserved_minor` 不得为负；预授权、释放和结算必须在同一账务事务中完成。
2. 同一 `idempotency_key` 在同一业务域只能成功一次。
3. `usage_event` 的客户金额按价格快照计算，价格变更不影响历史账单。
4. B/C 用户消费只扣自己的权益/钱包和已发放 allocation；渠道额度在充值发放时扣减，请求时只做剩余风险帽检查，不对同一请求再扣渠道。
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

封禁把未结算的 `frozen`/`available` 标成 `held`，解封后再按冻结截止时间回到 `frozen` 或 `available`。任一阶段均可因退款、冲正或风控进入 `reversed`，但不得删除原流水。

### 4.7 异步事件投递

业务事务提交时同时写入 `outbox_event`；投递器发布 CloudEvents 后标记 `published`。失败事件按指数退避重试，超过阈值进入死信/人工处理。推荐事件主题：`media.job.created`、`media.job.poll`、`payment.webhook.received`、`usage.reconciliation.requested`、`commission.settlement.requested`。事件消费者必须按事件 ID 幂等。
