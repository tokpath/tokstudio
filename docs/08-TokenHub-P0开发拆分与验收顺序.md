# TokenHub P0 开发拆分与验收顺序

目标是按可运行的纵向切片交付，优先打通“用户 -> API Key -> 模型请求 -> 预授权 -> 上游 -> usage -> 账单”主链路，再扩展媒体、套餐、支付和分销。

## 1. 里程碑

### M0 基础工程与安全底座

范围：Go 服务骨架、React/Next.js 工程、PostgreSQL migration、Redis、Outbox、配置/密钥管理、CI、结构化日志、OpenTelemetry、基础 RBAC。

验收：本地 Docker Compose 一键启动；健康检查、迁移、日志、trace 和基础审计可用；敏感配置不进入代码库和普通日志。

### M1 身份、渠道与品牌

范围：邮箱密码、Google OAuth、普通用户、管理角色、A/B/C/OEM 渠道、唯一归因、推广码、OEM 品牌和域名配置；公共站点、用户控制台、渠道控制台和平台管理控制台四类入口；公共开发者文档和品牌化接入示例。

验收：用户注册后渠道归属不可自行切换；不同渠道数据隔离；管理员权限和审计生效；四类入口可在同一前端代码库中按域名/角色正确渲染；OEM 站点可按品牌配置渲染；直接调用未授权 API 返回 403。

### M2 Provider、模型与 Bifrost 文本网关

范围：Provider/凭据/模型目录/映射/价格版本、路由组、健康检查、嵌入 Bifrost SDK 的文本网关、OpenAI Chat/Responses、Anthropic Messages、基础流式和 fallback；公共开发者文档、curl/Python/Node.js 示例和按品牌/权限过滤的接入说明。

验收：同一公开模型可配置多个 Provider；429/5xx 可 fallback；流式开始后不切换；每次 attempt 可追踪；不支持参数返回结构化错误；文档示例可使用测试 Key 端到端跑通；OEM 文档展示品牌化 Base URL 和模型白名单。

### M3 钱包、预授权与 usage 结算

范围：钱包/额度账户、充值订单（先人工/兑换码）、预授权、结算、释放、退款、usage pending reconciliation、价格快照、客户账单和成本报表。

验收：并发请求不双扣；余额不足不调用上游；重复 usage/webhook 幂等；旧价格账单不变；退款和佣金冲正可重算。

M3 实现补充：网关在调用 Adapter 前通过 `billing.Reserve` 预授权；余额不足返回 `402 insufficient_balance` 且 `AdapterCalls` 不增加。缺 usage 时进入 `pending_reconciliation`，不按估算扣款。佣金先按批发价快照挂 `frozen` 流水，完整分销策略仍在 M6。

### M4 Seedance 视频与媒体任务

范围：火山方舟直连、OpenRouter、视频创建/查询/取消/回调/下载、图像生成/编辑、媒体参数、媒体预授权和单位计费、对象存储和 7 天清理。

验收：任务拿到上游 ID 后不重复提交；回调可重试且不重复结算；失败/取消释放正确预授权；结果只能通过签名 URL 获取。

M4 实现补充：`TOKENHUB_ARK_BASE_URL` + `TOKENHUB_ARK_API_KEY` 齐时火山方舟走 `POST/GET /contents/generations/tasks`；`TOKENHUB_OPENROUTER_BASE_URL` + `TOKENHUB_OPENROUTER_API_KEY` 齐时 OpenRouter 走 `/videos`。缺任一项仍走沙箱 TestAdapter。拿到 `upstream_job_id` 后只 Get、不重复 Create。Worker/API 轮询 in_progress；`GET /v1/videos/{id}` 也会刷新。终端态向客户 `callback_url` 投递 `X-Tokenhub-Signature`（HMAC `event_id|job_id`），失败退避最多 8 次。沙箱入站仍是 `POST /v1/media/callbacks`。媒体计费单位为 `video_seconds` / `image_count` / `audio_seconds`。`GET /v1/videos/{id}/content` 只返回 HMAC 签名路径。D3.2 任务模式不变。独立音频/转写/视频理解/复杂时间线仍是 P1。

