package main

import (
	"encoding/json"
	"fmt"
	"io"

	opskat "github.com/opskat/extensions/sdk/go/opskat"
)

func init() {
	opskat.RegisterTool("tcp_roundtrip", func(ctx *opskat.ToolContext) (any, error) {
		var args struct {
			Addr string `json:"addr"`
		}
		if err := json.Unmarshal(ctx.Args, &args); err != nil {
			return nil, fmt.Errorf("parse args: %w", err)
		}
		conn, err := opskat.Dial("tcp", args.Addr)
		if err != nil {
			return nil, fmt.Errorf("dial %s: %w", args.Addr, err)
		}
		defer conn.Close()
		if _, err := conn.Write([]byte("ping")); err != nil {
			return nil, fmt.Errorf("write: %w", err)
		}
		buf, err := io.ReadAll(conn)
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("read: %w", err)
		}
		return map[string]string{"received": string(buf)}, nil
	})
}

func main() {
	opskat.Run()
}
