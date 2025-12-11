# 运行命令

## 在远程服务器上执行以下命令

### 1. 编译程序
```bash
make build
```

### 2. 配置系统
```bash
cp config.example.env config.env
# 编辑config.env，填写你的AWS资源信息和凭证
```

### 3. 访问 Web 管理界面
启动 API 服务器后，可以通过浏览器访问图形化管理界面：
```
http://your-server:9999/admin
```

Web 管理界面功能：
- 📊 仪表盘：查看任务统计（等待/处理中/完成/失败）、最近任务，手动刷新
- 📋 任务队列管理：队列状态、任务列表、状态筛选、日期筛选、详情查看、重试、取消、中止
- ➕ 添加任务：图形化添加转码任务，选择转码类型

### 2.1 AWS凭证配置 (重要!)

#### 方式1: EC2 IAM角色 (推荐)
```bash
# 在AWS Console操作，无需在服务器上执行命令
# 1. IAM -> 角色 -> 创建角色
# 2. 选择: AWS服务 -> EC2
# 3. 添加策略: AmazonS3FullAccess, AmazonSQSFullAccess, AmazonDynamoDBFullAccess
# 4. 角色名称: video-transcode-role
# 5. EC2控制台 -> 实例 -> 操作 -> 安全 -> 修改IAM角色

# 验证角色是否生效:
curl http://169.254.169.254/latest/meta-data/iam/security-credentials/
aws sts get-caller-identity
```

#### 方式2: 在config.env中配置AWS凭证
```bash
# 取消注释并填写真实凭证:
# AWS_ACCESS_KEY_ID=your-access-key-id
# AWS_SECRET_ACCESS_KEY=your-secret-access-key
```

#### 方式3: 使用AWS CLI配置 (全局)
```bash
aws configure
# 输入: Access Key ID, Secret Access Key, Region, Output format
```

### 3. 运行API服务器 (端口9999)
```bash
make start-api
```

### 4. 运行GPU处理器 (在GPU服务器上)
```bash
make start-gpu
```

### 5. 测试API
```bash
# 健康检查
curl http://localhost:9999/api/health

# 查看队列状态
curl http://localhost:9999/api/queue/status

# 添加转码任务
curl -X POST http://localhost:9999/api/queue/add \
  -H "Content-Type: application/json" \
  -d '{
    "input_bucket": "your-bucket",
    "input_key": "video.mp4",
    "transcode_types": ["mp4_standard", "thumbnail"]
  }'

# 查看任务列表
curl http://localhost:9999/api/tasks

# 查看任务详情
curl http://localhost:9999/api/tasks/{task-id}
```

### 6. 使用 Web 管理界面
除了 API 接口，还可以通过浏览器访问图形化管理界面：
```
http://localhost:9999/admin
```

---

## API 接口完整文档

### 队列管理接口 (`/api/queue`)

| 方法 | 路径 | 描述 |
|------|------|------|
| GET | `/api/queue/status` | 获取队列状态 |
| POST | `/api/queue/add` | 添加任务到队列 |
| DELETE | `/api/queue/purge` | 清空队列（管理接口） |

### 任务管理接口 (`/api/tasks`)

| 方法 | 路径 | 描述 |
|------|------|------|
| GET | `/api/tasks` | 获取任务列表 |
| GET | `/api/tasks/:task_id` | 获取任务详情 |
| POST | `/api/tasks/:task_id/retry` | 重试任务（支持任意非处理中状态） |
| POST | `/api/tasks/:task_id/abort` | 中止任务（仅处理中状态，中止后状态变为failed） |
| DELETE | `/api/tasks/:task_id` | 取消任务（仅等待中状态） |

### 其他接口

| 方法 | 路径 | 描述 |
|------|------|------|
| GET | `/api/health` | 健康检查 |
| POST | `/api/upload` | 文件上传（待实现） |

### Web 管理界面

