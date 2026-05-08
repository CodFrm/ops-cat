package aiagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/ai"
	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
	"github.com/opskat/opskat/internal/service/conversation_svc"
)

// convStore 是 gormStore 需要的 conversation_svc 子集。Tests 通过这个接口注入
// fake 实现，避免起 DB。
type convStore interface {
	Get(ctx context.Context, id int64) (*conversation_entity.Conversation, error)
	UpsertCagoMessages(ctx context.Context, conversationID int64, msgs []*conversation_entity.Message) error
	UpdateConversationState(ctx context.Context, conversationID int64, threadID string, values map[string]string) error
	LoadMessages(ctx context.Context, conversationID int64) ([]*conversation_entity.Message, error)
	UpdateMessageTokenUsage(ctx context.Context, conversationID int64, cagoID, tokenUsageJSON string) error
}

// pendingMentionsProvider 由 *System 实现：SendAIMessage 入口把当前轮的 mentions
// stash 进 System，gormStore 在 Save 同事务下把它绑到刚出现的 user 行。Task 5
// 只定义这个接口与挂载点；System 端的 stash/pop 在 Task 6 实现。
type pendingMentionsProvider interface {
	popPendingMentions() []ai.MentionedAsset
}

// pendingUsageProvider 由 *System 实现：event_bridge.EventUsage 把每轮的 token usage
// stash 进 System；gormStore.Save drain map 后按 cago_id 写到对应 message 行的
// token_usage 列。和 mentions 一样，所有持久化都在 cago 框架保护的 saveCtx 下完成。
type pendingUsageProvider interface {
	drainPendingUsage() map[string]*conversation_entity.TokenUsage
}

// gormStore 把 cago agent.SessionData 平铺到 conversation_messages（按 cago_id
// 行级 upsert）与 conversations.thread_id/state_values 上。它取代了原本写到
// conversations.session_data 的 JSON-blob 实现。
//
// 不变量：
//   - cago_id 即 cago Message.ID，在一次 Session 生命周期内稳定
//   - cago.Save 每次发的是全量 SessionData；本地 upsert 必须以"删除不在快照里的旧行"作为收敛
//   - 不需要 detachCtx：cago internalObserver.persist 已用 WithoutCancel +
//     saveTimeout 包过 ctx，本层直接用传入的 ctx
//   - mentions 列由 System 的 pendingMentions 缓存，在 Save 同事务里 drain 进刚
//     出现的 user 行；这里不主动写 mentions 的其他规则
type gormStore struct {
	store    convStore
	mentions pendingMentionsProvider // 可为 nil（Task 5 阶段、纯单测场景）
	usage    pendingUsageProvider    // 可为 nil（纯单测场景）
}

// NewGormStore 接 service 单例。mentions / usage 都由 *System 提供；
// 单测场景可传 nil 跳过对应 drain。
func NewGormStore(mentions pendingMentionsProvider, usage pendingUsageProvider) agent.Store {
	return &gormStore{store: conversation_svc.Conversation(), mentions: mentions, usage: usage}
}

// newGormStore 是测试构造器，接 fake convStore；mentions / usage 默认 nil。
func newGormStore(s convStore) *gormStore { return &gormStore{store: s} }

