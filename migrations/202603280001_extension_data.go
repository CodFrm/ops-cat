package migrations

import (
	"github.com/opskat/opskat/internal/model/entity/extension_data_entity"
	"github.com/opskat/opskat/internal/status"

	"github.com/cago-frame/cago/pkg/logger"
	"github.com/go-gormigrate/gormigrate/v2"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func migration202603280001() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202603280001",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.AutoMigrate(&extension_data_entity.ExtensionData{}); err != nil {
				logger.Default().Warn("migration 202603280001: 创建 extension_data 表失败", zap.Error(err))
				status.Add(status.Entry{
					Level:   status.LevelWarn,
					Source:  "migration",
					Message: "创建 extension_data 表失败",
					Detail:  err.Error(),
				})
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable("extension_data")
		},
	}
}