| 路径 | 描述 |
|------|------|
| `/` | 重定向到管理界面 |
| `/admin` | Web 图形化管理界面 |
| `/static/*` | 静态资源文件 |

---

### API 使用示例

#### 1. 健康检查

```bash
curl http://localhost:9999/api/health
```

响应：
```json
{
    "status": "healthy",
    "timestamp": 1702195200,
    "message": "API服务器运行正常"
}
```

#### 2. 获取队列状态

```bash
curl http://localhost:9999/api/queue/status
```

响应：
```json
{
    "approximate_number_of_messages": 5,
    "approximate_number_of_messages_not_visible": 2
}
```

#### 3. 添加任务到队列

```bash
curl -X POST http://localhost:9999/api/queue/add \
  -H "Content-Type: application/json" \
  -d '{
    "input_bucket": "my-input-bucket",
    "input_key": "videos/sample.mp4",
    "transcode_types": ["mp4_standard", "mp4_smooth", "thumbnail"]
  }'
```

响应：
```json
{
    "message": "任务已添加到队列",
    "task_id": "550e8400-e29b-41d4-a716-446655440000",
    "task": {
        "task_id": "550e8400-e29b-41d4-a716-446655440000",
        "input_bucket": "my-input-bucket",
        "input_key": "videos/sample.mp4",
        "output_bucket": "my-output-bucket",
        "transcode_types": ["mp4_standard", "mp4_smooth", "thumbnail"],
        "status": "pending",
        "created_at": "2025-12-10T10:00:00Z",
        "updated_at": "2025-12-10T10:00:00Z",
        "retry_count": 0,
        "max_retries": 3,
        "progress": {
            "mp4_standard": "pending",
            "mp4_smooth": "pending",
            "thumbnail": "pending"
        },
        "output_files": {}
    }
}
```

#### 4. 获取任务列表

```bash
# 获取所有任务（默认分页）
curl "http://localhost:9999/api/tasks"

# 按状态筛选 + 分页
curl "http://localhost:9999/api/tasks?status=pending&limit=20&offset=0"

# 获取失败的任务
curl "http://localhost:9999/api/tasks?status=failed"
```

响应：
```json
{
    "tasks": [
        {
            "task_id": "550e8400-e29b-41d4-a716-446655440000",
            "input_bucket": "my-input-bucket",
            "input_key": "videos/sample.mp4",
            "status": "completed",
            "progress": {
                "mp4_standard": "completed",
                "mp4_smooth": "completed",
                "thumbnail": "completed"
            },
            "output_files": {
                "mp4_standard": "sample_mp4_standard_1702195200.mp4",
                "mp4_smooth": "sample_mp4_smooth_1702195200.mp4",
                "thumbnail": "sample_thumbnail_1702195200.jpg"
            }
        }
    ],
    "total": 1,
    "limit": 10,
    "offset": 0
}
```

#### 5. 获取任务详情

```bash
curl http://localhost:9999/api/tasks/550e8400-e29b-41d4-a716-446655440000
```

响应：
```json
{
    "task_id": "550e8400-e29b-41d4-a716-446655440000",
    "input_bucket": "my-input-bucket",
    "input_key": "videos/sample.mp4",
    "output_bucket": "my-output-bucket",
    "transcode_types": ["mp4_standard", "mp4_smooth", "thumbnail"],
    "status": "completed",
    "created_at": "2025-12-10T10:00:00Z",
    "updated_at": "2025-12-10T10:05:00Z",
    "started_at": "2025-12-10T10:00:05Z",
    "completed_at": "2025-12-10T10:05:00Z",
    "retry_count": 0,
    "max_retries": 3,
    "progress": {
        "mp4_standard": "completed",
        "mp4_smooth": "completed",
        "thumbnail": "completed"
    },
    "output_files": {
        "mp4_standard": "sample_mp4_standard_1702195200.mp4",
        "mp4_smooth": "sample_mp4_smooth_1702195200.mp4",
        "thumbnail": "sample_thumbnail_1702195200.jpg"
    }
}
```

