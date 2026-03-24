package credential_repo

import (
	"context"

	"github.com/opskat/opskat/internal/model/entity/credential_entity"

	"github.com/cago-frame/cago/database/db"
)

// CredentialRepo 凭证数据访问接口
type CredentialRepo interface {
	Find(ctx context.Context, id int64) (*credential_entity.Credential, error)
	List(ctx context.Context) ([]*credential_entity.Credential, error)
	ListByType(ctx context.Context, credType string) ([]*credential_entity.Credential, error)
	Create(ctx context.Context, cred *credential_entity.Credential) error
	Update(ctx context.Context, cred *credential_entity.Credential) error
	Delete(ctx context.Context, id int64) error
}

var instance CredentialRepo

// RegisterCredential 注册实现
func RegisterCredential(repo CredentialRepo) {
	instance = repo
}

// Credential 获取全局实例
func Credential() CredentialRepo {
	return instance
}

// credentialRepo 默认实现
type credentialRepo struct{}

// NewCredential 创建默认实现
func NewCredential() CredentialRepo {
	return &credentialRepo{}
}

func (r *credentialRepo) Find(ctx context.Context, id int64) (*credential_entity.Credential, error) {
	var cred credential_entity.Credential
	if err := db.Ctx(ctx).Where("id = ?", id).First(&cred).Error; err != nil {
		return nil, err
	}
	return &cred, nil
}

func (r *credentialRepo) List(ctx context.Context) ([]*credential_entity.Credential, error) {
	var creds []*credential_entity.Credential
	if err := db.Ctx(ctx).Order("type ASC, createtime DESC").Find(&creds).Error; err != nil {
		return nil, err
	}
	return creds, nil
}

func (r *credentialRepo) ListByType(ctx context.Context, credType string) ([]*credential_entity.Credential, error) {
	var creds []*credential_entity.Credential
	if err := db.Ctx(ctx).Where("type = ?", credType).Order("createtime DESC").Find(&creds).Error; err != nil {
		return nil, err
	}
	return creds, nil
}

func (r *credentialRepo) Create(ctx context.Context, cred *credential_entity.Credential) error {
	return db.Ctx(ctx).Create(cred).Error
}

func (r *credentialRepo) Update(ctx context.Context, cred *credential_entity.Credential) error {
	return db.Ctx(ctx).Save(cred).Error
}

func (r *credentialRepo) Delete(ctx context.Context, id int64) error {
	return db.Ctx(ctx).Delete(&credential_entity.Credential{}, id).Error
}
