package api

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"enhanced_video_transcoder/internal/llm"
	"enhanced_video_transcoder/internal/transcode"
)

// LLMHandlers LLM 相关的处理器
type LLMHandlers struct {
	bedrockClient *llm.BedrockClient
	processor     *transcode.Processor
	presetManager *transcode.PresetManager
}

// NewLLMHandlers 创建 LLM 处理器
func NewLLMHandlers(bedrockClient *llm.BedrockClient, processor *transcode.Processor, presetManager *transcode.PresetManager) *LLMHandlers {
	return &LLMHandlers{
		bedrockClient: bedrockClient,
		processor:     processor,
		presetManager: presetManager,
	}
}

// GenerateFFmpegRequest 生成 FFmpeg 参数请求
type GenerateFFmpegRequest struct {
	Requirement string `json:"requirement" binding:"required"` // 用户需求描述
	InputFormat string `json:"input_format"`                   // 输入格式（可选）
}

// GenerateFFmpegResponse 生成 FFmpeg 参数响应
type GenerateFFmpegResponse struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	FFmpegArgs     []string `json:"ffmpeg_args"`
	OutputExt      string   `json:"output_ext"`
	Explanation    string   `json:"explanation"`
	EstimatedSpeed string   `json:"estimated_speed"`
	Platform       string   `json:"platform"`
}

// GenerateFFmpegParams 使用 LLM 生成 FFmpeg 参数
func (h *LLMHandlers) GenerateFFmpegParams(c *gin.Context) {
	var req GenerateFFmpegRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("❌ [LLM] 请求参数解析失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("请求参数错误: %v", err),
		})
		return
	}

	log.Printf("📝 [LLM] 收到生成请求: requirement=%q, input_format=%q", req.Requirement, req.InputFormat)

	if h.bedrockClient == nil {
		log.Printf("❌ [LLM] Bedrock 客户端未初始化")
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "LLM 服务未配置",
		})
		return
	}

	// 获取当前平台信息
	platformInfo := h.processor.GetPlatformInfo()
	platform := string(platformInfo.Platform)

	// 调用 LLM 生成参数
	llmReq := &llm.FFmpegGenerateRequest{
		UserRequirement: req.Requirement,
		InputFormat:     req.InputFormat,
		Platform:        platform,
	}

	log.Printf("🤖 [LLM] 调用 Bedrock 生成 FFmpeg 参数, 平台: %s", platform)

	result, err := h.bedrockClient.GenerateFFmpegParams(c.Request.Context(), llmReq)
	if err != nil {
		log.Printf("❌ [LLM] Bedrock 调用失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("生成参数失败: %v", err),
		})
		return
	}

	log.Printf("✅ [LLM] 参数生成成功: name=%s, args=%v", result.Name, result.FFmpegArgs)

	c.JSON(http.StatusOK, &GenerateFFmpegResponse{
		Name:           result.Name,
		Description:    result.Description,
		FFmpegArgs:     result.FFmpegArgs,
		OutputExt:      result.OutputExt,
		Explanation:    result.Explanation,
		EstimatedSpeed: result.EstimatedSpeed,
		Platform:       platform,
	})
}

// TestFFmpegRequest 测试 FFmpeg 参数请求
type TestFFmpegRequest struct {
	InputFile  string   `json:"input_file" binding:"required"` // 本地测试文件路径
	FFmpegArgs []string `json:"ffmpeg_args" binding:"required"`
	OutputExt  string   `json:"output_ext" binding:"required"`
}

// TestFFmpegParams 测试 FFmpeg 参数
func (h *LLMHandlers) TestFFmpegParams(c *gin.Context) {
	var req TestFFmpegRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("请求参数错误: %v", err),
		})
		return
	}

	result, err := h.processor.TestTranscode(req.InputFile, req.FFmpegArgs, req.OutputExt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   fmt.Sprintf("测试失败: %v", err),
			"command": result.Command,
			"output":  result.Output,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "测试成功",
		"command": result.Command,
		"output":  result.Output,
	})
}

// SavePresetRequest 保存预设请求
type SavePresetRequest struct {
	Name        string   `json:"name" binding:"required"`
	Description string   `json:"description"`
	FFmpegArgs  []string `json:"ffmpeg_args" binding:"required"`
	OutputExt   string   `json:"output_ext" binding:"required"`
}

// SavePreset 保存自定义预设
func (h *LLMHandlers) SavePreset(c *gin.Context) {
	var req SavePresetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("请求参数错误: %v", err),
		})
		return
	}

	platformInfo := h.processor.GetPlatformInfo()

	preset := &transcode.TranscodePreset{
		Name:        req.Name,
		Description: req.Description,
		FFmpegArgs:  req.FFmpegArgs,
		OutputExt:   req.OutputExt,
		Platform:    string(platformInfo.Platform),
	}

	if err := h.presetManager.SavePreset(preset); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("保存预设失败: %v", err),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":   "预设保存成功",
		"preset_id": preset.PresetID,
		"preset":    preset,
	})
}

// ListPresets 列出所有预设
func (h *LLMHandlers) ListPresets(c *gin.Context) {
	presets := h.presetManager.ListPresets()
	c.JSON(http.StatusOK, gin.H{
		"presets": presets,
		"total":   len(presets),
	})
}

// GetPreset 获取单个预设
func (h *LLMHandlers) GetPreset(c *gin.Context) {
	presetID := c.Param("preset_id")
	if presetID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "预设ID不能为空",
		})
		return
	}

	preset, err := h.presetManager.GetPreset(presetID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("预设不存在: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, preset)
}

// DeletePreset 删除预设
func (h *LLMHandlers) DeletePreset(c *gin.Context) {
	presetID := c.Param("preset_id")
	if presetID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "预设ID不能为空",
		})
		return
	}

	if err := h.presetManager.DeletePreset(presetID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("删除预设失败: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "预设删除成功",
	})
}

// GetPlatformInfo 获取平台信息
func (h *LLMHandlers) GetPlatformInfo(c *gin.Context) {
	platformInfo := h.processor.GetPlatformInfo()
	c.JSON(http.StatusOK, platformInfo)
}
