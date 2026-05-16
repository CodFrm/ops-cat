package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/model/entity/group_entity"
	"github.com/opskat/opskat/internal/repository/asset_repo"
	"github.com/opskat/opskat/internal/repository/asset_repo/mock_asset_repo"
	"github.com/opskat/opskat/internal/repository/group_repo"
	"github.com/stretchr/testify/assert"

	. "github.com/smartystreets/goconvey/convey"
	"go.uber.org/mock/gomock"
)

// stubGroupRepo 简易 group repo stub，返回预设的 group 链
type stubGroupRepo struct {
	group_repo.GroupRepo
	groups map[int64]*group_entity.Group
}

func (s *stubGroupRepo) Find(_ context.Context, id int64) (*group_entity.Group, error) {
	g, ok := s.groups[id]
	if !ok {
		return nil, fmt.Errorf("group not found: %d", id)
	}
	return g, nil
}

func (s *stubGroupRepo) List(_ context.Context) ([]*group_entity.Group, error) { return nil, nil }
func (s *stubGroupRepo) Create(_ context.Context, _ *group_entity.Group) error { return nil }
func (s *stubGroupRepo) Update(_ context.Context, _ *group_entity.Group) error { return nil }
func (s *stubGroupRepo) Delete(_ context.Context, _ int64) error               { return nil }
func (s *stubGroupRepo) ReparentChildren(_ context.Context, _, _ int64) error  { return nil }
func (s *stubGroupRepo) UpdateSortOrder(_ context.Context, _ int64, _ int) error {
	return nil
}

// setupPolicyTest 创建 mock asset repo + stub group repo，返回 cleanup 函数
func setupPolicyTest(t *testing.T) (context.Context, *mock_asset_repo.MockAssetRepo, *stubGroupRepo) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockAsset := mock_asset_repo.NewMockAssetRepo(ctrl)
	origAsset := asset_repo.Asset()
	asset_repo.RegisterAsset(mockAsset)

	stubGroup := &stubGroupRepo{groups: make(map[int64]*group_entity.Group)}
	origGroup := group_repo.Group()
	group_repo.RegisterGroup(stubGroup)

	t.Cleanup(func() {
		if origAsset != nil {
			asset_repo.RegisterAsset(origAsset)
		}
		if origGroup != nil {
			group_repo.RegisterGroup(origGroup)
		}
	})

	return context.Background(), mockAsset, stubGroup
}

// mustJSON 将结构体序列化为 JSON 字符串
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// --- CheckPolicyOnly HintRules ---

func TestCheckPolicyOnlyHintRules(t *testing.T) {
	Convey("CheckPolicyOnly returns HintRules on NeedConfirm", t, func() {
		ctx, mockRepo, _ := setupPolicyTest(t)

		Convey("NeedConfirm with allow rules returns only similar hints", func() {
			asset := &asset_entity.Asset{
				ID:   1,
				Type: asset_entity.AssetTypeSSH,
				CmdPolicy: mustJSON(asset_entity.CommandPolicy{
					AllowList: []string{"ls *", "cat *", "systemctl status *"},
				}),
			}
			mockRepo.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil)

			result := CheckPolicyOnly(ctx, 1, "systemctl restart nginx")
			So(result.Decision, ShouldEqual, NeedConfirm)
			// 只返回与命令程序名匹配的提示（systemctl），不返回 ls/cat
			So(result.HintRules, ShouldContain, "systemctl status *")
			So(result.HintRules, ShouldNotContain, "ls *")
			So(result.HintRules, ShouldNotContain, "cat *")
		})

		Convey("Allow returns no HintRules", func() {
			asset := &asset_entity.Asset{
				ID:   1,
				Type: asset_entity.AssetTypeSSH,
				CmdPolicy: mustJSON(asset_entity.CommandPolicy{
					AllowList: []string{"ls *"},
				}),
			}
			mockRepo.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil)

			result := CheckPolicyOnly(ctx, 1, "ls -la")
			So(result.Decision, ShouldEqual, Allow)
			So(result.HintRules, ShouldBeEmpty)
		})

		Convey("NeedConfirm with no allow rules returns empty HintRules", func() {
			asset := &asset_entity.Asset{
				ID:   1,
				Type: asset_entity.AssetTypeSSH,
			}
			mockRepo.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil)

			result := CheckPolicyOnly(ctx, 1, "ls -la")
			So(result.Decision, ShouldEqual, NeedConfirm)
			So(result.HintRules, ShouldBeEmpty)
		})
	})
}

