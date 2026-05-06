package migrations

import (
	"encoding/json"
	"strings"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"

	"github.com/opskat/opskat/internal/service/credential_svc"
)

// migration202604280001 把已有 K8S 资产中的明文 kubeconfig 加密入库。
// 启发式：YAML kubeconfig 必含 "apiVersion"。已加密的 base64 密文不会出现该字符串。
func migration202604280001() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202604280001",
		Migrate: func(tx *gorm.DB) error {
			svc := credential_svc.Default()
			if svc == nil {
				return nil
			}

			type assetRow struct {
				ID     int64
				Type   string
				Config string
			}
			var rows []assetRow
			if err := tx.Table("assets").
				Select("id, type, config").
				Where("type = ?", "k8s").
				Find(&rows).Error; err != nil {
				return err
			}

			for _, row := range rows {
				if row.Config == "" {
					continue
				}
				var raw map[string]any
				if err := json.Unmarshal([]byte(row.Config), &raw); err != nil {
					continue
				}
				kc, ok := raw["kubeconfig"].(string)
				if !ok || kc == "" {
					continue
				}
				if !looksLikePlaintextKubeconfig(kc) {
					continue
				}
				encrypted, err := svc.Encrypt(kc)
				if err != nil {
					return err
				}
				raw["kubeconfig"] = encrypted
				newCfg, err := json.Marshal(raw)
				if err != nil {
					return err
				}
				if err := tx.Table("assets").
					Where("id = ?", row.ID).
					Update("config", string(newCfg)).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}

func looksLikePlaintextKubeconfig(s string) bool {
	return strings.Contains(s, "apiVersion")
}
