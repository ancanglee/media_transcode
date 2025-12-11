package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"enhanced_video_transcoder/internal/task"
)

type Manager struct {
	sqsClient *sqs.Client
	queueURL  string
}

func NewManager(sqsClient *sqs.Client, queueURL string) *Manager {
	return &Manager{
		sqsClient: sqsClient,
		queueURL:  queueURL,
	}
}

// SendMessage 发送消息到队列
func (m *Manager) SendMessage(message *task.QueueMessage) error {
	messageBody, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("序列化消息失败: %v", err)
	}

	_, err = m.sqsClient.SendMessage(context.TODO(), &sqs.SendMessageInput{
		QueueUrl:    aws.String(m.queueURL),
		MessageBody: aws.String(string(messageBody)),
		MessageAttributes: map[string]types.MessageAttributeValue{
			"TaskID": {
				DataType:    aws.String("String"),
				StringValue: aws.String(message.TaskID),
			},
		},
	})

	if err != nil {
		return fmt.Errorf("发送消息到SQS失败: %v", err)
	}

	log.Printf("✅ 消息已发送到队列: TaskID=%s", message.TaskID)
	return nil
}

// ReceiveMessages 从队列接收消息
func (m *Manager) ReceiveMessages(maxMessages int32, waitTimeSeconds int32) ([]Message, error) {
	result, err := m.sqsClient.ReceiveMessage(context.TODO(), &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(m.queueURL),
		MaxNumberOfMessages: maxMessages,
		WaitTimeSeconds:     waitTimeSeconds,
		MessageAttributeNames: []string{"All"},
	})

	if err != nil {
		return nil, fmt.Errorf("从SQS接收消息失败: %v", err)
	}

	var messages []Message
	for _, msg := range result.Messages {
		queueMessage, err := m.parseMessage(*msg.Body)
		if err != nil {
			log.Printf("⚠️  解析消息失败: %v", err)
			continue
		}

		messages = append(messages, Message{
			ReceiptHandle: *msg.ReceiptHandle,
			MessageID:     *msg.MessageId,
			QueueMessage:  *queueMessage,
		})
	}

	return messages, nil
}

// parseMessage 解析消息，支持 API 格式和 S3 事件格式
func (m *Manager) parseMessage(body string) (*task.QueueMessage, error) {
	// 先尝试解析为 S3 事件消息
	var s3Event task.S3EventMessage
	if err := json.Unmarshal([]byte(body), &s3Event); err == nil && len(s3Event.Records) > 0 {
		return m.parseS3Event(&s3Event)
	}

	// 尝试解析为 API 发送的 QueueMessage
	var queueMessage task.QueueMessage
	if err := json.Unmarshal([]byte(body), &queueMessage); err != nil {
		return nil, fmt.Errorf("无法解析消息: %v", err)
	}

	return &queueMessage, nil
}

// parseS3Event 解析 S3 事件消息并转换为 QueueMessage
func (m *Manager) parseS3Event(s3Event *task.S3EventMessage) (*task.QueueMessage, error) {
	if len(s3Event.Records) == 0 {
		return nil, fmt.Errorf("S3事件记录为空")
	}

	record := s3Event.Records[0]
	
	// 只处理 ObjectCreated 事件
	if record.EventSource != "aws:s3" {
		return nil, fmt.Errorf("非S3事件: %s", record.EventSource)
	}

	// URL 解码 key (S3 事件中的 key 是 URL 编码的)
	key, err := url.QueryUnescape(record.S3.Object.Key)
	if err != nil {
		key = record.S3.Object.Key
	}

	// 检查是否为视频文件
	if !isVideoFile(key) {
		return nil, fmt.Errorf("非视频文件，跳过: %s", key)
	}

	log.Printf("📥 收到S3事件: bucket=%s, key=%s, event=%s", 
		record.S3.Bucket.Name, key, record.EventName)

	// 生成任务ID
	taskID := fmt.Sprintf("s3-%d", time.Now().UnixNano())

	return &task.QueueMessage{
		TaskID:         taskID,
		InputBucket:    record.S3.Bucket.Name,
		InputKey:       key,
		OutputBucket:   "", // 将在处理时使用配置的默认输出桶
		TranscodeTypes: []string{"mp4_standard", "mp4_smooth", "thumbnail"}, // 默认转码类型
	}, nil
}

