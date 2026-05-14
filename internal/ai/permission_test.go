package ai

import (
	"context"
	"testing"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/model/entity/grant_entity"
	"github.com/opskat/opskat/internal/model/entity/group_entity"
	"github.com/opskat/opskat/internal/repository/grant_repo"
	"github.com/opskat/opskat/internal/repository/group_repo"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	. "github.com/smartystreets/goconvey/convey"
)

// --- stubGrantRepo for tests ---

type stubGrantRepo struct {
	sessions map[string]*grant_entity.GrantSession
	items    map[string][]*grant_entity.GrantItem
}

func newStubGrantRepo() *stubGrantRepo {
	return &stubGrantRepo{
		sessions: make(map[string]*grant_entity.GrantSession),
		items:    make(map[string][]*grant_entity.GrantItem),
	}
}

func (r *stubGrantRepo) CreateSession(_ context.Context, s *grant_entity.GrantSession) error {
	r.sessions[s.ID] = s
	return nil
}

func (r *stubGrantRepo) GetSession(_ context.Context, id string) (*grant_entity.GrantSession, error) {
	if s, ok := r.sessions[id]; ok {
		return s, nil
	}
	return nil, assert.AnError
}

func (r *stubGrantRepo) UpdateSessionStatus(_ context.Context, id string, status int) error {
	if s, ok := r.sessions[id]; ok {
		s.Status = status
	}
	return nil
}

func (r *stubGrantRepo) CreateItems(_ context.Context, items []*grant_entity.GrantItem) error {
	for _, item := range items {
		r.items[item.GrantSessionID] = append(r.items[item.GrantSessionID], item)
	}
	return nil
}

func (r *stubGrantRepo) UpdateItems(_ context.Context, sessionID string, items []*grant_entity.GrantItem) error {
	r.items[sessionID] = items
	return nil
}

func (r *stubGrantRepo) ListItems(_ context.Context, sessionID string) ([]*grant_entity.GrantItem, error) {
	return r.items[sessionID], nil
}

func (r *stubGrantRepo) ListApprovedItems(_ context.Context, sessionID string) ([]*grant_entity.GrantItem, error) {
	s, ok := r.sessions[sessionID]
	if !ok || s.Status != grant_entity.GrantStatusApproved {
		return nil, nil
	}
	return r.items[sessionID], nil
}

// --- Tests ---

func TestCheckPermission_SSH(t *testing.T) {
	Convey("CheckPermission SSH", t, func() {
		ctx, mockAsset, _ := setupPolicyTest(t)

		Convey("deny list match → Deny", func() {
			asset := &asset_entity.Asset{
				ID:   1,
				Type: asset_entity.AssetTypeSSH,
				CmdPolicy: mustJSON(asset_entity.CommandPolicy{
					DenyList: []string{"rm -rf *"},
				}),
			}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckPermission(ctx, "ssh", 1, "rm -rf /")
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
		})

		Convey("allow list match → Allow", func() {
			asset := &asset_entity.Asset{
				ID:   1,
				Type: asset_entity.AssetTypeSSH,
				CmdPolicy: mustJSON(asset_entity.CommandPolicy{
					AllowList: []string{"ls *", "cat *"},
				}),
			}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckPermission(ctx, "ssh", 1, "ls -la")
			So(result.Decision, ShouldEqual, Allow)
			So(result.DecisionSource, ShouldEqual, SourcePolicyAllow)
		})

		Convey("no match → NeedConfirm with filtered HintRules", func() {
			asset := &asset_entity.Asset{
				ID:   1,
				Type: asset_entity.AssetTypeSSH,
				CmdPolicy: mustJSON(asset_entity.CommandPolicy{
					AllowList: []string{"ls *", "cat *", "systemctl status *"},
				}),
			}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckPermission(ctx, "ssh", 1, "systemctl restart nginx")
			So(result.Decision, ShouldEqual, NeedConfirm)
			// 只返回与命令程序名匹配的提示（systemctl），不返回 ls/cat
			So(result.HintRules, ShouldContain, "systemctl status *")
			So(result.HintRules, ShouldNotContain, "ls *")
			So(result.HintRules, ShouldNotContain, "cat *")
		})

		Convey("DB grant match → Allow", func() {
			asset := &asset_entity.Asset{ID: 1, Type: asset_entity.AssetTypeSSH}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			// Setup grant repo with approved item
			stubGrant := newStubGrantRepo()
			origGrant := grant_repo.Grant()
			grant_repo.RegisterGrant(stubGrant)
			t.Cleanup(func() {
				if origGrant != nil {
					grant_repo.RegisterGrant(origGrant)
				}
			})

			stubGrant.sessions["sess-1"] = &grant_entity.GrantSession{
				ID: "sess-1", Status: grant_entity.GrantStatusApproved,
			}
			stubGrant.items["sess-1"] = []*grant_entity.GrantItem{
				{GrantSessionID: "sess-1", AssetID: 1, Command: "uptime"},
			}

			grantCtx := WithSessionID(ctx, "sess-1")
			result := CheckPermission(grantCtx, "ssh", 1, "uptime")
			So(result.Decision, ShouldEqual, Allow)
			So(result.DecisionSource, ShouldEqual, SourceGrantAllow)
		})
	})
}

func TestCheckPermission_SSHAllowDenyDecisionMatrix(t *testing.T) {
	Convey("SSH allow/deny decision matrix", t, func() {
		ctx, mockAsset, _ := setupPolicyTest(t)

		Convey("allow matches and deny misses → Allow", func() {
			asset := &asset_entity.Asset{
				ID:   1,
				Type: asset_entity.AssetTypeSSH,
				CmdPolicy: mustJSON(asset_entity.CommandPolicy{
					AllowList: []string{"ls *"},
					DenyList:  []string{"rm *"},
				}),
			}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckPermission(ctx, "ssh", 1, "ls /tmp")
			So(result.Decision, ShouldEqual, Allow)
			So(result.DecisionSource, ShouldEqual, SourcePolicyAllow)
			So(result.MatchedPattern, ShouldEqual, "ls *")
		})

		Convey("allow misses and deny misses → NeedConfirm", func() {
			asset := &asset_entity.Asset{
				ID:   1,
				Type: asset_entity.AssetTypeSSH,
				CmdPolicy: mustJSON(asset_entity.CommandPolicy{
					AllowList: []string{"ls *"},
					DenyList:  []string{"rm *"},
				}),
			}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckPermission(ctx, "ssh", 1, "systemctl restart nginx")
			So(result.Decision, ShouldEqual, NeedConfirm)
		})

		Convey("allow misses and deny matches → Deny", func() {
			asset := &asset_entity.Asset{
				ID:   1,
				Type: asset_entity.AssetTypeSSH,
				CmdPolicy: mustJSON(asset_entity.CommandPolicy{
					AllowList: []string{"ls *"},
					DenyList:  []string{"rm *"},
				}),
			}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckPermission(ctx, "ssh", 1, "rm /tmp/foo")
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
			So(result.MatchedPattern, ShouldEqual, "rm *")
		})

		Convey("allow matches and deny matches → Deny", func() {
			asset := &asset_entity.Asset{
				ID:   1,
				Type: asset_entity.AssetTypeSSH,
				CmdPolicy: mustJSON(asset_entity.CommandPolicy{
					AllowList: []string{"rm *"},
					DenyList:  []string{"rm *"},
				}),
			}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckPermission(ctx, "ssh", 1, "rm /tmp/foo")
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
			So(result.MatchedPattern, ShouldEqual, "rm *")
		})

		Convey("allow * matches every command when deny misses → Allow", func() {
			asset := &asset_entity.Asset{
				ID:   1,
				Type: asset_entity.AssetTypeSSH,
				CmdPolicy: mustJSON(asset_entity.CommandPolicy{
					AllowList: []string{"*"},
					DenyList:  []string{"rm *"},
				}),
			}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckPermission(ctx, "ssh", 1, "systemctl restart nginx")
			So(result.Decision, ShouldEqual, Allow)
			So(result.DecisionSource, ShouldEqual, SourcePolicyAllow)
			So(result.MatchedPattern, ShouldEqual, "*")
		})

		Convey("deny * matches every command and wins over allow → Deny", func() {
			asset := &asset_entity.Asset{
				ID:   1,
				Type: asset_entity.AssetTypeSSH,
				CmdPolicy: mustJSON(asset_entity.CommandPolicy{
					AllowList: []string{"ls *"},
					DenyList:  []string{"*"},
				}),
			}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckPermission(ctx, "ssh", 1, "ls /tmp")
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
			So(result.MatchedPattern, ShouldEqual, "*")
		})
	})
}

