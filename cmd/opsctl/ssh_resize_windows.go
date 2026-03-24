//go:build windows

package main

// watchTerminalResizeCh is a no-op on Windows.
func watchTerminalResizeCh(fd int) (<-chan [2]uint16, func()) {
	return nil, func() {}
}
