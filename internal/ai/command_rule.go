package ai

import (
	"path"
	"strings"
)

// --- 命令规则匹配 ---

// ParsedCommand 解析后的命令结构
type ParsedCommand struct {
	Program     string
	SubCommands []string
	Flags       map[string]string
	Wildcard    bool
}

// ParseCommandRule 将规则字符串解析为结构化表示
func ParseCommandRule(rule string) *ParsedCommand {
	tokens := tokenize(rule)
	if len(tokens) == 0 {
		return &ParsedCommand{}
	}

	result := &ParsedCommand{
		Program: tokens[0],
		Flags:   make(map[string]string),
	}

	i := 1
	for i < len(tokens) {
		t := tokens[i]
		if isFlag(t) {
			if strings.Contains(t, "=") {
				// --flag=value
				parts := strings.SplitN(t, "=", 2)
				result.Flags[parts[0]] = parts[1]
			} else if i+1 < len(tokens) && !isFlag(tokens[i+1]) {
				// -f value（* 在 flag 后面作为值，不是通配符）
				result.Flags[t] = tokens[i+1]
				i++
			} else {
				// 布尔 flag
				result.Flags[t] = ""
			}
		} else if t == "*" {
			// 只有非 flag 值位置的 * 才是通配符
			result.Wildcard = true
		} else {
			result.SubCommands = append(result.SubCommands, t)
		}
		i++
	}

	return result
}

// ParseActualCommand 解析实际命令，用规则的 flag 列表作为参照判断哪些 flag 带值
func ParseActualCommand(command string, rule *ParsedCommand) *ParsedCommand {
	tokens := tokenize(command)
	if len(tokens) == 0 {
		return &ParsedCommand{}
	}

	result := &ParsedCommand{
		Program: tokens[0],
		Flags:   make(map[string]string),
	}

	i := 1
	for i < len(tokens) {
		t := tokens[i]
		if isFlag(t) {
			if strings.Contains(t, "=") {
				parts := strings.SplitN(t, "=", 2)
				result.Flags[parts[0]] = parts[1]
			} else if i+1 < len(tokens) && !isFlag(tokens[i+1]) {
				// 用规则判断：如果规则中该 flag 带值，则实际命令中也视为带值
				if _, hasValue := rule.Flags[t]; hasValue || rule.Flags[t] != "" {
					result.Flags[t] = tokens[i+1]
					i++
				} else {
					// 规则中没有该 flag，按启发式处理：
					// 如果下一个 token 不是 flag 且不以 - 开头，视为带值
					result.Flags[t] = tokens[i+1]
					i++
				}
			} else {
				result.Flags[t] = ""
			}
		} else {
			result.SubCommands = append(result.SubCommands, t)
		}
		i++
	}

	return result
}

// MatchCommandRule 检查实际命令是否匹配规则字符串
func MatchCommandRule(rule, command string) bool {
	parsedRule := ParseCommandRule(rule)
	if parsedRule.Program == "" {
		return false
	}

	parsedCmd := ParseActualCommand(command, parsedRule)
	if parsedCmd.Program == "" {
		return false
	}

	// 1. 程序名必须相同
	if parsedRule.Program != parsedCmd.Program {
		return false
	}

	// 2. 规则中所有子命令必须出现（顺序无关）
	for _, sub := range parsedRule.SubCommands {
		if !matchSubCommand(sub, parsedCmd.SubCommands) {
			return false
		}
	}

	// 3. 规则中所有 flag 必须匹配
	for flag, ruleVal := range parsedRule.Flags {
		actualVal, ok := parsedCmd.Flags[flag]
		if !ok {
			return false
		}
		if ruleVal != "" && ruleVal != "*" && !matchGlobPattern(ruleVal, actualVal) {
			return false
		}
	}

	// 4. 无通配符时，不允许多余子命令和多余 flag
	if !parsedRule.Wildcard {
		if len(parsedCmd.SubCommands) > len(parsedRule.SubCommands) {
			return false
		}
		// 检查是否有规则中未定义的 flag
		for flag := range parsedCmd.Flags {
			if _, ok := parsedRule.Flags[flag]; !ok {
				return false
			}
		}
	}

	return true
}

// --- 辅助函数 ---

func tokenize(s string) []string {
	fields := strings.Fields(s)
	result := make([]string, 0, len(fields))
	for _, f := range fields {
		result = append(result, expandShortFlag(f)...)
	}
	return result
}

// expandShortFlag 展开组合短 flag（如 -rf → -r, -f）
// 不展开：单字符 flag（-n）、长 flag（--verbose）、含 = 的 flag（-n=val）、非 flag
func expandShortFlag(token string) []string {
	if !strings.HasPrefix(token, "-") || strings.HasPrefix(token, "--") {
		return []string{token}
	}
	chars := token[1:]
	if len(chars) <= 1 || strings.Contains(token, "=") {
		return []string{token}
	}
	result := make([]string, len(chars))
	for i, c := range chars {
		result[i] = "-" + string(c)
	}
	return result
}

func isFlag(s string) bool {
	return strings.HasPrefix(s, "-")
}

func matchSubCommand(pattern string, subs []string) bool {
	for _, sub := range subs {
		if matchGlobPattern(pattern, sub) {
			return true
		}
	}
	return false
}

// matchGlobPattern 使用固定的 POSIX 路径分隔符语义做 glob 匹配，避免规则随本机 OS 改变。
func matchGlobPattern(pattern, value string) bool {
	matched, err := path.Match(pattern, value)
	if err != nil {
		return pattern == value
	}
	return matched
}

// allSubCommandsAllowed 检查所有子命令是否都匹配 allow 规则，返回是否全部匹配及命中的规则
func allSubCommandsAllowed(subCmds []string, allowRules []string) (bool, string) {
	if len(allowRules) == 0 {
		return false, ""
	}
	matchedRules := make(map[string]struct{})
	for _, cmd := range subCmds {
		matched := false
		for _, rule := range allowRules {
			if MatchCommandRule(rule, cmd) {
				matched = true
				matchedRules[rule] = struct{}{}
				break
			}
		}
		if !matched {
			return false, ""
		}
	}
	// 收集去重的匹配规则
	rules := make([]string, 0, len(matchedRules))
	for r := range matchedRules {
		rules = append(rules, r)
	}
	return true, strings.Join(rules, ", ")
}

// findHintRules 从 allow 规则中找同程序名的规则作为提示
func findHintRules(command string, allowRules []string) []string {
	tokens := tokenize(command)
	if len(tokens) == 0 {
		return nil
	}
	program := tokens[0]

	var hints []string
	for _, rule := range allowRules {
		ruleTokens := tokenize(rule)
		if len(ruleTokens) > 0 && ruleTokens[0] == program {
			hints = append(hints, rule)
		}
	}
	return hints
}
