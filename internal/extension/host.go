package extension

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	extism "github.com/extism/go-sdk"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
)

// HostServices 宿主提供给扩展的服务接口
type HostServices interface {
	KVGet(ctx context.Context, extension, key string) ([]byte, error)
	KVSet(ctx context.Context, extension, key string, value []byte) error
	EmitEvent(event string, data any)
	// HTTPRequest 代理 HTTP 请求
	HTTPRequest(ctx context.Context, method, url string, headers map[string]string, body []byte) (statusCode int, respHeaders map[string]string, respBody []byte, err error)
	// GetCredential 获取资产的解密凭据
	GetCredential(ctx context.Context, assetID int64) (map[string]string, error)
	// GetAssetConfig 获取资产配置 JSON
	GetAssetConfig(ctx context.Context, assetID int64) (json.RawMessage, error)
}

// LoadedPlugin 已加载的 WASM 插件
type LoadedPlugin struct {
	ExtName string
	plugin  *extism.Plugin
	mu      sync.Mutex
}

// CallTool 调用扩展的 execute_tool 导出函数
func (p *LoadedPlugin) CallTool(ctx context.Context, toolName string, args json.RawMessage) (json.RawMessage, error) {
	input, err := json.Marshal(map[string]any{
		"tool": toolName,
		"args": args,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal tool input: %w", err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	_, output, callErr := p.plugin.Call("execute_tool", input)
	if callErr != nil {
		return nil, fmt.Errorf("execute_tool %q: %w", toolName, callErr)
	}
	return output, nil
}

// CallCheckPolicy 调用扩展的 check_policy 导出函数
func (p *LoadedPlugin) CallCheckPolicy(ctx context.Context, toolName string, args json.RawMessage) (*PolicyCheckResult, error) {
	input, err := json.Marshal(map[string]any{
		"tool": toolName,
		"args": args,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal policy input: %w", err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	_, output, callErr := p.plugin.Call("check_policy", input)
	if callErr != nil {
		return nil, fmt.Errorf("check_policy: %w", callErr)
	}
	var result PolicyCheckResult
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("unmarshal policy result: %w", err)
	}
	return &result, nil
}

// PolicyCheckResult 扩展策略分类结果
type PolicyCheckResult struct {
	Action   string `json:"action"`
	Resource string `json:"resource"`
}

// Close 释放插件资源
func (p *LoadedPlugin) Close(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.plugin.Close(ctx); err != nil {
		logger.Default().Warn("close extension plugin",
			zap.String("name", p.ExtName),
			zap.Error(err),
		)
	}
}

// ExtensionHost 扩展宿主，管理 WASM 加载和 Host Function 注入
type ExtensionHost struct {
	services HostServices
	plugins  map[string]*LoadedPlugin
	mu       sync.RWMutex
}

// NewExtensionHost 创建扩展宿主
func NewExtensionHost(services HostServices) *ExtensionHost {
	return &ExtensionHost{
		services: services,
		plugins:  make(map[string]*LoadedPlugin),
	}
}

// LoadPlugin 加载单个扩展的 WASM 模块
func (h *ExtensionHost) LoadPlugin(ctx context.Context, ext *ExtensionInfo) (*LoadedPlugin, error) {
	wasmPath := filepath.Clean(filepath.Join(ext.Dir, ext.Manifest.Backend.Binary))
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		return nil, fmt.Errorf("read WASM binary %s: %w", wasmPath, err)
	}

	manifest := extism.Manifest{
		Wasm: []extism.Wasm{
			extism.WasmData{Data: wasmBytes},
		},
	}

	hostFunctions := h.buildHostFunctions(ext.Manifest.Name)
	config := extism.PluginConfig{
		EnableWasi: true,
	}

	plugin, err := extism.NewPlugin(ctx, manifest, config, hostFunctions)
	if err != nil {
		return nil, fmt.Errorf("create Extism plugin %q: %w", ext.Manifest.Name, err)
	}

	loaded := &LoadedPlugin{
		ExtName: ext.Manifest.Name,
		plugin:  plugin,
	}

	h.mu.Lock()
	h.plugins[ext.Manifest.Name] = loaded
	h.mu.Unlock()

	return loaded, nil
}

// GetPlugin 按名称获取已加载的插件
func (h *ExtensionHost) GetPlugin(name string) *LoadedPlugin {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.plugins[name]
}

// Close 释放所有插件资源
func (h *ExtensionHost) Close(ctx context.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for name, p := range h.plugins {
		p.Close(ctx)
		delete(h.plugins, name)
	}
}

// buildHostFunctions 构建宿主函数列表
func (h *ExtensionHost) buildHostFunctions(extName string) []extism.HostFunction {
	var funcs []extism.HostFunction

	// host_log: 扩展日志输出
	logFn := extism.NewHostFunctionWithStack(
		"host_log",
		func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
			input, err := p.ReadBytes(stack[0])
			if err != nil {
				return
			}
			var logMsg struct {
				Level   string `json:"level"`
				Message string `json:"message"`
			}
			if json.Unmarshal(input, &logMsg) != nil {
				return
			}
			switch logMsg.Level {
			case "error":
				logger.Default().Error("extension: "+extName, zap.String("msg", logMsg.Message))
			case "warn":
				logger.Default().Warn("extension: "+extName, zap.String("msg", logMsg.Message))
			default:
				logger.Default().Info("extension: "+extName, zap.String("msg", logMsg.Message))
			}
			offset, _ := p.WriteBytes([]byte("{}"))
			stack[0] = offset
		},
		[]extism.ValueType{extism.ValueTypeI64},
		[]extism.ValueType{extism.ValueTypeI64},
	)
	funcs = append(funcs, logFn)

	// host_kv_get: KV 存储读取
	kvGetFn := extism.NewHostFunctionWithStack(
		"host_kv_get",
		func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
			input, err := p.ReadBytes(stack[0])
			if err != nil {
				offset, _ := p.WriteBytes([]byte(`{"error":"read input failed"}`))
				stack[0] = offset
				return
			}
			var req struct {
				Key string `json:"key"`
			}
			if json.Unmarshal(input, &req) != nil {
				offset, _ := p.WriteBytes([]byte(`{"error":"invalid input"}`))
				stack[0] = offset
				return
			}
			if h.services == nil {
				offset, _ := p.WriteBytes([]byte(`{"error":"kv service unavailable"}`))
				stack[0] = offset
				return
			}
			value, kvErr := h.services.KVGet(ctx, extName, req.Key)
			if kvErr != nil {
				offset, _ := p.WriteBytes([]byte(fmt.Sprintf(`{"error":%q}`, kvErr.Error())))
				stack[0] = offset
				return
			}
			result, _ := json.Marshal(map[string]any{"value": string(value)})
			offset, _ := p.WriteBytes(result)
			stack[0] = offset
		},
		[]extism.ValueType{extism.ValueTypeI64},
		[]extism.ValueType{extism.ValueTypeI64},
	)
	funcs = append(funcs, kvGetFn)

	// host_kv_set: KV 存储写入
	kvSetFn := extism.NewHostFunctionWithStack(
		"host_kv_set",
		func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
			input, err := p.ReadBytes(stack[0])
			if err != nil {
				offset, _ := p.WriteBytes([]byte(`{"error":"read input failed"}`))
				stack[0] = offset
				return
			}
			var req struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			}
			if json.Unmarshal(input, &req) != nil {
				offset, _ := p.WriteBytes([]byte(`{"error":"invalid input"}`))
				stack[0] = offset
				return
			}
			if h.services == nil {
				offset, _ := p.WriteBytes([]byte(`{"error":"kv service unavailable"}`))
				stack[0] = offset
				return
			}
			if kvErr := h.services.KVSet(ctx, extName, req.Key, []byte(req.Value)); kvErr != nil {
				offset, _ := p.WriteBytes([]byte(fmt.Sprintf(`{"error":%q}`, kvErr.Error())))
				stack[0] = offset
				return
			}
			offset, _ := p.WriteBytes([]byte(`{}`))
			stack[0] = offset
		},
		[]extism.ValueType{extism.ValueTypeI64},
		[]extism.ValueType{extism.ValueTypeI64},
	)
	funcs = append(funcs, kvSetFn)

	// host_http_request: HTTP 代理请求
	httpRequestFn := extism.NewHostFunctionWithStack(
		"host_http_request",
		func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
			input, err := p.ReadBytes(stack[0])
			if err != nil {
				offset, _ := p.WriteBytes([]byte(`{"error":"read input failed"}`))
				stack[0] = offset
				return
			}
			var req struct {
				Method  string            `json:"method"`
				URL     string            `json:"url"`
				Headers map[string]string `json:"headers"`
				Body    []byte            `json:"body"`
			}
			if json.Unmarshal(input, &req) != nil {
				offset, _ := p.WriteBytes([]byte(`{"error":"invalid input"}`))
				stack[0] = offset
				return
			}
			if h.services == nil {
				offset, _ := p.WriteBytes([]byte(`{"error":"http service unavailable"}`))
				stack[0] = offset
				return
			}
			statusCode, respHeaders, respBody, httpErr := h.services.HTTPRequest(ctx, req.Method, req.URL, req.Headers, req.Body)
			if httpErr != nil {
				offset, _ := p.WriteBytes([]byte(fmt.Sprintf(`{"error":%q}`, httpErr.Error())))
				stack[0] = offset
				return
			}
			result, _ := json.Marshal(map[string]any{
				"status_code": statusCode,
				"headers":     respHeaders,
				"body":        respBody,
			})
			offset, _ := p.WriteBytes(result)
			stack[0] = offset
		},
		[]extism.ValueType{extism.ValueTypeI64},
		[]extism.ValueType{extism.ValueTypeI64},
	)
	funcs = append(funcs, httpRequestFn)

	// host_credential_get: 获取资产凭据
	credentialGetFn := extism.NewHostFunctionWithStack(
		"host_credential_get",
		func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
			input, err := p.ReadBytes(stack[0])
			if err != nil {
				offset, _ := p.WriteBytes([]byte(`{"error":"read input failed"}`))
				stack[0] = offset
				return
			}
			var req struct {
				AssetID int64 `json:"asset_id"`
			}
			if json.Unmarshal(input, &req) != nil {
				offset, _ := p.WriteBytes([]byte(`{"error":"invalid input"}`))
				stack[0] = offset
				return
			}
			if h.services == nil {
				offset, _ := p.WriteBytes([]byte(`{"error":"credential service unavailable"}`))
				stack[0] = offset
				return
			}
			cred, credErr := h.services.GetCredential(ctx, req.AssetID)
			if credErr != nil {
				offset, _ := p.WriteBytes([]byte(fmt.Sprintf(`{"error":%q}`, credErr.Error())))
				stack[0] = offset
				return
			}
			result, _ := json.Marshal(cred)
			offset, _ := p.WriteBytes(result)
			stack[0] = offset
		},
		[]extism.ValueType{extism.ValueTypeI64},
		[]extism.ValueType{extism.ValueTypeI64},
	)
	funcs = append(funcs, credentialGetFn)

	// host_asset_get_config: 获取资产配置 JSON
	assetGetConfigFn := extism.NewHostFunctionWithStack(
		"host_asset_get_config",
		func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
			input, err := p.ReadBytes(stack[0])
			if err != nil {
				offset, _ := p.WriteBytes([]byte(`{"error":"read input failed"}`))
				stack[0] = offset
				return
			}
			var req struct {
				AssetID int64 `json:"asset_id"`
			}
			if json.Unmarshal(input, &req) != nil {
				offset, _ := p.WriteBytes([]byte(`{"error":"invalid input"}`))
				stack[0] = offset
				return
			}
			if h.services == nil {
				offset, _ := p.WriteBytes([]byte(`{"error":"asset config service unavailable"}`))
				stack[0] = offset
				return
			}
			config, cfgErr := h.services.GetAssetConfig(ctx, req.AssetID)
			if cfgErr != nil {
				offset, _ := p.WriteBytes([]byte(fmt.Sprintf(`{"error":%q}`, cfgErr.Error())))
				stack[0] = offset
				return
			}
			result, _ := json.Marshal(map[string]any{"config": config})
			offset, _ := p.WriteBytes(result)
			stack[0] = offset
		},
		[]extism.ValueType{extism.ValueTypeI64},
		[]extism.ValueType{extism.ValueTypeI64},
	)
	funcs = append(funcs, assetGetConfigFn)

	return funcs
}