func TestCheckPermission_Database(t *testing.T) {
	Convey("CheckPermission Database", t, func() {
		ctx, mockAsset, _ := setupPolicyTest(t)

		Convey("allow types match → Allow", func() {
			asset := &asset_entity.Asset{
				ID:   1,
				Type: asset_entity.AssetTypeDatabase,
				CmdPolicy: mustJSON(asset_entity.QueryPolicy{
					AllowTypes: []string{"SELECT", "SHOW"},
				}),
			}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckPermission(ctx, "database", 1, "SELECT * FROM users")
			So(result.Decision, ShouldEqual, Allow)
			So(result.DecisionSource, ShouldEqual, SourcePolicyAllow)
		})

		Convey("deny type → Deny", func() {
			asset := &asset_entity.Asset{
				ID:   1,
				Type: asset_entity.AssetTypeDatabase,
				CmdPolicy: mustJSON(asset_entity.QueryPolicy{
					DenyTypes: []string{"DROP TABLE"},
				}),
			}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckPermission(ctx, "database", 1, "DROP TABLE users")
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
		})

		Convey("NeedConfirm returns SQL types as HintRules", func() {
			asset := &asset_entity.Asset{
				ID:   1,
				Type: asset_entity.AssetTypeDatabase,
				CmdPolicy: mustJSON(asset_entity.QueryPolicy{
					AllowTypes: []string{"SELECT", "SHOW"},
				}),
			}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckPermission(ctx, "database", 1, "INSERT INTO users VALUES (1)")
			So(result.Decision, ShouldEqual, NeedConfirm)
			So(result.HintRules, ShouldContain, "SELECT")
			So(result.HintRules, ShouldContain, "SHOW")
		})

		Convey("empty policy uses effective defaults", func() {
			asset := &asset_entity.Asset{ID: 1, Type: asset_entity.AssetTypeDatabase}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckPermission(ctx, "database", 1, "SELECT 1")
			So(result.Decision, ShouldEqual, Allow)
			So(result.DecisionSource, ShouldEqual, SourcePolicyAllow)

			result = CheckPermission(ctx, "database", 1, "INSERT INTO users VALUES (1)")
			So(result.Decision, ShouldEqual, NeedConfirm)

			result = CheckPermission(ctx, "database", 1, "DROP TABLE users")
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
		})

		Convey("invalid SQL → Deny", func() {
			asset := &asset_entity.Asset{ID: 1, Type: asset_entity.AssetTypeDatabase}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckPermission(ctx, "database", 1, "NOT VALID SQL !!!")
			So(result.Decision, ShouldEqual, Deny)
			assert.Contains(t, result.Message, "SQL")
		})

		Convey("group deny overrides asset allow", func() {
			stubGrp := &stubGroupRepo{groups: make(map[int64]*group_entity.Group)}
			origGroup := group_repo.Group()
			group_repo.RegisterGroup(stubGrp)
			t.Cleanup(func() {
				if origGroup != nil {
					group_repo.RegisterGroup(origGroup)
				}
			})
			stubGrp.groups[10] = &group_entity.Group{
				ID: 10, Name: "prod",
				CmdPolicy: `{"deny_list":["INSERT *"]}`,
			}
			asset := &asset_entity.Asset{
				ID: 1, Type: asset_entity.AssetTypeDatabase, GroupID: 10,
				CmdPolicy: mustJSON(asset_entity.QueryPolicy{AllowTypes: []string{"SELECT", "INSERT"}}),
			}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckPermission(ctx, "database", 1, "INSERT INTO users VALUES (1)")
			So(result.Decision, ShouldEqual, Deny)
		})
	})
}

