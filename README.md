# TokenHub

多模型 API 中转与分销平台。产品与架构以 `docs/` 为准；开发进度见 [docs/09-开发进度.md](docs/09-开发进度.md)。

## 当前里程碑

**M0 基础工程与安全底座**：Go 控制面、PostgreSQL/Redis、Outbox Worker、结构化日志、OpenTelemetry、基础 RBAC、审计、Next.js 状态页。

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
make api
# 另一个终端
make worker
make web
```

`make web` 在 http://localhost:3000。浏览器请求 `/api/*` 由 Next.js rewrite 转到本机 API（`TOKENHUB_API_INTERNAL_URL`，默认 `http://127.0.0.1:8080`）。

## 验收

```bash
make test
make e2e-m0
```

## 安全

密钥只从环境变量或本地 `.env` 读取。`.env` 已加入 `.gitignore`。日志会脱敏 `password`、`secret`、`token`、`authorization`、`api_key` 等字段。
