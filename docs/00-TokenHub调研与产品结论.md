# TokenHub 调研与产品结论

版本：0.1
日期：2026-08-29
状态：讨论稿

## 1. 文档用途

本文不是最终开发说明，而是把外部产品能力和仓库现有分销基线整理成可决策的产品边界。结论分为三类：

- 已确认：可以从公开资料和当前 TokenHub 需求文档直接得到。
- 建议：结合 TokenHub 的业务目标给出的第一版方案。
- 待确认：会影响账务、权限或交付顺序，需产品方逐项拍板。

## 2. 外部调研摘要

### 2.1 OpenRouter 可借鉴能力

根据 OpenRouter 官方文档，平台的核心抽象不是简单的“转发 URL”，而是“统一模型目录 + 提供商路由 + 统一用量/余额”。对 TokenHub 有直接参考价值的能力如下：

1. 统一模型目录：模型对象包含 `id`、上下文长度、架构能力、价格、顶级提供商能力、限制和支持参数；价格不仅有输入/输出 token，还可能包含 reasoning、缓存读写、按请求计费以及条件价格覆盖。
2. 提供商路由：请求可以按提供商顺序、价格、吞吐、延迟、允许/排除列表、参数支持、数据策略和 ZDR 约束路由；默认策略会考虑近期故障并优先低价候选。
3. 模型级 fallback：主模型失败时可以按顺序尝试备用模型；失败原因包括限流、宕机和内容审核拒绝。官方文档当前限制最多 3 个 fallback 条目。
4. BYOK：用户自己的提供商 Key 会被加密保存；同一提供商可以配置多把 Key，并支持主 Key、备用 Key、模型过滤和共享容量 fallback。
5. 预算：工作空间支持 daily/weekly/monthly/lifetime 预算，命中任一预算后在路由前阻断请求；预算有“越短周期越小”的严格递减约束，并可选择是否计入 BYOK 消耗。
6. 隐私策略：不同提供商有独立的数据训练和保留政策，路由时可以按数据政策过滤；企业场景还支持区域路由。
7. 计费口径：请求完成后根据提供商返回的 token 用量和模型价格扣除余额；模型可能使用不同 tokenizer，不能用一个全局 token 估算值代替上游 usage。

### 2.2 Sub2API 可借鉴能力

根据 `Wei-Shaw/sub2api` 仓库 README 及其公开文档，值得借鉴的能力包括：

- 多种上游账号：OAuth、API Key，以及按平台区分的账号池。
- 平台 API Key 发放与管理，可对 Key 做禁用和使用限制。
- token 级用量计费、账户选择、粘性会话、并发控制和 RPM/TPM 限流。
- 复合组：把一个面向客户的模型/套餐映射到多个具体提供商和上游模型。
- 管理后台、支付集成、请求日志和错误归因。
- 失败账号的临时摘除：401 视为凭据无效，403 视为访问/权益失败，429 根据 `Retry-After` 或冷却时间暂时摘除。
- 对上游没有显式配额时不臆造配额；例如其 xAI 实现只记录上游返回的白名单限额 Header，未知时显示 unknown。
- 安全配置：URL allowlist、HTTPS 约束、可信代理 IP、响应 Header 过滤、计费熔断器等。

### 2.3 本地分销基线

现有分销需求文档已归并为以下基线，本项目后续文档统一继承：

- 渠道主体分为平台直推 A、独立分销批发商 B、OEM 批发商 C。
- 一个自然人账号只能归属一个渠道组织；同渠道内可同时拥有代理商、1 级 KOL、2 级 KOL 等多个角色。
- 代理商可发展 1 级 KOL；1 级 KOL 可发展 2 级 KOL；2 级 KOL 不再发展下级。
- 终端用户注册时绑定唯一推广来源；人工改归属必须有平台管理员审计记录。
- 佣金基数是终端用户实际 API 消耗金额，按平台批发价计算，不是充值金额。
- 佣金先冻结，支持 available、settled、reversed、cancelled 等状态；退款需要原流水关联的冲正。
- 第一版不做无限多级分销、自动对外打款、复杂 CRM/合同/发票和多来源拆分归因。
- B/C 可以配置渠道内代理商等级和结算方式；OEM 需要独立 Logo、配色和域名。

### 2.4 同类网关产品补充

本轮补充查看 LiteLLM、Portkey 和 Helicone 的公开文档，得到三点设计启发：