#### 6. 重试任务（支持任意状态）

```bash
curl -X POST http://localhost:9999/api/tasks/550e8400-e29b-41d4-a716-446655440000/retry
```

说明：支持重试任意状态的任务（pending、completed、failed、cancelled 等），仅 processing 状态的任务不能重试。

响应：
```json
{
    "message": "任务重试成功",
    "task": {
        "task_id": "550e8400-e29b-41d4-a716-446655440000",
        "status": "retrying",
        "retry_count": 1,
        "progress": {
            "mp4_standard": "pending",
            "mp4_smooth": "pending",
            "thumbnail": "pending"
        }
    }
}
```

#### 7. 取消任务

```bash
curl -X DELETE http://localhost:9999/api/tasks/550e8400-e29b-41d4-a716-446655440000
```

响应：
```json
{
    "message": "任务已取消",
    "task_id": "550e8400-e29b-41d4-a716-446655440000",
    "removed_from_queue": true
}
```

#### 8. 清空队列

```bash
curl -X DELETE http://localhost:9999/api/queue/purge
```

响应：
```json
{
    "message": "队列已清空"
}
```

---

## 配置文件 (config.env)

```bash
# AWS配置
AWS_REGION=ap-southeast-1
INPUT_BUCKET=your-input-bucket
OUTPUT_BUCKET=your-output-bucket
SQS_QUEUE_URL=https://sqs.ap-southeast-1.amazonaws.com/123456789/your-queue-name
DYNAMODB_TABLE=your-dynamodb-table

# AWS凭证 (如果没有IAM角色，必须配置)
AWS_ACCESS_KEY_ID=your-access-key-id
AWS_SECRET_ACCESS_KEY=your-secret-access-key

# API服务器配置 (必须使用9999端口)
API_PORT=9999
API_HOST=0.0.0.0
DEBUG_MODE=false

# GPU处理器配置
TEMP_DIR=/tmp/ffmpeg_processing
MAX_CONCURRENT_TASKS=2
POLL_INTERVAL=10s
```

## 支持的转码格式

- `mp4_standard` - 标清MP4 (848x480, 800k码率)
- `mp4_smooth` - 流畅MP4 (640x360, 400k码率)
- `hdlbr_h265` - 高质量H265 (原分辨率, 6000k码率)
- `lcd_h265` - LCD优化H265 (原分辨率, CRF22)
- `h265_mute` - 静音H265 (原分辨率, 2867k码率)
- `custom_mute_preview` - 静音预览 (原分辨率, CRF23)
- `thumbnail` - 缩略图 (1280x720 JPG)

## 服务管理命令

### 启动服务
```bash
# 启动所有服务
make start-all

# 分别启动
make start-api    # API服务器
make start-gpu    # GPU处理器
```

### 停止服务
```bash
# 停止所有服务
make stop-all

# 分别停止
make stop-api   # 仅停止API服务器
make stop-gpu   # 仅停止GPU处理器

# 强制杀掉所有相关进程
pkill -9 -f "exe/api-server"
pkill -9 -f "exe/gpu-processor"

# 确认端口已释放
lsof -i :9999
```

### 查看状态和日志
```bash
# 查看服务状态
make status

# 查看日志
make logs                    # 查看所有日志摘要
tail -f api-server.log      # 实时查看API服务器日志
tail -f gpu-processor.log   # 实时查看GPU处理器日志
```

## 常见问题排查

### 1. API健康检查无响应
```bash
# 问题: curl http://localhost:9999/api/health 无响应

# 排查步骤:
make status                 # 检查服务状态
lsof -i :9999              # 检查端口占用
tail -20 api-server.log    # 查看日志
make stop-api && make start-api  # 重启服务
```

