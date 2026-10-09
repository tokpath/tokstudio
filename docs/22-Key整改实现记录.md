# T09：API Key 限制与模型使用说明实施记录

日期：2026-10-10。隔离分支 `codex/key-limits`，基线 `6c2e164`（包含远程 `release/v0.1.0@64e1825`）。依据 docs/18、19、20 及本轮最新业务决定。此记录只覆盖 T09，不替代项目整体放行。

## 已实现及验证的核心行为

- `/app` 与 `/app/keys` 复用 Key 主视图。创建和编辑直接配置所有/指定多个模型、累计 USD 上限及长期/指定到期；RPM、并发折叠。零余额可创建、阅读和复制说明，测试调用为主动执行且提示费用。
- 显式 `model_mode=all|selected`，指定集合为空拒绝保存，读取策略失败拒绝认证。Key 与模型策略同事务写入。普通用户管理、复制、轮换、启停和过期操作均验证本人所有权，平台角色不能通过个人入口绕过。
- 创建携带稳定 `operation_id`；同用户、同操作、相同归一参数返回同一逻辑 Key，不同参数返回 409。前端网络结果未知冻结原配置，恢复时复用原提交，不新建操作；明确成功后立即展示 Key，回读失败不冒充创建失败。切用户/品牌丢弃旧响应、明文和草稿。
- USD 上限以逻辑 Key 的累计净终端 API 消费计数。充值、赠送及套餐抵扣都计入；充值和轮换不重置，轮换不重新启用。已确认消费真实冲正后恢复 Key 用量，赠送是否退回仍遵循原钱包规则。
- 数据库请求锁与 Key 行锁覆盖预留、限制编辑、套餐消费和钱包账务。套餐预留/退回使用账务同一事务。同请求重放绑定原用户、Key、模型、品牌、金额、价格版本与快照，已结算请求不能重新授权调用；并发同请求只消费一次。
- 结算强制保留原预授权用户、Key、公开模型和品牌资金池。终端/批发/价格版本保持原快照，实际成功候选的内部成本字段单独白名单合入；公开请求不能提供成本。新原生事实区分 completion 已包含 reasoning，避免重复计价；未标记历史事实保持原口径。
- 正常结算将占用转已用；确定失败释放；缺用量、超时或未知媒体结果保留占用。预授权过期成为可查询的待对账用量事实及 outbox 事件。重复结算、释放、冲正不重复计数。
- 超出预授权的真实用量不截断。保留完整事实、原钱包预留和明确待对账状态，Key 占用按测得金额计入并阻止后续越额。相同事实重放不增加占用，不同测量事实冲突拒绝，不重复出 outbox。
- 公开 Chat、Messages、Responses 与媒体入口拒绝 provider/route/routing/retry/灰度及诊断控制的查询参数、顶层 JSON 字段和已支持诊断头，大小写及 Content-Type 不构成绕过；消息、工具定义和普通正文中的同名字段不扫描。独立 `/admin/diagnostics/*` 需要有权 staff session 和本人测试 Key。
- 用户保留本人请求、用量及计费状态查询，公开响应和回单不包含内部提供商、上游、路由及成本；详细诊断保留在内部授权入口。
- 公共模型目录提供按公开 ID 稳定排序的游标分页；实际 HTTP 103 个发布、定价、路由、授权模型已验证两页完整无重复。Key 模型选项完整读取，保留已选但下架 ID，加载失败不当零模型。
- 模型说明共用 `ModelUsagePanel`：当前品牌真实 Base URL、公开模型 ID、实际适配能力、价格及安全服务状态。协议/Agent/Key/模型上下文与充值回程保留；无 Key 或未登录也可阅读。Key 列表读取失败显示重试，不显示假空列表或原始失效 Key ID。
- 缺失或畸形品牌 API 域名返回明确不可用，不回退平台地址；只接受 host[:port]，拒绝 URL scheme、userinfo、路径、query、fragment 及无效端口。真实 OEM 与本地监听地址保留。协议能力覆盖运行时的明确 text adapter 别名（含 openai/openrouter），不按模型 vendor 推断；实际 ModelView 与独立能力回归均覆盖。
- `/docs`、`/docs/integrations`、公共模型详情与 `/app/docs` 复用真实说明，`/integrations` 进入同一公开入口；从用户目录详情进入同一说明。代码仅使用环境变量 Key，秘密不进入 URL 或示例。
- cURL、Python 标准库 urllib 与 Node 原生 fetch 示例直接执行本地 HTTP 验证，不安装 SDK、不以字符串快照充当执行验证。