func (g *gormStore) Save(ctx context.Context, sessionID string, data agent.SessionData) error {
	convID, err := parseSessionID(sessionID)
	if err != nil {
		return err
	}

	rows := make([]*conversation_entity.Message, 0, len(data.Messages))
	for i, m := range data.Messages {
		row, err := messageToRow(convID, i, m)
		if err != nil {
			return fmt.Errorf("gormStore.Save: convert message %d: %w", i, err)
		}
		rows = append(rows, row)
	}

	// drain pendingMentions：把上一次 SendAIMessage stash 的 mentions 关联到
	// 刚出现的 user 行（即在历史里没出现过的最后一条 origin=user 行）。
	if g.mentions != nil {
		existing, _ := g.store.LoadMessages(ctx, convID)
		known := make(map[string]bool, len(existing))
		for _, e := range existing {
			known[e.CagoID] = true
		}
		for i := len(rows) - 1; i >= 0; i-- {
			r := rows[i]
			if r.Origin == string(agent.MessageOriginUser) && !known[r.CagoID] {
				if pending := g.mentions.popPendingMentions(); len(pending) > 0 {
					refs := make([]conversation_entity.MentionRef, 0, len(pending))
					for _, m := range pending {
						refs = append(refs, conversation_entity.MentionRef{AssetID: m.AssetID, Name: m.Name})
					}
					b, _ := json.Marshal(refs)
					r.Mentions = string(b)
				}
				break
			}
		}
	}

	if err := g.store.UpsertCagoMessages(ctx, convID, rows); err != nil {
		return fmt.Errorf("gormStore.Save: upsert messages: %w", err)
	}
	if err := g.store.UpdateConversationState(ctx, convID, data.State.ThreadID, data.State.Values); err != nil {
		return fmt.Errorf("gormStore.Save: update state: %w", err)
	}
	// drain pendingUsage：bridge.EventUsage 已按 lastAssistantMsgID 把 usage stash
	// 进 System；这里按 cago_id 把对应行的 token_usage 列写一次。写不存在的
	// cago_id 在 repo 层是 silent no-op，符合预期（行未被 upsert 时跳过）。
	if g.usage != nil {
		for cagoID, u := range g.usage.drainPendingUsage() {
			b, err := json.Marshal(u)
			if err != nil {
				return fmt.Errorf("gormStore.Save: marshal token_usage for %s: %w", cagoID, err)
			}
			if err := g.store.UpdateMessageTokenUsage(ctx, convID, cagoID, string(b)); err != nil {
				return fmt.Errorf("gormStore.Save: update token_usage for %s: %w", cagoID, err)
			}
		}
	}
	return nil
}

func (g *gormStore) Load(ctx context.Context, sessionID string) (agent.SessionData, error) {
	convID, err := parseSessionID(sessionID)
	if err != nil {
		return agent.SessionData{}, err
	}
	conv, err := g.store.Get(ctx, convID)
	if err != nil {
		return agent.SessionData{}, fmt.Errorf("gormStore.Load: load conversation %d: %w", convID, err)
	}
	rows, err := g.store.LoadMessages(ctx, convID)
	if err != nil {
		return agent.SessionData{}, fmt.Errorf("gormStore.Load: load messages: %w", err)
	}
	values, err := conv.GetStateValues()
	if err != nil {
		return agent.SessionData{}, fmt.Errorf("gormStore.Load: parse state values: %w", err)
	}
	out := agent.SessionData{State: agent.State{ThreadID: conv.ThreadID, Values: values}}
	for _, row := range rows {
		msg, err := rowToMessage(row)
		if err != nil {
			return agent.SessionData{}, fmt.Errorf("gormStore.Load: convert row %d: %w", row.ID, err)
		}
		out.Messages = append(out.Messages, msg)
	}
	return out, nil
}

func (g *gormStore) Delete(ctx context.Context, sessionID string) error {
	convID, err := parseSessionID(sessionID)
	if err != nil {
		return err
	}
	if err := g.store.UpsertCagoMessages(ctx, convID, nil); err != nil {
		return fmt.Errorf("gormStore.Delete: clear messages: %w", err)
	}
	if err := g.store.UpdateConversationState(ctx, convID, "", nil); err != nil {
		return fmt.Errorf("gormStore.Delete: clear state: %w", err)
	}
	return nil
}

func parseSessionID(s string) (int64, error) {
	if !strings.HasPrefix(s, "conv_") {
		return 0, fmt.Errorf("gormStore: invalid session id %q (want conv_<id>)", s)
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(s, "conv_"), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("gormStore: invalid session id %q: %w", s, err)
	}
	return id, nil
}