func TestCheckPermission_Redis(t *testing.T) {
	Convey("CheckPermission Redis", t, func() {
		ctx, mockAsset, _ := setupPolicyTest(t)

		Convey("allow list match → Allow", func() {
			asset := &asset_entity.Asset{
				ID:   1,
				Type: asset_entity.AssetTypeRedis,
				CmdPolicy: mustJSON(asset_entity.RedisPolicy{
					AllowList: []string{"GET *", "HGETALL *"},
				}),
			}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckPermission(ctx, "redis", 1, "GET user:1")
			So(result.Decision, ShouldEqual, Allow)
			So(result.DecisionSource, ShouldEqual, SourcePolicyAllow)
		})

		Convey("deny list match → Deny", func() {
			asset := &asset_entity.Asset{
				ID:   1,
				Type: asset_entity.AssetTypeRedis,
				CmdPolicy: mustJSON(asset_entity.RedisPolicy{
					DenyList: []string{"FLUSHDB", "FLUSHALL"},
				}),
			}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckPermission(ctx, "redis", 1, "FLUSHDB")
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
		})

		Convey("NeedConfirm returns Redis commands as HintRules", func() {
			asset := &asset_entity.Asset{
				ID:   1,
				Type: asset_entity.AssetTypeRedis,
				CmdPolicy: mustJSON(asset_entity.RedisPolicy{
					AllowList: []string{"GET *", "HGETALL *"},
				}),
			}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckPermission(ctx, "redis", 1, "SET user:1 val")
			So(result.Decision, ShouldEqual, NeedConfirm)
			So(result.HintRules, ShouldContain, "GET *")
			So(result.HintRules, ShouldContain, "HGETALL *")
		})

		Convey("empty policy uses Redis defaults", func() {
			asset := &asset_entity.Asset{ID: 1, Type: asset_entity.AssetTypeRedis}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckPermission(ctx, "redis", 1, "GET user:1")
			So(result.Decision, ShouldEqual, Allow)

			result = CheckPermission(ctx, "redis", 1, "SET user:1 val")
			So(result.Decision, ShouldEqual, NeedConfirm)

			result = CheckPermission(ctx, "redis", 1, "DEBUG STATS")
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
		})

		Convey("grant match bypasses NeedConfirm", func() {
			asset := &asset_entity.Asset{
				ID:   1,
				Type: asset_entity.AssetTypeRedis,
				CmdPolicy: mustJSON(asset_entity.RedisPolicy{
					AllowList: []string{"GET *"},
				}),
			}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			stubGrant := newStubGrantRepo()
			origGrant := grant_repo.Grant()
			grant_repo.RegisterGrant(stubGrant)
			t.Cleanup(func() {
				if origGrant != nil {
					grant_repo.RegisterGrant(origGrant)
				}
			})

			stubGrant.sessions["sess-redis"] = &grant_entity.GrantSession{
				ID: "sess-redis", Status: grant_entity.GrantStatusApproved,
			}
			stubGrant.items["sess-redis"] = []*grant_entity.GrantItem{
				{GrantSessionID: "sess-redis", AssetID: 1, Command: "SET *"},
			}

			grantCtx := WithSessionID(ctx, "sess-redis")
			result := CheckPermission(grantCtx, "redis", 1, "SET user:1 val")
			So(result.Decision, ShouldEqual, Allow)
			So(result.DecisionSource, ShouldEqual, SourceGrantAllow)
		})
	})
}

func TestCheckPermission_K8s(t *testing.T) {
	Convey("CheckPermission K8s", t, func() {
		ctx, mockAsset, _ := setupPolicyTest(t)

		Convey("allow list match → Allow", func() {
			asset := &asset_entity.Asset{
				ID:   1,
				Type: asset_entity.AssetTypeK8s,
				CmdPolicy: mustJSON(asset_entity.K8sPolicy{
					AllowList: []string{"kubectl get *", "kubectl describe *"},
				}),
			}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckPermission(ctx, asset_entity.AssetTypeK8s, 1, "kubectl get pods -A")
			So(result.Decision, ShouldEqual, Allow)
			So(result.DecisionSource, ShouldEqual, SourcePolicyAllow)
		})

		Convey("deny list match → Deny", func() {
			asset := &asset_entity.Asset{
				ID:   1,
				Type: asset_entity.AssetTypeK8s,
				CmdPolicy: mustJSON(asset_entity.K8sPolicy{
					DenyList: []string{"kubectl delete *"},
				}),
			}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckPermission(ctx, asset_entity.AssetTypeK8s, 1, "kubectl delete pod api-0")
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
		})

		Convey("NeedConfirm returns kubectl rules as HintRules", func() {
			asset := &asset_entity.Asset{
				ID:   1,
				Type: asset_entity.AssetTypeK8s,
				CmdPolicy: mustJSON(asset_entity.K8sPolicy{
					AllowList: []string{"kubectl get *", "kubectl logs *"},
				}),
			}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckPermission(ctx, asset_entity.AssetTypeK8s, 1, "kubectl rollout restart deploy/api")
			So(result.Decision, ShouldEqual, NeedConfirm)
			So(result.HintRules, ShouldContain, "kubectl get *")
			So(result.HintRules, ShouldContain, "kubectl logs *")
		})

		Convey("grant match → Allow", func() {
			asset := &asset_entity.Asset{
				ID:   1,
				Type: asset_entity.AssetTypeK8s,
				CmdPolicy: mustJSON(asset_entity.K8sPolicy{
					AllowList: []string{"kubectl get *"},
				}),
			}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			stubGrant := newStubGrantRepo()
			origGrant := grant_repo.Grant()
			grant_repo.RegisterGrant(stubGrant)
			t.Cleanup(func() {
				if origGrant != nil {
					grant_repo.RegisterGrant(origGrant)
				}
			})

			stubGrant.sessions["sess-k8s"] = &grant_entity.GrantSession{
				ID: "sess-k8s", Status: grant_entity.GrantStatusApproved,
			}
			stubGrant.items["sess-k8s"] = []*grant_entity.GrantItem{
				{GrantSessionID: "sess-k8s", AssetID: 1, Command: "kubectl rollout restart *"},
			}

			grantCtx := WithSessionID(ctx, "sess-k8s")
			result := CheckPermission(grantCtx, asset_entity.AssetTypeK8s, 1, "kubectl rollout restart deploy/api")
			So(result.Decision, ShouldEqual, Allow)
			So(result.DecisionSource, ShouldEqual, SourceGrantAllow)
		})
	})
}

func TestSaveGrantPattern(t *testing.T) {
	Convey("SaveGrantPattern", t, func() {
		stubGrant := newStubGrantRepo()
		origGrant := grant_repo.Grant()
		grant_repo.RegisterGrant(stubGrant)
		t.Cleanup(func() {
			if origGrant != nil {
				grant_repo.RegisterGrant(origGrant)
			}
		})

		ctx := context.Background()

		Convey("creates session and item", func() {
			SaveGrantPattern(ctx, "sess-1", 1, "web-01", "uptime")

			So(stubGrant.sessions, ShouldContainKey, "sess-1")
			So(stubGrant.sessions["sess-1"].Status, ShouldEqual, grant_entity.GrantStatusApproved)
			So(stubGrant.items["sess-1"], ShouldHaveLength, 1)
			So(stubGrant.items["sess-1"][0].Command, ShouldEqual, "uptime")
			So(stubGrant.items["sess-1"][0].AssetID, ShouldEqual, 1)
			So(stubGrant.items["sess-1"][0].AssetName, ShouldEqual, "web-01")
		})

		Convey("adds to existing session", func() {
			stubGrant.sessions["sess-2"] = &grant_entity.GrantSession{
				ID: "sess-2", Status: grant_entity.GrantStatusApproved,
			}

			SaveGrantPattern(ctx, "sess-2", 1, "web-01", "ls *")
			SaveGrantPattern(ctx, "sess-2", 1, "web-01", "cat *")

			So(stubGrant.items["sess-2"], ShouldHaveLength, 2)
		})

		Convey("no-op for empty sessionID", func() {
			SaveGrantPattern(ctx, "", 1, "web-01", "uptime")
			So(stubGrant.sessions, ShouldBeEmpty)
		})

		Convey("no-op for empty command", func() {
			SaveGrantPattern(ctx, "sess-3", 1, "web-01", "")
			So(stubGrant.sessions, ShouldNotContainKey, "sess-3")
		})
	})
}

