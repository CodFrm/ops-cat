package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration202605120001 DROP conversation_messages.mentions 列。
// mentions 的元数据已在应用层迁移为 content 内联 <mention> XML 标签，
// 此处仅负责清理数据库列。
//
// SQLite 3.35+ 原生支持 ALTER TABLE DROP COLUMN，本项目内置 modernc.org/sqlite v1.23.1（SQLite 3.41+）
// 不再需要"重建表"workaround。
func migration202605120001() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202605120001",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`ALTER TABLE conversation_messages DROP COLUMN mentions`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}
