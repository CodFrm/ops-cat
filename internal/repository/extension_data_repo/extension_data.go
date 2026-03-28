package extension_data_repo

import (
	"context"
	"sync"

	"github.com/opskat/opskat/internal/model/entity/extension_data_entity"

	"github.com/cago-frame/cago/database/db"
)

type ExtensionDataRepo interface {
	Get(ctx context.Context, extension, key string) (*extension_data_entity.ExtensionData, error)
	Set(ctx context.Context, extension, key string, value []byte) error
	Delete(ctx context.Context, extension, key string) error
}

var (
	instance ExtensionDataRepo
	once     sync.Once
)

func NewExtensionData() ExtensionDataRepo {
	once.Do(func() {
		instance = &extensionDataRepo{}
	})
	return instance
}

func ExtensionData() ExtensionDataRepo {
	return instance
}

func RegisterExtensionData(repo ExtensionDataRepo) {
	instance = repo
}

type extensionDataRepo struct{}

func (r *extensionDataRepo) Get(ctx context.Context, extension, key string) (*extension_data_entity.ExtensionData, error) {
	var data extension_data_entity.ExtensionData
	err := db.Ctx(ctx).Where("extension = ? AND key = ?", extension, key).First(&data).Error
	if err != nil {
		return nil, err
	}
	return &data, nil
}

func (r *extensionDataRepo) Set(ctx context.Context, extension, key string, value []byte) error {
	var existing extension_data_entity.ExtensionData
	result := db.Ctx(ctx).Where("extension = ? AND key = ?", extension, key).First(&existing)
	if result.Error != nil {
		// 新建
		return db.Ctx(ctx).Create(&extension_data_entity.ExtensionData{
			Extension: extension,
			Key:       key,
			Value:     value,
		}).Error
	}
	// 更新
	return db.Ctx(ctx).Model(&existing).Update("value", value).Error
}

func (r *extensionDataRepo) Delete(ctx context.Context, extension, key string) error {
	return db.Ctx(ctx).Where("extension = ? AND key = ?", extension, key).Delete(&extension_data_entity.ExtensionData{}).Error
}
