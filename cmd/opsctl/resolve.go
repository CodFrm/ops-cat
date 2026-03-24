package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/repository/asset_repo"
	"github.com/opskat/opskat/internal/repository/group_repo"

	"strconv"
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
	groupMap, _ := buildGroupPathMap(ctx)
	var sb strings.Builder
	fmt.Fprintf(&sb, "ambiguous name %q, matches:\n", identifier) //nolint:gosec // identifier is from CLI args
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

	// Build path map for matching
	pathMap, err := buildGroupPathMap(ctx)
	if err != nil {
		return 0, "", err
	}

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

	byID := make(map[int64]string)
	idToParent := make(map[int64]int64)
	idToName := make(map[int64]string)
	for _, g := range groups {
		idToParent[g.ID] = g.ParentID
		idToName[g.ID] = g.Name
	}

	// Build paths by walking up parent chain
	var buildPath func(id int64, depth int) string
	buildPath = func(id int64, depth int) string {
		if id == 0 || depth > 5 {
			return ""
		}
		if cached, ok := byID[id]; ok {
			return cached
		}
		name := idToName[id]
		parent := idToParent[id]
		parentPath := buildPath(parent, depth+1)
		var path string
		if parentPath != "" {
			path = parentPath + "/" + name
		} else {
			path = name
		}
		byID[id] = path
		return path
	}

	for _, g := range groups {
		buildPath(g.ID, 0)
	}

	return byID, nil
}
