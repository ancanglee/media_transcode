package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)

// BedrockClient Bedrock LLM 客户端
type BedrockClient struct {
	client  *bedrockruntime.Client
	modelID string
}

// NewBedrockClient 创建 Bedrock 客户端
func NewBedrockClient(client *bedrockruntime.Client) *BedrockClient {
	// Claude Opus 4.5 跨区域推理配置文件 ID (us-west-2 区域)
	modelID := "us.anthropic.claude-opus-4-5-20251101-v1:0"
	log.Printf("🤖 [Bedrock] 初始化客户端, 模型: %s", modelID)
	return &BedrockClient{
		client:  client,
		modelID: modelID,
	}
}

// FFmpegGenerateRequest 生成 FFmpeg 参数的请求
type FFmpegGenerateRequest struct {
	UserRequirement string `json:"user_requirement"` // 用户的业务需求描述
	InputFormat     string `json:"input_format"`     // 输入文件格式 (可选)
	Platform        string `json:"platform"`         // 目标平台: linux_nvidia, macos_apple
}

// FFmpegGenerateResponse 生成的 FFmpeg 参数响应
type FFmpegGenerateResponse struct {
	Name           string   `json:"name"`            // 任务名称
	Description    string   `json:"description"`     // 任务描述
	FFmpegArgs     []string `json:"ffmpeg_args"`     // FFmpeg 参数列表
	OutputExt      string   `json:"output_ext"`      // 输出文件扩展名
	Explanation    string   `json:"explanation"`     // 参数解释
	EstimatedSpeed string   `json:"estimated_speed"` // 预估速度
}

// GenerateFFmpegParams 根据用户需求生成 FFmpeg 参数
func (b *BedrockClient) GenerateFFmpegParams(ctx context.Context, req *FFmpegGenerateRequest) (*FFmpegGenerateResponse, error) {
	systemPrompt := `你是一个专业的视频转码专家，精通 FFmpeg 的各种参数配置。
用户会用自然语言描述他们的视频转码需求，你需要生成对应的 FFmpeg 参数。

重要规则：
1. 生成的参数必须是有效的 FFmpeg 参数
2. 不要包含输入文件 (-i) 和输出文件路径，这些会由系统自动添加
3. 参数应该针对目标平台进行优化
4. 如果用户没有指定某些参数，使用合理的默认值
5. 始终添加 -y 参数以覆盖输出文件

平台特定编码器：
- linux_nvidia: 使用 NVIDIA GPU 加速 (hevc_nvenc, h264_nvenc)，硬件加速参数 -hwaccel cuda
- macos_apple: 使用 Apple VideoToolbox (hevc_videotoolbox, h264_videotoolbox)，硬件加速参数 -hwaccel videotoolbox

你必须以 JSON 格式返回结果，格式如下：
{
  "name": "任务名称（简短，用于标识）",
  "description": "任务描述（详细说明转码效果）",
  "ffmpeg_args": ["参数1", "参数2", ...],
  "output_ext": "输出扩展名（如 mp4, mkv, jpg）",
  "explanation": "参数解释（说明每个关键参数的作用）",
  "estimated_speed": "预估速度（如 2x, 实时, 0.5x）"
}

只返回 JSON，不要有其他内容。`

	userPrompt := fmt.Sprintf(`用户需求: %s

目标平台: %s
输入格式: %s

请生成对应的 FFmpeg 参数配置。`, req.UserRequirement, req.Platform, req.InputFormat)

	// 构建 Claude 消息格式
	messages := []map[string]interface{}{
		{
			"role": "user",
			"content": []map[string]string{
				{"type": "text", "text": userPrompt},
			},
		},
	}

	requestBody := map[string]interface{}{
		"anthropic_version": "bedrock-2023-05-31",
		"max_tokens":        4096,
		"system":            systemPrompt,
		"messages":          messages,
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %v", err)
	}

	log.Printf("🤖 [Bedrock] 调用模型: %s", b.modelID)
	log.Printf("📤 [Bedrock] 请求内容: %s", string(bodyBytes))

	output, err := b.client.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(b.modelID),
		ContentType: aws.String("application/json"),
		Accept:      aws.String("application/json"),
		Body:        bodyBytes,
	})
	if err != nil {
		log.Printf("❌ [Bedrock] 调用失败: %v", err)
		log.Printf("❌ [Bedrock] 模型ID: %s", b.modelID)
		return nil, fmt.Errorf("调用 Bedrock 失败: %v", err)
	}

	log.Printf("📥 [Bedrock] 收到响应, 长度: %d bytes", len(output.Body))

	// 解析响应
	var response struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}

	if err := json.Unmarshal(output.Body, &response); err != nil {
		return nil, fmt.Errorf("解析 Bedrock 响应失败: %v", err)
	}

	if len(response.Content) == 0 {
		return nil, fmt.Errorf("Bedrock 返回空响应")
	}

	// 提取 JSON 内容
	text := response.Content[0].Text
	text = strings.TrimSpace(text)

	// 尝试提取 JSON（可能被包裹在 markdown 代码块中）
	if strings.HasPrefix(text, "```") {
		lines := strings.Split(text, "\n")
		var jsonLines []string
		inJSON := false
		for _, line := range lines {
			if strings.HasPrefix(line, "```json") || strings.HasPrefix(line, "```") {
				if inJSON {
					break
				}
				inJSON = true
				continue
			}
			if inJSON {
				jsonLines = append(jsonLines, line)
			}
		}
		text = strings.Join(jsonLines, "\n")
	}

	var result FFmpegGenerateResponse
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		log.Printf("⚠️ 解析 LLM 返回的 JSON 失败: %v\n原始内容: %s", err, text)
		return nil, fmt.Errorf("解析生成的参数失败: %v", err)
	}

	log.Printf("✅ FFmpeg 参数生成成功: %s", result.Name)
	return &result, nil
}