func TestHandleConfirm_AllowAllGrantPatternSaving(t *testing.T) {
	Convey("handleConfirm allowAll 保存 grant 时按类型决定是否拆分", t, func() {

		Convey("SSH + EditedItems：按行 + shell 子命令拆", func() {
			ctx, mockAsset, _ := setupPolicyTest(t)
			stubGrant := newStubGrantRepo()
			origGrant := grant_repo.Grant()
			grant_repo.RegisterGrant(stubGrant)
			t.Cleanup(func() {
				if origGrant != nil {
					grant_repo.RegisterGrant(origGrant)
				}
			})

			asset := &asset_entity.Asset{ID: 1, Name: "web-01", Type: asset_entity.AssetTypeSSH}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			checker := NewCommandPolicyChecker(func(_ context.Context, kind string, items []ApprovalItem) ApprovalResponse {
				So(kind, ShouldEqual, "single")
				So(items, ShouldHaveLength, 1)
				return ApprovalResponse{
					Decision: "allowAll",
					EditedItems: []ApprovalItem{
						{Type: "exec", Command: "set -e; uname -a\ncat /etc/hosts"},
					},
				}
			})

			result := checker.Check(WithSessionID(ctx, "sess-edited"), 1, "set -e; uname -a")
			So(result.Decision, ShouldEqual, Allow)
			So(result.DecisionSource, ShouldEqual, SourceUserAllow)

			So(stubGrant.items["sess-edited"], ShouldHaveLength, 3)
			So(stubGrant.items["sess-edited"][0].Command, ShouldEqual, "set -e")
			So(stubGrant.items["sess-edited"][1].Command, ShouldEqual, "uname -a")
			So(stubGrant.items["sess-edited"][2].Command, ShouldEqual, "cat /etc/hosts")
		})

		Convey("SSH + 无 EditedItems：按 ExtractSubCommands 拆原始命令", func() {
			ctx, mockAsset, _ := setupPolicyTest(t)
			stubGrant := newStubGrantRepo()
			origGrant := grant_repo.Grant()
			grant_repo.RegisterGrant(stubGrant)
			t.Cleanup(func() {
				if origGrant != nil {
					grant_repo.RegisterGrant(origGrant)
				}
			})

			asset := &asset_entity.Asset{ID: 1, Name: "web-01", Type: asset_entity.AssetTypeSSH}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			checker := NewCommandPolicyChecker(func(_ context.Context, _ string, _ []ApprovalItem) ApprovalResponse {
				return ApprovalResponse{Decision: "allowAll"}
			})

			result := checker.Check(WithSessionID(ctx, "sess-noedit"), 1, "ls /tmp && cat /etc/hosts")
			So(result.Decision, ShouldEqual, Allow)

			So(stubGrant.items["sess-noedit"], ShouldHaveLength, 2)
			So(stubGrant.items["sess-noedit"][0].Command, ShouldEqual, "ls /tmp")
			So(stubGrant.items["sess-noedit"][1].Command, ShouldEqual, "cat /etc/hosts")
		})

		Convey("非 SSH 类型 allowAll 不走 shell AST，保留原命令一条 grant", func() {
			ctx, mockAsset, _ := setupPolicyTest(t)
			stubGrant := newStubGrantRepo()
			origGrant := grant_repo.Grant()
			grant_repo.RegisterGrant(stubGrant)
			t.Cleanup(func() {
				if origGrant != nil {
					grant_repo.RegisterGrant(origGrant)
				}
			})

			asset := &asset_entity.Asset{
				ID: 1, Name: "db-01", Type: asset_entity.AssetTypeDatabase,
				CmdPolicy: mustJSON(asset_entity.QueryPolicy{AllowTypes: []string{"SELECT"}}),
			}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			checker := NewCommandPolicyChecker(func(_ context.Context, _ string, _ []ApprovalItem) ApprovalResponse {
				return ApprovalResponse{Decision: "allowAll"}
			})

			result := checker.CheckForAsset(WithSessionID(ctx, "sess-db"), 1, asset_entity.AssetTypeDatabase, "INSERT INTO users VALUES (1); UPDATE users SET name='x'")
			So(result.Decision, ShouldEqual, Allow)

			So(stubGrant.items["sess-db"], ShouldHaveLength, 1)
			So(stubGrant.items["sess-db"][0].Command, ShouldEqual, "INSERT INTO users VALUES (1); UPDATE users SET name='x'")
		})
	})
}

func TestCheckPermission_TypeAlias(t *testing.T) {
	Convey("CheckPermission type alias mapping", t, func() {
		_, mockAsset, _ := setupPolicyTest(t)

		Convey("unknown type → NeedConfirm", func() {
			result := CheckPermission(context.Background(), "unknown", 1, "anything")
			So(result.Decision, ShouldEqual, NeedConfirm)
		})

		Convey("sql maps to database", func() {
			asset := &asset_entity.Asset{
				ID: 1, Type: asset_entity.AssetTypeDatabase,
				CmdPolicy: mustJSON(asset_entity.QueryPolicy{AllowTypes: []string{"SELECT"}}),
			}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckPermission(context.Background(), "sql", 1, "SELECT 1")
			So(result.Decision, ShouldEqual, Allow)
			So(result.DecisionSource, ShouldEqual, SourcePolicyAllow)
		})

		Convey("exec maps to ssh", func() {
			asset := &asset_entity.Asset{
				ID: 1, Type: asset_entity.AssetTypeSSH,
				CmdPolicy: mustJSON(asset_entity.CommandPolicy{AllowList: []string{"uptime"}}),
			}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckPermission(context.Background(), "exec", 1, "uptime")
			So(result.Decision, ShouldEqual, Allow)
		})
	})
}

