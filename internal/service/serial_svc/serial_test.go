package serial_svc

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.bug.st/serial"
)

type fakePort struct {
	mu         sync.Mutex
	writes     [][]byte
	readFn     func([]byte) (int, error)
	closeCount int
}

func (p *fakePort) SetMode(_ *serial.Mode) error { return nil }

func (p *fakePort) Read(buf []byte) (int, error) {
	if p.readFn != nil {
		return p.readFn(buf)
	}
	return 0, nil
}

func (p *fakePort) Write(buf []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	data := make([]byte, len(buf))
	copy(data, buf)
	p.writes = append(p.writes, data)
	return len(buf), nil
}

func (p *fakePort) Drain() error { return nil }

func (p *fakePort) ResetInputBuffer() error { return nil }

func (p *fakePort) ResetOutputBuffer() error { return nil }

func (p *fakePort) SetDTR(_ bool) error { return nil }

func (p *fakePort) SetRTS(_ bool) error { return nil }

func (p *fakePort) GetModemStatusBits() (*serial.ModemStatusBits, error) {
	return &serial.ModemStatusBits{}, nil
}

func (p *fakePort) SetReadTimeout(_ time.Duration) error { return nil }

func (p *fakePort) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeCount++
	return nil
}

func (p *fakePort) Break(_ time.Duration) error { return nil }

func (p *fakePort) writeStrings() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, 0, len(p.writes))
	for _, w := range p.writes {
		out = append(out, string(w))
	}
	return out
}

func TestSessionExecCommandSerializesWrites(t *testing.T) {
	port := &fakePort{}
	sess := &Session{ID: "serial-1", port: port}

	execDone := make(chan struct{})
	execErr := make(chan error, 1)
	go func() {
		defer close(execDone)
		_, err := sess.ExecCommand("display version", 40*time.Millisecond, 80*time.Millisecond)
		execErr <- err
	}()

	require.Eventually(t, func() bool {
		sess.mu.Lock()
		defer sess.mu.Unlock()
		return sess.cmdOutputCh != nil
	}, time.Second, 5*time.Millisecond)

	writeDone := make(chan error, 1)
	go func() {
		writeDone <- sess.Write([]byte("user input\r\n"))
	}()

	select {
	case err := <-writeDone:
		t.Fatalf("concurrent write should wait for ExecCommand, got %v", err)
	case <-time.After(15 * time.Millisecond):
	}

	select {
	case <-execDone:
	case <-time.After(time.Second):
		t.Fatal("ExecCommand did not finish")
	}
	require.NoError(t, <-execErr)

	select {
	case err := <-writeDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("blocked write did not resume after ExecCommand")
	}

	assert.Equal(t, []string{"display version\r\n", "user input\r\n"}, port.writeStrings())
}

func TestManagerReadOutputClosesSessionOnUnexpectedError(t *testing.T) {
	port := &fakePort{
		readFn: func([]byte) (int, error) {
			return 0, errors.New("boom")
		},
	}
	closed := make(chan string, 1)
	sess := &Session{
		ID:      "serial-2",
		AssetID: 42,
		port:    port,
		onClosed: func(sessionID string) {
			closed <- sessionID
		},
	}
	mgr := NewManager()
	mgr.sessions.Store(sess.ID, sess)

	done := make(chan struct{})
	go func() {
		mgr.readOutput(sess)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("readOutput did not exit after unexpected error")
	}

	select {
	case sessionID := <-closed:
		assert.Equal(t, sess.ID, sessionID)
	case <-time.After(time.Second):
		t.Fatal("session close callback was not triggered")
	}

	assert.Equal(t, 1, port.closeCount)
	_, ok := mgr.GetSession(sess.ID)
	assert.False(t, ok)
}
