# GPU视频转码系统

基于GPU加速的FFmpeg视频转码处理器，支持队列管理和任务监测。

## 系统架构

### 架构图

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   客户端应用     │    │   API服务器      │    │   GPU处理器      │
│                │    │  (任意服务器)    │    │  (GPU服务器)     │
│                │    │                │    │                │
│  - Web管理界面   │───▶│  - REST API     │    │  - FFmpeg处理   │
│  - 移动应用      │    │  - Web界面(/admin)│   │  - GPU加速      │
│  - 第三方集成    │    │  - 任务管理      │    │  - 并发处理      │
│                │    │  - 队列监控      │    │                │
└─────────────────┘    └─────────────────┘    └─────────────────┘
                              │                        │
                              ▼                        ▼
                    ┌─────────────────┐    ┌─────────────────┐
                    │   AWS SQS       │◀───│   AWS S3        │
                    │   消息队列       │    │   存储桶         │
                    │                │    │                │
                    │  - 任务队列      │    │  - 输入视频      │
                    │  - 状态更新      │    │  - 输出视频      │
                    │  - 错误处理      │    │  - S3事件通知    │
                    └─────────────────┘    └─────────────────┘
                              │              (自动触发转码)
                              ▼
                    ┌─────────────────┐
                    │   AWS DynamoDB  │
                    │   数据库         │
                    │                │
                    │  - 任务状态      │
                    │  - 处理历史      │
                    │  - 元数据        │
                    └─────────────────┘
```

### 任务触发方式

系统支持两种任务触发方式：

1. **自动触发（S3事件通知）**: 上传视频到 S3 输入桶时自动触发转码，使用默认转码类型
2. **手动触发（API调用）**: 通过 REST API 添加任务，可指定自定义转码类型

### 组件说明

**API服务器** (`cmd/api-server`)
- 提供REST API接口
- 提供Web图形化管理界面 (`/admin`)
- 管理转码任务队列
- 监控处理状态
- 可部署在任意服务器上

**GPU处理器** (`cmd/gpu-processor`)
- 执行实际的视频转码
- 利用GPU硬件加速
- 必须部署在配备GPU的服务器上
- 支持并发处理多个任务

**AWS服务**
- **S3**: 存储输入和输出视频文件
- **SQS**: 任务队列管理
- **DynamoDB**: 任务状态和元数据存储

## 快速开始

> **注意**: 完整部署请参考下面的"部署指南"部分，这里仅为开发测试提供快速启动方式

### 开发环境快速启动

1. **编译** (在开发机器上)
```bash
make build
```

2. **配置** (在开发机器上)
```bash
cp config.example.env config.env
# 编辑config.env填写AWS资源信息
```

3. **运行** (后台运行，适合同一台机器)
```bash
# 方式1: 分别启动
make start-api    # 启动API服务器 (后台运行，端口9999)
make start-gpu    # 启动GPU处理器 (后台运行)

# 方式2: 同时启动所有服务
make start-all

# 查看服务状态
make status

# 查看日志
make logs

# 停止服务
make stop-all     # 停止所有服务
make stop-api     # 仅停止API服务器
make stop-gpu     # 仅停止GPU处理器
```

4. **访问 Web 管理界面**

启动 API 服务器后，打开浏览器访问：
```
http://localhost:9999/admin
```

Web 管理界面提供：
- 📊 仪表盘：实时查看队列状态、今日统计、最近任务
- 📋 任务管理：任务列表、状态筛选、日期筛选、详情查看、重试、取消
- 📬 队列管理：查看队列状态、清空队列
- ➕ 添加任务：图形化添加转码任务，选择转码类型

5. **API 测试** (可选，也可以使用 Web 界面)
```bash
# 健康检查
curl http://localhost:9999/api/health

# 队列状态
curl http://localhost:9999/api/queue/status

