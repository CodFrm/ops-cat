package serial_svc

import (
	"fmt"
	"reflect"

	"go.bug.st/serial"
)

// extractPortHandle 反射拿到 go.bug.st/serial 内部 *unixPort / *windowsPort 的
// unexported `handle` 字段。v1.6.4 自身不暴露硬件流控配置，nativeOpen 又把
// CRTSCTS 显式关掉（serial_unix.go:242），所以只能这样取出 fd / Windows handle，
// 自己走平台 syscall 把 RTS/CTS handshake 打开。
//
// 升级 go.bug.st/serial 时若结构体或字段名变了，会带类型提示返回错误，
// 不会静默失效。
func extractPortHandle(port serial.Port) (uint64, error) {
	if port == nil {
		return 0, fmt.Errorf("serial.Port is nil")
	}
	v := reflect.ValueOf(port)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return 0, fmt.Errorf("serial.Port pointer is nil")
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return 0, fmt.Errorf("unexpected serial.Port underlying kind: %v", v.Kind())
	}
	f := v.FieldByName("handle")
	if !f.IsValid() {
		return 0, fmt.Errorf("serial.Port (%T) has no `handle` field; go.bug.st/serial layout changed", port)
	}
	switch f.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return uint64(f.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return f.Uint(), nil
	default:
		return 0, fmt.Errorf("unexpected `handle` field kind: %v", f.Kind())
	}
}
