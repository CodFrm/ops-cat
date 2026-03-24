package main

import (
	"context"
	"embed"
	"log"
	"path/filepath"
	"runtime"

	"github.com/opskat/opskat/internal/bootstrap"

	"github.com/cago-frame/cago/pkg/logger"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	ctx := context.Background()

	// 初始化数据库、凭证、Repository、迁移
	dataDir := bootstrap.AppDataDir()
	if err := bootstrap.Init(ctx, bootstrap.Options{}); err != nil {
		log.Fatalf("初始化失败: %v", err)
	}

	// 加载应用配置
	if _, err := bootstrap.LoadConfig(dataDir); err != nil {
		log.Printf("加载配置失败: %v", err)
	}

	// 初始化日志（桌面应用需要文件日志）
	logsDir := filepath.Join(dataDir, "logs")
	zapLogger, err := logger.New(
		logger.Level("info"),
		logger.AppendCore(logger.NewFileCore(logger.ToLevel("info"), filepath.Join(logsDir, "opskat.log"))),
		logger.AppendCore(logger.NewFileCore(logger.ToLevel("error"), filepath.Join(logsDir, "error.log"))),
	)
	if err != nil {
		log.Fatalf("初始化日志失败: %v", err)
	}
	logger.SetLogger(zapLogger)

	// 创建 Wails App
	app := NewApp()

	err = wails.Run(&options.App{
		Title:     "OpsKat",
		Width:     1280,
		Height:    800,
		Frameless: runtime.GOOS == "windows",
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.startup,
		OnShutdown: func(ctx context.Context) { app.cleanup() },
		Bind: []interface{}{
			app,
		},
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop:     true,
			DisableWebViewDrop: true,
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
