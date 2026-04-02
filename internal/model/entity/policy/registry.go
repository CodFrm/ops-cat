package policy

import "sync"

var defaultPolicyRegistry = struct {
	sync.RWMutex
	providers map[string]func() any
}{
	providers: make(map[string]func() any),
}

// RegisterDefaultPolicy 注册资产类型的默认策略提供者。
// 内置类型在 init() 中注册，扩展类型在 Bridge.Register 时注册。
func RegisterDefaultPolicy(assetType string, provider func() any) {
	defaultPolicyRegistry.Lock()
	defer defaultPolicyRegistry.Unlock()
	defaultPolicyRegistry.providers[assetType] = provider
}

// UnregisterDefaultPolicy 注销资产类型的默认策略提供者。
func UnregisterDefaultPolicy(assetType string) {
	defaultPolicyRegistry.Lock()
	defer defaultPolicyRegistry.Unlock()
	delete(defaultPolicyRegistry.providers, assetType)
}

// GetDefaultPolicyOf 获取指定资产类型的默认策略。
// 返回策略结构体和是否找到。
func GetDefaultPolicyOf(assetType string) (any, bool) {
	defaultPolicyRegistry.RLock()
	defer defaultPolicyRegistry.RUnlock()
	fn, ok := defaultPolicyRegistry.providers[assetType]
	if !ok {
		return nil, false
	}
	return fn(), true
}

func init() {
	RegisterDefaultPolicy("ssh", func() any { return DefaultCommandPolicy() })
	RegisterDefaultPolicy("database", func() any { return DefaultQueryPolicy() })
	RegisterDefaultPolicy("redis", func() any { return DefaultRedisPolicy() })
}
