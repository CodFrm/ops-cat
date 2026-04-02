// pkg/extension/hostfn.go
package extension

import (
	"context"
	"encoding/json"

	"github.com/tetratelabs/wazero/api"
)

// readGuestString reads a string from guest memory at (ptr, size).
func readGuestString(mod api.Module, ptr, size uint32) string {
	if size == 0 {
		return ""
	}
	data, ok := mod.Memory().Read(ptr, size)
	if !ok {
		return ""
	}
	return string(data)
}

// readGuestBytes reads bytes from guest memory and makes a copy.
func readGuestBytes(mod api.Module, ptr, size uint32) []byte {
	if size == 0 {
		return nil
	}
	data, ok := mod.Memory().Read(ptr, size)
	if !ok {
		return nil
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	return cp
}

// writeGuestBytes allocates guest memory via malloc and writes data.
// Returns packed (ptr, size) as uint64. Returns 0 on failure.
func writeGuestBytes(ctx context.Context, mod api.Module, data []byte) uint64 {
	if len(data) == 0 {
		return 0
	}
	malloc := mod.ExportedFunction("malloc")
	if malloc == nil {
		return 0
	}
	results, err := malloc.Call(ctx, uint64(len(data)))
	if err != nil || len(results) == 0 || results[0] == 0 {
		return 0
	}
	ptr := uint32(results[0])
	if !mod.Memory().Write(ptr, data) {
		return 0
	}
	return uint64(ptr)<<32 | uint64(len(data))
}

// writeGuestJSON marshals v to JSON and writes to guest memory.
func writeGuestJSON(ctx context.Context, mod api.Module, v any) uint64 {
	data, err := json.Marshal(v)
	if err != nil {
		return 0
	}
	return writeGuestBytes(ctx, mod, data)
}

// encodeError writes {"error": "msg"} to guest memory.
func encodeError(ctx context.Context, mod api.Module, err error) uint64 {
	return writeGuestJSON(ctx, mod, map[string]string{"error": err.Error()})
}
