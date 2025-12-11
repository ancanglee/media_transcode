package transcode

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"
)

// processTranscode 处理转码任务，支持GPU失败时的CPU回退
func (p *Processor) processTranscode(inputFile, outputFile, transcodeType string) error {
	// 首次尝试
	err := p.doTranscode(inputFile, outputFile, transcodeType)

	// 如果GPU模式失败，尝试CPU回退
	if err != nil && p.gpuAvailable && strings.Contains(err.Error(), "GPU编码失败") {
		log.Printf("🔄 GPU失败，切换到CPU模式重试...")
		p.gpuAvailable = false
		return p.doTranscode(inputFile, outputFile, transcodeType)
	}

	return err
}

// doTranscode 执行实际的转码操作
func (p *Processor) doTranscode(inputFile, outputFile, transcodeType string) error {
	switch transcodeType {
	case "mp4_standard":
		return p.createMp4Standard(inputFile, outputFile)
	case "mp4_smooth":
		return p.createMp4Smooth(inputFile, outputFile)
	case "hdlbr_h265":
		return p.createHdlbrH265(inputFile, outputFile)
	case "lcd_h265":
		return p.createLcdH265(inputFile, outputFile)
	case "h265_mute":
		return p.createH265MuteTranscode(inputFile, outputFile)
	case "custom_mute_preview":
		return p.createCustomMutePreview(inputFile, outputFile)
	case "thumbnail":
		return p.createThumbnail(inputFile, outputFile)
	default:
		return fmt.Errorf("未知的转码类型: %s", transcodeType)
	}
}

// TranscodeResult 转码结果，包含命令和输出信息
type TranscodeResult struct {
	Command string
	Output  string
	Error   error
}

// runFFmpegCommand 运行FFmpeg命令，支持GPU回退到CPU
func (p *Processor) runFFmpegCommand(cmd *exec.Cmd, taskName string) error {
	result := p.runFFmpegCommandWithLog(cmd, taskName)
	return result.Error
}

// runFFmpegCommandWithLog 运行FFmpeg命令并返回详细结果
func (p *Processor) runFFmpegCommandWithLog(cmd *exec.Cmd, taskName string) *TranscodeResult {
	start := time.Now()
	commandStr := strings.Join(cmd.Args, " ")
	log.Printf("开始执行 %s", taskName)
	log.Printf("FFmpeg命令: %s", commandStr)

	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	if err != nil {
		log.Printf("%s 失败: %v", taskName, err)
		log.Printf("FFmpeg输出: %s", outputStr)

		// 如果是GPU模式失败，尝试CPU回退
		if p.gpuAvailable && strings.Contains(outputStr, "nvenc") {
			log.Printf("⚠️  GPU编码失败，尝试CPU回退...")
			p.gpuAvailable = false
			return &TranscodeResult{
				Command: commandStr,
				Output:  outputStr,
				Error:   fmt.Errorf("GPU编码失败，需要CPU回退: %v", err),
			}
		}

		return &TranscodeResult{
			Command: commandStr,
			Output:  outputStr,
			Error:   fmt.Errorf("%s 失败: %v", taskName, err),
		}
	}

	duration := time.Since(start)
	log.Printf("%s 成功 (耗时: %v)", taskName, duration)
	return &TranscodeResult{
		Command: commandStr,
		Output:  outputStr,
		Error:   nil,
	}
}

// getVideoEncoder 根据平台选择视频编码器
func (p *Processor) getVideoEncoder() string {
	if p.platformInfo != nil {
		return p.platformInfo.H265Encoder
	}
	if p.gpuAvailable {
		return "hevc_nvenc"
	}
	return "libx265"
}

// getScaleFilter 根据GPU可用性选择缩放滤镜
func (p *Processor) getScaleFilter(width, height int) string {
	scaleStr := fmt.Sprintf("%d:%d:force_original_aspect_ratio=decrease", width, height)
	padStr := fmt.Sprintf("pad=%d:%d:(ow-iw)/2:(oh-ih)/2:black", width, height)

	// 统一使用CPU滤镜，避免GPU滤镜兼容性问题
	return fmt.Sprintf("scale=%s,%s", scaleStr, padStr)
}

