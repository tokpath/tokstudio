# T09：API Key 限制与模型使用说明实施记录

日期：2026-10-10。最新实际结算批次分支 `codex/actual-key-accounting`，基线 `627554f`；早期 T09 分支 `codex/key-limits`，基线 `6c2e164`。本记录按最新已确认业务决定更新，不替代项目整体放行。

## 最新业务规则：预估准入，实际结算

- 请求前按有效品牌终端价与已支持计费维度估算费用；同一数据库事务锁定逻辑 Key、账户钱包及适用 USD 套餐，核验累计 Key 剩余和账户净可用额并预留。账户净额包含负钱包，不能将欠额截成零后再用套餐放行；钱包为零且有效套餐足够仍可正常准入。
- 已准入且实际用量可靠的请求全额 `confirmed`，即使超过预估或之后下调的 Key 上限，仍按原品牌、用户、Key、模型和价格快照结算及计佣。实际候选成本独立保存。Key 已用可以超过上限，剩余可为负；现金余额可以因实际结算为负，预留、赠送及套餐计数保持非负。
- 超预估先继续消费尚未被其他请求占用的适用套餐和赠送余额，不足部分归充值余额。其他请求的已占用权益不会被挪用。授权保留最终套餐和赠送来源，真实冲正按来源返还；原有赠送不退规则保持。
- 新请求必须有足够净可用额覆盖估算，并满足当时 Key 限额。已达限或降限后无剩余的 Key 拒绝新请求；修改名称、模型、到期和正数上限不受历史超用量阻碍。充值自然补齐负钱包，但不重置 Key 用量；没有新增债务、催收、OEM 采购欠款或追收工作流。弃用的小额余额差额由原经营品牌承担，经营报表另由主工作流实现。
- 缺失必要用量维度、负数用量、超时或结果未知继续 `pending_reconciliation` 并保留占用。预估不用于制造实际消费。晚到可靠事实可以正常结算，旧待处理事实保留为 voided 历史，原 charge 对应新的 confirmed 事实；不同实际事实重放返回冲突。
- OEM 已有发放 allocation 只记实际可追溯的来源消耗，实际超额不凭空增加 allocation 或采购负债；完整终端消费、OEM 结算价、负钱包仍在原 charge/usage 可查。冲正只返还实际消费的来源额度。
- 媒体任务保存接受执行的候选成本及上游模型；回调、轮询结算不依赖当前价目仍发布。已准入后下架/改价不阻断结算，也不改用新价格。可靠结算用稳定 `media:<原任务ID>:accepted` 写入原供应商成本，重复回调只落一次，客户退款保留实际成本。缺少原候选成本的旧任务不猜补当前供应商价格。混合 Token 媒体价目缺少原价要求的实际 Token 维度继续 pending，不能按零收费确认。

本批新增 billing `0017_actual_usage_wallet.sql`，允许实际消费后的充值余额为负，保存最终套餐来源并保持预留、赠送、佣金桶非负；media `0005_actual_candidate_costs.sql` 保存执行候选成本。迁移不重算历史消费、旧价格或旧供应商成本。

本批验证：billing/catalog/gateway/identity/plans/media 与完整 app 回归通过（独立 PostgreSQL、Redis DB2、MinIO，app 43.077s）；新增 `TestActualKey*` 覆盖现金 .10 预估/.12 实扣、套餐/赠送超预估与并发来源、净负余额和有效权益准入、Key 降限/充值不清用量、同账户不同 Key 并发、OEM allocation 耗尽与精确冲正、媒体下架后回调/缺用量、未知收费配置、Chat/Responses/Messages/vision/stream。Key 与钱包前端 3 文件/34 用例及 TypeScript 通过，超用量和负余额按真实值展示。旧验证记录下方保留其历史批次范围；最终整合回归由根工作流执行。

## 已实现及验证的核心行为

