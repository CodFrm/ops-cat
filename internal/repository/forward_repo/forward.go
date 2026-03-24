package forward_repo

import (
	"context"

	"ops-cat/internal/model/entity/forward_entity"

	"github.com/cago-frame/cago/database/db"
)

type ForwardRepo interface {
	// Config CRUD
	FindConfig(ctx context.Context, id int64) (*forward_entity.ForwardConfig, error)
	ListConfigs(ctx context.Context) ([]*forward_entity.ForwardConfig, error)
	CreateConfig(ctx context.Context, config *forward_entity.ForwardConfig) error
	UpdateConfig(ctx context.Context, config *forward_entity.ForwardConfig) error
	DeleteConfig(ctx context.Context, id int64) error

	// Rule CRUD
	ListRulesByConfigID(ctx context.Context, configID int64) ([]*forward_entity.ForwardRule, error)
	ReplaceRules(ctx context.Context, configID int64, rules []*forward_entity.ForwardRule) error
	DeleteRulesByConfigID(ctx context.Context, configID int64) error
}

var instance ForwardRepo

func RegisterForward(repo ForwardRepo) {
	instance = repo
}

func Forward() ForwardRepo {
	return instance
}

func NewForward() ForwardRepo {
	return &forwardRepo{}
}

type forwardRepo struct{}

func (r *forwardRepo) FindConfig(ctx context.Context, id int64) (*forward_entity.ForwardConfig, error) {
	var config forward_entity.ForwardConfig
	if err := db.Ctx(ctx).Where("id = ?", id).First(&config).Error; err != nil {
		return nil, err
	}
	return &config, nil
}

func (r *forwardRepo) ListConfigs(ctx context.Context) ([]*forward_entity.ForwardConfig, error) {
	var configs []*forward_entity.ForwardConfig
	if err := db.Ctx(ctx).Order("createtime DESC").Find(&configs).Error; err != nil {
		return nil, err
	}
	return configs, nil
}

func (r *forwardRepo) CreateConfig(ctx context.Context, config *forward_entity.ForwardConfig) error {
	return db.Ctx(ctx).Create(config).Error
}

func (r *forwardRepo) UpdateConfig(ctx context.Context, config *forward_entity.ForwardConfig) error {
	return db.Ctx(ctx).Save(config).Error
}

func (r *forwardRepo) DeleteConfig(ctx context.Context, id int64) error {
	return db.Ctx(ctx).Delete(&forward_entity.ForwardConfig{}, id).Error
}

func (r *forwardRepo) ListRulesByConfigID(ctx context.Context, configID int64) ([]*forward_entity.ForwardRule, error) {
	var rules []*forward_entity.ForwardRule
	if err := db.Ctx(ctx).Where("config_id = ?", configID).Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

func (r *forwardRepo) ReplaceRules(ctx context.Context, configID int64, rules []*forward_entity.ForwardRule) error {
	// 删除旧规则，插入新规则
	if err := db.Ctx(ctx).Where("config_id = ?", configID).Delete(&forward_entity.ForwardRule{}).Error; err != nil {
		return err
	}
	if len(rules) == 0 {
		return nil
	}
	for i := range rules {
		rules[i].ConfigID = configID
	}
	return db.Ctx(ctx).Create(&rules).Error
}

func (r *forwardRepo) DeleteRulesByConfigID(ctx context.Context, configID int64) error {
	return db.Ctx(ctx).Where("config_id = ?", configID).Delete(&forward_entity.ForwardRule{}).Error
}
