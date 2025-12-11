package transcode

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"enhanced_video_transcoder/internal/task"
)

type Processor struct {
	s3Client      *s3.Client
	taskManager   *task.Manager
	presetManager *PresetManager
	tempDir       string
	outputBucket  string
	debug         bool
	gpuAvailable  bool
	platformInfo  *PlatformInfo
}

func NewProcessor(s3Client *s3.Client, taskManager *task.Manager, presetManager *PresetManager, tempDir, outputBucket string, debug bool) *Processor {
	processor := &Processor{
		s3Client:      s3Client,
		taskManager:   taskManager,
		presetManager: presetManager,
		tempDir:       tempDir,
		outputBucket:  outputBucket,
		debug:         debug,
	}

	// 检测平台和硬件加速能力
	processor.platformInfo = DetectPlatform()
	processor.gpuAvailable = processor.platformInfo.GPUAvailable

	// 创建临时目录
	if err := os.MkdirAll(processor.tempDir, 0755); err != nil {
		log.Printf("⚠️  无法创建临时目录: %v", err)
	}

	return processor
}

// GetPlatformInfo 获取平台信息
func (p *Processor) GetPlatformInfo() *PlatformInfo {
	return p.platformInfo
}

// ProcessTask 处理转码任务
func (p *Processor) ProcessTask(transcodeTask *task.TranscodeTask) error {
	log.Printf("🎬 开始处理任务: %s", transcodeTask.TaskID)

	// 检查任务是否存在，如果不存在则创建（S3事件触发的任务）
	_, err := p.taskManager.GetTask(transcodeTask.TaskID)
	if err != nil {
		log.Printf("📝 任务不存在，创建新任务记录: %s", transcodeTask.TaskID)
		if _, err := p.taskManager.CreateTaskWithID(
			transcodeTask.TaskID,
			transcodeTask.InputBucket,
			transcodeTask.InputKey,
			transcodeTask.OutputBucket,
			transcodeTask.TranscodeTypes,
		); err != nil {
			return fmt.Errorf("创建任务记录失败: %v", err)
		}
	}

	// 更新任务状态为处理中
	if err := p.taskManager.UpdateTaskStatus(transcodeTask.TaskID, task.TaskStatusProcessing, ""); err != nil {
		return fmt.Errorf("更新任务状态失败: %v", err)
	}

	// 下载输入文件
	inputFile, err := p.downloadFromS3(transcodeTask.InputBucket, transcodeTask.InputKey)
	if err != nil {
		errMsg := fmt.Sprintf("下载输入文件失败: %v", err)
		p.taskManager.AddErrorDetail(transcodeTask.TaskID, task.ErrorDetail{
			Stage:  "download",
			Error:  errMsg,
			Output: fmt.Sprintf("Bucket: %s, Key: %s", transcodeTask.InputBucket, transcodeTask.InputKey),
		})
		p.taskManager.UpdateTaskStatus(transcodeTask.TaskID, task.TaskStatusFailed, errMsg)
		return fmt.Errorf(errMsg)
	}
	defer os.Remove(inputFile)

	// 处理每个转码类型
	hasError := false
	aborted := false
	for _, transcodeType := range transcodeTask.TranscodeTypes {
		// 检查任务是否被中止
		if p.taskManager.IsTaskAborted(transcodeTask.TaskID) {
			log.Printf("⛔ 任务已被中止，停止处理: %s", transcodeTask.TaskID)
			aborted = true
			break
		}

		log.Printf("🔄 处理转码类型: %s", transcodeType)

		// 更新进度
		p.taskManager.UpdateTaskProgress(transcodeTask.TaskID, transcodeType, "processing")

		// 生成输出文件名
		outputFile, err := p.generateOutputFile(inputFile, transcodeType)
		if err != nil {
			errMsg := fmt.Sprintf("生成输出文件名失败: %v", err)
			log.Printf("❌ %s [%s]", errMsg, transcodeType)
			p.taskManager.AddErrorDetail(transcodeTask.TaskID, task.ErrorDetail{
				TranscodeType: transcodeType,
				Stage:         "prepare",
				Error:         errMsg,
			})
			p.taskManager.UpdateTaskProgress(transcodeTask.TaskID, transcodeType, "failed")
			hasError = true
			continue
		}

		// 执行转码
		if err := p.processTranscodeWithLog(transcodeTask.TaskID, inputFile, outputFile, transcodeType); err != nil {
			log.Printf("❌ 转码失败 [%s]: %v", transcodeType, err)
			p.taskManager.UpdateTaskProgress(transcodeTask.TaskID, transcodeType, "failed")
			hasError = true
			continue
		}

		// 再次检查任务是否被中止（转码完成后）
		if p.taskManager.IsTaskAborted(transcodeTask.TaskID) {
			log.Printf("⛔ 任务已被中止，停止处理: %s", transcodeTask.TaskID)
			aborted = true
			// 删除已生成的输出文件
			os.Remove(outputFile)
			break
		}

		// 上传到S3
		outputKey := filepath.Base(outputFile)
		if err := p.uploadToS3(outputFile, outputKey); err != nil {
			errMsg := fmt.Sprintf("上传失败: %v", err)
			log.Printf("❌ %s [%s]", errMsg, transcodeType)
			p.taskManager.AddErrorDetail(transcodeTask.TaskID, task.ErrorDetail{
				TranscodeType: transcodeType,
				Stage:         "upload",
				Error:         errMsg,
				Output:        fmt.Sprintf("OutputKey: %s", outputKey),
			})
			p.taskManager.UpdateTaskProgress(transcodeTask.TaskID, transcodeType, "failed")
			hasError = true
			continue
		}

		// 记录输出文件
		p.taskManager.AddOutputFile(transcodeTask.TaskID, transcodeType, outputKey)
		p.taskManager.UpdateTaskProgress(transcodeTask.TaskID, transcodeType, "completed")

		log.Printf("✅ 转码完成 [%s]", transcodeType)
	}

	// 更新最终任务状态
	if aborted {
		// 任务被中止，不更新状态（已经被 API 设置为 failed）
		log.Printf("⛔ 任务已中止: %s", transcodeTask.TaskID)
		return fmt.Errorf("任务已被用户中止")
	} else if hasError {
		p.taskManager.UpdateTaskStatus(transcodeTask.TaskID, task.TaskStatusFailed, "部分转码任务失败")
		return fmt.Errorf("部分转码任务失败")
	} else {
		p.taskManager.UpdateTaskStatus(transcodeTask.TaskID, task.TaskStatusCompleted, "")
		log.Printf("🎉 任务完成: %s", transcodeTask.TaskID)
	}

	return nil
}