- `/app` 与 `/app/keys` 复用 Key 主视图。创建和编辑直接配置所有/指定多个模型、累计 USD 上限及长期/指定到期；RPM、并发折叠。零余额可创建、阅读和复制说明，测试调用为主动执行且提示费用。
- 显式 `model_mode=all|selected`，指定集合为空拒绝保存，读取策略失败拒绝认证。Key 与模型策略同事务写入。普通用户管理、复制、轮换、启停和过期操作均验证本人所有权，平台角色不能通过个人入口绕过。
- 创建携带稳定 `operation_id`；同用户、同操作、相同归一参数返回同一逻辑 Key，不同参数返回 409。前端网络结果未知冻结原配置，恢复时复用原提交，不新建操作；明确成功后立即展示 Key，回读失败不冒充创建失败。切用户/品牌丢弃旧响应、明文和草稿。
- USD 上限以逻辑 Key 的累计净终端 API 消费计数。充值、赠送及套餐抵扣都计入；充值和轮换不重置，轮换不重新启用。已确认消费真实冲正后恢复 Key 用量，赠送是否退回仍遵循原钱包规则。
- 数据库请求锁与 Key 行锁覆盖预留、限制编辑、套餐消费和钱包账务。套餐预留/退回使用账务同一事务。同请求重放绑定原用户、Key、模型、品牌、金额、价格版本与快照，已结算请求不能重新授权调用；并发同请求只消费一次。
- 结算强制保留原预授权用户、Key、公开模型和品牌资金池。终端/批发/价格版本保持原快照，实际成功候选的内部成本字段单独白名单合入；公开请求不能提供成本。新原生事实区分 completion 已包含 reasoning，避免重复计价；未标记历史事实保持原口径。
- 正常结算将占用转已用；确定失败释放；缺用量、超时或未知媒体结果保留占用。预授权过期成为可查询的待对账用量事实及 outbox 事件。重复结算、释放、冲正不重复计数。
- 超出预授权的可靠实际用量不截断，按本记录最新规则全额结算；相同事实重放不重复收费或计佣，不同测量事实冲突拒绝。
- 公开 Chat、Messages、Responses 与媒体入口拒绝 provider/route/routing/retry/灰度及诊断控制的查询参数、顶层 JSON 字段和已支持诊断头，大小写及 Content-Type 不构成绕过；消息、工具定义和普通正文中的同名字段不扫描。独立 `/admin/diagnostics/*` 需要有权 staff session 和本人测试 Key。
- 用户保留本人请求、用量及计费状态查询，公开响应和回单不包含内部提供商、上游、路由及成本；详细诊断保留在内部授权入口。
- 公共模型目录提供按公开 ID 稳定排序的游标分页；实际 HTTP 103 个发布、定价、路由、授权模型已验证两页完整无重复。Key 模型选项完整读取，保留已选但下架 ID，加载失败不当零模型。
- 模型说明共用 `ModelUsagePanel`：当前品牌真实 Base URL、公开模型 ID、实际适配能力、价格及安全服务状态。协议/Agent/Key/模型上下文与充值回程保留；无 Key 或未登录也可阅读。Key 列表读取失败显示重试，不显示假空列表或原始失效 Key ID。
- 缺失或畸形品牌 API 域名返回明确不可用，不回退平台地址；只接受 host[:port]，拒绝 URL scheme、userinfo、路径、query、fragment 及无效端口。真实 OEM 与本地监听地址保留。协议能力覆盖运行时的明确 text adapter 别名（含 openai/openrouter），不按模型 vendor 推断；实际 ModelView 与独立能力回归均覆盖。
- `/docs`、`/docs/integrations`、公共模型详情与 `/app/docs` 复用真实说明，`/integrations` 进入同一公开入口；从用户目录详情进入同一说明。代码仅使用环境变量 Key，秘密不进入 URL 或示例。
- cURL、Python 标准库 urllib 与 Node 原生 fetch 示例直接执行本地 HTTP 验证，不安装 SDK、不以字符串快照充当执行验证。

## 成本预估能力与边界

