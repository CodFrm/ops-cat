package aiagent

import (
	"context"
	"testing"

	"github.com/cago-frame/agents/agent"
	"github.com/stretchr/testify/assert"
)

type fakeMentionResolver struct{}

func (f *fakeMentionResolver) Expand(ctx context.Context, raw string) (expanded string, mentions []map[string]any, openTabs []string, err error) {
	if raw == "@srv1 status" {
		return "[server srv1 (id=1)] status", []map[string]any{{"asset_id": 1, "asset_name": "srv1"}}, []string{"asset:1"}, nil
	}
	return raw, nil, nil, nil
}

type captureTabOpener struct{ opened []string }

func (c *captureTabOpener) Open(ctx context.Context, target string) error {
	c.opened = append(c.opened, target)
	return nil
}

func TestMentionsHook_RewritesAndOpensTab(t *testing.T) {
	opener := &captureTabOpener{}
	h := newMentionsHook(&fakeMentionResolver{}, opener)
	out, err := h(context.Background(), &agent.UserPromptInput{Text: "@srv1 status"})
	assert.NoError(t, err)
	assert.Equal(t, "[server srv1 (id=1)] status", out.ModifiedText)
	assert.Equal(t, []string{"asset:1"}, opener.opened)
}

func TestMentionsHook_NoMention_PassThrough(t *testing.T) {
	opener := &captureTabOpener{}
	h := newMentionsHook(&fakeMentionResolver{}, opener)
	out, err := h(context.Background(), &agent.UserPromptInput{Text: "no mention here"})
	assert.NoError(t, err)
	assert.Empty(t, out.ModifiedText)
	assert.Empty(t, opener.opened)
}
