package ai

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type recordingNotifier struct {
	resources []string
}

func (r *recordingNotifier) NotifyDataChanged(resource string) {
	r.resources = append(r.resources, resource)
}

func TestNotifyDataChanged(t *testing.T) {
	// 全局单例，记得跑完恢复，避免污染其他测试。
	prev := dataChangeNotifier
	t.Cleanup(func() { dataChangeNotifier = prev })

	t.Run("notifier 未注入时静默忽略", func(t *testing.T) {
		dataChangeNotifier = nil
		assert.NotPanics(t, func() { notifyDataChanged("asset") })
	})

	t.Run("注入后转发 resource 名", func(t *testing.T) {
		rec := &recordingNotifier{}
		SetDataChangeNotifier(rec)
		notifyDataChanged("asset")
		notifyDataChanged("group")
		assert.Equal(t, []string{"asset", "group"}, rec.resources)
	})
}