// --- CheckSQLPolicyForOpsctl ---

func TestCheckSQLPolicyForOpsctl(t *testing.T) {
	Convey("CheckSQLPolicyForOpsctl", t, func() {
		ctx, mockRepo, stubGrp := setupPolicyTest(t)

		Convey("allow list match returns Allow", func() {
			asset := &asset_entity.Asset{
				ID:   1,
				Type: asset_entity.AssetTypeDatabase,
				CmdPolicy: mustJSON(asset_entity.QueryPolicy{
					AllowTypes: []string{"SELECT", "SHOW"},
				}),
			}
			// CheckGroupGenericPolicy + resolveAssetForPolicy both call Find
			mockRepo.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckSQLPolicyForOpsctl(ctx, 1, "SELECT * FROM users")
			So(result.Decision, ShouldEqual, Allow)
			So(result.DecisionSource, ShouldEqual, SourcePolicyAllow)
		})

		Convey("deny type returns Deny", func() {
			asset := &asset_entity.Asset{
				ID:   1,
				Type: asset_entity.AssetTypeDatabase,
				CmdPolicy: mustJSON(asset_entity.QueryPolicy{
					DenyTypes: []string{"DROP TABLE"},
				}),
			}
			mockRepo.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckSQLPolicyForOpsctl(ctx, 1, "DROP TABLE users")
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
		})

		Convey("NeedConfirm returns allowed SQL types as HintRules", func() {
			asset := &asset_entity.Asset{
				ID:   1,
				Type: asset_entity.AssetTypeDatabase,
				CmdPolicy: mustJSON(asset_entity.QueryPolicy{
					AllowTypes: []string{"SELECT", "SHOW", "EXPLAIN"},
				}),
			}
			mockRepo.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckSQLPolicyForOpsctl(ctx, 1, "INSERT INTO users VALUES (1, 'a')")
			So(result.Decision, ShouldEqual, NeedConfirm)
			So(result.HintRules, ShouldContain, "SELECT")
			So(result.HintRules, ShouldContain, "SHOW")
			So(result.HintRules, ShouldContain, "EXPLAIN")
		})

		Convey("group generic deny overrides asset policy", func() {
			group := &group_entity.Group{
				ID:        10,
				Name:      "prod",
				CmdPolicy: `{"deny_list":["INSERT *"]}`,
			}
			stubGrp.groups[10] = group

			asset := &asset_entity.Asset{
				ID:      1,
				Type:    asset_entity.AssetTypeDatabase,
				GroupID: 10,
				CmdPolicy: mustJSON(asset_entity.QueryPolicy{
					AllowTypes: []string{"SELECT", "INSERT"},
				}),
			}
			mockRepo.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckSQLPolicyForOpsctl(ctx, 1, "INSERT INTO users VALUES (1, 'a')")
			So(result.Decision, ShouldEqual, Deny)
		})

		Convey("group generic allow overrides query NeedConfirm", func() {
			group := &group_entity.Group{
				ID:        10,
				Name:      "dev",
				CmdPolicy: `{"allow_list":["INSERT *"]}`,
			}
			stubGrp.groups[10] = group

			asset := &asset_entity.Asset{
				ID:      1,
				Type:    asset_entity.AssetTypeDatabase,
				GroupID: 10,
				CmdPolicy: mustJSON(asset_entity.QueryPolicy{
					AllowTypes: []string{"SELECT"},
				}),
			}
			mockRepo.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckSQLPolicyForOpsctl(ctx, 1, "INSERT INTO users VALUES (1, 'a')")
			So(result.Decision, ShouldEqual, Allow)
		})

		Convey("invalid SQL returns Deny", func() {
			asset := &asset_entity.Asset{
				ID:   1,
				Type: asset_entity.AssetTypeDatabase,
			}
			mockRepo.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckSQLPolicyForOpsctl(ctx, 1, "NOT VALID SQL !!!")
			So(result.Decision, ShouldEqual, Deny)
			assert.Contains(t, result.Message, "SQL")
		})
	})
}

