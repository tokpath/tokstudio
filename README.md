# TokenHub

多模型 API 中转与分销平台。产品与架构以 `docs/` 为准；开发进度见 [docs/09-开发进度.md](docs/09-开发进度.md)。

## 当前里程碑

**M0 基础工程与安全底座**：Go 控制面、PostgreSQL/Redis、Outbox Worker、结构化日志、OpenTelemetry、基础 RBAC、审计、Next.js 状态页。

## 本地启动

```bash
cp .env.example .env
docker compose up --build
```

默认只对外映射 Caddy（`edge`）的 **80 / 443**。浏览器打开同一个入口即可：

- Web：http://localhost
- API 探活：http://localhost/healthz
- 就绪：http://localhost/readyz
- 浏览器接口（同源）：http://localhost/api/v1/...
- SDK / API Key：http://api.localhost/v1/...（`api.oem.localhost` 同理）

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
