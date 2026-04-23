package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"

	"github.com/opskat/opskat/internal/assettype"
	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/model/entity/group_entity"
	"github.com/opskat/opskat/internal/repository/group_repo"
	"github.com/opskat/opskat/internal/service/asset_svc"
)

// --- 工具 handler 实现 ---

// safeAssetView 返回不含敏感信息的资产视图
type safeAssetView struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	GroupID     int64  `json:"group_id"`
	Description string `json:"description,omitempty"`
	SortOrder   int    `json:"sort_order"`
	Createtime  int64  `json:"createtime"`
	Updatetime  int64  `json:"updatetime"`
	// 连接信息（不含密码/密钥）
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	Username string `json:"username,omitempty"`
	AuthType string `json:"auth_type,omitempty"`
	// Database 专属
	Driver   string `json:"driver,omitempty"`
	Database string `json:"database,omitempty"`
	ReadOnly bool   `json:"read_only,omitempty"`
	// Redis 专属
	RedisDB int `json:"redis_db,omitempty"`
}

// safeGroupListView 列表视图（不含描述）
type safeGroupListView struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	ParentID  int64  `json:"parent_id"`
	Icon      string `json:"icon,omitempty"`
	SortOrder int    `json:"sort_order"`
}

// safeGroupDetailView 详情视图（含描述）
type safeGroupDetailView struct {
	safeGroupListView
	Description string `json:"description,omitempty"`
}

func toSafeView(a *asset_entity.Asset) safeAssetView {
	v := safeAssetView{
		ID:          a.ID,
		Name:        a.Name,
		Type:        a.Type,
		GroupID:     a.GroupID,
		Description: a.Description,
		SortOrder:   a.SortOrder,
		Createtime:  a.Createtime,
		Updatetime:  a.Updatetime,
	}
	if h, ok := assettype.Get(a.Type); ok {
		if fields := h.SafeView(a); fields != nil {
			if val, ok := fields["host"].(string); ok {
				v.Host = val
			}
			if val, ok := fields["port"].(int); ok {
				v.Port = val
			}
			if val, ok := fields["username"].(string); ok {
				v.Username = val
			}
			if val, ok := fields["driver"].(string); ok {
				v.Driver = val
			}
			if val, ok := fields["database"].(string); ok {
				v.Database = val
			}
			if val, ok := fields["read_only"].(bool); ok {
				v.ReadOnly = val
			}
			if val, ok := fields["redis_db"].(int); ok {
				v.RedisDB = val
			}
			if val, ok := fields["auth_type"].(string); ok {
				v.AuthType = val
			}
		}
	}
	return v
}

func handleListAssets(ctx context.Context, args map[string]any) (string, error) {
	assetType := argString(args, "asset_type")
	groupID := argInt64(args, "group_id")
	assets, err := asset_svc.Asset().List(ctx, assetType, groupID)
	if err != nil {
		return "", err
	}
	views := make([]safeAssetView, len(assets))
	for i, a := range assets {
		views[i] = toSafeView(a)
		views[i].Description = "" // list 不返回描述，通过 get_asset 查看
	}
	data, err := json.Marshal(views)
	if err != nil {
		logger.Default().Error("marshal asset list", zap.Error(err))
		return "", fmt.Errorf("failed to marshal asset list: %w", err)
	}
	return string(data), nil
}

func handleGetAsset(ctx context.Context, args map[string]any) (string, error) {
	id := argInt64(args, "id")
	if id == 0 {
		return "", fmt.Errorf("missing required parameter: id")
	}
	asset, err := asset_svc.Asset().Get(ctx, id)
	if err != nil {
		return "", fmt.Errorf("asset not found: %w", err)
	}
	data, err := json.Marshal(toSafeView(asset))
	if err != nil {
		logger.Default().Error("marshal asset detail", zap.Error(err))
		return "", fmt.Errorf("failed to marshal asset detail: %w", err)
	}
	return string(data), nil
}

func handleAddAsset(ctx context.Context, args map[string]any) (string, error) {
	name := argString(args, "name")
	host := argString(args, "host")
	port := argInt(args, "port")
	username := argString(args, "username")
	if name == "" || host == "" || port == 0 || username == "" {
		return "", fmt.Errorf("missing required parameters: name, host, port, username")
	}

	assetType := argString(args, "type")
	if assetType == "" {
		assetType = asset_entity.AssetTypeSSH
	}
	groupID := argInt64(args, "group_id")
	description := argString(args, "description")

	icon := argString(args, "icon")

	asset := &asset_entity.Asset{
		Name:        name,
		Type:        assetType,
		Icon:        icon,
		GroupID:     groupID,
		Description: description,
	}

	h, ok := assettype.Get(assetType)
	if !ok {
		return "", fmt.Errorf("unsupported asset type: %s", assetType)
	}
	if err := h.ApplyCreateArgs(ctx, asset, args); err != nil {
		return "", err
	}

	if err := asset_svc.Asset().Create(ctx, asset); err != nil {
		return "", fmt.Errorf("failed to create asset: %w", err)
	}
	notifyDataChanged("asset")
	return fmt.Sprintf(`{"id":%d,"message":"asset created successfully"}`, asset.ID), nil
}

