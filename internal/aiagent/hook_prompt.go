package aiagent

import (
	"context"
	"strings"
	"sync"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/ai"
)

// PerTurnState carries the latest aiCtx + extension SKILL.md table for the
// next stream call. SendAIMessage updates it before each Stream(...) call.
// One PerTurnState per *coding.System.
type PerTurnState struct {
	mu        sync.Mutex
	aiCtx     ai.AIContext
	extSkills map[string]string // ext name → SKILL.md
}

func (s *PerTurnState) Set(c ai.AIContext, ext map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.aiCtx = c
	s.extSkills = ext
}

func (s *PerTurnState) snapshot() (ai.AIContext, map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.aiCtx, s.extSkills
}

// makePromptHook returns a UserPromptSubmit hook that reads the latest
// PerTurnState and assembles an AdditionalContext bundle covering open tabs,
// @-mentions, and per-extension SKILL.md fragments.
func makePromptHook(state *PerTurnState) agent.HookFunc {
	return func(_ context.Context, _ agent.HookInput) (*agent.HookOutput, error) {
		c, ext := state.snapshot()

		var parts []string
		if t := buildTabContext(c.OpenTabs); t != "" {
			parts = append(parts, t)
		}
		if m := ai.RenderMentionContext(c.MentionedAssets); m != "" {
			parts = append(parts, m)
		}
		if len(ext) > 0 {
			for name, md := range ext {
				parts = append(parts, "## From extension: "+name+"\n"+md)
			}
		}
		if len(parts) == 0 {
			return nil, nil
		}
		return agent.AddContext(strings.Join(parts, "\n\n")), nil
	}
}

func buildTabContext(tabs []ai.TabInfo) string {
	if len(tabs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("The user currently has these tabs open:\n")
	for _, t := range tabs {
		typeName := t.Type
		switch t.Type {
		case "ssh":
			typeName = "SSH Terminal"
		case "database":
			typeName = "Database Query"
		case "redis":
			typeName = "Redis"
		case "sftp":
			typeName = "SFTP"
		}
		b.WriteString("- ")
		b.WriteString(typeName)
		b.WriteString(": \"")
		b.WriteString(t.AssetName)
		b.WriteString("\"\n")
	}
	return b.String()
}