func TestCheckPermission_GrantDoesNotOverridePolicyDeny(t *testing.T) {
	Convey("Grant cannot override policy deny", t, func() {
		ctx, mockAsset, _ := setupPolicyTest(t)

		stubGrant := newStubGrantRepo()
		origGrant := grant_repo.Grant()
		grant_repo.RegisterGrant(stubGrant)
		t.Cleanup(func() {
			if origGrant != nil {
				grant_repo.RegisterGrant(origGrant)
			}
		})

		Convey("SSH: grant exists but policy denies → Deny", func() {
			asset := &asset_entity.Asset{
				ID: 1, Type: asset_entity.AssetTypeSSH,
				CmdPolicy: mustJSON(asset_entity.CommandPolicy{DenyList: []string{"rm *"}}),
			}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			stubGrant.sessions["sess-1"] = &grant_entity.GrantSession{
				ID: "sess-1", Status: grant_entity.GrantStatusApproved,
			}
			stubGrant.items["sess-1"] = []*grant_entity.GrantItem{
				{GrantSessionID: "sess-1", AssetID: 1, Command: "rm *"},
			}

			grantCtx := WithSessionID(ctx, "sess-1")
			result := CheckPermission(grantCtx, "ssh", 1, "rm -rf /")
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
		})

		Convey("Database: grant exists but SQL type denied → Deny", func() {
			asset := &asset_entity.Asset{
				ID: 1, Type: asset_entity.AssetTypeDatabase,
				CmdPolicy: mustJSON(asset_entity.QueryPolicy{DenyTypes: []string{"DROP TABLE"}}),
			}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			stubGrant.sessions["sess-2"] = &grant_entity.GrantSession{
				ID: "sess-2", Status: grant_entity.GrantStatusApproved,
			}
			stubGrant.items["sess-2"] = []*grant_entity.GrantItem{
				{GrantSessionID: "sess-2", AssetID: 1, Command: "DROP TABLE *"},
			}

			grantCtx := WithSessionID(ctx, "sess-2")
			result := CheckPermission(grantCtx, "database", 1, "DROP TABLE users")
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
		})

		Convey("Database: grant cannot bypass default dangerous deny", func() {
			asset := &asset_entity.Asset{ID: 1, Type: asset_entity.AssetTypeDatabase}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			stubGrant.sessions["sess-default-db"] = &grant_entity.GrantSession{
				ID: "sess-default-db", Status: grant_entity.GrantStatusApproved,
			}
			stubGrant.items["sess-default-db"] = []*grant_entity.GrantItem{
				{GrantSessionID: "sess-default-db", AssetID: 1, Command: "DROP TABLE *"},
			}

			grantCtx := WithSessionID(ctx, "sess-default-db")
			result := CheckPermission(grantCtx, "database", 1, "DROP TABLE users")
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
		})

		Convey("Redis: grant exists but deny list matches → Deny", func() {
			asset := &asset_entity.Asset{
				ID: 1, Type: asset_entity.AssetTypeRedis,
				CmdPolicy: mustJSON(asset_entity.RedisPolicy{DenyList: []string{"FLUSHDB"}}),
			}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			stubGrant.sessions["sess-3"] = &grant_entity.GrantSession{
				ID: "sess-3", Status: grant_entity.GrantStatusApproved,
			}
			stubGrant.items["sess-3"] = []*grant_entity.GrantItem{
				{GrantSessionID: "sess-3", AssetID: 1, Command: "FLUSHDB"},
			}

			grantCtx := WithSessionID(ctx, "sess-3")
			result := CheckPermission(grantCtx, "redis", 1, "FLUSHDB")
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
		})

		Convey("Redis: grant cannot bypass default dangerous deny", func() {
			asset := &asset_entity.Asset{ID: 1, Type: asset_entity.AssetTypeRedis}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			stubGrant.sessions["sess-default-redis"] = &grant_entity.GrantSession{
				ID: "sess-default-redis", Status: grant_entity.GrantStatusApproved,
			}
			stubGrant.items["sess-default-redis"] = []*grant_entity.GrantItem{
				{GrantSessionID: "sess-default-redis", AssetID: 1, Command: "DEBUG *"},
			}

			grantCtx := WithSessionID(ctx, "sess-default-redis")
			result := CheckPermission(grantCtx, "redis", 1, "DEBUG STATS")
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
		})
	})
}

func TestCheckPermission_SessionIsolation(t *testing.T) {
	Convey("Grant session isolation", t, func() {
		ctx, mockAsset, _ := setupPolicyTest(t)

		stubGrant := newStubGrantRepo()
		origGrant := grant_repo.Grant()
		grant_repo.RegisterGrant(stubGrant)
		t.Cleanup(func() {
			if origGrant != nil {
				grant_repo.RegisterGrant(origGrant)
			}
		})

		asset := &asset_entity.Asset{ID: 1, Type: asset_entity.AssetTypeSSH}
		mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

		// session A has grant for "uptime"
		stubGrant.sessions["sess-A"] = &grant_entity.GrantSession{
			ID: "sess-A", Status: grant_entity.GrantStatusApproved,
		}
		stubGrant.items["sess-A"] = []*grant_entity.GrantItem{
			{GrantSessionID: "sess-A", AssetID: 1, Command: "uptime"},
		}

		Convey("session A can use its own grant", func() {
			grantCtx := WithSessionID(ctx, "sess-A")
			result := CheckPermission(grantCtx, "ssh", 1, "uptime")
			So(result.Decision, ShouldEqual, Allow)
			So(result.DecisionSource, ShouldEqual, SourceGrantAllow)
		})

		Convey("session B cannot use session A's grant", func() {
			grantCtx := WithSessionID(ctx, "sess-B")
			result := CheckPermission(grantCtx, "ssh", 1, "uptime")
			So(result.Decision, ShouldEqual, NeedConfirm)
		})

		Convey("no session cannot use any grant", func() {
			result := CheckPermission(ctx, "ssh", 1, "uptime")
			So(result.Decision, ShouldEqual, NeedConfirm)
		})
	})
}

func TestCheckPermission_MultiSubCommand(t *testing.T) {
	Convey("Multi sub-command grant matching", t, func() {
		ctx, mockAsset, _ := setupPolicyTest(t)

		stubGrant := newStubGrantRepo()
		origGrant := grant_repo.Grant()
		grant_repo.RegisterGrant(stubGrant)
		t.Cleanup(func() {
			if origGrant != nil {
				grant_repo.RegisterGrant(origGrant)
			}
		})

		asset := &asset_entity.Asset{ID: 1, Type: asset_entity.AssetTypeSSH}
		mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

		stubGrant.sessions["sess-1"] = &grant_entity.GrantSession{
			ID: "sess-1", Status: grant_entity.GrantStatusApproved,
		}
		stubGrant.items["sess-1"] = []*grant_entity.GrantItem{
			{GrantSessionID: "sess-1", AssetID: 1, Command: "ls *"},
			{GrantSessionID: "sess-1", AssetID: 1, Command: "cat *"},
		}

		grantCtx := WithSessionID(ctx, "sess-1")

		Convey("all sub-commands have grant → Allow", func() {
			result := CheckPermission(grantCtx, "ssh", 1, "ls /tmp && cat /etc/hosts")
			So(result.Decision, ShouldEqual, Allow)
			So(result.DecisionSource, ShouldEqual, SourceGrantAllow)
		})

		Convey("partial grant — one sub-command not covered → NeedConfirm", func() {
			result := CheckPermission(grantCtx, "ssh", 1, "ls /tmp && rm /tmp/foo")
			So(result.Decision, ShouldEqual, NeedConfirm)
		})
	})
}

func TestCheckPermission_ShellSubstitutionPolicy(t *testing.T) {
	Convey("shell command substitutions are checked as executable sub-commands", t, func() {
		ctx, mockAsset, _ := setupPolicyTest(t)

		Convey("allowing echo does not allow command substitution payload", func() {
			asset := &asset_entity.Asset{
				ID:   1,
				Type: asset_entity.AssetTypeSSH,
				CmdPolicy: mustJSON(asset_entity.CommandPolicy{
					AllowList: []string{"echo *"},
				}),
			}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckPermission(ctx, "ssh", 1, "echo $(rm -rf /)")
			So(result.Decision, ShouldEqual, NeedConfirm)
		})

		Convey("deny list still wins inside command substitution", func() {
			asset := &asset_entity.Asset{
				ID:   1,
				Type: asset_entity.AssetTypeSSH,
				CmdPolicy: mustJSON(asset_entity.CommandPolicy{
					AllowList: []string{"echo *", "rm *"},
					DenyList:  []string{"rm *"},
				}),
			}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckPermission(ctx, "ssh", 1, "echo $(rm -rf /)")
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
			So(result.MatchedPattern, ShouldEqual, "rm *")
		})
	})
}

