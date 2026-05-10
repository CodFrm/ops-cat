package aiagent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/cago-frame/agents/agent"
	agentstore "github.com/cago-frame/agents/agent/store"

	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
	"github.com/opskat/opskat/internal/repository/conversation_repo"
)

// gormStore 把 cago agent.Conversation 的增量变更落到 conversation_messages 的
// (conversation_id, sort_order) 行上。配合 agent/store.Recorder 监听 Conv.Watch 使用。
type gormStore struct {
	repo conversation_repo.ConversationRepo
}

// NewGormStore returns an agent/store.Store backed by OpsKat's conversation_repo.
func NewGormStore() agentstore.Store {
	return &gormStore{repo: conversation_repo.Conversation()}
}

// newGormStoreWithRepo lets tests inject a fake repo without touching the global registry.
func newGormStoreWithRepo(r conversation_repo.ConversationRepo) agentstore.Store {
	return &gormStore{repo: r}
}

func parseSessionID(s string) (int64, error) {
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("aiagent: bad session id %q: %w", s, err)
	}
	return id, nil
}

func (g *gormStore) AppendMessage(ctx context.Context, sessionID string, index int, msg agent.Message) error {
	convID, err := parseSessionID(sessionID)
	if err != nil {
		return err
	}
	row, err := messageToRow(convID, index, msg)
	if err != nil {
		return err
	}
	return g.repo.AppendAt(ctx, convID, index, row)
}

func (g *gormStore) UpdateMessage(ctx context.Context, sessionID string, index int, msg agent.Message) error {
	convID, err := parseSessionID(sessionID)
	if err != nil {
		return err
	}
	row, err := messageToRow(convID, index, msg)
	if err != nil {
		return err
	}
	return g.repo.UpdateAt(ctx, convID, index, row)
}

func (g *gormStore) TruncateAfter(ctx context.Context, sessionID string, index int) error {
	convID, err := parseSessionID(sessionID)
	if err != nil {
		return err
	}
	return g.repo.TruncateFrom(ctx, convID, index)
}

func (g *gormStore) LoadConversation(ctx context.Context, sessionID string) ([]agent.Message, agentstore.BranchInfo, error) {
	convID, err := parseSessionID(sessionID)
	if err != nil {
		return nil, agentstore.BranchInfo{}, err
	}
	rows, err := g.repo.LoadOrdered(ctx, convID)
	if err != nil {
		return nil, agentstore.BranchInfo{}, err
	}
	if len(rows) == 0 {
		return nil, agentstore.BranchInfo{}, nil
	}
	out := make([]agent.Message, 0, len(rows))
	for _, r := range rows {
		m, err := rowToMessage(r)
		if err != nil {
			return nil, agentstore.BranchInfo{}, err
		}
		out = append(out, m)
	}
	// crash-recovery invariant：尾部 PartialStreaming 残留改写为 errored，让接下
	// 来的 turn 把它当作历史而不是当前正在输出的消息。
	if n := len(out); n > 0 && out[n-1].PartialReason == agent.PartialStreaming {
		out[n-1].PartialReason = agent.PartialErrored
	}
	return out, agentstore.BranchInfo{}, nil
}

// messageToRow 把 cago Message 序列化为 DB 行。
//   - blocks: 全部 ContentBlock JSON（含 MetadataBlock）
//   - mentions: 从 MetadataBlock{Key:"mentions"} 提取并序列化
//   - token_usage: msg.Usage JSON（仅 assistant 非 nil 时填）
func messageToRow(convID int64, index int, m agent.Message) (*conversation_entity.Message, error) {
	blocksJSON, err := json.Marshal(serializeBlocks(m.Content))
	if err != nil {
		return nil, fmt.Errorf("marshal blocks: %w", err)
	}
	mentions := extractMentions(m.Content)
	mentionsJSON, err := json.Marshal(mentions)
	if err != nil {
		return nil, fmt.Errorf("marshal mentions: %w", err)
	}
	var usageJSON string
	if m.Usage != nil {
		b, err := json.Marshal(m.Usage)
		if err != nil {
			return nil, fmt.Errorf("marshal usage: %w", err)
		}
		usageJSON = string(b)
	}
	created := m.CreatedAt
	if created.IsZero() {
		created = time.Now()
	}
	return &conversation_entity.Message{
		ConversationID: convID,
		Role:           string(m.Role),
		Blocks:         string(blocksJSON),
		Mentions:       string(mentionsJSON),
		TokenUsage:     usageJSON,
		PartialReason:  m.PartialReason,
		SortOrder:      index,
		Createtime:     created.Unix(),
	}, nil
}