## 有限额 Key 的可执行预留契约

有限额不是入口处读一次历史汇总的软提示。当前可证明的纯文本路径为 Bifrost 对官方 OpenAI Chat、官方 Anthropic Messages，以及 OpenRouter 的明确 `openai/*`、`anthropic/*` 模型。校验是适配实现和实际官方端点契约，不能靠可编辑能力布尔开启任意兼容服务器。

实际出站测试确认 OpenAI/OpenRouter 发送 `max_completion_tokens`，Anthropic 发送 `max_tokens`；输出 ceiling 包含隐藏推理。平台显式发送默认/请求输出上限，以完整序列化输入字节及协议 framing 建保守输入边界，并按有效品牌终端价预留输入、输出、reasoning 各可能收费维度和缓冲。未知的收费维度、无法建立输入上界的视觉输入及未验证路径明确拒绝有限额 Key。异常上游违约超报仍进入上述待对账流程。

固定张数/时长的已支持图像生成与视频任务仅在有效价格维度可证明有界时支持限额；含 Token 等无法证明上界的媒体价目拒绝限额。公开能力与 Key 创建表单列出当前支持的模型，选择所有模型不会伪称每条路径均支持。

当前不承诺任意第三方 OpenAI-compatible URL、原生 Gemini、OpenRouter Gemini 或动态 router 的硬上限。这些模型仍可按已有正常协议使用不设单独 USD 上限的 Key，并受账户额度约束。配置必须由平台将实际已验证路径接入；本工作流未读取或验证生产供应账户，不声称部署后的每个模型都支持限额。

依据：[OpenAI Chat 参数](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create)、[Anthropic thinking 与输出上限](https://platform.claude.com/docs/en/build-with-claude/extended-thinking)。[OpenRouter Chat 参数](https://openrouter.ai/docs/api/api-reference/chat/create-a-chat-completion) 明确 `max_completion_tokens`，将 `max_tokens` 标为弃用；[OpenRouter reasoning](https://openrouter.ai/docs/guides/best-practices/reasoning-tokens) 说明两者共享可见输出与推理预算，并单列 Anthropic 总输出须高于 reasoning budget 的约束。这里只适用于上述明确模型族，不扩展到所有兼容服务。OpenRouter 最小输出可能为 16，平台拒绝低于 16 的请求，不自动提高用户上限。

Bifrost 当前 OpenRouter SDK 把 URLPath 拼到固定 origin，不能正确执行配置的完整 Base URL；本批使用其官方 Chat HTTP 契约替代这一调用，继续经过 TokenHub 原预授权/请求/attempt/结算。实际 localhost 测试只证明实现发送路径、鉴权、参数并正确解析用量，供应商会如何执行上限的依据仍为官方协议，不以 mock 当供应商证明。文档有 Token 边界小幅超报示例，因此上限控制不是对违约上游的无限保证；测量超过预授权保留全量事实并进入待对账。

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
- 真正待业务决定：上游违约超额的欠费、补扣、追收及承担方。已复用 `pending_reconciliation` 与原请求/预授权/用量/outbox，不新增债务、不自动负钱包或抹除差额。docs/03 原“预授权过期释放”不能应用于已提交且结果未知的请求，本批保留待核对占用；确定失败才释放。
- 后续兼容增强：未验证 adapter 的硬预算、完整 Responses/Messages Agent 生命周期、真实图像编辑。它们是明确技术兼容边界，不伪称已支持，也不混作待业务确认。
