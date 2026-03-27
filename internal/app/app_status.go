package app

import (
	"github.com/opskat/opskat/internal/status"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// GetSystemStatus 返回启动阶段收集的状态条目（前端设置页主动查询）
func (a *App) GetSystemStatus() []status.Entry {
	return status.List()
}

// emitSystemStatus 推送启动状态到前端
func (a *App) emitSystemStatus() {
	entries := status.List()
	if len(entries) > 0 {
		wailsRuntime.EventsEmit(a.ctx, "system:status", entries)
	}
}
