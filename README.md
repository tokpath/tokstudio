# TokenHub

TokenHub 为平台、OEM 和渠道提供独立经营系统，为终端客户提供一个品牌入口、多模型调用和通用 ATG 额度。业务以[产品说明书](docs/product-spec/TokenHub产品说明书.md)、[业务案例与验收标准](docs/product-spec/TokenHub关键业务案例与验收标准.md)为准，实施以[重构方案](docs/product-spec/TokenHub重构实施方案.md)为准。

本轮直接在 `release/v0.1.0` 重构，范围包含全部已确认功能。旧业务模块与旧数据没有兼容路径。本轮代码重构和本地集成验证已完成，用户已授权提交推送；发布仍须单独授权。实现范围、实际证据、浏览器覆盖边界及真实外部账户的启用条件见[实施记录](docs/product-spec/实施记录.md)。

## 运行结构

Next.js / React 前端，Go / Gin / GORM 模块化后端，PostgreSQL 16、Redis 7、私有 MinIO/S3、Caddy，以及既有监控和备份组件。Bifrost Core 嵌入 Go API 与 worker，不另起模型网关。API 处理请求，worker 处理持久任务、对账、通知、续费、消费确认和清理；资金和权益始终以 PostgreSQL 为事实源。

品牌身份、模型授权和经营方权限分开。API 启动不会创建演示客户、模型、资金或支付商户，也不会自动执行数据库迁移。`migrate` 是独立的初始化/部署命令；它拒绝旧数据库和修改过的已执行迁移。

## 初始化和本地运行

现有 `.env` 可能指向仓库外的本地配置。保留已有文件和链接；仅新环境复制 `.env.example`。至少填写 `TOKENHUB_APP_KEY`（32 字符以上随机值）、首次负责人邮箱、密码以及品牌、经营方与法律主体名称。API、worker、migrate 必须使用同一加密密钥。普通平台管理员使用邮箱密码登录，不再有固定 bootstrap API token。

```bash
# 仅没有 .env 的新环境执行
cp .env.example .env
# 填写上述配置后运行
docker compose up --build -d
```

Compose 使用独立 `tokenhub_v2_*` 数据卷，先等待 PostgreSQL 和私有对象桶就绪，再运行一次 `migrate`，成功后启动 API 与 worker。原有数据库和卷不会被重置。初始化人员不代表业务已开放：仍需配置模型、价格、经营规则、邮件及收款方式，并满足对应业务的就绪检查。

- 网站：`http://localhost:9080`。
- 同品牌模型 API Base URL：`http://localhost:9080/v1`。
- 浏览器业务接口：同源 `/api/v1/…`。
- 探活 `/healthz`；就绪 `/readyz` 检查实际依赖。
- 内部服务与指标不直接暴露公网。需要本机调试端口时，叠加 `docker-compose.debug.yml`，端口只绑定 `127.0.0.1`。

本机运行 Go 时，先准备隔离的新 PostgreSQL、Redis 和私有对象桶，再依次运行 `make migrate`、`make api`、`make worker`；前端运行 `make web`。配置读取器读取 `.env`，不要用 shell 展开命令导出含密码的文件。模型供应商、商户、OEM 和渠道必须通过实际配置流程建立，没有目录种子自动上架。

## 外部服务与联调

支付资料尚未准备齐全。本轮实现真实驱动、配置校验、签名验证、外部查询、异常恢复和账务交付；实际商户、供应商、稳定币网络及公网域名仍需在取得资料后验证。未配置的能力不可伪装成可用。

独立协议联调服务放在仓库外的 `../integration-lab`，支持模型 HTTP/SSE/异步任务、支付签名回调、重复事件和结果不明等情形。测试还使用独立 PostgreSQL、Redis 和 MinIO；这些服务不是生产的默认上游。用 mock 通过协议验收，不代表已经完成真实商户开通或公网域名验收。

普通 API 请求内容不长期保存。可回读的临时结果先加密再存入私有 S3 前缀，PostgreSQL 仅保存引用与期限：同步回读窗口 5 分钟、异步结果默认 24 小时。重试不延长期限，到期拒绝读取，后台删除失败保留引用重试；桶生命周期处理崩溃遗留对象。Redis 不保存模型响应正文。账务、必要技术记录与内容期限分别处理。

## 验证

```bash
make test
# 对真实隔离数据库运行集成测试，必须同时设置两个测试库变量
TOKENHUB_DATABASE_URL='…' TOKENHUB_TEST_DATABASE_URL='…' make test-integration
cd web && npm run build && npx playwright test
```

