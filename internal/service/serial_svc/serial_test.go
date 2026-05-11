package serial_svc

import (
	"errors"
	"io"
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
	writeFn    func([]byte) (int, error)
	closeFn    func() error
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
	if p.writeFn != nil {
		return p.writeFn(buf)
	}
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
	if p.closeFn != nil {
		return p.closeFn()
	}
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

func (p *fakePort) getCloseCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closeCount
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
		return sess.cmdCapture != nil
	}, time.Second, 5*time.Millisecond)
	sess.mu.Lock()
	capture := sess.cmdCapture
	sess.mu.Unlock()
	capture.Append([]byte("OK\r\n"))

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

func TestSessionExecCommandCollectsAllCapturedOutput(t *testing.T) {
	port := &fakePort{}
	sess := &Session{ID: "serial-3", port: port}

	resultCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		out, err := sess.ExecCommand("show data", 30*time.Millisecond, 200*time.Millisecond)
		resultCh <- out
		errCh <- err
	}()

	var capture *commandCapture
	require.Eventually(t, func() bool {
		sess.mu.Lock()
		defer sess.mu.Unlock()
		capture = sess.cmdCapture
		return capture != nil
	}, time.Second, 5*time.Millisecond)

	for i := 0; i < 512; i++ {
		capture.Append([]byte("x"))
	}

	select {
	case out := <-resultCh:
		require.NoError(t, <-errCh)
		assert.Len(t, out, 512)
	case <-time.After(time.Second):
		t.Fatal("ExecCommand did not finish")
	}
}

func TestSessionExecCommandReturnsErrorOnMaxTimeout(t *testing.T) {
	port := &fakePort{}
	sess := &Session{ID: "serial-timeout", port: port}

	resultCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		out, err := sess.ExecCommand("show data", 200*time.Millisecond, 40*time.Millisecond)
		resultCh <- out
		errCh <- err
	}()

	require.Eventually(t, func() bool {
		sess.mu.Lock()
		defer sess.mu.Unlock()
		return sess.cmdCapture != nil
	}, time.Second, 5*time.Millisecond)
	sess.mu.Lock()
	capture := sess.cmdCapture
	sess.mu.Unlock()
	capture.Append([]byte("partial output"))

	select {
	case out := <-resultCh:
		assert.Equal(t, "partial output", out)
		err := <-errCh
		require.Error(t, err)
		assert.ErrorIs(t, err, errCommandTimedOut)
	case <-time.After(time.Second):
		t.Fatal("ExecCommand did not time out")
	}
}

func TestSessionExecCommandReturnsErrorWhenSessionClosed(t *testing.T) {
	port := &fakePort{}
	sess := &Session{ID: "serial-closed", port: port}

	resultCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		out, err := sess.ExecCommand("show data", 200*time.Millisecond, time.Second)
		resultCh <- out
		errCh <- err
	}()

	require.Eventually(t, func() bool {
		sess.mu.Lock()
		defer sess.mu.Unlock()
		return sess.cmdCapture != nil
	}, time.Second, 5*time.Millisecond)
	sess.mu.Lock()
	capture := sess.cmdCapture
	sess.mu.Unlock()
	capture.Append([]byte("partial output"))
	sess.Close()

	select {
	case out := <-resultCh:
		assert.Equal(t, "partial output", out)
		err := <-errCh
		require.Error(t, err)
		assert.ErrorIs(t, err, errSessionClosed)
	case <-time.After(time.Second):
		t.Fatal("ExecCommand did not stop after session close")
	}
}

func TestSessionWriteDoesNotHoldStateLockDuringPortWrite(t *testing.T) {
	writeEntered := make(chan struct{}, 1)
	releaseWrite := make(chan struct{})
	port := &fakePort{
		writeFn: func(buf []byte) (int, error) {
			select {
			case writeEntered <- struct{}{}:
			default:
			}
			<-releaseWrite
			return len(buf), nil
		},
	}
	sess := &Session{ID: "serial-write-lock", port: port}

	errCh := make(chan error, 1)
	go func() {
		errCh <- sess.Write([]byte("abc"))
	}()

	select {
	case <-writeEntered:
	case <-time.After(time.Second):
		t.Fatal("port.Write was not reached")
	}

	isClosedDone := make(chan struct{})
	go func() {
		sess.IsClosed()
		close(isClosedDone)
	}()

	select {
	case <-isClosedDone:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("IsClosed blocked while Write was waiting on port.Write")
	}

	close(releaseWrite)
	require.NoError(t, <-errCh)
}

