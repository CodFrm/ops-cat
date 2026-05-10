package aiagent

import (
	"context"

	"github.com/cago-frame/agents/agent"
)

// MentionResolver parses @mention tokens in raw user input. It returns:
//   - expanded: the LLM-bound text (mentions replaced with descriptive form)
//   - mentions: structured records (asset_id, asset_name, etc.) for sidechannel
//     persistence — kept as []map[string]any to avoid coupling to ai package types
//   - openTabs: target identifiers to open in the UI (e.g., "asset:42")
//
// If raw contains no mentions, expanded should equal raw and mentions/openTabs
// should be empty (caller treats this as pass-through).
type MentionResolver interface {
	Expand(ctx context.Context, raw string) (expanded string, mentions []map[string]any, openTabs []string, err error)
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
//
// Note: cago's UserPromptOutput cannot inject extra ContentBlocks into the user
// message (it's text-only). The mentions slice returned by Expand is therefore
// NOT routed through this hook — Task 21's Manager.pendingMentions sidechannel
// handles that. Future cago PR (UserPromptOutput.AdditionalBlocks) will let us
// inline this cleanly.
func newMentionsHook(r MentionResolver, t TabOpener) agent.UserPromptHook {
	return func(ctx context.Context, in *agent.UserPromptInput) (*agent.UserPromptOutput, error) {
		expanded, _, tabs, err := r.Expand(ctx, in.Text)
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
