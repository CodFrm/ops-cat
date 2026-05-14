package ai

import (
	"context"
	"testing"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCheckK8sPolicy_CompositeShellCommand(t *testing.T) {
	Convey("CheckK8sPolicy checks every shell execution unit", t, func() {
		ctx := context.Background()

		Convey("partial allow does not allow a later unmatched command", func() {
			policy := &asset_entity.K8sPolicy{
				AllowList: []string{"kubectl get *"},
			}

			result := CheckK8sPolicy(ctx, policy, "kubectl get pods && kubectl apply -f deploy.yaml")
			So(result.Decision, ShouldEqual, NeedConfirm)
		})

		Convey("deny list wins even when * allows other sub-commands", func() {
			policy := &asset_entity.K8sPolicy{
				AllowList: []string{"*"},
				DenyList:  []string{"kubectl delete *"},
			}

			result := CheckK8sPolicy(ctx, policy, "kubectl get pods; kubectl delete pod api-0")
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
			So(result.MatchedPattern, ShouldEqual, "kubectl delete *")
		})

		Convey("command substitution payload is checked", func() {
			policy := &asset_entity.K8sPolicy{
				AllowList: []string{"kubectl get *"},
				DenyList:  []string{"kubectl delete *"},
			}

			result := CheckK8sPolicy(ctx, policy, "kubectl get pods $(kubectl delete pod api-0)")
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
			So(result.MatchedPattern, ShouldEqual, "kubectl delete *")
		})
	})
}

func TestCheckK8sPolicy_DefaultsAndWildcard(t *testing.T) {
	Convey("CheckK8sPolicy defaults and wildcard semantics", t, func() {
		ctx := context.Background()

		Convey("nil policy uses default read-only allow", func() {
			result := CheckK8sPolicy(ctx, nil, "kubectl get pods")
			So(result.Decision, ShouldEqual, Allow)
			So(result.DecisionSource, ShouldEqual, SourcePolicyAllow)
		})

		Convey("nil policy requires confirmation for non-default writes", func() {
			result := CheckK8sPolicy(ctx, nil, "kubectl apply -f deploy.yaml")
			So(result.Decision, ShouldEqual, NeedConfirm)
		})

		Convey("nil policy applies default dangerous deny", func() {
			result := CheckK8sPolicy(ctx, nil, "kubectl delete pod api-0")
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
			So(result.MatchedPattern, ShouldEqual, "kubectl delete *")
		})

		Convey("allow wildcard allows any non-dangerous kubectl command", func() {
			policy := &asset_entity.K8sPolicy{AllowList: []string{"*"}}
			result := CheckK8sPolicy(ctx, policy, "kubectl apply -f deploy.yaml")
			So(result.Decision, ShouldEqual, Allow)

			result = CheckK8sPolicy(ctx, policy, "kubectl delete pod api-0")
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
		})

		Convey("deny wildcard denies every command", func() {
			policy := &asset_entity.K8sPolicy{DenyList: []string{"*"}}
			result := CheckK8sPolicy(ctx, policy, "kubectl get pods")
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
			So(result.MatchedPattern, ShouldEqual, "*")
		})

		Convey("parser 成功但提取不到执行单元时不能整串匹配 allow *", func() {
			// 仅注释，ExtractSubCommands 返回 []。禁止退回到 `[]string{command}`，
			// 否则 allow `*` 会把整串当成命令并放行
			policy := &asset_entity.K8sPolicy{AllowList: []string{"*"}}
			result := CheckK8sPolicy(ctx, policy, "# kubectl delete pod nginx")
			So(result.Decision, ShouldEqual, NeedConfirm)
		})
	})
}
