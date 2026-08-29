# TokenHub

多模型 API 中转与分销平台。产品与架构以 `docs/` 为准；开发进度见 [docs/09-开发进度.md](docs/09-开发进度.md)。

## 当前里程碑

**M0 基础工程与安全底座**：Go 控制面、PostgreSQL/Redis、Outbox Worker、结构化日志、OpenTelemetry、基础 RBAC、审计、Next.js 状态页。

## 本地启动

```bash
cp .env.example .env
docker compose up --build
```

- API：http://localhost:8080/healthz
- 就绪：http://localhost:8080/readyz
- 指标：http://localhost:8080/metrics
- Web：http://localhost:3000

没有 Docker 时，先启动 PostgreSQL/Redis，再执行：

```bash
export $(grep -v '^#' .env | xargs)
make migrate
make api
# 另一个终端
make worker
make web
```

## 验收

```bash
make test
make e2e-m0
```

## 安全

密钥只从环境变量或本地 `.env` 读取。`.env` 已加入 `.gitignore`。日志会脱敏 `password`、`secret`、`token`、`authorization`、`api_key` 等字段。