// getHWAccelArgs 获取硬件加速参数
func (p *Processor) getHWAccelArgs() []string {
	if p.platformInfo != nil {
		return p.platformInfo.HWAccelArgs
	}
	if p.gpuAvailable {
		return []string{"-hwaccel", "cuda"}
	}
	return []string{}
}

// getQualityArgs 获取质量参数
func (p *Processor) getQualityArgs(quality int) []string {
	if p.platformInfo != nil {
		return p.platformInfo.GetQualityParam(quality)
	}
	if p.gpuAvailable {
		return []string{"-cq", fmt.Sprintf("%d", quality)}
	}
	return []string{"-crf", fmt.Sprintf("%d", quality)}
}

// getPresetArgs 获取预设参数
func (p *Processor) getPresetArgs(preset string) []string {
	if p.platformInfo != nil {
		return p.platformInfo.GetPresetParam(preset)
	}
	return []string{"-preset", preset}
}

// createMp4StandardWithLog MP4标清转码带日志
func (p *Processor) createMp4StandardWithLog(inputFile, outputFile string) *TranscodeResult {
	log.Printf("创建MP4标清(GPU加速 H.265+MP3智能缩放): %s -> %s", inputFile, outputFile)
	args := p.buildMp4StandardArgs(inputFile, outputFile)
	cmd := exec.Command("ffmpeg", args...)
	taskName := "MP4标清(H.265+MP3)"
	if p.gpuAvailable {
		taskName += " [GPU加速]"
	}
	return p.runFFmpegCommandWithLog(cmd, taskName)
}

// createMp4Standard MP4标清转码 - 跨平台硬件加速版本
func (p *Processor) createMp4Standard(inputFile, outputFile string) error {
	log.Printf("创建MP4标清(硬件加速 H.265+MP3智能缩放): %s -> %s", inputFile, outputFile)

	args := p.buildMp4StandardArgs(inputFile, outputFile)
	cmd := exec.Command("ffmpeg", args...)

	taskName := "MP4标清(H.265+MP3)"
	if p.gpuAvailable {
		taskName += fmt.Sprintf(" [%s]", p.platformInfo.Platform)
	}

	return p.runFFmpegCommand(cmd, taskName)
}

// buildMp4StandardArgs 构建MP4标清参数
func (p *Processor) buildMp4StandardArgs(inputFile, outputFile string) []string {
	args := []string{}
	args = append(args, p.getHWAccelArgs()...)
	args = append(args, "-i", inputFile)
	args = append(args, "-c:v", p.getVideoEncoder())
	args = append(args, p.getPresetArgs("fast")...)
	args = append(args, p.getQualityArgs(23)...)
	args = append(args, "-maxrate", "800k", "-bufsize", "1600k")
	args = append(args, "-vf", p.getScaleFilter(848, 480))
	args = append(args, "-c:a", "libmp3lame", "-b:a", "128k", "-ar", "44100", "-ac", "2")
	args = append(args, "-movflags", "+faststart", "-f", "mp4", "-y", outputFile)
	return args
}

// createMp4SmoothWithLog MP4流畅转码带日志
func (p *Processor) createMp4SmoothWithLog(inputFile, outputFile string) *TranscodeResult {
	log.Printf("创建MP4流畅(GPU加速 H.265+MP3智能缩放): %s -> %s", inputFile, outputFile)
	args := p.buildMp4SmoothArgs(inputFile, outputFile)
	cmd := exec.Command("ffmpeg", args...)
	taskName := "MP4流畅(H.265+MP3)"
	if p.gpuAvailable {
		taskName += " [GPU加速]"
	}
	return p.runFFmpegCommandWithLog(cmd, taskName)
}

