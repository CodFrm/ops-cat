package migrations

import (
	"encoding/json"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func migration202603300001() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202603300001",
		Migrate: func(tx *gorm.DB) error {
			// 1. Add ssh_tunnel_id column
			if err := tx.Exec("ALTER TABLE assets ADD COLUMN ssh_tunnel_id INTEGER NOT NULL DEFAULT 0").Error; err != nil {
				return err
			}

			type assetRow struct {
				ID     int64  `gorm:"column:id"`
				Config string `gorm:"column:config"`
			}

			// 2. Migrate SSH assets: extract jump_host_id → ssh_tunnel_id
			var sshRows []assetRow
			if err := tx.Raw(
				"SELECT id, config FROM assets WHERE type = 'ssh' AND status = 1 AND config != ''",
			).Scan(&sshRows).Error; err != nil {
				return err
			}
			for _, r := range sshRows {
				var cfg struct {
					JumpHostID int64 `json:"jump_host_id"`
				}
				if err := json.Unmarshal([]byte(r.Config), &cfg); err != nil {
					continue
				}
				if cfg.JumpHostID > 0 {
					if err := tx.Exec(
						"UPDATE assets SET ssh_tunnel_id = ? WHERE id = ?",
						cfg.JumpHostID, r.ID,
					).Error; err != nil {
						return err
					}
				}
			}

			// 3. Migrate Database assets: extract ssh_asset_id → ssh_tunnel_id
			var dbRows []assetRow
			if err := tx.Raw(
				"SELECT id, config FROM assets WHERE type = 'database' AND status = 1 AND config != ''",
			).Scan(&dbRows).Error; err != nil {
				return err
			}
			for _, r := range dbRows {
				var cfg struct {
					SSHAssetID int64 `json:"ssh_asset_id"`
				}
				if err := json.Unmarshal([]byte(r.Config), &cfg); err != nil {
					continue
				}
				if cfg.SSHAssetID > 0 {
					if err := tx.Exec(
						"UPDATE assets SET ssh_tunnel_id = ? WHERE id = ?",
						cfg.SSHAssetID, r.ID,
					).Error; err != nil {
						return err
					}
				}
			}

			// 4. Migrate Redis assets: extract ssh_asset_id → ssh_tunnel_id
			var redisRows []assetRow
			if err := tx.Raw(
				"SELECT id, config FROM assets WHERE type = 'redis' AND status = 1 AND config != ''",
			).Scan(&redisRows).Error; err != nil {
				return err
			}
			for _, r := range redisRows {
				var cfg struct {
					SSHAssetID int64 `json:"ssh_asset_id"`
				}
				if err := json.Unmarshal([]byte(r.Config), &cfg); err != nil {
					continue
				}
				if cfg.SSHAssetID > 0 {
					if err := tx.Exec(
						"UPDATE assets SET ssh_tunnel_id = ? WHERE id = ?",
						cfg.SSHAssetID, r.ID,
					).Error; err != nil {
						return err
					}
				}
			}

			return nil
		},
	}
}
