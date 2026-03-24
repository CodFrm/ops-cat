package ssh_key_repo

import (
	"context"

	"github.com/opskat/opskat/internal/model/entity/ssh_key_entity"

	"github.com/cago-frame/cago/database/db"
)

// SSHKeyRepo SSH 密钥数据访问接口
type SSHKeyRepo interface {
	Find(ctx context.Context, id int64) (*ssh_key_entity.SSHKey, error)
	List(ctx context.Context) ([]*ssh_key_entity.SSHKey, error)
	Create(ctx context.Context, key *ssh_key_entity.SSHKey) error
	Update(ctx context.Context, key *ssh_key_entity.SSHKey) error
	Delete(ctx context.Context, id int64) error
}

var instance SSHKeyRepo

// RegisterSSHKey 注册实现
func RegisterSSHKey(repo SSHKeyRepo) {
	instance = repo
}

// SSHKey 获取全局实例
func SSHKey() SSHKeyRepo {
	return instance
}

// sshKeyRepo 默认实现
type sshKeyRepo struct{}

// NewSSHKey 创建默认实现
func NewSSHKey() SSHKeyRepo {
	return &sshKeyRepo{}
}

func (r *sshKeyRepo) Find(ctx context.Context, id int64) (*ssh_key_entity.SSHKey, error) {
	var key ssh_key_entity.SSHKey
	if err := db.Ctx(ctx).Where("id = ?", id).First(&key).Error; err != nil {
		return nil, err
	}
	return &key, nil
}

func (r *sshKeyRepo) List(ctx context.Context) ([]*ssh_key_entity.SSHKey, error) {
	var keys []*ssh_key_entity.SSHKey
	if err := db.Ctx(ctx).Order("createtime DESC").Find(&keys).Error; err != nil {
		return nil, err
	}
	return keys, nil
}

func (r *sshKeyRepo) Create(ctx context.Context, key *ssh_key_entity.SSHKey) error {
	return db.Ctx(ctx).Create(key).Error
}

func (r *sshKeyRepo) Update(ctx context.Context, key *ssh_key_entity.SSHKey) error {
	return db.Ctx(ctx).Save(key).Error
}

func (r *sshKeyRepo) Delete(ctx context.Context, id int64) error {
	return db.Ctx(ctx).Delete(&ssh_key_entity.SSHKey{}, id).Error
}