# 添加任务
curl -X POST http://localhost:9999/api/queue/add \
  -H "Content-Type: application/json" \
  -d '{
    "input_bucket": "your-bucket",
    "input_key": "video.mp4", 
    "transcode_types": ["mp4_standard"]
  }'

# 查询任务列表（支持按日期和状态过滤）
curl "http://localhost:9999/api/tasks?date=2025-01-15&status=completed&limit=20"
```

## Web 管理界面

系统提供了一个易于使用的图形化 Web 管理界面，无需记忆 API 命令即可管理队列和任务。

### 访问方式

启动 API 服务器后，打开浏览器访问：
```
http://your-server:9999/admin
```

### 功能特性

| 功能模块 | 说明 |
|---------|------|
| 📊 仪表盘 | 实时显示队列状态、今日完成/失败统计、最近任务列表 |
| 📋 任务管理 | 任务列表查看、状态筛选、日期筛选、分页浏览、任务详情、重试、取消 |
| 📬 队列管理 | 查看队列等待/处理中消息数量、一键清空队列 |
| ➕ 添加任务 | 图形化添加转码任务，可选择多种转码类型 |

### 界面截图说明

- **仪表盘**: 首页展示系统整体状态，包括队列消息数、今日处理统计、最近任务快速预览
- **任务管理**: 支持按状态（等待中/处理中/已完成/失败等）和日期筛选任务，点击任务可查看详细信息
- **队列管理**: 显示 SQS 队列的实时状态，支持清空队列操作
- **添加任务**: 填写 S3 桶名和文件路径，勾选需要的转码类型即可提交任务

## 支持的转码格式

- `mp4_standard` - 标清MP4 (848x480)
- `mp4_smooth` - 流畅MP4 (640x360)  
- `hdlbr_h265` - 高质量H265
- `lcd_h265` - LCD优化H265
- `h265_mute` - 静音H265
- `custom_mute_preview` - 静音预览
- `thumbnail` - 缩略图JPG

## 部署指南

### 前置条件

**AWS Console 操作**
- AWS账户和适当的IAM权限
- 已创建的S3存储桶
- 已配置的SQS队列
- 已创建的DynamoDB表

**GPU服务器要求**
- NVIDIA GPU (支持CUDA)
- Ubuntu 20.04+ 或类似Linux发行版
- 已安装NVIDIA驱动和CUDA工具包
- 已安装FFmpeg (支持GPU加速)
- Go 1.19+ 运行环境

**API服务器要求**
- Go 1.19+ 运行环境
- 网络访问AWS服务

### 1. AWS资源配置 (在AWS Console执行)

#### 1.1 创建S3存储桶
```bash
# 在AWS Console或使用AWS CLI
aws s3 mb s3://your-input-bucket --region us-west-2
aws s3 mb s3://your-output-bucket --region us-west-2
```

#### 1.2 创建SQS队列
```bash
# 在AWS Console或使用AWS CLI
aws sqs create-queue --queue-name video-transcode-queue --region us-west-2
```

#### 1.3 配置S3事件通知 (自动触发转码)

当视频文件上传到 S3 输入桶时，系统可以自动触发转码任务。

**步骤1: 配置SQS队列策略**

首先需要允许 S3 向 SQS 发送消息。获取队列 ARN 后，添加以下策略：

```bash
# 获取队列ARN
aws sqs get-queue-attributes \
  --queue-url https://sqs.us-west-2.amazonaws.com/123456789/video-transcode-queue \
  --attribute-names QueueArn
```

在 SQS 控制台编辑队列的访问策略，添加：
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Service": "s3.amazonaws.com"
      },
      "Action": "sqs:SendMessage",
      "Resource": "arn:aws:sqs:us-west-2:123456789:video-transcode-queue",
      "Condition": {
        "ArnLike": {
          "aws:SourceArn": "arn:aws:s3:::your-input-bucket"
        }
      }
    }
  ]
}
```

