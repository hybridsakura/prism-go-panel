# RustDesk API 契约文档

本服务提供两类接口：

- **`/api/*`** — RustDesk 客户端契约，与 [kingmo888/rustdesk-api-server](https://github.com/kingmo888/rustdesk-api-server)（Python/Django）行为一致，官方客户端可直接对接
- **`/admin/api/*`** — 管理后台 JSON 接口，配合内嵌 SPA（`/admin`）使用

约定：

- 客户端接口除特别说明外均为 `POST`，请求体为 JSON
- 客户端认证使用 `Authorization: Bearer <access_token>`
- 错误以 HTTP 200 + `{"error": "中文提示"}` 返回（客户端只读取 error 字段）
- 管理后台接口使用会话 Cookie（`rdadmin_session`），未登录返回 401，非管理员返回 403

---

## 一、客户端契约（/api）

### 1. 登录

```
POST /api/login
```

请求：

```json
{
  "username": "admin",
  "password": "password123",
  "id": "123456789",
  "uuid": "machine-uuid",
  "autoLogin": true,
  "type": "client",
  "deviceInfo": {"os": "Windows 11"}
}
```

响应：

```json
{
  "access_token": "64位十六进制",
  "type": "access_token",
  "user": {"name": "admin"}
}
```

失败：`{"error": "帐号或密码错误！请重试，多次重试后将被锁定IP！"}`

说明：若设备已通过 sysinfo 注册且当前用户地址簿中还没有该设备，登录时会自动加入地址簿。

### 2. 登出

```
POST /api/logout
{"id": "123456789", "uuid": "machine-uuid"}
→ {"code": 1}
```

删除该设备名下的全部 token。

### 3. 当前用户

```
POST /api/currentUser
Authorization: Bearer <token>
```

响应：`{"access_token": "...", "type": "access_token", "name": "admin"}`
token 无效时返回空对象 `{}`。

### 4. 读取地址簿

```
GET /api/ab
Authorization: Bearer <token>
```

响应：

```json
{
  "updated_at": "2026-10-16T09:00:00+08:00",
  "data": "{\"tags\":[\"办公机\"],\"peers\":[{\"id\":\"123456789\",\"username\":\"alice\",\"hostname\":\"PC-1\",\"alias\":\"Alice\",\"platform\":\"Windows\",\"tags\":[\"办公机\"],\"hash\":\"...\"}],\"tag_colors\":\"{\\\"办公机\\\":16744192}\"}"
}
```

注意 `data` 是** JSON 字符串**，`tag_colors` 在其中又被编码为字符串（与参考版一致）。

### 5. 全量同步地址簿

```
POST /api/ab
Authorization: Bearer <token>
```

请求（`data` 为 JSON 字符串）：

```json
{
  "data": "{\"tags\":[\"办公机\"],\"tag_colors\":{\"办公机\":16744192},\"peers\":[{\"id\":\"987654321\",\"username\":\"bob\",\"hostname\":\"PC-BOB\",\"alias\":\"Bob\",\"platform\":\"Windows\",\"tags\":[\"办公机\"],\"hash\":\"...\"}]}"
}
```

响应：`{"code": 1, "data": "ok"}`（先删后插，单事务）

### 6. 旧版客户端兼容

```
POST /api/ab/get
Authorization: Bearer <token>
```

与 `GET /api/ab` 相同，供 x86-sciter 旧客户端使用。

### 7. 设备资产上报

```
POST /api/sysinfo
```

```json
{
  "id": "123456789",
  "uuid": "machine-uuid",
  "cpu": "Intel i7-12700",
  "hostname": "PC-ALICE",
  "memory": "16GB",
  "os": "Windows 11",
  "username": "alice",
  "version": "1.4.9"
}
```

响应：`{"data": "ok"}`

按 `(id, uuid)` UPSERT；客户端 IP 取 `X-Forwarded-For` 或连接地址。

### 8. 心跳

```
POST /api/heartbeat
{"id": "123456789", "uuid": "machine-uuid"}
→ {"data": "在线"}
```

作用：刷新设备 IP 与更新时间、判定在线状态、续期该设备名下全部 token。

### 9. 审计日志

```
POST /api/audit
```

三种形态：

```json
// 新建连接
{"action": "new", "conn_id": 1001, "ip": "10.0.0.5", "id": "123456789", "session_id": 55, "uuid": "machine-uuid"}

// 补充对端信息
{"conn_id": 1001, "session_id": 55, "peer": ["111222333", "..."]}

// 文件传输
{"is_file": true, "id": "123456789", "path": "/tmp/a.pdf", "peer_id": "111222333", "type": 1,
 "info": "{\"ip\": \"10.0.0.6\", \"files\": [[\"a.pdf\", 1048576]]}"}

// 关闭连接
{"action": "close", "conn_id": 1001}
```

响应均为 `{"code": 1, "data": "ok"}`。

### 10. 占位接口

`POST /api/users` → `{"code": 1, "data": "好的"}`
`POST /api/peers` → `{"code": 1, "data": "ok"}`

与参考版一致（客户端探测用，暂无实际数据）。

---

## 二、管理后台（/admin/api）

### 认证

| 接口 | 说明 |
|---|---|
| `POST /admin/api/login` | `{username, password}` → 设置会话 Cookie |
| `POST /admin/api/logout` | 清除会话 |
| `POST /admin/api/register` | `{user, pwd}`，需开放注册；首个用户成为管理员 |
| `GET /admin/api/me` | 当前登录用户信息 |

### 统计与设备

| 接口 | 说明 |
|---|---|
| `GET /admin/api/stats` | 设备总数/在线/离线/在线率、用户数、今日连接与文件传输数 |
| `GET /admin/api/devices?page=1&size=15&q=关键字&all=1` | 设备列表；`all=1` 仅管理员可见全量 |
| `GET /admin/api/devices/export` | 导出全部设备资产 xlsx（仅管理员） |

设备行字段：`rid, alias, hostname, username, owner, platform, cpu, memory, os, version, ip_address, status(online/offline), create_time, update_time`。

### 日志

| 接口 | 说明 |
|---|---|
| `GET /admin/api/conn_logs?page=1&size=15` | 连接日志 |
| `GET /admin/api/file_logs?page=1&size=15` | 文件传输日志 |

### 分享

| 接口 | 说明 |
|---|---|
| `GET /admin/api/shares` | 我的分享链接列表 |
| `POST /admin/api/shares` | `{peers: ["rid", ...]}` 生成链接（15 分钟有效，一次性） |
| `POST /admin/api/shares/{shash}/claim` | 领取他人分享的设备到本人地址簿 |

### 用户管理（仅管理员）

| 接口 | 说明 |
|---|---|
| `GET /admin/api/users` | 用户列表 |
| `POST /admin/api/users` | `{username, password, is_admin}` 创建用户 |
| `DELETE /admin/api/users/{id}` | 删除用户（不能删自己） |
| `POST /admin/api/users/{id}/password` | `{password}` 重置密码 |

### 服务端配置档案（仅管理员）

用于对接 hbbs/hbbr 部署信息并生成客户端配置；档案只是「记录」，不会改动 hbbs/hbbr 运行参数。

| 接口 | 说明 |
|---|---|
| `GET /admin/api/server-profile` | 读取档案 `{id_server, relay_server, api_server, key, updated_at, suggested_id_server, detected_key?}`；`detected_key` 仅在配置了 `HBBS_KEY_FILE` 且档案无 key 时返回 |
| `PUT /admin/api/server-profile` | 保存档案 `{id_server, relay_server, api_server, key}`；地址为 `host[:port]`，校验失败返回 `code:0` |
| `POST /admin/api/server-profile/check` | 连通性自检：按档案推导端口并 TCP 探测（UDP 跳过）→ `{results: [{name, addr, ok, latency, error}]}` |
| `GET /admin/api/server-profile/client-config` | 生成客户端配置：`filename`（重命名串）、`filename_elevate`（`,-qs` 提权变体）、`manual`（手填四项）、`ports`（端口放行清单，按 hbbs 基准端口 -1/+0/+2、hbbr +0/+2 推导）、`custom_build`（Actions 编译 secrets）、`notes` |

端口推导规则：hbbs 以 ID 服务器端口（默认 21116）为基准：TCP `p-1`（NAT 测试）/`p`（注册心跳）/`p+2`（WS 信令），UDP `p`；hbbr 以中继端口（默认 21117）为基准：TCP `q`/`q+2`；API 默认 21114。

---

## 三、其他

| 接口 | 说明 |
|---|---|
| `GET /healthz` | 健康检查 |
| `GET /admin` | 管理后台 SPA |
| `GET /webui` | RustDesk Web 控制端（需配置 `WEBCLIENT_DIR`） |
