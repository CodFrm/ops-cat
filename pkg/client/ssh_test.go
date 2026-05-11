package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// testServer is an in-process SSH server used to exercise the client. It
// supports password and publickey auth and a single "exec" subsystem that
// echoes a scripted reply per request.
type testServer struct {
	t          *testing.T
	listener   net.Listener
	hostKey    ssh.Signer
	config     *ssh.ServerConfig
	clientKey  ssh.Signer // accepted publickey
	username   string
	password   string
	wg         sync.WaitGroup
	stop       chan struct{}
	execHandle func(cmd string) (stdout, stderr string, exit int)
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()

	hostKey := generateSigner(t)
	clientKey := generateSigner(t)

	srv := &testServer{
		t:         t,
		hostKey:   hostKey,
		clientKey: clientKey,
		username:  "alice",
		password:  "s3cret",
		stop:      make(chan struct{}),
		execHandle: func(cmd string) (string, string, int) {
			return "ok " + cmd + "\n", "", 0
		},
	}

	srv.config = &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
			if c.User() == srv.username && string(pw) == srv.password {
				return nil, nil
			}
			return nil, fmt.Errorf("auth failed")
		},
		PublicKeyCallback: func(c ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if c.User() == srv.username && bytes.Equal(key.Marshal(), srv.clientKey.PublicKey().Marshal()) {
				return nil, nil
			}
			return nil, fmt.Errorf("auth failed")
		},
	}
	srv.config.AddHostKey(hostKey)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv.listener = ln

	srv.wg.Add(1)
	go srv.serve()

	t.Cleanup(srv.Close)
	return srv
}

func (s *testServer) addr() (host string, port int) {
	a := s.listener.Addr().(*net.TCPAddr)
	return a.IP.String(), a.Port
}

func (s *testServer) Close() {
	select {
	case <-s.stop:
		return
	default:
		close(s.stop)
	}
	_ = s.listener.Close()
	s.wg.Wait()
}

func (s *testServer) serve() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

func (s *testServer) handleConn(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	_, chans, reqs, err := ssh.NewServerConn(conn, s.config)
	if err != nil {
		return
	}
	go ssh.DiscardRequests(reqs)

	for newCh := range chans {
		if newCh.ChannelType() != "session" {
			_ = newCh.Reject(ssh.UnknownChannelType, "unsupported")
			continue
		}
		ch, requests, err := newCh.Accept()
		if err != nil {
			return
		}
		go s.handleSession(ch, requests)
	}
}

func (s *testServer) handleSession(ch ssh.Channel, requests <-chan *ssh.Request) {
	defer func() { _ = ch.Close() }()
	for req := range requests {
		switch req.Type {
		case "exec":
			cmd := string(req.Payload[4:]) // ssh exec payload: 4-byte length prefix
			if req.WantReply {
				_ = req.Reply(true, nil)
			}
			out, errOut, exit := s.execHandle(cmd)
			_, _ = io.Copy(ch, strings.NewReader(out))
			_, _ = io.Copy(ch.Stderr(), strings.NewReader(errOut))
			status := struct{ Status uint32 }{Status: uint32(exit)}
			_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(&status))
			return
		default:
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
		}
	}
}

func generateSigner(t *testing.T) ssh.Signer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(key)
	require.NoError(t, err)
	return signer
}

func generateRSAKey(t *testing.T) (*rsa.PrivateKey, ssh.Signer, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(key)
	require.NoError(t, err)
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	return key, signer, pemBytes
}

func acceptingPrompt() HostKeyPrompt {
	return func(string, int, string) HostKeyDecision { return HostKeyAccept }
}

func rejectingPrompt() HostKeyPrompt {
	return func(string, int, string) HostKeyDecision { return HostKeyReject }
}

// --- tests ---

func TestDial_ValidationErrors(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(c *SSHConfig)
		hosts  KnownHosts
		prompt HostKeyPrompt
		want   string
	}{
		{"nil hostKeys", func(*SSHConfig) {}, nil, acceptingPrompt(), "KnownHosts is required"},
		{"nil prompt", func(*SSHConfig) {}, NewInMemoryKnownHosts(), nil, "HostKeyPrompt is required"},
		{"missing host", func(c *SSHConfig) { c.Host = "" }, NewInMemoryKnownHosts(), acceptingPrompt(), "Host, Port and User are required"},
		{"both creds", func(c *SSHConfig) { c.PrivateKey = []byte("x") }, NewInMemoryKnownHosts(), acceptingPrompt(), "Password OR PrivateKey, not both"},
		{"no creds", func(c *SSHConfig) { c.Password = "" }, NewInMemoryKnownHosts(), acceptingPrompt(), "Password or PrivateKey required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := SSHConfig{Host: "h", Port: 22, User: "u", Password: "p"}
			tc.mutate(&cfg)
			_, err := Dial(context.Background(), cfg, tc.hosts, tc.prompt)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestDial_PasswordAuth_TOFU(t *testing.T) {
	srv := newTestServer(t)
	host, port := srv.addr()

	hosts := NewInMemoryKnownHosts()
	promptCalls := 0
	prompt := func(h string, p int, fp string) HostKeyDecision {
		promptCalls++
		assert.Equal(t, host, h)
		assert.Equal(t, port, p)
		assert.True(t, strings.HasPrefix(fp, "SHA256:"), "fingerprint must be SHA256: %s", fp)
		return HostKeyAccept
	}

	sess, err := Dial(context.Background(), SSHConfig{
		Host: host, Port: port, User: "alice", Password: "s3cret",
	}, hosts, prompt)
	require.NoError(t, err)
	defer func() { _ = sess.Close() }()
	assert.Equal(t, 1, promptCalls, "prompt should be called exactly once on first connect")

	stored, err := hosts.Lookup(host, port)
	require.NoError(t, err)
	assert.Equal(t, srv.hostKey.PublicKey().Marshal(), stored, "TOFU must save offered host key")
}

func TestDial_KnownHost_NoPromptCall(t *testing.T) {
	srv := newTestServer(t)
	host, port := srv.addr()

	hosts := NewInMemoryKnownHosts()
	require.NoError(t, hosts.Save(host, port, srv.hostKey.PublicKey().Marshal()))

	prompt := func(string, int, string) HostKeyDecision {
		t.Fatalf("prompt must not be called when host key is already known")
		return HostKeyReject
	}

	sess, err := Dial(context.Background(), SSHConfig{
		Host: host, Port: port, User: "alice", Password: "s3cret",
	}, hosts, prompt)
	require.NoError(t, err)
	defer func() { _ = sess.Close() }()
}

func TestDial_HostKeyMismatch(t *testing.T) {
	srv := newTestServer(t)
	host, port := srv.addr()

	hosts := NewInMemoryKnownHosts()
	bogusSigner := generateSigner(t)
	require.NoError(t, hosts.Save(host, port, bogusSigner.PublicKey().Marshal()))

	_, err := Dial(context.Background(), SSHConfig{
		Host: host, Port: port, User: "alice", Password: "s3cret",
	}, hosts, rejectingPrompt())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrHostKeyMismatch), "expected ErrHostKeyMismatch, got %v", err)
}

