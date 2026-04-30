package ssh_svc

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

func detectRemoteShell(client *ssh.Client) (string, string) {
	session, err := client.NewSession()
	if err != nil {
		return "/bin/sh", shellTypeUnsupported
	}
	defer func() {
		if closeErr := session.Close(); closeErr != nil && closeErr != io.EOF {
			logger.Default().Warn("close shell probe session", zap.Error(closeErr))
		}
	}()

	var out bytes.Buffer
	session.Stdout = &out
	session.Stderr = io.Discard
	if err := session.Run(`sh -lc 'printf "%s" "${SHELL:-/bin/sh}"'`); err != nil {
		return "/bin/sh", shellTypeUnsupported
	}

	shellPath := strings.TrimSpace(out.String())
	if shellPath == "" {
		shellPath = "/bin/sh"
	}
	return shellPath, normalizeShellType(shellPath)
}

func normalizeShellType(shellPath string) string {
	switch path.Base(shellPath) {
	case "bash":
		return shellTypeBash
	case "zsh":
		return shellTypeZsh
	case "ksh":
		return shellTypeKsh
	case "mksh":
		return shellTypeMksh
	default:
		return shellTypeUnsupported
	}
}

func generateSyncToken() (string, error) {
	buf := make([]byte, syncSequenceTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// buildEnableSyncCommand returns a single-line shell statement to be written
// to the running interactive shell's stdin. It installs the prompt-proof
// hook, sets the initial nonce, and emits the init:pid marker so the host
// can confirm the shell received the injection.
//
// shellType MUST be one of shellTypeBash/shellTypeZsh/shellTypeKsh/shellTypeMksh.
// Returns empty string for shellTypeUnsupported (or anything else).
func buildEnableSyncCommand(shellType, syncToken, promptNonce string) string {
	switch shellType {
	case shellTypeBash:
		return fmt.Sprintf(
			`opskat_next_prompt_nonce(){ local n r;n=$(date +%%s%%N 2>/dev/null||date +%%s 2>/dev/null||printf 0);r=${RANDOM:-0};printf '%%s-%%s-%%s' "$$" "$r" "$n";};`+
				`opskat_prompt_proof(){ local p c x;c=${OPSKAT_PROMPT_NONCE:-};[ -n "$c" ]||return;x=$(opskat_next_prompt_nonce);p=$(builtin pwd -P 2>/dev/null||builtin pwd 2>/dev/null||printf '');printf '\033]1337;opskat:%s:prompt:%%s:%%s:%%s\007' "$c" "$x" "$p";OPSKAT_PROMPT_NONCE=$x;};`+
				`OPSKAT_PROMPT_NONCE=%s;`+
				`PROMPT_COMMAND="opskat_prompt_proof${PROMPT_COMMAND:+;$PROMPT_COMMAND}";`+
				`printf '\033]1337;opskat:%s:init:pid:%%s\007' "$$"`+"\r",
			syncToken, shellQuote(promptNonce), syncToken)
	case shellTypeZsh:
		return fmt.Sprintf(
			`opskat_next_prompt_nonce(){ local n r;n=$(date +%%s%%N 2>/dev/null||date +%%s 2>/dev/null||printf 0);r=${RANDOM:-0};printf '%%s-%%s-%%s' "$$" "$r" "$n";};`+
				`opskat_prompt_proof(){ local p c x;c=${OPSKAT_PROMPT_NONCE:-};[[ -n "$c" ]]||return;x=$(opskat_next_prompt_nonce);p=$(pwd -P 2>/dev/null||pwd 2>/dev/null||printf '');printf '\033]1337;opskat:%s:prompt:%%s:%%s:%%s\007' "$c" "$x" "$p";OPSKAT_PROMPT_NONCE=$x;};`+
				`OPSKAT_PROMPT_NONCE=%s;`+
				`autoload -Uz add-zsh-hook;add-zsh-hook precmd opskat_prompt_proof;`+
				`printf '\033]1337;opskat:%s:init:pid:%%s\007' "$$"`+"\r",
			syncToken, shellQuote(promptNonce), syncToken)
	case shellTypeKsh, shellTypeMksh:
		return fmt.Sprintf(
			`opskat_next_prompt_nonce(){ OPSKAT_NOW=$(date +%%s%%N 2>/dev/null||date +%%s 2>/dev/null||printf 0);OPSKAT_RAND=${RANDOM:-0};printf '%%s-%%s-%%s' "$$" "$OPSKAT_RAND" "$OPSKAT_NOW";};`+
				`opskat_prompt_proof(){ OPSKAT_CURRENT=${OPSKAT_PROMPT_NONCE:-};[ -n "$OPSKAT_CURRENT" ]||return;OPSKAT_NEXT=$(opskat_next_prompt_nonce);OPSKAT_PWD=$(pwd -P 2>/dev/null||pwd 2>/dev/null||printf '');printf '\033]1337;opskat:%s:prompt:%%s:%%s:%%s\007' "$OPSKAT_CURRENT" "$OPSKAT_NEXT" "$OPSKAT_PWD";OPSKAT_PROMPT_NONCE=$OPSKAT_NEXT;};`+
				`OPSKAT_PROMPT_NONCE=%s;`+
				`OPSKAT_ORIG_PS1=${OPSKAT_ORIG_PS1:-$PS1};PS1='$(opskat_prompt_proof)'"$OPSKAT_ORIG_PS1";`+
				`printf '\033]1337;opskat:%s:init:pid:%%s\007' "$$"`+"\r",
			syncToken, shellQuote(promptNonce), syncToken)
	default:
		return ""
	}
}

// buildDisableSyncCommand returns a one-line statement that removes the hook
// and unsets helper functions. Safe to send even if EnableSync was never run.
func buildDisableSyncCommand(shellType string) string {
	switch shellType {
	case shellTypeBash:
		return `PROMPT_COMMAND=${PROMPT_COMMAND#opskat_prompt_proof};PROMPT_COMMAND=${PROMPT_COMMAND#;};unset -f opskat_prompt_proof opskat_next_prompt_nonce 2>/dev/null;unset OPSKAT_PROMPT_NONCE` + "\r"
	case shellTypeZsh:
		return `add-zsh-hook -d precmd opskat_prompt_proof 2>/dev/null;unset -f opskat_prompt_proof opskat_next_prompt_nonce 2>/dev/null;unset OPSKAT_PROMPT_NONCE` + "\r"
	case shellTypeKsh, shellTypeMksh:
		return `[ -n "$OPSKAT_ORIG_PS1" ] && PS1=$OPSKAT_ORIG_PS1;unset -f opskat_prompt_proof opskat_next_prompt_nonce 2>/dev/null;unset OPSKAT_PROMPT_NONCE OPSKAT_ORIG_PS1` + "\r"
	default:
		return ""
	}
}

func buildDirectoryChangeCommand(targetPath string) string {
	return fmt.Sprintf("builtin cd -- %s\r", shellQuote(targetPath))
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
