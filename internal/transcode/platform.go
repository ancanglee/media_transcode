package transcode

import (
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"strings"
)

// Platform 平台类型
type Platform string

const (
	PlatformLinuxNvidia Platform = "linux_nvidia"  // Linux + NVIDIA GPU
	PlatformMacOSApple  Platform = "macos_apple"   // macOS + Apple Silicon
	PlatformCPU         Platform = "cpu"           // 纯 CPU 模式
)

// PlatformInfo 平台信息
type PlatformInfo struct {
	Platform       Platform `json:"platform"`
	OS             string   `json:"os"`
	Arch           string   `json:"arch"`
	GPUAvailable   bool     `json:"gpu_available"`
	GPUName        string   `json:"gpu_name,omitempty"`
	HWAccel        string   `json:"hw_accel,omitempty"`
	VideoEncoder   string   `json:"video_encoder"`
	H264Encoder    string   `json:"h264_encoder"`
	H265Encoder    string   `json:"h265_encoder"`
	HWAccelArgs    []string `json:"hw_accel_args"`
}

// DetectPlatform 检测当前平台和硬件加速能力
func DetectPlatform() *PlatformInfo {
	info := &PlatformInfo{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	}

	log.Printf("🔍 检测平台环境: OS=%s, Arch=%s", info.OS, info.Arch)

	switch runtime.GOOS {
	case "darwin":
		info.detectMacOS()
	case "linux":
		info.detectLinux()
	default:
		info.setupCPUMode()
	}

	log.Printf("✅ 平台检测完成: %s, GPU=%v, 编码器=%s", info.Platform, info.GPUAvailable, info.H265Encoder)
	return info
}

// detectMacOS 检测 macOS 平台
func (p *PlatformInfo) detectMacOS() {
	p.Platform = PlatformMacOSApple

	// 检查是否为 Apple Silicon
	if p.Arch == "arm64" {
		log.Printf("✅ 检测到 Apple Silicon (arm64)")
	}

	// 检查 VideoToolbox 支持
	if p.checkVideoToolbox() {
		p.GPUAvailable = true
		p.GPUName = "Apple VideoToolbox"
		p.HWAccel = "videotoolbox"
		p.H264Encoder = "h264_videotoolbox"
		p.H265Encoder = "hevc_videotoolbox"
		p.VideoEncoder = p.H265Encoder
		p.HWAccelArgs = []string{"-hwaccel", "videotoolbox"}
		log.Printf("✅ VideoToolbox 硬件加速可用")
	} else {
		p.setupCPUMode()
	}
}

// detectLinux 检测 Linux 平台
func (p *PlatformInfo) detectLinux() {
	// 检查 NVIDIA GPU
	if p.checkNvidiaGPU() {
		p.Platform = PlatformLinuxNvidia
		p.GPUAvailable = true
		p.HWAccel = "cuda"
		p.H264Encoder = "h264_nvenc"
		p.H265Encoder = "hevc_nvenc"
		p.VideoEncoder = p.H265Encoder
		p.HWAccelArgs = []string{"-hwaccel", "cuda"}
		log.Printf("✅ NVIDIA NVENC 硬件加速可用")
	} else {
		p.setupCPUMode()
	}
}

// setupCPUMode 设置 CPU 模式
func (p *PlatformInfo) setupCPUMode() {
	p.Platform = PlatformCPU
	p.GPUAvailable = false
	p.H264Encoder = "libx264"
	p.H265Encoder = "libx265"
	p.VideoEncoder = p.H265Encoder
	p.HWAccelArgs = []string{}
	log.Printf("⚠️ 使用 CPU 软件编码模式")
}

// checkVideoToolbox 检查 VideoToolbox 是否可用
func (p *PlatformInfo) checkVideoToolbox() bool {
	// 检查 FFmpeg 是否支持 VideoToolbox
	cmd := exec.Command("ffmpeg", "-encoders")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("⚠️ 无法检查 FFmpeg 编码器: %v", err)
		return false
	}

	encoderOutput := string(output)
	if !strings.Contains(encoderOutput, "hevc_videotoolbox") {
		log.Printf("⚠️ FFmpeg 不支持 hevc_videotoolbox")
		return false
	}

	// 测试 VideoToolbox 是否真正可用
	testCmd := exec.Command("ffmpeg", "-f", "lavfi", "-i", "testsrc=duration=1:size=320x240:rate=1",
		"-c:v", "hevc_videotoolbox", "-f", "null", "-")
	if err := testCmd.Run(); err != nil {
		log.Printf("⚠️ VideoToolbox 测试失败: %v", err)
		return false
	}

	return true
}

