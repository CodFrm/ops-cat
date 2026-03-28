package app

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/opskat/opskat/internal/ai"
	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/model/entity/group_entity"
	"github.com/opskat/opskat/internal/model/entity/policy_group_entity"
	"github.com/opskat/opskat/internal/service/asset_svc"
	"github.com/opskat/opskat/internal/service/group_svc"
	"github.com/opskat/opskat/internal/service/policy_group_svc"
)

// --- 策略测试 ---

// PolicyTestRequest 策略测试请求
type PolicyTestRequest struct {
	PolicyType string `json:"policyType"` // "ssh" | "database" | "redis"
	PolicyJSON string `json:"policyJSON"` // JSON 编码的策略结构体（当前编辑状态）
	Command    string `json:"command"`    // 待测试的命令/SQL/Redis命令
	AssetID    int64  `json:"assetID"`    // 资产ID（用于解析资产组链）
	GroupID    int64  `json:"groupID"`    // 资产组ID（用于解析父组链）
}

// PolicyTestResult 策略测试结果
type PolicyTestResult struct {
	Decision       string `json:"decision"`       // "allow" | "deny" | "need_confirm"
	MatchedPattern string `json:"matchedPattern"` // 匹配到的规则
	MatchedSource  string `json:"matchedSource"`  // 匹配来源: "" 当前策略, "default" 默认规则, 或组名
	Message        string `json:"message"`        // 可读说明
}

// TestPolicyRule 测试命令/SQL/Redis 命令是否匹配当前策略（含资产组继承）
func (a *App) TestPolicyRule(req PolicyTestRequest) (*PolicyTestResult, error) {
	command := strings.TrimSpace(req.Command)
	if command == "" {
		return nil, fmt.Errorf("command is empty")
	}

	// 解析当前编辑中的策略 JSON
	input := ai.PolicyTestInput{
		PolicyType: req.PolicyType,
		AssetID:    req.AssetID,
		GroupID:    req.GroupID,
	}
	if req.PolicyJSON != "" {
		switch req.PolicyType {
		case "ssh":
			var p asset_entity.CommandPolicy
			if err := json.Unmarshal([]byte(req.PolicyJSON), &p); err != nil {
				return nil, fmt.Errorf("invalid SSH policy JSON: %w", err)
			}
			input.CurrentSSH = &p
		case "database":
			var p asset_entity.QueryPolicy
			if err := json.Unmarshal([]byte(req.PolicyJSON), &p); err != nil {
				return nil, fmt.Errorf("invalid query policy JSON: %w", err)
			}
			input.CurrentQuery = &p
		case "redis":
			var p asset_entity.RedisPolicy
			if err := json.Unmarshal([]byte(req.PolicyJSON), &p); err != nil {
				return nil, fmt.Errorf("invalid Redis policy JSON: %w", err)
			}
			input.CurrentRedis = &p
		default:
			return nil, fmt.Errorf("unsupported policy type: %s", req.PolicyType)
		}
	}

	result := ai.TestPolicy(a.langCtx(), input, command)

	decision := "need_confirm"
	switch result.Decision {
	case ai.Allow:
		decision = "allow"
	case ai.Deny:
		decision = "deny"
	}

	return &PolicyTestResult{
		Decision:       decision,
		MatchedPattern: result.MatchedPattern,
		MatchedSource:  result.MatchedSource,
		Message:        result.Message,
	}, nil
}

// GetDefaultPolicy 获取指定资产类型的默认策略 JSON
func (a *App) GetDefaultPolicy(assetType string) (string, error) {
	switch assetType {
	case asset_entity.AssetTypeSSH:
		data, err := json.Marshal(asset_entity.DefaultCommandPolicy())
		if err != nil {
			return "", err
		}
		return string(data), nil
	case asset_entity.AssetTypeDatabase:
		data, err := json.Marshal(asset_entity.DefaultQueryPolicy())
		if err != nil {
			return "", err
		}
		return string(data), nil
	case asset_entity.AssetTypeRedis:
		data, err := json.Marshal(asset_entity.DefaultRedisPolicy())
		if err != nil {
			return "", err
		}
		return string(data), nil
	default:
		// 扩展类型：查询已注册的扩展默认权限组
		defaultPolicy := getExtensionDefaultPolicy(assetType)
		if defaultPolicy == "" {
			return "{}", nil
		}
		return defaultPolicy, nil
	}
}

