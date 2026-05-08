package aiagent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
)

// fakeConvStore covers the convStore interface. Each method delegates to
// in-memory state so tests don't need a real DB.
type fakeConvStore struct {
	row      *conversation_entity.Conversation
	messages []*conversation_entity.Message
	getErr   error
	upsErr   error
	stateErr error
	listErr  error
}

func (f *fakeConvStore) Get(_ context.Context, id int64) (*conversation_entity.Conversation, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.row == nil || f.row.ID != id {
		return nil, errors.New("not found")
	}
	cp := *f.row
	return &cp, nil
}

func (f *fakeConvStore) UpsertMessages(_ context.Context, id int64, msgs []*conversation_entity.Message) error {
	if f.upsErr != nil {
		return f.upsErr
	}
	keep := map[string]bool{}
	for _, m := range msgs {
		keep[m.CagoID] = true
	}
	out := make([]*conversation_entity.Message, 0, len(f.messages))
	for _, existing := range f.messages {
		if keep[existing.CagoID] {
			out = append(out, existing)
		}
	}
	f.messages = out
	for _, m := range msgs {
		var found *conversation_entity.Message
		for _, e := range f.messages {
			if e.ConversationID == id && e.CagoID == m.CagoID {
				found = e
				break
			}
		}
		if found == nil {
			cp := *m
			f.messages = append(f.messages, &cp)
			continue
		}
		// 更新 cago 字段；保留 mentions / token_usage（System 写入路径模拟）
		found.Role = m.Role
		found.Content = m.Content
		found.Kind = m.Kind
		found.Origin = m.Origin
		found.ParentID = m.ParentID
		found.Thinking = m.Thinking
		found.ToolCallJSON = m.ToolCallJSON
		found.ToolResultJSON = m.ToolResultJSON
		found.Persist = m.Persist
		found.Raw = m.Raw
		found.MsgTime = m.MsgTime
		found.SortOrder = m.SortOrder
		if m.Mentions != "" {
			found.Mentions = m.Mentions
		}
	}
	return nil
}

func (f *fakeConvStore) UpdateConversationState(_ context.Context, id int64, threadID string, vals map[string]string) error {
	if f.stateErr != nil {
		return f.stateErr
	}
	if f.row == nil || f.row.ID != id {
		return errors.New("conv not found")
	}
	f.row.ThreadID = threadID
	if len(vals) == 0 {
		f.row.StateValues = ""
	} else {
		b, _ := json.Marshal(vals)
		f.row.StateValues = string(b)
	}
	return nil
}

func (f *fakeConvStore) LoadMessages(_ context.Context, id int64) ([]*conversation_entity.Message, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]*conversation_entity.Message, 0, len(f.messages))
	for _, m := range f.messages {
		if m.ConversationID == id {
			out = append(out, m)
		}
	}
	return out, nil
}

func (f *fakeConvStore) UpdateMessageTokenUsage(_ context.Context, id int64, cagoID, tokenUsageJSON string) error {
	for _, m := range f.messages {
		if m.ConversationID == id && m.CagoID == cagoID {
			m.TokenUsage = tokenUsageJSON
			return nil
		}
	}
	// missing cago_id is silent no-op (matches repo semantics)
	return nil
}

