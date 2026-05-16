//go:build e2e

package extension

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"go.uber.org/zap"
)

// TestE2E_TCP_Roundtrip exercises the full TCP-via-WASM path:
// guest calls opskat.Dial → SDK hostIOOpen(tcp) → wazero host import →
// DefaultHostProvider.openTCP dials the real listener → data round-trips
// back through host_io_read/write to the guest.
//
// Requires the fixture WASM at testdata/tcp_e2e_fixture.wasm — build via
// `make test-fixtures`. Skipped by default; run with `go test -tags=e2e`.
func TestE2E_TCP_Roundtrip(t *testing.T) {
	wasmBytes, err := os.ReadFile("testdata/tcp_e2e_fixture.wasm")
	if err != nil {
		t.Skipf("fixture not built (run `make test-fixtures`): %v", err)
	}

	Convey("WASM extension can dial TCP via host_io_open(tcp)", t, func() {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		So(err, ShouldBeNil)
		defer ln.Close()

		done := make(chan struct{})
		go func() {
			defer close(done)
			c, err := ln.Accept()
			if err != nil {
				return
			}
			defer c.Close()
			buf := make([]byte, 1024)
			n, _ := c.Read(buf)
			if _, err := c.Write(append([]byte("pong:"), buf[:n]...)); err != nil {
				return
			}
		}()

		ctx := context.Background()
		host := NewDefaultHostProvider(DefaultHostConfig{Logger: zap.NewNop()})
		defer host.CloseAll()

		manifest := &Manifest{Name: "tcp-e2e", Version: "1.0.0"}
		plugin, err := LoadPlugin(ctx, manifest, wasmBytes, host, nil)
		So(err, ShouldBeNil)
		defer plugin.Close(ctx)

		args, _ := json.Marshal(map[string]string{"addr": ln.Addr().String()})
		result, err := plugin.CallTool(ctx, "tcp_roundtrip", args)
		So(err, ShouldBeNil)

		var out struct {
			Received string `json:"received"`
		}
		So(json.Unmarshal(result, &out), ShouldBeNil)
		So(out.Received, ShouldEqual, "pong:ping")

		<-done
	})
}