**步骤2: 配置S3事件通知**

```bash
# 创建事件通知配置文件 s3-notification.json
cat > s3-notification.json << 'EOF'
{
  "QueueConfigurations": [
    {
      "QueueArn": "arn:aws:sqs:ap-southeast-1:286345677825:video-transcode",
      "Events": ["s3:ObjectCreated:*"]
    }
  ]
}
EOF

# 应用配置到S3桶
aws s3api put-bucket-notification-configuration \
  --bucket your-input-bucket \
  --notification-configuration file://s3-notification.json
```

> **注意**: S3 Filter 只支持单个 suffix，如需支持多种视频格式，可以：
> 1. 不设置 Filter，让所有文件触发事件（GPU处理器会自动过滤非视频文件）
> 2. 或创建多个 QueueConfigurations，每个配置一个 suffix

**简化配置（推荐）- 不过滤文件类型：**
```json
{
  "QueueConfigurations": [
    {
      "QueueArn": "arn:aws:sqs:us-west-2:123456789:video-transcode-queue",
      "Events": ["s3:ObjectCreated:*"]
    }
  ]
}
```

GPU处理器会自动识别视频文件（.mp4, .mov, .avi, .mkv, .wmv, .flv, .webm, .m4v, .mpeg, .mpg），非视频文件会被跳过。

**验证配置：**
```bash
# 查看当前S3事件通知配置
aws s3api get-bucket-notification-configuration --bucket your-input-bucket

# 测试：上传视频文件
aws s3 cp test-video.mp4 s3://your-input-bucket/

# 检查SQS是否收到消息
aws sqs get-queue-attributes \
  --queue-url your-queue-url \
  --attribute-names ApproximateNumberOfMessages
```

#### 1.4 创建DynamoDB表
```bash
# 在AWS Console或使用AWS CLI
# 创建表并配置 GSI（全局二级索引）用于高效查询
aws dynamodb create-table \
  --table-name video-transcode-tasks \
  --attribute-definitions \
    AttributeName=task_id,AttributeType=S \
    AttributeName=date_partition,AttributeType=S \
    AttributeName=status,AttributeType=S \
    AttributeName=created_at,AttributeType=S \
  --key-schema AttributeName=task_id,KeyType=HASH \
  --global-secondary-indexes \
    '[
      {
        "IndexName": "date-index",
        "KeySchema": [
          {"AttributeName": "date_partition", "KeyType": "HASH"},
          {"AttributeName": "created_at", "KeyType": "RANGE"}
        ],
        "Projection": {"ProjectionType": "ALL"}
      },
      {
        "IndexName": "status-index",
        "KeySchema": [
          {"AttributeName": "status", "KeyType": "HASH"},
          {"AttributeName": "created_at", "KeyType": "RANGE"}
        ],
        "Projection": {"ProjectionType": "ALL"}
      }
    ]' \
  --billing-mode PAY_PER_REQUEST \
  --region us-west-2
```

**GSI 索引说明:**
- `date-index`: 按日期分区查询任务，适合查看某天的所有任务
- `status-index`: 按状态查询任务，适合查看所有 pending/failed 等状态的任务

> **已有表升级**: 如果你已经创建了旧版本的表，请参考 [docs/dynamodb_migration.md](docs/dynamodb_migration.md) 进行升级。

#### 1.4 配置IAM权限

**方式1: EC2 IAM角色 (推荐，最安全)**

**步骤1: 创建IAM角色**
1. 登录AWS Console，进入IAM服务
2. 点击"角色" -> "创建角色"
3. 选择可信实体类型: "AWS服务"
4. 选择使用案例: "EC2"
5. 点击"下一步"

**步骤2: 添加权限策略**
选择以下AWS托管策略：
- `AmazonS3FullAccess` (S3存储桶访问)
- `AmazonSQSFullAccess` (SQS队列访问)
- `AmazonDynamoDBFullAccess` (DynamoDB表访问)

