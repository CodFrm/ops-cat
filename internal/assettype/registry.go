// internal/assettype/registry.go
package assettype

import (
	"context"
	"fmt"
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
	ApplyCreateArgs(ctx context.Context, a *asset_entity.Asset, args map[string]any) error
	ApplyUpdateArgs(ctx context.Context, a *asset_entity.Asset, args map[string]any) error
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
