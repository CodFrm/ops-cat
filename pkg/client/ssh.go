package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSHConfig is the connection input. Exactly one of Password or PrivateKey
// must be non-empty; supplying both is an error to prevent accidental
// fallbacks that hide credential mistakes.
type SSHConfig struct {
	Host string
	Port int
	User string

	Password             string
	PrivateKey           []byte // PEM-encoded; empty when using password auth
	PrivateKeyPassphrase []byte // optional, only used with PrivateKey

	// DialTimeout caps the TCP dial + SSH handshake. Zero uses 15s.
	DialTimeout time.Duration
}

// HostKeyDecision tells Dial what to do when KnownHosts has no record for the
// target host. Returning Accept saves the key (TOFU) and continues; Reject
// aborts the dial.
type HostKeyDecision int

const (
	HostKeyReject HostKeyDecision = iota
	HostKeyAccept
)

// HostKeyPrompt is invoked exactly once per Dial when the remote presents a
// key not in KnownHosts. Implementations are expected to surface the
// fingerprint to the human operator (TOFU prompt) and return their decision.
//
// fingerprint is the SHA-256 fingerprint string ("SHA256:..."), suitable for
// display alongside what the user would see from `ssh -o
// VisualHostKey=no`.
type HostKeyPrompt func(host string, port int, fingerprint string) HostKeyDecision

// Session is an established SSH connection. It is not safe for concurrent
// Run calls — the typical use is one session per logical "shell" the user
// opened.
type Session struct {
	client *ssh.Client
	host   string
	port   int
}

// Dial opens an SSH connection. If the server's host key is unknown to
// hostKeys, prompt is invoked; if prompt approves, the key is persisted via
// hostKeys.Save before authentication proceeds.
//
// hostKeys and prompt are both required: nil hostKeys would silently disable
// MITM protection, nil prompt would deadlock TOFU.
func Dial(ctx context.Context, cfg SSHConfig, hostKeys KnownHosts, prompt HostKeyPrompt) (*Session, error) {
	if hostKeys == nil {
		return nil, errors.New("client: KnownHosts is required")
	}
	if prompt == nil {
		return nil, errors.New("client: HostKeyPrompt is required")
	}
	if cfg.Host == "" || cfg.Port <= 0 || cfg.User == "" {
		return nil, errors.New("client: Host, Port and User are required")
	}
	if cfg.Password != "" && len(cfg.PrivateKey) > 0 {
		return nil, errors.New("client: supply Password OR PrivateKey, not both")
	}
	if cfg.Password == "" && len(cfg.PrivateKey) == 0 {
		return nil, errors.New("client: Password or PrivateKey required")
	}

	auth, err := buildAuthMethod(cfg)
	if err != nil {
		return nil, err
	}

	timeout := cfg.DialTimeout
	if timeout == 0 {
		timeout = 15 * time.Second
	}

	sshCfg := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            []ssh.AuthMethod{auth},
		HostKeyCallback: tofuHostKeyCallback(cfg.Host, cfg.Port, hostKeys, prompt),
		Timeout:         timeout,
	}

	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))

	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(dialCtx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("client: tcp dial: %w", err)
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, sshCfg)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("client: ssh handshake: %w", err)
	}

	return &Session{
		client: ssh.NewClient(sshConn, chans, reqs),
		host:   cfg.Host,
		port:   cfg.Port,
	}, nil
}

// Run executes cmd remotely, streaming stdout/stderr to the supplied writers.
// Returns the remote exit code (0 on success, non-zero from
// *ssh.ExitError, or -1 for transport-level errors).
func (s *Session) Run(ctx context.Context, cmd string, stdout, stderr io.Writer) (int, error) {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}

	sess, err := s.client.NewSession()
	if err != nil {
		return -1, fmt.Errorf("client: new session: %w", err)
	}
	defer func() { _ = sess.Close() }()

	sess.Stdout = stdout
	sess.Stderr = stderr

	done := make(chan error, 1)
	go func() { done <- sess.Run(cmd) }()

	select {
	case <-ctx.Done():
		_ = sess.Signal(ssh.SIGKILL)
		return -1, ctx.Err()
	case err := <-done:
		if err == nil {
			return 0, nil
		}
		var exitErr *ssh.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitStatus(), nil
		}
		return -1, fmt.Errorf("client: run: %w", err)
	}
}

// Close shuts down the SSH client. Subsequent Run calls will fail.
func (s *Session) Close() error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Close()
}

func buildAuthMethod(cfg SSHConfig) (ssh.AuthMethod, error) {
	if cfg.Password != "" {
		return ssh.Password(cfg.Password), nil
	}
	var (
		signer ssh.Signer
		err    error
	)
	if len(cfg.PrivateKeyPassphrase) > 0 {
		signer, err = ssh.ParsePrivateKeyWithPassphrase(cfg.PrivateKey, cfg.PrivateKeyPassphrase)
	} else {
		signer, err = ssh.ParsePrivateKey(cfg.PrivateKey)
	}
	if err != nil {
		return nil, fmt.Errorf("client: parse private key: %w", err)
	}
	return ssh.PublicKeys(signer), nil
}

func tofuHostKeyCallback(host string, port int, hostKeys KnownHosts, prompt HostKeyPrompt) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		offered := key.Marshal()

		stored, err := hostKeys.Lookup(host, port)
		switch {
		case err == nil:
			if !bytes.Equal(stored, offered) {
				return fmt.Errorf("%w for %s:%d", ErrHostKeyMismatch, host, port)
			}
			return nil
		case errors.Is(err, ErrHostKeyNotFound):
			// fall through to prompt
		default:
			return fmt.Errorf("client: known_hosts lookup: %w", err)
		}

		decision := prompt(host, port, ssh.FingerprintSHA256(key))
		if decision != HostKeyAccept {
			return fmt.Errorf("client: host key rejected for %s:%d", host, port)
		}
		if err := hostKeys.Save(host, port, offered); err != nil {
			return fmt.Errorf("client: known_hosts save: %w", err)
		}
		return nil
	}
}
