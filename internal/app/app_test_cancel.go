package app

import "github.com/opskat/opskat/internal/service/testreg"

// CancelTest 取消正在进行的资产「测试连接」操作。
// testID 由前端在调用 Test*Connection 时生成；未匹配到的 ID 静默忽略。
func (a *App) CancelTest(testID string) {
	testreg.Cancel(testID)
}
