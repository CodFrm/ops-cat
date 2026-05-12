package ai

import (
	"sort"
	"strings"
)

// ModelDefaults 已知模型的默认参数
type ModelDefaults struct {
	MaxOutputTokens int // 最大输出 token 数
	ContextWindow   int // 上下文窗口大小
}

type prefixDefault struct {
	Prefix   string
	Defaults ModelDefaults
}

// modelPrefixDefaults 按前缀匹配的默认参数
// init() 中按前缀长度降序排序，确保最长（最具体）的前缀优先匹配
var modelPrefixDefaults []prefixDefault

func init() {
	d := func(maxTokens, contextWindow int) ModelDefaults {
		return ModelDefaults{MaxOutputTokens: maxTokens, ContextWindow: contextWindow}
	}
	modelPrefixDefaults = []prefixDefault{
		// OpenAI GPT-5.x
		{"gpt-5.4-pro", d(128000, 1050000)},
		{"gpt-5.4-mini", d(128000, 400000)},
		{"gpt-5.4-nano", d(128000, 400000)},
		{"gpt-5.4", d(128000, 1050000)},
		{"gpt-5.3", d(128000, 400000)},
		{"gpt-5.2-chat", d(16384, 128000)},
		{"gpt-5.2", d(128000, 400000)},
		{"gpt-5-mini", d(128000, 400000)},
		{"gpt-5-nano", d(128000, 400000)},
		{"gpt-5", d(128000, 400000)},
		{"gpt-4o-mini", d(16384, 128000)},
		{"gpt-4o", d(16384, 128000)},
		{"chatgpt-4o", d(16384, 128000)},

		// OpenAI reasoning models
		{"o4-mini", d(100000, 200000)},
		{"o3-mini", d(65536, 200000)},
		{"o3", d(100000, 200000)},

		// Anthropic (Opus/Sonnet 1M context GA, Haiku 200K)
		{"claude-opus-4", d(128000, 1000000)},
		{"claude-sonnet-4", d(64000, 1000000)},
		{"claude-haiku-4", d(64000, 200000)},
		{"claude-3-5-sonnet", d(8192, 200000)},
		{"claude-3-5-haiku", d(8192, 200000)},

		// DeepSeek
		{"deepseek-v4", d(16384, 1000000)},
		{"deepseek-reasoner", d(16384, 164000)},
		{"deepseek-chat", d(8192, 164000)},
		{"deepseek", d(8192, 128000)},

		// Google Gemini
		{"gemini-3.1", d(65536, 1048576)},
		{"gemini-3-pro", d(65536, 1048576)},
		{"gemini-3-flash", d(65536, 1048576)},
		{"gemini-3", d(65536, 1048576)},
		{"gemini-2.5-pro", d(65536, 1048576)},
		{"gemini-2.5-flash", d(65536, 1048576)},
		{"gemini-2.0-flash", d(8192, 1048576)},

		// Qwen (通义千问)
		{"qwen3.5-plus", d(65536, 1000000)},
		{"qwen3.5", d(65536, 262144)},
		{"qwen3-max", d(32768, 262144)},
		{"qwen3", d(32768, 131072)},
		{"qwen-max", d(8192, 32768)},
		{"qwen-plus", d(8192, 131072)},
		{"qwen-turbo", d(8192, 131072)},
		{"qwen-long", d(8192, 10000000)},
		{"qwen2.5", d(8192, 131072)},
		{"qwen", d(8192, 32768)},

		// GLM (智谱)
		{"glm-4.7-flash", d(65535, 200000)},
		{"glm-4.7", d(65535, 200000)},
		{"glm-4.6", d(128000, 205000)},
		{"glm-4.5", d(96000, 131072)},
		{"glm-4", d(4096, 128000)},

		// MiniMax
		{"minimax-m2.7", d(131072, 204800)},
		{"minimax-m2.5", d(65536, 196608)},
		{"minimax-m2", d(65536, 196608)},
		{"minimax", d(65536, 196608)},

		// Moonshot / Kimi
		{"kimi-k2", d(8192, 256000)},
		{"moonshot", d(4096, 128000)},

		// Doubao (豆包)
		{"doubao", d(4096, 128000)},

		// Llama 4 (Meta)
		{"llama-4-scout", d(8192, 10000000)},
		{"llama-4-maverick", d(8192, 1000000)},
		{"llama-4-behemoth", d(8192, 256000)},
		{"llama-4", d(8192, 256000)},

		// Mistral
		{"mistral-large-3", d(8192, 256000)},
		{"mistral-large", d(8192, 131072)},
		{"mistral-small", d(8192, 32768)},
		{"mistral", d(8192, 32768)},
	}
	// 按前缀长度降序排序，确保最长前缀优先匹配
	sort.Slice(modelPrefixDefaults, func(i, j int) bool {
		return len(modelPrefixDefaults[i].Prefix) > len(modelPrefixDefaults[j].Prefix)
	})
}

// GetModelDefaults 获取模型默认参数：前缀匹配（最长前缀优先），未知模型返回 nil
func GetModelDefaults(model string) *ModelDefaults {
	if model == "" {
		return nil
	}
	lower := strings.ToLower(model)
	for _, p := range modelPrefixDefaults {
		if strings.HasPrefix(lower, p.Prefix) {
			d := p.Defaults
			return &d
		}
	}
	return nil
}

const (
	// FallbackMaxOutputTokens 未知模型的默认最大输出 token
	FallbackMaxOutputTokens = 16384
	// FallbackContextWindow 未知模型的默认上下文窗口
	FallbackContextWindow = 128000
)
