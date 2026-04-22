package main

import (
	"context"
	"embed"
	"log"
	"path/filepath"
	"runtime"
	"time"

	"github.com/opskat/opskat/internal/app"
	"github.com/opskat/opskat/internal/bootstrap"
	skillplugin "github.com/opskat/opskat/plugin"

	"github.com/cago-frame/cago/pkg/logger"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
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
	a := app.NewApp(app.SkillContent{
		SkillMD:               skillplugin.SkillMD,
		CommandsMD:            skillplugin.CommandsMD,
		InitMD:                skillplugin.InitMD,
		PluginJSON:            skillplugin.PluginJSON,
		MarketplaceJSON:       skillplugin.MarketplaceJSON,
		PluginMarketplaceJSON: skillplugin.PluginMarketplaceJSON,
	})

	err = wails.Run(&options.App{
		Title:     "OpsKat",
		Width:     1280,
		Height:    800,
		Frameless: runtime.GOOS == "windows",
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: app.NewExtensionAssetHandler(filepath.Join(bootstrap.AppDataDir(), "extensions"), nil),
		},
		OnStartup: a.Startup,
		// OnBeforeClose 在窗口真正关闭前触发：emit ai:flush-all 让前端落盘所有活跃会话，
		// 前端完成后 EventsEmit("ai:flush-done") 回执，后端从 flushAckCh 收到信号立刻放行；
		// 超时 2s 兜底避免前端异常时永久阻塞。返回 false 允许关闭；返回 true 则阻止关闭。
		OnBeforeClose: func(ctx context.Context) bool {
			a.DrainAIFlushAck()
			wailsRuntime.EventsEmit(ctx, "ai:flush-all")
			select {
			case <-a.WaitAIFlushAck():
			case <-time.After(2 * time.Second):
			}
			return false
		},
		OnShutdown: func(ctx context.Context) { a.Cleanup() },
		Bind: []interface{}{
			a,
		},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "com.opskat.desktop",
			OnSecondInstanceLaunch: func(secondInstanceData options.SecondInstanceData) {
				a.OnSecondInstanceLaunch()
			},
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
