.PHONY: dev run build build-embed clean install build-cli install-cli lint test test-cover install-skill devserver build-devserver-ui

UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Darwin)
    BIN_PATH := ./build/bin/opskat.app/Contents/MacOS/opskat
else ifeq ($(UNAME_S),Linux)
    BIN_PATH := ./build/bin/opskat
else
    BIN_PATH := ./build/bin/opskat.exe
endif

VERSION ?= 1.0.0
COMMIT_ID := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
VERSION_PKG := github.com/cago-frame/cago/configs
BUILDINFO_PKG := github.com/opskat/opskat/internal/buildinfo
LDFLAGS := -s -w -X $(VERSION_PKG).Version=$(VERSION) -X $(BUILDINFO_PKG).CommitID=$(COMMIT_ID)

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

# 安装前端依赖
install:
	cd frontend && pnpm install

# 构建 opsctl CLI
build-cli:
	go build -ldflags="$(LDFLAGS)" -o ./build/bin/opsctl ./cmd/opsctl/

# 安装 opsctl 到 GOPATH/bin
install-cli:
	go install -ldflags="$(LDFLAGS)" ./cmd/opsctl/

# 代码检查
lint:
	golangci-lint run --timeout 10m

# 代码检查并自动修复
lint-fix:
	golangci-lint run --timeout 10m --fix

# 运行测试
test:
	go test ./internal/... ./cmd/opsctl/...

# 测试覆盖率（生成 HTML 报告并在浏览器打开）
test-cover:
	go test -coverprofile=coverage.out ./internal/... ./cmd/opsctl/...
	go tool cover -html=coverage.out -o coverage.html
	@echo "覆盖率报告已生成: coverage.html"
	@open coverage.html 2>/dev/null || xdg-open coverage.html 2>/dev/null || echo "请手动打开 coverage.html"

# 构建 DevServer UI 前端
build-devserver-ui:
	cd frontend/packages/devserver-ui && pnpm build
	@touch cmd/devserver/embed.go

# 运行扩展 DevServer（需指定 EXT=扩展名，如 make devserver EXT=oss）
devserver: build-devserver-ui
ifndef EXT
	$(error EXT is required. Usage: make devserver EXT=oss)
endif
	$(MAKE) -C ../extensions build EXT=$(EXT)
	go run ./cmd/devserver/ \
		--ext-dir ../extensions/extensions/$(EXT)/dist \
		--manifest ../extensions/extensions/$(EXT)/manifest.json

# 安装 Claude Code plugin（创建 symlink，注册 marketplace + plugin）
install-skill:
	@# 清理旧路径
	@rm -rf ~/.claude/skills/opsctl ~/.claude/plugins/cache/opskat
	@# Marketplace symlink → 市场根目录（含 .claude-plugin/marketplace.json + opsctl/ 插件目录）
	@rm -rf ~/.claude/plugins/marketplaces/opskat
	@mkdir -p ~/.claude/plugins/marketplaces
	@ln -s $(CURDIR)/plugin ~/.claude/plugins/marketplaces/opskat
	@# 注册到 installed_plugins.json + known_marketplaces.json + settings.json
	@python3 -c "\
	import json, os, datetime; \
	home = os.path.expanduser('~'); \
	now = datetime.datetime.now(datetime.timezone.utc).strftime('%Y-%m-%dT%H:%M:%S.000Z'); \
	plugin_path = os.path.join(home, '.claude/plugins/marketplaces/opskat/opsctl'); \
	mkt_path = os.path.join(home, '.claude/plugins/marketplaces/opskat'); \
	key = 'opsctl@opskat'; \
	pf = os.path.join(home, '.claude/plugins/installed_plugins.json'); \
	cfg = json.load(open(pf)) if os.path.exists(pf) else {'version': 2, 'plugins': {}}; \
	entries = cfg['plugins'].get(key, []); \
	ue = [e for e in entries if e.get('scope') == 'user']; \
	(ue[0].update({'installPath': plugin_path, 'version': 'dev', 'lastUpdated': now}) if ue else \
	 entries.append({'scope': 'user', 'installPath': plugin_path, 'version': 'dev', 'installedAt': now, 'lastUpdated': now})); \
	cfg['plugins'].pop('opsctl@local', None); \
	cfg['plugins'][key] = entries; \
	json.dump(cfg, open(pf, 'w'), indent=2, ensure_ascii=False); \
	kf = os.path.join(home, '.claude/plugins/known_marketplaces.json'); \
	km = json.load(open(kf)) if os.path.exists(kf) else {}; \
	km['opskat'] = {'source': {'source': 'directory', 'path': mkt_path}, 'installLocation': mkt_path, 'lastUpdated': now}; \
	json.dump(km, open(kf, 'w'), indent=2, ensure_ascii=False); \
	sf = os.path.join(home, '.claude/settings.json'); \
	sc = json.load(open(sf)) if os.path.exists(sf) else {}; \
	sc.setdefault('enabledPlugins', {})[key] = True; \
	sc.setdefault('extraKnownMarketplaces', {})['opskat'] = {'source': {'source': 'directory', 'path': mkt_path}}; \
	json.dump(sc, open(sf, 'w'), indent=2, ensure_ascii=False); \
	print(f'Registered plugin: {key}')"
	@echo "Plugin installed: marketplace -> $(CURDIR)/plugin"

# 清理构建产物
clean:
	rm -rf build/bin frontend/dist internal/embedded/opsctl_bin coverage.out coverage.html
