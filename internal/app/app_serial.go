package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/service/asset_svc"
	"github.com/opskat/opskat/internal/service/serial_svc"
	"github.com/opskat/opskat/internal/service/testreg"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// SerialConnectRequest 前端串口连接请求
type SerialConnectRequest struct {
	AssetID int64 `json:"assetId"`
}

// SerialConnectEvent 串口异步连接事件
type SerialConnectEvent struct {
	Type      string `json:"type"`                // "progress" | "connected" | "error"
	Step      string `json:"step,omitempty"`      // "open"
	Message   string `json:"message,omitempty"`   // 进度消息
	SessionID string `json:"sessionId,omitempty"` // type=connected 时返回的会话ID
	Error     string `json:"error,omitempty"`     // type=error 时的错误信息
}

// ListSerialPorts 列出系统可用串口
func (a *App) ListSerialPorts() ([]serial_svc.SerialPortInfo, error) {
	return a.serialManager.ListPorts()
}

// ConnectSerialAsync 异步打开串口连接，立即返回 connectionId
func (a *App) ConnectSerialAsync(req SerialConnectRequest) (string, error) {
	asset, err := asset_svc.Asset().Get(a.langCtx(), req.AssetID)
	if err != nil {
		return "", fmt.Errorf("%s: %w", a.pickMsg("资产不存在", "asset not found"), err)
	}
	if !asset.IsSerial() {
		return "", fmt.Errorf("%s", a.pickMsg("资产不是串口类型", "asset is not a serial type"))
	}

	serialCfg, err := asset.GetSerialConfig()
	if err != nil {
		return "", fmt.Errorf("%s: %w", a.pickMsg("解析串口配置失败", "parse serial config failed"), err)
	}

	// 生成 connectionId
	a.mu.Lock()
	a.connCounter++
	connectionId := fmt.Sprintf("conn-%d", a.connCounter)
	a.mu.Unlock()

	connCtx, cancel := context.WithCancel(a.ctx)
	a.pendingConnections.Store(connectionId, cancel)

	eventName := "serial:connect:" + connectionId

	emitEvent := func(event SerialConnectEvent) {
		wailsRuntime.EventsEmit(a.ctx, eventName, event)
	}

	go func() {
		defer a.pendingConnections.Delete(connectionId)

		if connCtx.Err() != nil {
			return
		}
		emitEvent(SerialConnectEvent{Type: "progress", Step: "open", Message: a.pickMsg("正在打开串口...", "Opening serial port...")})

		sessionID, err := a.serialManager.Connect(serial_svc.ConnectConfig{
			PortPath:    serialCfg.PortPath,
			BaudRate:    serialCfg.BaudRate,
			DataBits:    serialCfg.DataBits,
			StopBits:    serialCfg.StopBits,
			Parity:      serialCfg.Parity,
			FlowControl: serialCfg.FlowControl,
			AssetID:     req.AssetID,
		})
		if err != nil {
			emitEvent(SerialConnectEvent{Type: "error", Error: err.Error()})
			return
		}
		if connCtx.Err() != nil {
			a.serialManager.Disconnect(sessionID)
			return
		}

		// 设置回调（sessionID 已知）
		a.serialManager.SetCallbacks(
			sessionID,
			func(data []byte) {
				wailsRuntime.EventsEmit(a.ctx, "serial:data:"+sessionID, base64.StdEncoding.EncodeToString(data))
			},
			func(sid string) {
				wailsRuntime.EventsEmit(a.ctx, "serial:closed:"+sid, nil)
			},
		)

		emitEvent(SerialConnectEvent{Type: "connected", SessionID: sessionID})
	}()

	return connectionId, nil
}

// TestSerialConnection 测试串口连接（打开后立即关闭）
// testID: 前端生成的本次测试唯一标识，用于配合 CancelTest 中断。
func (a *App) TestSerialConnection(testID string, configJSON string) error {
	var cfg asset_entity.SerialConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return fmt.Errorf("%s: %w", a.pickMsg("配置解析失败", "parse config failed"), err)
	}

	parent, parentCancel := context.WithTimeout(a.langCtx(), 10*time.Second)
	defer parentCancel()
	ctx, release := testreg.Begin(parent, testID)
	defer release()

	return a.serialManager.TestConnection(ctx, serial_svc.ConnectConfig{
		PortPath:    cfg.PortPath,
		BaudRate:    cfg.BaudRate,
		DataBits:    cfg.DataBits,
		StopBits:    cfg.StopBits,
		Parity:      cfg.Parity,
		FlowControl: cfg.FlowControl,
	})
}

// WriteSerial 向串口终端写入数据（base64 编码）
func (a *App) WriteSerial(sessionID string, dataB64 string) error {
	sess, ok := a.serialManager.GetSession(sessionID)
	if !ok {
		return fmt.Errorf("%s: %s", a.pickMsg("串口会话不存在", "serial session not found"), sessionID)
	}

	data, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		return fmt.Errorf("%s: %w", a.pickMsg("解码数据失败", "decode data failed"), err)
	}

	return sess.Write(data)
}

// DisconnectSerial 断开串口连接
func (a *App) DisconnectSerial(sessionID string) {
	a.serialManager.Disconnect(sessionID)
}

// ResizeSerialTerminal 调整串口终端尺寸（当前为 no-op，仅保持前后端接口一致）。
func (a *App) ResizeSerialTerminal(sessionID string, cols int, rows int) error {
	sess, ok := a.serialManager.GetSession(sessionID)
	if !ok {
		return fmt.Errorf("%s: %s", a.pickMsg("串口会话不存在", "serial session not found"), sessionID)
	}

	return sess.Resize(cols, rows)
}
