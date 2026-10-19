# Prism-Go-Panel（`prism-go-panel`）

> **命名与商标声明**：本项目以 **Prism-Go-Panel** 名义发布，与 RustDesk 官方及其商业公司**无任何隶属或背书关系**。项目名称、镜像名与产物名均不含第三方商标，以避免商标纠纷；代码中出现的 "RustDesk" 仅用于说明协议对象与兼容目标（指代客户端软件本身）。
>
> **署名**：`com.akiraprism`
>
> **项目声明**：本项目是对原项目 [kingmo888/rustdesk-api-server](https://github.com/kingmo888/rustdesk-api-server)（Python/Django 实现）的 **Go 语言重构版**，目的是在保持客户端 API 契约完全一致的前提下获得更高的性能与更简单的部署。感谢原作者 kingmo888 及原项目社区贡献者的工作。
>
> **AI 声明**：本项目的构建过程有 AI 助手参与，具体见文末 [AI 协作声明](#ai-协作声明)。

**目标运行环境为 Linux 与 Docker**（生产推荐），同时提供 **Windows 独立安装包**（注册为系统服务，开机自启）。
所有界面均为 Web UI：管理后台单页应用直接内嵌于二进制中，另可选托管 RustDesk Web 控制端。
**默认端口沿用原项目：`21114`**。

相比 Python 版的改进：

| 维度 | Python 原项目 | 本 Go 版 |
|---|---|---|
| 运行时 | Django + Gunicorn/uWSGI（多进程） | 单二进制，无外部依赖 |
| 并发模型 | 每请求进程/线程 | goroutine，单机可支撑数万并发心跳 |
| 内存占用 | 100MB+ | 通常 < 30MB |
| 在线状态 | 每次请求查库比对 `update_time` | 内存 Map O(1) 判定 |
| token 校验 | 每次请求查库 | 内存缓存，未命中才回查 |
| 资产写入 | ORM update | UPSERT 单条语句 |
| 数据库 | SQLite / MySQL | SQLite（WAL，纯 Go 驱动，免 CGO）/ MySQL |
| 部署 | Python 环境 + 依赖安装 | 单文件二进制 / Docker 镜像 / Windows 安装包 |

## 相比原项目新增的功能

在保持客户端 API 契约兼容的基础上，本 Go 版还额外增加了以下原项目不具备的能力：

- **服务端配置档案**（管理员）：在后台记录 hbbs/hbbr 的部署信息（地址、自定义端口、公钥），据此一键生成客户端配置——Windows 重命名串（`rustdesk-host=...,key=....exe` 及 `,-qs` 自动提权变体）、客户端手动填写对照、防火墙端口放行清单、GitHub Actions 编译专属客户端所需 secrets，并提供端口**连通性自检**
- **管理后台整体重设计**：GitHub Primer 风格 UI，浅色 / 深色双主题（跟随系统 + 手动切换），无外部依赖、内嵌于单个二进制
- **Windows 独立安装包**：注册为原生系统服务（开机自启、优雅启停），一键安装 / 卸载脚本
- **一键导出全部设备资产为 Excel（xlsx）**
- **GitHub Actions 全自动 CI/CD**：push 即跑 `gofmt`/`vet`/`test -race`/多平台交叉编译；打 `v*` tag 自动发布多平台产物与多架构 Docker 镜像（ghcr.io）
- **运维加固**：优雅退出、panic 恢复、请求体大小限制、结构化日志（slog，可落文件）、`/healthz` 健康检查、可选登录限速

## 功能特性

- **RustDesk 客户端契约**（`/api/*`）：登录/登出、当前用户、地址簿读写（含旧版 sciter 客户端）、设备资产上报（sysinfo）、心跳与 token 保活、连接/文件审计日志
- **管理后台**（`/admin`，内嵌单页应用，GitHub Primer 风格，浅色/深色双主题）：
  - 设备在线状态实时查看（心跳驱动）
  - 资产统计面板（设备总数/在线/离线/在线率/今日连接/今日文件传输）
  - 设备列表（我的设备 / 管理员全量视图）、关键字搜索
  - 一键导出全部设备资产为 Excel（xlsx）
  - 连接日志、文件传输日志（分页）
  - 分享链接管理（生成 / 领取，15 分钟有效，一次性）
  - 用户管理（管理员）：创建、删除、重置密码、管理员授权
  - **服务端配置档案**（管理员）：记录 hbbs/hbbr 部署信息（地址、自定义端口、公钥），一键生成客户端配置（Windows 重命名串 / 手动填写 / 端口放行清单 / Actions 编译 secrets），并提供端口连通性自检
  - **ID 冲突检测**（管理员）：同一远程 ID 被多台机器占用（硬盘克隆后遗症）时自动识别，向冲突方推送「请更换 ID」提示，避免远程连接出现「无法验证端到端加密」
- **RustDesk Web 控制端**（`/webui`，可选）：托管 Flutter 编译产物，与原项目目录结构一致
- **运维特性**：优雅退出（含 Windows 服务 SCM 启停）、panic 恢复、请求体大小限制、结构化日志（slog，支持输出到文件）、`/healthz` 健康检查、可选登录限速

## 快速开始

### Docker（Linux 生产环境，推荐）

```bash
docker build -t github.com/hybridsakura/prism-go-panel .
docker run -d --name prism-go-panel \
  -p 21114:21114 \
  -e ADMIN_USER=admin -e ADMIN_PASS=change-me-please \
  -e ALLOW_REGISTRATION=true \
  -v ./data:/data \
  github.com/hybridsakura/prism-go-panel
```

或使用 docker-compose：

```bash
docker compose up -d
```

打 tag 后 GitHub Actions 会自动构建并推送多架构镜像（linux/amd64、linux/arm64）到 `ghcr.io`，届时可直接：

```bash
docker pull ghcr.io/<你的用户名>/github.com/hybridsakura/prism-go-panel:latest
```

### Linux 二进制

从 Releases 下载对应架构的 tar.gz，解压后直接运行：

```bash
./prism-go-panel
# 配合 systemd 时可参考 packaging/ 或自行编写 unit 文件
```

### Windows 安装包

从 Releases 下载 `prism-go-panel-windows-amd64.zip`，解压后以**管理员身份**运行：

```powershell
powershell -ExecutionPolicy Bypass -File install.ps1
# 可选：powershell -ExecutionPolicy Bypass -File install.ps1 -AdminUser admin -AdminPass 'StrongPass!2026'
```

安装程序会把服务注册为 Windows 服务（`prism-go-panel`，开机自启），数据与日志默认位于 `C:\ProgramData\prism-go-panel`。
卸载：`uninstall.ps1`（默认保留数据，加 `-RemoveData` 一并清除）。

### 源码运行

```bash
go run ./cmd/server
```

启动后访问：

- 管理后台：`http://<服务器IP>:21114/admin`（首个注册账号自动成为管理员）
- Web 控制端：`http://<服务器IP>:21114/webui`（需提供静态资源目录）
- 健康检查：`http://<服务器IP>:21114/healthz`

## 部署拓扑

本服务与官方 OSS 版 `hbbs`/`hbbr` **并列部署**，互不替代，端口沿用 RustDesk 与原项目的既有约定：

```
RustDesk 客户端
   ├── hbbs  21115/21116/21118  ID 注册、NAT 打洞、信令
   ├── hbbr  21117/21119        流量中继
   └── 本服务 21114             账号、地址簿、心跳、资产、管理后台
```

客户端需要把 **API 服务器地址** 配置为本服务（安装时 `--custom-server` 或企业版部署脚本）。
被控端需以**服务模式**安装，才会定时上报 sysinfo 资产与心跳。

## 配置项

全部通过环境变量配置，详见 [.env.example](.env.example)：

| 变量 | 默认值 | 说明 |
|---|---|---|
| `HOST` / `PORT` | `0.0.0.0` / `21114` | 监听地址与端口（沿用原项目端口） |
| `DB_URL` | `./prism.db` | SQLite 路径（支持 `file:` URI）或 MySQL DSN |
| `ALLOW_REGISTRATION` | `true` | 是否开放网页自助注册 |
| `ADMIN_USER` / `ADMIN_PASS` | 空 | 初始管理员（仅空库时创建） |
| `TOKEN_TTL_SECONDS` | `7200` | access_token 有效期，心跳续期 |
| `ONLINE_TIMEOUT_SECONDS` | `120` | 在线判定阈值 |
| `SESSION_TTL_SECONDS` | `43200` | 管理后台会话有效期 |
| `LOGIN_RATE_LIMIT` | `0`（关闭） | 登录限速（次/分钟/IP） |
| `WEBCLIENT_DIR` | `./webui-dist` | Web 控制端静态目录 |
| `ID_SERVER` | 空（取请求主机） | Web 控制端使用的 ID 服务器 |
| `HBBS_KEY_FILE` | 空 | hbbs 的 `id_ed25519.pub` 路径（一体部署时挂载 hbbs 目录），后台可自动填入公钥 |
| `ID_CONFLICT_ENFORCE` | `true` | 检测到远程 ID 重复时，拒绝「非原机」登录并提示更换 ID；设为 `false` 仅记录不拦截 |
| `LOG_LEVEL` | `info` | 日志级别（`RUST_LOG` 为兼容别名） |
| `LOG_FILE` | 空（标准输出） | 日志文件路径，Windows 服务模式建议配置 |

## 启用 Web 控制端（/webui）

把 Python 原项目的 `static/web_client` 目录拷贝为 `webui-dist`（或设置 `WEBCLIENT_DIR` 指向它），
目录结构保持为 `{module,js,assets,canvaskit@*,ogvjs-*}`。入口 JS 与 canvaskit 目录名会自动探测
（Flutter 构建产物带 hash 后缀，无需手工修改）。

## 从 Python 版迁移

- 数据库**不通用**（Django schema 不同），Go 版使用独立的 `prism.db`
- 内置迁移工具可导入旧版的用户、设备资产与日志（**密码哈希直接复用，用户可继续用原密码登录**）：

```bash
# 先停掉 API 容器，把旧库和本服务数据库放在同一台机器上
./legacy-import -src /path/to/old/db.sqlite3 -dst /path/to/prism.db --dry-run  # 先预览
./legacy-import -src /path/to/old/db.sqlite3 -dst /path/to/prism.db
```

- 若不想迁移，也可让用户重新注册；第一个注册的用户自动成为管理员（与原项目一致）
- 客户端无需任何改动，API 地址与端口不变

## 与 Python 原项目的行为差异

1. `POST /api/ab` 成功时返回 `{"code":1}`（原项目固定返回 `code:102`，客户端不解析该响应）
2. 登录时自动绑定设备到地址簿的判重范围为「同一用户下的同一设备」（原项目为全局 rid，会把他人已添加的设备跳过）
3. 地址簿中无标签的设备输出 `tags: []`（原项目可能输出 `[""]`）
4. 新增可选的登录限速、请求体大小限制、panic 恢复等加固项（默认不改变既有行为）

## API 文档

完整的接口契约见 [docs/api-contract.md](docs/api-contract.md)。

## 开发与发布

```bash
go test ./...        # 运行测试（含端到端 API 契约测试）
go vet ./...         # 静态检查
gofmt -l .           # 格式检查（应无输出）

# 构建全平台发布产物（Linux/macOS tar.gz + Windows 安装包 zip）
./packaging/build-release.sh v1.0.0
```

CI/CD（GitHub Actions）：

- `ci.yml`：push/PR 时运行 gofmt、vet、`go test -race`、五平台交叉编译
- `release.yml`：打 `v*` tag 时自动构建各平台产物发布 Release，并推送多架构 Docker 镜像到 ghcr.io

目录结构：

```
cmd/server/            程序入口（含 Windows 服务托管支持）
internal/
  api/                 HTTP 处理器：/api 客户端契约 + /admin/api 管理接口
  auth/                密码哈希、access_token、管理后台会话
  config/              环境变量配置
  database/            连接池、WAL、自动迁移
  models/              数据模型
  stats/               设备在线状态跟踪
  web/                 内嵌管理后台 SPA（admin/）+ Web 控制端托管
packaging/
  build-release.sh     全平台发布打包脚本
  windows/             Windows 安装包（服务注册/卸载脚本）
.github/workflows/     CI 与 Release 流水线
```

## AI 协作声明

本项目在构建过程中由以下 AI 助手参与完成：

| AI | 参与时间 | 参与环节 |
|---|---|---|
| Kimi K3（月之暗面 Moonshot AI，经由 WorkBuddy 编程助手） | 2026-10 | 原项目调研与 API 契约梳理、架构设计、全部 Go 代码编写、测试体系建设、文档撰写、打包与发布配置 |
| DeepSeek-V4-Pro（深度求索 DeepSeek，经由 WorkBuddy 编程助手） | 2026-10 | 服务端配置档案（对接 hbbs/hbbr）与客户端配置生成、管理后台 GitHub Primer 风格 UI 重写（浅色/深色双主题）、测试与发布维护 |

> 如后续维护中有其他 AI 工具参与，请在此表中补充记录，保持透明。

## 许可证

MIT。注意：本仓库为独立重构实现；原项目 [kingmo888/rustdesk-api-server](https://github.com/kingmo888/rustdesk-api-server) 未附带开源许可证文件，若需引用其代码或资源文件（如 Web 控制端静态产物），请遵循原作者的相关要求。
