package aiagent

import (
	"strings"
	"testing"
)

func TestStaticSystemPrompt_CoversAllSections(t *testing.T) {
	got := StaticSystemPrompt("zh-cn")
	wantSubstrings := []string{
		// opsIdentity
		"OpsKat AI assistant",
		"primary role",
		"Do NOT use bash for remote operations",
		// languageHint
		"Chinese (Simplified)",
		// opsToolPlaybook
		"OpsKat tool playbook",
		"list_assets",
		"batch_command",
		"request_permission",
		"exec_tool",
		// subagent routing 现在由 cago coding 动态 prompt 生成（含 dispatch_subagent +
		// explore/plan/general-purpose）；OpsKat 不再自己列 ops-* 子 agent，因为它们
		// 实际未注册。如果将来通过 coding.WithExtraSubagents 接回 ops-* 三种，再在此
		// 测试加守卫。
		// mentionProtocol
		"@-mention protocol",
		"Assets referenced in the user's message",
		// approvalContext
		"policy checker",
		"audit",
		// knowledgeGuidance
		"update_asset",
		"exec_k8s",
		// errorRecoveryGuidance
		"tool execution fails",
		// userDenialGuidance — match the actual phrasing in the prompt
		"user denies a command execution",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(got, want) {
			t.Errorf("missing keyword %q", want)
		}
	}
}

func TestStaticSystemPrompt_LanguageHintSwitches(t *testing.T) {
	cases := map[string]string{
		"zh-cn":     "Chinese (Simplified)",
		"en":        "respond in English",
		"something": "same language the user uses",
	}
	for lang, want := range cases {
		got := strings.ToLower(StaticSystemPrompt(lang))
		if !strings.Contains(got, strings.ToLower(want)) {
			t.Errorf("lang=%s: expected %q in prompt", lang, want)
		}
	}
}
