package migrations

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/cago-frame/cago/pkg/logger"
	"github.com/go-gormigrate/gormigrate/v2"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// oldIDToNewID 旧负整数 ID → 新字符串 ID 映射
var oldIDToNewID = map[int64]string{
	-1: "builtin:linux-readonly",
	-2: "builtin:k8s-readonly",
	-3: "builtin:docker-readonly",
	-4: "builtin:dangerous-deny",
	-5: "builtin:sql-readonly",
	-6: "builtin:sql-dangerous-deny",
	-7: "builtin:redis-readonly",
	-8: "builtin:redis-dangerous-deny",
}

func migration202603290001() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202603290001",
		Migrate: func(tx *gorm.DB) error {
			// 迁移 assets 表的 command_policy 字段
			if err := migratePolicyColumn(tx, "assets", "command_policy"); err != nil {
				return fmt.Errorf("migrate assets.command_policy: %w", err)
			}

			// 迁移 groups 表的 command_policy、query_policy、redis_policy 字段
			for _, col := range []string{"command_policy", "query_policy", "redis_policy"} {
				if err := migratePolicyColumn(tx, "groups", col); err != nil {
					return fmt.Errorf("migrate groups.%s: %w", col, err)
				}
			}

			return nil
		},
	}
}

// migratePolicyColumn 扫描表中某列的 JSON，将 groups 数组中的 int64 ID 替换为 string ID
func migratePolicyColumn(tx *gorm.DB, table, column string) error {
	type row struct {
		ID     int64  `gorm:"column:id"`
		Policy string `gorm:"column:policy_val"`
	}

	var rows []row
	query := fmt.Sprintf("SELECT id, %s AS policy_val FROM %s WHERE %s IS NOT NULL AND %s != ''", column, table, column, column)
	if err := tx.Raw(query).Scan(&rows).Error; err != nil {
		return err
	}

	for _, r := range rows {
		newPolicy, changed := convertGroupIDs(r.Policy)
		if !changed {
			continue
		}
		updateSQL := fmt.Sprintf("UPDATE %s SET %s = ? WHERE id = ?", table, column)
		if err := tx.Exec(updateSQL, newPolicy, r.ID).Error; err != nil {
			logger.Default().Warn("migration: update policy column",
				zap.String("table", table), zap.String("column", column),
				zap.Int64("id", r.ID), zap.Error(err))
			return err
		}
	}
	return nil
}

// convertGroupIDs 将 JSON 中 groups 数组的 int64 元素转为 string
func convertGroupIDs(policyJSON string) (string, bool) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(policyJSON), &raw); err != nil {
		return policyJSON, false
	}
	groupsRaw, ok := raw["groups"]
	if !ok {
		return policyJSON, false
	}

	// 尝试解析为 []interface{}，因为旧格式是数字，新格式可能已是字符串
	var mixedGroups []json.RawMessage
	if err := json.Unmarshal(groupsRaw, &mixedGroups); err != nil {
		return policyJSON, false
	}

	newGroups := make([]string, 0, len(mixedGroups))
	changed := false
	for _, item := range mixedGroups {
		s := string(item)
		// 尝试作为数字解析
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			if newID, ok := oldIDToNewID[n]; ok {
				newGroups = append(newGroups, newID)
				changed = true
			} else {
				// 正数 ID（用户自定义组）转为字符串
				newGroups = append(newGroups, strconv.FormatInt(n, 10))
				changed = true
			}
		} else {
			// 已经是字符串（去除引号）
			var str string
			if err := json.Unmarshal(item, &str); err == nil {
				newGroups = append(newGroups, str)
			}
		}
	}

	if !changed {
		return policyJSON, false
	}

	newGroupsRaw, err := json.Marshal(newGroups)
	if err != nil {
		return policyJSON, false
	}
	raw["groups"] = newGroupsRaw
	result, err := json.Marshal(raw)
	if err != nil {
		return policyJSON, false
	}
	return string(result), true
}