或创建自定义策略 (最小权限原则):
```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "s3:GetObject",
                "s3:PutObject",
                "s3:DeleteObject"
            ],
            "Resource": [
                "arn:aws:s3:::your-input-bucket/*",
                "arn:aws:s3:::your-output-bucket/*"
            ]
        },
        {
            "Effect": "Allow",
            "Action": [
                "sqs:SendMessage",
                "sqs:ReceiveMessage",
                "sqs:DeleteMessage",
                "sqs:GetQueueAttributes"
            ],
            "Resource": "arn:aws:sqs:ap-southeast-1:286345677825:video-transcode"
        },
        {
            "Effect": "Allow",
            "Action": [
                "dynamodb:GetItem",
                "dynamodb:PutItem",
                "dynamodb:UpdateItem",
                "dynamodb:Query",
                "dynamodb:Scan"
            ],
            "Resource": "arn:aws:dynamodb:ap-southeast-1:286345677825:table/video-transcode"
        }
    ]
}
```

**步骤3: 完成角色创建**
1. 角色名称: `video-transcode-role`
2. 描述: `GPU视频转码系统IAM角色`
3. 点击"创建角色"

**步骤4: 将角色附加到EC2实例**
1. 进入EC2控制台
2. 选择你的EC2实例
3. 点击"操作" -> "安全" -> "修改IAM角色"
4. 选择刚创建的 `video-transcode-role`
5. 点击"更新IAM角色"

**步骤5: 验证角色配置**
```bash
# 在EC2实例上验证角色是否生效
curl http://169.254.169.254/latest/meta-data/iam/security-credentials/
# 应该返回角色名称: video-transcode-role

# 测试AWS服务访问
aws sts get-caller-identity
# 应该显示角色信息而不是用户信息
```

**方式2: AWS凭证**
如果不使用IAM角色，需要配置AWS凭证：
```bash
# 方法1: 在config.env中配置
AWS_ACCESS_KEY_ID=your-access-key-id
AWS_SECRET_ACCESS_KEY=your-secret-access-key

# 方法2: 使用AWS CLI配置
aws configure
```

**所需权限:**
- S3: GetObject, PutObject, DeleteObject
- SQS: SendMessage, ReceiveMessage, DeleteMessage, GetQueueAttributes
- DynamoDB: GetItem, PutItem, UpdateItem, Query

### 2. GPU服务器配置 (在GPU机器上执行)

#### 2.1 安装NVIDIA驱动和CUDA

**步骤1: 检查GPU硬件**
```bash
# 检查是否有NVIDIA GPU
lspci | grep -i nvidia

# 查看系统信息
sudo lshw -c display
```

**步骤2: 安装NVIDIA驱动**
```bash
# 更新系统
sudo apt update && sudo apt upgrade -y

# 方式1: 自动检测安装 (推荐，适用于所有环境)
sudo ubuntu-drivers autoinstall

# 方式2: 手动选择版本
# 注意: 在AWS EC2上使用 nvidia-driver-* 而不是 nvidia-utils-*

# 对于AWS EC2 GPU实例:
sudo apt install nvidia-driver-580-server

# 对于普通服务器:
sudo apt install nvidia-utils-580-server

# 对于桌面环境:
sudo apt install nvidia-utils-580
```

**驱动版本选择指南:**
- **580系列**: 最新版本，支持RTX 40/30系列等新GPU
- **570系列**: 较新版本，兼容性好
- **550系列**: 长期支持版本，稳定性高，推荐服务器使用
- **535系列**: 较老但稳定的版本
- **525/470系列**: 适合较老的GPU (GTX 10系列等)

**如何选择:**
```bash
# 查看GPU型号后选择
lspci | grep -i nvidia

# RTX 40/30系列 → 580系列
# RTX 20/GTX 16系列 → 570或550系列  
# GTX 10系列及更老 → 525或470系列
```

