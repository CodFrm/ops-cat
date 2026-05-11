package app

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/opskat/opskat/internal/ai"
	"github.com/opskat/opskat/internal/aiagent"
	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/service/asset_svc"
	"github.com/opskat/opskat/internal/service/group_svc"
)

// === Audit Writer ===

// opsAuditWriter 实现 aiagent.AuditWriter，把每次工具调用经 ai.DefaultAuditWriter
// 写入 audit_repo。Write 误吞错误（审计失败不影响 agent 主流程，但 logger 会记）。
type opsAuditWriter struct {
	inner *ai.DefaultAuditWriter
}

func newOpsAuditWriter() *opsAuditWriter {
	return &opsAuditWriter{inner: ai.NewDefaultAuditWriter()}
}

func (w *opsAuditWriter) Write(ctx context.Context, toolName, inputJSON, outputJSON string, isError bool, decision *ai.CheckResult) error {
	var errVal error
	if isError {
		errVal = fmt.Errorf("tool reported error")
	}
	w.inner.WriteToolCall(ctx, ai.ToolCallInfo{
		ToolName: toolName,
		ArgsJSON: inputJSON,
		Result:   outputJSON,
		Error:    errVal,
		Decision: decision,
	})
	return nil
}

// === Policy Checker ===

// opsPolicyChecker 实现 aiagent.PolicyChecker：按 toolName 提取 assetID+command+kind，
// 没匹配（list_assets 等只读工具）直接放行；匹配后走 ai.CheckPermission：
//   - Allow → PolicyAllow
//   - Deny  → PolicyDeny + 原因
//   - NeedConfirm → PolicyConfirm + 构造 ApprovalItem
type opsPolicyChecker struct{}

func newOpsPolicyChecker() *opsPolicyChecker { return &opsPolicyChecker{} }

func (c *opsPolicyChecker) Check(ctx context.Context, toolName string, input map[string]any) (aiagent.PolicyOutcome, error) {
	assetID, command, kind, ok := extractPolicyTarget(toolName, input)
	if !ok {
		// list_assets / get_asset / list_groups / 等 read-only 工具不走策略门控。
		// DecisionSource 留空 —— 不是来自策略放行（无策略涉及），而是"未受策略管辖"。
		return aiagent.PolicyOutcome{Decision: aiagent.PolicyAllow}, nil
	}

	// 本地 cago 工具（bash/write/edit）无 assetID，跳过 ai.CheckPermission，直接走 Confirm。
	if isLocalKind(kind) {
		return aiagent.PolicyOutcome{
			Decision: aiagent.PolicyConfirm,
			Item: ai.ApprovalItem{
				Type:    kind,
				Command: command,
				Detail:  approvalDetail(command),
			},
		}, nil
	}

	assetType := policyAssetTypeForKind(kind)
	result := ai.CheckPermission(ctx, assetType, assetID, command)
	switch result.Decision {
	case ai.Allow:
		return aiagent.PolicyOutcome{
			Decision:       aiagent.PolicyAllow,
			DecisionSource: result.DecisionSource,
			MatchedPattern: result.MatchedPattern,
		}, nil
	case ai.Deny:
		return aiagent.PolicyOutcome{
			Decision:       aiagent.PolicyDeny,
			Reason:         result.Message,
			DecisionSource: result.DecisionSource,
			MatchedPattern: result.MatchedPattern,
		}, nil
	default:
		return aiagent.PolicyOutcome{
			Decision: aiagent.PolicyConfirm,
			Item:     buildApprovalItem(ctx, kind, assetID, command),
		}, nil
	}
}

// approvalDetailMax 是 ApprovalItem.Detail 的截断阈值。前端审批卡用单行小字
// 展示 Detail，太长会撑破布局；超过截断 + "…" 后缀。
const approvalDetailMax = 256

// buildApprovalItem 把策略 NeedConfirm 的目标拼成富展示项：补 assetName / group /
// detail。任何查询失败都按空串兜底，绝不让 UI 弹卡链路失败（用户看不到弹卡时
// 这条 hook 会卡住整个 turn）。
func buildApprovalItem(ctx context.Context, kind string, assetID int64, command string) ai.ApprovalItem {
	item := ai.ApprovalItem{
		Type:    kind,
		AssetID: assetID,
		Command: command,
		Detail:  approvalDetail(command),
	}
	if assetID <= 0 {
		return item
	}
	asset, err := asset_svc.Asset().Get(ctx, assetID)
	if err != nil || asset == nil {
		return item
	}
	item.AssetName = asset.Name
	if asset.GroupID > 0 {
		if g, err := group_svc.Group().Get(ctx, asset.GroupID); err == nil && g != nil {
			item.GroupID = g.ID
			item.GroupName = g.Name
		}
	}
	return item
}

