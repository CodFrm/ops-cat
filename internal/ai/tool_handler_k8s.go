package ai

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
	"mvdan.cc/sh/v3/syntax"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/repository/asset_repo"
	"github.com/opskat/opskat/internal/service/asset_svc"
)

type k8sCommandPlan struct {
	Args             []string
	EffectiveCommand string
}

func handleExecK8s(ctx context.Context, args map[string]any) (string, error) {
	assetID := argInt64(args, "asset_id")
	command := argString(args, "command")
	if assetID == 0 {
		return "", fmt.Errorf("missing required parameter: asset_id")
	}
	if strings.TrimSpace(command) == "" {
		return "", fmt.Errorf("missing required parameter: command")
	}

	asset, err := asset_svc.Asset().Get(ctx, assetID)
	if err != nil {
		return "", fmt.Errorf("get asset: %w", err)
	}
	if asset == nil || !asset.IsK8s() {
		return "", fmt.Errorf("asset %d is not a k8s cluster", assetID)
	}

	cfg, err := asset.GetK8sConfig()
	if err != nil {
		return "", fmt.Errorf("get k8s config: %w", err)
	}
	if cfg.Kubeconfig == "" {
		return "", fmt.Errorf("no kubeconfig configured for this k8s asset")
	}

	plan, err := buildK8sCommandPlan(command, cfg)
	if err != nil {
		return "", err
	}

	if checker := GetPolicyChecker(ctx); checker != nil {
		result := checker.CheckForAsset(ctx, assetID, asset_entity.AssetTypeK8s, plan.EffectiveCommand)
		setCheckResult(ctx, result)
		if result.Decision != Allow {
			return result.Message, nil
		}
	}

	if asset.SSHTunnelID != 0 {
		return executeK8sCommandOverSSH(ctx, asset.SSHTunnelID, cfg.Kubeconfig, plan.Args)
	}
	return executeK8sCommandLocal(ctx, cfg.Kubeconfig, plan.Args)
}

func k8sAuditCommandFromArgs(args map[string]any) string {
	command := argString(args, "command")
	if strings.TrimSpace(command) == "" {
		return ""
	}

	var cfg *asset_entity.K8sConfig
	assetID := argInt64(args, "asset_id")
	if assetID > 0 && asset_repo.Asset() != nil {
		if asset, err := asset_repo.Asset().Find(context.Background(), assetID); err == nil && asset != nil && asset.IsK8s() {
			if k8sCfg, cfgErr := asset.GetK8sConfig(); cfgErr == nil {
				cfg = k8sCfg
			}
		}
	}

	plan, err := buildK8sCommandPlan(command, cfg)
	if err != nil {
		return command
	}
	return plan.EffectiveCommand
}

func buildK8sCommandPlan(rawCommand string, cfg *asset_entity.K8sConfig) (*k8sCommandPlan, error) {
	args, err := parseK8sCommandArgs(rawCommand)
	if err != nil {
		return nil, err
	}

	finalArgs := make([]string, 0, len(args)+4)
	if cfg != nil {
		if cfg.Context != "" && !hasLongFlag(args, "--context") {
			finalArgs = append(finalArgs, "--context", cfg.Context)
		}
		if cfg.Namespace != "" && !hasNamespaceFlag(args) {
			finalArgs = append(finalArgs, "--namespace", cfg.Namespace)
		}
	}
	finalArgs = append(finalArgs, args...)

	return &k8sCommandPlan{
		Args:             finalArgs,
		EffectiveCommand: "kubectl " + strings.Join(finalArgs, " "),
	}, nil
}

func parseK8sCommandArgs(command string) ([]string, error) {
	parser := syntax.NewParser()
	file, err := parser.Parse(strings.NewReader(command), "")
	if err != nil {
		return nil, fmt.Errorf("invalid kubectl command: %w", err)
	}
	if len(file.Stmts) != 1 {
		return nil, fmt.Errorf("kubectl tool only supports a single command")
	}

	stmt := file.Stmts[0]
	if len(stmt.Redirs) > 0 {
		return nil, fmt.Errorf("kubectl tool does not allow shell redirection")
	}

	call, ok := stmt.Cmd.(*syntax.CallExpr)
	if !ok {
		return nil, fmt.Errorf("kubectl tool only supports a simple command")
	}
	if len(call.Assigns) > 0 {
		return nil, fmt.Errorf("kubectl tool does not allow shell variable assignments")
	}
	if len(call.Args) == 0 {
		return nil, fmt.Errorf("missing kubectl command")
	}

	args := make([]string, 0, len(call.Args))
	for _, word := range call.Args {
		arg, err := shellWordLiteral(word)
		if err != nil {
			return nil, err
		}
		if arg != "" {
			args = append(args, arg)
		}
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("missing kubectl command")
	}

	if isKubectlProgram(args[0]) {
		args = args[1:]
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("missing kubectl subcommand")
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--kubeconfig":
			return nil, fmt.Errorf("do not pass --kubeconfig to exec_k8s; the asset kubeconfig is used automatically")
		case strings.HasPrefix(arg, "--kubeconfig="):
			return nil, fmt.Errorf("do not pass --kubeconfig to exec_k8s; the asset kubeconfig is used automatically")
		}
	}

	return args, nil
}

func shellWordLiteral(word *syntax.Word) (string, error) {
	var b strings.Builder
	for _, part := range word.Parts {
		if err := appendShellWordPart(&b, part); err != nil {
			return "", err
		}
	}
	return b.String(), nil
}

