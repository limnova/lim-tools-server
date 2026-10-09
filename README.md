# Lim Tools Server

Lim Tools 后端。Go + Gin 的 HTTP API 服务。

## 环境要求

- Go 1.26+

## 快速开始

```bash
make run          # 启动，默认监听 :8080
curl localhost:8080/healthz
curl localhost:8080/api/v1/info
```

## 常用命令

```bash
make help         # 列出所有目标
make run          # 本地启动
make build        # 编译到 bin/，版本号从 git describe 注入
make test         # 跑测试
make test-race    # 带竞态检测
make fmt          # gofmt -s -w .
make tidy         # go mod tidy
make lint         # golangci-lint（需自行安装）
```

## 配置

按 12-Factor，配置全部来自环境变量，不读配置文件。变量名到字段的映射用 struct tag 声明
（[caarlos0/env](https://github.com/caarlos0/env)），见 `internal/config/config.go`。

| 变量 | 默认值 | 说明 |
|---|---|---|
| `LIM_TOOLS_ADDR` | `:8080` | HTTP 监听地址 |
| `LIM_TOOLS_ENV` | `development` | 运行环境。设为 `production` 会切 gin release 模式、日志升到 Info 级，并关闭 `.env` 加载 |
| `LIM_TOOLS_SHUTDOWN_TIMEOUT` | `10s` | 优雅关闭时等待在途请求的上限；零或负值回落默认值，超时后关闭活跃连接 |
| `LIM_TOOLS_WORKBOOK_DIR` | `./data/workbooks` | 工作簿持久目录，一个服务进程独占；部署时挂载持久卷 |
| `LIM_TOOLS_DB_HOST` | `127.0.0.1` | PostgreSQL 主机 |
| `LIM_TOOLS_DB_PORT` | `5432` | PostgreSQL 端口 |
| `LIM_TOOLS_DB_NAME` | 无 | 数据库名 |
| `LIM_TOOLS_DB_USER` | 无 | 连接账号 |
| `LIM_TOOLS_DB_PASSWORD` | 无 | 连接密码，没有默认值 |
| `LIM_TOOLS_DB_SSLMODE` | `disable` | libpq 风格 SSL 模式 |

**校验**：变量缺失用默认值补上；值格式非法（如时长写成 `not-a-duration`、端口写成非数字）
会让 `Load()` 返回错误，进程启动即退出，不会带着可疑的值继续跑。

**`.env`**：本地开发可以 `cp .env.example .env`。该文件仅在**非 production** 环境加载，
且**不覆盖**进程中已存在的环境变量。生产部署请直接设置环境变量 —— 留一个 `.env` 会把部署
问题掩盖成本地文件问题。

数据库字段目前都不是必填：连接层还没落地，服务需要在没有任何数据库配置的情况下也能启动。
连接层接上后，`Name` / `User` / `Password` 应当补上 required。

## 目录结构

```
cmd/server/main.go        入口：读配置 → 装配依赖 → 启动
internal/
├── config/               从环境变量读配置
├── logging/              slog 构造与 context 传递
│   ├── logging.go        按环境选 JSON / 文本 handler，并设为进程默认
│   └── context.go        WithLogger / FromContext
├── middleware/           gin 中间件
│   ├── recovery.go       捕获 panic，用 slog 记录堆栈后返回 500
│   ├── requestid.go      X-Request-ID：上游带了就沿用，没带就生成
│   └── logger.go         注入请求级 logger + 访问日志
├── service/              业务逻辑（不依赖 HTTP 层）
├── handler/              HTTP 层：解析、调用 service、组装响应
│   ├── router.go         Handler 结构与路由注册，新增接口在这里加一行
│   └── health.go         /healthz 与 /api/v1/info
└── server/               gin 引擎组装 + 优雅关闭
```

依赖用手动构造函数注入，在 `cmd/server/main.go` 里一眼可见：

```
config → service → handler → server
```

分层是轻量的：handler 不写业务，service 不碰 HTTP。等确实复杂了再拆，不提前上 Clean Architecture。

## 接口

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/healthz` | 存活探针，不查依赖 |
| GET | `/api/v1/info` | 服务自述：名称、环境、版本、启动时间 |
| GET | `/api/v1/workbooks` | 文件列表，按最后修改时间降序，不包含快照 |
| POST | `/api/v1/workbooks` | 创建，JSON 请求包含 `name` 和 `snapshot`，返回完整文件 |
| GET | `/api/v1/workbooks/:id` | 获取元数据与完整 Univer 快照 |
| PUT | `/api/v1/workbooks/:id` | 保存，包含 `name`、`snapshot`、当前 `revision`，返回更新后的元数据 |
| DELETE | `/api/v1/workbooks/:id?revision=N` | 删除指定版本；版本过期返回 409 |

### 在线表格存储

`internal/service/workbook.go` 保存每个工作簿的完整 JSON 快照，包括公式、样式和插件资源。
写入临时文件、同步并关闭成功后替换正式文件；进程内锁保护读取版本与替换的完整事务。
数据刷新或服务重启后保留，目录已忽略提交。文件 ID 由服务端生成；读取与删除通过 `os.Root`
限制在配置目录内。单实例独占目录，多个实例共享目录不受支持。

请求必须为 `application/json`，最大 10 MB。名称最多 120 字符；最多 100 张工作表，
每表 100,000 行 / 1,024 列，全部工作表最多 200,000 个已存单元格。
无效输入返回 400，超限请求返回 413；不存在返回 404，版本冲突返回 409，存储错误返回 500。
内部存储错误只记录在服务端日志，HTTP 返回中文提示。

首版是共享工作区，没有用户身份和文件权限。该存储不连接现有 MongoDB / PostgreSQL；
之后可以换数据库存储并保留 HTTP 契约。生产使用须接入身份与权限，并挂载、备份持久目录。
评论与实时协作规划见 [聚合仓库实施方案](../docs/spreadsheet-plan.md)。

## 日志

用标准库 `log/slog`，不引第三方日志库 —— Go 1.21 起它是官方方案，生态已收敛到它。

- **生产**输出 JSON 到 stdout。必须 JSON：纯文本的多行记录（如 panic 栈）会被日志采集器拆成多条
- **开发**输出可读文本，本地排查不用跟 JSON 较劲
- 每个请求带 `request_id`：上游网关带了 `X-Request-ID` 就沿用（跨服务可串），没带就生成，并回写到响应头
- 用 `slog.InfoContext(ctx, ...)` 而非 `slog.Info(...)`，让日志跟随 context —— 接上 OpenTelemetry 后
  `trace_id` / `span_id` 会自动注入
- **不记录客户端 IP**：属于个人数据。要追攻击来源，反向代理那一层的访问日志更合适

handler 里取请求级 logger（自动带 `request_id`）：

```go
logging.FromContext(c.Request.Context()).InfoContext(ctx, "something happened", "key", value)
```

```json
{"time":"...","level":"INFO","msg":"request","request_id":"gw-999","method":"GET","path":"/api/v1/info","status":200,"duration_ms":0,"route":"/api/v1/info"}
```

## 所属聚合仓库

本仓库作为子模块挂载在 [lim-tools-group](https://github.com/limnova/lim-tools-group) 下，
由聚合仓库记录具体 commit。提交时先推本仓库、再 bump 聚合仓库的指针。