### M5 套餐、订阅与支付

范围：平台及渠道套餐、token/video_second/image_count 等权益、赠送额度、月度订阅、自动续费、Stripe/支付宝/微信/人工入账/兑换码适配器、退款和对账。

验收：套餐发布自动校验和异常审核；权益按最早到期优先扣减；续费失败按重试/宽限期规则处理；支付 webhook 验签幂等；国内/国际支付订单都能发放和冲正权益。

M5 实现补充：`plans` 与 `payment` 模块各自 migration。渠道套餐低于 1 USD 底线、非 token 超额或视频秒数 > 3600 进入 `pending_review`。预授权先按「赠送即将到期 → 套餐即将到期」扣 `usd_credit`，差额才冻现金钱包。Stripe 沙箱可自动续费；支付宝/微信到期进入 `past_due`，不伪造代扣成功。续费失败按到期日、+1/+3/+5 天重试，7 天宽限后 `cancelled`。管理页 `/admin/plans` 可沙箱强制到期并手动续费扫描（生产禁止）。

### M6 分销、佣金与渠道运营

范围：渠道树 A/B/C、代理商与个人推广员、积分划转、两级分佣、营销冻结/已发放、人工打款、冲正。

验收：每个用户唯一归因；渠道额度不超发；佣金基于实际 usage；代理商只能看授权范围；退款同步冲正佣金；敏感身份脱敏且不暴露 prompt/completion。

M6 实现补充：以 `docs/15` 为准。种子树仍可保留 `acr_b_agent` → kol 演示码；计佣改为直接+间接。用户充值 1:1 从渠道积分池划转。无推广码不计佣。

### M7 运营、运维与上线加固

范围：Provider/模型/渠道/套餐/账务后台、指标看板、告警、审计检索、备份恢复演练、限流/熔断压测、支付和媒体异常演练、灰度发布。

验收：可按 Provider/模型/渠道/代理商/用户/API Key 查看成功率、延迟、错误、usage、成本、收入、毛利、佣金和待对账；关键故障有告警和 runbook；数据库可恢复到最近备份。

Ofox 公开目录预置：独立命令 `backend/cmd/catalog-seed`（`make catalog-seed`，Compose `docker compose --profile seed run --rm catalog-seed`）把嵌入的 `ofox-models.json` 写入 PostgreSQL。预置模型直接是 `status=published`、`sync_state=published`（已审核并已发布），创建人/审核人为空；只授权官方与分销渠道，不进 OEM 白名单；跳过空 id 与 `tokenhub/*`。命令幂等，先执行 migration。API `Seed()` 仍会调用同一导入，便于开发环境；不必先起 API 也能预置空库。

M2 实现补充：用户 API Key 支持 `allowlist`、`rpm_limit` 与 `concurrency_limit`；`GET /v1/me/api-keys` 回带白名单和限额。空白名单不限制；非空时 `GET /v1/models` 只列名单内模型，聊天不在名单内返回 `403 model_not_allowed`。RPM 默认 60，并发默认 5；占满并发槽返回 `429 rate_limited`。用户控制台可填写逗号分隔白名单、RPM 和并发。文本网关明确处理 `tools`/`tool_calls`、`response_format`（json_object/json_schema）、vision 多模态 content、以及 `reasoning`/`reasoning_effort`（usage 含 `reasoning_tokens`，按输出价计费）。`logit_bias` 等目录声明不支持的参数仍返回结构化 4xx。路由组 `strategy` 支持 `priority` / `weight` / `price` / `health`。权重按 candidate.weight 从高到低；价格按 Provider 成本价从低到高；健康优先把 `available` 排在 `degraded` 前面，并跳过 `unavailable`/`maintenance`。`provider.only` / `provider.ignore` / `provider.order` 仍可覆盖自动排序。一个 Provider 可挂多条上游账号（`catalog_provider_credentials`）：401 标 `invalid`，429 进入 30 秒 `cooldown`，池内无可用账号时跳过该 Provider。live 且 `TOKENHUB_BIFROST_SANDBOX=false` 时，进程环境 Key 与目录解密 Key 一并交给内嵌 Bifrost；请求 metadata 含 request_id/attempt_id/user/channel/model。Gemini 沙箱仍回声，live 走 Bifrost Gemini。Provider 同步先写入 `draft` 模型和价格，另一名 `ops_admin`/`platform_admin` `review` 后再 `publish` 进入客户目录；创建人不能审核或发布；`deprecate` 只改状态，不删历史映射和价格。

