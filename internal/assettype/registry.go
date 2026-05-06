// internal/assettype/registry.go
package assettype

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
)

// AssetTypeHandler 资产类型处理器接口。
type AssetTypeHandler interface {
	Type() string
	DefaultPort() int
	SafeView(a *asset_entity.Asset) map[string]any
	ResolvePassword(ctx context.Context, a *asset_entity.Asset) (string, error)
	DefaultPolicy() any
	// ValidateCreateArgs 校验 AI 工具创建资产时的必填字段。
	// 由 handleAddAsset 在 ApplyCreateArgs 之前调用，每种类型自行声明所需字段。
	ValidateCreateArgs(args map[string]any) error
	ApplyCreateArgs(ctx context.Context, a *asset_entity.Asset, args map[string]any) error
	ApplyUpdateArgs(ctx context.Context, a *asset_entity.Asset, args map[string]any) error
}

// validateRemoteServerArgs 是 ssh/database/redis/mongodb 共用的 host/port/username 校验。
func validateRemoteServerArgs(args map[string]any) error {
	if ArgString(args, "host") == "" || ArgInt(args, "port") == 0 || ArgString(args, "username") == "" {
		return fmt.Errorf("missing required parameters: host, port, username")
	}
	return nil
}

var (
	mu       sync.RWMutex
	registry = map[string]AssetTypeHandler{}
)

func Register(h AssetTypeHandler) {
	mu.Lock()
	defer mu.Unlock()
	registry[h.Type()] = h
}

func Get(assetType string) (AssetTypeHandler, bool) {
	mu.RLock()
	defer mu.RUnlock()
	h, ok := registry[assetType]
	return h, ok
}

func All() []AssetTypeHandler {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]AssetTypeHandler, 0, len(registry))
	for _, h := range registry {
		out = append(out, h)
	}
	return out
}

// --- Arg extraction helpers ---

func ArgString(args map[string]any, key string) string {
	v, ok := args[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	return s
}

func ArgInt(args map[string]any, key string) int {
	v, ok := args[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	default:
		return 0
	}
}

func ArgInt64(args map[string]any, key string) int64 {
	v, ok := args[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int:
		return int64(n)
	case int64:
		return n
	default:
		return 0
	}
}

// ArgBool 从 args 中解析 bool。支持 bool、字符串 ("true"/"1"/"yes")、数字 1。
func ArgBool(args map[string]any, key string) bool {
	v, ok := args[key]
	if !ok {
		return false
	}
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return strings.EqualFold(x, "true") || x == "1" || strings.EqualFold(x, "yes")
	default:
		return fmt.Sprintf("%v", x) == "1"
	}
}

// ArgStringSlice 从 args 中解析字符串数组。支持 []string、[]any、用逗号/分号/换行分隔的字符串。
// 自动 trim 空白并丢弃空项。
func ArgStringSlice(args map[string]any, key string) []string {
	v, ok := args[key]
	if !ok || v == nil {
		return nil
	}
	switch x := v.(type) {
	case []string:
		return cleanStrings(x)
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			out = append(out, fmt.Sprintf("%v", item))
		}
		return cleanStrings(out)
	case string:
		parts := strings.FieldsFunc(x, func(r rune) bool { return r == ',' || r == '\n' || r == ';' })
		return cleanStrings(parts)
	default:
		return nil
	}
}

func cleanStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
