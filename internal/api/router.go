package api

import (
	"time"

	"github.com/gin-gonic/gin"
)

func SetupRouter(handlers *Handlers, llmHandlers *LLMHandlers, authHandlers *AuthHandlers, debug bool) *gin.Engine {
	if !debug {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	// 中间件
	router.Use(gin.Logger())
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())
	router.Use(timestampMiddleware())

	// 设置静态文件和 Web 管理界面路由
	SetupStaticRoutes(router)

	// API路由组
	api := router.Group("/api")
	{
		// 公开接口（无需认证）
		api.GET("/health", handlers.HealthCheck)
		api.POST("/auth/login", authHandlers.Login)

		// 需要认证的接口
		authenticated := api.Group("")
		authenticated.Use(authHandlers.AuthMiddleware())
		{
			// 用户信息
			authenticated.GET("/auth/me", authHandlers.GetCurrentUser)
			authenticated.PUT("/auth/password", authHandlers.UpdatePassword)

			// 系统配置
			authenticated.GET("/config", handlers.GetConfig)

			// 平台信息
			authenticated.GET("/platform", llmHandlers.GetPlatformInfo)

			// 队列管理
			queue := authenticated.Group("/queue")
			{
				queue.GET("/status", handlers.GetQueueStatus)
				queue.POST("/add", handlers.AddTaskToQueue)
				queue.DELETE("/purge", handlers.PurgeQueue)
			}

			// 任务管理
			tasks := authenticated.Group("/tasks")
			{
				tasks.GET("", handlers.ListTasks)
				tasks.GET("/:task_id", handlers.GetTask)
				tasks.POST("/:task_id/retry", handlers.RetryTask)
				tasks.POST("/:task_id/abort", handlers.AbortTask)
				tasks.DELETE("/:task_id", handlers.CancelTask)
			}

			// LLM 智能转码
			llm := authenticated.Group("/llm")
			{
				llm.POST("/generate", llmHandlers.GenerateFFmpegParams)
				llm.POST("/test", llmHandlers.TestFFmpegParams)
				llm.POST("/fix", llmHandlers.FixFFmpegParams)
			}

			// 预设管理
			presets := authenticated.Group("/presets")
			{
				presets.GET("", llmHandlers.ListPresets)
				presets.POST("", llmHandlers.SavePreset)
				presets.GET("/:preset_id", llmHandlers.GetPreset)
				presets.DELETE("/:preset_id", llmHandlers.DeletePreset)
			}

			// 文件上传
			authenticated.POST("/upload", handlers.UploadFile)

			// 用户管理（仅管理员）
			users := authenticated.Group("/users")
			users.Use(authHandlers.AdminMiddleware())
			{
				users.GET("", authHandlers.ListUsers)
				users.POST("", authHandlers.CreateUser)
				users.DELETE("/:username", authHandlers.DeleteUser)
				users.PUT("/:username/password", authHandlers.UpdatePassword)
			}
		}
	}

	return router
}

// corsMiddleware CORS中间件
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

// timestampMiddleware 时间戳中间件
func timestampMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("timestamp", time.Now().Unix())
		c.Next()
	}
}