// buildMp4SmoothArgs 构建MP4流畅参数
func (p *Processor) buildMp4SmoothArgs(inputFile, outputFile string) []string {
	args := []string{}
	args = append(args, p.getHWAccelArgs()...)
	args = append(args, "-i", inputFile)
	args = append(args, "-c:v", p.getVideoEncoder())
	args = append(args, p.getPresetArgs("fast")...)
	args = append(args, p.getQualityArgs(25)...)
	args = append(args, "-maxrate", "400k", "-bufsize", "800k")
	args = append(args, "-vf", p.getScaleFilter(640, 360))
	args = append(args, "-c:a", "libmp3lame", "-b:a", "128k", "-ar", "44100", "-ac", "2")
	args = append(args, "-movflags", "+faststart", "-f", "mp4", "-y", outputFile)
	return args
}

// createMp4Smooth MP4流畅转码 - 跨平台硬件加速版本
func (p *Processor) createMp4Smooth(inputFile, outputFile string) error {
	log.Printf("创建MP4流畅(硬件加速 H.265+MP3智能缩放): %s -> %s", inputFile, outputFile)

	args := p.buildMp4SmoothArgs(inputFile, outputFile)
	cmd := exec.Command("ffmpeg", args...)

	taskName := "MP4流畅(H.265+MP3)"
	if p.gpuAvailable {
		taskName += fmt.Sprintf(" [%s]", p.platformInfo.Platform)
	}

	return p.runFFmpegCommand(cmd, taskName)
}

// createHdlbrH265WithLog HDLBR H265转码带日志
func (p *Processor) createHdlbrH265WithLog(inputFile, outputFile string) *TranscodeResult {
	log.Printf("创建HDLBR H265全量(GPU加速): %s -> %s", inputFile, outputFile)
	args := []string{}
	args = append(args, p.getHWAccelArgs()...)
	args = append(args, "-i", inputFile)
	args = append(args, "-c:v", p.getVideoEncoder())
	args = append(args, p.getPresetArgs("fast")...)
	args = append(args, p.getQualityArgs(20)...)
	args = append(args, "-maxrate", "6000k", "-bufsize", "12000k", "-r", "25", "-g", "250")
	args = append(args, "-c:a", "libmp3lame", "-b:a", "128k", "-ar", "44100", "-ac", "2")
	args = append(args, "-af", "loudnorm=I=-17:TP=-1:LRA=11")
	args = append(args, "-movflags", "+faststart", "-f", "mp4", "-y", outputFile)
	cmd := exec.Command("ffmpeg", args...)
	taskName := "HDLBR H265全量(H.265+MP3)"
	if p.gpuAvailable {
		taskName += " [GPU加速]"
	}
	return p.runFFmpegCommandWithLog(cmd, taskName)
}

// createHdlbrH265 HDLBR有声H265转码 - 跨平台硬件加速版本
func (p *Processor) createHdlbrH265(inputFile, outputFile string) error {
	log.Printf("创建HDLBR H265全量(硬件加速): %s -> %s", inputFile, outputFile)

	args := []string{}
	args = append(args, p.getHWAccelArgs()...)
	args = append(args, "-i", inputFile)
	args = append(args, "-c:v", p.getVideoEncoder())
	args = append(args, p.getPresetArgs("fast")...)
	args = append(args, p.getQualityArgs(20)...)
	args = append(args, "-maxrate", "6000k", "-bufsize", "12000k")
	args = append(args, "-r", "25", "-g", "250")
	args = append(args, "-c:a", "libmp3lame", "-b:a", "128k", "-ar", "44100", "-ac", "2")
	args = append(args, "-af", "loudnorm=I=-17:TP=-1:LRA=11")
	args = append(args, "-movflags", "+faststart", "-f", "mp4", "-y", outputFile)

	cmd := exec.Command("ffmpeg", args...)
	taskName := "HDLBR H265全量(H.265+MP3)"
	if p.gpuAvailable {
		taskName += fmt.Sprintf(" [%s]", p.platformInfo.Platform)
	}
	return p.runFFmpegCommand(cmd, taskName)
}

