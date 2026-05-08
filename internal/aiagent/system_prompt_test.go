package aiagent

import (
	"strings"
	"testing"
)

func TestStaticSystemPrompt_Has5Sections(t *testing.T) {
	got := StaticSystemPrompt("zh-cn")
	for _, want := range []string{
		"OpsKat AI assistant",
		"Chinese (Simplified)",
		"update_asset",
		"tool execution fails",
		"user denies",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing section keyword %q", want)
		}
	}
}
