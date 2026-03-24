package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/opskat/opskat/internal/ai"
	"github.com/opskat/opskat/internal/sshpool"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

func cmdSSH(ctx context.Context, args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		printSSHUsage()
		if len(args) > 0 {
			return 0
		}
		return 1
	}

	asset, err := resolveAsset(ctx, args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// 尝试通过 proxy 连接（复用 ops-cat 连接池）
	if proxy := getSSHProxyClient(); proxy != nil {
		return cmdSSHViaProxy(proxy, asset.ID)
	}

	// Fallback: 直连
	return cmdSSHDirect(ctx, asset.ID)
}

// cmdSSHViaProxy 通过 ops-cat 连接池代理建立交互式 SSH
func cmdSSHViaProxy(proxy *sshpool.Client, assetID int64) int {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to set raw terminal: %v\n", err)
		return 1
	}
	defer func() { _ = term.Restore(fd, oldState) }()

	width, height, err := term.GetSize(fd)
	if err != nil {
		width, height = 80, 24
	}

	resizeCh, stopResize := watchTerminalResizeCh(fd)
	defer stopResize()

	exitCode, err := proxy.InteractiveSSH(sshpool.ProxyRequest{
		AssetID: assetID,
		Cols:    width,
		Rows:    height,
	}, os.Stdin, os.Stdout, resizeCh)
	if err != nil {
		_ = term.Restore(fd, oldState)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	return exitCode
}

// cmdSSHDirect 直连建立交互式 SSH（原逻辑）
func cmdSSHDirect(ctx context.Context, assetID int64) int {
	client, err := ai.DialSSHClient(ctx, assetID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	defer func() { _ = client.Close() }()

	session, err := client.NewSession()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create session: %v\n", err)
		return 1
	}
	defer func() { _ = session.Close() }()

	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to set raw terminal: %v\n", err)
		return 1
	}
	defer func() { _ = term.Restore(fd, oldState) }()

	width, height, err := term.GetSize(fd)
	if err != nil {
		width, height = 80, 24
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm-256color", height, width, modes); err != nil {
		_ = term.Restore(fd, oldState)
		fmt.Fprintf(os.Stderr, "Error: failed to request PTY: %v\n", err)
		return 1
	}

	session.Stdin = os.Stdin
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	stopResize := watchTerminalResize(session, fd)
	defer stopResize()

	if err := session.Shell(); err != nil {
		_ = term.Restore(fd, oldState)
		fmt.Fprintf(os.Stderr, "Error: failed to start shell: %v\n", err)
		return 1
	}

	if err := session.Wait(); err != nil {
		var exitErr *ssh.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitStatus()
		}
		return 1
	}
	return 0
}

func printSSHUsage() {
	fmt.Fprint(os.Stderr, `Usage:
  opsctl ssh <asset>

Arguments:
  asset     Asset name or numeric ID

Opens an interactive SSH terminal session to the specified asset.
This is intended for human use and does not require desktop app approval.

Examples:
  opsctl ssh web-server
  opsctl ssh 1
  opsctl ssh production/web-01
`)
}
