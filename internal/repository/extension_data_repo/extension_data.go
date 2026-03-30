package extension_data_repo

import (
	"context"
	"time"

	"github.com/cago-frame/cago/database/db"
)

type ExtensionData struct {
	ExtensionName string `gorm:"column:extension_name;primaryKey"`
	Key           string `gorm:"column:key;primaryKey"`
	Value         []byte `gorm:"column:value;type:blob"`
	Updatetime    int64  `gorm:"column:updatetime"`
}

func (ExtensionData) TableName() string {
	return "extension_data"
}

type ExtensionDataRepo interface {
	Get(ctx context.Context, extName, key string) ([]byte, error)
	Set(ctx context.Context, extName, key string, value []byte) error
	Delete(ctx context.Context, extName, key string) error
	DeleteAll(ctx context.Context, extName string) error
}

var defaultRepo ExtensionDataRepo

func ExtData() ExtensionDataRepo {
	return defaultRepo
}

func Register(r ExtensionDataRepo) {
	defaultRepo = r
}

type extensionDataRepo struct{}

func New() ExtensionDataRepo {
	return &extensionDataRepo{}
}

func (r *extensionDataRepo) Get(ctx context.Context, extName, key string) ([]byte, error) {
	var row ExtensionData
	err := db.Ctx(ctx).Where("extension_name = ? AND key = ?", extName, key).First(&row).Error
	if err != nil {
		return nil, err
	}
	return row.Value, nil
}

func (r *extensionDataRepo) Set(ctx context.Context, extName, key string, value []byte) error {
	row := ExtensionData{
		ExtensionName: extName,
		Key:           key,
		Value:         value,
		Updatetime:    time.Now().Unix(),
	}
	return db.Ctx(ctx).Save(&row).Error
}

func (r *extensionDataRepo) Delete(ctx context.Context, extName, key string) error {
	return db.Ctx(ctx).Where("extension_name = ? AND key = ?", extName, key).Delete(&ExtensionData{}).Error
}

func (r *extensionDataRepo) DeleteAll(ctx context.Context, extName string) error {
	return db.Ctx(ctx).Where("extension_name = ?", extName).Delete(&ExtensionData{}).Error
}
