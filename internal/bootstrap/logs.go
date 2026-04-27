package bootstrap

import (
	"path/filepath"

	"github.com/cago-frame/cago/pkg/logger"
)

// GetLogsDir 返回日志目录路径（依赖 AppDataDir，无需提前初始化）
func GetLogsDir() string {
	return filepath.Join(AppDataDir(), "logs")
}

// InitLogger 根据当前 AppConfig.DebugMode 构建 zap logger 并设为全局实例。
// Debug 模式下 opskat.log 记录 debug+；否则记录 info+。error.log 始终只收 error+。
// 运行时切换 Debug 开关可再次调用本函数热更新。
func InitLogger() error {
	level := "info"
	if cfg := GetConfig(); cfg != nil && cfg.DebugMode {
		level = "debug"
	}
	logsDir := GetLogsDir()
	zapLogger, err := logger.New(
		logger.Level(level),
		logger.AppendCore(logger.NewFileCore(logger.ToLevel(level), filepath.Join(logsDir, "opskat.log"))),
		logger.AppendCore(logger.NewFileCore(logger.ToLevel("error"), filepath.Join(logsDir, "error.log"))),
	)
	if err != nil {
		return err
	}
	logger.SetLogger(zapLogger)
	return nil
}
