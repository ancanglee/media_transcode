# 用户认证与管理

## 概述

系统支持两种认证方式：
1. **JWT Token** - 用于前端用户登录
2. **API Key** - 用于外部系统调用 API

用户信息存储在 DynamoDB 中。

## 快速开始

### 1. 创建用户表

```bash
# 使用提供的脚本创建 DynamoDB 用户表
./docs/create_user_table.sh video-transcode-users us-west-2
```

### 2. 配置环境变量

在 `config.env` 中添加：

```env
# 用户表名称
USER_TABLE=video-transcode-users

# JWT_SECRET 和 API_KEY 会自动生成，无需配置
# 如需固定 API Key，可手动指定：
# API_KEY=your-custom-api-key
```

### 3. 启动服务

服务启动时会：
- 自动创建默认管理员账户 (`admin/admin`)
- 自动生成 API Key 并在日志中打印

```
🔑 API Key: vt_xxxxxxxxxxxxxxxxxxxx
```

⚠️ **请在首次登录后立即修改默认密码！**

---

## 认证方式

### 方式一：JWT Token（前端/用户登录）

适用于 Web 管理界面和需要用户身份的场景。

#### 1. 登录获取 Token

```bash
curl -X POST http://localhost:9999/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}'
```

**响应:**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "username": "admin",
  "role": "admin"
}
```

#### 2. 使用 Token 调用 API

```bash
curl http://localhost:9999/api/tasks \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIs..."
```

### 方式二：API Key（外部系统调用）

适用于脚本、自动化工具、外部系统集成等场景，无需登录流程。

#### 获取 API Key

- 服务启动时在日志中查看：`🔑 API Key: vt_xxxxxxxxxxxx`
- 或在 `config.env` 中手动指定 `API_KEY=your-key`

#### 使用 API Key 调用 API

```bash
# 查询任务列表
curl http://localhost:9999/api/tasks \
  -H "X-API-Key: vt_xxxxxxxxxxxxxxxxxxxx"

# 添加转码任务
curl -X POST http://localhost:9999/api/queue/add \
  -H "X-API-Key: vt_xxxxxxxxxxxxxxxxxxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "input_bucket": "my-bucket",
    "input_key": "videos/sample.mp4",
    "transcode_types": ["mp4_standard", "thumbnail"]
  }'

# 查询任务详情
curl http://localhost:9999/api/tasks/task-id-here \
  -H "X-API-Key: vt_xxxxxxxxxxxxxxxxxxxx"
```

---

## API 接口

### 公开接口（无需认证）

| 接口 | 说明 |
|------|------|
| `GET /api/health` | 健康检查 |
| `POST /api/auth/login` | 用户登录 |

### 需要认证的接口

以下接口需要 `Authorization: Bearer <token>` 或 `X-API-Key: <key>`

#### 用户相关

| 接口 | 说明 |
|------|------|
| `GET /api/auth/me` | 获取当前用户信息 |
| `PUT /api/auth/password` | 修改自己的密码 |

#### 任务管理

| 接口 | 说明 |
|------|------|
| `GET /api/tasks` | 获取任务列表 |
| `GET /api/tasks/:task_id` | 获取任务详情 |
| `POST /api/tasks/:task_id/retry` | 重试任务 |
| `POST /api/tasks/:task_id/abort` | 中止任务 |
| `DELETE /api/tasks/:task_id` | 取消任务 |

#### 队列管理

| 接口 | 说明 |
|------|------|
| `GET /api/queue/status` | 获取队列状态 |
| `POST /api/queue/add` | 添加任务到队列 |
| `DELETE /api/queue/purge` | 清空队列 |

#### 预设管理

| 接口 | 说明 |
|------|------|
| `GET /api/presets` | 获取预设列表 |
| `POST /api/presets` | 创建预设 |
| `DELETE /api/presets/:preset_id` | 删除预设 |

### 管理员专用接口

以下接口需要管理员权限（role=admin）

| 接口 | 说明 |
|------|------|
| `GET /api/users` | 获取用户列表 |
| `POST /api/users` | 创建用户 |
| `DELETE /api/users/:username` | 删除用户 |
| `PUT /api/users/:username/password` | 修改用户密码 |

---

## 修改密码

### 用户修改自己的密码

```
PUT /api/auth/password
Authorization: Bearer <token>
```

**请求体:**
```json
{
  "old_password": "current_password",
  "new_password": "new_password"
}
```

## 用户管理 (仅管理员)

### 获取用户列表

```
GET /api/users
Authorization: Bearer <token>
```

### 创建用户

```
POST /api/users
Authorization: Bearer <token>
```

**请求体:**
```json
{
  "username": "newuser",
  "password": "password123",
  "role": "user"
}
```

角色可选值: `admin`, `user`

### 删除用户

```
DELETE /api/users/:username
Authorization: Bearer <token>
```

注意: 不能删除 `admin` 用户

### 修改用户密码 (管理员)

```
PUT /api/users/:username/password
Authorization: Bearer <token>
```

**请求体:**
```json
{
  "new_password": "new_password"
}
```

## 前端使用

### 登录流程

1. 访问 `/login` 页面
2. 输入用户名和密码
3. 登录成功后自动跳转到 `/admin` 管理界面

### Token 存储

登录成功后，Token 存储在 `localStorage`:
- `auth_token`: JWT 令牌
- `auth_user`: 用户信息 (JSON)

### 自动登出

当 Token 过期或无效时，系统会自动跳转到登录页面。

## 安全建议

1. **修改默认密码**: 首次登录后立即修改 admin 密码
2. **使用强密码**: 密码应包含大小写字母、数字和特殊字符
3. **保护 API Key**: API Key 具有管理员权限，请妥善保管
4. **固定 API Key**: 生产环境建议在 config.env 中固定 API_KEY，避免重启后变化
5. **HTTPS**: 生产环境建议使用 HTTPS 加密传输
6. **定期更换密码**: 建议定期更换用户密码

## DynamoDB 表结构

| 字段 | 类型 | 说明 |
|------|------|------|
| username | String (PK) | 用户名，主键 |
| password | String | 密码哈希 (SHA256) |
| role | String | 角色: admin/user |
| created_at | Timestamp | 创建时间 |
| updated_at | Timestamp | 更新时间 |

## 常见问题

### Q: API Key 每次重启都会变化？

A: 是的，如果没有在 config.env 中配置 `API_KEY`，每次启动会自动生成新的。生产环境建议固定配置：

```env
API_KEY=vt_your_fixed_api_key_here
```

### Q: JWT Token 和 API Key 有什么区别？

| 特性 | JWT Token | API Key |
|------|-----------|---------|
| 获取方式 | 登录获取 | 配置或自动生成 |
| 有效期 | 24小时 | 永久有效 |
| 适用场景 | Web界面、用户操作 | 脚本、自动化、外部系统 |
| 用户身份 | 关联具体用户 | 系统级别 (api用户) |

### Q: 如何在脚本中使用 API？

```bash
#!/bin/bash
API_KEY="vt_your_api_key"
API_URL="http://localhost:9999"

# 添加转码任务
curl -X POST "$API_URL/api/queue/add" \
  -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "input_bucket": "my-bucket",
    "input_key": "videos/input.mp4",
    "transcode_types": ["mp4_standard"]
  }'
```
