package task

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

type Manager struct {
	dynamoClient *dynamodb.Client
	tableName    string
}

func NewManager(dynamoClient *dynamodb.Client, tableName string) *Manager {
	return &Manager{
		dynamoClient: dynamoClient,
		tableName:    tableName,
	}
}

// CreateTask 创建新任务
func (m *Manager) CreateTask(inputBucket, inputKey, outputBucket string, transcodeTypes []string) (*TranscodeTask, error) {
	return m.CreateTaskWithID(uuid.New().String(), inputBucket, inputKey, outputBucket, transcodeTypes)
}

// CreateTaskWithID 使用指定ID创建任务
func (m *Manager) CreateTaskWithID(taskID, inputBucket, inputKey, outputBucket string, transcodeTypes []string) (*TranscodeTask, error) {
	now := time.Now()
	task := &TranscodeTask{
		TaskID:         taskID,
		DatePartition:  now.Format("2006-01-02"), // 日期分区键
		InputBucket:    inputBucket,
		InputKey:       inputKey,
		OutputBucket:   outputBucket,
		TranscodeTypes: transcodeTypes,
		Status:         TaskStatusPending,
		CreatedAt:      now,
		UpdatedAt:      now,
		RetryCount:     0,
		MaxRetries:     3,
		Progress:       make(map[string]string),
		OutputFiles:    make(map[string]string),
	}

	// 初始化进度
	for _, transcodeType := range transcodeTypes {
		task.Progress[transcodeType] = "pending"
	}

	if err := m.SaveTask(task); err != nil {
		return nil, fmt.Errorf("保存任务失败: %v", err)
	}

	log.Printf("✅ 创建任务成功: %s", task.TaskID)
	return task, nil
}

// SaveTask 保存任务到DynamoDB
func (m *Manager) SaveTask(task *TranscodeTask) error {
	task.UpdatedAt = time.Now()

	item, err := attributevalue.MarshalMap(task)
	if err != nil {
		return fmt.Errorf("序列化任务失败: %v", err)
	}

	_, err = m.dynamoClient.PutItem(context.TODO(), &dynamodb.PutItemInput{
		TableName: aws.String(m.tableName),
		Item:      item,
	})

	if err != nil {
		return fmt.Errorf("保存任务到DynamoDB失败: %v", err)
	}

	return nil
}

