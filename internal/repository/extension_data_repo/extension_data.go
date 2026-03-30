package extension_data_repo

import (
	"context"
	"time"

	"github.com/opskat/opskat/internal/model/entity/extension_data_entity"

	"github.com/cago-frame/cago/database/db"
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
	var existing extension_data_entity.ExtensionData
	now := time.Now().Unix()
	err := db.Ctx(ctx).Where("extension_name = ? AND key = ?", extName, key).First(&existing).Error
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