func TestCheckPermission_EnvironmentPrefixPolicy(t *testing.T) {
	Convey("environment assignments before a command do not bypass command policy matching", t, func() {
		ctx, mockAsset, _ := setupPolicyTest(t)
		asset := &asset_entity.Asset{
			ID:   1,
			Type: asset_entity.AssetTypeSSH,
			CmdPolicy: mustJSON(asset_entity.CommandPolicy{
				AllowList: []string{"apt-get *"},
			}),
		}
		mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

		result := CheckPermission(ctx, "ssh", 1, "DEBIAN_FRONTEND=noninteractive apt-get update -qq")
		So(result.Decision, ShouldEqual, Allow)
		So(result.DecisionSource, ShouldEqual, SourcePolicyAllow)
		So(result.MatchedPattern, ShouldEqual, "apt-get *")
	})
}

func TestCheckPermission_ComplexShellCommandPolicy(t *testing.T) {
	Convey("complex shell command requires every executable unit to be allowed", t, func() {
		ctx, mockAsset, _ := setupPolicyTest(t)
		asset := &asset_entity.Asset{
			ID:   1,
			Type: asset_entity.AssetTypeSSH,
			CmdPolicy: mustJSON(asset_entity.CommandPolicy{
				AllowList: []string{
					"cd *",
					"apt-get *",
					"echo *",
					"printf *",
					"whoami",
					"grep *",
					"hostname",
					"uname *",
				},
				DenyList: []string{"rm *"},
			}),
		}
		mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

		command := "cd /tmp && DEBIAN_FRONTEND=noninteractive apt-get update -qq; echo \"$(printf '%s' \"$(whoami)\")\" | grep \"$(hostname)\" || echo '$(rm -rf /)' && printf %s `uname -s`"

		result := CheckPermission(ctx, "ssh", 1, command)
		So(result.Decision, ShouldEqual, Allow)
		So(result.DecisionSource, ShouldEqual, SourcePolicyAllow)
	})
}

func TestCheckPermission_SQLGrantWithTypeAlias(t *testing.T) {
	Convey("通过 'sql' 别名 + grant 放行 INSERT（覆盖别名 ∩ grant 这一交叉路径）", t, func() {
		ctx, mockAsset, _ := setupPolicyTest(t)

		stubGrant := newStubGrantRepo()
		origGrant := grant_repo.Grant()
		grant_repo.RegisterGrant(stubGrant)
		t.Cleanup(func() {
			if origGrant != nil {
				grant_repo.RegisterGrant(origGrant)
			}
		})

		asset := &asset_entity.Asset{
			ID: 1, Type: asset_entity.AssetTypeDatabase,
			CmdPolicy: mustJSON(asset_entity.QueryPolicy{AllowTypes: []string{"SELECT"}}),
		}
		mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

		stubGrant.sessions["sess-sql"] = &grant_entity.GrantSession{
			ID: "sess-sql", Status: grant_entity.GrantStatusApproved,
		}
		stubGrant.items["sess-sql"] = []*grant_entity.GrantItem{
			{GrantSessionID: "sess-sql", AssetID: 1, Command: "INSERT *"},
		}

		grantCtx := WithSessionID(ctx, "sess-sql")
		result := CheckPermission(grantCtx, "sql", 1, "INSERT INTO users VALUES (1)")
		So(result.Decision, ShouldEqual, Allow)
		So(result.DecisionSource, ShouldEqual, SourceGrantAllow)
	})
}

// TestCheckPermission_GrantSaveReuseRoundTrip 覆盖 allowAll 拆分保存 → 后续 Check
// 命中复用的端到端闭环。验证 EditedItems 中的通配 pattern 拆条后能让不同的
// 组合命令复用。
func TestCheckPermission_GrantSaveReuseRoundTrip(t *testing.T) {
	Convey("allowAll 保存的 grant 在后续 Check 中能命中复用", t, func() {
		ctx, mockAsset, _ := setupPolicyTest(t)
		stubGrant := newStubGrantRepo()
		origGrant := grant_repo.Grant()
		grant_repo.RegisterGrant(stubGrant)
		t.Cleanup(func() {
			if origGrant != nil {
				grant_repo.RegisterGrant(origGrant)
			}
		})

		asset := &asset_entity.Asset{ID: 1, Name: "web-01", Type: asset_entity.AssetTypeSSH}
		mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

		confirmCalls := 0
		checker := NewCommandPolicyChecker(func(_ context.Context, _ string, _ []ApprovalItem) ApprovalResponse {
			confirmCalls++
			return ApprovalResponse{
				Decision: "allowAll",
				EditedItems: []ApprovalItem{
					{Type: "exec", Command: "ls *\ncat *"},
				},
			}
		})

		sessCtx := WithSessionID(ctx, "sess-roundtrip")

		// 第一次：触发 allowAll，按行 + ExtractSubCommands 拆成 "ls *" + "cat *" 两条 grant
		result := checker.Check(sessCtx, 1, "ls /tmp && cat /etc/hosts")
		So(result.Decision, ShouldEqual, Allow)
		So(result.DecisionSource, ShouldEqual, SourceUserAllow)
		So(confirmCalls, ShouldEqual, 1)
		So(stubGrant.items["sess-roundtrip"], ShouldHaveLength, 2)
		So(stubGrant.items["sess-roundtrip"][0].Command, ShouldEqual, "ls *")
		So(stubGrant.items["sess-roundtrip"][1].Command, ShouldEqual, "cat *")

		// 第二次：不同的组合命令，每个子命令都能被已存 grant 命中 → SourceGrantAllow
		result2 := CheckPermission(sessCtx, "ssh", 1, "ls /var/log && cat /etc/passwd")
		So(result2.Decision, ShouldEqual, Allow)
		So(result2.DecisionSource, ShouldEqual, SourceGrantAllow)

		// 第三次：未覆盖的 rm 子命令应回到 NeedConfirm（不会偷偷复用 ls / cat grant）
		result3 := CheckPermission(sessCtx, "ssh", 1, "ls /tmp && rm /tmp/foo")
		So(result3.Decision, ShouldEqual, NeedConfirm)
		So(confirmCalls, ShouldEqual, 1) // 第二、第三次没有再次触发 confirm 回调
	})
}

