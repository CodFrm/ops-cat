package extension

import (
	"github.com/opskat/opskat/pkg/extruntime"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
)

// Manager type alias.
type Manager = extruntime.Manager

// cagoLoggerAdapter bridges extruntime.Logger to cago/zap logger.
type cagoLoggerAdapter struct{}

func (a *cagoLoggerAdapter) Warn(msg string, keysAndValues ...any) {
	fields := make([]zap.Field, 0, len(keysAndValues)/2)
	for i := 0; i+1 < len(keysAndValues); i += 2 {
		key, _ := keysAndValues[i].(string)
		fields = append(fields, zap.Any(key, keysAndValues[i+1]))
	}
	logger.Default().Warn(msg, fields...)
}

// NewManager creates a Manager with cago logger and app version.
func NewManager(extensionsDir, appVersion string) *Manager {
	return extruntime.NewManager(extensionsDir,
		extruntime.WithLogger(&cagoLoggerAdapter{}),
		extruntime.WithAppVersion(appVersion),
	)
}
