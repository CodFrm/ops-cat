package extension_data_repo

import (
	"context"
	"fmt"
	"time"

	"github.com/opskat/opskat/internal/model/entity/extension_data_entity"

	"github.com/cago-frame/cago/database/db"
)

const (
	MaxKVValueSize    = 1024 * 1024       // 1MB per value
	MaxKVPerExtension = 10 * 1024 * 1024  // 10MB total per extension
)

type ExtensionDataRepo interface {
	Get(ctx context.Context, extName, key string) ([]byte, error)
	Set(ctx context.Context, extName, key string, value []byte) error
	Delete(ctx context.Context, extName, key string) error
	DeleteAll(ctx context.Context, extName string) error
}

var defaultRepo ExtensionDataRepo

func ExtensionData() ExtensionDataRepo {
	return defaultRepo
}

func RegisterExtensionData(r ExtensionDataRepo) {
	defaultRepo = r
}

type extensionDataRepo struct{}

func NewExtensionData() ExtensionDataRepo {
	return &extensionDataRepo{}
}

func (r *extensionDataRepo) Get(ctx context.Context, extName, key string) ([]byte, error) {
	var row extension_data_entity.ExtensionData
	err := db.Ctx(ctx).Where("extension_name = ? AND key = ?", extName, key).First(&row).Error
	if err != nil {
		return nil, err
	}
	return row.Value, nil
}

func (r *extensionDataRepo) Set(ctx context.Context, extName, key string, value []byte) error {
	if len(value) > MaxKVValueSize {
		return fmt.Errorf("extension KV: value exceeds %d bytes (got %d)", MaxKVValueSize, len(value))
	}

	// Compute total size of existing keys for this extension (excluding the one we're updating)
	var totalSize int64
	err := db.Ctx(ctx).Model(&extension_data_entity.ExtensionData{}).
		Where("extension_name = ? AND key != ?", extName, key).
		Select("COALESCE(SUM(LENGTH(value)), 0)").
		Row().Scan(&totalSize)
	if err != nil {
		return fmt.Errorf("query KV total size: %w", err)
	}
	if totalSize+int64(len(value)) > MaxKVPerExtension {
		return fmt.Errorf("extension KV: extension %q quota exceeded (%d + %d > %d)",
			extName, totalSize, len(value), MaxKVPerExtension)
	}

	var existing extension_data_entity.ExtensionData
	now := time.Now().Unix()
	err = db.Ctx(ctx).Where("extension_name = ? AND key = ?", extName, key).First(&existing).Error
	if err == nil {
		return db.Ctx(ctx).Model(&existing).Updates(map[string]any{
			"value":      value,
			"updatetime": now,
		}).Error
	}
	row := extension_data_entity.ExtensionData{
		ExtensionName: extName,
		Key:           key,
		Value:         value,
		Updatetime:    now,
	}
	return db.Ctx(ctx).Create(&row).Error
}

func (r *extensionDataRepo) Delete(ctx context.Context, extName, key string) error {
	return db.Ctx(ctx).Where("extension_name = ? AND key = ?", extName, key).Delete(&extension_data_entity.ExtensionData{}).Error
}

func (r *extensionDataRepo) DeleteAll(ctx context.Context, extName string) error {
	return db.Ctx(ctx).Where("extension_name = ?", extName).Delete(&extension_data_entity.ExtensionData{}).Error
}
