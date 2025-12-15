package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	"enhanced_video_transcoder/internal/api"
	"enhanced_video_transcoder/internal/config"
	"enhanced_video_transcoder/internal/llm"
	"enhanced_video_transcoder/internal/queue"
	"enhanced_video_transcoder/internal/task"
	"enhanced_video_transcoder/internal/transcode"
	"enhanced_video_transcoder/internal/user"
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

	// 加载 AWS 配置，支持多种凭证方式
	awsCfg, credSource, err := loadAWSConfig(cfg.AWSRegion)
	if err != nil {
		log.Fatalf("❌ 无法加载AWS配置: %v", err)
	}
	log.Printf("✅ AWS 凭证加载成功，来源: %s", credSource)

	// 创建AWS客户端
	sqsClient := sqs.NewFromConfig(awsCfg)
	dynamoClient := dynamodb.NewFromConfig(awsCfg)
	s3Client := s3.NewFromConfig(awsCfg)

	// 创建 Bedrock 客户端（用于 LLM）
	// Bedrock 可能需要使用特定区域，并且需要代理支持（用于访问 Anthropic 模型）
	bedrockRegion := os.Getenv("BEDROCK_REGION")
	if bedrockRegion == "" {
		bedrockRegion = "us-west-2" // Bedrock 默认区域
	}
	// Bedrock 使用代理，因为 Anthropic 模型有地区限制
	bedrockCfg, _, err := loadAWSConfigWithProxy(bedrockRegion, true)
	if err != nil {
		log.Printf("⚠️ 无法加载 Bedrock 配置: %v", err)
	}
	bedrockClient := bedrockruntime.NewFromConfig(bedrockCfg)

	// 创建管理器
	queueManager := queue.NewManager(sqsClient, cfg.SQSQueueURL)
	taskManager := task.NewManager(dynamoClient, cfg.DynamoDBTable)
	presetManager := transcode.NewPresetManager(dynamoClient, cfg.DynamoDBTable)
	userManager := user.NewManager(dynamoClient, cfg.UserTable, cfg.JWTSecret)

	// 初始化默认管理员账户
	if err := userManager.InitDefaultAdmin(); err != nil {
		log.Printf("⚠️ 初始化默认管理员失败: %v", err)
	}

	// 加载自定义预设
	if err := presetManager.LoadCustomPresets(); err != nil {
		log.Printf("⚠️ 加载自定义预设失败: %v", err)
	}

	// 创建转码处理器（用于测试转码）
	processor := transcode.NewProcessor(s3Client, taskManager, presetManager, cfg.TempDir, cfg.OutputBucket, cfg.Debug)

	// 创建 LLM 客户端
	llmClient := llm.NewBedrockClient(bedrockClient)

	// 创建API处理器
	handlers := api.NewHandlers(queueManager, taskManager, cfg.InputBucket, cfg.OutputBucket)
	llmHandlers := api.NewLLMHandlers(llmClient, processor, presetManager)
	authHandlers := api.NewAuthHandlers(userManager, cfg.APIKey)

	// 设置路由
	router := api.SetupRouter(handlers, llmHandlers, authHandlers, cfg.Debug)

	// 启动服务器
	addr := fmt.Sprintf("%s:%s", cfg.APIHost, cfg.APIPort)
	log.Printf("✅ API服务器启动成功")
	log.Printf("📍 监听地址: %s", addr)
	log.Printf("🌐 Web管理界面: http://%s:%s/admin", cfg.APIHost, cfg.APIPort)
	log.Printf("🪣 输出桶: %s", cfg.OutputBucket)
	log.Printf("📋 队列URL: %s", cfg.SQSQueueURL)
	log.Printf("🗄️  DynamoDB表: %s", cfg.DynamoDBTable)
	log.Printf("👤 用户表: %s", cfg.UserTable)
	log.Printf("🔑 API Key: %s", cfg.APIKey)
	log.Printf("🤖 Bedrock区域: %s", bedrockRegion)
	log.Printf("🖥️  平台: %s (GPU: %v)", processor.GetPlatformInfo().Platform, processor.GetPlatformInfo().GPUAvailable)

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


// getProxyHTTPClient 创建支持代理的 HTTP 客户端
// 优先使用 BEDROCK_PROXY_URL 配置，其次检查系统代理环境变量
func getProxyHTTPClient() *http.Client {
	// 优先检查配置文件中的代理设置
	proxyURL := os.Getenv("BEDROCK_PROXY_URL")

	// 如果配置文件没有设置，检查系统代理环境变量
	if proxyURL == "" {
		proxyURL = os.Getenv("HTTPS_PROXY")
	}
	if proxyURL == "" {
		proxyURL = os.Getenv("HTTP_PROXY")
	}
	if proxyURL == "" {
		proxyURL = os.Getenv("ALL_PROXY")
	}
	if proxyURL == "" {
		proxyURL = os.Getenv("https_proxy")
	}
	if proxyURL == "" {
		proxyURL = os.Getenv("http_proxy")
	}
	if proxyURL == "" {
		proxyURL = os.Getenv("all_proxy")
	}

	if proxyURL == "" {
		log.Println("🌐 [Proxy] 未配置代理，Bedrock 使用直连")
		return nil
	}

	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		log.Printf("⚠️ [Proxy] 解析代理URL失败: %v，使用直连", err)
		return nil
	}

	log.Printf("🌐 [Proxy] Bedrock 使用代理: %s", proxyURL)

	transport := &http.Transport{
		Proxy: http.ProxyURL(parsedURL),
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
		},
	}

	return &http.Client{
		Transport: transport,
		Timeout:   120 * time.Second, // Claude Opus 4.5 响应可能较慢
	}
}