func TestDial_RejectedTOFU(t *testing.T) {
	srv := newTestServer(t)
	host, port := srv.addr()

	_, err := Dial(context.Background(), SSHConfig{
		Host: host, Port: port, User: "alice", Password: "s3cret",
	}, NewInMemoryKnownHosts(), rejectingPrompt())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "host key rejected")
}

func TestDial_WrongPassword(t *testing.T) {
	srv := newTestServer(t)
	host, port := srv.addr()

	_, err := Dial(context.Background(), SSHConfig{
		Host: host, Port: port, User: "alice", Password: "wrong",
	}, NewInMemoryKnownHosts(), acceptingPrompt())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ssh handshake")
}

func TestDial_PrivateKeyAuth(t *testing.T) {
	// The test server is initialized with a fixed clientKey. We need to mint
	// a signer the server will accept. Since newTestServer generates the
	// clientKey internally, expose it via the server struct and recover the
	// PEM from the underlying rsa.PrivateKey when generating it ourselves.
	hostKey := generateSigner(t)
	rsaKey, clientSigner, pemBytes := generateRSAKey(t)
	_ = rsaKey

	srv := &testServer{
		t:         t,
		hostKey:   hostKey,
		clientKey: clientSigner,
		username:  "alice",
		password:  "unused",
		stop:      make(chan struct{}),
		execHandle: func(cmd string) (string, string, int) {
			return "pk-ok\n", "", 0
		},
	}
	srv.config = &ssh.ServerConfig{
		PublicKeyCallback: func(c ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if c.User() == srv.username && bytes.Equal(key.Marshal(), srv.clientKey.PublicKey().Marshal()) {
				return nil, nil
			}
			return nil, fmt.Errorf("auth failed")
		},
	}
	srv.config.AddHostKey(hostKey)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv.listener = ln
	srv.wg.Add(1)
	go srv.serve()
	t.Cleanup(srv.Close)
	host, port := srv.addr()

	sess, err := Dial(context.Background(), SSHConfig{
		Host: host, Port: port, User: "alice", PrivateKey: pemBytes,
	}, NewInMemoryKnownHosts(), acceptingPrompt())
	require.NoError(t, err)
	defer func() { _ = sess.Close() }()

	var out bytes.Buffer
	exit, err := sess.Run(context.Background(), "whoami", &out, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, exit)
	assert.Equal(t, "pk-ok\n", out.String())
}

func TestSession_Run_ExitCodes(t *testing.T) {
	srv := newTestServer(t)
	srv.execHandle = func(cmd string) (string, string, int) {
		switch cmd {
		case "ok":
			return "alive\n", "", 0
		case "fail":
			return "", "boom\n", 7
		}
		return "", "?", 1
	}
	host, port := srv.addr()

	sess, err := Dial(context.Background(), SSHConfig{
		Host: host, Port: port, User: "alice", Password: "s3cret",
	}, NewInMemoryKnownHosts(), acceptingPrompt())
	require.NoError(t, err)
	defer func() { _ = sess.Close() }()

	t.Run("zero exit", func(t *testing.T) {
		var out, errOut bytes.Buffer
		exit, err := sess.Run(context.Background(), "ok", &out, &errOut)
		require.NoError(t, err)
		assert.Equal(t, 0, exit)
		assert.Equal(t, "alive\n", out.String())
	})

	t.Run("non-zero exit reports code", func(t *testing.T) {
		var out, errOut bytes.Buffer
		exit, err := sess.Run(context.Background(), "fail", &out, &errOut)
		require.NoError(t, err)
		assert.Equal(t, 7, exit)
		assert.Equal(t, "boom\n", errOut.String())
	})
}

func TestDial_RespectsParentContextCancel(t *testing.T) {
	// We can't reliably test Dialer.Timeout against OS TCP retransmit on
	// every platform, but we can check that an early ctx cancel propagates.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled

	_, err := Dial(ctx, SSHConfig{
		Host: "198.51.100.1", Port: 22, User: "x", Password: "y",
		DialTimeout: 30 * time.Second,
	}, NewInMemoryKnownHosts(), acceptingPrompt())
	require.Error(t, err)
}
