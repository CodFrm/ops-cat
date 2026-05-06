package ai

import (
	"testing"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBuildK8sCommandPlan(t *testing.T) {
	Convey("buildK8sCommandPlan", t, func() {
		Convey("injects default context and namespace", func() {
			plan, err := buildK8sCommandPlan("get pods", &asset_entity.K8sConfig{
				Context:   "prod",
				Namespace: "app",
			})
			So(err, ShouldBeNil)
			So(plan.EffectiveCommand, ShouldEqual, "kubectl --context prod --namespace app get pods")
		})

		Convey("keeps explicit namespace", func() {
			plan, err := buildK8sCommandPlan("kubectl get pods -n kube-system", &asset_entity.K8sConfig{
				Namespace: "app",
			})
			So(err, ShouldBeNil)
			So(plan.EffectiveCommand, ShouldEqual, "kubectl get pods -n kube-system")
		})

		Convey("rejects shell composition", func() {
			_, err := buildK8sCommandPlan("kubectl get pods && kubectl delete pod api-0", nil)
			So(err, ShouldNotBeNil)
		})

		Convey("rejects explicit kubeconfig override", func() {
			_, err := buildK8sCommandPlan("kubectl --kubeconfig /tmp/demo get pods", nil)
			So(err, ShouldNotBeNil)
		})
	})
}
