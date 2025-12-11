package api

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"enhanced_video_transcoder/internal/queue"
	"enhanced_video_transcoder/internal/task"
)

type Handlers struct {
	queueManager *queue.Manager
	taskManager  *task.Manager
	inputBucket  string
	outputBucket string
}

func NewHandlers(queueManager *queue.Manager, taskManager *task.Manager, inputBucket, outputBucket string) *Handlers {
	return &Handlers{
		queueManager: queueManager,
		taskManager:  taskManager,
		inputBucket:  inputBucket,
		outputBucket: outputBucket,
	}
}

// GetQueueStatus 获取队列状态
func (h *Handlers) GetQueueStatus(c *gin.Context) {
	status, err := h.queueManager.GetQueueAttributes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("获取队列状态失败: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, status)
}

// AddTaskToQueue 添加任务到队列
func (h *Handlers) AddTaskToQueue(c *gin.Context) {
	var req task.AddTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("请求参数错误: %v", err),
		})
		return
	}

	// 创建任务记录
	transcodeTask, err := h.taskManager.CreateTask(req.InputBucket, req.InputKey, h.outputBucket, req.TranscodeTypes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("创建任务失败: %v", err),
		})
		return
	}

	// 发送消息到队列
	queueMessage := &task.QueueMessage{
		TaskID:         transcodeTask.TaskID,
		InputBucket:    req.InputBucket,
		InputKey:       req.InputKey,
		OutputBucket:   h.outputBucket,
		TranscodeTypes: req.TranscodeTypes,
	}

	if err := h.queueManager.SendMessage(queueMessage); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("发送消息到队列失败: %v", err),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "任务已添加到队列",
		"task_id": transcodeTask.TaskID,
		"task":    transcodeTask,
	})
}

// GetTask 获取任务详情
func (h *Handlers) GetTask(c *gin.Context) {
	taskID := c.Param("task_id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "任务ID不能为空",
		})
		return
	}

	transcodeTask, err := h.taskManager.GetTask(taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("获取任务失败: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, transcodeTask)
}

// ListTasks 获取任务列表
func (h *Handlers) ListTasks(c *gin.Context) {
	// 直接从查询参数获取，避免绑定问题
	status := c.Query("status")
	date := c.Query("date")
	limit := 10
	offset := 0

	if l := c.Query("limit"); l != "" {
		if parsed, err := fmt.Sscanf(l, "%d", &limit); err != nil || parsed != 1 {
			limit = 10
		}
	}
	if o := c.Query("offset"); o != "" {
		if parsed, err := fmt.Sscanf(o, "%d", &offset); err != nil || parsed != 1 {
			offset = 0
		}
	}

	// 调试：打印查询参数
	log.Printf("🔍 ListTasks 原始查询参数: %s", c.Request.URL.RawQuery)
	log.Printf("🔍 ListTasks 解析后: status=%q, date=%q, limit=%d, offset=%d", status, date, limit, offset)

	// 限制范围
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	tasks, total, err := h.taskManager.ListTasks(status, date, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("获取任务列表失败: %v", err),
		})
		return
	}

	response := &task.TaskListResponse{
		Tasks:  tasks,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}

	c.JSON(http.StatusOK, response)
}

// RetryTask 重试任务
func (h *Handlers) RetryTask(c *gin.Context) {
	taskID := c.Param("task_id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "任务ID不能为空",
		})
		return
	}

	// 重试任务
	if err := h.taskManager.RetryTask(taskID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("重试任务失败: %v", err),
		})
		return
	}

	// 获取更新后的任务
	transcodeTask, err := h.taskManager.GetTask(taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("获取任务失败: %v", err),
		})
		return
	}

	// 重新发送到队列
	queueMessage := &task.QueueMessage{
		TaskID:         transcodeTask.TaskID,
		InputBucket:    transcodeTask.InputBucket,
		InputKey:       transcodeTask.InputKey,
		OutputBucket:   transcodeTask.OutputBucket,
		TranscodeTypes: transcodeTask.TranscodeTypes,
	}

	if err := h.queueManager.SendMessage(queueMessage); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("发送消息到队列失败: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "任务重试成功",
		"task":    transcodeTask,
	})
}

// UploadFile 上传文件接口
func (h *Handlers) UploadFile(c *gin.Context) {
	// 获取上传的文件
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("获取上传文件失败: %v", err),
		})
		return
	}

	// 获取转码类型参数
	transcodeTypesStr := c.PostForm("transcode_types")
	if transcodeTypesStr == "" {
		transcodeTypesStr = "mp4_standard,mp4_smooth,thumbnail" // 默认转码类型
	}

	// TODO: 实现文件上传到S3的逻辑
	// 这里需要根据实际需求实现文件上传功能

	c.JSON(http.StatusOK, gin.H{
		"message":   "文件上传功能待实现",
		"filename":  file.Filename,
		"size":      file.Size,
		"transcode": transcodeTypesStr,
	})
}

// HealthCheck 健康检查
func (h *Handlers) HealthCheck(c *gin.Context) {
	// 简化版本，避免可能的类型转换错误
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"timestamp": time.Now().Unix(),
		"message":   "API服务器运行正常",
	})
}

// PurgeQueue 清空队列（管理接口）
func (h *Handlers) PurgeQueue(c *gin.Context) {
	if err := h.queueManager.PurgeQueue(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("清空队列失败: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "队列已清空",
	})
}

// CancelTask 取消任务（从队列中移除）
func (h *Handlers) CancelTask(c *gin.Context) {
	taskID := c.Param("task_id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "任务ID不能为空",
		})
		return
	}

	// 获取任务
	transcodeTask, err := h.taskManager.GetTask(taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("任务不存在: %v", err),
		})
		return
	}

	// 只能取消 pending 状态的任务
	if transcodeTask.Status != task.TaskStatusPending {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("只能取消等待中的任务，当前状态: %s", transcodeTask.Status),
		})
		return
	}

	// 尝试从队列中移除消息
	removed, err := h.queueManager.RemoveMessageByTaskID(taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("从队列移除消息失败: %v", err),
		})
		return
	}

	// 更新任务状态为已取消
	if err := h.taskManager.UpdateTaskStatus(taskID, task.TaskStatusCancelled, "用户取消"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("更新任务状态失败: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":            "任务已取消",
		"task_id":            taskID,
		"removed_from_queue": removed,
	})
}

// AbortTask 中止正在运行的任务
func (h *Handlers) AbortTask(c *gin.Context) {
	taskID := c.Param("task_id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "任务ID不能为空",
		})
		return
	}

	// 获取任务
	transcodeTask, err := h.taskManager.GetTask(taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("任务不存在: %v", err),
		})
		return
	}

	// 只能中止 processing 状态的任务
	if transcodeTask.Status != task.TaskStatusProcessing {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("只能中止处理中的任务，当前状态: %s", transcodeTask.Status),
		})
		return
	}

	// 将未完成的转码类型状态设置为 failed
	if err := h.taskManager.MarkIncompleteProgressAsFailed(taskID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("更新转码进度失败: %v", err),
		})
		return
	}

	// 更新任务状态为失败（中止的任务统一使用 failed 状态）
	if err := h.taskManager.UpdateTaskStatus(taskID, task.TaskStatusFailed, "用户手动中止"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("更新任务状态失败: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "任务已中止",
		"task_id": taskID,
	})
}

// GetConfig 获取系统配置
func (h *Handlers) GetConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"input_bucket":  h.inputBucket,
		"output_bucket": h.outputBucket,
	})
}