// isVideoFile 检查文件是否为视频文件
func isVideoFile(key string) bool {
	key = strings.ToLower(key)
	videoExtensions := []string{".mp4", ".mov", ".avi", ".mkv", ".wmv", ".flv", ".webm", ".m4v", ".mpeg", ".mpg"}
	for _, ext := range videoExtensions {
		if strings.HasSuffix(key, ext) {
			return true
		}
	}
	return false
}

// DeleteMessage 删除队列中的消息
func (m *Manager) DeleteMessage(receiptHandle string) error {
	_, err := m.sqsClient.DeleteMessage(context.TODO(), &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(m.queueURL),
		ReceiptHandle: aws.String(receiptHandle),
	})

	if err != nil {
		return fmt.Errorf("删除SQS消息失败: %v", err)
	}

	return nil
}

// GetQueueAttributes 获取队列属性
func (m *Manager) GetQueueAttributes() (*task.QueueStatusResponse, error) {
	result, err := m.sqsClient.GetQueueAttributes(context.TODO(), &sqs.GetQueueAttributesInput{
		QueueUrl: aws.String(m.queueURL),
		AttributeNames: []types.QueueAttributeName{
			types.QueueAttributeNameApproximateNumberOfMessages,
			types.QueueAttributeNameApproximateNumberOfMessagesNotVisible,
		},
	})

	if err != nil {
		return nil, fmt.Errorf("获取队列属性失败: %v", err)
	}

	status := &task.QueueStatusResponse{}

	if val, ok := result.Attributes[string(types.QueueAttributeNameApproximateNumberOfMessages)]; ok {
		if count, err := strconv.Atoi(val); err == nil {
			status.ApproximateNumberOfMessages = count
		}
	}

	if val, ok := result.Attributes[string(types.QueueAttributeNameApproximateNumberOfMessagesNotVisible)]; ok {
		if count, err := strconv.Atoi(val); err == nil {
			status.ApproximateNumberOfMessagesNotVisible = count
		}
	}

	return status, nil
}

// PurgeQueue 清空队列
func (m *Manager) PurgeQueue() error {
	_, err := m.sqsClient.PurgeQueue(context.TODO(), &sqs.PurgeQueueInput{
		QueueUrl: aws.String(m.queueURL),
	})

	if err != nil {
		return fmt.Errorf("清空队列失败: %v", err)
	}

	log.Printf("✅ 队列已清空")
	return nil
}

// RemoveMessageByTaskID 根据任务ID从队列中移除消息
func (m *Manager) RemoveMessageByTaskID(taskID string) (bool, error) {
	// 接收队列中的消息（最多10条）
	result, err := m.sqsClient.ReceiveMessage(context.TODO(), &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(m.queueURL),
		MaxNumberOfMessages: 10,
		WaitTimeSeconds:     0, // 不等待
		VisibilityTimeout:   30, // 30秒可见性超时
		MessageAttributeNames: []string{"All"},
	})

	if err != nil {
		return false, fmt.Errorf("接收消息失败: %v", err)
	}

	// 遍历消息查找匹配的任务ID
	for _, msg := range result.Messages {
		// 尝试从消息属性中获取TaskID
		if attr, ok := msg.MessageAttributes["TaskID"]; ok && attr.StringValue != nil {
			if *attr.StringValue == taskID {
				// 找到匹配的消息，删除它
				if err := m.DeleteMessage(*msg.ReceiptHandle); err != nil {
					return false, err
				}
				log.Printf("✅ 已从队列移除任务: %s", taskID)
				return true, nil
			}
		}

		// 也尝试从消息体中解析TaskID
		queueMessage, err := m.parseMessage(*msg.Body)
		if err == nil && queueMessage.TaskID == taskID {
			if err := m.DeleteMessage(*msg.ReceiptHandle); err != nil {
				return false, err
			}
			log.Printf("✅ 已从队列移除任务: %s", taskID)
			return true, nil
		}
	}

	// 没有找到匹配的消息（可能已被处理或不在当前批次中）
	log.Printf("⚠️  未在队列中找到任务: %s (可能已被处理)", taskID)
	return false, nil
}

// Message 包装的消息结构
type Message struct {
	ReceiptHandle string
	MessageID     string
	QueueMessage  task.QueueMessage
}