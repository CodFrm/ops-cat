package ai

import (
	"context"
	"path"
	"strings"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"

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
	if isWildcardAll(rule) {
		cmdCmd, _ := ExtractRedisCommand(cmd)
		return cmdCmd != ""
	}

	ruleParts := strings.Fields(strings.TrimSpace(rule))
	cmdParts := strings.Fields(strings.TrimSpace(cmd))
	if len(ruleParts) == 0 || len(cmdParts) == 0 {
		return false
	}
	if redisMultiWordCmds[strings.ToUpper(ruleParts[0])] &&
		len(ruleParts) == 2 &&
		isWildcardAll(ruleParts[1]) &&
		len(cmdParts) >= 2 &&
		strings.EqualFold(ruleParts[0], cmdParts[0]) {
		return true
	}

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
	matched, err := path.Match(ruleFirstArg, cmdFirstArg)
	if err != nil {
		logger.Default().Warn("redis policy path match", zap.String("pattern", ruleFirstArg), zap.Error(err))
	}
	return matched
}

// CheckRedisPolicy 检查 Redis 命令是否符合策略（合并默认策略后检查）
func CheckRedisPolicy(ctx context.Context, policy *asset_entity.RedisPolicy, cmd string) CheckResult {
	merged := effectiveRedisPolicy(ctx, policy)
	return checkRedisPolicyRules(ctx, merged, cmd)
}

// checkRedisPolicyRules 检查 Redis 命令是否符合给定策略（不合并默认策略）
func checkRedisPolicyRules(ctx context.Context, policy *asset_entity.RedisPolicy, cmd string) CheckResult {
	if policy == nil {
		return CheckResult{Decision: Allow, DecisionSource: SourcePolicyAllow}
	}
	// deny list 检查
	for _, rule := range policy.DenyList {
		if MatchRedisRule(rule, cmd) {
			return CheckResult{
				Decision:       Deny,
				Message:        policyFmt(ctx, "Redis command denied by policy: %s", "Redis 命令被策略禁止: %s", cmd),
				DecisionSource: SourcePolicyDeny,
				MatchedPattern: rule,
			}
		}
	}
	// allow list 白名单
	if len(policy.AllowList) > 0 {
		for _, rule := range policy.AllowList {
			if MatchRedisRule(rule, cmd) {
				return CheckResult{Decision: Allow, DecisionSource: SourcePolicyAllow}
			}
		}
		return CheckResult{Decision: NeedConfirm}
	}
	return CheckResult{Decision: Allow, DecisionSource: SourcePolicyAllow}
}
