//go:build darwin || freebsd || openbsd || netbsd || dragonfly

package serial_svc

import (
	"fmt"

	"go.bug.st/serial"
	"golang.org/x/sys/unix"
)

// enableHardwareFlowControl 在已打开的串口上把 CRTSCTS（RTS/CTS 硬件流控）位置 1。
// macOS / *BSD 走 TIOCGETA / TIOCSETA；CRTSCTS 在 Darwin 是
// CCTS_OFLOW | CRTS_IFLOW，BSD 是 CCTS_OFLOW，golang.org/x/sys/unix
// 已经按平台 alias 成 unix.CRTSCTS。
func enableHardwareFlowControl(port serial.Port) error {
	h, err := extractPortHandle(port)
	if err != nil {
		return err
	}
	fd := int(h)
	termios, err := unix.IoctlGetTermios(fd, unix.TIOCGETA)
	if err != nil {
		return fmt.Errorf("get termios for hw flow control: %w", err)
	}
	termios.Cflag |= unix.CRTSCTS
	if err := unix.IoctlSetTermios(fd, unix.TIOCSETA, termios); err != nil {
		return fmt.Errorf("set termios for hw flow control: %w", err)
	}
	return nil
}