**步骤3: 安装CUDA工具包**
```bash
# 安装CUDA工具包 (FFmpeg GPU加速需要)
sudo apt install nvidia-cuda-toolkit

# 重启系统使驱动生效 (必须重启!)
sudo reboot
```

> **重要**: 安装NVIDIA驱动后必须重启系统，否则会出现 "couldn't communicate with the NVIDIA driver" 错误。

#### 2.2 验证GPU环境
```bash
# 检查NVIDIA驱动 (重启后执行)
nvidia-smi

# 检查CUDA
nvcc --version

# 检查FFmpeg GPU支持
ffmpeg -hwaccels
```

**预期输出示例:**
```bash
# nvidia-smi 应该显示GPU信息和驱动版本
# nvcc --version 应该显示CUDA编译器版本
# ffmpeg -hwaccels 应该包含 cuda, nvenc, nvdec 等
```

#### 2.3 获取代码和编译
```bash
# 方式1: 使用Git克隆 (如果有Git环境)
git clone <repository-url>
cd gpu-video-transcode

# 方式2: 手工复制代码 (推荐用于生产环境)
# 将整个项目文件夹复制到GPU服务器
# 注意: 将下面的路径替换为你实际的项目路径和目标路径

# 如果你当前在 video_transcode 项目目录内:
scp -r . user@gpu-server:/opt/video_transcode/

# 如果你在 video_transcode 项目目录外:
scp -r ./video_transcode user@gpu-server:/opt/

# 完整路径示例:
# scp -r /home/yourname/video_transcode user@gpu-server:/opt/

# 登录到GPU服务器后，进入项目目录
cd /opt/video_transcode  # 或你复制到的目录

# 编译GPU处理器
make build

# 配置环境变量
cp config.example.env config.env
```

#### 2.4 编辑GPU服务器配置
编辑 `config.env` (GPU服务器专用配置):
```bash
# AWS配置
AWS_REGION=us-west-2
INPUT_BUCKET=your-input-bucket
OUTPUT_BUCKET=your-output-bucket
SQS_QUEUE_URL=https://sqs.us-west-2.amazonaws.com/123456789/video-transcode-queue
DYNAMODB_TABLE=video-transcode-tasks

# GPU处理器配置 (仅GPU服务器需要)
TEMP_DIR=/tmp/ffmpeg_processing
MAX_CONCURRENT_TASKS=2  # 根据GPU性能调整
POLL_INTERVAL=10s

# AWS凭证 (如果不使用IAM角色)
AWS_ACCESS_KEY_ID=your-access-key
AWS_SECRET_ACCESS_KEY=your-secret-key
```

#### 2.5 启动GPU处理器
```bash
# 在GPU服务器上运行 (后台运行)
make start-gpu

# 查看运行状态
make status

# 查看日志
tail -f gpu-processor.log

# 或者编译后运行
make build
nohup ./bin/gpu-processor > gpu-processor.log 2>&1 &
```

### 3. API服务器配置 (在API服务器上执行)

#### 3.1 获取代码和编译
```bash
# 方式1: 使用Git克隆 (如果有Git环境)
git clone <repository-url>
cd gpu-video-transcode

# 方式2: 手工复制代码 (推荐用于生产环境)
# 将整个项目文件夹复制到API服务器
# 注意: 将下面的路径替换为你实际的项目路径和目标路径

# 如果你当前在 video_transcode 项目目录内:
scp -r . user@api-server:/opt/video_transcode/

# 如果你在 video_transcode 项目目录外:
scp -r ./video_transcode user@api-server:/opt/

# 完整路径示例:
# scp -r /home/yourname/video_transcode user@api-server:/opt/

# 登录到API服务器后，进入项目目录
cd /opt/video_transcode  # 或你复制到的目录

# 编译API服务器
make build

# 配置环境变量
cp config.example.env config.env
```

