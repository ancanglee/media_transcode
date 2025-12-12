package transcode

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

// TranscodePreset 转码预设定义
type TranscodePreset struct {
	PresetID    string    `json:"preset_id" dynamodbav:"preset_id"`
	Name        string    `json:"name" dynamodbav:"name"`
	Description string    `json:"description" dynamodbav:"description"`
	FFmpegArgs  []string  `json:"ffmpeg_args" dynamodbav:"ffmpeg_args"`
	OutputExt   string    `json:"output_ext" dynamodbav:"output_ext"`
	Platform    string    `json:"platform" dynamodbav:"platform"` // all, linux_nvidia, macos_apple
	IsBuiltin   bool      `json:"is_builtin" dynamodbav:"is_builtin"`
	CreatedAt   time.Time `json:"created_at" dynamodbav:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" dynamodbav:"updated_at"`
}

// PresetManager 预设管理器
type PresetManager struct {
	dynamoClient *dynamodb.Client
	tableName    string
	presets      map[string]*TranscodePreset
	mu           sync.RWMutex
}

// NewPresetManager 创建预设管理器
func NewPresetManager(dynamoClient *dynamodb.Client, tableName string) *PresetManager {
	pm := &PresetManager{
		dynamoClient: dynamoClient,
		tableName:    tableName,
		presets:      make(map[string]*TranscodePreset),
	}
	pm.loadBuiltinPresets()
	return pm
}

// loadBuiltinPresets 加载内置预设
func (pm *PresetManager) loadBuiltinPresets() {
	builtins := []*TranscodePreset{
		{
			PresetID:    "mp4_standard",
			Name:        "MP4标清",
			Description: "848x480 分辨率，H.265编码，适合普通播放",
			OutputExt:   "mp4",
			Platform:    "all",
			IsBuiltin:   true,
		},
		{
			PresetID:    "mp4_smooth",
			Name:        "MP4流畅",
			Description: "640x360 分辨率，H.265编码，适合低带宽环境",
			OutputExt:   "mp4",
			Platform:    "all",
			IsBuiltin:   true,
		},
		{
			PresetID:    "hdlbr_h265",
			Name:        "HDLBR H265全量",
			Description: "高质量H.265编码，保持原始分辨率",
			OutputExt:   "mp4",
			Platform:    "all",
			IsBuiltin:   true,
		},
		{
			PresetID:    "lcd_h265",
			Name:        "LCD H265",
			Description: "LCD显示优化的H.265编码",
			OutputExt:   "mp4",
			Platform:    "all",
			IsBuiltin:   true,
		},
		{
			PresetID:    "h265_mute",
			Name:        "H265静音",
			Description: "H.265编码，移除音频轨道",
			OutputExt:   "mp4",
			Platform:    "all",
			IsBuiltin:   true,
		},
		{
			PresetID:    "custom_mute_preview",
			Name:        "静音预览",
			Description: "静音预览版本，适合快速预览",
			OutputExt:   "mp4",
			Platform:    "all",
			IsBuiltin:   true,
		},
		{
			PresetID:    "thumbnail",
			Name:        "缩略图",
			Description: "生成视频缩略图，1280x720",
			OutputExt:   "jpg",
			Platform:    "all",
			IsBuiltin:   true,
		},
	}

	for _, preset := range builtins {
		pm.presets[preset.PresetID] = preset
	}
	log.Printf("✅ 加载了 %d 个内置转码预设", len(builtins))
}

// SavePreset 保存自定义预设到 DynamoDB
func (pm *PresetManager) SavePreset(preset *TranscodePreset) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if preset.PresetID == "" {
		preset.PresetID = "custom_" + uuid.New().String()[:8]
	}
	preset.CreatedAt = time.Now()
	preset.UpdatedAt = time.Now()
	preset.IsBuiltin = false

	// 保存到 DynamoDB
	item, err := attributevalue.MarshalMap(preset)
	if err != nil {
		return fmt.Errorf("序列化预设失败: %v", err)
	}

	_, err = pm.dynamoClient.PutItem(context.TODO(), &dynamodb.PutItemInput{
		TableName: aws.String(pm.tableName + "-presets"),
		Item:      item,
	})
	if err != nil {
		log.Printf("⚠️ 保存预设到 DynamoDB 失败: %v (将仅保存在内存中)", err)
	}

	// 保存到内存
	pm.presets[preset.PresetID] = preset
	log.Printf("✅ 保存预设成功: %s (%s)", preset.Name, preset.PresetID)
	return nil
}

