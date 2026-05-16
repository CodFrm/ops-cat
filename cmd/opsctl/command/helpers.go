package command

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// extractCommand extracts the command string after "--" separator.
// If no "--" is found, all args are joined as the command.
func extractCommand(args []string) string {
	for i, arg := range args {
		if arg == "--" {
			parts := args[i+1:]
			if len(parts) == 0 {
				return ""
			}
			return strings.Join(parts, " ")
		}
	}
	if len(args) > 0 {
		return strings.Join(args, " ")
	}
	return ""
}

// parseRemotePathCtx parses <asset>:<path> format where <asset> is an ID or name.
// Returns (assetID, path, error). If not a remote path, assetID is 0.
func parseRemotePathCtx(ctx context.Context, s string) (int64, string, error) {
	idx := strings.Index(s, ":")
	if idx <= 0 {
		return 0, s, nil
	}
	prefix := s[:idx]
	remotePath := s[idx+1:]

	// Must start with / to be a remote path (avoid matching C:\windows paths or names without colon)
	if !strings.HasPrefix(remotePath, "/") {
		return 0, s, nil
	}

	id, err := resolveAssetID(ctx, prefix)
	if err != nil {
		return 0, "", fmt.Errorf("resolving asset %q: %w", prefix, err)
	}
	return id, remotePath, nil
}

// parseRemotePath parses numeric assetID:path strings without repository lookup.
func parseRemotePath(s string) (int64, string) {
	idx := strings.Index(s, ":")
	if idx <= 0 {
		return 0, s
	}
	id, err := strconv.ParseInt(s[:idx], 10, 64)
	if err != nil {
		return 0, s
	}
	return id, s[idx+1:]
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
