package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"enhanced_video_transcoder/internal/user"
)

type AuthHandlers struct {
	userManager *user.Manager
	apiKey      string
}

func NewAuthHandlers(userManager *user.Manager, apiKey string) *AuthHandlers {
	return &AuthHandlers{
		userManager: userManager,
		apiKey:      apiKey,
	}
}

// Login 用户登录
func (h *AuthHandlers) Login(c *gin.Context) {
	var req user.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请求参数错误",
		})
		return
	}

	// 验证用户名密码
	u, err := h.userManager.ValidatePassword(req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "用户名或密码错误",
		})
		return
	}

	// 生成JWT令牌
	token, err := h.userManager.GenerateToken(u)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "生成令牌失败",
		})
		return
	}

	c.JSON(http.StatusOK, user.LoginResponse{
		Token:    token,
		Username: u.Username,
		Role:     u.Role,
	})
}

// GetCurrentUser 获取当前用户信息
func (h *AuthHandlers) GetCurrentUser(c *gin.Context) {
	username, exists := c.Get("username")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "未登录",
		})
		return
	}

	u, err := h.userManager.GetUser(username.(string))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "用户不存在",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"username": u.Username,
		"role":     u.Role,
	})
}

// ListUsers 获取用户列表（仅管理员）
func (h *AuthHandlers) ListUsers(c *gin.Context) {
	users, err := h.userManager.ListUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取用户列表失败",
		})
		return
	}

	c.JSON(http.StatusOK, user.UserListResponse{
		Users: users,
		Total: len(users),
	})
}

// CreateUser 创建用户（仅管理员）
func (h *AuthHandlers) CreateUser(c *gin.Context) {
	var req user.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请求参数错误",
		})
		return
	}

	u, err := h.userManager.CreateUser(req.Username, req.Password, req.Role)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":  "用户创建成功",
		"username": u.Username,
		"role":     u.Role,
	})
}

// DeleteUser 删除用户（仅管理员）
func (h *AuthHandlers) DeleteUser(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "用户名不能为空",
		})
		return
	}

	if err := h.userManager.DeleteUser(username); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "用户删除成功",
	})
}

// UpdatePassword 修改密码
// 两种场景：
// 1. PUT /api/auth/password - 用户修改自己的密码，需要旧密码
// 2. PUT /api/users/:username/password - 管理员修改任意用户密码，不需要旧密码
func (h *AuthHandlers) UpdatePassword(c *gin.Context) {
	var req user.UpdatePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请求参数错误",
		})
		return
	}

	currentUsername, _ := c.Get("username")
	role, _ := c.Get("role")

	// 检查是否是管理员通过用户管理接口修改密码
	targetUsername := c.Param("username")
	if targetUsername != "" {
		// 管理员通过 /api/users/:username/password 修改密码
		if role.(string) != "admin" {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "无权限修改用户密码",
			})
			return
		}
		// 管理员可以修改任何人的密码（包括自己），不需要旧密码
		if err := h.userManager.UpdatePassword(targetUsername, req.NewPassword); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "修改密码失败",
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"message": "密码修改成功",
		})
		return
	}

	// 用户通过 /api/auth/password 修改自己的密码，需要验证旧密码
	if req.OldPassword == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请输入旧密码",
		})
		return
	}
	_, err := h.userManager.ValidatePassword(currentUsername.(string), req.OldPassword)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "旧密码错误",
		})
		return
	}

	if err := h.userManager.UpdatePassword(currentUsername.(string), req.NewPassword); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "修改密码失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "密码修改成功",
	})
}

// AuthMiddleware JWT/API Key 认证中间件
func (h *AuthHandlers) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从Header获取认证信息
		authHeader := c.GetHeader("Authorization")
		apiKey := c.GetHeader("X-API-Key")

		// 优先检查 API Key
		if apiKey != "" {
			if apiKey == h.apiKey {
				// API Key 认证成功，设置为系统用户
				c.Set("username", "api")
				c.Set("role", "admin")
				c.Set("auth_type", "api_key")
				c.Next()
				return
			}
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "API Key 无效",
			})
			c.Abort()
			return
		}

		// 检查 Bearer Token
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "未提供认证令牌，请使用 Authorization: Bearer <token> 或 X-API-Key: <key>",
			})
			c.Abort()
			return
		}

		// 解析Bearer token
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "认证令牌格式错误",
			})
			c.Abort()
			return
		}

		// 验证token
		claims, err := h.userManager.ValidateToken(parts[1])
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "认证令牌无效或已过期",
			})
			c.Abort()
			return
		}

		// 将用户信息存入上下文
		c.Set("username", (*claims)["username"])
		c.Set("role", (*claims)["role"])
		c.Set("auth_type", "jwt")
		c.Next()
	}
}

// AdminMiddleware 管理员权限中间件
func (h *AuthHandlers) AdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists || role.(string) != "admin" {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "需要管理员权限",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
