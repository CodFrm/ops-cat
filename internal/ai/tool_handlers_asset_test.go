package ai

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
)

// TestToSafeViewSerial 防止 SerialHandler.SafeView 返回的字段被丢掉。
// 之前 toSafeView 只接 host/port/username/database/redis/k8s 等几个 key，
// 串口资产对 AI 暴露时拿不到 port_path / baud_rate 等关键参数。
func TestToSafeViewSerial(t *testing.T) {
	asset := &asset_entity.Asset{
		ID:   42,
		Name: "console-1",
		Type: asset_entity.AssetTypeSerial,
	}
	require.NoError(t, asset.SetSerialConfig(&asset_entity.SerialConfig{
		PortPath:    "/dev/ttyUSB0",
		BaudRate:    115200,
		DataBits:    8,
		StopBits:    "1",
		Parity:      "none",
		FlowControl: "hardware",
	}))

	v := toSafeView(asset)

	assert.Equal(t, "/dev/ttyUSB0", v.PortPath)
	assert.Equal(t, 115200, v.BaudRate)
	assert.Equal(t, 8, v.DataBits)
	assert.Equal(t, "1", v.StopBits)
	assert.Equal(t, "none", v.Parity)
	assert.Equal(t, "hardware", v.FlowControl)
	// 串口没有 host/port/username 概念，确认没有被错误映射。
	assert.Empty(t, v.Host)
	assert.Equal(t, 0, v.Port)
	assert.Empty(t, v.Username)
}