// messageToRow 把 cago.Message 平铺成 conversation_messages 行（不动 mentions/token_usage 扩展列）。
func messageToRow(convID int64, idx int, m agent.Message) (*conversation_entity.Message, error) {
	row := &conversation_entity.Message{
		ConversationID: convID,
		CagoID:         m.ID,
		ParentID:       m.ParentID,
		Kind:           string(m.Kind),
		Origin:         string(m.Origin),
		Role:           string(m.Role),
		Content:        m.Text,
		Persist:        m.Persist,
		SortOrder:      idx,
	}
	if !m.Time.IsZero() {
		row.MsgTime = m.Time.Unix()
	}
	if len(m.Thinking) > 0 {
		b, err := json.Marshal(m.Thinking)
		if err != nil {
			return nil, fmt.Errorf("marshal thinking: %w", err)
		}
		row.Thinking = string(b)
	}
	if m.ToolCall != nil {
		b, err := json.Marshal(m.ToolCall)
		if err != nil {
			return nil, fmt.Errorf("marshal tool_call: %w", err)
		}
		row.ToolCallJSON = string(b)
	}
	if m.ToolResult != nil {
		// ToolResult.Err 是 error 接口，json.Marshal 会丢成 {}；编码成纯字符串字段。
		type wireResult struct {
			Result   any           `json:"result,omitempty"`
			Err      string        `json:"err,omitempty"`
			Duration time.Duration `json:"duration,omitempty"`
		}
		w := wireResult{Result: m.ToolResult.Result, Duration: m.ToolResult.Duration}
		if m.ToolResult.Err != nil {
			w.Err = m.ToolResult.Err.Error()
		}
		b, err := json.Marshal(w)
		if err != nil {
			return nil, fmt.Errorf("marshal tool_result: %w", err)
		}
		row.ToolResultJSON = string(b)
	}
	if len(m.Raw) > 0 {
		row.Raw = string(m.Raw)
	}
	return row, nil
}

// rowToMessage 反向：从 DB 行重建 cago.Message。Load 路径用。
func rowToMessage(row *conversation_entity.Message) (agent.Message, error) {
	msg := agent.Message{
		ID:       row.CagoID,
		ParentID: row.ParentID,
		Kind:     agent.MessageKind(row.Kind),
		Origin:   agent.MessageOrigin(row.Origin),
		Role:     agent.MessageRole(row.Role),
		Text:     row.Content,
		Persist:  row.Persist,
	}
	if row.MsgTime > 0 {
		msg.Time = time.Unix(row.MsgTime, 0)
	}
	if row.Thinking != "" {
		if err := json.Unmarshal([]byte(row.Thinking), &msg.Thinking); err != nil {
			return agent.Message{}, fmt.Errorf("unmarshal thinking: %w", err)
		}
	}
	if row.ToolCallJSON != "" {
		var tc agent.ToolCall
		if err := json.Unmarshal([]byte(row.ToolCallJSON), &tc); err != nil {
			return agent.Message{}, fmt.Errorf("unmarshal tool_call: %w", err)
		}
		msg.ToolCall = &tc
	}
	if row.ToolResultJSON != "" {
		type wireResult struct {
			Result   any           `json:"result,omitempty"`
			Err      string        `json:"err,omitempty"`
			Duration time.Duration `json:"duration,omitempty"`
		}
		var w wireResult
		if err := json.Unmarshal([]byte(row.ToolResultJSON), &w); err != nil {
			return agent.Message{}, fmt.Errorf("unmarshal tool_result: %w", err)
		}
		msg.ToolResult = &agent.ToolResult{Result: w.Result, Duration: w.Duration}
		if w.Err != "" {
			msg.ToolResult.Err = fmt.Errorf("%s", w.Err)
		}
	}
	if row.Raw != "" {
		msg.Raw = json.RawMessage(row.Raw)
	}
	return msg, nil
}
