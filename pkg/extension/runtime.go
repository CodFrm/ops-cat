// pkg/extension/runtime.go
package extension

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cago-frame/cago/pkg/logger"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"go.uber.org/zap"
)

// Plugin represents a loaded WASM extension.
type Plugin struct {
	manifest     *Manifest
	compiled     wazero.CompiledModule
	runtime      wazero.Runtime
	host         HostProvider
	mu           sync.Mutex
	closed       atomic.Bool
	activeCancel atomic.Pointer[ActionCancellation]
}

// LoadPlugin compiles a WASM binary and prepares it for execution.
// If cache is non-nil, compiled modules are cached to disk for faster subsequent loads.
func LoadPlugin(ctx context.Context, manifest *Manifest, wasmBytes []byte, host HostProvider, cache wazero.CompilationCache) (*Plugin, error) {
	cfg := wazero.NewRuntimeConfig().WithMemoryLimitPages(1024)
	if cache != nil {
		cfg = cfg.WithCompilationCache(cache)
	}
	r := wazero.NewRuntimeWithConfig(ctx, cfg)

	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	// Register host functions module
	if err := registerHostModule(ctx, r, host); err != nil {
		if closeErr := r.Close(ctx); closeErr != nil {
			logger.Ctx(ctx).Warn("close wasm runtime after host module error", zap.Error(closeErr))
		}
		return nil, fmt.Errorf("register host functions: %w", err)
	}

	compiled, err := r.CompileModule(ctx, wasmBytes)
	if err != nil {
		if closeErr := r.Close(ctx); closeErr != nil {
			logger.Ctx(ctx).Warn("close wasm runtime after compile error", zap.Error(closeErr))
		}
		return nil, fmt.Errorf("compile wasm: %w", err)
	}

	return &Plugin{
		manifest: manifest,
		compiled: compiled,
		runtime:  r,
		host:     host,
	}, nil
}

