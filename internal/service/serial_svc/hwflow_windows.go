//go:build windows

package serial_svc

import (
	"fmt"

	"go.bug.st/serial"
	"golang.org/x/sys/windows"
)

// DCB.Flags 位布局（Win32 docs）：
//
//	bit  2: fOutxCtsFlow
//	bit 12-13: fRtsControl (0=disable, 1=enable, 2=handshake, 3=toggle)
//
// 这里把 OutxCtsFlow 打开 + RtsControl 设为 RTS_CONTROL_HANDSHAKE，
// 等价于 Linux/Unix 的 CRTSCTS。其他位（fBinary、fParity 等）保留 GetCommState
// 读到的原值，避免顺手改坏其它配置。
const (
	dcbOutxCtsFlow         uint32 = 1 << 2
	dcbRtsControlMask      uint32 = 0x3 << 12
	dcbRtsControlHandshake uint32 = 0x2 << 12
)

func enableHardwareFlowControl(port serial.Port) error {
	h, err := extractPortHandle(port)
	if err != nil {
		return err
	}
	handle := windows.Handle(uintptr(h))
	var dcb windows.DCB
	if err := windows.GetCommState(handle, &dcb); err != nil {
		return fmt.Errorf("GetCommState for hw flow control: %w", err)
	}
	dcb.Flags |= dcbOutxCtsFlow
	dcb.Flags = (dcb.Flags &^ dcbRtsControlMask) | dcbRtsControlHandshake
	if err := windows.SetCommState(handle, &dcb); err != nil {
		return fmt.Errorf("SetCommState for hw flow control: %w", err)
	}
	return nil
}