// createLcdH265WithLog LCD H265转码带日志
func (p *Processor) createLcdH265WithLog(inputFile, outputFile string) *TranscodeResult {
	log.Printf("创建LCD H265(GPU加速): %s -> %s", inputFile, outputFile)
	args := []string{}
	args = append(args, p.getHWAccelArgs()...)
	args = append(args, "-i", inputFile)
	args = append(args, "-c:v", p.getVideoEncoder())
	args = append(args, p.getPresetArgs("fast")...)
	args = append(args, p.getQualityArgs(22)...)
	args = append(args, "-r", "25", "-g", "250")
	args = append(args, "-c:a", "libmp3lame", "-b:a", "128k", "-ar", "44100", "-ac", "2")
	args = append(args, "-af", "loudnorm=I=-10")
	args = append(args, "-movflags", "+faststart", "-f", "mp4", "-y", outputFile)
	cmd := exec.Command("ffmpeg", args...)
	taskName := "LCD H265(H.265+MP3)"
	if p.gpuAvailable {
		taskName += " [GPU加速]"
	}
	return p.runFFmpegCommandWithLog(cmd, taskName)
}

// createLcdH265 LCD H265转码 - 跨平台硬件加速版本
func (p *Processor) createLcdH265(inputFile, outputFile string) error {
	log.Printf("创建LCD H265(硬件加速): %s -> %s", inputFile, outputFile)

	args := []string{}
	args = append(args, p.getHWAccelArgs()...)
	args = append(args, "-i", inputFile)
	args = append(args, "-c:v", p.getVideoEncoder())
	args = append(args, p.getPresetArgs("fast")...)
	args = append(args, p.getQualityArgs(22)...)
	args = append(args, "-r", "25", "-g", "250")
	args = append(args, "-c:a", "libmp3lame", "-b:a", "128k", "-ar", "44100", "-ac", "2")
	args = append(args, "-af", "loudnorm=I=-10")
	args = append(args, "-movflags", "+faststart", "-f", "mp4", "-y", outputFile)

	cmd := exec.Command("ffmpeg", args...)
	taskName := "LCD H265(H.265+MP3)"
	if p.gpuAvailable {
		taskName += fmt.Sprintf(" [%s]", p.platformInfo.Platform)
	}
	return p.runFFmpegCommand(cmd, taskName)
}

// createH265MuteTranscodeWithLog H265静音转码带日志
func (p *Processor) createH265MuteTranscodeWithLog(inputFile, outputFile string) *TranscodeResult {
	log.Printf("创建H265静音转码(GPU加速): %s -> %s", inputFile, outputFile)
	args := []string{}
	args = append(args, p.getHWAccelArgs()...)
	args = append(args, "-i", inputFile)
	args = append(args, "-c:v", p.getVideoEncoder())
	args = append(args, p.getPresetArgs("fast")...)
	args = append(args, p.getQualityArgs(23)...)
	args = append(args, "-maxrate", "2867k", "-bufsize", "5734k")
	args = append(args, "-r", "25", "-g", "250", "-an")
	args = append(args, "-movflags", "+faststart", "-f", "mp4", "-y", outputFile)
	cmd := exec.Command("ffmpeg", args...)
	taskName := "H265静音转码"
	if p.gpuAvailable {
		taskName += " [GPU加速]"
	}
	return p.runFFmpegCommandWithLog(cmd, taskName)
}

// createH265MuteTranscode H265静音转码 - 跨平台硬件加速版本
func (p *Processor) createH265MuteTranscode(inputFile, outputFile string) error {
	log.Printf("创建H265静音转码(硬件加速): %s -> %s", inputFile, outputFile)

	args := []string{}
	args = append(args, p.getHWAccelArgs()...)
	args = append(args, "-i", inputFile)
	args = append(args, "-c:v", p.getVideoEncoder())
	args = append(args, p.getPresetArgs("fast")...)
	args = append(args, p.getQualityArgs(23)...)
	args = append(args, "-maxrate", "2867k", "-bufsize", "5734k")
	args = append(args, "-r", "25", "-g", "250")
	args = append(args, "-an") // 移除音频
	args = append(args, "-movflags", "+faststart", "-f", "mp4", "-y", outputFile)

	cmd := exec.Command("ffmpeg", args...)
	taskName := "H265静音转码"
	if p.gpuAvailable {
		taskName += fmt.Sprintf(" [%s]", p.platformInfo.Platform)
	}
	return p.runFFmpegCommand(cmd, taskName)
}

