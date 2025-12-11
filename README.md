# GPU视频转码系统

基于GPU加速的FFmpeg视频转码处理器，支持队列管理、任务监测和AI智能参数生成。

## 主要特性

- 🚀 GPU硬件加速（NVIDIA NVENC / Apple VideoToolbox）
- 🤖 AI智能转码参数生成（AWS Bedrock Claude）
- 🌐 Web图形化管理界面
- 📊 任务队列管理和状态监控
- ☁️ AWS云服务集成（S3/SQS/DynamoDB）

## 快速开始

```bash
# 编译
make build

# 配置
cp config.example.env config.env
# 编辑 config.env 填写AWS资源信息

# 启动所有服务
make start-all

# 访问Web管理界面
open http://localhost:9999/admin
```

## 文档目录

| 文档 | 说明 |
|-----|------|
| [功能介绍](docs/features.md) | 系统功能、特性、支持的转码格式、系统要求 |
| [架构设计](docs/architecture.md) | 系统架构图、组件说明、项目结构、数据流 |
| [部署指南](docs/deployment.md) | AWS资源配置、GPU服务器配置、API服务器配置、故障排除 |
| [API接口](docs/api.md) | REST API接口说明、请求示例、响应格式 |
| [DynamoDB迁移](docs/dynamodb_migration.md) | 数据库表结构升级指南 |

## 常用命令

```bash
make build       # 编译项目
make start-all   # 启动所有服务
make stop-all    # 停止所有服务
make status      # 查看服务状态
make logs        # 查看日志
```

详细命令请参考 [COMMANDS.md](COMMANDS.md)

## 许可证

[添加许可证信息]