// GetTask 根据ID获取任务
func (m *Manager) GetTask(taskID string) (*TranscodeTask, error) {
	result, err := m.dynamoClient.GetItem(context.TODO(), &dynamodb.GetItemInput{
		TableName: aws.String(m.tableName),
		Key: map[string]types.AttributeValue{
			"task_id": &types.AttributeValueMemberS{Value: taskID},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("从DynamoDB获取任务失败: %v", err)
	}

	if result.Item == nil {
		return nil, fmt.Errorf("任务不存在: %s", taskID)
	}

	var task TranscodeTask
	if err := attributevalue.UnmarshalMap(result.Item, &task); err != nil {
		return nil, fmt.Errorf("反序列化任务失败: %v", err)
	}

	return &task, nil
}

// ListTasks 获取任务列表
// 优先使用 GSI 查询：有日期用 date-index，有状态用 status-index，都没有用 Scan
func (m *Manager) ListTasks(status, date string, limit, offset int) ([]TranscodeTask, int, error) {
	log.Printf("📋 ListTasks 请求: status=%q, date=%q, limit=%d, offset=%d", status, date, limit, offset)
	
	// 先获取总数
	total, err := m.countTasks(status, date)
	if err != nil {
		log.Printf("❌ countTasks 失败: %v", err)
		return nil, 0, err
	}
	log.Printf("📊 countTasks 返回: total=%d", total)

	// 如果 offset 超出范围，直接返回空
	if offset >= total {
		return []TranscodeTask{}, total, nil
	}

	// 获取分页数据
	var tasks []TranscodeTask
	if date != "" {
		// 有日期，使用 date-index GSI
		tasks, err = m.fetchTasksByDate(status, date, limit, offset)
	} else if status != "" {
		// 有状态但没日期，使用 status-index GSI（更高效）
		tasks, err = m.fetchTasksByStatusIndex(status, limit, offset)
	} else {
		// 都没有，使用 Scan
		tasks, err = m.fetchTasksByScan(status, limit, offset)
	}

	if err != nil {
		return nil, 0, err
	}

	return tasks, total, nil
}

// countTasks 统计任务数量（使用 SELECT COUNT 高效统计）
func (m *Manager) countTasks(status, date string) (int, error) {
	if date != "" {
		// 有日期，使用 date-index GSI
		return m.countTasksByDate(status, date)
	}
	if status != "" {
		// 有状态但没日期，使用 status-index GSI（更高效）
		return m.countTasksByStatusIndex(status)
	}
	// 都没有，使用 Scan
	return m.countTasksByScan(status)
}

// countTasksByDate 按日期统计任务数量
func (m *Manager) countTasksByDate(status, date string) (int, error) {
	queryInput := &dynamodb.QueryInput{
		TableName:              aws.String(m.tableName),
		IndexName:              aws.String("date-index"),
		KeyConditionExpression: aws.String("date_partition = :date"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":date": &types.AttributeValueMemberS{Value: date},
		},
		Select: types.SelectCount,
	}

	if status != "" {
		queryInput.FilterExpression = aws.String("#status = :status")
		queryInput.ExpressionAttributeNames = map[string]string{
			"#status": "status",
		}
		queryInput.ExpressionAttributeValues[":status"] = &types.AttributeValueMemberS{Value: status}
	}

	var total int
	var lastEvaluatedKey map[string]types.AttributeValue
	for {
		if lastEvaluatedKey != nil {
			queryInput.ExclusiveStartKey = lastEvaluatedKey
		}

		result, err := m.dynamoClient.Query(context.TODO(), queryInput)
		if err != nil {
			return 0, fmt.Errorf("统计任务失败: %v", err)
		}

		total += int(result.Count)

		if result.LastEvaluatedKey == nil {
			break
		}
		lastEvaluatedKey = result.LastEvaluatedKey
	}

	return total, nil
}

// countTasksByScan 使用 Scan 统计任务数量
func (m *Manager) countTasksByScan(status string) (int, error) {
	scanInput := &dynamodb.ScanInput{
		TableName: aws.String(m.tableName),
		Select:    types.SelectCount,
	}

	if status != "" {
		scanInput.FilterExpression = aws.String("#status = :status")
		scanInput.ExpressionAttributeNames = map[string]string{
			"#status": "status",
		}
		scanInput.ExpressionAttributeValues = map[string]types.AttributeValue{
			":status": &types.AttributeValueMemberS{Value: status},
		}
	}

	var total int
	var lastEvaluatedKey map[string]types.AttributeValue
	for {
		if lastEvaluatedKey != nil {
			scanInput.ExclusiveStartKey = lastEvaluatedKey
		}

		result, err := m.dynamoClient.Scan(context.TODO(), scanInput)
		if err != nil {
			return 0, fmt.Errorf("统计任务失败: %v", err)
		}

		total += int(result.Count)
		log.Printf("📊 Scan 统计 [status=%s]: Count=%d, ScannedCount=%d", status, result.Count, result.ScannedCount)

		if result.LastEvaluatedKey == nil {
			break
		}
		lastEvaluatedKey = result.LastEvaluatedKey
	}

	log.Printf("📊 Scan 统计状态 [%s] 任务总数: %d", status, total)
	return total, nil
}

// fetchTasksByDate 按日期获取任务列表（带分页）
func (m *Manager) fetchTasksByDate(status, date string, limit, offset int) ([]TranscodeTask, error) {
	var tasks []TranscodeTask

	queryInput := &dynamodb.QueryInput{
		TableName:              aws.String(m.tableName),
		IndexName:              aws.String("date-index"),
		KeyConditionExpression: aws.String("date_partition = :date"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":date": &types.AttributeValueMemberS{Value: date},
		},
		ScanIndexForward: aws.Bool(false),
	}

	if status != "" {
		queryInput.FilterExpression = aws.String("#status = :status")
		queryInput.ExpressionAttributeNames = map[string]string{
			"#status": "status",
		}
		queryInput.ExpressionAttributeValues[":status"] = &types.AttributeValueMemberS{Value: status}
	}

	// 跳过 offset 条记录，获取 limit 条
	skipped := 0
	collected := 0
	var lastEvaluatedKey map[string]types.AttributeValue

	for collected < limit {
		if lastEvaluatedKey != nil {
			queryInput.ExclusiveStartKey = lastEvaluatedKey
		}

		result, err := m.dynamoClient.Query(context.TODO(), queryInput)
		if err != nil {
			return nil, fmt.Errorf("查询任务失败: %v", err)
		}

		for _, item := range result.Items {
			if skipped < offset {
				skipped++
				continue
			}

			if collected >= limit {
				break
			}

			var task TranscodeTask
			if err := attributevalue.UnmarshalMap(item, &task); err != nil {
				log.Printf("⚠️  反序列化任务失败: %v", err)
				continue
			}
			tasks = append(tasks, task)
			collected++
		}

		if result.LastEvaluatedKey == nil || collected >= limit {
			break
		}
		lastEvaluatedKey = result.LastEvaluatedKey
	}

	return tasks, nil
}

// fetchTasksByScan 使用 Scan 获取任务列表（带分页）
func (m *Manager) fetchTasksByScan(status string, limit, offset int) ([]TranscodeTask, error) {
	var allTasks []TranscodeTask

	scanInput := &dynamodb.ScanInput{
		TableName: aws.String(m.tableName),
	}

	if status != "" {
		scanInput.FilterExpression = aws.String("#status = :status")
		scanInput.ExpressionAttributeNames = map[string]string{
			"#status": "status",
		}
		scanInput.ExpressionAttributeValues = map[string]types.AttributeValue{
			":status": &types.AttributeValueMemberS{Value: status},
		}
	}

	// Scan 无法保证顺序，需要获取足够数据后排序
	needTotal := offset + limit
	var lastEvaluatedKey map[string]types.AttributeValue

	for len(allTasks) < needTotal {
		if lastEvaluatedKey != nil {
			scanInput.ExclusiveStartKey = lastEvaluatedKey
		}

		result, err := m.dynamoClient.Scan(context.TODO(), scanInput)
		if err != nil {
			return nil, fmt.Errorf("扫描任务失败: %v", err)
		}

		for _, item := range result.Items {
			var task TranscodeTask
			if err := attributevalue.UnmarshalMap(item, &task); err != nil {
				log.Printf("⚠️  反序列化任务失败: %v", err)
				continue
			}
			allTasks = append(allTasks, task)
		}

		if result.LastEvaluatedKey == nil {
			break
		}
		lastEvaluatedKey = result.LastEvaluatedKey
	}

	// 按创建时间倒序排序
	sortTasksByCreatedAtDesc(allTasks)

	// 应用分页
	if offset >= len(allTasks) {
		return []TranscodeTask{}, nil
	}

	end := offset + limit
	if end > len(allTasks) {
		end = len(allTasks)
	}

	return allTasks[offset:end], nil
}

// sortTasksByCreatedAtDesc 按创建时间倒序排序
func sortTasksByCreatedAtDesc(tasks []TranscodeTask) {
	for i := 0; i < len(tasks)-1; i++ {
		for j := i + 1; j < len(tasks); j++ {
			if tasks[i].CreatedAt.Before(tasks[j].CreatedAt) {
				tasks[i], tasks[j] = tasks[j], tasks[i]
			}
		}
	}
}

// ListTasksByStatus 使用 status-index GSI 按状态查询任务
func (m *Manager) ListTasksByStatus(status string, limit, offset int) ([]TranscodeTask, int, error) {
	// 先统计总数
	total, err := m.countTasksByStatusIndex(status)
	if err != nil {
		return nil, 0, err
	}

	if offset >= total {
		return []TranscodeTask{}, total, nil
	}

	// 获取分页数据
	tasks, err := m.fetchTasksByStatusIndex(status, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	return tasks, total, nil
}

// countTasksByStatusIndex 使用 status-index GSI 统计任务数量
func (m *Manager) countTasksByStatusIndex(status string) (int, error) {
	queryInput := &dynamodb.QueryInput{
		TableName:              aws.String(m.tableName),
		IndexName:              aws.String("status-index"),
		KeyConditionExpression: aws.String("#status = :status"),
		ExpressionAttributeNames: map[string]string{
			"#status": "status",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":status": &types.AttributeValueMemberS{Value: status},
		},
		Select: types.SelectCount,
	}

	var total int
	var lastEvaluatedKey map[string]types.AttributeValue
	for {
		if lastEvaluatedKey != nil {
			queryInput.ExclusiveStartKey = lastEvaluatedKey
		}

		result, err := m.dynamoClient.Query(context.TODO(), queryInput)
		if err != nil {
			log.Printf("⚠️  status-index GSI 查询失败，回退到 Scan: %v", err)
			// GSI 可能不存在，回退到 Scan
			return m.countTasksByScan(status)
		}

		total += int(result.Count)

		if result.LastEvaluatedKey == nil {
			break
		}
		lastEvaluatedKey = result.LastEvaluatedKey
	}

	log.Printf("📊 统计状态 [%s] 任务数量: %d", status, total)
	return total, nil
}

// fetchTasksByStatusIndex 使用 status-index GSI 获取任务列表
func (m *Manager) fetchTasksByStatusIndex(status string, limit, offset int) ([]TranscodeTask, error) {
	var tasks []TranscodeTask

	queryInput := &dynamodb.QueryInput{
		TableName:              aws.String(m.tableName),
		IndexName:              aws.String("status-index"),
		KeyConditionExpression: aws.String("#status = :status"),
		ExpressionAttributeNames: map[string]string{
			"#status": "status",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":status": &types.AttributeValueMemberS{Value: status},
		},
		ScanIndexForward: aws.Bool(false),
	}

	skipped := 0
	collected := 0
	var lastEvaluatedKey map[string]types.AttributeValue

	for collected < limit {
		if lastEvaluatedKey != nil {
			queryInput.ExclusiveStartKey = lastEvaluatedKey
		}

		result, err := m.dynamoClient.Query(context.TODO(), queryInput)
		if err != nil {
			return nil, fmt.Errorf("查询任务失败: %v", err)
		}

		for _, item := range result.Items {
			if skipped < offset {
				skipped++
				continue
			}

			if collected >= limit {
				break
			}

			var task TranscodeTask
			if err := attributevalue.UnmarshalMap(item, &task); err != nil {
				log.Printf("⚠️  反序列化任务失败: %v", err)
				continue
			}
			tasks = append(tasks, task)
			collected++
		}

		if result.LastEvaluatedKey == nil || collected >= limit {
			break
		}
		lastEvaluatedKey = result.LastEvaluatedKey
	}

	return tasks, nil
}

// UpdateTaskStatus 更新任务状态
func (m *Manager) UpdateTaskStatus(taskID string, status TaskStatus, errorMessage string) error {
	task, err := m.GetTask(taskID)
	if err != nil {
		return err
	}

	task.Status = status
	task.UpdatedAt = time.Now()

	if status == TaskStatusProcessing && task.StartedAt == nil {
		now := time.Now()
		task.StartedAt = &now
	}

	if status == TaskStatusCompleted || status == TaskStatusFailed {
		now := time.Now()
		task.CompletedAt = &now
	}

	if errorMessage != "" {
		task.ErrorMessage = errorMessage
	}

	return m.SaveTask(task)
}

// UpdateTaskProgress 更新任务进度
func (m *Manager) UpdateTaskProgress(taskID, transcodeType, progress string) error {
	task, err := m.GetTask(taskID)
	if err != nil {
		return err
	}

	task.Progress[transcodeType] = progress
	return m.SaveTask(task)
}

// AddOutputFile 添加输出文件
func (m *Manager) AddOutputFile(taskID, transcodeType, outputKey string) error {
	task, err := m.GetTask(taskID)
	if err != nil {
		return err
	}

	task.OutputFiles[transcodeType] = outputKey
	return m.SaveTask(task)
}

// RetryTask 重试任务（支持任意状态的任务）
func (m *Manager) RetryTask(taskID string) error {
	task, err := m.GetTask(taskID)
	if err != nil {
		return err
	}

	// 如果任务正在处理中，不允许重试
	if task.Status == TaskStatusProcessing {
		return fmt.Errorf("任务正在处理中，无法重试")
	}

	task.RetryCount++
	task.Status = TaskStatusRetrying
	task.ErrorMessage = ""
	task.ErrorDetails = nil // 清空错误详情
	task.UpdatedAt = time.Now()
	task.StartedAt = nil
	task.CompletedAt = nil

	// 重置进度
	for transcodeType := range task.Progress {
		task.Progress[transcodeType] = "pending"
	}

	// 重置输出文件
	task.OutputFiles = make(map[string]string)

	return m.SaveTask(task)
}

// AddErrorDetail 添加错误详情
func (m *Manager) AddErrorDetail(taskID string, detail ErrorDetail) error {
	task, err := m.GetTask(taskID)
	if err != nil {
		return err
	}

	detail.Timestamp = time.Now()

	// 限制输出日志长度，避免 DynamoDB 存储过大
	if len(detail.Output) > 5000 {
		detail.Output = detail.Output[:5000] + "\n... [日志已截断]"
	}
	if len(detail.Command) > 1000 {
		detail.Command = detail.Command[:1000] + "... [命令已截断]"
	}

	task.ErrorDetails = append(task.ErrorDetails, detail)
	return m.SaveTask(task)
}

// IsTaskAborted 检查任务是否被中止（状态变为 failed 且错误信息包含"中止"）
func (m *Manager) IsTaskAborted(taskID string) bool {
	task, err := m.GetTask(taskID)
	if err != nil {
		return false
	}
	// 如果任务状态不是 processing，说明被中止或已完成
	return task.Status != TaskStatusProcessing
}

// MarkIncompleteProgressAsFailed 将未完成的转码类型状态设置为 failed
func (m *Manager) MarkIncompleteProgressAsFailed(taskID string) error {
	task, err := m.GetTask(taskID)
	if err != nil {
		return err
	}

	// 遍历所有转码类型，将非 completed 状态的设置为 failed
	for transcodeType, status := range task.Progress {
		if status != "completed" {
			task.Progress[transcodeType] = "failed"
		}
	}

	return m.SaveTask(task)
}