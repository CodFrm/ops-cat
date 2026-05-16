package serial_svc

import (
	"io"
	"strings"
	"testing"
	"time"

	"go.bug.st/serial"
)

// TestExtractPortHandleRejectsNil covers the nil-interface guard.
func TestExtractPortHandleRejectsNil(t *testing.T) {
	if _, err := extractPortHandle(nil); err == nil {
		t.Fatalf("expected error for nil port, got nil")
	}
}

func TestExtractPortHandle(t *testing.T) {
	got, err := extractPortHandle(&fakeSerialPort{handle: 42})
	if err != nil {
		t.Fatalf("extract handle: %v", err)
	}
	if got != 42 {
		t.Fatalf("expected handle 42, got %d", got)
	}

	var typedNil *fakeSerialPort
	if _, err := extractPortHandle(typedNil); err == nil || !strings.Contains(err.Error(), "pointer is nil") {
		t.Fatalf("expected typed nil pointer error, got %v", err)
	}

	if _, err := extractPortHandle(&fakeSerialPortNoHandle{}); err == nil || !strings.Contains(err.Error(), "has no `handle` field") {
		t.Fatalf("expected missing handle field error, got %v", err)
	}

	if _, err := extractPortHandle(&fakeSerialPortBadHandle{handle: "bad"}); err == nil || !strings.Contains(err.Error(), "unexpected `handle` field kind") {
		t.Fatalf("expected bad handle kind error, got %v", err)
	}
}

type serialPortStub struct{}

func (*serialPortStub) SetMode(*serial.Mode) error                           { return nil }
func (*serialPortStub) Read([]byte) (int, error)                             { return 0, io.EOF }
func (*serialPortStub) Write(p []byte) (int, error)                          { return len(p), nil }
func (*serialPortStub) Drain() error                                         { return nil }
func (*serialPortStub) ResetInputBuffer() error                              { return nil }
func (*serialPortStub) ResetOutputBuffer() error                             { return nil }
func (*serialPortStub) SetDTR(bool) error                                    { return nil }
func (*serialPortStub) SetRTS(bool) error                                    { return nil }
func (*serialPortStub) GetModemStatusBits() (*serial.ModemStatusBits, error) { return nil, nil }
func (*serialPortStub) SetReadTimeout(time.Duration) error                   { return nil }
func (*serialPortStub) Close() error                                         { return nil }
func (*serialPortStub) Break(time.Duration) error                            { return nil }

type fakeSerialPort struct {
	serialPortStub
	handle uintptr
}

type fakeSerialPortNoHandle struct {
	serialPortStub
}

type fakeSerialPortBadHandle struct {
	serialPortStub
	handle string
}
