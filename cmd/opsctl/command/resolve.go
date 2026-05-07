package command

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/model/entity/group_entity"
	"github.com/opskat/opskat/internal/repository/asset_repo"
	"github.com/opskat/opskat/internal/repository/group_repo"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
)

// resolveAsset resolves an asset identifier (numeric ID or name).
// Supports "group/name" for disambiguation when names are not unique.
func resolveAsset(ctx context.Context, identifier string) (*asset_entity.Asset, error) {
	// Try numeric ID first
	if id, err := strconv.ParseInt(identifier, 10, 64); err == nil {
		asset, err := asset_repo.Asset().Find(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("asset not found: ID %d", id)
		}
		return asset, nil
	}

	// Name-based lookup
	groupPart := ""
	namePart := identifier
	if idx := strings.Index(identifier, "/"); idx >= 0 {
		groupPart = identifier[:idx]
		namePart = identifier[idx+1:]
	}

	// List all assets and filter by name
	allAssets, err := asset_repo.Asset().List(ctx, asset_repo.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list assets: %w", err)
	}

	var candidates []*asset_entity.Asset
	for _, a := range allAssets {
		if a.Name == namePart {
			candidates = append(candidates, a)
		}
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no asset found matching %q", identifier)
	}

	// Filter by group if specified
	if groupPart != "" {
		groupMap, err := buildGroupPathMap(ctx)
		if err != nil {
			return nil, err
		}
		var filtered []*asset_entity.Asset
		for _, a := range candidates {
			path := groupMap[a.GroupID]
			if path == groupPart || strings.HasSuffix(path, "/"+groupPart) {
				filtered = append(filtered, a)
			}
		}
		candidates = filtered
		if len(candidates) == 0 {
			return nil, fmt.Errorf("no asset found matching %q", identifier)
		}
	}

	if len(candidates) == 1 {
		return candidates[0], nil
	}

	// Ambiguous - list candidates
	groupMap, err := buildGroupPathMap(ctx)
	if err != nil {
		logger.Ctx(ctx).Warn("build group path map", zap.Error(err))
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "ambiguous name %q, matches:\n", identifier)
	for _, a := range candidates {
		group := groupMap[a.GroupID]
		if group == "" {
			group = "(ungrouped)"
		}
		fmt.Fprintf(&sb, "  [ID=%d] %s (group: %s)\n", a.ID, a.Name, group)
	}
	sb.WriteString("Use ID or group/name to disambiguate.")
	return nil, fmt.Errorf("%s", sb.String())
}

// resolveAssetID is a convenience wrapper returning just the ID.
func resolveAssetID(ctx context.Context, identifier string) (int64, error) {
	asset, err := resolveAsset(ctx, identifier)
	if err != nil {
		return 0, err
	}
	return asset.ID, nil
}

// resolveGroup resolves a group identifier (numeric ID or name/path).
func resolveGroup(ctx context.Context, identifier string) (int64, string, error) {
	// Try numeric ID first
	if id, err := strconv.ParseInt(identifier, 10, 64); err == nil {
		g, err := group_repo.Group().Find(ctx, id)
		if err != nil {
			return 0, "", fmt.Errorf("group not found: ID %d", id)
		}
		return g.ID, g.Name, nil
	}

	// Name-based lookup
	groups, err := group_repo.Group().List(ctx)
	if err != nil {
		return 0, "", fmt.Errorf("failed to list groups: %w", err)
	}

	// Build path map from the already-fetched groups (avoid a second List).
	pathMap := buildGroupPathMapFromGroups(groups)

	var candidates []struct {
		id   int64
		name string
		path string
	}
	for _, g := range groups {
		path := pathMap[g.ID]
		if g.Name == identifier || path == identifier || strings.HasSuffix(path, "/"+identifier) {
			candidates = append(candidates, struct {
				id   int64
				name string
				path string
			}{g.ID, g.Name, path})
		}
	}

	if len(candidates) == 0 {
		return 0, "", fmt.Errorf("no group found matching %q", identifier)
	}
	if len(candidates) == 1 {
		return candidates[0].id, candidates[0].name, nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "ambiguous group %q, matches:\n", identifier)
	for _, c := range candidates {
		fmt.Fprintf(&sb, "  [ID=%d] %s\n", c.id, c.path)
	}
	sb.WriteString("Use ID or full path to disambiguate.")
	return 0, "", fmt.Errorf("%s", sb.String())
}

// buildGroupPathMap builds a map of groupID -> full path string (e.g. "parent/child").
func buildGroupPathMap(ctx context.Context) (map[int64]string, error) {
	groups, err := group_repo.Group().List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list groups: %w", err)
	}

	return buildGroupPathMapFromGroups(groups), nil
}

func buildGroupPathMapFromGroups(groups []*group_entity.Group) map[int64]string {
	byID := make(map[int64]*group_entity.Group, len(groups))
	for _, g := range groups {
		byID[g.ID] = g
	}

	paths := make(map[int64]string, len(groups))
	for _, g := range groups {
		paths[g.ID] = buildGroupPathFromLookup(byID, g.ID)
	}
	return paths
}

func buildGroupPathFromLookup(groups map[int64]*group_entity.Group, groupID int64) string {
	if groupID == 0 {
		return ""
	}

	var names []string
	seen := make(map[int64]struct{})
	for id := groupID; id > 0; {
		if _, ok := seen[id]; ok {
			break
		}
		seen[id] = struct{}{}

		group, ok := groups[id]
		if !ok {
			break
		}
		names = append(names, group.Name)
		id = group.ParentID
	}

	for i, j := 0, len(names)-1; i < j; i, j = i+1, j-1 {
		names[i], names[j] = names[j], names[i]
	}
	return strings.Join(names, "/")
}
