package aiagent

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/agents/provider"
	"github.com/cago-frame/agents/provider/providertest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
)

// TestGormStoreE2E_BuiltinTurnPreservesAllMessages 复现 OpsKat 历史 bug：
// cago builtin backend 跑完一轮后，gormStore.Save 落盘的 user + assistant 两条
// 消息必须都在 DB，不能因为 cago_id 自然键 upsert 互相覆盖只剩最后一条。
//
// 这是 cago 端 newMessageID 兜底 + opskat 端 (conversation_id, cago_id) upsert
// 的端到端契约：上游 framework 给每条 Message 分配稳定 ID，下游 store 的自然键
// upsert 才能不打架。
func TestGormStoreE2E_BuiltinTurnPreservesAllMessages(t *testing.T) {
	ctx, gdb := setupE2E(t)
	c := newE2EConv(t, ctx)

	mock := providertest.New().QueueStream(
		provider.StreamChunk{ContentDelta: "world"},
		provider.StreamChunk{FinishReason: provider.FinishStop, Usage: &provider.Usage{TotalTokens: 1}},
	)
	store := NewGormStore(nil, nil)

	a := agent.NewWithBackend(agent.NewBuiltinBackend(mock))
	sess := a.Session(
		agent.WithStore(store),
		agent.WithID(fmt.Sprintf("conv_%d", c.ID)),
	)

	streamCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	stream, err := sess.Stream(streamCtx, "hello")
	require.NoError(t, err)
	for stream.Next() {
	}
	select {
	case <-stream.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("stream.Done() never closed")
	}

	var rows []conversation_entity.Message
	require.NoError(t, gdb.Where("conversation_id = ?", c.ID).Order("sort_order ASC").Find(&rows).Error)

	require.GreaterOrEqual(t, len(rows), 2,
		"expected at least 2 rows (user + assistant); only %d survived → empty cago_id collision regression", len(rows))

	var sawUser, sawAssistant bool
	seenIDs := make(map[string]int)
	for i, row := range rows {
		assert.NotEmpty(t, row.CagoID,
			"row %d (role=%s text=%q) has empty cago_id; cago framework must mint Message.ID", i, row.Role, row.Content)
		if prev, dup := seenIDs[row.CagoID]; dup && row.CagoID != "" {
			t.Errorf("row %d.cago_id=%q duplicates row %d", i, row.CagoID, prev)
		}
		seenIDs[row.CagoID] = i
		if row.Role == "user" && row.Content == "hello" {
			sawUser = true
		}
		if row.Role == "assistant" && row.Content == "world" {
			sawAssistant = true
		}
	}
	assert.True(t, sawUser, "user row missing; rows=%+v", rows)
	assert.True(t, sawAssistant, "assistant row missing; rows=%+v", rows)
}