1. LiteLLM 的预算层级覆盖 proxy、team、user、virtual key 和 model，并区分预算重置周期、RPM/TPM 和模型访问权限；其文档特别提醒，预算 enforcement 依赖持久化数据库，DB-less 模式不能作为可靠的消费上限。这支持 TokenHub 将数据库账本作为限额事实源，Redis 只做并发/短窗口加速。
2. Portkey 将 Virtual Keys 演进为 Model Catalog：一个客户 Key 可以访问多个提供商，但模型目录负责集中治理、Key 轮换和模型级控制。这与 TokenHub 的“API Key + 模型目录 + 路由组”结构一致。
3. Helicone 将限流做成全局、按用户、按任意业务属性三种 scope，并支持按请求数或成本单位限制。TokenHub 应支持按 API Key、用户、项目和渠道的成本限额；终端用户标识应从已认证上下文取得，不能只信任客户端自报 Header。

这些产品普遍把“可观测性”和“控制面”放在网关旁边，而不是把策略硬编码在每个提供商适配器中。TokenHub 应保持同样的边界：适配器负责协议和 usage 解析，控制面负责价格、预算、路由和分销政策。

### 2.5 视频与多媒体调研结论

本轮重点核查 Seedance、OpenAI Sora 以及 OpenRouter 的视频接口：

- OpenAI 和 OpenRouter 都采用视频任务异步模式：创建任务后返回 ID，客户端查询状态/进度，完成后再下载内容；OpenRouter 还支持签名 webhook。
- OpenRouter 的视频请求支持文生视频、首帧/首尾帧、参考素材、分辨率、宽高比、时长和生成音频；其视频模型目录独立暴露支持的分辨率、宽高比和 pricing SKU。
- 火山方舟 Seedance 接入返回任务 ID，任务完成后可能同时返回 video URL、usage token、时长、比例、分辨率、帧率、音频和结果过期时间。说明媒体计费可能同时依赖 token、秒数、分辨率和音频开关。
- 第三方媒体平台通常也采用 submit -> poll -> download/callback 队列模式；这不是聊天接口的简单扩展，必须有任务、资产、回调重试和结果保留模型。

因此，Seedance 等视频模型放入 P0 时，P0 的核心不是一次性支持所有视频厂商，而是先交付稳定的“媒体任务引擎”和一个可验证的 Seedance 适配器。后续新增 Sora、Veo、Kling 等模型时复用同一任务协议和账务模型。

## 3. 产品定位

TokenHub 是面向开发者和渠道商的多模型 API 中转与分销平台：

> 对终端开发者提供 OpenAI/Anthropic 等兼容接口；对平台运营方提供模型提供商、模型价格、余额、用量、计费和风控控制；对代理商提供隔离的推广、额度和佣金结算体系。

第一版按 Sub2API 风格优先交付中转站核心能力，不把 OpenRouter/LiteLLM 的企业 workspace 控制面全部搬进来。第一版的竞争力不应是“模型数量最多”，而是：

1. 同一模型多提供商的可用性和可解释路由。
2. 余额和成本的可审计性，避免因重试、流式响应、退款产生账不平。
3. B/C 渠道可以独立运营，但平台仍能控制额度、价格和最大佣金风险。

因此 P0 的限制模型收敛为“用户余额 + API Key 限流 + B/C 渠道额度”；workspace/project 多层预算、复杂继承、BYOK 预算和企业区域预算列为后续扩展。

## 4. 关键产品决策建议

| 决策 | 建议第一版 | 原因 |
| --- | --- | --- |
| 计费主账本 | 预付余额 + 不可变账务流水 | 最容易做幂等、对账和冲正 |
| 面向客户的价格 | 平台销售价按模型/计费项配置；可选渠道价覆盖 | 与上游成本、代理佣金解耦 |
| 上游成本 | 记录请求实际 provider、model、价格版本和 usage 快照 | 价格变更不能改写历史账单 |
| 余额扣款 | 请求前预授权，收到最终 usage 后结算，多余部分释放 | 避免并发超扣和余额透支 |
| 失败重试 | 同一业务请求只产生一笔客户账单；每次上游尝试单独记录成本 | 兼顾用户体验与真实成本 |
| 渠道额度 | 以“按批发价折算的累计 API 消耗成本”为口径 | 与现有 V2.1 一致，避免把充值额误作用量 |
| 佣金 | 消耗结算事件产生冻结佣金；退款/纠错产生反向流水 | 防止充值套佣和账务不可追溯 |
| 钱包币种 | 内部统一 USD 定点记账，展示层可换算 CNY/JPY | 上游模型价格多以 USD/token 表示 |
| 对外打款 | 第一版只做站内余额、批发商手动扣款、线下标记 | 降低支付和合规风险 |
| 兼容协议 | P0 OpenAI Chat Completions + Responses + Anthropic Messages；其他协议按适配器扩展 | 先覆盖主流 SDK 和编码工具 |

