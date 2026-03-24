package secure

import (
	"runtime"
	"sync"
)

// Bytes 封装敏感字节数据，支持手动清零和 GC 自动清零
type Bytes struct {
	data   []byte
	mu     sync.Mutex
	zeroed bool
}

// NewBytes 创建安全字节封装，会复制传入的数据
func NewBytes(data []byte) *Bytes {
	s := &Bytes{data: make([]byte, len(data))}
	copy(s.data, data)
	runtime.SetFinalizer(s, (*Bytes).Zero)
	return s
}

// NewBytesFromString 从字符串创建安全字节封装
func NewBytesFromString(s string) *Bytes {
	return NewBytes([]byte(s))
}

// Bytes 返回底层数据，已清零则返回 nil
func (s *Bytes) Bytes() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.zeroed {
		return nil
	}
	return s.data
}

// String 返回字符串形式，已清零则返回空字符串
func (s *Bytes) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.zeroed {
		return ""
	}
	return string(s.data)
}

// Zero 清零底层数据
func (s *Bytes) Zero() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.zeroed {
		for i := range s.data {
			s.data[i] = 0
		}
		s.zeroed = true
	}
}

// IsZeroed 检查是否已清零
func (s *Bytes) IsZeroed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.zeroed
}

// ZeroSlice 清零一个字节切片
func ZeroSlice(data []byte) {
	for i := range data {
		data[i] = 0
	}
}
