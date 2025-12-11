# GPU视频转码系统 - API接口说明

## 基础信息

- 基础URL: `http://your-server:9999`
- 内容类型: `application/json`

---

## 健康检查

### GET /api/health

检查API服务器健康状态。

**响应示例:**
```json
{
  "status": "ok",
  "timestamp": "2025-01-15T10:30:00Z"
}
```

---

## 队列管理

### GET /api/queue/status

获取SQS队列状态。

**响应示例:**
```json
{
  "available_messages": 5,
  "in_flight_messages": 2,
  "queue_url": "https://sqs.us-west-2.amazonaws.com/123456789/video-transcode-queue"
}
```

### POST /api/queue/add

添加转码任务到队列。

**请求参数:**
| 参数 | 类型 | 必填 | 说明 |
|-----|------|-----|------|
| input_bucket | string | 是 | S3输入桶名称 |
| input_key | string | 是 | S3文件路径 |
| transcode_types | array | 是 | 转码类型列表 |

**请求示例:**
```bash
curl -X POST http://localhost:9999/api/queue/add \
  -H "Content-Type: application/json" \
  -d '{
    "input_bucket": "my-input-bucket",
    "input_key": "videos/sample.mp4", 
    "transcode_types": ["mp4_standard", "thumbnail"]
  }'
```

**响应示例:**
```json
{
  "success": true,
  "message": "Task added to queue",
  "task_id": "abc123-def456"
}
```

### POST /api/queue/purge

清空队列中的所有消息。

**请求示例:**
```bash
curl -X POST http://localhost:9999/api/queue/purge
```

---

## 任务管理

### GET /api/tasks

查询任务列表，支持按日期和状态过滤。

**查询参数:**
| 参数 | 类型 | 必填 | 说明 |
|-----|------|-----|------|
| date | string | 否 | 日期过滤 (YYYY-MM-DD) |
| status | string | 否 | 状态过滤 (pending/processing/completed/failed) |
| limit | int | 否 | 返回数量限制，默认20 |

**请求示例:**
```bash
# 查询所有任务
curl "http://localhost:9999/api/tasks"

# 按日期查询
curl "http://localhost:9999/api/tasks?date=2025-01-15"

# 按状态查询
curl "http://localhost:9999/api/tasks?status=completed"

# 组合查询
curl "http://localhost:9999/api/tasks?date=2025-01-15&status=completed&limit=50"
```

**响应示例:**
```json
{
  "tasks": [
    {
      "task_id": "abc123",
      "input_key": "videos/sample.mp4",
      "status": "completed",
      "transcode_type": "mp4_standard",
      "created_at": "2025-01-15T10:00:00Z",
      "completed_at": "2025-01-15T10:05:00Z"
    }
  ],
  "count": 1
}
```

### GET /api/tasks/:id

获取单个任务详情。

**请求示例:**
```bash
curl "http://localhost:9999/api/tasks/abc123"
```

### POST /api/tasks/:id/retry

重试失败的任务。

**请求示例:**
```bash
curl -X POST "http://localhost:9999/api/tasks/abc123/retry"
```

### POST /api/tasks/:id/cancel

取消等待中的任务。

**请求示例:**
```bash
curl -X POST "http://localhost:9999/api/tasks/abc123/cancel"
```

---

## AI智能转码

### POST /api/llm/generate

使用AI生成FFmpeg转码参数。

**请求参数:**
| 参数 | 类型 | 必填 | 说明 |
|-----|------|-----|------|
| user_requirement | string | 是 | 用户需求描述（自然语言） |
| input_format | string | 否 | 输入文件格式 |
| platform | string | 否 | 目标平台 (linux_nvidia/macos_apple) |

**请求示例:**
```bash
curl -X POST http://localhost:9999/api/llm/generate \
  -H "Content-Type: application/json" \
  -d '{
    "user_requirement": "把视频转成720p分辨率，保持较高画质",
    "input_format": "mp4",
    "platform": "linux_nvidia"
  }'
```

**响应示例:**
```json
{
  "name": "720p_high_quality",
  "description": "720p高画质转码",
  "ffmpeg_args": ["-y", "-vf", "scale=1280:720", "-c:v", "hevc_nvenc", "-preset", "slow", "-crf", "18"],
  "output_ext": "mp4",
  "explanation": "使用NVIDIA GPU加速，CRF 18保证高画质",
  "estimated_speed": "3x"
}
```

### POST /api/llm/test

测试AI生成的转码参数。

**请求参数:**
| 参数 | 类型 | 必填 | 说明 |
|-----|------|-----|------|
| input_file | string | 是 | 本地测试文件路径 |
| ffmpeg_args | array | 是 | FFmpeg参数列表 |
| output_ext | string | 是 | 输出文件扩展名 |

**请求示例:**
```bash
curl -X POST http://localhost:9999/api/llm/test \
  -H "Content-Type: application/json" \
  -d '{
    "input_file": "/tmp/test.mp4",
    "ffmpeg_args": ["-y", "-vf", "scale=1280:720", "-c:v", "hevc_nvenc"],
    "output_ext": "mp4"
  }'
```

### POST /api/llm/save-preset

保存AI生成的参数为预设。

**请求参数:**
| 参数 | 类型 | 必填 | 说明 |
|-----|------|-----|------|
| name | string | 是 | 预设名称 |
| description | string | 是 | 预设描述 |
| ffmpeg_args | array | 是 | FFmpeg参数列表 |
| output_ext | string | 是 | 输出文件扩展名 |

**请求示例:**
```bash
curl -X POST http://localhost:9999/api/llm/save-preset \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my_720p_preset",
    "description": "自定义720p高画质预设",
    "ffmpeg_args": ["-y", "-vf", "scale=1280:720", "-c:v", "hevc_nvenc"],
    "output_ext": "mp4"
  }'
```

### GET /api/llm/presets

获取所有预设列表（包括内置和自定义）。

**请求示例:**
```bash
curl "http://localhost:9999/api/llm/presets"
```

---

## 转码类型

可用的内置转码类型：

| 类型 | 说明 |
|-----|------|
| `mp4_standard` | 标清MP4 (848x480) |
| `mp4_smooth` | 流畅MP4 (640x360) |
| `hdlbr_h265` | 高质量H265 |
| `lcd_h265` | LCD优化H265 |
| `h265_mute` | 静音H265 |
| `custom_mute_preview` | 静音预览 |
| `thumbnail` | 缩略图JPG |

---

## 错误响应

所有API在发生错误时返回统一格式：

```json
{
  "error": "错误描述信息",
  "code": "ERROR_CODE"
}
```

常见HTTP状态码：
- `200` - 成功
- `400` - 请求参数错误
- `404` - 资源不存在
- `500` - 服务器内部错误
