package api

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"

	"enhanced_video_transcoder/internal/llm"
	"enhanced_video_transcoder/internal/transcode"
)

// 本地测试视频相对路径（相对于 internal/api 目录）
const localTestVideoPath = "resources/test-video.mp4"

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
	TestResult     *TestResultInfo `json:"test_result,omitempty"`
}

// TestResultInfo 测试结果信息
type TestResultInfo struct {
	Success   bool   `json:"success"`
	Command   string `json:"command"`
	Output    string `json:"output"`
	Error     string `json:"error,omitempty"`
	Retries   int    `json:"retries"`
}

// GenerateFFmpegRequest 扩展请求
type GenerateFFmpegRequestExt struct {
	Requirement string `json:"requirement" binding:"required"`
	InputFormat string `json:"input_format"`
	AutoTest    bool   `json:"auto_test"` // 是否自动测试
}

// GenerateFFmpegParams 使用 LLM 生成 FFmpeg 参数（支持自动测试和修正）
func (h *LLMHandlers) GenerateFFmpegParams(c *gin.Context) {
	var req GenerateFFmpegRequestExt
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("❌ [LLM] 请求参数解析失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("请求参数错误: %v", err),
		})
		return
	}

	log.Printf("📝 [LLM] 收到生成请求: requirement=%q, input_format=%q, auto_test=%v", req.Requirement, req.InputFormat, req.AutoTest)

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

	response := &GenerateFFmpegResponse{
		Name:           result.Name,
		Description:    result.Description,
		FFmpegArgs:     result.FFmpegArgs,
		OutputExt:      result.OutputExt,
		Explanation:    result.Explanation,
		EstimatedSpeed: result.EstimatedSpeed,
		Platform:       platform,
	}

	// 如果启用自动测试
	if req.AutoTest {
		testResult := h.autoTestAndFix(c, llmReq, result, 3) // 最多重试3次
		response.FFmpegArgs = testResult.FinalArgs
		response.TestResult = &TestResultInfo{
			Success: testResult.Success,
			Command: testResult.Command,
			Output:  testResult.Output,
			Error:   testResult.Error,
			Retries: testResult.Retries,
		}
		if testResult.UpdatedExplanation != "" {
			response.Explanation = testResult.UpdatedExplanation
		}
	}

	c.JSON(http.StatusOK, response)
}

// AutoTestResult 自动测试结果
type AutoTestResult struct {
	Success            bool
	FinalArgs          []string
	Command            string
	Output             string
	Error              string
	Retries            int
	UpdatedExplanation string
}

// autoTestAndFix 自动测试并修正参数
func (h *LLMHandlers) autoTestAndFix(c *gin.Context, originalReq *llm.FFmpegGenerateRequest, result *llm.FFmpegGenerateResponse, maxRetries int) *AutoTestResult {
	// 确保有测试视频
	testVideo, err := h.ensureTestVideo()
	if err != nil {
		log.Printf("❌ [AutoTest] 获取测试视频失败: %v", err)
		return &AutoTestResult{
			Success:   false,
			FinalArgs: result.FFmpegArgs,
			Error:     fmt.Sprintf("获取测试视频失败: %v", err),
		}
	}

	currentArgs := result.FFmpegArgs
	var lastError string
	var lastOutput string
	var lastCommand string

	for retry := 0; retry <= maxRetries; retry++ {
		log.Printf("🧪 [AutoTest] 测试参数 (尝试 %d/%d): %v", retry+1, maxRetries+1, currentArgs)

		// 执行测试
		testResult, err := h.processor.TestTranscode(testVideo, currentArgs, result.OutputExt)
		lastCommand = testResult.Command
		lastOutput = testResult.Output

		if err == nil {
			log.Printf("✅ [AutoTest] 测试成功!")
			return &AutoTestResult{
				Success:   true,
				FinalArgs: currentArgs,
				Command:   lastCommand,
				Output:    lastOutput,
				Retries:   retry,
			}
		}

		lastError = err.Error()
		log.Printf("❌ [AutoTest] 测试失败 (尝试 %d): %v", retry+1, err)

		// 如果还有重试机会，让 LLM 修正参数
		if retry < maxRetries {
			log.Printf("🔄 [AutoTest] 请求 LLM 修正参数...")
			fixedResult, fixErr := h.bedrockClient.FixFFmpegParams(c.Request.Context(), &llm.FFmpegFixRequest{
				OriginalRequest: originalReq,
				FailedArgs:      currentArgs,
				ErrorMessage:    lastError,
				FFmpegOutput:    lastOutput,
			})
			if fixErr != nil {
				log.Printf("❌ [AutoTest] LLM 修正失败: %v", fixErr)
				continue
			}
			currentArgs = fixedResult.FFmpegArgs
			log.Printf("📝 [AutoTest] LLM 修正后的参数: %v", currentArgs)
		}
	}

	return &AutoTestResult{
		Success:   false,
		FinalArgs: currentArgs,
		Command:   lastCommand,
		Output:    lastOutput,
		Error:     lastError,
		Retries:   maxRetries,
	}
}

