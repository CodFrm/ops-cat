package aiagent

import (
	"context"

	"github.com/cago-frame/agents/agent"
)

// MentionResolver parses @mention tokens in raw user input. It returns:
//   - expanded: the LLM-bound text (mentions replaced with descriptive form,
//     e.g. inline <mention> XML tags)
//   - openTabs: target identifiers to open in the UI (e.g., "asset:42")
//
// If raw contains no mentions, expanded should equal raw and openTabs should
// be empty (caller treats this as pass-through).
//
// Mention 元数据本身通过 expanded 文本里的 inline `<mention>` 标签携带；
// gormStore.AppendMessage 在 user role 时反向 ai.ParseMentions 落到 row.Mentions。
// 不再走旁路（Manager.StashMentions 已删）。
type MentionResolver interface {
	Expand(ctx context.Context, raw string) (expanded string, openTabs []string, err error)
}

// TabOpener side-effects opening tabs in the desktop UI (asset detail / extension
// page / etc). Errors propagate up to the hook caller; cago will surface them
// as EventError.
type TabOpener interface {
	Open(ctx context.Context, target string) error
}

// newMentionsHook returns a UserPromptSubmit hook that:
//   - Calls resolver.Expand on the raw user text
//   - Opens each tab returned (best-effort; first error aborts)
//   - Sets UserPromptOutput.ModifiedText only if the expansion changed the text
func newMentionsHook(r MentionResolver, t TabOpener) agent.UserPromptHook {
	return func(ctx context.Context, in *agent.UserPromptInput) (*agent.UserPromptOutput, error) {
		expanded, tabs, err := r.Expand(ctx, in.Text)
		if err != nil {
			return nil, err
		}
		for _, target := range tabs {
			if err := t.Open(ctx, target); err != nil {
				return nil, err
			}
		}
		out := &agent.UserPromptOutput{}
		if expanded != in.Text {
			out.ModifiedText = expanded
		}
		return out, nil
	}
}
