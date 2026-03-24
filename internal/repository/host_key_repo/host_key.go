package host_key_repo

import (
	"context"

	"ops-cat/internal/model/entity/host_key_entity"

	"github.com/cago-frame/cago/database/db"
)

// HostKeyRepo 主机密钥数据访问接口
type HostKeyRepo interface {
	FindByHostPort(ctx context.Context, host string, port int) (*host_key_entity.HostKey, error)
	Upsert(ctx context.Context, key *host_key_entity.HostKey) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context) ([]*host_key_entity.HostKey, error)
}

var instance HostKeyRepo

// RegisterHostKey 注册实现
func RegisterHostKey(repo HostKeyRepo) {
	instance = repo
}

// HostKey 获取全局实例
func HostKey() HostKeyRepo {
	return instance
}

// hostKeyRepo 默认实现
type hostKeyRepo struct{}

// NewHostKey 创建默认实现
func NewHostKey() HostKeyRepo {
	return &hostKeyRepo{}
}

func (r *hostKeyRepo) FindByHostPort(ctx context.Context, host string, port int) (*host_key_entity.HostKey, error) {
	var key host_key_entity.HostKey
	result := db.Ctx(ctx).Where("host = ? AND port = ?", host, port).First(&key)
	if result.Error != nil {
		return nil, result.Error
	}
	return &key, nil
}

func (r *hostKeyRepo) Upsert(ctx context.Context, key *host_key_entity.HostKey) error {
	if key.ID > 0 {
		return db.Ctx(ctx).Save(key).Error
	}
	return db.Ctx(ctx).Create(key).Error
}

func (r *hostKeyRepo) Delete(ctx context.Context, id int64) error {
	return db.Ctx(ctx).Delete(&host_key_entity.HostKey{}, id).Error
}

func (r *hostKeyRepo) List(ctx context.Context) ([]*host_key_entity.HostKey, error) {
	var keys []*host_key_entity.HostKey
	if err := db.Ctx(ctx).Order("last_seen DESC").Find(&keys).Error; err != nil {
		return nil, err
	}
	return keys, nil
}
