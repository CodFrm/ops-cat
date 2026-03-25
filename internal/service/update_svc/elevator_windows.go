//go:build windows

package update_svc

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type shellExecuteInfo struct {
	cbSize       uint32
	fMask        uint32
	hwnd         uintptr
	lpVerb       uintptr
	lpFile       uintptr
	lpParameters uintptr
	lpDirectory  uintptr
	nShow        int32
	hInstApp     uintptr
	lpIDList     uintptr
	lpClass      uintptr
	hkeyClass    uintptr
	dwHotKey     uint32
	hIcon        uintptr
	hProcess     uintptr
}

func runInstallerElevated(exePath, args string) error {
	verb, _ := syscall.UTF16PtrFromString("runas")
	exe, _ := syscall.UTF16PtrFromString(exePath)
	arg, _ := syscall.UTF16PtrFromString(args)
	cwd, _ := syscall.UTF16PtrFromString("")

	const (
		seeMaskNoCloseProcess = 0x00000040
		swShowNormal          = 1
	)

	sei := shellExecuteInfo{
		fMask:        seeMaskNoCloseProcess,
		lpVerb:       uintptr(unsafe.Pointer(verb)),
		lpFile:       uintptr(unsafe.Pointer(exe)),
		lpParameters: uintptr(unsafe.Pointer(arg)),
		lpDirectory:  uintptr(unsafe.Pointer(cwd)),
		nShow:        swShowNormal,
	}
	sei.cbSize = uint32(unsafe.Sizeof(sei))

	shell32 := windows.NewLazySystemDLL("shell32.dll")
	shellExecuteEx := shell32.NewProc("ShellExecuteExW")

	ret, _, err := shellExecuteEx.Call(uintptr(unsafe.Pointer(&sei)))
	if ret == 0 {
		return fmt.Errorf("ShellExecuteEx failed: %v", err)
	}

	if sei.hProcess != 0 {
		_, _ = windows.WaitForSingleObject(windows.Handle(sei.hProcess), windows.INFINITE)

		var exitCode uint32
		if err := windows.GetExitCodeProcess(windows.Handle(sei.hProcess), &exitCode); err != nil {
			_ = windows.CloseHandle(windows.Handle(sei.hProcess))
			return fmt.Errorf("get exit code failed: %w", err)
		}
		_ = windows.CloseHandle(windows.Handle(sei.hProcess))

		if exitCode != 0 {
			return fmt.Errorf("installer exited with code %d", exitCode)
		}
	}

	return nil
}