// GetPreset 获取预设
func (pm *PresetManager) GetPreset(presetID string) (*TranscodePreset, error) {
	pm.mu.RLock()
	preset, ok := pm.presets[presetID]
	pm.mu.RUnlock()

	if ok {
		return preset, nil
	}

	// 如果是自定义预设但不在内存中，尝试从 DynamoDB 加载
	if strings.HasPrefix(presetID, "custom_") {
		if loadedPreset, err := pm.loadPresetFromDynamoDB(presetID); err == nil {
			return loadedPreset, nil
		}
	}

	return nil, fmt.Errorf("预设不存在: %s", presetID)
}

// loadPresetFromDynamoDB 从 DynamoDB 加载单个预设
func (pm *PresetManager) loadPresetFromDynamoDB(presetID string) (*TranscodePreset, error) {
	result, err := pm.dynamoClient.GetItem(context.TODO(), &dynamodb.GetItemInput{
		TableName: aws.String(pm.tableName + "-presets"),
		Key: map[string]types.AttributeValue{
			"preset_id": &types.AttributeValueMemberS{Value: presetID},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("从 DynamoDB 获取预设失败: %v", err)
	}

	if result.Item == nil {
		return nil, fmt.Errorf("预设不存在: %s", presetID)
	}

	var preset TranscodePreset
	if err := attributevalue.UnmarshalMap(result.Item, &preset); err != nil {
		return nil, fmt.Errorf("反序列化预设失败: %v", err)
	}

	// 缓存到内存
	pm.mu.Lock()
	pm.presets[presetID] = &preset
	pm.mu.Unlock()

	log.Printf("✅ 从 DynamoDB 动态加载预设: %s (%s)", preset.Name, presetID)
	return &preset, nil
}

// ListPresets 列出所有预设
func (pm *PresetManager) ListPresets() []*TranscodePreset {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var presets []*TranscodePreset
	for _, preset := range pm.presets {
		presets = append(presets, preset)
	}
	return presets
}

// DeletePreset 删除自定义预设
func (pm *PresetManager) DeletePreset(presetID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	preset, ok := pm.presets[presetID]
	if !ok {
		return fmt.Errorf("预设不存在: %s", presetID)
	}

	if preset.IsBuiltin {
		return fmt.Errorf("不能删除内置预设: %s", presetID)
	}

	// 从 DynamoDB 删除
	_, err := pm.dynamoClient.DeleteItem(context.TODO(), &dynamodb.DeleteItemInput{
		TableName: aws.String(pm.tableName + "-presets"),
		Key: map[string]types.AttributeValue{
			"preset_id": &types.AttributeValueMemberS{Value: presetID},
		},
	})
	if err != nil {
		log.Printf("⚠️ 从 DynamoDB 删除预设失败: %v", err)
	}

	delete(pm.presets, presetID)
	log.Printf("✅ 删除预设成功: %s", presetID)
	return nil
}

// LoadCustomPresets 从 DynamoDB 加载自定义预设
func (pm *PresetManager) LoadCustomPresets() error {
	result, err := pm.dynamoClient.Scan(context.TODO(), &dynamodb.ScanInput{
		TableName: aws.String(pm.tableName + "-presets"),
	})
	if err != nil {
		log.Printf("⚠️ 从 DynamoDB 加载预设失败: %v", err)
		return nil // 不返回错误，允许系统继续运行
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	count := 0
	for _, item := range result.Items {
		var preset TranscodePreset
		if err := attributevalue.UnmarshalMap(item, &preset); err != nil {
			log.Printf("⚠️ 反序列化预设失败: %v", err)
			continue
		}
		pm.presets[preset.PresetID] = &preset
		count++
	}

	log.Printf("✅ 从 DynamoDB 加载了 %d 个自定义预设", count)
	return nil
}

// IsBuiltinPreset 检查是否为内置预设
func (pm *PresetManager) IsBuiltinPreset(presetID string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if preset, ok := pm.presets[presetID]; ok {
		return preset.IsBuiltin
	}
	return false
}

// ExportPreset 导出预设为 JSON
func (pm *PresetManager) ExportPreset(presetID string) (string, error) {
	preset, err := pm.GetPreset(presetID)
	if err != nil {
		return "", err
	}

	data, err := json.MarshalIndent(preset, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化预设失败: %v", err)
	}
	return string(data), nil
}

// ImportPreset 从 JSON 导入预设
func (pm *PresetManager) ImportPreset(jsonData string) (*TranscodePreset, error) {
	var preset TranscodePreset
	if err := json.Unmarshal([]byte(jsonData), &preset); err != nil {
		return nil, fmt.Errorf("解析预设 JSON 失败: %v", err)
	}

	// 生成新 ID 避免冲突
	preset.PresetID = "imported_" + uuid.New().String()[:8]
	preset.IsBuiltin = false

	if err := pm.SavePreset(&preset); err != nil {
		return nil, err
	}
	return &preset, nil
}
