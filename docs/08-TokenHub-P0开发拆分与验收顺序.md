# TokenHub P0 开发拆分与验收顺序

> 本文件的 M7 实现补充保留历史验收记录。当前提供商密钥管理与模型发现以 [目录与网关重构方案](17-目录与网关重构方案.md) 为准：模拟同步与整池凭据轮换接口已移除。

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

M4 实现补充：`TOKENHUB_ARK_BASE_URL` + `TOKENHUB_ARK_API_KEY` 齐时火山方舟走 `POST/GET /contents/generations/tasks`；`TOKENHUB_OPENROUTER_BASE_URL` + `TOKENHUB_OPENROUTER_API_KEY` 齐时 OpenRouter 走 `/videos`。缺任一项仍走沙箱 TestAdapter。拿到 `upstream_job_id` 后只 Get、不重复 Create。Worker/API 轮询 in_progress；`GET /v1/videos/{id}` 也会刷新。终端态向客户 `callback_url` 投递 `X-Tokenhub-Signature`（HMAC `event_id|job_id`），失败退避最多 8 次。沙箱入站仍是 `POST /v1/media/callbacks`。媒体计费单位为 `video_seconds` / `image_count` / `audio_seconds`。`GET /v1/videos/{id}/content` 返回短期 S3 预签名 URL（Compose/CI 无云 Key 时走 MinIO）。对象存储必须走 S3 API；缺桶或失败返回错误，禁止回退本地盘并假装成功。D3.2 任务模式不变。独立音频/转写/视频理解/复杂时间线仍是 P1。

### M5 套餐、订阅与支付

范围：平台及渠道套餐、token/video_second/image_count 等权益、赠送额度、月度订阅、自动续费、Stripe/支付宝/微信/人工入账/兑换码适配器、退款和对账。

验收：所有套餐创建后待审核，发布后才可购买；创建、发布、下架、拒绝留操作人和时间；一次性套餐不进入续费，月度续费失败按重试/宽限期规则处理；权益按最早到期优先扣减；支付 webhook 验签幂等；国内/国际支付订单都能发放和冲正权益。

M5 实现补充：`plans` 与 `payment` 模块各自 migration。所有新套餐进入 `pending_review`。一次性购买额度长期有效，不进入续费扫描；包月、季付、年付套餐按各自周期补充额度。预授权先按「赠送即将到期 → 套餐即将到期」扣 `usd_credit`，差额才冻现金钱包。新购买当前提供手动续费；Stripe一次性checkout不产生未来扣款授权。仅保留确有授权支付方式引用的历史自动续费；没有授权的到期订阅进入相应到期/续费状态，不伪造代扣成功。续费失败按到期日、+1/+3/+5 天重试，7 天宽限后 `cancelled`。沙箱强制到期和手动扫描接口保留给运维测试，不放在套餐审核页。

### M6 分销、佣金与渠道运营

范围：渠道树 A/B/C、代理商与个人推广员、积分划转、两级分佣、营销冻结/已发放、人工打款、冲正。

验收：每个用户唯一归因；渠道额度不超发；佣金基于实际 usage；代理商只能看授权范围；退款同步冲正佣金；敏感身份脱敏且不暴露 prompt/completion。

M6 实现补充：以 `docs/15` 为准。种子树仍可保留 `acr_b_agent` → kol 演示码；计佣改为直接+间接。用户充值 1:1 从渠道积分池划转。无推广码不计佣。

### M7 运营、运维与上线加固

范围：Provider/模型/渠道/套餐/账务后台、指标看板、告警、审计检索、备份恢复演练、限流/熔断压测、支付和媒体异常演练、灰度发布。

验收：可按 Provider/模型/渠道/代理商/用户/API Key 查看成功率、延迟、错误、usage、成本、收入、毛利、佣金和待对账；关键故障有告警和 runbook；数据库可恢复到最近备份。

Ofox 公开目录预置：独立命令 `backend/cmd/catalog-seed`（`make catalog-seed`，Compose `docker compose --profile seed run --rm catalog-seed`）把嵌入的 `ofox-models.json` 写入 PostgreSQL。预置模型直接是 `status=published`、`sync_state=published`（已审核并已发布），创建人/审核人为空；只授权官方与分销渠道，不进 OEM 白名单；跳过空 id 与 `tokenhub/*`。命令幂等，先执行 migration。API `Seed()` 仍会调用同一导入，便于开发环境；不必先起 API 也能预置空库。

M2 当前实现：本人 Key 直接配置模型范围、累计 USD 上限和有效期，RPM/并发为高级项。原子预算预留/结算/释放/冲正，轮换不重置；有限额Key按合理预估准入，已执行请求可靠实际用量即使超预估也正常结算并允许负余额；未知定价/计量仍拒绝。公开请求禁止提供商和路由控制。Chat Completions 按实际模型和已部署适配器提供协议；Responses 与 Messages 只支持已实现的子集，未知或未支持参数返回结构化错误。公开模型的 supported_endpoints 是选择调用方式的依据。当前支持的 Agent 配置见 docs/20；Codex、Claude Code 未完成协议验收，不列为已支持 Agent。图像生成按真实能力提供，图像编辑当前关闭，不以生成冒充编辑。

M7 当前实现：员工按真实岗位进入平台/OEM任务工作区；普通账户邀请与收益在 `/app/referral`，旧 `/partner` 入口合并。平台用户 Key 管理已退役，本人读写严格所有权。OEM 收款和服务池按品牌，渠道无资金操作；资金操作记录原对象、金额、实际时间与确认，外部参考号可选，未知结果按原系统操作重试。汇总/搜索由服务端全范围查询，读取失败与真实零区分，直接 ID 与批量动作再次鉴权。运营诊断只在有权内部页；生产启动不安装测试回声 Harness。请求与账务日志只记录必要元数据，不默认持久化 prompt/completion；例外是标准 API 的幂等响应缓存：为原操作重试，完整响应正文在 Redis 保存 24 小时，访问按原操作身份控制。媒体素材另按对象存储保留策略处理。 本轮验证与未验证边界以 docs/21 和 docs/26 为准，旧里程碑不代表当前生产验收。

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