#### 3.2 编辑API服务器配置
编辑 `config.env` (API服务器专用配置):
```bash
# AWS配置
AWS_REGION=us-west-2
INPUT_BUCKET=your-input-bucket
OUTPUT_BUCKET=your-output-bucket
SQS_QUEUE_URL=https://sqs.us-west-2.amazonaws.com/123456789/video-transcode-queue
DYNAMODB_TABLE=video-transcode-tasks

# API服务器配置 (必须使用9999端口)
API_PORT=9999
API_HOST=0.0.0.0

# AWS凭证 (如果不使用IAM角色)
AWS_ACCESS_KEY_ID=your-access-key
AWS_SECRET_ACCESS_KEY=your-secret-key

# GPU处理器配置项在API服务器上不需要
# TEMP_DIR, MAX_CONCURRENT_TASKS, POLL_INTERVAL 可以忽略
```

#### 3.3 启动API服务器
```bash
# 在API服务器上运行 (后台运行)
make start-api

# 查看运行状态
make status

# 查看日志
tail -f api-server.log

# 或者编译后运行
make build
nohup ./bin/api-server > api-server.log 2>&1 &
```

### 4. 验证部署

#### 4.1 验证AWS凭证配置
```bash
# 测试AWS凭证是否有效
aws sts get-caller-identity

# 测试各项AWS服务权限
aws s3 ls s3://your-input-bucket/
aws sqs get-queue-attributes --queue-url your-queue-url
aws dynamodb describe-table --table-name your-table-name
```

#### 4.2 在API服务器上测试
```bash
# 健康检查
curl http://localhost:9999/api/health

# 队列状态 (需要AWS凭证正确配置)
curl http://localhost:9999/api/queue/status
```

#### 4.2 提交测试任务
```bash
# 首先上传测试视频到S3输入桶
aws s3 cp test-video.mp4 s3://your-input-bucket/

# 提交转码任务
curl -X POST http://localhost:9999/api/queue/add \
  -H "Content-Type: application/json" \
  -d '{
    "input_bucket": "your-input-bucket",
    "input_key": "test-video.mp4", 
    "transcode_types": ["mp4_standard"]
  }'
```

#### 4.3 监控处理过程
```bash
# 查看GPU处理器日志
tail -f gpu-processor.log

# 查看API服务器日志  
tail -f api-server.log

# 检查SQS队列
aws sqs get-queue-attributes --queue-url your-sqs-queue-url --attribute-names All
```
#
# 故障排除

### 常见问题

#### 进程管理问题

**问题**: 如何停止后台运行的服务
```bash
# 停止所有服务
make stop-all

# 分别停止
make stop-api    # 停止API服务器
make stop-gpu    # 停止GPU处理器

# 手动停止 (如果make命令不可用)
pkill -9 -f "exe/api-server"
pkill -9 -f "exe/gpu-processor"

# 查看运行状态
make status
```

**问题**: API健康检查无响应
```bash
# curl http://localhost:9999/api/health 没有回显

# 步骤1: 检查服务是否运行
make status
ps aux | grep "go run ./cmd/api-server"

# 步骤2: 检查端口监听
lsof -i :9999
netstat -tlnp | grep 9999

# 步骤3: 查看日志
tail -20 api-server.log
tail -f api-server.log

# 步骤4: 如果服务未运行，启动它
make start-api
# 或者
make start-all

# 步骤5: 测试端口连通性
nc -zv localhost 9999
telnet localhost 9999
```

**问题**: 端口被占用或服务无法启动
```bash
# 检查端口占用
lsof -i :9999

# 强制停止占用端口的进程
sudo kill -9 $(lsof -t -i:9999)

# 检查是否有僵尸进程
ps aux | grep "go run"

# 检查配置文件
grep API_PORT config.env
```

**问题**: 服务意外停止
```bash
# 查看日志找出原因
tail -50 api-server.log
tail -50 gpu-processor.log

# 重新启动服务
make start-all
```

