package ai

import (
	"context"
	"fmt"
	"time"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/service/serial_svc"
)

type serialManagerKeyType struct{}

// WithSerialManager 将串口管理器注入 context
func WithSerialManager(ctx context.Context, mgr serial_svc.CommandManager) context.Context {
	return context.WithValue(ctx, serialManagerKeyType{}, mgr)
}

func getSerialManager(ctx context.Context) serial_svc.CommandManager {
	if mgr, ok := ctx.Value(serialManagerKeyType{}).(serial_svc.CommandManager); ok {
		return mgr
	}

	return nil
}

func handleRunSerialCommand(ctx context.Context, args map[string]any) (string, error) {
	assetID := argInt64(args, "asset_id")
	command := argString(args, "command")
	if assetID == 0 {
		return "", fmt.Errorf("missing required parameter: asset_id")
	}
	if command == "" {
		return "", fmt.Errorf("missing required parameter: command")
	}

	mgr := getSerialManager(ctx)
	if mgr == nil {
		return "", fmt.Errorf("serial manager not available")
	}

	sess, ok := mgr.GetSessionByAssetID(assetID)
	if !ok {
		return "", fmt.Errorf("no active serial session for asset %d — please connect the serial port first", assetID)
	}

	// 权限检查
	if checker := GetPolicyChecker(ctx); checker != nil {
		result := checker.CheckForAsset(ctx, assetID, asset_entity.AssetTypeSerial, command)
		setCheckResult(ctx, result)
		if result.Decision != Allow {
			return result.Message, nil
		}
	}

	output, err := sess.ExecCommand(command, 2*time.Second, 15*time.Second)
	if err != nil {
		return "", fmt.Errorf("serial command failed: %w", err)
	}
	if output == "" {
		return "(no output)", nil
	}

	return output, nil
}
