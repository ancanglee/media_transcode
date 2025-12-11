package config

import (
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

	// API服务器配置
	APIPort  string
	APIHost  string
	Debug    bool

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