// checkNvidiaGPU 检查 NVIDIA GPU 是否可用
func (p *PlatformInfo) checkNvidiaGPU() bool {
	// 检查 nvidia-smi
	cmd := exec.Command("nvidia-smi", "--query-gpu=name,driver_version", "--format=csv,noheader")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("⚠️ NVIDIA GPU 不可用: %v", err)
		return false
	}

	p.GPUName = strings.TrimSpace(string(output))
	log.Printf("✅ 检测到 NVIDIA GPU: %s", p.GPUName)

	// 检查 FFmpeg NVENC 支持
	cmd = exec.Command("ffmpeg", "-encoders")
	output, err = cmd.Output()
	if err != nil {
		log.Printf("⚠️ 无法检查 FFmpeg 编码器: %v", err)
		return false
	}

	encoderOutput := string(output)
	if !strings.Contains(encoderOutput, "hevc_nvenc") {
		log.Printf("⚠️ FFmpeg 不支持 hevc_nvenc")
		return false
	}

	// 测试 NVENC 是否真正可用
	testCmd := exec.Command("ffmpeg", "-f", "lavfi", "-i", "testsrc=duration=1:size=320x240:rate=1",
		"-c:v", "hevc_nvenc", "-preset", "fast", "-f", "null", "-")
	if err := testCmd.Run(); err != nil {
		log.Printf("⚠️ NVENC 测试失败: %v", err)
		return false
	}

	return true
}

// GetScaleFilter 获取缩放滤镜
func (p *PlatformInfo) GetScaleFilter(width, height int) string {
	scaleStr := fmt.Sprintf("%d:%d:force_original_aspect_ratio=decrease", width, height)
	padStr := fmt.Sprintf("pad=%d:%d:(ow-iw)/2:(oh-ih)/2:black", width, height)
	return fmt.Sprintf("scale=%s,%s", scaleStr, padStr)
}

// GetQualityParam 获取质量参数
func (p *PlatformInfo) GetQualityParam(quality int) []string {
	switch p.Platform {
	case PlatformLinuxNvidia:
		// NVENC 使用 -cq 参数
		return []string{"-cq", fmt.Sprintf("%d", quality)}
	case PlatformMacOSApple:
		// VideoToolbox 使用 -q:v 参数 (1-100, 值越高质量越好)
		// 将 CRF 风格的值转换为 VideoToolbox 的质量值
		vtQuality := 100 - quality*3 // 大致转换
		if vtQuality < 1 {
			vtQuality = 1
		}
		if vtQuality > 100 {
			vtQuality = 100
		}
		return []string{"-q:v", fmt.Sprintf("%d", vtQuality)}
	default:
		// CPU 使用 -crf 参数
		return []string{"-crf", fmt.Sprintf("%d", quality)}
	}
}

// GetPresetParam 获取预设参数
func (p *PlatformInfo) GetPresetParam(preset string) []string {
	switch p.Platform {
	case PlatformLinuxNvidia:
		return []string{"-preset", preset}
	case PlatformMacOSApple:
		// VideoToolbox 不支持 preset，使用 realtime 或 quality 模式
		if preset == "fast" || preset == "veryfast" || preset == "ultrafast" {
			return []string{"-realtime", "1"}
		}
		return []string{}
	default:
		return []string{"-preset", preset}
	}
}

// BuildEncoderArgs 构建编码器参数
func (p *PlatformInfo) BuildEncoderArgs(codec string, quality int, preset string) []string {
	args := []string{}

	// 选择编码器
	var encoder string
	switch codec {
	case "h264":
		encoder = p.H264Encoder
	case "h265", "hevc":
		encoder = p.H265Encoder
	default:
		encoder = p.VideoEncoder
	}

	args = append(args, "-c:v", encoder)
	args = append(args, p.GetPresetParam(preset)...)
	args = append(args, p.GetQualityParam(quality)...)

	return args
}
