package redis_svc

import (
	"fmt"
	"strings"
	"sync"
)

const redactedHistoryValue = "<redacted>"

type CommandHistoryEntry struct {
	AssetID    int64  `json:"assetId"`
	DB         int    `json:"db"`
	Command    string `json:"command"`
	CostMillis int64  `json:"costMillis"`
	Error      string `json:"error,omitempty"`
	Timestamp  int64  `json:"timestamp"`
}

type CommandHistory struct {
	mu      sync.RWMutex
	limit   int
	entries []CommandHistoryEntry
}

func NewCommandHistory(limit int) *CommandHistory {
	if limit <= 0 {
		limit = 200
	}
	return &CommandHistory{limit: limit}
}

func (h *CommandHistory) Add(entry CommandHistoryEntry) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.entries = append([]CommandHistoryEntry{entry}, h.entries...)
	if len(h.entries) > h.limit {
		h.entries = h.entries[:h.limit]
	}
}

func (h *CommandHistory) List(assetID int64, limit int) []CommandHistoryEntry {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if limit <= 0 || limit > h.limit {
		limit = h.limit
	}
	out := make([]CommandHistoryEntry, 0, min(limit, len(h.entries)))
	for _, entry := range h.entries {
		if assetID > 0 && entry.AssetID != assetID {
			continue
		}
		out = append(out, entry)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func formatCommandForHistory(args []any) string {
	if len(args) == 0 {
		return ""
	}
	parts := redactCommandArgs(args)
	return strings.Join(parts, " ")
}

func redactCommandArgs(args []any) []string {
	cmd := strings.ToUpper(fmt.Sprint(args[0]))
	switch cmd {
	case "SET":
		return redactSetCommand(args)
	case "SETEX", "PSETEX":
		return redactByIndexes(args, map[int]bool{3: true})
	case "GETSET", "SETRANGE", "HSETNX", "LSET", "LREM", "LINSERT":
		return redactLastArg(args)
	case "HSET", "HMSET":
		return redactFieldValuePairs(args, 2)
	case "LPUSH", "RPUSH", "LPUSHX", "RPUSHX", "SADD":
		return redactFromIndex(args, 2)
	case "ZADD":
		if len(args) <= 2 {
			return redactFromIndex(args, 1)
		}
		return []string{quoteHistoryArg(args[0]), quoteHistoryArg(args[1]), redactedHistoryValue}
	case "XADD":
		return redactFieldValuePairs(args, 3)
	case "MSET", "MSETNX":
		return redactFieldValuePairs(args, 1)
	default:
		parts := make([]string, 0, len(args))
		for _, arg := range args {
			parts = append(parts, quoteHistoryArg(arg))
		}
		return parts
	}
}

func redactSetCommand(args []any) []string {
	parts := make([]string, 0, len(args))
	for i, arg := range args {
		if i == 2 {
			parts = append(parts, redactedHistoryValue)
			continue
		}
		parts = append(parts, quoteHistoryArg(arg))
	}
	return parts
}

func redactLastArg(args []any) []string {
	redacted := map[int]bool{}
	if len(args) > 1 {
		redacted[len(args)-1] = true
	}
	return redactByIndexes(args, redacted)
}

func redactFromIndex(args []any, start int) []string {
	redacted := map[int]bool{}
	for i := start; i < len(args); i++ {
		redacted[i] = true
	}
	return redactByIndexes(args, redacted)
}

func redactFieldValuePairs(args []any, firstFieldIndex int) []string {
	redacted := map[int]bool{}
	for i := firstFieldIndex + 1; i < len(args); i += 2 {
		redacted[i] = true
	}
	return redactByIndexes(args, redacted)
}

func redactByIndexes(args []any, redacted map[int]bool) []string {
	parts := make([]string, 0, len(args))
	for i, arg := range args {
		if redacted[i] {
			parts = append(parts, redactedHistoryValue)
			continue
		}
		parts = append(parts, quoteHistoryArg(arg))
	}
	return parts
}

func quoteHistoryArg(arg any) string {
	s := fmt.Sprint(arg)
	if strings.ContainsAny(s, " \t\r\n\"") {
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return s
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
