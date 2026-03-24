//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"
)

// watchTerminalResizeCh 监听终端大小变化，发送到 channel（供 proxy client 使用）
// 返回 channel 和 stop 函数
func watchTerminalResizeCh(fd int) (<-chan [2]uint16, func()) {
	ch := make(chan [2]uint16, 4)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)

	done := make(chan struct{})
	go func() {
		defer close(ch)
		for {
			select {
			case <-sigCh:
				w, h, err := term.GetSize(fd)
				if err == nil {
					ch <- [2]uint16{uint16(w), uint16(h)}
				}
			case <-done:
				return
			}
		}
	}()

	return ch, func() {
		signal.Stop(sigCh)
		close(done)
	}
}
