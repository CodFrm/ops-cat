// pkg/extension/io_http.go
package extension

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
)

type httpPhase int

const (
	httpPhaseWriting httpPhase = iota // can write request body
	httpPhaseFlushed                  // request sent, can read response
	httpPhaseClosed
)

// DialFunc is a custom dialer for HTTP transports (e.g. SSH tunnel).
type DialFunc func(network, addr string) (net.Conn, error)

type httpHandle struct {
	mu      sync.Mutex
	client  *http.Client
	method  string
	url     string
	headers map[string]string
	ctx     context.Context
	cancel  context.CancelFunc
	bodyBuf *bytes.Buffer // buffered request body (for POST/PUT/PATCH)
	resp    *http.Response
	phase   httpPhase
	hasBody bool // true for POST/PUT/PATCH
}

// newHTTPHandle creates an HTTP handle ready for writing (POST/PUT/PATCH)
// or immediate flushing (GET/HEAD/DELETE/OPTIONS).
func newHTTPHandle(params IOOpenParams, dial DialFunc) (*httpHandle, error) {
	method := strings.ToUpper(params.Method)
	if method == "" {
		method = "GET"
	}

	if params.URL == "" {
		return nil, fmt.Errorf("URL is required for HTTP handle")
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Build transport; clone default so we don't mutate the global one.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if dial != nil {
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dial(network, addr)
		}
	}

	hasBody := method == "POST" || method == "PUT" || method == "PATCH"

	return &httpHandle{
		client:  &http.Client{Transport: transport},
		method:  method,
		url:     params.URL,
		headers: params.Headers,
		ctx:     ctx,
		cancel:  cancel,
		bodyBuf: &bytes.Buffer{},
		phase:   httpPhaseWriting,
		hasBody: hasBody,
	}, nil
}

// Write writes data to the request body buffer. Only valid before Flush.
func (h *httpHandle) Write(data []byte) (int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.phase != httpPhaseWriting {
		return 0, fmt.Errorf("not in writing phase")
	}
	return h.bodyBuf.Write(data)
}

// Flush builds the HTTP request, executes it, and returns response metadata.
// Blocks until the response headers arrive.
func (h *httpHandle) Flush() (*IOMeta, error) {
	h.mu.Lock()
	if h.phase != httpPhaseWriting {
		h.mu.Unlock()
		return nil, fmt.Errorf("already flushed or closed")
	}
	h.phase = httpPhaseFlushed

	// Build request body.
	var body *bytes.Reader
	if h.hasBody {
		body = bytes.NewReader(h.bodyBuf.Bytes())
	}

	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequestWithContext(h.ctx, h.method, h.url, body)
	} else {
		req, err = http.NewRequestWithContext(h.ctx, h.method, h.url, nil)
	}
	if err != nil {
		h.mu.Unlock()
		return nil, fmt.Errorf("create HTTP request: %w", err)
	}

	for k, v := range h.headers {
		req.Header.Set(k, v)
	}

	client := h.client
	h.mu.Unlock()

	resp, err := client.Do(req) //nolint:bodyclose // body is read by Read() and closed by Close()
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}

	h.mu.Lock()
	h.resp = resp
	h.mu.Unlock()

	meta := &IOMeta{
		Status:      resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Size:        resp.ContentLength,
		Headers:     make(map[string]string),
	}
	for k := range resp.Header {
		meta.Headers[k] = resp.Header.Get(k)
	}

	return meta, nil
}

// Read reads from the response body. Only valid after Flush.
func (h *httpHandle) Read(buf []byte) (int, error) {
	h.mu.Lock()
	if h.phase != httpPhaseFlushed {
		h.mu.Unlock()
		return 0, fmt.Errorf("response not flushed yet; call Flush first")
	}
	resp := h.resp
	h.mu.Unlock()

	if resp == nil {
		return 0, fmt.Errorf("no response available")
	}
	return resp.Body.Read(buf)
}

// Close cancels the context and closes the response body.
func (h *httpHandle) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.phase == httpPhaseClosed {
		return nil
	}
	h.phase = httpPhaseClosed
	h.cancel()
	if h.resp != nil {
		if err := h.resp.Body.Close(); err != nil {
			logger.Default().Warn("close HTTP response body", zap.Error(err))
		}
	}
	return nil
}
