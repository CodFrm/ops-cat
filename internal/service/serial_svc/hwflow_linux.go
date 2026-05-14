//go:build linux

package serial_svc

import (
	"fmt"

	"go.bug.st/serial"
	"golang.org/x/sys/unix"
)

// enableHardwareFlowControl 在已打开的串口上把 CRTSCTS（RTS/CTS 硬件流控）位置 1。
// Linux 走 TCGETS / TCSETS。
func enableHardwareFlowControl(port serial.Port) error {
	h, err := extractPortHandle(port)
	if err != nil {
		return err
	}
	fd := int(h)
	termios, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return fmt.Errorf("get termios for hw flow control: %w", err)
	}
	termios.Cflag |= unix.CRTSCTS
	if err := unix.IoctlSetTermios(fd, unix.TCSETS, termios); err != nil {
		return fmt.Errorf("set termios for hw flow control: %w", err)
	}
	return nil
}