M7 实现补充：D23 管理角色有独立 bootstrap token（`{admin}-finance|ops|tech|audit`）。财务不能看 Provider 列表或轮换凭据；只读审计不能退款；技术值班可做健康检查但不能退款。文本请求的 `Idempotency-Key` 在 Redis 保存 24 小时。过期预授权由 Worker `ReapExpired` 回收。用户控制台可用登录会话创建媒体任务。API 进程内嵌 Bifrost SDK（默认 sandbox plugin 回声）与每日加密备份循环（保留 30 天）。`POST /admin/models` 手工创建公开模型（需二次确认，永远 `draft`）；`GET/PATCH /admin/models/{id}` 读取/改属性（public_id 可含斜杠）；管理页 `/admin/models` 列出后端目录并含「模型审核」队列（待审核/已通过待发布/已拒绝，通过、拒绝、发布分开），`/admin/models/{public_id}` 编辑属性与定价，上架为通过/拒绝/发布。创建人不能审核或发布自己建的模型。`POST /admin/models/attach` 把 Provider 挂到公开模型。用户 API Key 支持 `rotate` / `disable` / `expire` / `copy`，过期或禁用后网关返回 403。用户控制台创建 Key 可写模型白名单、RPM 与并发限额，`GET /v1/me/api-keys` 回显 allowlist/`concurrency_limit`；不在名单内聊天返回 `403 model_not_allowed`；占满并发槽返回 `429 rate_limited`。平台管理员可用 `POST /admin/api-keys/{id}/disable` 禁任意用户 Key（需二次确认；管理页 `/admin/keys`）。管理列表按 `limit`+`cursor` 分页，`GET /admin/billing/export`、`GET /admin/usage?format=csv`、`GET /admin/media?format=csv` 与 `GET /admin/api-keys?format=csv` 做导出。用户控制台可列出自己的媒体任务。上游 Base URL 走 allowlist，私网与元数据地址一律拒绝，生产只允许 https。OEM 域名 CNAME 到 `edge.tokenhub.local`，Caddy on-demand TLS 先问 `GET /v1/public/tls-check`。前端共享 shadcn/ui（Button/Input/Card）与 Radix Slot。独立 `ops` 模块。`GET /admin/ops/dashboard` 按 Provider/模型/渠道/代理商/用户/API Key 聚合网关请求和账务金额，总览含成功率、P50/P95、上游错误、余额风险、渠道消耗、佣金和待对账。`GET /admin/metrics/series` 与 `GET /admin/metrics/daily` 提供按日时间序列和账期 CSV 导出，ops 只通过网关/账务接口取数。用户控制台个人设置支持显示名、zh/en/ja 语言和改密，渠道仍不可自助切换。用户控制台与公共文档提供可复制的 curl/Python/Node 示例，自动带入品牌 Base URL 和模型白名单，不嵌入完整 Key。渠道控制台列出本渠道用户、套餐、推广链接、归因、用量和结算，并可用 `POST /channel/plans` 自建套餐（归属强制本渠道，低价进审核）。分销控制台 `/partner` 用 `GET /v1/partner/me|users|commissions|settlements` 按代理商/推广员 层级看不同范围。公共站 `GET /v1/public/models` 展示品牌模型目录，首页可兑换充值和订阅已发布套餐（未登录返回 403）。`POST /admin/ops/drills/tls` 与 `scripts/tls_drill.sh` 演练 OEM on-demand 门禁（已知域名 200 / 未知 404 / 沙箱 issued）；`TOKENHUB_ACME_DIRECTORY` + `scripts/e2e_acme.sh` 对 Pebble 走 RFC 8555（`.localhost` 默认不真签发）；公网 Let's Encrypt 仍由 Caddy 边缘节点对真实 DNS 完成，不把沙箱/Pebble 标成公网签发。看板维度含 P99、fallback、429/5xx、错误码分布、超时、Token/媒体用量，以及预授权失败次数和媒体回调 P95。`GET/PATCH /admin/ops/thresholds` 配置成功率/最少请求/待对账告警阈值。Provider `timeout_ms` 会套到上游调用，超时记 `error_code=timeout` 与 HTTP 408。API Key `rpm_limit` 走 Redis 滑动计数，超限 429。连续失败打开 Provider 熔断并跳过该候选。审计支持 action/resource/q 检索。`GET /admin/outbox/stats` 与 `POST /admin/audit-probes` 在管理页 `/admin/audit` 可读取 pending/published/failed 并写入沙箱探测（生产 403，不强制确认）。`POST /admin/ops/backup-drill` 记录 RPO 15 分钟 / RTO 1 小时演练；`scripts/backup_drill.sh` 可做 schema-only dump，`scripts/backup_encrypt.sh` 做 AES-256 异地加密备份，Compose Postgres 打开 WAL archive 以支持 PITR。支付伪造签名与媒体 force-fail 有演练入口。`X-Tokenhub-Canary` 或 100% 灰度切到指定 Provider。敏感写操作必须带 `X-Tokenhub-Confirm: 1`；管理员可启用 TOTP，启用后再带 `X-Tokenhub-TOTP`；设置页可读取/绑定/启用/关闭，关闭需确认。管理后台覆盖总览/提供商（列表每行健康探测，不会计费；`PATCH` 改状态/RPM 需二次确认）/模型/路由/API Key/用户（封禁、解封、人工改归因）/套餐审核/价格/支付/账务（退款、确认入账、赠送额度）/用量（`POST /admin/usage/replay` 回放待对账，需二次确认，Settle 幂等不双扣）/指标/告警/应急手册/渠道（额度发放；`disabled` 后聊天/媒体 `403 channel_disabled`，余额和历史仍可读）/推广码（`/admin/promos` 创建 acquisition 角色与推广码，需二次确认）/佣金策略（手工解冻、月结、人工打款）/审计（Outbox 统计与沙箱探测）/设置（运维开关：健康探测、打开/复位熔断、读写灰度；探测/熔断/灰度不强制确认头），看板用 Apache ECharts，文案走 next-intl（中/英/日），TypeScript Client 由 `backend/openapi/p0.yaml` 生成。`scripts/loadtest_limits.sh` 做限流/熔断压测。Compose 含 Prometheus 与 Grafana 业务/运行时 dashboard。P0 文本目录含 Google Gemini 沙箱适配器。