#### GPU处理器问题

**问题**: nvidia-smi 命令未找到
```bash
# 如果提示 Command 'nvidia-smi' not found
# 安装NVIDIA驱动
sudo apt install nvidia-utils-580        # 现代GPU
sudo apt install nvidia-utils-580-server # 服务器环境
sudo apt install nvidia-utils-550-server # 稳定版本

# 或者自动安装
sudo ubuntu-drivers autoinstall

# 重启系统
sudo reboot
```

**问题**: nvidia-smi 无法与驱动通信
```bash
# 错误: "NVIDIA-SMI has failed because it couldn't communicate with the NVIDIA driver"

# 步骤1: 重启系统 (最重要)
sudo reboot

# 步骤2: 检查驱动状态
lsmod | grep nvidia
dpkg -l | grep nvidia

# 步骤3: 手动加载驱动模块
sudo modprobe nvidia
sudo modprobe nvidia_uvm
sudo modprobe nvidia_drm

# 步骤4: 如果仍然失败，重新安装驱动
sudo apt purge nvidia-* libnvidia-*
sudo apt autoremove
sudo apt update
sudo apt install nvidia-utils-580-server
sudo reboot

# 步骤5: 检查安全启动状态
mokutil --sb-state
# 如果启用了Secure Boot，在BIOS中禁用它
```

**问题**: AWS EC2上NVIDIA模块未找到
```bash
# 错误: "Module nvidia not found in directory /lib/modules/..."
# 这通常发生在AWS EC2实例上

# 步骤1: 确认是GPU实例
lspci | grep -i nvidia
# 如果没有输出，需要使用p3, p4, g4, g5等GPU实例类型

# 步骤2: 安装完整驱动包 (不是utils包)
sudo apt remove nvidia-utils-*
sudo apt install nvidia-driver-580-server
sudo reboot

# 步骤3: 如果仍然失败，使用自动安装
sudo apt purge nvidia-* libnvidia-*
sudo apt autoremove
sudo ubuntu-drivers autoinstall
sudo reboot

# 步骤4: Tesla T4专用 (如果是T4 GPU)
sudo apt install nvidia-driver-470-server
sudo reboot

# 步骤5: 验证安装
nvidia-smi
```

**AWS EC2 GPU实例类型对应:**
- Tesla T4: g4dn.* 实例 (推荐用于视频转码)
- Tesla V100: p3.* 实例
- Tesla A100: p4d.* 实例
- Tesla K80: p2.* 实例

**问题**: GPU处理器无法启动
```bash
# 检查GPU驱动
nvidia-smi

# 检查CUDA
nvcc --version

# 检查FFmpeg GPU支持
ffmpeg -hwaccels

# 如果FFmpeg不支持GPU，重新安装
sudo apt install ffmpeg
```

**问题**: CUDA相关错误
```bash
# 安装CUDA工具包
sudo apt install nvidia-cuda-toolkit

# 检查CUDA路径
echo $CUDA_HOME
export CUDA_HOME=/usr/local/cuda
export PATH=$PATH:$CUDA_HOME/bin

# 重新编译项目
make clean && make build
```

**问题**: 转码失败或性能差
```bash
# 检查GPU使用率
nvidia-smi -l 1
# 调整并发任务数
# 在config.env中修改 MAX_CONCURRENT_TASKS=1
```

**问题**: 临时文件空间不足
```bash
# 检查磁盘空间
df -h /tmp
# 修改临时目录
# 在config.env中设置 TEMP_DIR=/path/to/large/disk
```

#### API服务器问题

**问题**: 端口9999被占用
```bash
# 检查端口占用
lsof -i :9999
# 杀死占用进程或修改端口配置
```

**问题**: AWS连接失败
```bash
# 检查AWS凭证
aws sts get-caller-identity
# 检查网络连接
curl -I https://s3.amazonaws.com
```

