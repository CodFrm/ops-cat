package aiagent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
)

// fakeConvStore is an in-memory stand-in for conversation_svc/conversation_repo
// access — avoids a real DB while still exercising the JSON envelope path.
type fakeConvStore struct {
	row    *conversation_entity.Conversation
	getErr error
	updErr error
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

func (f *fakeConvStore) Update(_ context.Context, conv *conversation_entity.Conversation) error {
	if f.updErr != nil {
		return f.updErr
	}
	cp := *conv
	f.row = &cp
	return nil
}

func TestGormStore_RoundTripsSessionData(t *testing.T) {
	fake := &fakeConvStore{row: &conversation_entity.Conversation{ID: 42}}
	st := newGormStore(fake)

	data := agent.SessionData{
		Messages: []agent.Message{
			{Kind: agent.MessageKindText, Role: agent.RoleUser, Text: "hello"},
			{Kind: agent.MessageKindToolCall, Role: agent.RoleAssistant, ToolCall: &agent.ToolCall{
				ID: "call-1", Name: "list_assets", Args: json.RawMessage(`{}`),
			}},
			{Kind: agent.MessageKindToolResult, Role: agent.RoleTool, ToolResult: &agent.ToolResult{
				Result: "ok",
			}},
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
		t.Fatalf("Messages length = %d, want 3", len(got.Messages))
	}
	if got.Messages[1].ToolCall == nil || got.Messages[1].ToolCall.Name != "list_assets" {
		t.Errorf("tool_call lost: %+v", got.Messages[1])
	}
	if got.Messages[2].ToolResult == nil || got.Messages[2].ToolResult.Result != "ok" {
		t.Errorf("tool_result lost: %+v", got.Messages[2])
	}
	if got.State.ThreadID != "thread-xyz" {
		t.Errorf("ThreadID lost: %q", got.State.ThreadID)
	}
	if got.State.Values["k"] != "v" {
		t.Errorf("State.Values lost: %+v", got.State.Values)
	}
}

func TestGormStore_LoadEmptyReturnsZero(t *testing.T) {
	fake := &fakeConvStore{row: &conversation_entity.Conversation{ID: 1}}
	st := newGormStore(fake)
	got, err := st.Load(context.Background(), "conv_1")
	if err != nil {
		t.Fatalf("Load on empty session_data: %v", err)
	}
	if len(got.Messages) != 0 || got.State.ThreadID != "" {
		t.Errorf("expected zero SessionData, got %+v", got)
	}
}

func TestGormStore_DeleteClearsSessionData(t *testing.T) {
	fake := &fakeConvStore{row: &conversation_entity.Conversation{
		ID: 1, SessionData: `{"Messages":[{"Kind":"text"}]}`,
	}}
	st := newGormStore(fake)

	if err := st.Delete(context.Background(), "conv_1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if fake.row.SessionData != "" {
		t.Errorf("SessionData not cleared: %q", fake.row.SessionData)
	}
}

func TestGormStore_RejectsBadSessionID(t *testing.T) {
	fake := &fakeConvStore{}
	st := newGormStore(fake)

	if err := st.Save(context.Background(), "bogus", agent.SessionData{}); err == nil {
		t.Error("Save with bad id: want error, got nil")
	}
	if _, err := st.Load(context.Background(), "bogus"); err == nil {
		t.Error("Load with bad id: want error, got nil")
	}
	if err := st.Delete(context.Background(), "bogus"); err == nil {
		t.Error("Delete with bad id: want error, got nil")
	}
}
