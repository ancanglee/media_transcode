package task

import (
	"time"
)

// TaskStatus 任务状态枚举
type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"    // 等待处理
	TaskStatusProcessing TaskStatus = "processing" // 处理中
	TaskStatusCompleted  TaskStatus = "completed"  // 已完成
	TaskStatusFailed     TaskStatus = "failed"     // 失败
	TaskStatusRetrying   TaskStatus = "retrying"   // 重试中
	TaskStatusCancelled  TaskStatus = "cancelled"  // 已取消
)

// TranscodeTask 转码任务结构
type TranscodeTask struct {
	TaskID         string            `json:"task_id" dynamodbav:"task_id"`
	DatePartition  string            `json:"date_partition" dynamodbav:"date_partition"` // 日期分区键，格式: 2025-01-15
	InputBucket    string            `json:"input_bucket" dynamodbav:"input_bucket"`
	InputKey       string            `json:"input_key" dynamodbav:"input_key"`
	OutputBucket   string            `json:"output_bucket" dynamodbav:"output_bucket"`
	TranscodeTypes []string          `json:"transcode_types" dynamodbav:"transcode_types"`
	Status         TaskStatus        `json:"status" dynamodbav:"status"`
	CreatedAt      time.Time         `json:"created_at" dynamodbav:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at" dynamodbav:"updated_at"`
	StartedAt      *time.Time        `json:"started_at,omitempty" dynamodbav:"started_at,omitempty"`
	CompletedAt    *time.Time        `json:"completed_at,omitempty" dynamodbav:"completed_at,omitempty"`
	ErrorMessage   string            `json:"error_message,omitempty" dynamodbav:"error_message,omitempty"`
	ErrorDetails   []ErrorDetail     `json:"error_details,omitempty" dynamodbav:"error_details,omitempty"` // 详细错误信息
	RetryCount     int               `json:"retry_count" dynamodbav:"retry_count"`
	MaxRetries     int               `json:"max_retries" dynamodbav:"max_retries"`
	Progress       map[string]string `json:"progress" dynamodbav:"progress"`       // 各转码类型的进度
	OutputFiles    map[string]string `json:"output_files" dynamodbav:"output_files"` // 输出文件映射
}

// ErrorDetail 错误详情
type ErrorDetail struct {
	TranscodeType string    `json:"transcode_type" dynamodbav:"transcode_type"` // 转码类型
	Stage         string    `json:"stage" dynamodbav:"stage"`                   // 失败阶段: download/transcode/upload
	Error         string    `json:"error" dynamodbav:"error"`                   // 错误信息
	Command       string    `json:"command,omitempty" dynamodbav:"command,omitempty"` // 执行的命令
	Output        string    `json:"output,omitempty" dynamodbav:"output,omitempty"`   // 命令输出/日志
	Timestamp     time.Time `json:"timestamp" dynamodbav:"timestamp"`           // 错误发生时间
}

// QueueMessage SQS队列消息结构 (API发送的格式)
type QueueMessage struct {
	TaskID         string   `json:"task_id"`
	InputBucket    string   `json:"input_bucket"`
	InputKey       string   `json:"input_key"`
	OutputBucket   string   `json:"output_bucket"`
	TranscodeTypes []string `json:"transcode_types"`
}

// S3EventMessage S3事件通知消息结构
type S3EventMessage struct {
	Records []S3EventRecord `json:"Records"`
}

// S3EventRecord S3事件记录
type S3EventRecord struct {
	EventSource string    `json:"eventSource"`
	EventName   string    `json:"eventName"`
	EventTime   string    `json:"eventTime"`
	S3          S3Entity  `json:"s3"`
}

// S3Entity S3实体信息
type S3Entity struct {
	Bucket S3Bucket `json:"bucket"`
	Object S3Object `json:"object"`
}

// S3Bucket S3桶信息
type S3Bucket struct {
	Name string `json:"name"`
	Arn  string `json:"arn"`
}

// S3Object S3对象信息
type S3Object struct {
	Key  string `json:"key"`
	Size int64  `json:"size"`
	ETag string `json:"eTag"`
}

// TaskListRequest 任务列表请求
type TaskListRequest struct {
	Status string `form:"status"`
	Date   string `form:"date"`   // 日期过滤，格式: 2025-01-15
	Limit  int    `form:"limit"`
	Offset int    `form:"offset"`
}

// TaskListResponse 任务列表响应
type TaskListResponse struct {
	Tasks  []TranscodeTask `json:"tasks"`
	Total  int             `json:"total"`
	Limit  int             `json:"limit"`
	Offset int             `json:"offset"`
}

// AddTaskRequest 添加任务请求
type AddTaskRequest struct {
	InputBucket    string   `json:"input_bucket" binding:"required"`
	InputKey       string   `json:"input_key" binding:"required"`
	TranscodeTypes []string `json:"transcode_types" binding:"required"`
}

// QueueStatusResponse 队列状态响应
type QueueStatusResponse struct {
	ApproximateNumberOfMessages           int `json:"approximate_number_of_messages"`
	ApproximateNumberOfMessagesNotVisible int `json:"approximate_number_of_messages_not_visible"`
}