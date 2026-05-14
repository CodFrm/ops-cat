package ai

import (
	"context"
	"testing"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"

	. "github.com/smartystreets/goconvey/convey"
)

func TestClassifyStatements(t *testing.T) {
	Convey("ClassifyStatements", t, func() {
		Convey("SELECT 1", func() {
			stmts, err := ClassifyStatements("SELECT 1")
			So(err, ShouldBeNil)
			So(stmts, ShouldHaveLength, 1)
			So(stmts[0].Type, ShouldEqual, "SELECT")
		})

		Convey("SELECT * FROM users", func() {
			stmts, err := ClassifyStatements("SELECT * FROM users")
			So(err, ShouldBeNil)
			So(stmts, ShouldHaveLength, 1)
			So(stmts[0].Type, ShouldEqual, "SELECT")
		})

		Convey("INSERT", func() {
			stmts, err := ClassifyStatements("INSERT INTO users (name) VALUES ('test')")
			So(err, ShouldBeNil)
			So(stmts[0].Type, ShouldEqual, "INSERT")
		})

		Convey("DELETE without WHERE is dangerous", func() {
			stmts, err := ClassifyStatements("DELETE FROM users")
			So(err, ShouldBeNil)
			So(stmts[0].Type, ShouldEqual, "DELETE")
			So(stmts[0].Dangerous, ShouldBeTrue)
			So(stmts[0].Reason, ShouldEqual, "no_where_delete")
		})

		Convey("DROP TABLE", func() {
			stmts, err := ClassifyStatements("DROP TABLE users")
			So(err, ShouldBeNil)
			So(stmts[0].Type, ShouldEqual, "DROP TABLE")
		})

		Convey("SHOW TABLES", func() {
			stmts, err := ClassifyStatements("SHOW TABLES")
			So(err, ShouldBeNil)
			So(stmts[0].Type, ShouldEqual, "SHOW")
		})

		Convey("multiple statements", func() {
			stmts, err := ClassifyStatements("SELECT 1; SHOW TABLES")
			So(err, ShouldBeNil)
			So(stmts, ShouldHaveLength, 2)
			So(stmts[0].Type, ShouldEqual, "SELECT")
			So(stmts[1].Type, ShouldEqual, "SHOW")
		})
	})
}

func TestCheckQueryPolicy(t *testing.T) {
	ctx := context.Background()

	Convey("CheckQueryPolicy", t, func() {
		Convey("SELECT allowed by allow_types", func() {
			p := &asset_entity.QueryPolicy{
				AllowTypes: []string{"SELECT", "SHOW"},
			}
			stmts, _ := ClassifyStatements("SELECT 1")
			result := CheckQueryPolicy(ctx, p, stmts)
			So(result.Decision, ShouldEqual, Allow)
		})

		Convey("SELECT * FROM users allowed", func() {
			p := &asset_entity.QueryPolicy{
				AllowTypes: []string{"SELECT", "SHOW"},
			}
			stmts, _ := ClassifyStatements("SELECT * FROM users LIMIT 1")
			result := CheckQueryPolicy(ctx, p, stmts)
			So(result.Decision, ShouldEqual, Allow)
		})

		Convey("INSERT not in allow_types → NeedConfirm", func() {
			p := &asset_entity.QueryPolicy{
				AllowTypes: []string{"SELECT", "SHOW"},
			}
			stmts, _ := ClassifyStatements("INSERT INTO users (name) VALUES ('test')")
			result := CheckQueryPolicy(ctx, p, stmts)
			So(result.Decision, ShouldEqual, NeedConfirm)
		})

		Convey("explicit allow_types still keeps default dangerous deny", func() {
			p := &asset_entity.QueryPolicy{
				AllowTypes: []string{"SELECT"},
			}
			stmts, _ := ClassifyStatements("DROP TABLE users")
			result := CheckQueryPolicy(ctx, p, stmts)
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
		})

		Convey("DROP TABLE in deny_types → Deny", func() {
			p := &asset_entity.QueryPolicy{
				DenyTypes: []string{"DROP TABLE"},
			}
			stmts, _ := ClassifyStatements("DROP TABLE users")
			result := CheckQueryPolicy(ctx, p, stmts)
			So(result.Decision, ShouldEqual, Deny)
		})

		Convey("nil policy uses default read-only allow", func() {
			stmts, _ := ClassifyStatements("SELECT 1")
			result := CheckQueryPolicy(ctx, nil, stmts)
			So(result.Decision, ShouldEqual, Allow)
		})

		Convey("nil policy requires confirmation for non-default write SQL", func() {
			stmts, _ := ClassifyStatements("INSERT INTO users (name) VALUES ('test')")
			result := CheckQueryPolicy(ctx, nil, stmts)
			So(result.Decision, ShouldEqual, NeedConfirm)
		})

		Convey("nil policy applies default dangerous deny", func() {
			stmts, _ := ClassifyStatements("DROP TABLE users")
			result := CheckQueryPolicy(ctx, nil, stmts)
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
		})

		Convey("explicit allow_types with SELECT matches SELECT 1", func() {
			// 这是关键场景：AllowTypes 包含 "SELECT"，SQL 是 "SELECT 1"
			// ClassifyStatements 将 "SELECT 1" 分类为 Type="SELECT"
			// 类型规则比较 "SELECT" == "SELECT" → 匹配
			p := &asset_entity.QueryPolicy{
				AllowTypes: []string{"SELECT", "SHOW", "DESCRIBE", "EXPLAIN", "USE"},
			}
			stmts, _ := ClassifyStatements("SELECT 1")
			result := CheckQueryPolicy(ctx, p, stmts)
			So(result.Decision, ShouldEqual, Allow)
		})

		Convey("allow_types wildcard allows any non-dangerous SQL type", func() {
			p := &asset_entity.QueryPolicy{AllowTypes: []string{"*"}}
			stmts, _ := ClassifyStatements("INSERT INTO users (name) VALUES ('test')")
			result := CheckQueryPolicy(ctx, p, stmts)
			So(result.Decision, ShouldEqual, Allow)
		})

		Convey("allow_types wildcard does not override default dangerous deny", func() {
			p := &asset_entity.QueryPolicy{AllowTypes: []string{"*"}}
			stmts, _ := ClassifyStatements("DROP TABLE users")
			result := CheckQueryPolicy(ctx, p, stmts)
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
		})

		Convey("deny_types wildcard denies every SQL statement", func() {
			p := &asset_entity.QueryPolicy{DenyTypes: []string{"*"}}
			stmts, _ := ClassifyStatements("SELECT 1")
			result := CheckQueryPolicy(ctx, p, stmts)
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
			So(result.MatchedPattern, ShouldEqual, "*")
		})
	})
}
