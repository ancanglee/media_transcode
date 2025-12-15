# 智能媒体转码系统 - 部署指南

## 快速开始（开发环境）

1. **编译**
```bash
make build
```

2. **配置**
```bash
cp config.example.env config.env
# 编辑config.env填写AWS资源信息
```

3. **运行**
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
```
http://localhost:9999/admin
```

---

## 生产环境部署

### 1. AWS资源配置

#### 1.1 创建S3存储桶
```bash
aws s3 mb s3://your-input-bucket --region us-west-2
aws s3 mb s3://your-output-bucket --region us-west-2
```

#### 1.2 创建SQS队列
```bash
aws sqs create-queue --queue-name video-transcode-queue --region us-west-2
```

#### 1.3 配置S3事件通知（自动触发转码）

**步骤1: 配置SQS队列策略**
```bash
# 获取队列ARN
aws sqs get-queue-attributes \
  --queue-url https://sqs.us-west-2.amazonaws.com/123456789/video-transcode-queue \
  --attribute-names QueueArn
```

在 SQS 控制台编辑队列的访问策略：
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
cat > s3-notification.json << 'EOF'
{
  "QueueConfigurations": [
    {
      "QueueArn": "arn:aws:sqs:us-west-2:123456789:video-transcode-queue",
      "Events": ["s3:ObjectCreated:*"]
    }
  ]
}
EOF

aws s3api put-bucket-notification-configuration \
  --bucket your-input-bucket \
  --notification-configuration file://s3-notification.json
```

> GPU处理器会自动识别视频文件（.mp4, .mov, .avi, .mkv, .wmv, .flv, .webm, .m4v, .mpeg, .mpg），非视频文件会被跳过。

#### 1.4 创建DynamoDB表
```bash
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

> 已有表升级请参考 [dynamodb_migration.md](dynamodb_migration.md)

#### 1.5 配置IAM权限

**方式1: EC2 IAM角色（推荐）**

1. 创建IAM角色，选择 EC2 作为可信实体
2. 添加权限策略：
   - `AmazonS3FullAccess`
   - `AmazonSQSFullAccess`
   - `AmazonDynamoDBFullAccess`
3. 将角色附加到EC2实例

**方式2: AWS凭证**
```bash
# 在config.env中配置
AWS_ACCESS_KEY_ID=your-access-key-id
AWS_SECRET_ACCESS_KEY=your-secret-access-key
```

---

### 2. GPU服务器配置

#### 2.1 安装NVIDIA驱动和CUDA

```bash
# 检查GPU硬件
lspci | grep -i nvidia

# 安装驱动
sudo apt update && sudo apt upgrade -y
sudo ubuntu-drivers autoinstall

# 安装CUDA工具包
sudo apt install nvidia-cuda-toolkit

# 重启系统
sudo reboot
```

#### 2.2 验证GPU环境
```bash
nvidia-smi
nvcc --version
ffmpeg -hwaccels
```

#### 2.3 部署GPU处理器
```bash
# 复制代码到服务器
scp -r . user@gpu-server:/opt/video_transcode/

# 登录服务器
ssh user@gpu-server
cd /opt/video_transcode

# 编译
make build

# 配置
cp config.example.env config.env
# 编辑 config.env
```

GPU服务器 `config.env` 配置：
```bash
AWS_REGION=us-west-2
INPUT_BUCKET=your-input-bucket
OUTPUT_BUCKET=your-output-bucket
SQS_QUEUE_URL=https://sqs.us-west-2.amazonaws.com/123456789/video-transcode-queue
DYNAMODB_TABLE=video-transcode-tasks
TEMP_DIR=/tmp/ffmpeg_processing
MAX_CONCURRENT_TASKS=2
POLL_INTERVAL=10s
```

#### 2.4 启动GPU处理器
```bash
make start-gpu
```

---

### 3. API服务器配置

#### 3.1 部署API服务器
```bash
# 复制代码到服务器
scp -r . user@api-server:/opt/video_transcode/

# 登录服务器
ssh user@api-server
cd /opt/video_transcode

# 编译
make build

# 配置
cp config.example.env config.env
```

API服务器 `config.env` 配置：
```bash
AWS_REGION=us-west-2
INPUT_BUCKET=your-input-bucket
OUTPUT_BUCKET=your-output-bucket
SQS_QUEUE_URL=https://sqs.us-west-2.amazonaws.com/123456789/video-transcode-queue
DYNAMODB_TABLE=video-transcode-tasks
API_PORT=9999
API_HOST=0.0.0.0
```

#### 3.2 启动API服务器
```bash
make start-api
```

---

### 4. 验证部署

```bash
# 健康检查
curl http://localhost:9999/api/health

# 队列状态
curl http://localhost:9999/api/queue/status

# 提交测试任务
aws s3 cp test-video.mp4 s3://your-input-bucket/

curl -X POST http://localhost:9999/api/queue/add \
  -H "Content-Type: application/json" \
  -d '{
    "input_bucket": "your-input-bucket",
    "input_key": "test-video.mp4", 
    "transcode_types": ["mp4_standard"]
  }'
```

---

## 故障排除

### 进程管理问题

**服务无响应**
```bash
make status
ps aux | grep "go run"
lsof -i :9999
tail -20 api-server.log
```

**端口被占用**
```bash
lsof -i :9999
sudo kill -9 $(lsof -t -i:9999)
```

### GPU处理器问题

**nvidia-smi 命令未找到**
```bash
sudo apt install nvidia-driver-580-server
sudo reboot
```

**nvidia-smi 无法与驱动通信**
```bash
sudo reboot
# 如果仍然失败
sudo apt purge nvidia-* libnvidia-*
sudo apt autoremove
sudo ubuntu-drivers autoinstall
sudo reboot
```

### AWS服务问题

**AWS凭证错误**
```bash
# 验证凭证
aws sts get-caller-identity

# 测试各项服务
aws s3 ls s3://your-input-bucket/
aws sqs get-queue-attributes --queue-url your-queue-url
aws dynamodb describe-table --table-name your-table-name
```

### 性能优化

```bash
# 根据GPU性能调整并发数
MAX_CONCURRENT_TASKS=4  # 高端GPU
MAX_CONCURRENT_TASKS=2  # 中端GPU
MAX_CONCURRENT_TASKS=1  # 低端GPU

# 调整轮询间隔
POLL_INTERVAL=5s   # 高负载
POLL_INTERVAL=30s  # 低负载
```

### 系统资源监控
```bash
nvidia-smi -l 1  # GPU使用率
htop             # CPU和内存
iotop            # 磁盘I/O
iftop            # 网络带宽
```
