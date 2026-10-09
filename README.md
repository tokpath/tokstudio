# TokenHub

多模型 API 中转与分销平台。产品与架构以 `docs/` 为准；本轮整改实施见 [docs/21-整改实施记录.md](docs/21-整改实施记录.md)，验收边界由最终测试证据记录；docs/09 是历史开发进度。

## 当前里程碑

平台负责技术底座与 OEM 交付，OEM 经营自己的品牌；渠道只负责推广归属与收益。普通用户直接创建具有模型范围、累计 USD 上限和有效期的 Key，邀请与收益在同一账户。

公开模型只展示真实协议与安全服务状态。Responses/Messages 为已实现子集；当前不承诺 Codex、Claude Code 或图像编辑支持。未配置真实上游、域名和商户前，本地测试不代表上线营业验收。

## 本地启动

```bash
cp .env.example .env
docker compose up --build
```

默认只对外映射 Caddy（`edge`）。80/443 经常被占用或需要 root，所以宿主机用 **9080 / 9443**（可用 `TOKENHUB_EDGE_HTTP_PORT`、`TOKENHUB_EDGE_HTTPS_PORT` 改）：

- Web：http://localhost:9080
- API 探活：http://localhost:9080/healthz
- 就绪：http://localhost:9080/readyz
- 浏览器接口（同源）：http://localhost:9080/api/v1/...
- SDK / API Key：http://api.localhost:9080/v1/...（`api.oem.localhost` 同理）
- HTTPS 演练：https://localhost:9443

Postgres、Redis、API `:8080`、Web `:3000`、Grafana、Prometheus、Bifrost、Pebble 都留在 compose 内网。本机 `make api` 需要这些端口时：

```bash
docker compose -f docker-compose.yml -f docker-compose.debug.yml up
```

没有 Docker 时，先启动 PostgreSQL/Redis，再执行：

```bash
export $(grep -v '^#' .env | xargs)
make migrate
make catalog-seed
make api
# 另一个终端
make worker
make web
```

`make catalog-seed` 把嵌入的 ofox 公开目录快照写入 PostgreSQL。预置模型直接是**已审核并已发布**（`status=published`、`sync_state=published`），只授权官方和分销渠道，不进 OEM 白名单。命令幂等，会先跑 migration。Docker 里可以用：

```bash
docker compose --profile seed run --rm catalog-seed
```

API 启动时的 `Seed()` 仍会调用同一导入，方便开发环境；生产或空库也可以只跑这条独立命令，不必先起 API。

`make web` 在 http://localhost:3000。浏览器请求 `/api/*` 由 Next.js rewrite 转到本机 API（`TOKENHUB_API_INTERNAL_URL`，默认 `http://127.0.0.1:8080`）。

## 验收

```bash
make test
make e2e-m0
```

## 预览部署（Atlas）

Atlas 拥有编排：`release/v0.1.0` 分支 CI 全绿后，GitHub Actions SSH 到宿主机跑幂等脚本，更新 https://test.tokpath.com。

| 栈 | 触发分支 | 宿主机检出 | compose | 公网 |
| --- | --- | --- | --- | --- |
| test | `release/v0.1.0` | `/root/workspace/tokstudio` | 目录默认名 + `docker-compose.token.yml` | https://test.tokpath.com |

仓库 **Secret**（Settings → Secrets and variables → Actions → Secrets，不是 Variables）：

- `ALIYUN_HOST`：SSH 主机。工作流写成 `${{ secrets.ALIYUN_HOST }}`，不要写死 IP。
- `TOKEN_DEPLOY_SSH_KEY`：SSH 私钥。

`deploy-token` 只维护 nova Caddy 的 `test.tokpath.com`。本地可用 `make assert-deploy` 做 dry-run / grep 门禁。

对象存储使用 S3 兼容接口，本地隔离测试使用 MinIO；真实生产存储仍须独立验证。

## 安全

密钥只从环境变量或本地 `.env` 读取。`.env` 已加入 `.gitignore`。日志会脱敏 `password`、`secret`、`token`、`authorization`、`api_key` 等字段。


请求与账务日志只记录必要元数据；标准 API 幂等重试例外会在 Redis 缓存完整响应正文 24 小时，媒体素材另按存储策略处理。
