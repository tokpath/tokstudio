# TokenHub 与 Bifrost 集成边界与架构建议

## 1. 调研结论

根据 Bifrost 官方仓库和文档，Bifrost 是高性能 AI Gateway，提供统一的 OpenAI 兼容接口、多 Provider 接入、自动 fallback、负载均衡、流式、多模态、日志和 Prometheus/Tracing 能力；其 OpenAI Provider 文档还覆盖 Chat Completions、Responses、图片、音频和 `/v1/videos` 等接口。

参考：

- [Bifrost GitHub](https://github.com/maximhq/bifrost)
- [Bifrost Provider 配置](https://docs.getbifrost.ai/quickstart/gateway/provider-configuration)
- [Bifrost 重试与 fallback](https://docs.getbifrost.ai/features/retries-and-fallbacks)
- [Bifrost 观测能力](https://docs.getbifrost.ai/features/observability/default)
- [Bifrost OpenAI Provider](https://docs.getbifrost.ai/providers/supported-providers/openai)

Bifrost 能减少 TokenHub 在协议转换、Provider 连接、重试、fallback、流式透传和基础观测上的重复开发，但它不是 TokenHub 的业务账务系统。

## 2. 责任边界

### TokenHub 自己负责

- 用户注册、登录、普通用户和管理角色；
- A/B/C/OEM 渠道归属、推广归因、代理层级和佣金；
- 钱包、预授权、额度/套餐/订阅、充值、退款和对账；
- 客户销售价、渠道批发价、Provider 成本价和价格版本；
- 模型公开目录、渠道模型权限和 API Key；
- 媒体素材保留、签名 URL、7 天清理和内容安全策略；
- OEM 域名、Logo、配色、站点和后台品牌配置；
- 面向客户和代理商的运营报表。

### Bifrost 优先复用

- OpenAI Chat Completions、Responses 以及兼容协议转换；
- Anthropic、Google 等 Provider 适配器；
- Provider Key 池、连接池、超时、重试、熔断和基础 fallback；
- SSE 流式透传和多模态请求转换；
- Provider 级请求日志、延迟、错误和 Prometheus/Tracing 指标；
- 在 Bifrost 能力范围内的图像、音频和视频接口。

## 3. 推荐集成形态

P0 采用“TokenHub 控制面 + Bifrost 数据面”的边界：

```text
客户端
  -> TokenHub API/Fassade
     认证、渠道、模型权限、预授权、限流
  -> Bifrost 内部 Gateway
     协议适配、Provider 路由、重试、流式/多模态透传
  -> 上游 Provider
     OpenAI / Anthropic / Gemini / 火山方舟 / OpenRouter
```

TokenHub 在调用 Bifrost 前完成余额和权益预授权；Bifrost 返回或回调 usage 后，TokenHub 完成客户结算、成本入账和佣金流水。每次调用必须透传 `request_id`、`attempt_id`、用户/渠道/模型等不可变 metadata，确保 Bifrost 日志可以回关联账务。

P0 采用嵌入 Bifrost Go SDK：TokenHub API 进程内 `bifrost.Init`，通过 `GatewayAdapter` 调用 `ChatCompletionRequest`，不再起独立 sidecar。默认 `TOKENHUB_BIFROST_SANDBOX=true`，用 LLM plugin 短路回声，本地和 CI 不依赖真实 Provider Key；设为 `false` 并配置 `TOKENHUB_OPENAI_API_KEY` 等后才会打上游。账务不能依赖 Bifrost 的日志存储作为唯一事实源。

异步任务基础设施采用与 Dapr 兼容的事件边界：P0 默认使用 PostgreSQL Outbox + Worker，事件使用 CloudEvents envelope；如果部署环境已运行 Dapr，可将投递器替换为 Dapr Pub/Sub，未来拆分媒体、支付、对账和佣金服务时复用 Service Invocation、State 和 Secrets 抽象。P0 不把 Dapr Workflow 作为强依赖，避免在业务状态机尚未稳定时引入额外编排复杂度。

部署演进目标是“单节点容器 -> Kubernetes + Dapr”：P0 使用 Docker Compose 或等价单节点部署，规模达到明确阈值后再迁移。迁移只替换运行时和服务发现，不改变客户 API、领域事件、账务流水和幂等契约。

为支持未来微服务化，P0 模块化单体必须遵循服务边界：每个模块拥有自己的数据和 migration，禁止跨模块直连表；同步调用使用明确的 service interface，异步流程使用带版本的 CloudEvents。跨模块业务不使用分布式事务，而使用 Outbox、幂等消费者和补偿事件。优先拆分媒体 Worker、支付 webhook、usage 对账/佣金 Worker 和观测服务，账务核心最后拆分。

前端与 TokenHub API 通过 OpenAPI 生成的 TypeScript Client 通信。Next.js 负责多门户渲染，Tailwind CSS + shadcn/ui/Radix UI 负责共享组件和 OEM 主题；Zustand 不承载服务端事实数据，余额/账单/指标等统一由 TanStack Query 管理。Web 认证使用 HttpOnly/Secure Cookie，前端权限判断只用于界面显示，后端 RBAC/scope 才是安全边界。TokenHub 后端使用 Go + Gin，zerolog 记录结构化日志，Viper 管理配置，PostgreSQL 通过 GORM 访问；账务核心通过显式事务、锁和版本化 migration 保证一致性。

## 4. 必须补齐的集成能力

1. Usage 可靠回传：从 Bifrost 响应、流式最终 chunk 或日志事件提取 Token/媒体 usage；缺失时进入 `pending_reconciliation`。
2. Attempt 级信息：必须获得每次 Provider 尝试的 Provider、上游模型、错误码、延迟和最终状态，不能只拿最终响应。
3. 禁止双重 fallback：TokenHub 维护业务级路由和渠道策略，Bifrost 只执行授予它的候选 Provider；两层 fallback 必须有唯一 attempt 链。
4. 流式中断：客户端断开后通知 Bifrost 停止上游读取，同时等待或补采最终 usage。
5. 媒体幂等：Seedance 等异步任务拿到上游任务 ID 后只允许查询/回调，不允许重复创建；若 Bifrost 不提供对应媒体生命周期能力，由 TokenHub Media Worker 直接调用 Provider Adapter。
6. 隐私控制：默认关闭 prompt/completion 持久化；对 Bifrost 日志配置脱敏、采样和保留期限，媒体 URL 使用 allowlist。
7. 指标桥接：将 Bifrost 指标映射到 TokenHub 的 Provider、公开模型、渠道、代理商和用户维度；账务收入/成本/毛利由 TokenHub 计算。

## 5. Bifrost 能力不足时的降级原则

- Bifrost 不支持的 Provider 协议由 TokenHub 自己实现最小 Adapter，不阻塞整体平台；
- Bifrost 不支持 Seedance 某个异步参数时，由 TokenHub Media Adapter 直连火山方舟或 OpenRouter；
- Bifrost 的企业预算、虚拟 Key、团队层级预算不作为 TokenHub 的账务事实源；TokenHub 的钱包、套餐、渠道额度优先级更高；
- Bifrost 升级必须经过兼容性测试，Provider 路由、usage 字段和流式协议变更需要灰度发布。

## 6. 验收标准

- 任一文本请求可在 TokenHub 账单中关联到 Bifrost 的 request/attempt、Provider、模型和 usage；
- fallback 发生时客户只扣一次，平台能看到每次上游尝试成本；
- Bifrost 不可用时，TokenHub 能明确返回 `provider_unavailable`，而不是产生无来源扣费；
- Prompt 默认不落盘，usage 和运维指标仍完整；
- Seedance 异步任务重试不会重复提交上游任务；
- 替换 Bifrost 或直连某一 Provider 不改变客户 API 契约和账务结果。
