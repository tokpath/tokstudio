# Agent 与代码接入实施记录

基线：`627554f`，分支：`codex/agent-setup-guides`。2026-10-10 在已 fetch 的远程基线之上实施；保留原 `codex/referral-experience` 分支。本批不等待运营主体资料，不变更品牌财务、Key 预算算法或网关协议生命周期。

## 实际入口与步骤

- 公共说明标题统一为“使用说明”，页内收紧标题、模型与配置间距；切换标签不留下过期标题。公共 `/docs`、`/docs/integrations`、模型详情中的使用说明及登录后的 `/app/docs` 复用同一组件。游客可以阅读并复制说明；创建 Key 才进入登录。
- 按当前品牌查找模型，指定模型使用精确 ID 查询，避免模型落在目录第 100 条之后时找不到。说明接口按请求主机解析真实品牌 API 域名；本地品牌保留 HTTP 和监听端口。缺失域名、模型、协议或读取失败不生成替代地址。
- Cline：设置中选择 **OpenAI Compatible** → 填当前品牌 `/v1` Base URL、本人 Key 与完整 Model ID → 按已公布的上下文、输出和图片能力设置。地址与 ID 单独复制，Key 默认只显示掩码。
- Aider：按官方安装步骤安装 → 在项目终端设置本人 Key，复制生成的环境变量与启动命令 → 用 `/ask` 发一条短消息，查看请求记录后再 `/add` 文件。`openai/` 是客户端协议前缀；如本站模型 ID 已是 `openai/xxx`，命令仍应为 `openai/openai/xxx`，不能删掉模型自己的厂商前缀。当前命令模板面向 macOS/Linux shell。
- 代码接入：选模型实际提供的协议 → 选 curl、Python/Node OpenAI SDK 或无依赖 HTTP 示例 → 设置 `TOKENHUB_API_KEY` → 按示例的安装、保存和运行命令执行。Base URL 的 `/v1` 只出现一次；SDK 不使用默认 OpenAI 地址。
- 协议、工具、语言、模型、Key 及合法返回目标保留在链接上下文中。重新加载可恢复；进入钱包后可以回到原公开说明页，邀请码也保留。示例和配置不嵌入完整 Key；复制本人 Key 仍经现有审计接口。

## 工具范围与协议边界

| 用途 | 本批提供 | 限制 |
| --- | --- | --- |
| Cline GUI | OpenAI Compatible 配置步骤、当前品牌字段和官方入口 | 以 Chat Completions 为入口；工具自身的模型能力及完整实际 Agent 会话未联调 |
| Aider CLI | OpenAI Compatible shell 配置及完整模型名称 | 以 Chat Completions 为入口；未知模型的上下文/编辑格式按官方提示核对，实际代码编辑会话未联调 |
| Python / Node OpenAI SDK | Chat Completions、基础 Responses 调用 | 仅模型返回该 endpoint 时展示；无自动重试；Responses 示例不含 `store`、流式、工具、续接字段 |
| curl / 标准库 HTTP / native fetch | 已提供 endpoint 的基础请求 | Messages 仅字符串消息；媒体沿用原任务接口示例，不新增协议能力 |
| Codex 原生 | 显式缺口与官方参考链接 | 官方自定义网关使用 Responses；本站缺完整事件流、函数调用回传及响应生命周期，不给可用配置 |
| Claude Code 原生 | 显式缺口与官方参考链接 | 官方网关还要求 Anthropic 内容块、工具结果、流式事件及能力头传递；本站仅有基础 Messages 转换，不给可用配置 |

工具兼容性依据实际协议，不按模型厂商名称推断。站内可选短文本测试有真实调用费用，仍需显式点击；本批未点击真实上游测试。

## 预算与排错

同步最新业务：开始前按当前品牌价格及计费维度预估是否准入，结束后按真实用量扣费。一次实际消费可超过 Key 上限或使余额为负；超限后阻止新请求。充值补账户余额，不重置 Key 累计消费。缺失用量仍待核对，不按估算结算。

说明移除 `key_budget_unbounded` 和绝对硬上限承诺，使用与 Key 实施代理约定的 `price_estimate_unavailable`。Key 错误、模型范围、Key 上限、账户余额、估算不可用、429、未知结果及重复 `/v1` 各有具体处理。超时先查原请求；SDK 显式关闭自动重试。curl 使用 `--fail-with-body`，标准库 Python 在 HTTP 失败时输出原错误体，保留错误 code 与请求编号。

## 官方依据

以下资料于 2026-10-10 实际搜索并打开；Python SDK 参考页直接打开多次超时，官方搜索返回其完整 SDK 配置与重试章节，并通过正式安装包实际执行再次核验接口。