### 2. AWS凭证错误
```bash
# 错误: "no EC2 IMDS role found" 或 "get credentials: failed"

# 解决方案1: 配置AWS凭证
# 编辑config.env，取消注释并填写:
# AWS_ACCESS_KEY_ID=your-key
# AWS_SECRET_ACCESS_KEY=your-secret

# 解决方案2: 验证AWS配置
aws sts get-caller-identity  # 测试凭证是否有效
aws sqs get-queue-attributes --queue-url your-queue-url  # 测试SQS权限

# 解决方案3: 配置IAM角色 (推荐)
# 在AWS Console为EC2实例配置IAM角色:
# 1. IAM -> 角色 -> 创建角色 -> EC2
# 2. 添加权限策略 (S3, SQS, DynamoDB)
# 3. EC2 -> 实例 -> 修改IAM角色
# 4. 验证: curl http://169.254.169.254/latest/meta-data/iam/security-credentials/
```

### 3. GPU驱动问题
```bash
# 错误: "nvidia-smi: command not found" 或 "couldn't communicate with driver"

# 解决步骤:
lspci | grep -i nvidia      # 确认有GPU硬件
sudo apt install nvidia-driver-580-server  # 安装驱动
sudo reboot                 # 重启系统
nvidia-smi                  # 验证驱动
```

### 4. 编译错误
```bash
# 错误: "imported and not used" 或其他Go编译错误

# 解决步骤:
make clean                  # 清理编译文件
make build                  # 重新编译
go mod tidy                 # 整理依赖
```

### 5. 端口被占用
```bash
# 错误: "bind: address already in use"

# 解决步骤:
lsof -i :9999              # 查看端口占用
make stop-all              # 停止所有服务
sudo kill -9 $(lsof -t -i:9999)  # 强制停止占用进程
make start-api             # 重新启动
```

## 测试验证

### 完整测试流程
```bash
# 1. 启动服务
make start-all

# 2. 等待服务启动
sleep 5

# 3. 健康检查
curl http://localhost:9999/api/health

# 4. 队列状态检查
curl http://localhost:9999/api/queue/status

# 5. 提交测试任务 (需要先上传视频到S3)
curl -X POST http://localhost:9999/api/queue/add \
  -H "Content-Type: application/json" \
  -d '{
    "input_bucket": "your-input-bucket",
    "input_key": "test-video.mp4",
    "transcode_types": ["mp4_standard", "thumbnail"]
  }'

# 6. 查看任务状态
curl http://localhost:9999/api/tasks

# 7. 监控处理日志
tail -f gpu-processor.log
```

### AWS资源验证
```bash
# 验证S3访问
aws s3 ls s3://your-input-bucket/
aws s3 ls s3://your-output-bucket/

# 验证SQS访问
aws sqs get-queue-attributes --queue-url your-queue-url

# 验证DynamoDB访问
aws dynamodb describe-table --table-name your-table-name
```## IA
M角色配置详细指南

### 为什么使用IAM角色？
- **安全性**: 不需要在代码中存储AWS凭证
- **自动轮换**: AWS自动管理临时凭证
- **最佳实践**: AWS推荐的安全方式
- **简化管理**: 无需手动更新凭证

### 详细配置步骤

#### 步骤1: 创建IAM角色
```bash
# 在AWS Console操作:
# 1. 登录AWS Console
# 2. 进入IAM服务
# 3. 点击"角色" -> "创建角色"
# 4. 可信实体类型: "AWS服务"
# 5. 使用案例: "EC2"
# 6. 点击"下一步"
```

#### 步骤2: 选择权限策略
```bash
# 选择AWS托管策略 (简单方式):
# ✓ AmazonS3FullAccess
# ✓ AmazonSQSFullAccess
# ✓ AmazonDynamoDBFullAccess

# 或创建自定义策略 (最小权限原则):
# 策略名称: VideoTranscodeCustomPolicy
# 权限: 仅访问特定的S3桶、SQS队列、DynamoDB表
```

#### 步骤3: 完成角色创建
```bash
# 角色详情:
# - 角色名称: video-transcode-role
# - 描述: GPU视频转码系统专用IAM角色
# - 最大会话持续时间: 1小时 (默认)
# 点击"创建角色"
```