// TestCheckPermission_SSHAstParseError 覆盖 #1：AST 解析失败时不能退回到整串 allow 匹配。
func TestCheckPermission_SSHAstParseError(t *testing.T) {
	Convey("shell AST 解析失败时即便有 allow * 也只能 NeedConfirm", t, func() {
		ctx, mockAsset, _ := setupPolicyTest(t)
		asset := &asset_entity.Asset{
			ID:   1,
			Type: asset_entity.AssetTypeSSH,
			CmdPolicy: mustJSON(asset_entity.CommandPolicy{
				AllowList: []string{"*"},
			}),
		}
		mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

		// 未闭合的命令替换，mvdan.cc/sh parser 会报错
		result := CheckPermission(ctx, "ssh", 1, "echo $(")
		So(result.Decision, ShouldEqual, NeedConfirm)
	})
}

// TestCheckPermission_K8sAstParseError 覆盖 #1（K8s 路径同样不能整串放行）。
func TestCheckPermission_K8sAstParseError(t *testing.T) {
	Convey("K8s shell AST 解析失败时即便 allow * 也只能 NeedConfirm", t, func() {
		ctx, mockAsset, _ := setupPolicyTest(t)
		asset := &asset_entity.Asset{
			ID:   1,
			Type: asset_entity.AssetTypeK8s,
			CmdPolicy: mustJSON(asset_entity.K8sPolicy{
				AllowList: []string{"*"},
			}),
		}
		mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

		result := CheckPermission(ctx, asset_entity.AssetTypeK8s, 1, "kubectl get $(")
		So(result.Decision, ShouldEqual, NeedConfirm)
	})
}

// TestCheckPermission_K8sAllowAllGrantSplit 覆盖 K8s allowAll 保存 grant 时按子命令拆，
// 防止 `kubectl get *` 这种宽规则被存成单条 grant 后被 `kubectl get pods && kubectl apply -f x` 绕过。
func TestCheckPermission_K8sAllowAllGrantSplit(t *testing.T) {
	Convey("K8s allowAll 也按子命令拆 grant 并按子命令匹配", t, func() {
		ctx, mockAsset, _ := setupPolicyTest(t)
		stubGrant := newStubGrantRepo()
		origGrant := grant_repo.Grant()
		grant_repo.RegisterGrant(stubGrant)
		t.Cleanup(func() {
			if origGrant != nil {
				grant_repo.RegisterGrant(origGrant)
			}
		})

		asset := &asset_entity.Asset{ID: 1, Name: "k8s-01", Type: asset_entity.AssetTypeK8s}
		mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

		checker := NewCommandPolicyChecker(func(_ context.Context, _ string, _ []ApprovalItem) ApprovalResponse {
			return ApprovalResponse{Decision: "allowAll"}
		})

		sessCtx := WithSessionID(ctx, "sess-k8s-split")

		// 默认 K8s 策略只允许 read-only：apply 与 rollout restart 都会触发 NeedConfirm 走 confirm 回调
		// 期望保存成 ["kubectl apply -f deploy.yaml", "kubectl rollout restart deploy/api"] 两条，而不是单条原文
		result := checker.CheckForAsset(sessCtx, 1, asset_entity.AssetTypeK8s, "kubectl apply -f deploy.yaml && kubectl rollout restart deploy/api")
		So(result.Decision, ShouldEqual, Allow)
		So(stubGrant.items["sess-k8s-split"], ShouldHaveLength, 2)
		So(stubGrant.items["sess-k8s-split"][0].Command, ShouldEqual, "kubectl apply -f deploy.yaml")
		So(stubGrant.items["sess-k8s-split"][1].Command, ShouldEqual, "kubectl rollout restart deploy/api")

		// 第二次：组合命令里 `kubectl delete` 被默认 deny 拦截 → Deny
		result2 := CheckPermission(sessCtx, asset_entity.AssetTypeK8s, 1, "kubectl apply -f deploy.yaml && kubectl delete pod api-0")
		So(result2.Decision, ShouldEqual, Deny)
	})
}

// TestCheckPermission_K8sGrantNotReusedAcrossComposite 覆盖 K8s grant 匹配也要走子命令，
// 否则 `kubectl get *` grant 整串匹配会让 `kubectl get pods && kubectl apply -f x` 被错误放行。
func TestCheckPermission_K8sGrantNotReusedAcrossComposite(t *testing.T) {
	Convey("K8s grant 整串匹配不能绕过子命令检查", t, func() {
		ctx, mockAsset, _ := setupPolicyTest(t)
		stubGrant := newStubGrantRepo()
		origGrant := grant_repo.Grant()
		grant_repo.RegisterGrant(stubGrant)
		t.Cleanup(func() {
			if origGrant != nil {
				grant_repo.RegisterGrant(origGrant)
			}
		})

		asset := &asset_entity.Asset{ID: 1, Type: asset_entity.AssetTypeK8s}
		mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

		stubGrant.sessions["sess-k8s-grant"] = &grant_entity.GrantSession{
			ID: "sess-k8s-grant", Status: grant_entity.GrantStatusApproved,
		}
		stubGrant.items["sess-k8s-grant"] = []*grant_entity.GrantItem{
			{GrantSessionID: "sess-k8s-grant", AssetID: 1, Command: "kubectl get *"},
		}

		grantCtx := WithSessionID(ctx, "sess-k8s-grant")
		result := CheckPermission(grantCtx, asset_entity.AssetTypeK8s, 1, "kubectl get pods && kubectl apply -f deploy.yaml")
		So(result.Decision, ShouldNotEqual, Allow)
	})
}

// TestCheckPermission_RedirectionSideEffect 覆盖：echo * 不能放行 echo pwned > /etc/cron.d/x。
// 重定向到任意路径会产生写副作用，必须作为额外执行单元强制 NeedConfirm（除非策略显式覆盖）。
func TestCheckPermission_RedirectionSideEffect(t *testing.T) {
	Convey("echo * 不能放行 echo pwned > /etc/cron.d/x", t, func() {
		ctx, mockAsset, _ := setupPolicyTest(t)
		asset := &asset_entity.Asset{
			ID:   1,
			Type: asset_entity.AssetTypeSSH,
			CmdPolicy: mustJSON(asset_entity.CommandPolicy{
				AllowList: []string{"echo *"},
			}),
		}
		mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

		result := CheckPermission(ctx, "ssh", 1, "echo pwned > /etc/cron.d/x")
		So(result.Decision, ShouldNotEqual, Allow)
	})

	Convey("/dev/null 重定向不影响匹配（公认安全目标）", t, func() {
		ctx, mockAsset, _ := setupPolicyTest(t)
		asset := &asset_entity.Asset{
			ID:   1,
			Type: asset_entity.AssetTypeSSH,
			CmdPolicy: mustJSON(asset_entity.CommandPolicy{
				AllowList: []string{"systemctl *"},
			}),
		}
		mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

		result := CheckPermission(ctx, "ssh", 1, "systemctl stop nginx 2>/dev/null")
		So(result.Decision, ShouldEqual, Allow)
	})
}

