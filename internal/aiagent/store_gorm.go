package aiagent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cago-frame/agents/agent"
	agentstore "github.com/cago-frame/agents/agent/store"

	"github.com/opskat/opskat/internal/ai"
	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
	"github.com/opskat/opskat/internal/repository/conversation_repo"
)

// gormStore 把 cago agent.Conversation 的增量变更落到 conversation_messages 的
// (conversation_id, sort_order) 行上。配合 agent/store.Recorder 监听 Conv.Watch 使用。
//
// 三路投影分工（重要）：
//   - LLM：BuildRequest 按 Audience ToLLM 过滤；DisplayTextBlock 不带 ToLLM 自动剥离。
//     这里不参与。
//   - 储存（本文件）：Recorder 按 Audience ToStore 过滤后调进来，落到 DB 行的
//     blocks/mentions/token_usage/partial_reason 几列。
//   - UI：前端走 ai:event 流；历史回放走 app_ai.buildDisplayMessages 解析这里写出的
//     flat JSON（{"type":"text","text":...} / {"type":"display_text","text":...}/...）。
//
// Mentions：user 消息的 LLM body（TextBlock 内容）携带 inline `<mention>` 标签，
// 本 store 在 AppendMessage user-role 路径里调 ai.ParseMentions 反向解析，写入
// row.Mentions JSON。这是为了取代旧的 Manager.StashMentions 旁路——元数据与消息
// 正文原子绑定，避免历史 mention offset 被误打到下条 row 上。
type gormStore struct {
	repo conversation_repo.ConversationRepo
}

// NewGormStore returns an agent/store.Store backed by OpsKat's conversation_repo.
func NewGormStore() agentstore.Store {
	return &gormStore{
		repo: conversation_repo.Conversation(),
	}
}

func parseSessionID(s string) (int64, error) {
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("aiagent: bad session id %q: %w", s, err)
	}
	return id, nil
}

func (g *gormStore) AppendMessage(ctx context.Context, sessionID string, index int, sm agentstore.StoredMessage) error {
	convID, err := parseSessionID(sessionID)
	if err != nil {
		return err
	}
	msg, err := agentstore.DecodeMessage(sm)
	if err != nil {
		return fmt.Errorf("aiagent: decode stored message: %w", err)
	}
	row, err := messageToRow(convID, index, msg)
	if err != nil {
		return err
	}
	// user 消息从 TextBlock 反向解析 inline `<mention>` 标签 → row.Mentions。
	// 这条路径取代了旧的 Manager.StashMentions 旁路：mention 元数据与消息正文
	// 原子绑定，不会被前端历史 mention 聚合错位污染下一行。
	if msg.Role == agent.RoleUser {
		if mentionsJSON := extractMentionsJSON(msg.Content); mentionsJSON != "" {
			row.Mentions = mentionsJSON
		}
	}
	return g.repo.AppendAt(ctx, convID, index, row)
}

// extractMentionsJSON 从 user 消息的 TextBlock 里扫 <mention> 标签反序列化成
// row.Mentions JSON。无 mention 时返回 ""（保留 messageToRow 给出的默认 "[]"）。
func extractMentionsJSON(blocks []agent.ContentBlock) string {
	var body strings.Builder
	for _, b := range blocks {
		if tb, ok := b.(agent.TextBlock); ok {
			body.WriteString(tb.Text)
		}
	}
	mentions := ai.ParseMentions(body.String())
	if len(mentions) == 0 {
		return ""
	}
	mb, err := json.Marshal(mentions)
	if err != nil {
		return ""
	}
	return string(mb)
}