#### AWS服务问题

**问题**: AWS凭证错误
```bash
# 错误: "no EC2 IMDS role found" 或 "failed to refresh cached credentials"

# 解决方案1: 配置AWS凭证
# 编辑config.env，取消注释并填写真实凭证:
AWS_ACCESS_KEY_ID=your-access-key-id
AWS_SECRET_ACCESS_KEY=your-secret-access-key

# 解决方案2: 使用AWS CLI配置
aws configure

# 解决方案3: 配置EC2 IAM角色 (推荐)
# 在AWS Console为EC2实例配置IAM角色，包含以下权限:
# - AmazonS3FullAccess
# - AmazonSQSFullAccess  
# - AmazonDynamoDBFullAccess

# 验证凭证是否有效
aws sts get-caller-identity
```

**问题**: SQS权限错误
```bash
# 检查SQS权限
aws sqs get-queue-attributes --queue-url your-queue-url

# 确认IAM权限包含:
# - sqs:SendMessage
# - sqs:ReceiveMessage
# - sqs:DeleteMessage
# - sqs:GetQueueAttributes
```

**问题**: S3访问被拒绝
```bash
# 测试S3访问
aws s3 ls s3://your-input-bucket/
aws s3 ls s3://your-output-bucket/

# 确认IAM权限包含:
# - s3:GetObject
# - s3:PutObject
# - s3:DeleteObject
```

**问题**: DynamoDB表不存在
```bash
# 检查表是否存在
aws dynamodb describe-table --table-name your-table-name

# 确认表名和区域配置正确
grep DYNAMODB_TABLE config.env
grep AWS_REGION config.env
```

### 日志分析

#### 启用详细日志
```bash
# 设置日志级别
export LOG_LEVEL=debug

# 查看实时日志
tail -f api-server.log
tail -f gpu-processor.log
```

#### 关键日志信息
- `Task received`: 任务接收成功
- `Transcode started`: 转码开始
- `Transcode completed`: 转码完成
- `Upload completed`: 上传完成
- `Task failed`: 任务失败

### 性能优化

#### GPU服务器优化
```bash
# 根据GPU性能调整并发数
MAX_CONCURRENT_TASKS=4  # 高端GPU
MAX_CONCURRENT_TASKS=2  # 中端GPU
MAX_CONCURRENT_TASKS=1  # 低端GPU

# 调整轮询间隔
POLL_INTERVAL=5s   # 高负载时缩短间隔
POLL_INTERVAL=30s  # 低负载时延长间隔
```

#### 系统资源监控
```bash
# GPU使用率
nvidia-smi -l 1

# CPU和内存
htop

# 磁盘I/O
iotop

# 网络带宽
iftop
```

## 开发指南

### 项目结构
```
├── cmd/                    # 可执行程序入口
│   ├── api-server/        # API服务器
│   └── gpu-processor/     # GPU处理器
├── internal/              # 内部包
│   ├── api/              # API处理逻辑
│   │   ├── handlers.go   # API处理器
│   │   ├── router.go     # 路由配置
│   │   ├── static.go     # 静态文件服务
│   │   └── web/          # Web管理界面
│   │       ├── index.html
│   │       ├── style.css
│   │       └── app.js
│   ├── config/           # 配置管理
│   ├── queue/            # 队列管理
│   ├── task/             # 任务管理
│   └── transcode/        # 转码逻辑
├── docs/                 # 文档
├── config.env            # 环境配置
└── Makefile             # 构建脚本
```

### 添加新的转码格式

1. 在 `internal/transcode/processor.go` 中添加新的转码配置
2. 更新 `internal/task/models.go` 中的转码类型定义
3. 测试新格式的转码效果

### 贡献代码

1. Fork项目
2. 创建功能分支
3. 提交代码变更
4. 创建Pull Request

## 许可证

[添加许可证信息]