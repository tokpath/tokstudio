# TokenHub

TokenHub 为平台、OEM 和渠道提供独立经营系统，为终端客户提供一个品牌入口、多模型调用和通用 ATG 额度。业务以[产品说明书](docs/product-spec/TokenHub产品说明书.md)、[业务案例与验收标准](docs/product-spec/TokenHub关键业务案例与验收标准.md)为准，实施以[重构方案](docs/product-spec/TokenHub重构实施方案.md)为准。

本轮直接在 `release/v0.1.0` 重构，范围包含全部已确认功能。旧业务模块与旧数据没有兼容路径。业务与前端重构、多角色仿真及本地集成验证已完成；用户于 2026-10-18 授权提交、推送并部署至现有测试环境。实现范围、实际证据、浏览器覆盖边界及真实外部账户的启用条件见[实施记录](docs/product-spec/实施记录.md)。

## 登录和经营开通

登录只填写邮箱和密码，系统自动识别客户或经营人员；只有凭据同时匹配两种独立账号时，才选择本次进入的空间。人员在各经营方的权限仍分别校验。

建立 OEM / 渠道后，可以直接开通负责人账号并手动交付邮箱、初始密码和登录入口，不依赖邮件服务。初始密码至少 10 字节、最多 72 字节，首次登录必须修改。若该邮箱已有经营人员账号，系统生成 72 小时邀请链接，由本人使用原密码接受，不能覆盖原密码。配置邮件服务后也可发送邀请及找回密码邮件。已有演示账号仍使用种子约定的测试密码。

品牌 API 地址由官方配置或已验证并启用的 OEM 域名自动生成，以 `/v1` 结尾；渠道继承品牌入口。OEM 未配置有效域名时明确显示待接入，不显示占位地址。合作和供货报价填写实际名称、金额、额度及主动退出的余额处理方式，可另行上传私有合同；内部编号和版本由系统维护。合同仅供上传经营方及有关采购方按权限读取，并纳入加密对象备份。

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

邮件入口位于各经营方的「设置 → 邮件服务」。平台维护默认 SMTP，OEM 可选自己的 SMTP，渠道仅查看所属品牌的有效服务状态。SMTP 主机、端口和授权信息由邮件服务商提供；域名的 DNS 指向及 HTTPS 由服务器或云服务配置，系统负责检查与启用；Google 登录需在 Google Cloud 建立 OAuth 客户端并登记品牌回调地址，属于可选登录方式。未接入邮件时可用既有测试账号验收其他业务，员工账号也可手动交付。注册与找回密码可先在隔离环境使用 SMTP 接收服务联调；真实投递、域名和 Google 登录仍需资料齐全后验证。

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

推送 `release/v0.1.0` 后，完整 `ci` 成功会自动触发 `deploy-token`，将通过测试的精确提交部署到 `https://test.tokpath.com`，无需人工触发。PR、其他分支、失败或取消的 CI 不发布；已被新提交取代的自动任务跳过。部署串行执行，不中断正在进行的发布。保留人工 `workflow_dispatch` 作为恢复入口：选择 release 分支、填写已经通过 CI 的完整 SHA 和 `DEPLOY_TEST`。两种入口都只使用 `tokenhub-test` 环境和 `/root/workspace/tokstudio` 测试栈，保留已有账号、配置与业务数据。

自动触发依赖默认分支上的 `deploy.yml`，当前默认分支为 `release/v0.1.0`；调整默认分支时须同步保留该工作流。CI 根据提交差异运行相关测试：纯前端改动不跑后端测试，纯后端改动不重建前端；共享翻译、接口契约、工作流及无法识别的公共文件会触发相关或全部检查。缺少可比较的历史时运行全部测试，部署约束、契约和翻译同步检查始终运行。

后端分为 14 组：门户六组、退款纠错两组、通用包两组，组织、激励、交易、调用各一组。每组使用独立 PostgreSQL、Redis 和 MinIO，保留 `-race -count=1`；依赖并行编译，实际测试包串行执行以隔离故障状态。测试由 Go 自动枚举，新测试和新包自动纳入。`backend` 汇总检查仅在确认后端未变更时接受跳过，否则要求所有分组成功。Go 依赖和编译缓存统一管理，首次建立缓存仍可能较慢。前端继续执行单元测试、生产构建与浏览器测试，复用 npm 和 Next.js 缓存。新推送取消同分支未完成的 CI，不取消正在部署的任务。

部署只构建一次 API、网页和备份镜像；worker、migrate 共用 API 镜像，迁移完成后以 `--no-build` 启动已构建镜像。构建或迁移失败会停止发布。仅磁盘空间不足时回收悬空镜像和部分构建缓存，避免每次部署重新准备基础镜像。

部署前准备新库、加密密钥、初始负责人、邮件、对象存储和备份密钥；运行 `make assert-deploy` 检查隔离约定。资金参数、上游供应商和商户配置通过各自经营权限管理，不能通过运维 SQL 直接改钱。凭据不进入 Git、日志、导出或通知。


## 测试站验收数据

全系统清算币种与 ATG 基础比例须由平台负责人先在“设置 → 营业状态”预约并正式生效；生效前不开始资金交易、注册奖励或模型调用，生效后永久锁定，暂停营业不解锁。生效前应核对已录入的套餐、报价和费率。系统不维护汇率，原币收支与成本按币种分别汇总，财务人员据原始明细核算。

`seed-demo` 是显式管理命令，不在 API、迁移或正常启动时运行。只允许 `test.tokpath.com`，本地非生产联调可使用 `localhost`；必须传入匹配的 `--host` 与 `--confirm TEST_DATA_ONLY`。先部署并验证项目外的独立测试模型，再通过环境变量 `TOKENHUB_DEMO_MODEL_KEY` 提供其凭据（不要放进命令参数或提交 Git）。

```sh
/seed-demo --host test.tokpath.com --confirm TEST_DATA_ONLY --model-base-url https://test.tokpath.com/__tokenhub_demo__
```

一次事务建立具名测试 OEM、直属渠道、OEM 渠道、采购余额与报价、测试套餐、人工收款配置、注册归属、购买订单、USD 余额、ATG 和推广活动。所有资金依据均标注 `TEST DATA ONLY`，无外部真实付款。提交完成后重复运行只返回原结果，不重复发放、不重置用户后续验收操作。若准备材料或权限核实失败，整个事务回滚。现有负责人账号和密码不被改写，OEM 域名也不会被伪造为已验证。

新增客户账号为 `new@tokenhub.local`（新客户）、`customer@tokenhub.local`（已购客户，直属 KOL 邀请）、`kol@tokenhub.local`（已有资格的测试 KOL）、`channel-customer@tokenhub.local`（官方渠道客户）。新增经营账号为 `operations@tokenhub.local`、`finance@tokenhub.local`、`support@tokenhub.local`、`readonly@tokenhub.local`、`tech@tokenhub.local`、`oem@tokenhub.local`、`channel@tokenhub.local`、`oem-channel@tokenhub.local`、`channel-finance@tokenhub.local`。这些新增账号密码统一为 `password1`，仅供测试环境。

公开模型 `demo-chat` 显式标注固定合成回复，独立上游需分别验证同步和流式能力。真实模型、支付商户及邮件资料仍由经营方配置。测试种子不放宽正常注册密码要求，也不开放无验证邮件能力的公众注册。
