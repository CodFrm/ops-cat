package main

import (
	"context"
	"embed"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"ops-cat/internal/repository/asset_repo"
	"ops-cat/internal/repository/group_repo"
	"ops-cat/internal/repository/ssh_key_repo"
	"ops-cat/internal/service/credential_svc"
	"ops-cat/migrations"

	"github.com/cago-frame/cago"
	"github.com/cago-frame/cago/configs"
	"github.com/cago-frame/cago/database/db"
	"github.com/cago-frame/cago/pkg/logger"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"

	_ "ops-cat/internal/pkg/code"
	_ "github.com/cago-frame/cago/database/db/sqlite"
)

//go:embed all:frontend/dist
var assets embed.FS

// appDataDir 返回应用数据目录
func appDataDir() string {
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "ops-cat")
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "ops-cat")
	default:
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "ops-cat")
	}
}

func main() {
	ctx := context.Background()

	// 确保应用数据目录存在
	dataDir := appDataDir()
	if err := os.MkdirAll(filepath.Join(dataDir, "logs"), 0755); err != nil {
		log.Fatalf("创建数据目录失败: %v", err)
	}

	cfg, err := configs.NewConfig("ops-cat")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 手动初始化日志（路径需要动态指向数据目录）
	zapLogger, err := logger.New(
		logger.Level("info"),
		logger.AppendCore(logger.NewFileCore(logger.ToLevel("info"), filepath.Join(dataDir, "logs", "ops-cat.log"))),
		logger.AppendCore(logger.NewFileCore(logger.ToLevel("error"), filepath.Join(dataDir, "logs", "error.log"))),
	)
	if err != nil {
		log.Fatalf("初始化日志失败: %v", err)
	}
	logger.SetLogger(zapLogger)

	// 初始化数据库（直接引入 db 包，避免 component 包拉入大量未使用依赖）
	cago.New(ctx, cfg).
		Registry(db.Database())

	// 初始化凭证加密服务（masterKey 后续可从配置/密钥链获取）
	credential_svc.SetDefault(credential_svc.New("ops-cat-default-master-key"))

	// 注册 Repository
	asset_repo.RegisterAsset(asset_repo.NewAsset())
	group_repo.RegisterGroup(group_repo.NewGroup())
	ssh_key_repo.RegisterSSHKey(ssh_key_repo.NewSSHKey())

	// 运行数据库迁移
	if err := migrations.RunMigrations(db.Default()); err != nil {
		log.Fatalf("数据库迁移失败: %v", err)
	}

	// 创建 Wails App
	app := NewApp()

	err = wails.Run(&options.App{
		Title:     "Ops Cat",
		Width:     1280,
		Height:    800,
		Frameless: runtime.GOOS == "windows",
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: app.startup,
		Bind: []interface{}{
			app,
		},
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarHiddenInset(),
			WebviewIsTransparent: true,
		},
	})
	if err != nil {
		log.Fatalf("Wails启动失败: %v", err)
	}
}
