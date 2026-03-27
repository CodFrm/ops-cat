package status

import (
	"sync"
	"time"
)

// Level 状态级别
type Level string

const (
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

// Entry 单条状态条目
type Entry struct {
	Level   Level     `json:"level"`
	Source  string    `json:"source"`
	Message string    `json:"message"`
	Detail  string    `json:"detail"`
	Time    time.Time `json:"time"`
}

var (
	mu      sync.Mutex
	entries []Entry
)

// Add 添加一条状态条目。如果 Time 为零值则自动设为当前时间。
func Add(e Entry) {
	mu.Lock()
	defer mu.Unlock()
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	entries = append(entries, e)
}

// List 返回所有条目的副本
func List() []Entry {
	mu.Lock()
	defer mu.Unlock()
	cp := make([]Entry, len(entries))
	copy(cp, entries)
	return cp
}

// HasProblems 是否存在 warn 或 error 级别的条目
func HasProblems() bool {
	mu.Lock()
	defer mu.Unlock()
	for _, e := range entries {
		if e.Level == LevelWarn || e.Level == LevelError {
			return true
		}
	}
	return false
}

// Reset 清空所有条目（测试用）
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	entries = nil
}