func TestSessionCloseDoesNotHoldStateLockDuringPortClose(t *testing.T) {
	closeEntered := make(chan struct{}, 1)
	releaseClose := make(chan struct{})
	port := &fakePort{
		closeFn: func() error {
			select {
			case closeEntered <- struct{}{}:
			default:
			}
			<-releaseClose
			return nil
		},
	}
	sess := &Session{ID: "serial-close-lock", port: port}

	closeDone := make(chan struct{})
	go func() {
		sess.Close()
		close(closeDone)
	}()

	select {
	case <-closeEntered:
	case <-time.After(time.Second):
		t.Fatal("port.Close was not reached")
	}

	isClosedDone := make(chan struct{})
	go func() {
		assert.True(t, sess.IsClosed())
		close(isClosedDone)
	}()

	select {
	case <-isClosedDone:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("IsClosed blocked while Close was waiting on port.Close")
	}

	close(releaseClose)
	select {
	case <-closeDone:
	case <-time.After(time.Second):
		t.Fatal("Close did not finish")
	}
}

func TestManagerSetCallbacksStartsReaderOnlyOnce(t *testing.T) {
	var mu sync.Mutex
	activeReads := 0
	maxActiveReads := 0
	releaseRead := make(chan struct{})
	port := &fakePort{
		readFn: func([]byte) (int, error) {
			mu.Lock()
			activeReads++
			if activeReads > maxActiveReads {
				maxActiveReads = activeReads
			}
			mu.Unlock()

			<-releaseRead

			mu.Lock()
			activeReads--
			mu.Unlock()
			return 0, io.EOF
		},
	}
	sess := &Session{ID: "serial-reader-once", port: port}
	mgr := NewManager()
	mgr.sessions.Store(sess.ID, sess)

	mgr.SetCallbacks(sess.ID, nil, nil)
	mgr.SetCallbacks(sess.ID, nil, nil)

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return maxActiveReads > 0
	}, time.Second, 5*time.Millisecond)

	time.Sleep(20 * time.Millisecond)
	mu.Lock()
	gotMaxActiveReads := maxActiveReads
	mu.Unlock()

	close(releaseRead)
	require.Eventually(t, func() bool {
		_, ok := mgr.GetSession(sess.ID)
		return !ok
	}, time.Second, 5*time.Millisecond)
	assert.Equal(t, 1, gotMaxActiveReads)
}

func TestManagerWatchCallbackSetupClosesUninitializedSession(t *testing.T) {
	port := &fakePort{}
	sess := &Session{ID: "serial-orphan", port: port}
	mgr := NewManager()
	mgr.sessions.Store(sess.ID, sess)

	mgr.watchCallbackSetup(sess, 20*time.Millisecond)

	require.Eventually(t, func() bool {
		_, ok := mgr.GetSession(sess.ID)
		return !ok
	}, time.Second, 5*time.Millisecond)
	assert.True(t, sess.IsClosed())
	assert.Equal(t, 1, port.getCloseCount())
}

func TestManagerWatchCallbackSetupKeepsSessionAfterCallbacks(t *testing.T) {
	releaseRead := make(chan struct{})
	port := &fakePort{
		readFn: func([]byte) (int, error) {
			<-releaseRead
			return 0, io.EOF
		},
	}
	sess := &Session{ID: "serial-callback-ready", port: port}
	mgr := NewManager()
	mgr.sessions.Store(sess.ID, sess)

	mgr.watchCallbackSetup(sess, 20*time.Millisecond)
	mgr.SetCallbacks(sess.ID, nil, nil)

	time.Sleep(50 * time.Millisecond)
	_, ok := mgr.GetSession(sess.ID)
	assert.True(t, ok)

	close(releaseRead)
	require.Eventually(t, func() bool {
		_, ok := mgr.GetSession(sess.ID)
		return !ok
	}, time.Second, 5*time.Millisecond)
}

func TestBuildSerialModeRejectsInvalidStopBits(t *testing.T) {
	_, err := buildSerialMode(ConnectConfig{StopBits: "3"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported stop bits")
}

func TestBuildSerialModeRejectsInvalidParity(t *testing.T) {
	_, err := buildSerialMode(ConnectConfig{Parity: "invalid"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported parity")
}

func TestBuildSerialModeAppliesDefaults(t *testing.T) {
	mode, err := buildSerialMode(ConnectConfig{})
	require.NoError(t, err)
	assert.Equal(t, 115200, mode.BaudRate)
	assert.Equal(t, 8, mode.DataBits)
	assert.Equal(t, serial.OneStopBit, mode.StopBits)
	assert.Equal(t, serial.NoParity, mode.Parity)
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