// approvalDetail 把命令文本压成单行 detail：换行替成 ↵，长度按 rune 截断到
// approvalDetailMax 上限。Detail 是给用户一眼能看出"我在批准什么"的二次提示，
// 完整命令仍在 Command 字段里供 UI 展开查看。
func approvalDetail(command string) string {
	if command == "" {
		return ""
	}
	flat := strings.ReplaceAll(strings.ReplaceAll(command, "\r\n", "↵"), "\n", "↵")
	runes := []rune(flat)
	if len(runes) <= approvalDetailMax {
		return flat
	}
	return string(runes[:approvalDetailMax]) + "…"
}

// extractPolicyTarget 按 toolName 从 cago tool input 提取 (assetID, commandSummary, kind, isPolicyGated)。
// 与 v1 extractAssetAndCommand 同语义。返回 ok=false 表示这个工具不属于策略门控范围。
func extractPolicyTarget(toolName string, args map[string]any) (int64, string, string, bool) {
	getStr := func(k string) string {
		if v, ok := args[k].(string); ok {
			return v
		}
		return ""
	}
	getNum := func(k string) int64 {
		switch v := args[k].(type) {
		case float64:
			return int64(v)
		case int:
			return int64(v)
		case int64:
			return v
		case json.Number:
			n, _ := v.Int64()
			return n
		}
		return 0
	}
	switch toolName {
	case "run_command":
		return getNum("asset_id"), getStr("command"), "exec", true
	case "exec_sql":
		return getNum("asset_id"), getStr("sql"), "sql", true
	case "exec_redis":
		return getNum("asset_id"), getStr("command"), "redis", true
	case "exec_mongo":
		return getNum("asset_id"), getStr("operation"), "mongo", true
	case "exec_k8s":
		return getNum("asset_id"), getStr("command"), "k8s", true
	case "upload_file":
		return getNum("asset_id"), "upload " + getStr("local_path") + " → " + getStr("remote_path"), "cp", true
	case "download_file":
		return getNum("asset_id"), "download " + getStr("remote_path") + " → " + getStr("local_path"), "cp", true
	case "kafka_cluster", "kafka_topic", "kafka_consumer_group", "kafka_acl",
		"kafka_schema", "kafka_connect", "kafka_message":
		return getNum("asset_id"), getStr("operation") + ":" + getStr("topic"), "kafka", true
	// cago built-in 本地工具
	case "bash":
		return 0, getStr("command"), "local_bash", true
	case "write":
		return 0, getStr("path"), "local_write", true
	case "edit":
		return 0, getStr("path"), "local_edit", true
	}
	return 0, "", "", false
}

func isLocalKind(kind string) bool {
	return kind == "local_bash" || kind == "local_write" || kind == "local_edit"
}

func policyAssetTypeForKind(kind string) string {
	switch kind {
	case "exec", "cp":
		return asset_entity.AssetTypeSSH
	case "sql":
		return asset_entity.AssetTypeDatabase
	case "redis":
		return asset_entity.AssetTypeRedis
	case "mongo":
		return asset_entity.AssetTypeMongoDB
	case "kafka":
		return asset_entity.AssetTypeKafka
	case "k8s":
		return asset_entity.AssetTypeK8s
	}
	return asset_entity.AssetTypeSSH
}

// === Mention Resolver ===

// mentionPattern 匹配 @name 中的 name 部分；name 允许字母数字下划线点和连字符（与
// 前端 chip 显示规则保持一致）。中文名也允许（\p{L}）。
var mentionPattern = regexp.MustCompile(`@([\p{L}\p{N}_\-.]+)`)

// mentionCacheTTL 是资产索引缓存的有效期：单次用户输入里通常 < 1s 间内会被
// 多次 Expand（mention hook + 历史回放 + edit/regenerate 多入口），TTL 5s 让
// 连击复用同一份快照，又短到资产新增 / 删除后能自然失效不至于陈旧太久。
//
// 5s 是经验值：足以吃掉一次正常 "type + send" 的节奏，又不会因为资产改名等
// 后台变更让 mention 链路盯着旧名几分钟。资产变更不走主动失效（asset_svc 没
// 提供变更事件钩子），靠 TTL 自然过期即可。
const mentionCacheTTL = 5 * time.Second

