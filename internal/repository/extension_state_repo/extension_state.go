package extension_state_repo

import (
	"context"
	"time"

	"github.com/opskat/opskat/internal/model/entity/extension_state_entity"

	"github.com/cago-frame/cago/database/db"
)

type ExtensionStateRepo interface {
	Find(ctx context.Context, name string) (*extension_state_entity.ExtensionState, error)
	FindAll(ctx context.Context) ([]*extension_state_entity.ExtensionState, error)
	Create(ctx context.Context, state *extension_state_entity.ExtensionState) error
	Update(ctx context.Context, state *extension_state_entity.ExtensionState) error
	Delete(ctx context.Context, name string) error
}

var defaultRepo ExtensionStateRepo

func ExtensionState() ExtensionStateRepo {
	return defaultRepo
}

func RegisterExtensionState(r ExtensionStateRepo) {
	defaultRepo = r
}

type extensionStateRepo struct{}

func NewExtensionState() ExtensionStateRepo {
	return &extensionStateRepo{}
}

func (r *extensionStateRepo) Find(ctx context.Context, name string) (*extension_state_entity.ExtensionState, error) {
	var row extension_state_entity.ExtensionState
	err := db.Ctx(ctx).Where("name = ?", name).First(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *extensionStateRepo) FindAll(ctx context.Context) ([]*extension_state_entity.ExtensionState, error) {
	var rows []*extension_state_entity.ExtensionState
	err := db.Ctx(ctx).Find(&rows).Error
	return rows, err
}

func (r *extensionStateRepo) Create(ctx context.Context, state *extension_state_entity.ExtensionState) error {
	now := time.Now().Unix()
	state.Createtime = now
	state.Updatetime = now
	return db.Ctx(ctx).Create(state).Error
}

func (r *extensionStateRepo) Update(ctx context.Context, state *extension_state_entity.ExtensionState) error {
	state.Updatetime = time.Now().Unix()
	return db.Ctx(ctx).Save(state).Error
}

func (r *extensionStateRepo) Delete(ctx context.Context, name string) error {
	return db.Ctx(ctx).Where("name = ?", name).Delete(&extension_state_entity.ExtensionState{}).Error
}
