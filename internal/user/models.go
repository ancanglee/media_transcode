package user

import "time"

// User 用户模型
type User struct {
	Username  string    `json:"username" dynamodbav:"username"`
	Password  string    `json:"-" dynamodbav:"password"` // 密码不返回给前端
	Role      string    `json:"role" dynamodbav:"role"`  // admin, user
	CreatedAt time.Time `json:"created_at" dynamodbav:"created_at"`
	UpdatedAt time.Time `json:"updated_at" dynamodbav:"updated_at"`
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	Token    string `json:"token"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

// CreateUserRequest 创建用户请求
type CreateUserRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Role     string `json:"role"`
}

// UpdatePasswordRequest 修改密码请求
type UpdatePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password" binding:"required"`
}

// UserListResponse 用户列表响应
type UserListResponse struct {
	Users []User `json:"users"`
	Total int    `json:"total"`
}