已撤销早期为了绝对硬上限建立的官方主机、模型族和视觉输入白名单。已部署文本 adapter（Bifrost/OpenAI/Anthropic/OpenRouter/Gemini/Google 及测试 adapter）使用同一合理估算准入；不再以不设 USD 上限绕过估算配置缺失。公开能力 `budget_estimate_supported` 与有效品牌价一致，旧 `budget_control_supported` 字段保留为兼容别名，含义同为可建立合理估算，不承诺上游绝不超报。

文本按约每三个文本字节一个 Token、消息/工具 framing、每图 1024 Token、请求/默认输出参数和 reasoning 维度加缓冲估算。图像按张数、视频按请求时长/音频及分辨率估算；混合媒体价目也计入已知文本维度。真实输出和图像 Token 数可能不同，可靠实际用量仍全额结算。未知非零收费维度、非法/缺失售价或非 USD 价格返回 `price_estimate_unavailable`，属于品牌定价配置问题；用户不应被引导删除 USD 上限绕过。已支持的收费维度是 input/output/reasoning、image_count/video_second/audio_second；未知维度为零可保留，但不宣称实现该维度收费。

估算能力不增加协议兼容能力，仍由实际 adapter/公开 supported_endpoints 判断。模型被临时下架或路由不可用的既有请求不受影响，新请求继续按原就绪和授权校验。

当前不承诺绝对硬上限，也不读取或验证生产供应账户。适配器发送真实输出参数的既有本地 HTTP 回归保留，但不会把该参数或 mock 捕获当成供应商不会超额的证明。