// --- CheckRedisPolicyForOpsctl ---

func TestCheckRedisPolicyForOpsctl(t *testing.T) {
	Convey("CheckRedisPolicyForOpsctl", t, func() {
		ctx, mockRepo, stubGrp := setupPolicyTest(t)

		Convey("allow list match returns Allow", func() {
			asset := &asset_entity.Asset{
				ID:   1,
				Type: asset_entity.AssetTypeRedis,
				CmdPolicy: mustJSON(asset_entity.RedisPolicy{
					AllowList: []string{"GET *", "HGETALL *"},
				}),
			}
			mockRepo.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckRedisPolicyForOpsctl(ctx, 1, "GET user:1")
			So(result.Decision, ShouldEqual, Allow)
			So(result.DecisionSource, ShouldEqual, SourcePolicyAllow)
		})

		Convey("deny list match returns Deny", func() {
			asset := &asset_entity.Asset{
				ID:   1,
				Type: asset_entity.AssetTypeRedis,
				CmdPolicy: mustJSON(asset_entity.RedisPolicy{
					DenyList: []string{"FLUSHDB", "FLUSHALL"},
				}),
			}
			mockRepo.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckRedisPolicyForOpsctl(ctx, 1, "FLUSHDB")
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
		})

		Convey("NeedConfirm returns allowed Redis commands as HintRules", func() {
			asset := &asset_entity.Asset{
				ID:   1,
				Type: asset_entity.AssetTypeRedis,
				CmdPolicy: mustJSON(asset_entity.RedisPolicy{
					AllowList: []string{"GET *", "HGETALL *", "KEYS *"},
				}),
			}
			mockRepo.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckRedisPolicyForOpsctl(ctx, 1, "SET user:1 val")
			So(result.Decision, ShouldEqual, NeedConfirm)
			So(result.HintRules, ShouldContain, "GET *")
			So(result.HintRules, ShouldContain, "HGETALL *")
			So(result.HintRules, ShouldContain, "KEYS *")
		})

		Convey("group generic deny overrides asset allow", func() {
			group := &group_entity.Group{
				ID:        10,
				Name:      "prod",
				CmdPolicy: `{"deny_list":["SET *"]}`,
			}
			stubGrp.groups[10] = group

			asset := &asset_entity.Asset{
				ID:      1,
				Type:    asset_entity.AssetTypeRedis,
				GroupID: 10,
				CmdPolicy: mustJSON(asset_entity.RedisPolicy{
					AllowList: []string{"GET *", "SET *"},
				}),
			}
			mockRepo.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckRedisPolicyForOpsctl(ctx, 1, "SET user:1 val")
			So(result.Decision, ShouldEqual, Deny)
		})

		Convey("group generic allow overrides redis NeedConfirm", func() {
			group := &group_entity.Group{
				ID:        10,
				Name:      "dev",
				CmdPolicy: `{"allow_list":["DEL *"]}`,
			}
			stubGrp.groups[10] = group

			asset := &asset_entity.Asset{
				ID:      1,
				Type:    asset_entity.AssetTypeRedis,
				GroupID: 10,
				CmdPolicy: mustJSON(asset_entity.RedisPolicy{
					AllowList: []string{"GET *"},
				}),
			}
			mockRepo.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckRedisPolicyForOpsctl(ctx, 1, "DEL user:1")
			So(result.Decision, ShouldEqual, Allow)
		})

		Convey("empty policy uses Redis defaults", func() {
			asset := &asset_entity.Asset{
				ID:   1,
				Type: asset_entity.AssetTypeRedis,
			}
			mockRepo.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckRedisPolicyForOpsctl(ctx, 1, "GET user:1")
			So(result.Decision, ShouldEqual, Allow)

			result = CheckRedisPolicyForOpsctl(ctx, 1, "SET user:1 val")
			So(result.Decision, ShouldEqual, NeedConfirm)

			result = CheckRedisPolicyForOpsctl(ctx, 1, "DEBUG STATS")
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
		})
	})
}