// TestTranscode 测试转码（用于 LLM 生成的参数测试）
func (p *Processor) TestTranscode(inputFile string, ffmpegArgs []string, outputExt string) (*TranscodeResult, error) {
	// 生成临时输出文件
	baseName := strings.TrimSuffix(filepath.Base(inputFile), filepath.Ext(inputFile))
	outputFile := filepath.Join(p.tempDir, fmt.Sprintf("%s_test_%d.%s", baseName, time.Now().Unix(), outputExt))

	// 构建完整命令
	args := []string{}
	args = append(args, p.platformInfo.HWAccelArgs...)
	args = append(args, "-i", inputFile)
	args = append(args, ffmpegArgs...)
	args = append(args, "-y", outputFile)

	cmd := exec.Command("ffmpeg", args...)
	result := p.runFFmpegCommandWithLog(cmd, "测试转码")

	// 清理测试输出文件
	if result.Error == nil {
		os.Remove(outputFile)
	}

	return result, result.Error
}

// ProcessCustomPreset 处理自定义预设转码
func (p *Processor) ProcessCustomPreset(inputFile, outputFile string, preset *TranscodePreset) error {
	log.Printf("🔄 使用自定义预设转码: %s -> %s (预设: %s)", inputFile, outputFile, preset.Name)

	// 构建命令参数
	args := []string{}
	args = append(args, p.platformInfo.HWAccelArgs...)
	args = append(args, "-i", inputFile)
	args = append(args, preset.FFmpegArgs...)
	args = append(args, "-y", outputFile)

	cmd := exec.Command("ffmpeg", args...)
	return p.runFFmpegCommand(cmd, fmt.Sprintf("自定义预设: %s", preset.Name))
}

// downloadFromS3 从S3下载文件
func (p *Processor) downloadFromS3(bucket, key string) (string, error) {
	log.Printf("📥 从S3下载文件: s3://%s/%s", bucket, key)

	// 生成本地文件路径
	localFile := filepath.Join(p.tempDir, fmt.Sprintf("input_%d_%s", time.Now().Unix(), filepath.Base(key)))

	// 下载文件
	result, err := p.s3Client.GetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return "", fmt.Errorf("从S3获取对象失败: %v", err)
	}
	defer result.Body.Close()

	// 创建本地文件
	file, err := os.Create(localFile)
	if err != nil {
		return "", fmt.Errorf("创建本地文件失败: %v", err)
	}
	defer file.Close()

	// 复制内容
	if _, err := file.ReadFrom(result.Body); err != nil {
		return "", fmt.Errorf("写入本地文件失败: %v", err)
	}

	log.Printf("✅ 文件下载完成: %s", localFile)
	return localFile, nil
}