func (g *gormStore) UpdateMessage(ctx context.Context, sessionID string, index int, sm agentstore.StoredMessage) error {
	convID, err := parseSessionID(sessionID)
	if err != nil {
		return err
	}
	msg, err := agentstore.DecodeMessage(sm)
	if err != nil {
		return fmt.Errorf("aiagent: decode stored message: %w", err)
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

func (g *gormStore) LoadConversation(ctx context.Context, sessionID string) ([]agentstore.StoredMessage, agentstore.BranchInfo, error) {
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
	out := make([]agentstore.StoredMessage, 0, len(rows))
	for _, r := range rows {
		m, err := rowToMessage(r)
		if err != nil {
			return nil, agentstore.BranchInfo{}, err
		}
		// crash-recovery invariant：尾部 PartialStreaming 残留改写为 errored，
		// 让接下来的 turn 把它当作历史而不是当前正在输出的消息。
		// 只对最后一条做改写，前面的保留原样（历史的 streaming 可能已经被后续
		// turn 覆盖过，无需再动）。
		sm, err := agentstore.EncodeMessage(m)
		if err != nil {
			return nil, agentstore.BranchInfo{}, fmt.Errorf("aiagent: encode message: %w", err)
		}
		out = append(out, sm)
	}
	if n := len(out); n > 0 && agent.PartialState(out[n-1].PartialReason) == agent.PartialStreaming {
		out[n-1].PartialReason = string(agent.PartialErrored)
		// 进程崩溃 / 异常退出导致没机会写错误详情 —— 给一段固定文案，让 UI
		// 仍能渲染出"已中断"的提示，不至于看起来像一条正常截断的消息。
		if out[n-1].PartialDetail == "" {
			out[n-1].PartialDetail = "stream interrupted (recovered)"
		}
	}
	return out, agentstore.BranchInfo{}, nil
}

// messageToRow 把 cago Message 序列化为 DB 行。
//
// 序列化形态有意保 flat（{"type":...,"text":...}），不用 cago StoredBlock 的
// {type,data} 嵌套形态——
//   - migration 202605100001 backfill 写的是 flat
//   - 前端 aiStore / app_ai.buildDisplayMessages 消费 flat
//
// 切换嵌套形态需要再写一次迁移 + 改前端解析，目前不值得。
//
// 字段映射：
//   - blocks: 所有 ContentBlock 的 flat JSON
//   - mentions: 由 AppendMessage 的旁路 (PopPendingMentions) 覆盖；这里默认空数组
//   - token_usage: msg.Usage JSON（仅 assistant 非 nil 时填）
//   - partial_reason: agent.PartialState 字符串值
func messageToRow(convID int64, index int, m agent.Message) (*conversation_entity.Message, error) {
	blocksJSON, err := json.Marshal(serializeBlocks(m.Content))
	if err != nil {
		return nil, fmt.Errorf("marshal blocks: %w", err)
	}
	// 默认空 mentions（旁路写入会覆盖；非 user 行就此为空，与 v1 行为一致）。
	mentionsJSON := []byte("[]")
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
		PartialReason:  string(m.PartialReason),
		PartialDetail:  m.PartialDetail,
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
		PartialReason: agent.PartialState(r.PartialReason),
		PartialDetail: r.PartialDetail,
		Usage:         usage,
	}, nil
}

// serializeBlocks 把 ContentBlock slice 序列化成 [{"type":..., ...}, ...]。
// tag 用 Block.Type()（cago Audience 体系下的 on-wire 名）。
//
// flat 形态：每个块的字段直接平铺到 JSON 对象里，不嵌套 data 子对象。
// 与 migration 202605100001 backfill + 前端 buildDisplayMessages 对齐。
//
// 已知不会被 ToStore 投影写入的块（如 NoticeBlock 仅 ToUI|ToStore，目前
// 我们不发送）走 default 分支按 {type} 写出来 —— 不丢类型信息，但也不展开
// 字段，反序列化时会被忽略。
func serializeBlocks(blocks []agent.ContentBlock) []map[string]any {
	out := make([]map[string]any, 0, len(blocks))
	for _, b := range blocks {
		entry := map[string]any{"type": b.Type()}
		switch v := b.(type) {
		case agent.TextBlock:
			entry["text"] = v.Text
		case agent.DisplayTextBlock:
			// UI-only：BuildRequest 自动剥离（Audience 不含 ToLLM）。
			// 储存这里需要 round-trip 回来，前端 UserMessage 才能渲染 raw 文本。
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
		case "display_text":
			out = append(out, agent.DisplayTextBlock{Text: asString(r["text"])})
		case "metadata":
			// 兼容老 DB 行（cago 新 API 前写入的）：metadata{key:"display"} 等价于
			// DisplayTextBlock。其他 key（mentions 等）历史上靠 extractMentions
			// 再写到 mentions 列，typed message 这里就直接丢弃 —— 反正旁路覆盖了。
			if asString(r["key"]) == "display" {
				if s, ok := r["value"].(string); ok {
					out = append(out, agent.DisplayTextBlock{Text: s})
				}
			}
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