// loadAWSConfig 加载 AWS 配置，支持多种凭证方式
// 优先级: 1. 环境变量 AK/SK  2. 共享凭证文件 (~/.aws/credentials)  3. EC2 IAM Role
// 返回: aws.Config, 凭证来源描述, error
func loadAWSConfig(region string) (aws.Config, string, error) {
	return loadAWSConfigWithProxy(region, false)
}

// loadAWSConfigWithProxy 加载 AWS 配置，可选择是否使用代理
func loadAWSConfigWithProxy(region string, useProxy bool) (aws.Config, string, error) {
	ctx := context.TODO()

	// 构建配置选项
	configOpts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(region),
	}

	// 如果需要代理，添加自定义 HTTP 客户端
	if useProxy {
		httpClient := getProxyHTTPClient()
		if httpClient != nil {
			configOpts = append(configOpts, awsconfig.WithHTTPClient(httpClient))
		}
	}

	// 1. 检查环境变量中的 AK/SK
	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")

	if accessKey != "" && secretKey != "" {
		log.Println("🔑 检测到环境变量中的 AWS 凭证 (AK/SK)")
		sessionToken := os.Getenv("AWS_SESSION_TOKEN") // 可选的 session token

		configOpts = append(configOpts, awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			accessKey, secretKey, sessionToken,
		)))

		cfg, err := awsconfig.LoadDefaultConfig(ctx, configOpts...)
		if err != nil {
			return aws.Config{}, "", fmt.Errorf("使用环境变量凭证加载配置失败: %v", err)
		}

		// 验证凭证有效
		creds, err := cfg.Credentials.Retrieve(ctx)
		if err != nil {
			return aws.Config{}, "", fmt.Errorf("环境变量凭证无效: %v", err)
		}
		log.Printf("✅ 环境变量凭证验证成功: AccessKeyID=%s...", creds.AccessKeyID[:min(10, len(creds.AccessKeyID))])
		return cfg, "环境变量 (AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY)", nil
	}

	// 2. 尝试从共享凭证文件加载 (~/.aws/credentials)
	log.Println("🔍 尝试从共享凭证文件加载 AWS 凭证...")
	configOpts = append(configOpts, awsconfig.WithSharedConfigProfile(os.Getenv("AWS_PROFILE")))

	sharedCfg, err := awsconfig.LoadDefaultConfig(ctx, configOpts...)
	if err == nil {
		creds, err := sharedCfg.Credentials.Retrieve(ctx)
		if err == nil && creds.AccessKeyID != "" {
			log.Printf("✅ 共享凭证文件验证成功: AccessKeyID=%s...", creds.AccessKeyID[:min(10, len(creds.AccessKeyID))])
			profile := os.Getenv("AWS_PROFILE")
			if profile == "" {
				profile = "default"
			}
			return sharedCfg, fmt.Sprintf("共享凭证文件 (~/.aws/credentials, profile: %s)", profile), nil
		}
	}

	// 3. 尝试使用 EC2 IAM Role (IMDS)
	log.Println("🔍 尝试使用 EC2 IAM Role (IMDS)...")
	ec2Opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(region),
		awsconfig.WithEC2IMDSClientEnableState(imds.ClientEnabled),
	}
	if useProxy {
		httpClient := getProxyHTTPClient()
		if httpClient != nil {
			ec2Opts = append(ec2Opts, awsconfig.WithHTTPClient(httpClient))
		}
	}

	ec2Cfg, err := awsconfig.LoadDefaultConfig(ctx, ec2Opts...)
	if err == nil {
		creds, err := ec2Cfg.Credentials.Retrieve(ctx)
		if err == nil && creds.AccessKeyID != "" {
			log.Printf("✅ EC2 IAM Role 凭证获取成功: AccessKeyID=%s...", creds.AccessKeyID[:min(10, len(creds.AccessKeyID))])
			return ec2Cfg, "EC2 IAM Role (IMDS)", nil
		}
	}

	// 所有方式都失败
	return aws.Config{}, "", fmt.Errorf("无法获取 AWS 凭证。请确保:\n" +
		"  1. 设置环境变量 AWS_ACCESS_KEY_ID 和 AWS_SECRET_ACCESS_KEY，或\n" +
		"  2. 配置 ~/.aws/credentials 文件，或\n" +
		"  3. 在 EC2 实例上配置 IAM Role")
}
