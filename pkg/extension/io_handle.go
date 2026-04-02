// pkg/extension/io_handle.go
package extension

import (
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
)

// IOMeta contains metadata about an IO handle.
type IOMeta struct {
	Size        int64             `json:"size,omitempty"`
	ContentType string            `json:"contentType,omitempty"`
	Status      int               `json:"status,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
}

type ioEntry struct {
	reader io.Reader
	writer io.Writer
	closer io.Closer
	meta   IOMeta
	http   *httpHandle // non-nil for HTTP handles
}

// Adapter types to bridge httpHandle to io.Reader/Writer/Closer.
type httpReadAdapter struct{ h *httpHandle }

func (a *httpReadAdapter) Read(p []byte) (int, error) { return a.h.Read(p) }

type httpWriteAdapter struct{ h *httpHandle }

func (a *httpWriteAdapter) Write(p []byte) (int, error) { return a.h.Write(p) }

type httpCloseAdapter struct{ h *httpHandle }

func (a *httpCloseAdapter) Close() error { return a.h.Close() }

// IOHandleManager manages IO handles for a single WASM invocation.
type IOHandleManager struct {
	mu      sync.Mutex
	handles map[uint32]*ioEntry
	nextID  atomic.Uint32
}

func NewIOHandleManager() *IOHandleManager {
	m := &IOHandleManager{
		handles: make(map[uint32]*ioEntry),
	}
	m.nextID.Store(1)
	return m
}

func (m *IOHandleManager) OpenFile(path string, mode string) (uint32, IOMeta, error) {
	var entry ioEntry
	switch mode {
	case "read":
		f, err := os.Open(path) //nolint:gosec // path provided by extension runtime within sandbox
		if err != nil {
			return 0, IOMeta{}, fmt.Errorf("open file for read: %w", err)
		}
		info, err := f.Stat()
		if err != nil {
			if closeErr := f.Close(); closeErr != nil {
				logger.Default().Warn("close file after stat error", zap.Error(closeErr))
			}
			return 0, IOMeta{}, fmt.Errorf("stat file: %w", err)
		}
		entry = ioEntry{reader: f, closer: f, meta: IOMeta{Size: info.Size()}}
	case "write":
		f, err := os.Create(path) //nolint:gosec // path provided by extension runtime within sandbox
		if err != nil {
			return 0, IOMeta{}, fmt.Errorf("open file for write: %w", err)
		}
		entry = ioEntry{writer: f, closer: f}
	default:
		return 0, IOMeta{}, fmt.Errorf("unknown file mode: %q", mode)
	}

	id := m.nextID.Add(1) - 1
	m.mu.Lock()
	m.handles[id] = &entry
	m.mu.Unlock()
	return id, entry.meta, nil
}

// Register adds an externally-created handle entry and returns its ID.
func (m *IOHandleManager) Register(r io.Reader, w io.Writer, c io.Closer, meta IOMeta) uint32 {
	id := m.nextID.Add(1) - 1
	m.mu.Lock()
	m.handles[id] = &ioEntry{reader: r, writer: w, closer: c, meta: meta}
	m.mu.Unlock()
	return id
}

func (m *IOHandleManager) Read(id uint32, buf []byte) (int, error) {
	e, err := m.get(id)
	if err != nil {
		return 0, err
	}
	if e.reader == nil {
		return 0, fmt.Errorf("handle %d is not readable", id)
	}
	return e.reader.Read(buf)
}

func (m *IOHandleManager) Write(id uint32, data []byte) (int, error) {
	e, err := m.get(id)
	if err != nil {
		return 0, err
	}
	if e.writer == nil {
		return 0, fmt.Errorf("handle %d is not writable", id)
	}
	return e.writer.Write(data)
}

func (m *IOHandleManager) GetMeta(id uint32) (IOMeta, error) {
	e, err := m.get(id)
	if err != nil {
		return IOMeta{}, err
	}
	return e.meta, nil
}

func (m *IOHandleManager) Close(id uint32) error {
	m.mu.Lock()
	e, ok := m.handles[id]
	if ok {
		delete(m.handles, id)
	}
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("handle %d not found", id)
	}
	if e.closer != nil {
		return e.closer.Close()
	}
	return nil
}

func (m *IOHandleManager) CloseAll() {
	m.mu.Lock()
	handles := m.handles
	m.handles = make(map[uint32]*ioEntry)
	m.mu.Unlock()
	for _, e := range handles {
		if e.closer != nil {
			if err := e.closer.Close(); err != nil {
				logger.Default().Warn("close IO handle", zap.Error(err))
			}
		}
	}
}

// OpenHTTP creates an HTTP handle, wraps it with adapters, and stores it.
func (m *IOHandleManager) OpenHTTP(params IOOpenParams, dial DialFunc) (uint32, IOMeta, error) {
	h, err := newHTTPHandle(params, dial)
	if err != nil {
		return 0, IOMeta{}, err
	}

	entry := &ioEntry{
		reader: &httpReadAdapter{h: h},
		writer: &httpWriteAdapter{h: h},
		closer: &httpCloseAdapter{h: h},
		http:   h,
	}

	id := m.nextID.Add(1) - 1
	m.mu.Lock()
	m.handles[id] = entry
	m.mu.Unlock()
	return id, entry.meta, nil
}

// Flush flushes the HTTP handle (sends the request and waits for response).
func (m *IOHandleManager) Flush(id uint32) (*IOMeta, error) {
	e, err := m.get(id)
	if err != nil {
		return nil, err
	}
	if e.http == nil {
		return nil, fmt.Errorf("handle %d is not an HTTP handle", id)
	}
	return e.http.Flush()
}

func (m *IOHandleManager) get(id uint32) (*ioEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.handles[id]
	if !ok {
		return nil, fmt.Errorf("handle %d not found", id)
	}
	return e, nil
}
