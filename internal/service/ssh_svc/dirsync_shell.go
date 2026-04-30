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

func buildInteractiveShellCommand(shellPath, shellType, syncToken, promptNonce string) string {
	switch shellType {
	case shellTypeBash:
		return fmt.Sprintf(`rc="$(mktemp "${TMPDIR:-/tmp}/opskat-bash-XXXXXX")" && cat >"$rc" <<'EOF'
[ -f "$HOME/.bashrc" ] && . "$HOME/.bashrc"
opskat_next_prompt_nonce() {
  local opskat_now opskat_rand
  opskat_now=$(date +%%s%%N 2>/dev/null || date +%%s 2>/dev/null || printf '0')
  opskat_rand=${RANDOM:-0}
  printf '%%s-%%s-%%s' "$$" "$opskat_rand" "$opskat_now"
}
opskat_prompt_proof() {
  local opskat_pwd opskat_current opskat_next
  opskat_current=${OPSKAT_PROMPT_NONCE:-}
  [ -n "$opskat_current" ] || return
  opskat_next=$(opskat_next_prompt_nonce)
  opskat_pwd=$(builtin pwd -P 2>/dev/null || builtin pwd 2>/dev/null || printf '')
  printf '\033]1337;opskat:%s:prompt:%%s:%%s:%%s\007' "$opskat_current" "$opskat_next" "$opskat_pwd"
  OPSKAT_PROMPT_NONCE=$opskat_next
}
OPSKAT_PROMPT_NONCE=%s
PROMPT_COMMAND="opskat_prompt_proof${PROMPT_COMMAND:+;$PROMPT_COMMAND}"
EOF
printf '\033]1337;opskat:%s:init:pid:%%s\007' "$$"
exec %s --rcfile "$rc" -i`, syncToken, shellQuote(promptNonce), syncToken, shellQuote(shellPath))
	case shellTypeZsh:
		return fmt.Sprintf(`dir="$(mktemp -d "${TMPDIR:-/tmp}/opskat-zsh-XXXXXX")" && cat >"$dir/.zshenv" <<'EOF_ENV'
[[ -f "$HOME/.zshenv" ]] && source "$HOME/.zshenv"
EOF_ENV
cat >"$dir/.zshrc" <<'EOF_RC'
[[ -f "$HOME/.zprofile" ]] && source "$HOME/.zprofile"
[[ -f "$HOME/.zshrc" ]] && source "$HOME/.zshrc"
autoload -Uz add-zsh-hook
opskat_next_prompt_nonce() {
  local opskat_now opskat_rand
  opskat_now=$(date +%%s%%N 2>/dev/null || date +%%s 2>/dev/null || printf '0')
  opskat_rand=${RANDOM:-0}
  printf '%%s-%%s-%%s' "$$" "$opskat_rand" "$opskat_now"
}
opskat_prompt_proof() {
  local opskat_pwd opskat_current opskat_next
  opskat_current=${OPSKAT_PROMPT_NONCE:-}
  [[ -n "$opskat_current" ]] || return
  opskat_next=$(opskat_next_prompt_nonce)
  opskat_pwd=$(pwd -P 2>/dev/null || pwd 2>/dev/null || printf '')
  printf '\033]1337;opskat:%s:prompt:%%s:%%s:%%s\007' "$opskat_current" "$opskat_next" "$opskat_pwd"
  OPSKAT_PROMPT_NONCE=$opskat_next
}
OPSKAT_PROMPT_NONCE=%s
add-zsh-hook precmd opskat_prompt_proof
EOF_RC
export ZDOTDIR="$dir"
printf '\033]1337;opskat:%s:init:pid:%%s\007' "$$"
exec %s -i`, syncToken, shellQuote(promptNonce), syncToken, shellQuote(shellPath))
	case shellTypeKsh, shellTypeMksh:
		return fmt.Sprintf(`envfile="$(mktemp "${TMPDIR:-/tmp}/opskat-ksh-XXXXXX")" && cat >"$envfile" <<'EOF'
[ -f "$HOME/.profile" ] && . "$HOME/.profile"
opskat_next_prompt_nonce() {
  OPSKAT_NOW=$(date +%%s%%N 2>/dev/null || date +%%s 2>/dev/null || printf '0')
  OPSKAT_RAND=${RANDOM:-0}
  printf '%%s-%%s-%%s' "$$" "$OPSKAT_RAND" "$OPSKAT_NOW"
}
opskat_prompt_proof() {
  OPSKAT_CURRENT=${OPSKAT_PROMPT_NONCE:-}
  [ -n "$OPSKAT_CURRENT" ] || return
  OPSKAT_NEXT=$(opskat_next_prompt_nonce)
  OPSKAT_PWD=$(pwd -P 2>/dev/null || pwd 2>/dev/null || printf '')
  printf '\033]1337;opskat:%s:prompt:%%s:%%s:%%s\007' "$OPSKAT_CURRENT" "$OPSKAT_NEXT" "$OPSKAT_PWD"
  OPSKAT_PROMPT_NONCE=$OPSKAT_NEXT
}
OPSKAT_PROMPT_NONCE=%s
PS1='$(opskat_prompt_proof)'"$PS1"
EOF
export ENV="$envfile"
printf '\033]1337;opskat:%s:init:pid:%%s\007' "$$"
exec %s -i`, syncToken, shellQuote(promptNonce), syncToken, shellQuote(shellPath))
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
