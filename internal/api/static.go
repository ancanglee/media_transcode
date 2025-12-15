package api

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed web/*
var webFS embed.FS

// SetupStaticRoutes 设置静态文件路由
func SetupStaticRoutes(router *gin.Engine) {
	// 获取 web 子目录
	webContent, err := fs.Sub(webFS, "web")
	if err != nil {
		panic("无法加载嵌入的 web 文件: " + err.Error())
	}

	// 静态文件服务（禁用缓存以便开发调试）
	router.GET("/static/*filepath", func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		c.FileFromFS(c.Param("filepath"), http.FS(webContent))
	})

	// 根路径重定向到管理界面
	router.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/admin")
	})

	// 登录页面
	router.GET("/login", func(c *gin.Context) {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		data, err := webFS.ReadFile("web/login.html")
		if err != nil {
			c.String(http.StatusInternalServerError, "无法加载登录页面")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	})

	// 管理界面
	router.GET("/admin", func(c *gin.Context) {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		data, err := webFS.ReadFile("web/index.html")
		if err != nil {
			c.String(http.StatusInternalServerError, "无法加载管理界面")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	})
}