func rowToMessage(r *conversation_entity.Message) (agent.Message, error) {
	var rawBlocks []map[string]any
	if r.Blocks != "" {
		if err := json.Unmarshal([]byte(r.Blocks), &rawBlocks); err != nil {
			return agent.Message{}, fmt.Errorf("unmarshal blocks: %w", err)
		}
	}
	content, err := deserializeBlocks(rawBlocks)
	if err != nil {
		return agent.Message{}, err
	}
	var usage *agent.Usage
	if r.TokenUsage != "" {
		var u agent.Usage
		if err := json.Unmarshal([]byte(r.TokenUsage), &u); err != nil {
			return agent.Message{}, fmt.Errorf("unmarshal usage: %w", err)
		}
		usage = &u
	}
	created := time.Unix(r.Createtime, 0)
	return agent.Message{
		Role:          agent.Role(r.Role),
		Content:       content,
		CreatedAt:     created,
		PartialReason: r.PartialReason,
		Usage:         usage,
	}, nil
}

// serializeBlocks 把 ContentBlock slice 序列化成 [{"type":..., ...}, ...]。
// tag 用 ContentBlockType()，便于 deserializeBlocks 反向构造。
//
// 重要：text 块 key 用 "text"（与 migration 202605100001 backfill 对齐）。
func serializeBlocks(blocks []agent.ContentBlock) []map[string]any {
	out := make([]map[string]any, 0, len(blocks))
	for _, b := range blocks {
		entry := map[string]any{"type": b.ContentBlockType()}
		switch v := b.(type) {
		case agent.TextBlock:
			entry["text"] = v.Text
		case agent.ToolUseBlock:
			entry["id"] = v.ID
			entry["name"] = v.Name
			entry["input"] = v.Input
			entry["raw_args"] = v.RawArgs
		case agent.ToolResultBlock:
			entry["tool_use_id"] = v.ToolUseID
			entry["is_error"] = v.IsError
			entry["content"] = serializeBlocks(v.Content)
		case agent.ThinkingBlock:
			entry["text"] = v.Text
			entry["signature"] = v.Signature
		case agent.MetadataBlock:
			entry["key"] = v.Key
			entry["value"] = v.Value
		case agent.ImageBlock:
			entry["media_type"] = v.MediaType
			entry["url"] = v.Source.URL
			if len(v.Source.Inline) > 0 {
				entry["inline"] = v.Source.Inline // json.Marshal encodes []byte as base64 automatically
			}
		}
		out = append(out, entry)
	}
	return out
}

func deserializeBlocks(raw []map[string]any) ([]agent.ContentBlock, error) {
	out := make([]agent.ContentBlock, 0, len(raw))
	for _, r := range raw {
		switch r["type"] {
		case "text":
			out = append(out, agent.TextBlock{Text: asString(r["text"])})
		case "tool_use":
			input, _ := r["input"].(map[string]any)
			out = append(out, agent.ToolUseBlock{
				ID:      asString(r["id"]),
				Name:    asString(r["name"]),
				Input:   input,
				RawArgs: asString(r["raw_args"]),
			})
		case "tool_result":
			subRaw, _ := r["content"].([]any)
			subTyped := make([]map[string]any, 0, len(subRaw))
			for _, x := range subRaw {
				if m, ok := x.(map[string]any); ok {
					subTyped = append(subTyped, m)
				}
			}
			sub, err := deserializeBlocks(subTyped)
			if err != nil {
				return nil, err
			}
			isErr, _ := r["is_error"].(bool)
			out = append(out, agent.ToolResultBlock{
				ToolUseID: asString(r["tool_use_id"]),
				IsError:   isErr,
				Content:   sub,
			})
		case "thinking":
			out = append(out, agent.ThinkingBlock{Text: asString(r["text"]), Signature: asString(r["signature"])})
		case "metadata":
			out = append(out, agent.MetadataBlock{Key: asString(r["key"]), Value: r["value"]})
		case "image":
			src := agent.BlobSource{URL: asString(r["url"])}
			if inlineStr, ok := r["inline"].(string); ok && inlineStr != "" {
				// json.Unmarshal decoded []byte → base64 string in the map[string]any.
				// Decode it back to []byte.
				if dec, err := base64.StdEncoding.DecodeString(inlineStr); err == nil {
					src.Inline = dec
				}
				// If decode fails (malformed), leave Inline nil — tolerant parse.
			}
			out = append(out, agent.ImageBlock{
				MediaType: asString(r["media_type"]),
				Source:    src,
			})
		}
	}
	return out, nil
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// extractMentions 从 Content 里查找 MetadataBlock{Key:"mentions"}，返回 Value 原值
// （让后续 JSON marshal 决定形态）。提供给 messageToRow 把 mentions 列单独写库
// （保 v1 行为）。
func extractMentions(blocks []agent.ContentBlock) any {
	for _, b := range blocks {
		if mb, ok := b.(agent.MetadataBlock); ok && mb.Key == "mentions" {
			return mb.Value
		}
	}
	return []any{}
}