// TestCheckPermission_DangerousEnvPrefixBypass 覆盖：PATH=/tmp/evil ls 不能被 ls * 放行。
func TestCheckPermission_DangerousEnvPrefixBypass(t *testing.T) {
	Convey("PATH= 等危险环境变量前缀必须保留参与匹配", t, func() {
		ctx, mockAsset, _ := setupPolicyTest(t)
		asset := &asset_entity.Asset{
			ID:   1,
			Type: asset_entity.AssetTypeSSH,
			CmdPolicy: mustJSON(asset_entity.CommandPolicy{
				AllowList: []string{"ls *"},
			}),
		}
		mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

		result := CheckPermission(ctx, "ssh", 1, "PATH=/tmp/evil ls -la")
		So(result.Decision, ShouldNotEqual, Allow)
	})
}

// TestCheckPermission_SQLMultiStatementGroupBypass 覆盖 #3：组通用 SQL allow `SELECT *`
// 不能放行 `SELECT 1; UPDATE users SET name='x' WHERE id=1` 这样把 UPDATE 挂在分号后的语句。
// 同时 `UPDATE *` 组通用 deny 必须能命中分号后的 UPDATE。
func TestCheckPermission_SQLMultiStatementGroupBypass(t *testing.T) {
	Convey("SQL 多语句不能整串过组通用策略", t, func() {
		Convey("组 allow SELECT * 不放行后续 UPDATE 语句", func() {
			ctx, mockAsset, stubGrp := setupPolicyTest(t)
			stubGrp.groups[10] = &group_entity.Group{
				ID: 10, Name: "dev",
				CmdPolicy: `{"allow_list":["SELECT *"]}`,
			}
			asset := &asset_entity.Asset{ID: 1, Type: asset_entity.AssetTypeDatabase, GroupID: 10}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckPermission(ctx, "database", 1, "SELECT 1; UPDATE users SET name='x' WHERE id=1")
			So(result.Decision, ShouldNotEqual, Allow)
		})

		Convey("组 deny UPDATE * 命中分号后的 UPDATE 语句", func() {
			ctx, mockAsset, stubGrp := setupPolicyTest(t)
			stubGrp.groups[10] = &group_entity.Group{
				ID: 10, Name: "prod",
				CmdPolicy: `{"deny_list":["UPDATE *"]}`,
			}
			asset := &asset_entity.Asset{ID: 1, Type: asset_entity.AssetTypeDatabase, GroupID: 10}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			result := CheckPermission(ctx, "database", 1, "SELECT 1; UPDATE users SET name='x' WHERE id=1")
			So(result.Decision, ShouldEqual, Deny)
		})

		Convey("DB grant SELECT * 不能放行包含 UPDATE 的多语句 SQL", func() {
			ctx, mockAsset, _ := setupPolicyTest(t)
			stubGrant := newStubGrantRepo()
			origGrant := grant_repo.Grant()
			grant_repo.RegisterGrant(stubGrant)
			t.Cleanup(func() {
				if origGrant != nil {
					grant_repo.RegisterGrant(origGrant)
				}
			})

			asset := &asset_entity.Asset{ID: 1, Type: asset_entity.AssetTypeDatabase}
			mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

			stubGrant.sessions["sess-multi"] = &grant_entity.GrantSession{
				ID: "sess-multi", Status: grant_entity.GrantStatusApproved,
			}
			stubGrant.items["sess-multi"] = []*grant_entity.GrantItem{
				{GrantSessionID: "sess-multi", AssetID: 1, Command: "SELECT *"},
			}

			grantCtx := WithSessionID(ctx, "sess-multi")
			result := CheckPermission(grantCtx, "database", 1, "SELECT 1; UPDATE users SET name='x' WHERE id=1")
			So(result.Decision, ShouldNotEqual, Allow)
		})
	})
}

// TestNormalizeGrantPatterns 覆盖 #6：grant 拆分逻辑要在所有 SaveGrantPattern 调用前集中处理，
// 否则其他审批路径直接存复合命令会让后续 grant 匹配失败或绕过子命令检查。
func TestNormalizeGrantPatterns(t *testing.T) {
	Convey("NormalizeGrantPatterns", t, func() {
		Convey("SSH 类型按行 + ExtractSubCommands 拆", func() {
			patterns := NormalizeGrantPatterns("exec", "ls /tmp && cat /etc/hosts\nuptime")
			So(patterns, ShouldResemble, []string{"ls /tmp", "cat /etc/hosts", "uptime"})
		})

		Convey("K8s 类型与 SSH 一致拆分", func() {
			patterns := NormalizeGrantPatterns("k8s", "kubectl get pods && kubectl apply -f x.yaml")
			So(patterns, ShouldResemble, []string{"kubectl get pods", "kubectl apply -f x.yaml"})
		})

		Convey("非 shell 类型保留原命令", func() {
			patterns := NormalizeGrantPatterns("sql", "SELECT 1; UPDATE users SET name='x'")
			So(patterns, ShouldResemble, []string{"SELECT 1; UPDATE users SET name='x'"})

			patterns = NormalizeGrantPatterns("redis", "GET user:1")
			So(patterns, ShouldResemble, []string{"GET user:1"})
		})

		Convey("空命令返回 nil", func() {
			So(NormalizeGrantPatterns("exec", ""), ShouldBeNil)
			So(NormalizeGrantPatterns("exec", "   "), ShouldBeNil)
		})

		Convey("AST 解析失败保留原行", func() {
			patterns := NormalizeGrantPatterns("exec", "echo $(")
			So(patterns, ShouldResemble, []string{"echo $("})
		})

		Convey("asset_entity 类型常量与 approval type 都能识别", func() {
			// 单元测试不依赖具体常量值；只要传 AssetTypeSSH/K8s 也能走 shell 路径
			patterns := NormalizeGrantPatterns(asset_entity.AssetTypeSSH, "ls && pwd")
			So(patterns, ShouldResemble, []string{"ls", "pwd"})

			patterns = NormalizeGrantPatterns(asset_entity.AssetTypeK8s, "kubectl get pods")
			So(patterns, ShouldResemble, []string{"kubectl get pods"})
		})
	})
}

// TestCheckPermission_K8sGroupGenericBypass 覆盖 #2：组通用 allow 不能用整串匹配绕过子命令检查。
func TestCheckPermission_K8sGroupGenericBypass(t *testing.T) {
	Convey("组通用 CmdPolicy allow 必须按子命令逐条命中", t, func() {
		ctx, mockAsset, stubGrp := setupPolicyTest(t)
		stubGrp.groups[10] = &group_entity.Group{
			ID: 10, Name: "k8s-team",
			CmdPolicy: `{"allow_list":["kubectl *"]}`,
		}
		asset := &asset_entity.Asset{
			ID: 1, Type: asset_entity.AssetTypeK8s, GroupID: 10,
		}
		mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

		// 前段被 kubectl * 命中，后段是非 kubectl 命令；组层整串匹配会误放行
		result := CheckPermission(ctx, asset_entity.AssetTypeK8s, 1, "kubectl get pods && curl http://evil.com")
		So(result.Decision, ShouldNotEqual, Allow)
	})
}
