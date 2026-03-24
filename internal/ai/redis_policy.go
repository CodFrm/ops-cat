package ai

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
)

// redisMultiWordCmds 多词 Redis 命令的前缀
var redisMultiWordCmds = map[string]bool{
	"CONFIG":  true,
	"ACL":     true,
	"CLUSTER": true,
	"CLIENT":  true,
	"DEBUG":   true,
	"MEMORY":  true,
	"MODULE":  true,
	"SCRIPT":  true,
	"SLOWLOG": true,
	"OBJECT":  true,
	"XGROUP":  true,
	"XINFO":   true,
}

// ExtractRedisCommand 提取 Redis 命令名（含子命令）和参数
func ExtractRedisCommand(cmd string) (fullCmd string, args string) {
	parts := strings.Fields(strings.TrimSpace(cmd))
	if len(parts) == 0 {
		return "", ""
	}
	name := strings.ToUpper(parts[0])
	if len(parts) > 1 && redisMultiWordCmds[name] {
		fullCmd = name + " " + strings.ToUpper(parts[1])
		if len(parts) > 2 {
			args = strings.Join(parts[2:], " ")
		}
	} else {
		fullCmd = name
		if len(parts) > 1 {
			args = strings.Join(parts[1:], " ")
		}
	}
	return
}

// MatchRedisRule 检查 Redis 命令是否匹配规则
// 规则格式: "FLUSHDB", "CONFIG SET *", "DEL user:*"
func MatchRedisRule(rule, cmd string) bool {
	ruleCmd, ruleArgs := ExtractRedisCommand(rule)
	cmdCmd, cmdArgs := ExtractRedisCommand(cmd)

	if ruleCmd != cmdCmd {
		return false
	}
	// 无参数规则或 * 通配 → 匹配
	if ruleArgs == "" || ruleArgs == "*" {
		return true
	}
	if cmdArgs == "" {
		return false
	}
	// 按首个参数做 glob 匹配（key pattern）
	ruleFirstArg := strings.Fields(ruleArgs)[0]
	cmdFirstArg := strings.Fields(cmdArgs)[0]
	matched, _ := filepath.Match(ruleFirstArg, cmdFirstArg)
	return matched
}

// CheckRedisPolicy 检查 Redis 命令是否符合策略
func CheckRedisPolicy(policy *asset_entity.RedisPolicy, cmd string) CheckResult {
	merged := mergeRedisPolicy(policy, asset_entity.DefaultRedisPolicy())

	// deny list 检查
	for _, rule := range merged.DenyList {
		if MatchRedisRule(rule, cmd) {
			return CheckResult{
				Decision:       Deny,
				Message:        fmt.Sprintf("Redis 命令被策略禁止: %s", cmd),
				DecisionSource: SourcePolicyDeny,
				MatchedPattern: rule,
			}
		}
	}
	// allow list 白名单
	if len(merged.AllowList) > 0 {
		for _, rule := range merged.AllowList {
			if MatchRedisRule(rule, cmd) {
				return CheckResult{Decision: Allow, DecisionSource: SourcePolicyAllow}
			}
		}
		return CheckResult{Decision: NeedConfirm}
	}
	return CheckResult{Decision: Allow, DecisionSource: SourcePolicyAllow}
}

func mergeRedisPolicy(custom, defaults *asset_entity.RedisPolicy) *asset_entity.RedisPolicy {
	result := &asset_entity.RedisPolicy{}
	if custom != nil {
		result.AllowList = custom.AllowList
		result.DenyList = append(result.DenyList, custom.DenyList...)
	}
	if defaults != nil {
		// 去重追加默认 deny
		seen := make(map[string]bool, len(result.DenyList))
		for _, r := range result.DenyList {
			seen[strings.ToUpper(r)] = true
		}
		for _, r := range defaults.DenyList {
			if !seen[strings.ToUpper(r)] {
				result.DenyList = append(result.DenyList, r)
			}
		}
	}
	return result
}
