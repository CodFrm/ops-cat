package migrations

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// builtinIDMap 旧 int64 ID → 新字符串 ID 映射
var builtinIDMap = map[int64]string{
	-1: "builtin.linux_readonly",
	-2: "builtin.k8s_readonly",
	-3: "builtin.docker_readonly",
	-4: "builtin.dangerous_deny",
	-5: "builtin.sql_readonly",
	-6: "builtin.sql_dangerous_deny",
	-7: "builtin.redis_readonly",
	-8: "builtin.redis_dangerous_deny",
}

func migration202603290001() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202603290001",
		Migrate: func(tx *gorm.DB) error {
			// 迁移 assets.command_policy
			if err := migratePolicyColumn(tx, "assets", "command_policy"); err != nil {
				return fmt.Errorf("migrate assets.command_policy: %w", err)
			}
			// 迁移 groups 的三个策略列
			for _, col := range []string{"command_policy", "query_policy", "redis_policy"} {
				if err := migratePolicyColumn(tx, "groups", col); err != nil {
					return fmt.Errorf("migrate groups.%s: %w", col, err)
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}

// migratePolicyColumn 将指定表列中的 groups 字段从 []int64 转为 []string
func migratePolicyColumn(tx *gorm.DB, table, column string) error {
	type row struct {
		ID     int64  `gorm:"column:id"`
		Policy string `gorm:"column:policy_val"`
	}

	var rows []row
	query := fmt.Sprintf("SELECT id, %s AS policy_val FROM %s WHERE %s != '' AND %s IS NOT NULL", column, table, column, column)
	if err := tx.Raw(query).Scan(&rows).Error; err != nil {
		return err
	}

	for _, r := range rows {
		converted, changed := convertPolicyGroupIDs(r.Policy)
		if !changed {
			continue
		}
		update := fmt.Sprintf("UPDATE %s SET %s = ? WHERE id = ?", table, column)
		if err := tx.Exec(update, converted, r.ID).Error; err != nil {
			return err
		}
	}
	return nil
}

// convertPolicyGroupIDs 将策略 JSON 中的 groups 从 []number 转为 []string
func convertPolicyGroupIDs(policyJSON string) (string, bool) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(policyJSON), &raw); err != nil {
		return policyJSON, false
	}
	groupsRaw, ok := raw["groups"]
	if !ok {
		return policyJSON, false
	}

	// 尝试解析为 []interface{}（可能是 int 或 string）
	var items []interface{}
	if err := json.Unmarshal(groupsRaw, &items); err != nil {
		return policyJSON, false
	}

	newGroups := make([]string, 0, len(items))
	changed := false
	for _, item := range items {
		switch v := item.(type) {
		case float64: // JSON numbers are float64
			intID := int64(v)
			if str, ok := builtinIDMap[intID]; ok {
				newGroups = append(newGroups, str)
				changed = true
			} else {
				newGroups = append(newGroups, strconv.FormatInt(intID, 10))
				changed = true
			}
		case string:
			// 已经是字符串，保持不变
			newGroups = append(newGroups, v)
		default:
			return policyJSON, false
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