- [Cline OpenAI Compatible](https://docs.cline.bot/provider-config/openai-compatible)：设置字段及模型能力设置。
- [Aider OpenAI compatible APIs](https://aider.chat/docs/llms/openai-compat.html)：环境变量、安装与 `openai/` 模型前缀。
- [OpenAI SDK 快速开始](https://developers.openai.com/api/docs/quickstart)、[Chat Completions 请求](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create)、[Python SDK](https://developers.openai.com/api/reference/python)、[TypeScript SDK](https://developers.openai.com/api/reference/typescript)：SDK 方法、Base URL、环境变量、超时和关闭重试。
- [Codex 网关兼容说明](https://learn.chatgpt.com/docs/enterprise/gateway-compatibility)、[配置参考](https://learn.chatgpt.com/docs/config-file/config-reference)：Responses 网关及事件、工具回合要求。
- [Claude Code LLM gateway](https://code.claude.com/docs/en/llm-gateway) 及其 Compatibility guide：Messages 格式、能力字段及头传递要求。

## 本地验证与复现

真实 SDK 测试依赖只装到临时目录，未改项目依赖或主机全局包。本批使用官方 **Python openai 3.28.0**、**Node openai 7.32.0**。Go `httptest` 本地服务、虚拟 Key，两个 SDK × Chat/Responses × 成功/503/连接断开，共 **12 个实际执行场景**。核对路径、Bearer、完整带引号模型 ID、32 个输出 token、请求编号和失败不自动重发。另执行原 curl/标准库/native fetch 示例与 Aider shell 参数转义；Aider stub 仅验证命令编码，不视为实际 Agent 联调。

准备依赖（宿主 bundled Python 为 3.12；Go 镜像 Python 为 Linux ARM64 3.11）：

```sh
mkdir -p /private/tmp/tokpath-guide-sdk
docker run --rm -v /private/tmp/tokpath-guide-sdk:/sdk -w /sdk \
  tokstudio-ci-runner:local npm install --ignore-scripts --no-audit --no-fund openai@7.32.0
/Users/hengzi/.cache/codex-runtimes/codex-primary-runtime/dependencies/python/bin/python3 \
  -m pip install --platform manylinux2014_aarch64 --python-version 3.11 \
  --implementation cp --abi cp311 --only-binary=:all: \
  --target /private/tmp/tokpath-guide-sdk/python openai==3.28.0
```

执行本地示例测试；`TOKENHUB_DOCS_SDK_DIR` 不设置时仅 SDK 实际执行测试跳过，其余普通示例与契约测试仍执行：

```sh
docker run --rm --network tokpath-ux-20261010 \
  -v /Users/hengzi/code/tokpath/tokstudio-referral/backend:/src:ro \
  -v /private/tmp/tokpath-guide-sdk:/sdk:ro \
  -v tokpath-ux-gocache-20261010:/root/.cache/go-build -w /src \
  -e TOKENHUB_ENV=test -e GIN_MODE=release -e GOPROXY=off \
  -e DATABASE_URL='postgres://tokenhub:tokenhub@postgres:5432/tokpath_referral?sslmode=disable' \
  -e REDIS_URL=redis://redis:6379/4 -e TOKENHUB_DOCS_SDK_DIR=/sdk \
  tokpath-ux-go-test:20261010 go test ./internal/app -run '^TestDocs' -count=1
```

Web 验证使用现有 Linux ARM64 `node_modules`，无网络，不重新安装项目依赖：

```sh
docker run --rm --network none \
  -v /Users/hengzi/code/tokpath/tokstudio-referral/web:/app \
  -v /Users/hengzi/code/tokpath/tokstudio/web/node_modules:/app/node_modules -w /app \
  tokstudio-ci-runner:local npm test -- \
  lib/model-usage.test.tsx lib/model-instructions-context.test.ts lib/public-model-instructions.test.tsx
```

上述 **28/28** 组件与入口用例通过；Go `TestDocs*` 通过，包含 12 个真实 SDK 子用例和 3 个 HTTP 错误体实际执行子用例。最终生产构建 **PASS**，构建后 `tsc --noEmit` **PASS**。定向 Playwright **1/1 PASS（2.2s）**：真实浏览器复制品牌配置、Cline→Aider 选择、刷新后工具/协议/SDK语言恢复、钱包返回目标与邀请码保留、原生工具缺口可见，且没有 POST 模型请求。截图：`web/test-results/agent-setup-guides.png`（测试产物不提交）。整合集成全组由 root 负责，未重复执行全套 E2E。

生产构建、类型与浏览器采用同一个无网络 runner，分别执行 `npm run build`、`node node_modules/typescript/bin/tsc --noEmit`、`npm run e2e:web -- agent-setup-guides.spec.ts --workers=1`。设置 `NEXT_TELEMETRY_DISABLED=1`；其余沿用仓库 Playwright 配置，浏览器与 Next 服务在同一容器，测试内 `127.0.0.1:8080` HTTP fixture 提供品牌模型/说明契约，不需真实 API 服务。

初次浏览器验收定位包含复制按钮的字段时使用了不匹配的精确文本，后改按真实字段定位；协议选择器也补明确的可访问名称。读取链接属性遗漏 `await` 的测试缺陷已修。没有通过延长超时、删除上下文断言或触发真实调用规避问题。

未验证：真实付费上游、外部 Cline/Aider 实际 Agent 会话、Codex/Claude Code 原生客户端联调。运营主体资料仍由主线处理，不阻塞本批说明交付。
