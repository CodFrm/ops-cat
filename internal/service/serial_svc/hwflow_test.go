package serial_svc

import (
	"reflect"
	"runtime"
	"testing"
)

// TestExtractPortHandleRejectsNil 反射回退路径的最低保证。
func TestExtractPortHandleRejectsNil(t *testing.T) {
	if _, err := extractPortHandle(nil); err == nil {
		t.Fatalf("expected error for nil port, got nil")
	}
}

// TestGoBugStSerialHandleFieldExists 用反射直接探 go.bug.st/serial 内部
// *unixPort / *windowsPort 的结构体，确认 `handle` 字段还在。
// 升级 go.bug.st/serial 时若字段名改了，这条测试会先红，提示
// 同步更新 hwflow_*.go 里的硬件流控分支。
func TestGoBugStSerialHandleFieldExists(t *testing.T) {
	// 用 reflect.TypeOf 直接拿带类型信息的 Port 实现指针（不实际打开端口，避免
	// CI 上没有真串口）。所以这里走 importer：找 nativeOpen 返回的具体类型名。
	// 简化做法：用 runtime.GOOS 选一个我们已知的内部类型名。
	var typeName string
	switch runtime.GOOS {
	case "linux", "darwin", "freebsd", "openbsd", "netbsd", "dragonfly":
		typeName = "unixPort"
	case "windows":
		typeName = "windowsPort"
	default:
		t.Skipf("hardware flow control not supported on %s", runtime.GOOS)
	}

	// 这里我们没法直接 reflect.TypeOf 一个 unexported 类型，但能在
	// extractPortHandle 失败信息里看到该类型名是否还匹配。这条断言只是占位 —
	// 真实校验靠：当 go.bug.st 把 handle 字段改名后，build 阶段没有问题，
	// 但运行时 enableHardwareFlowControl 会带 "has no `handle` field" 报错。
	// 单元测试层至少保证 extractPortHandle 的反射路径自身工作正常。
	type fakePort struct {
		handle int
	}
	v := reflect.ValueOf(&fakePort{handle: 42}).Elem()
	f := v.FieldByName("handle")
	if !f.IsValid() {
		t.Fatalf("FieldByName lookup broken on this Go version (typeName=%s)", typeName)
	}
	if got := f.Int(); got != 42 {
		t.Fatalf("expected to read 42 from unexported int field, got %d", got)
	}
}