func handleUpdateAsset(ctx context.Context, args map[string]any) (string, error) {
	id := argInt64(args, "id")
	if id == 0 {
		return "", fmt.Errorf("missing required parameter: id")
	}

	asset, err := asset_svc.Asset().Get(ctx, id)
	if err != nil {
		return "", fmt.Errorf("asset not found: %w", err)
	}

	if name := argString(args, "name"); name != "" {
		asset.Name = name
	}
	if _, ok := args["description"]; ok {
		asset.Description = argString(args, "description")
	}
	if _, ok := args["group_id"]; ok {
		asset.GroupID = argInt64(args, "group_id")
	}
	if icon := argString(args, "icon"); icon != "" {
		asset.Icon = icon
	}

	if h, ok := assettype.Get(asset.Type); ok {
		if err := h.ApplyUpdateArgs(ctx, asset, args); err != nil {
			return "", fmt.Errorf("apply update args failed: %w", err)
		}
	}

	if err := asset_svc.Asset().Update(ctx, asset); err != nil {
		return "", fmt.Errorf("failed to update asset: %w", err)
	}
	notifyDataChanged("asset")
	return `{"message":"asset updated successfully"}`, nil
}

func handleListGroups(ctx context.Context, _ map[string]any) (string, error) {
	groups, err := group_repo.Group().List(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to list groups: %w", err)
	}
	views := make([]safeGroupListView, len(groups))
	for i, g := range groups {
		views[i] = safeGroupListView{
			ID:        g.ID,
			Name:      g.Name,
			ParentID:  g.ParentID,
			Icon:      g.Icon,
			SortOrder: g.SortOrder,
		}
	}
	data, err := json.Marshal(views)
	if err != nil {
		logger.Default().Error("marshal group list", zap.Error(err))
		return "", fmt.Errorf("failed to marshal group list: %w", err)
	}
	return string(data), nil
}

func handleGetGroup(ctx context.Context, args map[string]any) (string, error) {
	id := argInt64(args, "id")
	if id == 0 {
		return "", fmt.Errorf("missing required parameter: id")
	}
	group, err := group_repo.Group().Find(ctx, id)
	if err != nil {
		return "", fmt.Errorf("group not found: %w", err)
	}
	view := safeGroupDetailView{
		safeGroupListView: safeGroupListView{
			ID:        group.ID,
			Name:      group.Name,
			ParentID:  group.ParentID,
			Icon:      group.Icon,
			SortOrder: group.SortOrder,
		},
		Description: group.Description,
	}
	data, err := json.Marshal(view)
	if err != nil {
		logger.Default().Error("marshal group detail", zap.Error(err))
		return "", fmt.Errorf("failed to marshal group detail: %w", err)
	}
	return string(data), nil
}

func handleAddGroup(ctx context.Context, args map[string]any) (string, error) {
	name := argString(args, "name")
	if name == "" {
		return "", fmt.Errorf("missing required parameter: name")
	}
	now := time.Now().Unix()
	group := &group_entity.Group{
		Name:        name,
		ParentID:    argInt64(args, "parent_id"),
		Icon:        argString(args, "icon"),
		Description: argString(args, "description"),
		SortOrder:   argInt(args, "sort_order"),
		Createtime:  now,
		Updatetime:  now,
	}
	if err := group_repo.Group().Create(ctx, group); err != nil {
		return "", fmt.Errorf("failed to create group: %w", err)
	}
	notifyDataChanged("group")
	return fmt.Sprintf(`{"id":%d,"message":"group created successfully"}`, group.ID), nil
}

func handleUpdateGroup(ctx context.Context, args map[string]any) (string, error) {
	id := argInt64(args, "id")
	if id == 0 {
		return "", fmt.Errorf("missing required parameter: id")
	}
	group, err := group_repo.Group().Find(ctx, id)
	if err != nil {
		return "", fmt.Errorf("group not found: %w", err)
	}
	if name := argString(args, "name"); name != "" {
		group.Name = name
	}
	if _, ok := args["parent_id"]; ok {
		group.ParentID = argInt64(args, "parent_id")
	}
	if _, ok := args["icon"]; ok {
		group.Icon = argString(args, "icon")
	}
	if _, ok := args["description"]; ok {
		group.Description = argString(args, "description")
	}
	if _, ok := args["sort_order"]; ok {
		group.SortOrder = argInt(args, "sort_order")
	}
	group.Updatetime = time.Now().Unix()
	if err := group_repo.Group().Update(ctx, group); err != nil {
		return "", fmt.Errorf("failed to update group: %w", err)
	}
	notifyDataChanged("group")
	return `{"message":"group updated successfully"}`, nil
}