## 5. 必须先讨论的矛盾或空白

1. V2.1 同时出现“实时计算冻结佣金”和“每日根据消耗扣款”，需要确定是实时事件记账、日终对账，还是实时预估 + 日终结算。
2. “平台批发价”“批发商可见批发价”“终端销售价”是三套价格，不能只用一个 `price` 字段。
3. 失败重试、流式中断、上游返回 usage 缺失、图片/音频按请求计费时，计费基数不同，需要定义统一的 usage 事件协议。
4. 代理商管理奖励和渠道佣金由谁承担，需要避免“平台毛利为负但佣金仍成功发放”。
5. B/C 的预充值余额既是服务成本账户，又可能承担推广者佣金支付；是否拆分营销子账户会影响扣款优先级和停服规则。
6. 自然人“不可跨渠道”需要手机号、邮箱、OAuth subject 还是实名主体作为唯一键，需明确反滥用策略。
7. OEM 独立域名涉及证书、DNS、租户识别和品牌资源安全，但已确认纳入 P0，采用 CNAME + 自动 HTTPS；实现上与核心网关模块解耦。

本节中的矛盾和空白是调研初稿记录。经过 D1-D32 逐项讨论后，最终以 `docs/01-TokenHub统一产品需求文档.md`、`docs/04-TokenHub逐项讨论清单.md` 及其关联设计文档为准。

## 6. 外部来源

- OpenRouter Pricing: https://openrouter.ai/pricing
- OpenRouter FAQ: https://openrouter.ai/docs/faq
- OpenRouter Models: https://openrouter.ai/docs/guides/overview/models
- OpenRouter Provider Routing: https://openrouter.ai/docs/guides/routing/provider-selection
- OpenRouter Model Fallbacks: https://openrouter.ai/docs/guides/routing/model-fallbacks
- OpenRouter BYOK: https://openrouter.ai/docs/guides/overview/auth/byok
- OpenRouter Management API Keys: https://openrouter.ai/docs/guides/overview/auth/management-api-keys
- OpenRouter Workspace Budgets: https://openrouter.ai/docs/guides/features/workspaces/workspace-budgets
- OpenRouter Provider Logging: https://openrouter.ai/docs/guides/privacy/provider-logging
- Sub2API: https://github.com/Wei-Shaw/sub2api
- Sub2API Composite Groups: https://github.com/Wei-Shaw/sub2api/blob/main/docs/COMPOSITE_GROUPS.md
- Sub2API Payment: https://github.com/Wei-Shaw/sub2api/blob/main/docs/PAYMENT.md
- Bifrost AI Gateway: https://github.com/maximhq/bifrost
- Bifrost Provider 配置: https://docs.getbifrost.ai/quickstart/gateway/provider-configuration
- Bifrost 重试与 fallback: https://docs.getbifrost.ai/features/retries-and-fallbacks
- Bifrost 观测能力: https://docs.getbifrost.ai/features/observability/default
- Bifrost OpenAI Provider: https://docs.getbifrost.ai/providers/supported-providers/openai
- LiteLLM Budgets and Rate Limits: https://docs.litellm.ai/docs/proxy/users
- LiteLLM Virtual Keys: https://docs.litellm.ai/docs/proxy/virtual_keys
- Portkey Virtual Keys / Model Catalog: https://docs.portkey.ai/docs/product/ai-gateway/virtual-keys
- Helicone Custom Rate Limits: https://docs.helicone.ai/features/advanced-usage/custom-rate-limits
- OpenRouter Video Generation: https://openrouter.ai/docs/guides/overview/multimodal/video-generation
- OpenAI Video Generation: https://developers.openai.com/api/docs/guides/video-generation
- 火山方舟 Seedance 视频生成: https://docs.volcengine.com/docs/82379/1520757