依据：[OpenAI Chat 参数](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create)、[Anthropic thinking 与输出上限](https://platform.claude.com/docs/en/build-with-claude/extended-thinking)。[OpenRouter Chat 参数](https://openrouter.ai/docs/api/api-reference/chat/create-a-chat-completion) 明确 `max_completion_tokens`，将 `max_tokens` 标为弃用；[OpenRouter reasoning](https://openrouter.ai/docs/guides/best-practices/reasoning-tokens) 说明两者共享可见输出与推理预算，并单列 Anthropic 总输出须高于 reasoning budget 的约束。这里只适用于上述明确模型族，不扩展到所有兼容服务。OpenRouter 最小输出可能为 16，平台拒绝低于 16 的请求，不自动提高用户上限。

Bifrost 当前 OpenRouter SDK 把 URLPath 拼到固定 origin，不能正确执行配置的完整 Base URL；既有原生 Chat HTTP 适配继续经过 TokenHub 预授权/请求/attempt/结算。本地 HTTP 测试证明出站字段和真实用量解析；供应商实际报告超过预估仍按最新规则全额结算，只有结果未知或缺用量才待对账。

## Agent 与协议边界

可复制说明提供通用 OpenAI-compatible 字段与 Cline 的实际设置位置、兼容类型、品牌 `/v1` 地址和公开模型 ID。真实 OpenAI HTTP 上游两轮测试已验证 tools、assistant tool_calls、tool_call_id 与工具结果内容不丢失。依据：[Cline 官方配置](https://docs.cline.bot/provider-config/openai-compatible)。

核心提交 `d7b71ab` 时 Chat SSE 的工具保真仍在进行。后续必要批次已补 delta 中 tool_calls 的 index、ID、函数名和参数，finish_reason 与 usage chunk；真实 OpenAI 本地 HTTP 两轮工具往返及 SSE 数据断言通过。网关将 SSE 的 model 投影为原请求公开模型 ID，不泄漏实际上游模型。当前桥接为上游响应完成后输出 SSE，不是实时逐 Token 透传。尚未宣称完成安装的 Cline 客户端端到端验收。

SDK 没有明确 HTTP status 或返回真实 5xx，OpenRouter 返回 5xx/超时/无法解析结果，均标为结果未知并保留原请求与待对账占用；成功响应无用量不造计量。确定的参数拒绝/429 仍可按既有失败规则处理。测试覆盖真实适配器未知分类、空用量与低于最小输出拒绝。

Responses 当前支持字符串 input、非流式文本 JSON 输出、输出上限及用量；高级输入、会话续接、工具生命周期、存储和事件流明确拒绝，未实现检索/取消等完整生命周期。Messages 当前支持字符串消息/system、工具定义转换及工具输出；原生内容块、工具结果往返、thinking 与 Messages 事件流未实现并明确拒绝。它们是部分协议能力，不能据简易 POST 成功宣称完整 Agent 接通。

Codex 与 Claude Code 未列为正常可用 Agent 选项，说明分别指出 Responses 事件流/工具与生命周期、Messages 内容块/事件流/工具结果缺口。此轮按根代理确认不扩建完整协议生命周期。依据：[Codex 配置](https://developers.openai.com/codex/config-reference)、[Claude Code gateway](https://code.claude.com/docs/en/llm-gateway)。不使用模型 vendor 猜协议。

实际图像适配器只实现 `/images/generations`，原 `/images/edits` 是假编辑行为：本批明确拒绝且不建任务/扣额度，公开能力不再声明 edits，辅助媒体 UI 标明禁用。保留真实图像生成和对象存储验证。

## 迁移与合并点

1. identity `0015_key_limits.sql`：旧有策略集合迁为指定，空策略迁为全部；预算无限，原到期保持。新增逻辑 Key 计数。
2. billing `0013_key_budget_facts.sql`：从原请求/用量补逻辑 Key 与公开模型关联，按原已确认收费及未结预留恢复计数，不改历史消费金额或价格。
3. media `0004_key_budget_backfill.sql`：补历史媒体 Key/模型关联后恢复计数。应用迁移顺序已有 gateway、billing、media 依赖顺序。
4. identity `0019_key_create_operations.sql`：稳定创建操作；0016/0018 留给 OAuth，0017 留给 OEM。billing0014 留给主工作流额度操作。

主工作流另修公共 `ModelView` 就绪/脱敏、有效品牌售价、临时不可用目录保留与单目标 `GetVisibleModel`。合并时保留这些投影，并结合本批实际协议及预算能力；能力需按有效品牌价计算，不以基础价覆盖品牌能力。前端已读取安全 `service_status`，缺少值显示未知，不一律绿色可用。

## 验证状态与剩余项

- 隔离 Go 容器执行 `go test ./... -count=1` 全通过（2026-10-10 核心批次），包括独立 PostgreSQL、Redis DB2 与 MinIO 的真实对象存储分支。
- 必要收尾批次定向执行 catalog/gateway/app：协议别名与实际 ModelView、真实 HTTP 两轮工具 SSE、公开模型 ID、未知 5xx/空用量/小输出拒绝、品牌地址与未知计费占用回归通过；核心全量结果仍为上述批次。
- 离线 Node 容器执行 `npm test`：79 文件、408 用例通过；`tsc --noEmit` 通过。未删计费断言；旧内部控制测试迁至独立诊断，公共安全另验证。旧超预留夹具改为正常有界调用，超额事实由专门回归保护。
- G20/G26/G27/G29：零余额创建、原子模型策略、并发限额/套餐、编辑占用边界、到期/停用/轮换、未知/重复/冲正、全公开入口内部控制和正确恢复动作均有自动化证据；G18 创建未知复用同操作、成功后回读失败与旧对象晚响应有回归。
- G09 原预授权品牌/用户/Key/模型/价格快照及实际候选成本保存；G14 真品牌地址与可执行示例；真实 HTTP >100 模型目录有专门回归。
- 浏览器宽度/键盘/完整角色任务与真实初用者可用性测试由根工作流集成后继续验收；尚未声称完成这些验收。最终 Next 生产构建须在合入主工作流离线字体/导航后执行。
- 早期“超预估财务处置待定”已被本记录最新业务规则取代，不再以 pending 阻断可靠实际用量结算。docs/03 原“预授权过期释放”仍不能应用于已提交且结果未知的请求；确定失败才释放。
- 后续兼容增强仍是完整 Responses/Messages Agent 生命周期和真实图像编辑。本次估算规则不扩建这些协议。
