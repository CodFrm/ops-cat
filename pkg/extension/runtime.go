// pkg/extension/runtime.go
package extension

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// Plugin represents a loaded WASM extension.
type Plugin struct {
	manifest *Manifest
	compiled wazero.CompiledModule
	runtime  wazero.Runtime
	host     HostProvider
	mu       sync.Mutex
}

// LoadPlugin compiles a WASM binary and prepares it for execution.
func LoadPlugin(ctx context.Context, manifest *Manifest, wasmBytes []byte, host HostProvider) (*Plugin, error) {
	r := wazero.NewRuntime(ctx)

	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	// Register host functions module
	if err := registerHostModule(ctx, r, host); err != nil {
		r.Close(ctx)
		return nil, fmt.Errorf("register host functions: %w", err)
	}

	compiled, err := r.CompileModule(ctx, wasmBytes)
	if err != nil {
		r.Close(ctx)
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
	input, _ := json.Marshal(map[string]any{
		"tool": toolName,
		"args": json.RawMessage(args),
	})
	return p.call(ctx, "execute_tool", input)
}

// CallAction calls execute_action on the extension.
func (p *Plugin) CallAction(ctx context.Context, actionName string, args json.RawMessage) (json.RawMessage, error) {
	input, _ := json.Marshal(map[string]any{
		"action": actionName,
		"args":   json.RawMessage(args),
	})
	return p.call(ctx, "execute_action", input)
}

// CheckPolicy calls check_policy on the extension.
func (p *Plugin) CheckPolicy(ctx context.Context, toolName string, args json.RawMessage) (action, resource string, err error) {
	input, _ := json.Marshal(map[string]any{
		"tool": toolName,
		"args": json.RawMessage(args),
	})
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
	return p.runtime.Close(ctx)
}

// Manifest returns the plugin's manifest.
func (p *Plugin) Manifest() *Manifest {
	return p.manifest
}

// call invokes a WASM function using stdin/stdout for I/O.
func (p *Plugin) call(ctx context.Context, fnName string, input []byte) (json.RawMessage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	stdin := &bytesReader{data: input}
	stdout := &bytesWriter{}
	stderr := &bytesWriter{}

	cfg := wazero.NewModuleConfig().
		WithStdin(stdin).
		WithStdout(stdout).
		WithStderr(stderr).
		WithArgs(fnName).
		WithName("")

	mod, err := p.runtime.InstantiateModule(ctx, p.compiled, cfg)
	if err != nil {
		return nil, fmt.Errorf("instantiate module for %s: %w", fnName, err)
	}
	defer mod.Close(ctx)

	return stdout.Bytes(), nil
}

// registerHostModule registers all host functions as a wazero host module named "opskat".
// For Phase 1, this registers the module structure. The actual host function
// implementations will be refined when the guest SDK is built in Phase 2.
func registerHostModule(ctx context.Context, r wazero.Runtime, host HostProvider) error {
	_, err := r.NewHostModuleBuilder("opskat").
		NewFunctionBuilder().
		WithFunc(func() {
			// host_log placeholder - actual implementation needs memory access
			// Will be properly implemented when guest SDK defines the ABI
		}).Export("host_log").
		Instantiate(ctx)
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