#### 步骤4: 附加角色到EC2实例
```bash
# 在AWS Console操作:
# 1. 进入EC2控制台
# 2. 选择你的GPU实例
# 3. 点击"操作" -> "安全" -> "修改IAM角色"
# 4. IAM角色下拉菜单选择: video-transcode-role
# 5. 点击"更新IAM角色"
```

#### 步骤5: 验证配置
```bash
# 在EC2实例上执行验证命令:

# 1. 检查实例元数据中的角色信息
curl http://169.254.169.254/latest/meta-data/iam/security-credentials/
# 应该返回: video-transcode-role

# 2. 获取临时凭证
curl http://169.254.169.254/latest/meta-data/iam/security-credentials/video-transcode-role
# 应该返回JSON格式的临时凭证

# 3. 测试AWS身份
aws sts get-caller-identity
# 应该显示角色ARN，而不是用户ARN

# 4. 测试各项服务权限
aws s3 ls s3://your-input-bucket/
aws sqs get-queue-attributes --queue-url your-queue-url
aws dynamodb describe-table --table-name your-table-name
```

### 自定义IAM策略示例

如果选择创建自定义策略，使用以下JSON (替换资源ARN):

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "S3BucketAccess",
            "Effect": "Allow",
            "Action": [
                "s3:GetObject",
                "s3:PutObject",
                "s3:DeleteObject",
                "s3:ListBucket"
            ],
            "Resource": [
                "arn:aws:s3:::your-input-bucket",
                "arn:aws:s3:::your-input-bucket/*",
                "arn:aws:s3:::your-output-bucket",
                "arn:aws:s3:::your-output-bucket/*"
            ]
        },
        {
            "Sid": "SQSQueueAccess",
            "Effect": "Allow",
            "Action": [
                "sqs:SendMessage",
                "sqs:ReceiveMessage",
                "sqs:DeleteMessage",
                "sqs:GetQueueAttributes",
                "sqs:ChangeMessageVisibility"
            ],
            "Resource": "arn:aws:sqs:ap-southeast-1:286345677825:video-transcode"
        },
        {
            "Sid": "DynamoDBTableAccess",
            "Effect": "Allow",
            "Action": [
                "dynamodb:GetItem",
                "dynamodb:PutItem",
                "dynamodb:UpdateItem",
                "dynamodb:DeleteItem",
                "dynamodb:Query",
                "dynamodb:Scan"
            ],
            "Resource": "arn:aws:dynamodb:ap-southeast-1:286345677825:table/video-transcode"
        }
    ]
}
```

### 故障排除

#### 角色未生效
```bash
# 问题: 配置角色后仍然报凭证错误

# 解决步骤:
# 1. 等待几分钟让角色生效
# 2. 重启应用服务
make stop-all && make start-all

# 3. 检查角色是否正确附加
curl http://169.254.169.254/latest/meta-data/iam/security-credentials/

# 4. 检查角色权限是否足够
aws iam get-role --role-name video-transcode-role
aws iam list-attached-role-policies --role-name video-transcode-role
```

#### 权限不足
```bash
# 问题: 特定AWS服务访问被拒绝

# 解决步骤:
# 1. 检查具体的权限错误
tail -f api-server.log | grep -i "access denied\|forbidden"

# 2. 测试特定服务权限
aws s3 ls s3://your-bucket/ --debug
aws sqs receive-message --queue-url your-queue-url --debug

# 3. 在IAM控制台添加缺失的权限
# 或使用策略模拟器测试权限
```

### 配置完成后

配置IAM角色后，确保在config.env中**注释掉**或**删除**AWS凭证配置：

```bash
# 注释掉这些行，让系统使用IAM角色:
# AWS_ACCESS_KEY_ID=your-access-key-id
# AWS_SECRET_ACCESS_KEY=your-secret-access-key

# 重启服务使配置生效
make stop-all
make start-all
```