// generateOutputFile 生成输出文件路径
func (p *Processor) generateOutputFile(inputFile, transcodeType string) (string, error) {
	baseName := strings.TrimSuffix(filepath.Base(inputFile), filepath.Ext(inputFile))
	timestamp := time.Now().Unix()

	var outputFile string
	if transcodeType == "thumbnail" {
		outputFile = filepath.Join(p.tempDir, fmt.Sprintf("%s_%s_%d.jpg", baseName, transcodeType, timestamp))
	} else {
		outputFile = filepath.Join(p.tempDir, fmt.Sprintf("%s_%s_%d.mp4", baseName, transcodeType, timestamp))
	}

	return outputFile, nil
}

// processTranscodeWithLog 处理转码并记录详细日志
func (p *Processor) processTranscodeWithLog(taskID, inputFile, outputFile, transcodeType string) error {
	result := p.doTranscodeWithLog(inputFile, outputFile, transcodeType)

	// 如果GPU模式失败，尝试CPU回退
	if result.Error != nil && p.gpuAvailable && strings.Contains(result.Error.Error(), "GPU编码失败") {
		log.Printf("🔄 GPU失败，切换到CPU模式重试...")
		p.gpuAvailable = false
		result = p.doTranscodeWithLog(inputFile, outputFile, transcodeType)
	}

	// 如果失败，记录详细错误信息
	if result.Error != nil {
		p.taskManager.AddErrorDetail(taskID, task.ErrorDetail{
			TranscodeType: transcodeType,
			Stage:         "transcode",
			Error:         result.Error.Error(),
			Command:       result.Command,
			Output:        result.Output,
		})
	}

	return result.Error
}

// doTranscodeWithLog 执行转码并返回详细结果
func (p *Processor) doTranscodeWithLog(inputFile, outputFile, transcodeType string) *TranscodeResult {
	switch transcodeType {
	case "mp4_standard":
		return p.createMp4StandardWithLog(inputFile, outputFile)
	case "mp4_smooth":
		return p.createMp4SmoothWithLog(inputFile, outputFile)
	case "hdlbr_h265":
		return p.createHdlbrH265WithLog(inputFile, outputFile)
	case "lcd_h265":
		return p.createLcdH265WithLog(inputFile, outputFile)
	case "h265_mute":
		return p.createH265MuteTranscodeWithLog(inputFile, outputFile)
	case "custom_mute_preview":
		return p.createCustomMutePreviewWithLog(inputFile, outputFile)
	case "thumbnail":
		return p.createThumbnailWithLog(inputFile, outputFile)
	default:
		return &TranscodeResult{Error: fmt.Errorf("未知的转码类型: %s", transcodeType)}
	}
}

// uploadToS3 上传文件到S3
func (p *Processor) uploadToS3(localFile, s3Key string) error {
	log.Printf("📤 上传文件到S3: %s -> s3://%s/%s", localFile, p.outputBucket, s3Key)

	// 检查本地文件是否存在
	if _, err := os.Stat(localFile); os.IsNotExist(err) {
		return fmt.Errorf("本地文件不存在: %s", localFile)
	}

	// 打开文件
	file, err := os.Open(localFile)
	if err != nil {
		return fmt.Errorf("无法打开文件 %s: %v", localFile, err)
	}
	defer file.Close()

	// 获取文件信息
	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("无法获取文件信息: %v", err)
	}

	log.Printf("📊 上传文件大小: %.2f MB", float64(fileInfo.Size())/1024/1024)

	// 上传到S3
	_, err = p.s3Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket: aws.String(p.outputBucket),
		Key:    aws.String(s3Key),
		Body:   file,
	})

	if err != nil {
		return fmt.Errorf("S3上传失败: %v", err)
	}

	log.Printf("✅ 文件上传完成: s3://%s/%s", p.outputBucket, s3Key)

	// 删除本地临时文件
	if err := os.Remove(localFile); err != nil {
		log.Printf("⚠️  删除临时文件失败: %v", err)
	}

	return nil
}