## 2. 依赖关系

```text
M0 -> M1 -> M2 -> M3
                 ├-> M4
                 ├-> M5
                 └-> M6
M2/M3/M4/M5/M6 -> M7
```

M2 可以先用测试 Provider 验证协议和路由；M3 完成后才允许真实客户流量。M4、M5、M6 可并行开发，但必须共享 M3 的账务和幂等能力。

## 3. 每个里程碑的完成定义

- 代码、migration、API 文档、配置示例和测试一起交付；
- 至少有单元测试、集成测试和一条端到端验收脚本；
- 账务、支付、媒体回调和权限变更必须有审计用例；
- 所有外部依赖支持 mock/sandbox，不以真实支付或真实媒体费用作为日常测试前提；
- 任何新增字段或状态必须同步更新 `05` 数据模型、`06` OpenAPI 和本开发拆分文档。
- 模块代码、数据表、migration 和事件发布接口按未来服务边界隔离；不得为了 P0 方便而引入跨模块表依赖或共享内部 ORM Model。

## 4. P0 完成后仍保留的 P1

- 更复杂的 workspace/project/TPM/lifetime 预算；
- 更复杂代理等级和自动打款；
- 更多媒体能力（独立音频、转写、视频理解、复杂时间线）。

企业级（有真实企业客户再立项，不进当前 P1）：Provider 数据策略、ZDR、按区域过滤路由。