// --- 权限组管理 ---

// ListPolicyGroups 列出权限组（内置 + 自定义）
func (a *App) ListPolicyGroups(policyType string) ([]*policy_group_entity.PolicyGroupItem, error) {
	return policy_group_svc.PolicyGroup().List(a.langCtx(), policyType)
}

// CreatePolicyGroup 创建自定义权限组
func (a *App) CreatePolicyGroup(pg policy_group_entity.PolicyGroup) (*policy_group_entity.PolicyGroup, error) {
	if err := policy_group_svc.PolicyGroup().Create(a.langCtx(), &pg); err != nil {
		return nil, err
	}
	return &pg, nil
}

// UpdatePolicyGroup 更新自定义权限组
func (a *App) UpdatePolicyGroup(pg policy_group_entity.PolicyGroup) error {
	return policy_group_svc.PolicyGroup().Update(a.langCtx(), &pg)
}

// DeletePolicyGroup 删除自定义权限组
func (a *App) DeletePolicyGroup(id string) error {
	return policy_group_svc.PolicyGroup().Delete(a.langCtx(), id)
}

// CopyPolicyGroup 复制权限组（内置或自定义）
func (a *App) CopyPolicyGroup(id string, name string) (*policy_group_entity.PolicyGroup, error) {
	return policy_group_svc.PolicyGroup().Copy(a.langCtx(), id, name)
}

// --- 资产操作 ---

// GetAsset 获取资产详情
func (a *App) GetAsset(id int64) (*asset_entity.Asset, error) {
	return asset_svc.Asset().Get(a.langCtx(), id)
}

// ListAssets 列出资产
func (a *App) ListAssets(assetType string, groupID int64) ([]*asset_entity.Asset, error) {
	return asset_svc.Asset().List(a.langCtx(), assetType, groupID)
}

// CreateAsset 创建资产
func (a *App) CreateAsset(asset *asset_entity.Asset) error {
	return asset_svc.Asset().Create(a.langCtx(), asset)
}

// UpdateAsset 更新资产
func (a *App) UpdateAsset(asset *asset_entity.Asset) error {
	return asset_svc.Asset().Update(a.langCtx(), asset)
}

// DeleteAsset 删除资产
func (a *App) DeleteAsset(id int64) error {
	return asset_svc.Asset().Delete(a.langCtx(), id)
}

// MoveAsset 移动资产排序（up/down/top）
func (a *App) MoveAsset(id int64, direction string) error {
	return asset_svc.Asset().Move(a.langCtx(), id, direction)
}

// MoveGroup 移动分组排序（up/down/top）
func (a *App) MoveGroup(id int64, direction string) error {
	return group_svc.Group().Move(a.langCtx(), id, direction)
}

// --- 分组操作 ---

// ListGroups 列出所有分组
func (a *App) ListGroups() ([]*group_entity.Group, error) {
	return group_svc.Group().List(a.langCtx())
}

// GetGroup 获取单个分组详情
func (a *App) GetGroup(id int64) (*group_entity.Group, error) {
	return group_svc.Group().Get(a.langCtx(), id)
}

// CreateGroup 创建分组
func (a *App) CreateGroup(group *group_entity.Group) error {
	return group_svc.Group().Create(a.langCtx(), group)
}

// UpdateGroup 更新分组
func (a *App) UpdateGroup(group *group_entity.Group) error {
	return group_svc.Group().Update(a.langCtx(), group)
}

// DeleteGroup 删除分组
// deleteAssets: true 删除分组下的资产，false 移动到未分组
func (a *App) DeleteGroup(id int64, deleteAssets bool) error {
	return group_svc.Group().Delete(a.langCtx(), id, deleteAssets)
}
