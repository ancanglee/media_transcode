package config

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"strconv"
	"time"
)

type Config struct {
	// AWS配置
	AWSRegion     string
	InputBucket   string
	OutputBucket  string
	SQSQueueURL   string
	DynamoDBTable string

	// 用户认证配置
	UserTable string
	JWTSecret string
	APIKey    string // API Key 认证，用于外部系统调用

	// API服务器配置
	APIPort string
	APIHost string
	Debug   bool

	// GPU处理器配置
	TempDir            string
	MaxConcurrentTasks int
	PollInterval       time.Duration
}

func LoadConfig() *Config {
	pollInterval, _ := time.ParseDuration(getEnv("POLL_INTERVAL", "10s"))
	maxTasks, _ := strconv.Atoi(getEnv("MAX_CONCURRENT_TASKS", "2"))
	debug, _ := strconv.ParseBool(getEnv("DEBUG_MODE", "false"))

	return &Config{
		AWSRegion:     getEnv("AWS_REGION", "us-west-2"),
		InputBucket:   getEnv("INPUT_BUCKET", ""),
		OutputBucket:  getEnv("OUTPUT_BUCKET", ""),
		SQSQueueURL:   getEnv("SQS_QUEUE_URL", ""),
		DynamoDBTable: getEnv("DYNAMODB_TABLE", "video-transcode-tasks"),

		UserTable: getEnv("USER_TABLE", "video-transcode-users"),
		JWTSecret: getOrGenerateJWTSecret(),
		APIKey:    getOrGenerateAPIKey(),

		APIPort: getEnv("API_PORT", "8080"),
		APIHost: getEnv("API_HOST", "0.0.0.0"),
		Debug:   debug,

		TempDir:            getEnv("TEMP_DIR", "/tmp/ffmpeg_processing"),
		MaxConcurrentTasks: maxTasks,
		PollInterval:       pollInterval,
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getOrGenerateJWTSecret 获取或自动生成JWT密钥
func getOrGenerateJWTSecret() string {
	if secret := os.Getenv("JWT_SECRET"); secret != "" {
		return secret
	}
	// 自动生成随机密钥
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// getOrGenerateAPIKey 获取或自动生成 API Key
func getOrGenerateAPIKey() string {
	if key := os.Getenv("API_KEY"); key != "" {
		return key
	}
	// 自动生成随机 API Key
	bytes := make([]byte, 24)
	rand.Read(bytes)
	return "vt_" + hex.EncodeToString(bytes) // vt_ 前缀表示 video transcoder
}