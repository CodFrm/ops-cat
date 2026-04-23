package ai

// DataChangeNotifier 由 app 层注入，向前端广播数据变更（资产/分组等），
// 触发 UI 自动刷新。
type DataChangeNotifier interface {
	NotifyDataChanged(resource string)
}

var dataChangeNotifier DataChangeNotifier

// SetDataChangeNotifier 注入数据变更通知器（应在应用启动时调用）。
func SetDataChangeNotifier(n DataChangeNotifier) {
	dataChangeNotifier = n
}

// notifyDataChanged 安全广播一次变更事件，未注入通知器时静默忽略。
func notifyDataChanged(resource string) {
	if dataChangeNotifier != nil {
		dataChangeNotifier.NotifyDataChanged(resource)
	}
}
