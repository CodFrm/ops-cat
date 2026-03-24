package migrations

import (
	"ops-cat/internal/model/entity/asset_entity"
	"ops-cat/internal/model/entity/audit_entity"
	"ops-cat/internal/model/entity/conversation_entity"
	"ops-cat/internal/model/entity/credential_entity"
	"ops-cat/internal/model/entity/forward_entity"
	"ops-cat/internal/model/entity/group_entity"
	"ops-cat/internal/model/entity/plan_entity"
	"ops-cat/internal/model/entity/ssh_key_entity"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration202603220001 初始化所有表
func migration202603220001() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202603220001",
		Migrate: func(tx *gorm.DB) error {
			return tx.AutoMigrate(
				&asset_entity.Asset{},
				&group_entity.Group{},
				&ssh_key_entity.SSHKey{},
				&conversation_entity.Conversation{},
				&conversation_entity.Message{},
				&audit_entity.AuditLog{},
				&plan_entity.PlanSession{},
				&plan_entity.PlanItem{},
				&forward_entity.ForwardConfig{},
				&forward_entity.ForwardRule{},
				&credential_entity.Credential{},
			)
		},
		Rollback: func(tx *gorm.DB) error {
			tables := []string{
				"credentials",
				"forward_rules",
				"forward_configs",
				"plan_items",
				"plan_sessions",
				"audit_logs",
				"conversation_messages",
				"conversations",
				"ssh_keys",
				"groups",
				"assets",
			}
			for _, table := range tables {
				if err := tx.Migrator().DropTable(table); err != nil {
					return err
				}
			}
			return nil
		},
	}
}
