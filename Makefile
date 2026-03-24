.PHONY: dev run build build-embed clean install build-cli build-cli-upx install-cli lint

UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Darwin)
    BIN_PATH := ./build/bin/opskat.app/Contents/MacOS/opskat
    UPX_FLAGS := --best --force-macos
else ifeq ($(UNAME_S),Linux)
    BIN_PATH := ./build/bin/opskat
    UPX_FLAGS := --best
else
    BIN_PATH := ./build/bin/opskat.exe
    UPX_FLAGS := --best
endif

VERSION ?= 1.0.0
VERSION_PKG := github.com/cago-frame/cago/configs
LDFLAGS := -s -w -X $(VERSION_PKG).Version=$(VERSION)

# 开发模式（前后端热重载）
dev:
	wails dev

# 直接运行（不热重载）
run: build-embed
	$(BIN_PATH)

# 构建生产版本
build:
	wails build -ldflags="$(LDFLAGS)"

# 构建生产版本（内嵌 opsctl CLI）
build-embed: build-cli-embed
	wails build -ldflags="$(LDFLAGS)" -tags embed_opsctl

# 构建 opsctl 用于嵌入桌面端
build-cli-embed:
	go build -ldflags="$(LDFLAGS)" -o ./internal/embedded/opsctl_bin ./cmd/opsctl/

# 构建生产版本（UPX 压缩，需要安装 upx）
build-upx:
	wails build -ldflags="$(LDFLAGS)"
	upx $(UPX_FLAGS) $(BIN_PATH)

# 安装前端依赖
install:
	cd frontend && pnpm install

# 构建 opsctl CLI
build-cli:
	go build -ldflags="$(LDFLAGS)" -o ./build/bin/opsctl ./cmd/opsctl/

# 构建 opsctl CLI（UPX 压缩）
build-cli-upx: build-cli
	upx $(UPX_FLAGS) ./build/bin/opsctl

# 安装 opsctl 到 GOPATH/bin
install-cli:
	go install -ldflags="$(LDFLAGS)" ./cmd/opsctl/

# 代码检查
lint:
	golangci-lint run --timeout 10m

# 代码检查并自动修复
lint-fix:
	golangci-lint run --timeout 10m --fix

# 清理构建产物
clean:
	rm -rf build/bin frontend/dist internal/embedded/opsctl_bin