// opsMentionResolver 扫描用户原文中的 @name，按 name 精确匹配资产；命中后把
// asset 渲染成 RenderMentionContext 段落 prepend 到 LLM body，并返回开 tab 目标。
//
// 资产列表用 TTL 缓存：一次 mention 命中触发 asset_svc.List 全表，缓存命中后
// 同一个 mentionCacheTTL 窗口内复用，避免 mention 多匹配或 hook 多入口连击导
// 致一条消息打 N 次 List。
type opsMentionResolver struct {
	mu        sync.Mutex
	index     map[string]*asset_entity.Asset // name → asset 快照
	expiresAt time.Time
}

func newOpsMentionResolver() *opsMentionResolver { return &opsMentionResolver{} }

// assetByName 取 name 对应的 asset，命中返回快照指针；未命中返回 nil。
// 缓存过期或首次调用时重新拉取全表 + 建索引。失败按 "无该 mention" 处理，
// 不向上抛错（mention 解析是 best-effort，不该阻断用户消息）。
func (m *opsMentionResolver) assetByName(ctx context.Context, name string) *asset_entity.Asset {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.index == nil || time.Now().After(m.expiresAt) {
		assets, err := asset_svc.Asset().List(ctx, "", 0)
		if err != nil {
			return nil
		}
		idx := make(map[string]*asset_entity.Asset, len(assets))
		for _, a := range assets {
			if a == nil || a.Name == "" {
				continue
			}
			// 同名资产只保留首个 —— mention 本身就有歧义，UI 应当限制重名。
			if _, ok := idx[a.Name]; !ok {
				idx[a.Name] = a
			}
		}
		m.index = idx
		m.expiresAt = time.Now().Add(mentionCacheTTL)
	}
	return m.index[name]
}

func (m *opsMentionResolver) Expand(ctx context.Context, raw string) (string, []string, error) {
	indices := mentionPattern.FindAllStringSubmatchIndex(raw, -1)
	if len(indices) == 0 {
		return raw, nil, nil
	}
	seen := map[string]bool{}
	var mentioned []ai.MentionedAsset
	var tabs []string
	// 按 match 顺序收 mention 元数据，start/end 是 raw 中对应 "@name" 字面的
	// JS UTF-16 偏移（前端 UserMessage.buildSegments 渲染 chip 用同一约定）。
	for _, idx := range indices {
		matchStart, matchEnd := idx[0], idx[1] // "@name" 在 raw 中的 byte 范围
		nameStart := idx[2]
		name := raw[nameStart:idx[3]]
		if seen[name] {
			continue
		}
		seen[name] = true
		a := m.assetByName(ctx, name)
		if a == nil {
			continue
		}
		// Host 存在 Asset.Config 的 JSON 里（按 type 不同字段），解析成本高且对
		// mention 上下文展示用处有限——名字+ID+类型已经足够让模型识别。留空。
		mentioned = append(mentioned, ai.MentionedAsset{
			AssetID:   a.ID,
			Name:      a.Name,
			Type:      a.Type,
			Host:      "",
			GroupPath: "",
			Start:     ai.ByteToUTF16(raw, matchStart),
			End:       ai.ByteToUTF16(raw, matchEnd),
		})
		tabs = append(tabs, fmt.Sprintf("asset:%d", a.ID))
	}
	if len(mentioned) == 0 {
		return raw, nil, nil
	}
	return ai.WrapMentions(raw, mentioned), tabs, nil
}

// === Tab Opener ===

// opsTabOpener 把 hook_mentions 决定要打开的 tab 目标转成 Wails 事件，让前端在
// 资产侧栏 / 终端 / 等响应。target 形如 "asset:42"。
type opsTabOpener struct{ app *App }

func newOpsTabOpener(a *App) *opsTabOpener { return &opsTabOpener{app: a} }

func (o *opsTabOpener) Open(ctx context.Context, target string) error {
	if o.app == nil || o.app.ctx == nil {
		return nil
	}
	parts := strings.SplitN(target, ":", 2)
	if len(parts) != 2 {
		return nil
	}
	wailsRuntime.EventsEmit(o.app.ctx, "ai:open-tab", map[string]any{
		"kind":   parts[0],
		"target": parts[1],
	})
	return nil
}