func appendShellWordPart(b *strings.Builder, part syntax.WordPart) error {
	switch x := part.(type) {
	case *syntax.Lit:
		b.WriteString(x.Value)
		return nil
	case *syntax.SglQuoted:
		b.WriteString(x.Value)
		return nil
	case *syntax.DblQuoted:
		for _, inner := range x.Parts {
			if err := appendShellWordPart(b, inner); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("kubectl tool does not allow shell expansions or command substitution")
	}
}

func isKubectlProgram(program string) bool {
	normalized := strings.ReplaceAll(program, "\\", "/")
	if idx := strings.LastIndex(normalized, "/"); idx >= 0 {
		normalized = normalized[idx+1:]
	}
	return strings.EqualFold(normalized, "kubectl") || strings.EqualFold(normalized, "kubectl.exe")
}

func hasLongFlag(args []string, name string) bool {
	for _, arg := range args {
		if arg == name || strings.HasPrefix(arg, name+"=") {
			return true
		}
	}
	return false
}

func hasNamespaceFlag(args []string) bool {
	for _, arg := range args {
		if arg == "-n" || arg == "--namespace" || strings.HasPrefix(arg, "--namespace=") {
			return true
		}
	}
	return false
}

func executeK8sCommandLocal(ctx context.Context, kubeconfig string, args []string) (string, error) {
	if _, err := exec.LookPath("kubectl"); err != nil {
		return "", fmt.Errorf("kubectl not found on local machine: %w", err)
	}

	kubeconfigPath, err := writeTempKubeconfig(kubeconfig)
	if err != nil {
		return "", err
	}
	defer removeTempFile(kubeconfigPath)

	cmd := exec.CommandContext(ctx, "kubectl", args...) //nolint:gosec
	cmd.Env = append(os.Environ(), "KUBECONFIG="+kubeconfigPath)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("kubectl command failed: %s", strings.TrimSpace(stderr.String()))
		}
		return "", fmt.Errorf("kubectl command failed: %w", err)
	}

	return formatCommandOutput(stdout.String(), stderr.String()), nil
}

func executeK8sCommandOverSSH(ctx context.Context, sshAssetID int64, kubeconfig string, args []string) (string, error) {
	if strings.TrimSpace(kubeconfig) == "" {
		return "", fmt.Errorf("no kubeconfig configured for this k8s asset")
	}

	kubectlCmd := buildQuotedKubectlCommand(args)
	script := fmt.Sprintf(`tmp="${TMPDIR:-/tmp}/opskat-kubeconfig-$$"; umask 077; cat > "$tmp"; KUBECONFIG="$tmp" %s; status=$?; rm -f "$tmp"; exit $status`, kubectlCmd)
	return executeSSHCommandWithStdin(ctx, sshAssetID, "sh -lc "+quotePosixArg(script), strings.NewReader(kubeconfig))
}

func buildQuotedKubectlCommand(args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, "kubectl")
	for _, arg := range args {
		parts = append(parts, quotePosixArg(arg))
	}
	return strings.Join(parts, " ")
}

func quotePosixArg(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func writeTempKubeconfig(kubeconfig string) (string, error) {
	file, err := os.CreateTemp("", "opskat-kubeconfig-*")
	if err != nil {
		return "", fmt.Errorf("create temp kubeconfig: %w", err)
	}
	path := file.Name()
	if closeErr := file.Close(); closeErr != nil {
		removeTempFile(path)
		return "", fmt.Errorf("close temp kubeconfig: %w", closeErr)
	}
	if err := os.WriteFile(path, []byte(kubeconfig), 0o600); err != nil {
		removeTempFile(path)
		return "", fmt.Errorf("write temp kubeconfig: %w", err)
	}
	return path, nil
}

func removeTempFile(path string) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		logger.Default().Warn("remove temp kubeconfig", zap.String("path", path), zap.Error(err))
	}
}

func formatCommandOutput(stdout, stderr string) string {
	output := stdout
	if strings.TrimSpace(stderr) != "" {
		if output != "" && !strings.HasSuffix(output, "\n") {
			output += "\n"
		}
		output += "STDERR:\n" + stderr
	}
	return output
}

func executeSSHCommandWithStdin(ctx context.Context, assetID int64, command string, stdin io.Reader) (string, error) {
	client, cleanup, err := dialAssetSSH(ctx, assetID)
	if err != nil {
		return "", err
	}
	defer cleanup()
	return runSSHCommandWithStdin(ctx, client, command, stdin)
}

func runSSHCommandWithStdin(ctx context.Context, client *ssh.Client, command string, stdin io.Reader) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}
	defer func() {
		if err := session.Close(); err != nil && !isExpectedCloseErr(err) {
			logger.Default().Warn("close SSH session", zap.Error(err))
		}
	}()

	if stdin != nil {
		session.Stdin = stdin
	}

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	runCh := make(chan error, 1)
	go func() {
		runCh <- session.Run(command)
	}()

	select {
	case err := <-runCh:
		if err != nil {
			if stderr.Len() > 0 {
				return "", fmt.Errorf("command failed: %s", strings.TrimSpace(stderr.String()))
			}
			return "", fmt.Errorf("command failed: %w", err)
		}
	case <-ctx.Done():
		if err := session.Close(); err != nil && !isExpectedCloseErr(err) {
			logger.Default().Warn("close SSH session on cancel", zap.Error(err))
		}
		if err := client.Close(); err != nil && !isExpectedCloseErr(err) {
			logger.Default().Warn("close SSH client on cancel", zap.Error(err))
		}
		return "", ctx.Err()
	}

	return formatCommandOutput(stdout.String(), stderr.String()), nil
}
