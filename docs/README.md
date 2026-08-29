# TokenHub 需求与设计文档索引

## 当前有效文档

1. [调研与产品结论](00-TokenHub调研与产品结论.md)：外部产品调研、竞品能力和设计结论。
2. [统一产品需求文档](01-TokenHub统一产品需求文档.md)：全平台最终 PRD，定义 P0/P1 范围和业务规则。
3. [分销与代理商需求](02-分销与代理商需求.md)：A/B/C、代理商、KOL、归因和佣金细则。
4. [账务与技术模型](03-账务与技术模型.md)：价格、钱包、预授权、usage 和账务原则。
5. [逐项讨论清单](04-TokenHub逐项讨论清单.md)：D1-D32 的拍板记录，作为决策日志。
6. [P0 数据模型与状态机](05-TokenHub-P0数据模型与状态机.md)：数据库实体、账务不变量和状态机。
7. [P0 OpenAPI 契约](06-TokenHub-P0-OpenAPI契约.md)：对外 API、错误码、幂等和管理 API 骨架。
8. [Bifrost 集成边界](07-TokenHub-Bifrost集成边界与架构建议.md)：TokenHub 控制面、Bifrost 数据面和 Dapr 兼容事件边界。
9. [P0 开发拆分与验收](08-TokenHub-P0开发拆分与验收顺序.md)：M0-M7 纵向切片、依赖和验收标准。
10. [开发进度](09-开发进度.md)：每个里程碑的实现清单、端到端验证记录和 PR。

## 决策状态

D1-D32 已确认。当前已锁定：

- P0：OpenAI Chat/Responses、Anthropic Messages、Google Gemini、Seedance 视频/图像媒体；
- P0：Stripe、支付宝、微信支付、人工入账/兑换码；
- P0：A/B/C/OEM、两级 KOL、套餐/订阅/赠送额度/自动续费；
- 架构：Go + Gin + PostgreSQL/GORM + Redis + Outbox Worker + S3 + React/Next.js；zerolog、Viper、Prometheus、Grafana、OpenTelemetry；
- Bifrost：独立内部服务/Sidecar，TokenHub 保留业务控制面和账务事实源；
- 演进：单节点容器 -> Kubernetes + Dapr；
- 前端：Next.js + TypeScript + Tailwind CSS + shadcn/ui/Radix UI + Zustand/TanStack Query + OpenAPI 生成客户端；
- 可靠性：RPO ≤ 15 分钟、RTO ≤ 1 小时、每日备份 + PITR、季度恢复演练。

## 文档清理说明

旧版分销 Markdown/Word 文档中的有效内容已归并到 `01`、`02`、`03` 和 `04`；旧版文件不再作为项目文档维护。
