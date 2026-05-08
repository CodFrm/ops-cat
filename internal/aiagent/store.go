package aiagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
	"github.com/opskat/opskat/internal/service/conversation_svc"
)

// convStore is the small subset of conversation_svc.ConversationSvc that
// gormStore needs. Lets tests inject a fake without touching globals.
type convStore interface {
	Get(ctx context.Context, id int64) (*conversation_entity.Conversation, error)
	Update(ctx context.Context, conv *conversation_entity.Conversation) error
}

// gormStore satisfies cago's agent.Store by reading/writing a JSON-serialized
// SessionData blob on the Conversation.SessionData column (a TEXT field).
//
// SessionID format: "conv_<conversationID>".
//
// This is intentionally separate from the conversation_messages table — that
// table is the UI display history, written by app_ai.go:SaveConversationMessages
// from the frontend. Mixing cago's internal Session state into that table
// would corrupt the UI's view (different schemas, different lifecycles).
type gormStore struct {
	store convStore
}

// NewGormStore wires gormStore to the singleton conversation service. Used
// from System construction.
func NewGormStore() agent.Store { return newGormStore(conversation_svc.Conversation()) }

// newGormStore is the test-friendly constructor.
func newGormStore(s convStore) *gormStore { return &gormStore{store: s} }

func (g *gormStore) Save(ctx context.Context, sessionID string, data agent.SessionData) error {
	convID, err := parseSessionID(sessionID)
	if err != nil {
		return err
	}
	conv, err := g.store.Get(ctx, convID)
	if err != nil {
		return fmt.Errorf("gormStore.Save: load conversation %d: %w", convID, err)
	}
	blob, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("gormStore.Save: marshal SessionData: %w", err)
	}
	conv.SessionData = string(blob)
	return g.store.Update(ctx, conv)
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
	if conv.SessionData == "" {
		return agent.SessionData{}, nil
	}
	var data agent.SessionData
	if err := json.Unmarshal([]byte(conv.SessionData), &data); err != nil {
		return agent.SessionData{}, fmt.Errorf("gormStore.Load: parse SessionData: %w", err)
	}
	return data, nil
}

func (g *gormStore) Delete(ctx context.Context, sessionID string) error {
	convID, err := parseSessionID(sessionID)
	if err != nil {
		return err
	}
	conv, err := g.store.Get(ctx, convID)
	if err != nil {
		return fmt.Errorf("gormStore.Delete: load conversation %d: %w", convID, err)
	}
	if conv.SessionData == "" {
		return nil
	}
	conv.SessionData = ""
	return g.store.Update(ctx, conv)
}

// parseSessionID parses cago Session IDs of the form "conv_<id>" into the
// underlying OpsKat conversation row id.
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
