package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	appConfig "enhanced_video_transcoder/internal/config"
	"enhanced_video_transcoder/internal/queue"
	"enhanced_video_transcoder/internal/task"
	"enhanced_video_transcoder/internal/transcode"
)

func main() {
	log.Println("🚀 启动GPU视频转码处理器...")

	// 加载配置
	cfg := appConfig.LoadConfig()

	// 验证必要的配置
	if cfg.SQSQueueURL == "" {
		log.Fatal("❌ SQS_QUEUE_URL 环境变量未设置")
	}
	if cfg.OutputBucket == "" {
		log.Fatal("❌ OUTPUT_BUCKET 环境变量未设置")
	}

	// 加载AWS配置
	awsCfg, err := awsconfig.LoadDefaultConfig(context.TODO(), awsconfig.WithRegion(cfg.AWSRegion))
	if err != nil {
		log.Fatalf("❌ 无法加载AWS配置: %v", err)
	}

	// 创建AWS客户端
	s3Client := s3.NewFromConfig(awsCfg)
	sqsClient := sqs.NewFromConfig(awsCfg)
	dynamoClient := dynamodb.NewFromConfig(awsCfg)

	// 创建管理器
	queueManager := queue.NewManager(sqsClient, cfg.SQSQueueURL)
	taskManager := task.NewManager(dynamoClient, cfg.DynamoDBTable)

	// 创建转码处理器
	processor := transcode.NewProcessor(s3Client, taskManager, cfg.TempDir, cfg.OutputBucket, cfg.Debug)

	log.Printf("✅ GPU处理器初始化完成")
	log.Printf("📁 临时目录: %s", cfg.TempDir)
	log.Printf("🪣 输出桶: %s", cfg.OutputBucket)
	log.Printf("📋 队列URL: %s", cfg.SQSQueueURL)
	log.Printf("🗄️  DynamoDB表: %s", cfg.DynamoDBTable)
	log.Printf("⚙️  最大并发任务: %d", cfg.MaxConcurrentTasks)
	log.Printf("⏱️  轮询间隔: %v", cfg.PollInterval)

	// 创建工作协程池
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())

	// 启动工作协程
	for i := 0; i < cfg.MaxConcurrentTasks; i++ {
		wg.Add(1)
		go worker(ctx, &wg, i+1, queueManager, processor, cfg.PollInterval, cfg.OutputBucket)
	}

	log.Printf("🔄 已启动 %d 个工作协程", cfg.MaxConcurrentTasks)

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("🛑 正在关闭处理器...")

	// 取消所有工作协程
	cancel()

	// 等待所有协程完成
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	// 给工作协程30秒时间完成当前任务
	select {
	case <-done:
		log.Println("✅ 所有工作协程已完成")
	case <-time.After(30 * time.Second):
		log.Println("⚠️  强制关闭处理器")
	}

	log.Println("✅ 处理器已关闭")
}

// worker 工作协程
func worker(ctx context.Context, wg *sync.WaitGroup, workerID int, queueManager *queue.Manager, processor *transcode.Processor, pollInterval time.Duration, defaultOutputBucket string) {
	defer wg.Done()

	log.Printf("🔧 工作协程 %d 已启动", workerID)

	for {
		select {
		case <-ctx.Done():
			log.Printf("🔧 工作协程 %d 正在关闭", workerID)
			return
		default:
			// 从队列接收消息
			messages, err := queueManager.ReceiveMessages(1, int32(pollInterval.Seconds()))
			if err != nil {
				log.Printf("⚠️  工作协程 %d 接收消息失败: %v", workerID, err)
				time.Sleep(pollInterval)
				continue
			}

			if len(messages) == 0 {
				// 没有消息，继续轮询
				continue
			}

			message := messages[0]
			log.Printf("🔧 工作协程 %d 接收到任务: %s", workerID, message.QueueMessage.TaskID)

			// 处理任务
			// 如果消息没有指定 OutputBucket（S3事件消息），使用配置的默认值
			outputBucket := message.QueueMessage.OutputBucket
			if outputBucket == "" {
				outputBucket = defaultOutputBucket
			}

			transcodeTask := &task.TranscodeTask{
				TaskID:         message.QueueMessage.TaskID,
				InputBucket:    message.QueueMessage.InputBucket,
				InputKey:       message.QueueMessage.InputKey,
				OutputBucket:   outputBucket,
				TranscodeTypes: message.QueueMessage.TranscodeTypes,
			}

			if err := processor.ProcessTask(transcodeTask); err != nil {
				log.Printf("❌ 工作协程 %d 处理任务失败: %v", workerID, err)
			} else {
				log.Printf("✅ 工作协程 %d 任务完成: %s", workerID, message.QueueMessage.TaskID)
			}

			// 删除队列中的消息
			if err := queueManager.DeleteMessage(message.ReceiptHandle); err != nil {
				log.Printf("⚠️  工作协程 %d 删除消息失败: %v", workerID, err)
			}
		}
	}
}