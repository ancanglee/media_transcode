# GPU视频转码系统 Makefile
# 支持 macOS (Apple Silicon VideoToolbox) 和 Linux (NVIDIA NVENC)

.PHONY: help build build-linux build-macos clean deps start-api start-gpu start-all stop-all stop-api stop-gpu status logs check-platform

# 默认目标
help:
	@echo "GPU视频转码系统 - 可用命令:"
	@echo ""
	@echo "  build         - 编译所有组件 (当前平台)"
	@echo "  build-linux   - 交叉编译为 Linux (用于 NVIDIA GPU 服务器)"
	@echo "  build-macos   - 编译为 macOS (用于 Apple Silicon)"
	@echo "  deps          - 安装Go依赖"
	@echo "  start-api     - 启动API服务器 (后台，端口9999)"
	@echo "  start-gpu     - 启动GPU处理器 (后台)"
	@echo "  start-all     - 同时启动API服务器和GPU处理器"
	@echo "  stop-all      - 停止所有服务"
	@echo "  stop-api      - 仅停止API服务器"
	@echo "  stop-gpu      - 仅停止GPU处理器"
	@echo "  status        - 查看服务运行状态"
	@echo "  logs          - 查看所有服务日志"
	@echo "  check-platform- 检测当前平台和硬件加速支持"
	@echo "  clean         - 清理编译文件"
	@echo "  help          - 显示此帮助信息"
	@echo ""
	@echo "Web管理界面: http://localhost:9999/admin"
	@echo ""
	@echo "支持的平台:"
	@echo "  - macOS (Apple Silicon): 使用 VideoToolbox 硬件加速"
	@echo "  - Linux (NVIDIA GPU): 使用 NVENC 硬件加速"
	@echo "  - 其他: 使用 CPU 软件编码"

# 编译所有组件（本地编译）
build:
	go mod tidy
	mkdir -p bin
	go build -o bin/api-server ./cmd/api-server
	go build -o bin/gpu-processor ./cmd/gpu-processor

# 交叉编译为 Linux（用于部署到 Linux NVIDIA GPU 服务器）
build-linux:
	go mod tidy
	mkdir -p bin
	GOOS=linux GOARCH=amd64 go build -o bin/api-server-linux ./cmd/api-server
	GOOS=linux GOARCH=amd64 go build -o bin/gpu-processor-linux ./cmd/gpu-processor
	@echo "Linux 二进制文件已生成: bin/api-server-linux, bin/gpu-processor-linux"

# 编译为 macOS (Apple Silicon)
build-macos:
	go mod tidy
	mkdir -p bin
	GOOS=darwin GOARCH=arm64 go build -o bin/api-server-macos ./cmd/api-server
	GOOS=darwin GOARCH=arm64 go build -o bin/gpu-processor-macos ./cmd/gpu-processor
	@echo "macOS 二进制文件已生成: bin/api-server-macos, bin/gpu-processor-macos"

# 检测当前平台和硬件加速支持
check-platform:
	@echo "=== 平台检测 ==="
	@echo ""
	@echo "操作系统: $$(uname -s)"
	@echo "架构: $$(uname -m)"
	@echo ""
	@echo "FFmpeg 版本:"
	@ffmpeg -version 2>/dev/null | head -1 || echo "  FFmpeg 未安装"
	@echo ""
	@echo "硬件加速支持:"
	@if [ "$$(uname -s)" = "Darwin" ]; then \
		echo "  检测 VideoToolbox..."; \
		ffmpeg -encoders 2>/dev/null | grep -q "hevc_videotoolbox" && echo "  ✓ hevc_videotoolbox 可用" || echo "  ✗ hevc_videotoolbox 不可用"; \
		ffmpeg -encoders 2>/dev/null | grep -q "h264_videotoolbox" && echo "  ✓ h264_videotoolbox 可用" || echo "  ✗ h264_videotoolbox 不可用"; \
	else \
		echo "  检测 NVIDIA NVENC..."; \
		nvidia-smi 2>/dev/null | head -1 || echo "  ✗ NVIDIA GPU 不可用"; \
		ffmpeg -encoders 2>/dev/null | grep -q "hevc_nvenc" && echo "  ✓ hevc_nvenc 可用" || echo "  ✗ hevc_nvenc 不可用"; \
		ffmpeg -encoders 2>/dev/null | grep -q "h264_nvenc" && echo "  ✓ h264_nvenc 可用" || echo "  ✗ h264_nvenc 不可用"; \
	fi

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
