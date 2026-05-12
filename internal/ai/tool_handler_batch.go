package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
)

// batchCommandItem 是 LLM 提交的批量项：对 N 个资产并发跑同一类命令。
type batchCommandItem struct {
	Asset   string `json:"asset"`
	Type    string `json:"type"`
	Command string `json:"command"`
}

// batchResultItem 是单条命令的执行结果（聚合后整体返回给 LLM）。
type batchResultItem struct {
	AssetID   int64  `json:"asset_id"`
	AssetName string `json:"asset_name"`
	Type      string `json:"type"`
	Command   string `json:"command"`
	ExitCode  int    `json:"exit_code"`
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr,omitempty"`
	Error     string `json:"error,omitempty"`
}

// handleBatchCommand 并发执行多条命令并聚合返回。
// 流程：解析 → 策略预检 → 聚合 needConfirm 一次审批 → max 10 并发执行。
// 与 main 上的语义对齐；refactor 上派发改为直接调 package 内 handler，
// agentRole 参数随 CommandConfirmFunc 签名简化一并去掉。
func handleBatchCommand(ctx context.Context, args map[string]any) (string, error) {
	commandsRaw, ok := args["commands"]
	if !ok {
		return "", fmt.Errorf("missing required parameter: commands")
	}

	commandsJSON, err := json.Marshal(commandsRaw)
	if err != nil {
		return "", fmt.Errorf("invalid commands parameter: %w", err)
	}
	var commands []batchCommandItem
	// LLM 偶尔会把 commands 当字符串 JSON 传，兜底再 unmarshal 一层。
	if err := json.Unmarshal(commandsJSON, &commands); err != nil {
		var str string
		if jerr := json.Unmarshal(commandsJSON, &str); jerr == nil {
			if uerr := json.Unmarshal([]byte(str), &commands); uerr != nil {
				return "", fmt.Errorf("invalid commands format: %w", uerr)
			}
		} else {
			return "", fmt.Errorf("invalid commands format: %w", err)
		}
	}

	if len(commands) == 0 {
		return "No commands to execute.", nil
	}

	for i := range commands {
		if commands[i].Type == "" {
			commands[i].Type = "exec"
		}
	}

	checker := GetPolicyChecker(ctx)

	type resolvedCmd struct {
		item      batchCommandItem
		assetID   int64
		assetName string
		decision  string // "allow" / "deny" / "needConfirm"
		denyMsg   string
	}
	resolved := make([]resolvedCmd, 0, len(commands))

	for _, cmd := range commands {
		assetID, assetName, resolveErr := resolveAssetForBatch(ctx, cmd.Asset)
		if resolveErr != nil {
			resolved = append(resolved, resolvedCmd{
				item: cmd, decision: "deny", denyMsg: fmt.Sprintf("asset not found: %s", cmd.Asset),
			})
			continue
		}

		decision := "allow"
		denyMsg := ""
		if checker != nil {
			result := CheckPermission(ctx, batchApprovalAssetType(cmd.Type), assetID, cmd.Command)
			switch result.Decision {
			case Deny:
				decision = "deny"
				denyMsg = result.Message
			case NeedConfirm:
				decision = "needConfirm"
			case Allow:
				decision = "allow"
			}
		}

		resolved = append(resolved, resolvedCmd{
			item: cmd, assetID: assetID, assetName: assetName,
			decision: decision, denyMsg: denyMsg,
		})
	}

	// 聚合 needConfirm，一次性弹审批。
	if checker != nil && checker.ConfirmFunc() != nil {
		var needConfirmItems []ApprovalItem
		var needConfirmIndices []int
		for i, r := range resolved {
			if r.decision == "needConfirm" {
				needConfirmItems = append(needConfirmItems, ApprovalItem{
					Type:      r.item.Type,
					AssetID:   r.assetID,
					AssetName: r.assetName,
					Command:   r.item.Command,
				})
				needConfirmIndices = append(needConfirmIndices, i)
			}
		}
		if len(needConfirmItems) > 0 {
			resp := checker.ConfirmFunc()(ctx, "batch", needConfirmItems)
			for _, idx := range needConfirmIndices {
				if resp.Decision == "deny" {
					resolved[idx].decision = "deny"
					resolved[idx].denyMsg = "user denied batch execution"
				} else {
					resolved[idx].decision = "allow"
				}
			}
		}
	}

	var approved, denied []resolvedCmd
	for _, r := range resolved {
		if r.decision == "allow" {
			approved = append(approved, r)
		} else {
			denied = append(denied, r)
		}
	}

	const maxConcurrency = 10
	sem := make(chan struct{}, maxConcurrency)
	var mu sync.Mutex
	results := make([]batchResultItem, 0, len(commands))

	for _, r := range denied {
		results = append(results, batchResultItem{
			AssetID: r.assetID, AssetName: r.assetName,
			Type: r.item.Type, Command: r.item.Command,
			ExitCode: -1, Error: fmt.Sprintf("denied: %s", r.denyMsg),
		})
	}

	var wg sync.WaitGroup
	for _, r := range approved {
		wg.Add(1)
		go func(r resolvedCmd) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			result := executeBatchItem(ctx, r.item, r.assetID, r.assetName)
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		}(r)
	}
	wg.Wait()

	output, err := json.MarshalIndent(map[string]any{"results": results}, "", "  ")
	if err != nil {
		return "", err
	}
	return string(output), nil
}

// executeBatchItem 把单条命令派发到对应 handler。
// 直接调 package 内私有函数，避免再走 AllToolDefs 名字派发表。
func executeBatchItem(ctx context.Context, item batchCommandItem, assetID int64, assetName string) batchResultItem {
	result := batchResultItem{
		AssetID: assetID, AssetName: assetName,
		Type: item.Type, Command: item.Command,
	}

	var (
		output string
		err    error
	)
	switch item.Type {
	case "exec":
		output, err = handleRunCommand(ctx, map[string]any{"asset_id": assetID, "command": item.Command})
	case "sql":
		output, err = handleExecSQL(ctx, map[string]any{"asset_id": assetID, "sql": item.Command})
	case "redis":
		output, err = handleExecRedis(ctx, map[string]any{"asset_id": assetID, "command": item.Command})
	default:
		result.ExitCode = -1
		result.Error = fmt.Sprintf("unknown type: %s", item.Type)
		return result
	}

	if err != nil {
		result.ExitCode = -1
		result.Error = err.Error()
		return result
	}
	result.ExitCode = 0
	result.Stdout = output
	return result
}

// resolveAssetForBatch 把 LLM 传入的 asset 标识（name 或 id）解析成 (id, name)。
// 复用 handleGetAsset 的解析逻辑，避免重复实现 name→id 查询。
func resolveAssetForBatch(ctx context.Context, assetRef string) (int64, string, error) {
	out, err := handleGetAsset(ctx, map[string]any{"id": assetRef})
	if err != nil {
		return 0, "", err
	}
	var asset struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(out), &asset); err != nil {
		return 0, "", fmt.Errorf("cannot resolve asset: %s", assetRef)
	}
	return asset.ID, asset.Name, nil
}

// batchApprovalAssetType 把 batch 的 type 映射成 CheckPermission 期望的资产类型字符串。
// CheckPermission 内部据此选择对应的策略组（exec→ssh、sql→database、redis→redis）。
func batchApprovalAssetType(batchType string) string {
	switch batchType {
	case "sql":
		return asset_entity.AssetTypeDatabase
	case "redis":
		return asset_entity.AssetTypeRedis
	default:
		return asset_entity.AssetTypeSSH
	}
}