外部用例使用显式的 `TOKENHUB_MODEL_MOCK_URL`、支付测试通道变量、`TOKENHUB_TEST_S3_*` 和 `TOKENHUB_TEST_REDIS_URL`。未设置时对应外部用例会跳过；跳过不计作联调完成。测试必须连接隔离环境，不对生产账户运行故障注入。

完整协议联调使用 `make test-external`，它要求同时配置两项测试数据库变量、`TOKENHUB_MODEL_MOCK_URL`、`TOKENHUB_PAYMENT_MOCK_URL`、`TOKENHUB_SMTP_MOCK_ADDR`、`TOKENHUB_TEST_REDIS_URL` 以及 S3 的 ENDPOINT、REGION、BUCKET、ACCESS_KEY、SECRET_KEY。`TOKENHUB_TEST_S3_REGION` 由测试夹具单独读取，MinIO 可设置为 `us-east-1`。容器需要回调主机上的测试监听器时，另设 `TOKENHUB_MOCK_CALLBACK_HOST=host.docker.internal`。各包按顺序执行，避免不同测试同时改动同一个协议服务的故障状态。协议服务自身部署和合成凭据见仓库外 integration-lab 的 README；不要将这些配置复制到正式业务环境。

持续负载为显式运行的 `TestExternalConcurrentCallingAndSalesSoak`，另设 `TOKENHUB_LOAD_DURATION=30m`；默认 100 个调用协程、50 个购买协程，可用 `TOKENHUB_LOAD_CALLERS` 和 `TOKENHUB_LOAD_BUYERS` 调整，并用 `TOKENHUB_LOAD_REPORT` 指定原始报告路径。它使用同一热点客户，并在结束后核对全部账本和上游尝试，时延包含排队。已运行结果见[账本、调用与运行证据](docs/product-spec/账本调用与运行验收证据.md)。

## 备份和隔离恢复

设置独立随机的 `TOKENHUB_BACKUP_KEY`（至少 32 字符，另存至密钥保管系统）后，运行 `docker compose --profile backup up --build -d backup`。备份容器使用真实 `pg_dump`，归档数据库以及私有桶的 `branding/`、`invoices/`、`finance-evidence/` 文件清单、长度和 SHA-256，再进行认证加密；不包含模型临时结果或可重建的导出。默认每天备份一次，成功后清理超过 30 天的本地备份。`tokenhub_v2_offsite` 只是本机卷的名字，并不意味着异地容灾；须另行复制加密文件至独立存储并监控备份失败。

原应用加密密钥和备份密钥都必须保留，分别用于恢复业务密文和解开备份。恢复只接受显式配置、名称以 `tokenhub_restore_drill_` 开头的空数据库及独立空对象桶。预先创建它们，在演练进程中设置 `TOKENHUB_RESTORE_DATABASE_URL` 和目的桶的 `TOKENHUB_S3_*`，保持 `TOKENHUB_DATABASE_URL` 指向源库，再运行 `bash scripts/restore_drill.sh /绝对路径/备份.thbk`。本机执行需要 PostgreSQL 16 客户端；也可使用 Dockerfile 的 `backup` 构建阶段运行 `tokenhub-backup --mode=restore-drill --input=…`。

恢复命令逐一回读文件校验摘要，并检查借贷平衡、账户余额与流水、占用与明细一致。既有数据库、对象桶和输出文件不会被覆盖。演练失败留下隔离现场供排查，不自动清库。恢复成功也不会启动 worker：正式恢复前，必须核查备份后真实支付、退款和打款事实，处理待办与重复执行风险，再由运维切换。临时模型结果不恢复，其过期引用由清理任务处理。

## 部署边界

普通 push 只运行 CI，不发布。预览部署必须人工运行 `deploy-token`，选择 `release/v0.1.0` 中通过 CI 的完整提交 SHA，输入 `DEPLOY_TEST`，经 `tokenhub-test` Environment 门槛后部署该精确提交。部署仓库路径为 `/root/workspace/tokstudio`，目标预览主机名为 `test.tokpath.com`。本轮已获提交推送授权，不包含发布。

部署前准备新库、加密密钥、初始负责人、邮件、对象存储和备份密钥；运行 `make assert-deploy` 检查隔离约定。资金参数、上游供应商和商户配置通过各自经营权限管理，不能通过运维 SQL 直接改钱。凭据不进入 Git、日志、导出或通知。
