package user

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/golang-jwt/jwt/v5"
)

type Manager struct {
	dynamoClient *dynamodb.Client
	tableName    string
	jwtSecret    []byte
}

func NewManager(dynamoClient *dynamodb.Client, tableName string, jwtSecret string) *Manager {
	return &Manager{
		dynamoClient: dynamoClient,
		tableName:    tableName,
		jwtSecret:    []byte(jwtSecret),
	}
}

// hashPassword 对密码进行哈希
func hashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(hash[:])
}

// GenerateJWTSecret 生成随机JWT密钥
func GenerateJWTSecret() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// InitDefaultAdmin 初始化默认管理员账户
func (m *Manager) InitDefaultAdmin() error {
	// 检查是否已存在admin用户
	_, err := m.GetUser("admin")
	if err == nil {
		log.Println("✅ 默认管理员账户已存在")
		return nil
	}

	// 创建默认admin用户
	admin := &User{
		Username:  "admin",
		Password:  hashPassword("admin"),
		Role:      "admin",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := m.SaveUser(admin); err != nil {
		return fmt.Errorf("创建默认管理员失败: %v", err)
	}

	log.Println("✅ 默认管理员账户创建成功 (admin/admin)")
	return nil
}


// SaveUser 保存用户到DynamoDB
func (m *Manager) SaveUser(user *User) error {
	user.UpdatedAt = time.Now()

	item, err := attributevalue.MarshalMap(user)
	if err != nil {
		return fmt.Errorf("序列化用户失败: %v", err)
	}

	_, err = m.dynamoClient.PutItem(context.TODO(), &dynamodb.PutItemInput{
		TableName: aws.String(m.tableName),
		Item:      item,
	})

	if err != nil {
		return fmt.Errorf("保存用户到DynamoDB失败: %v", err)
	}

	return nil
}

// GetUser 根据用户名获取用户
func (m *Manager) GetUser(username string) (*User, error) {
	result, err := m.dynamoClient.GetItem(context.TODO(), &dynamodb.GetItemInput{
		TableName: aws.String(m.tableName),
		Key: map[string]types.AttributeValue{
			"username": &types.AttributeValueMemberS{Value: username},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("从DynamoDB获取用户失败: %v", err)
	}

	if result.Item == nil {
		return nil, fmt.Errorf("用户不存在: %s", username)
	}

	var user User
	if err := attributevalue.UnmarshalMap(result.Item, &user); err != nil {
		return nil, fmt.Errorf("反序列化用户失败: %v", err)
	}

	return &user, nil
}

// ListUsers 获取所有用户
func (m *Manager) ListUsers() ([]User, error) {
	result, err := m.dynamoClient.Scan(context.TODO(), &dynamodb.ScanInput{
		TableName: aws.String(m.tableName),
	})

	if err != nil {
		return nil, fmt.Errorf("扫描用户表失败: %v", err)
	}

	var users []User
	for _, item := range result.Items {
		var user User
		if err := attributevalue.UnmarshalMap(item, &user); err != nil {
			log.Printf("⚠️ 反序列化用户失败: %v", err)
			continue
		}
		users = append(users, user)
	}

	return users, nil
}

// DeleteUser 删除用户
func (m *Manager) DeleteUser(username string) error {
	// 不允许删除admin用户
	if username == "admin" {
		return fmt.Errorf("不能删除默认管理员账户")
	}

	_, err := m.dynamoClient.DeleteItem(context.TODO(), &dynamodb.DeleteItemInput{
		TableName: aws.String(m.tableName),
		Key: map[string]types.AttributeValue{
			"username": &types.AttributeValueMemberS{Value: username},
		},
	})

	if err != nil {
		return fmt.Errorf("删除用户失败: %v", err)
	}

	return nil
}

// CreateUser 创建新用户
func (m *Manager) CreateUser(username, password, role string) (*User, error) {
	// 检查用户是否已存在
	_, err := m.GetUser(username)
	if err == nil {
		return nil, fmt.Errorf("用户已存在: %s", username)
	}

	if role == "" {
		role = "user"
	}

	user := &User{
		Username:  username,
		Password:  hashPassword(password),
		Role:      role,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := m.SaveUser(user); err != nil {
		return nil, err
	}

	return user, nil
}

// UpdatePassword 更新用户密码
func (m *Manager) UpdatePassword(username, newPassword string) error {
	user, err := m.GetUser(username)
	if err != nil {
		return err
	}

	user.Password = hashPassword(newPassword)
	return m.SaveUser(user)
}

// ValidatePassword 验证密码
func (m *Manager) ValidatePassword(username, password string) (*User, error) {
	user, err := m.GetUser(username)
	if err != nil {
		return nil, fmt.Errorf("用户名或密码错误")
	}

	if user.Password != hashPassword(password) {
		return nil, fmt.Errorf("用户名或密码错误")
	}

	return user, nil
}

// GenerateToken 生成JWT令牌
func (m *Manager) GenerateToken(user *User) (string, error) {
	claims := jwt.MapClaims{
		"username": user.Username,
		"role":     user.Role,
		"exp":      time.Now().Add(24 * time.Hour).Unix(),
		"iat":      time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.jwtSecret)
}

// ValidateToken 验证JWT令牌
func (m *Manager) ValidateToken(tokenString string) (*jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("无效的签名方法")
		}
		return m.jwtSecret, nil
	})

	if err != nil {
		return nil, fmt.Errorf("令牌验证失败: %v", err)
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return &claims, nil
	}

	return nil, fmt.Errorf("无效的令牌")
}