func TestGormStore_RoundTripsCagoMessages(t *testing.T) {
	fake := &fakeConvStore{row: &conversation_entity.Conversation{ID: 42}}
	st := newGormStore(fake)
	data := agent.SessionData{
		Messages: []agent.Message{
			{ID: "m1", Kind: agent.MessageKindText, Role: agent.RoleUser, Origin: agent.MessageOriginUser, Text: "hello", Persist: true},
			{ID: "m2", Kind: agent.MessageKindToolCall, Role: agent.RoleAssistant, Origin: agent.MessageOriginModel, Persist: true,
				ToolCall: &agent.ToolCall{ID: "call-1", Name: "list_assets", Args: json.RawMessage(`{}`)}},
			{ID: "m3", Kind: agent.MessageKindToolResult, Role: agent.RoleTool, Origin: agent.MessageOriginTool, Persist: true,
				ToolResult: &agent.ToolResult{Result: "ok"}},
		},
		State: agent.State{ThreadID: "thread-xyz", Values: map[string]string{"k": "v"}},
	}

	if err := st.Save(context.Background(), "conv_42", data); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(context.Background(), "conv_42")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Messages) != 3 {
		t.Fatalf("Messages len = %d, want 3", len(got.Messages))
	}
	if got.Messages[0].ID != "m1" || got.Messages[0].Text != "hello" {
		t.Errorf("text msg lost: %+v", got.Messages[0])
	}
	if got.Messages[1].ToolCall == nil || got.Messages[1].ToolCall.Name != "list_assets" {
		t.Errorf("tool_call lost: %+v", got.Messages[1])
	}
	if got.Messages[2].ToolResult == nil {
		t.Fatalf("tool_result lost: %+v", got.Messages[2])
	}
	if s, ok := got.Messages[2].ToolResult.Result.(string); !ok || s != "ok" {
		t.Errorf("tool_result.Result lost: %#v", got.Messages[2].ToolResult.Result)
	}
	if got.State.ThreadID != "thread-xyz" || got.State.Values["k"] != "v" {
		t.Errorf("state lost: %+v", got.State)
	}
}

func TestGormStore_UpsertReplacesOnSecondSave(t *testing.T) {
	fake := &fakeConvStore{row: &conversation_entity.Conversation{ID: 1}}
	st := newGormStore(fake)
	first := agent.SessionData{Messages: []agent.Message{
		{ID: "m1", Kind: agent.MessageKindText, Role: agent.RoleUser, Text: "v1", Persist: true},
		{ID: "m2", Kind: agent.MessageKindText, Role: agent.RoleAssistant, Text: "a1", Persist: true},
	}}
	second := agent.SessionData{Messages: []agent.Message{
		{ID: "m1", Kind: agent.MessageKindText, Role: agent.RoleUser, Text: "v1-edited", Persist: true},
		{ID: "m3", Kind: agent.MessageKindText, Role: agent.RoleAssistant, Text: "a2", Persist: true},
	}}
	if err := st.Save(context.Background(), "conv_1", first); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(context.Background(), "conv_1", second); err != nil {
		t.Fatal(err)
	}
	got, _ := st.Load(context.Background(), "conv_1")
	if len(got.Messages) != 2 {
		t.Fatalf("want 2, got %d", len(got.Messages))
	}
	if got.Messages[0].Text != "v1-edited" || got.Messages[1].ID != "m3" {
		t.Errorf("upsert wrong: %+v", got.Messages)
	}
}

func TestGormStore_LoadEmpty(t *testing.T) {
	fake := &fakeConvStore{row: &conversation_entity.Conversation{ID: 1}}
	st := newGormStore(fake)
	got, err := st.Load(context.Background(), "conv_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Messages) != 0 || got.State.ThreadID != "" {
		t.Errorf("expected zero, got %+v", got)
	}
}

func TestGormStore_DeleteClearsRowsAndState(t *testing.T) {
	fake := &fakeConvStore{row: &conversation_entity.Conversation{ID: 1, ThreadID: "t"}}
	fake.messages = []*conversation_entity.Message{{ConversationID: 1, CagoID: "m1", Persist: true}}
	st := newGormStore(fake)
	if err := st.Delete(context.Background(), "conv_1"); err != nil {
		t.Fatal(err)
	}
	if len(fake.messages) != 0 {
		t.Errorf("messages not cleared: %+v", fake.messages)
	}
	if fake.row.ThreadID != "" {
		t.Errorf("thread_id not cleared: %q", fake.row.ThreadID)
	}
}

func TestGormStore_RejectsBadSessionID(t *testing.T) {
	fake := &fakeConvStore{}
	st := newGormStore(fake)
	if err := st.Save(context.Background(), "bogus", agent.SessionData{}); err == nil {
		t.Error("Save with bad id: want error")
	}
	if _, err := st.Load(context.Background(), "bogus"); err == nil {
		t.Error("Load with bad id: want error")
	}
	if err := st.Delete(context.Background(), "bogus"); err == nil {
		t.Error("Delete with bad id: want error")
	}
}
