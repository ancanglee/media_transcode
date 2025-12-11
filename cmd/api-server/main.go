package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	"enhanced_video_transcoder/internal/api"
	"enhanced_video_transcoder/internal/config"
	"enhanced_video_transcoder/internal/queue"
	"enhanced_video_transcoder/internal/task"
)

func main() {
	log.Println("🚀 启动视频转码API服务器...")

	// 加载配置
	cfg := config.LoadConfig()

	// 验证必要的配置
	if cfg.SQSQueueURL == "" {
		log.Fatal("❌ SQS_QUEUE_URL 环境变量未设置")
	}
	if cfg.OutputBucket == "" {
		log.Fatal("❌ OUTPUT_BUCKET 环境变量未设置")
	}

	// 加载AWS配置，增加 IMDS 超时时间
	awsCfg, err := awsconfig.LoadDefaultConfig(context.TODO(),
		awsconfig.WithRegion(cfg.AWSRegion),
		awsconfig.WithEC2IMDSClientEnableState(imds.ClientEnabled),
	)
	if err != nil {
		log.Fatalf("❌ 无法加载AWS配置: %v", err)
	}

	// 测试凭证
	creds, err := awsCfg.Credentials.Retrieve(context.TODO())
	if err != nil {
		log.Printf("⚠️  凭证获取失败: %v", err)
		log.Println("💡 尝试使用环境变量中的凭证...")
	} else {
		log.Printf("✅ 凭证获取成功: AccessKeyID=%s...", creds.AccessKeyID[:10])
	}

	// 创建AWS客户端
	sqsClient := sqs.NewFromConfig(awsCfg)
	dynamoClient := dynamodb.NewFromConfig(awsCfg)

	// 创建管理器
	queueManager := queue.NewManager(sqsClient, cfg.SQSQueueURL)
	taskManager := task.NewManager(dynamoClient, cfg.DynamoDBTable)

	// 创建API处理器
	handlers := api.NewHandlers(queueManager, taskManager, cfg.InputBucket, cfg.OutputBucket)

	// 设置路由
	router := api.SetupRouter(handlers, cfg.Debug)

	// 启动服务器
	addr := fmt.Sprintf("%s:%s", cfg.APIHost, cfg.APIPort)
	log.Printf("✅ API服务器启动成功")
	log.Printf("📍 监听地址: %s", addr)
	log.Printf("🌐 Web管理界面: http://%s:%s/admin", cfg.APIHost, cfg.APIPort)
	log.Printf("🪣 输出桶: %s", cfg.OutputBucket)
	log.Printf("📋 队列URL: %s", cfg.SQSQueueURL)
	log.Printf("🗄️  DynamoDB表: %s", cfg.DynamoDBTable)

	// 优雅关闭
	go func() {
		if err := router.Run(addr); err != nil {
			log.Fatalf("❌ 服务器启动失败: %v", err)
		}
	}()

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("🛑 正在关闭服务器...")

	// 给服务器5秒时间完成正在处理的请求
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	select {
	case <-ctx.Done():
		log.Println("✅ 服务器已关闭")
	}
}