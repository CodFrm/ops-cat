package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// RunMigrations 执行数据库迁移
func RunMigrations(db *gorm.DB) error {
	m := gormigrate.New(db, gormigrate.DefaultOptions, []*gormigrate.Migration{
		migration202603220001(),
		migration202603260001(),
		migration202603270001(),
		migration202603290001(),
		migration202603300001(),
		migration202603300002(),
		migration202603310001(),
		migration202604050001(),
		migration202604140001(),
		migration202604160001(),
		migration202604170001(),
		migration202604220001(),
		migration202604230001(),
		migration202604270001(),
		migration202605010001(),
	})
	return m.Migrate()
}