// CallTool calls execute_tool on the extension.
func (p *Plugin) CallTool(ctx context.Context, toolName string, args json.RawMessage) (json.RawMessage, error) {
	input, err := json.Marshal(map[string]any{
		"tool": toolName,
		"args": json.RawMessage(args),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal %s input: %w", "execute_tool", err)
	}
	return p.call(ctx, "execute_tool", input)
}

// CallAction calls execute_action on the extension.
//
// The cancellation is installed AFTER acquiring p.mu so that concurrent
// CallAction invocations don't race on host.activeCancel: the setup and the
// WASM execution live in the same critical section, and the defer clears the
// cancel before releasing the lock, so the next caller installs fresh state.
func (p *Plugin) CallAction(ctx context.Context, actionName string, args json.RawMessage) (json.RawMessage, error) {
	input, err := json.Marshal(map[string]any{
		"action": actionName,
		"args":   json.RawMessage(args),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal %s input: %w", "execute_action", err)
	}
	if p.closed.Load() {
		return nil, fmt.Errorf("plugin closed")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	cancel := NewActionCancellation()
	p.host.SetActiveCancellation(cancel)
	p.activeCancel.Store(cancel)
	defer func() {
		p.activeCancel.Store(nil)
		p.host.SetActiveCancellation(nil)
	}()

	return p.callLocked(ctx, "execute_action", input)
}

// CancelActiveAction triggers cancellation of the currently running action.
// Due to Plugin.mu serializing CallAction invocations, at most one action
// runs per plugin at a time — this cancels that one. No-op if idle.
func (p *Plugin) CancelActiveAction() {
	if c := p.activeCancel.Load(); c != nil {
		c.Cancel()
	}
}

// CheckPolicy calls check_policy on the extension.
func (p *Plugin) CheckPolicy(ctx context.Context, toolName string, args json.RawMessage) (action, resource string, err error) {
	input, err := json.Marshal(map[string]any{
		"tool": toolName,
		"args": json.RawMessage(args),
	})
	if err != nil {
		return "", "", fmt.Errorf("marshal %s input: %w", "check_policy", err)
	}
	result, err := p.call(ctx, "check_policy", input)
	if err != nil {
		return "", "", err
	}
	var decision struct {
		Action   string `json:"action"`
		Resource string `json:"resource"`
	}
	if err := json.Unmarshal(result, &decision); err != nil {
		return "", "", fmt.Errorf("unmarshal policy decision: %w", err)
	}
	return decision.Action, decision.Resource, nil
}

// ValidateConfig calls validate_config on the extension.
func (p *Plugin) ValidateConfig(ctx context.Context, config json.RawMessage) ([]ValidationError, error) {
	result, err := p.call(ctx, "validate_config", config)
	if err != nil {
		return nil, err
	}
	var errors []ValidationError
	if err := json.Unmarshal(result, &errors); err != nil {
		return nil, fmt.Errorf("unmarshal validation errors: %w", err)
	}
	return errors, nil
}

// ValidationError represents a config validation error.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Close releases the WASM runtime resources.
func (p *Plugin) Close(ctx context.Context) error {
	p.closed.Store(true)
	return p.runtime.Close(ctx)
}

// Manifest returns the plugin's manifest.
func (p *Plugin) Manifest() *Manifest {
	return p.manifest
}

// call invokes a WASM function using stdin/stdout for I/O.
func (p *Plugin) call(ctx context.Context, fnName string, input []byte) (json.RawMessage, error) {
	if p.closed.Load() {
		return nil, fmt.Errorf("plugin closed")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	return p.callLocked(ctx, fnName, input)
}

// callLocked executes a WASM function. Caller must hold p.mu.
func (p *Plugin) callLocked(ctx context.Context, fnName string, input []byte) (json.RawMessage, error) {
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	stdin := &bytesReader{data: input}
	stdout := &bytesWriter{}
	stderr := &bytesWriter{}

	cfg := wazero.NewModuleConfig().
		WithStdin(stdin).
		WithStdout(stdout).
		WithStderr(stderr).
		WithArgs(fnName).
		WithName("").
		WithSysWalltime().
		WithSysNanotime()

	mod, err := p.runtime.InstantiateModule(callCtx, p.compiled, cfg)
	if err != nil {
		return nil, fmt.Errorf("instantiate module for %s: %w", fnName, err)
	}
	defer func() {
		if err := mod.Close(callCtx); err != nil {
			logger.Ctx(ctx).Warn("close wasm module", zap.Error(err))
		}
	}()

	// The guest SDK encodes handler errors as {"error":"..."} on stdout.
	// Detect this and propagate as a Go error so callers (Wails, AI bridge,
	// devserver) can distinguish success from failure.
	out := stdout.Bytes()
	var errEnvelope struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(out, &errEnvelope) == nil && errEnvelope.Error != "" {
		return nil, fmt.Errorf("%s", errEnvelope.Error)
	}

	return out, nil
}

// registerHostModule registers all 13 host functions as a wazero host module named "opskat".
// Guest and host share memory using the convention:
//   - Guest exports malloc(size) -> ptr and free(ptr)
//   - Return values packed as uint64: high 32 bits = ptr, low 32 bits = size
//   - Errors returned as JSON {"error": "message"}
func registerHostModule(ctx context.Context, r wazero.Runtime, host HostProvider) error {
	b := r.NewHostModuleBuilder("opskat")

	// host_log(level_ptr, level_len, msg_ptr, msg_len)
	b.NewFunctionBuilder().WithFunc(func(ctx context.Context, mod api.Module, levelPtr, levelLen, msgPtr, msgLen uint32) {
		level := readGuestString(mod, levelPtr, levelLen)
		msg := readGuestString(mod, msgPtr, msgLen)
		host.Log(level, msg)
	}).Export("host_log")

	// host_io_open(params_ptr, params_len) -> packed(result_ptr, result_len)
	b.NewFunctionBuilder().WithFunc(func(ctx context.Context, mod api.Module, paramsPtr, paramsLen uint32) uint64 {
		var params IOOpenParams
		if err := json.Unmarshal(readGuestBytes(mod, paramsPtr, paramsLen), &params); err != nil {
			return encodeError(ctx, mod, err)
		}
		handleID, meta, err := host.IOOpen(params)
		if err != nil {
			return encodeError(ctx, mod, err)
		}
		return writeGuestJSON(ctx, mod, map[string]any{"handle_id": handleID, "meta": meta})
	}).Export("host_io_open")

	// host_io_read(handle_id, size) -> packed(data_ptr, data_len)
	b.NewFunctionBuilder().WithFunc(func(ctx context.Context, mod api.Module, handleID, size uint32) uint64 {
		data, err := host.IORead(handleID, int(size))
		if err == io.EOF {
			// Signal EOF as empty result (size=0). The guest SDK's
			// IOHandle.Read converts len(data)==0 into the real io.EOF.
			// Encoding io.EOF as a JSON error would produce fmt.Errorf("EOF")
			// on the guest, which != io.EOF and breaks io.ReadAll / AWS SDK.
			return writeGuestBytes(ctx, mod, nil)
		}
		if err != nil {
			return encodeError(ctx, mod, err)
		}
		return writeGuestBytes(ctx, mod, data)
	}).Export("host_io_read")

	// host_io_write(handle_id, data_ptr, data_len) -> n
	b.NewFunctionBuilder().WithFunc(func(ctx context.Context, mod api.Module, handleID, dataPtr, dataLen uint32) uint32 {
		data := readGuestBytes(mod, dataPtr, dataLen)
		n, err := host.IOWrite(handleID, data)
		if err != nil {
			return 0
		}
		return uint32(n)
	}).Export("host_io_write")

	// host_io_flush(handle_id) -> packed(meta_ptr, meta_len)
	b.NewFunctionBuilder().WithFunc(func(ctx context.Context, mod api.Module, handleID uint32) uint64 {
		meta, err := host.IOFlush(handleID)
		if err != nil {
			return encodeError(ctx, mod, err)
		}
		return writeGuestJSON(ctx, mod, meta)
	}).Export("host_io_flush")

	// host_io_close(handle_id)
	b.NewFunctionBuilder().WithFunc(func(ctx context.Context, mod api.Module, handleID uint32) {
		if err := host.IOClose(handleID); err != nil {
			logger.Ctx(ctx).Warn("close IO handle from host", zap.Uint32("handleID", handleID), zap.Error(err))
		}
	}).Export("host_io_close")

	// host_io_set_deadline(handle_id, kind_ptr, kind_len, unix_nanos) -> packed(err_ptr, err_len) or 0 on success
	b.NewFunctionBuilder().WithFunc(func(ctx context.Context, mod api.Module, handleID, kindPtr, kindLen uint32, unixNanos int64) uint64 {
		kind := readGuestString(mod, kindPtr, kindLen)
		if err := host.IOSetDeadline(handleID, kind, unixNanos); err != nil {
			return encodeError(ctx, mod, err)
		}
		return 0
	}).Export("host_io_set_deadline")

	// host_asset_get_config(asset_id) -> packed(result_ptr, result_len)
	b.NewFunctionBuilder().WithFunc(func(ctx context.Context, mod api.Module, assetID uint64) uint64 {
		cfg, err := host.GetAssetConfig(int64(assetID))
		if err != nil {
			return encodeError(ctx, mod, err)
		}
		return writeGuestBytes(ctx, mod, cfg)
	}).Export("host_asset_get_config")

	// host_file_dialog(params_ptr, params_len) -> packed(result_ptr, result_len)
	b.NewFunctionBuilder().WithFunc(func(ctx context.Context, mod api.Module, paramsPtr, paramsLen uint32) uint64 {
		var req struct {
			Type string        `json:"type"`
			Opts DialogOptions `json:"opts"`
		}
		if err := json.Unmarshal(readGuestBytes(mod, paramsPtr, paramsLen), &req); err != nil {
			return encodeError(ctx, mod, err)
		}
		result, err := host.FileDialog(req.Type, req.Opts)
		if err != nil {
			return encodeError(ctx, mod, err)
		}
		return writeGuestBytes(ctx, mod, []byte(result))
	}).Export("host_file_dialog")

	// host_kv_get(key_ptr, key_len) -> packed(val_ptr, val_len)
	b.NewFunctionBuilder().WithFunc(func(ctx context.Context, mod api.Module, keyPtr, keyLen uint32) uint64 {
		key := readGuestString(mod, keyPtr, keyLen)
		val, err := host.KVGet(key)
		if err != nil {
			return encodeError(ctx, mod, err)
		}
		return writeGuestBytes(ctx, mod, val)
	}).Export("host_kv_get")

	// host_kv_set(key_ptr, key_len, val_ptr, val_len)
	b.NewFunctionBuilder().WithFunc(func(ctx context.Context, mod api.Module, keyPtr, keyLen, valPtr, valLen uint32) {
		key := readGuestString(mod, keyPtr, keyLen)
		val := readGuestBytes(mod, valPtr, valLen)
		if err := host.KVSet(key, val); err != nil {
			logger.Ctx(ctx).Warn("host KV set", zap.String("key", key), zap.Error(err))
		}
	}).Export("host_kv_set")

	// host_action_event(type_ptr, type_len, data_ptr, data_len)
	b.NewFunctionBuilder().WithFunc(func(ctx context.Context, mod api.Module, typePtr, typeLen, dataPtr, dataLen uint32) {
		eventType := readGuestString(mod, typePtr, typeLen)
		data := readGuestBytes(mod, dataPtr, dataLen)
		if err := host.ActionEvent(eventType, data); err != nil {
			logger.Ctx(ctx).Warn("host action event", zap.String("eventType", eventType), zap.Error(err))
		}
	}).Export("host_action_event")

	// host_action_should_stop() -> 0 or 1
	b.NewFunctionBuilder().WithFunc(func(ctx context.Context, mod api.Module) uint32 {
		if host.ActionShouldStop() {
			return 1
		}
		return 0
	}).Export("host_action_should_stop")

	_, err := b.Instantiate(ctx)
	return err
}

// bytesReader implements io.Reader over a byte slice.
type bytesReader struct {
	data []byte
	pos  int
}

func (r *bytesReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

// bytesWriter implements io.Writer that accumulates bytes.
type bytesWriter struct {
	data []byte
}

func (w *bytesWriter) Write(p []byte) (int, error) {
	w.data = append(w.data, p...)
	return len(p), nil
}

func (w *bytesWriter) Bytes() []byte {
	return w.data
}
