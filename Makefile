# GPU视频转码系统 Makefile

.PHONY: help build clean deps start-api start-gpu start-all stop-all stop-api stop-gpu status logs

# 默认目标
help:
	@echo "GPU视频转码系统 - 可用命令:"
	@echo ""
	@echo "  build         - 编译所有组件"
	@echo "  deps          - 安装Go依赖"
	@echo "  start-api     - 启动API服务器 (后台，端口9999)"
	@echo "  start-gpu     - 启动GPU处理器 (后台)"
	@echo "  start-all     - 同时启动API服务器和GPU处理器"
	@echo "  stop-all      - 停止所有服务"
	@echo "  stop-api      - 仅停止API服务器"
	@echo "  stop-gpu      - 仅停止GPU处理器"
	@echo "  status        - 查看服务运行状态"
	@echo "  logs          - 查看所有服务日志"
	@echo "  clean         - 清理编译文件"
	@echo "  help          - 显示此帮助信息"
	@echo ""
	@echo "Web管理界面: http://localhost:9999/admin"

# 编译所有组件（本地编译）
build:
	go mod tidy
	mkdir -p bin
	go build -o bin/api-server ./cmd/api-server
	go build -o bin/gpu-processor ./cmd/gpu-processor

# 交叉编译为 Linux（用于部署到 Linux 服务器）
build-linux:
	go mod tidy
	mkdir -p bin
	GOOS=linux GOARCH=amd64 go build -o bin/api-server ./cmd/api-server
	GOOS=linux GOARCH=amd64 go build -o bin/gpu-processor ./cmd/gpu-processor

# 启动API服务器 (端口9999) - 后台运行
start-api:
	@if [ -f config.env ]; then \
		if [ ! -f bin/api-server ]; then \
			echo "二进制文件不存在，先编译..."; \
			$(MAKE) build; \
		fi; \
		echo "启动API服务器 (后台运行)..."; \
		set -a && . ./config.env && set +a && nohup ./bin/api-server > api-server.log 2>&1 & \
		sleep 1; \
		echo "API服务器已启动"; \
		echo "日志文件: api-server.log"; \
		echo "查看日志: tail -f api-server.log"; \
	else \
		echo "错误: config.env 文件不存在，请先复制 config.example.env 到 config.env"; \
		exit 1; \
	fi

# 启动GPU处理器 - 后台运行
start-gpu:
	@if [ -f config.env ]; then \
		if [ ! -f bin/gpu-processor ]; then \
			echo "二进制文件不存在，先编译..."; \
			$(MAKE) build; \
		fi; \
		echo "启动GPU处理器 (后台运行)..."; \
		set -a && . ./config.env && set +a && nohup ./bin/gpu-processor > gpu-processor.log 2>&1 & \
		sleep 1; \
		echo "GPU处理器已启动"; \
		echo "日志文件: gpu-processor.log"; \
		echo "查看日志: tail -f gpu-processor.log"; \
	else \
		echo "错误: config.env 文件不存在，请先复制 config.example.env 到 config.env"; \
		exit 1; \
	fi

# 清理编译文件
clean:
	rm -rf bin/
	go clean

# 安装依赖
deps:
	go mod download
	go mod tidy

# 同时启动所有服务
start-all: start-api start-gpu
	@echo ""
	@echo "所有服务已启动完成！"
	@echo "API服务器: http://localhost:9999"
	@echo "Web管理界面: http://localhost:9999/admin"
	@echo "查看状态: make status"
	@echo "查看日志: make logs"

# 停止所有服务
stop-all:
	@echo "停止所有服务..."
	@-pkill -9 -f "api-server" 2>/dev/null; true
	@-pkill -9 -f "gpu-processor" 2>/dev/null; true
	@sleep 1
	@echo "所有服务已停止"

# 仅停止API服务器
stop-api:
	@echo "停止API服务器..."
	@-pkill -9 -f "api-server" 2>/dev/null; true
	@sleep 1
	@echo "API服务器已停止"

# 仅停止GPU处理器
stop-gpu:
	@echo "停止GPU处理器..."
	@-pkill -9 -f "gpu-processor" 2>/dev/null; true
	@sleep 1
	@echo "GPU处理器已停止"

# 查看服务状态
status:
	@echo "=== 服务运行状态 ==="
	@echo ""
	@echo "API服务器:"
	@-pgrep -f "api-server" > /dev/null 2>&1 && echo "  ✓ 运行中" || echo "  ✗ 未运行"
	@echo ""
	@echo "GPU处理器:"
	@-pgrep -f "gpu-processor" > /dev/null 2>&1 && echo "  ✓ 运行中" || echo "  ✗ 未运行"
	@echo ""
	@echo "端口占用情况:"
	@-lsof -i :9999 2>/dev/null | head -2 || echo "  端口9999未被占用"

# 查看日志
logs:
	@echo "=== 最近的日志 ==="
	@echo ""
	@if [ -f api-server.log ]; then \
		echo "API服务器日志 (最后10行):"; \
		tail -10 api-server.log; \
		echo ""; \
	fi
	@if [ -f gpu-processor.log ]; then \
		echo "GPU处理器日志 (最后10行):"; \
		tail -10 gpu-processor.log; \
		echo ""; \
	fi
	@echo "实时查看日志:"
	@echo "  API服务器: tail -f api-server.log"
	@echo "  GPU处理器: tail -f gpu-processor.log"