// ensureTestVideo 确保测试视频存在
func (h *LLMHandlers) ensureTestVideo() (string, error) {
	// 获取可执行文件所在目录
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("获取可执行文件路径失败: %v", err)
	}
	execDir := filepath.Dir(execPath)

	// 尝试多个可能的路径
	possiblePaths := []string{
		// 相对于可执行文件的路径
		filepath.Join(execDir, "internal", "api", localTestVideoPath),
		// 相对于当前工作目录的路径
		filepath.Join("internal", "api", localTestVideoPath),
		// 开发环境：直接使用相对路径
		filepath.Join(".", "internal", "api", localTestVideoPath),
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			absPath, _ := filepath.Abs(path)
			log.Printf("✅ [AutoTest] 使用本地测试视频: %s", absPath)
			return absPath, nil
		}
	}

	return "", fmt.Errorf("本地测试视频不存在，请确保文件存在于 internal/api/%s", localTestVideoPath)
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

// FixFFmpegRequest 修正 FFmpeg 参数请求
type FixFFmpegRequest struct {
	Requirement  string   `json:"requirement" binding:"required"`  // 原始需求
	InputFormat  string   `json:"input_format"`                    // 输入格式
	FailedArgs   []string `json:"failed_args" binding:"required"`  // 失败的参数
	OutputExt    string   `json:"output_ext" binding:"required"`   // 输出扩展名
	ErrorMessage string   `json:"error_message" binding:"required"` // 错误信息
	FFmpegOutput string   `json:"ffmpeg_output"`                   // FFmpeg 输出
}

// FixFFmpegResponse 修正 FFmpeg 参数响应
type FixFFmpegResponse struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	FFmpegArgs  []string `json:"ffmpeg_args"`
	OutputExt   string   `json:"output_ext"`
	Explanation string   `json:"explanation"`
}

// FixFFmpegParams 让 LLM 修正失败的 FFmpeg 参数
func (h *LLMHandlers) FixFFmpegParams(c *gin.Context) {
	var req FixFFmpegRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("❌ [LLM Fix] 请求参数解析失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("请求参数错误: %v", err),
		})
		return
	}

	log.Printf("🔧 [LLM Fix] 收到修正请求: requirement=%q, failed_args=%v", req.Requirement, req.FailedArgs)

	if h.bedrockClient == nil {
		log.Printf("❌ [LLM Fix] Bedrock 客户端未初始化")
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "LLM 服务未配置",
		})
		return
	}

	// 获取当前平台信息
	platformInfo := h.processor.GetPlatformInfo()
	platform := string(platformInfo.Platform)

	// 调用 LLM 修正参数
	fixReq := &llm.FFmpegFixRequest{
		OriginalRequest: &llm.FFmpegGenerateRequest{
			UserRequirement: req.Requirement,
			InputFormat:     req.InputFormat,
			Platform:        platform,
		},
		FailedArgs:   req.FailedArgs,
		ErrorMessage: req.ErrorMessage,
		FFmpegOutput: req.FFmpegOutput,
	}

	result, err := h.bedrockClient.FixFFmpegParams(c.Request.Context(), fixReq)
	if err != nil {
		log.Printf("❌ [LLM Fix] Bedrock 调用失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("修正参数失败: %v", err),
		})
		return
	}

	log.Printf("✅ [LLM Fix] 参数修正成功: args=%v", result.FFmpegArgs)

	c.JSON(http.StatusOK, &FixFFmpegResponse{
		Name:        result.Name,
		Description: result.Description,
		FFmpegArgs:  result.FFmpegArgs,
		OutputExt:   result.OutputExt,
		Explanation: result.Explanation,
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