// createCustomMutePreviewWithLog 自定义静音预览带日志
func (p *Processor) createCustomMutePreviewWithLog(inputFile, outputFile string) *TranscodeResult {
	log.Printf("创建自定义静音预览(GPU加速): %s -> %s", inputFile, outputFile)
	args := []string{}
	args = append(args, p.getHWAccelArgs()...)
	args = append(args, "-i", inputFile)
	args = append(args, "-c:v", p.getVideoEncoder())
	args = append(args, p.getPresetArgs("fast")...)
	args = append(args, p.getQualityArgs(23)...)
	args = append(args, "-r", "25", "-g", "250", "-an")
	args = append(args, "-movflags", "+faststart", "-f", "mp4", "-y", outputFile)
	cmd := exec.Command("ffmpeg", args...)
	taskName := "自定义静音预览"
	if p.gpuAvailable {
		taskName += " [GPU加速]"
	}
	return p.runFFmpegCommandWithLog(cmd, taskName)
}

// createCustomMutePreview 自定义静音预览 - 跨平台硬件加速版本
func (p *Processor) createCustomMutePreview(inputFile, outputFile string) error {
	log.Printf("创建自定义静音预览(硬件加速): %s -> %s", inputFile, outputFile)

	args := []string{}
	args = append(args, p.getHWAccelArgs()...)
	args = append(args, "-i", inputFile)
	args = append(args, "-c:v", p.getVideoEncoder())
	args = append(args, p.getPresetArgs("fast")...)
	args = append(args, p.getQualityArgs(23)...)
	args = append(args, "-r", "25", "-g", "250")
	args = append(args, "-an") // 移除音频
	args = append(args, "-movflags", "+faststart", "-f", "mp4", "-y", outputFile)

	cmd := exec.Command("ffmpeg", args...)
	taskName := "自定义静音预览"
	if p.gpuAvailable {
		taskName += fmt.Sprintf(" [%s]", p.platformInfo.Platform)
	}
	return p.runFFmpegCommand(cmd, taskName)
}

// createThumbnailWithLog 生成缩略图带日志
func (p *Processor) createThumbnailWithLog(inputFile, outputFile string) *TranscodeResult {
	log.Printf("创建缩略图(GPU加速): %s -> %s", inputFile, outputFile)
	args := []string{}
	args = append(args, p.getHWAccelArgs()...)
	args = append(args, "-i", inputFile, "-ss", "00:00:04", "-vframes", "1")
	args = append(args, "-vf", "scale=1280:720:force_original_aspect_ratio=decrease,pad=1280:720:(ow-iw)/2:(oh-ih)/2:black")
	args = append(args, "-q:v", "2", "-y", outputFile)
	cmd := exec.Command("ffmpeg", args...)
	taskName := "缩略图生成"
	if p.gpuAvailable {
		taskName += " [GPU解码加速]"
	}
	return p.runFFmpegCommandWithLog(cmd, taskName)
}

// createThumbnail 生成缩略图 - 跨平台硬件加速版本
func (p *Processor) createThumbnail(inputFile, outputFile string) error {
	log.Printf("创建缩略图(硬件加速): %s -> %s", inputFile, outputFile)

	args := []string{}
	// 添加硬件加速参数（仅用于解码）
	args = append(args, p.getHWAccelArgs()...)
	args = append(args, "-i", inputFile)
	args = append(args, "-ss", "00:00:04")
	args = append(args, "-vframes", "1")
	args = append(args, "-vf", "scale=1280:720:force_original_aspect_ratio=decrease,pad=1280:720:(ow-iw)/2:(oh-ih)/2:black")
	args = append(args, "-q:v", "2")
	args = append(args, "-y", outputFile)

	cmd := exec.Command("ffmpeg", args...)
	taskName := "缩略图生成"
	if p.gpuAvailable {
		taskName += fmt.Sprintf(" [%s解码加速]", p.platformInfo.Platform)
	}
	return p.runFFmpegCommand(cmd, taskName)
}