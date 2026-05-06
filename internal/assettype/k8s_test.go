package assettype

import (
	"context"
	"strings"
	"testing"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/service/credential_svc"

	"github.com/smartystreets/goconvey/convey"
)

const sampleKubeconfig = `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://example.com
  name: demo
contexts:
- context:
    cluster: demo
    user: admin
  name: demo
current-context: demo
users:
- name: admin
  user:
    token: secret-token
`

func setupTestCredentialSvc() {
	salt, _ := credential_svc.GenerateSalt()
	credential_svc.SetDefault(credential_svc.New("test-master-key", salt))
}

func TestK8sHandler(t *testing.T) {
	convey.Convey("K8s Handler", t, func() {
		setupTestCredentialSvc()
		h := &k8sHandler{}

		convey.Convey("Type and DefaultPort", func() {
			convey.So(h.Type(), convey.ShouldEqual, "k8s")
			convey.So(h.DefaultPort(), convey.ShouldEqual, 0)
		})

		convey.Convey("ApplyCreateArgs encrypts kubeconfig", func() {
			a := &asset_entity.Asset{Type: "k8s"}
			err := h.ApplyCreateArgs(context.Background(), a, map[string]any{
				"kubeconfig":   sampleKubeconfig,
				"namespace":    "production",
				"context":      "demo",
				"ssh_asset_id": float64(7),
			})
			convey.So(err, convey.ShouldBeNil)

			cfg, err := a.GetK8sConfig()
			convey.So(err, convey.ShouldBeNil)
			convey.So(cfg.Namespace, convey.ShouldEqual, "production")
			convey.So(cfg.Context, convey.ShouldEqual, "demo")
			convey.So(a.SSHTunnelID, convey.ShouldEqual, 7)

			// 落库的 kubeconfig 必须是密文，明文 YAML 标记不应残留
			convey.So(cfg.Kubeconfig, convey.ShouldNotEqual, sampleKubeconfig)
			convey.So(strings.Contains(cfg.Kubeconfig, "apiVersion"), convey.ShouldBeFalse)
			convey.So(strings.Contains(cfg.Kubeconfig, "secret-token"), convey.ShouldBeFalse)

			// 解密后能还原
			decrypted, err := credential_svc.Default().Decrypt(cfg.Kubeconfig)
			convey.So(err, convey.ShouldBeNil)
			convey.So(decrypted, convey.ShouldEqual, sampleKubeconfig)
		})

		convey.Convey("ApplyCreateArgs without kubeconfig leaves field empty", func() {
			a := &asset_entity.Asset{Type: "k8s"}
			err := h.ApplyCreateArgs(context.Background(), a, map[string]any{
				"namespace": "production",
			})
			convey.So(err, convey.ShouldBeNil)
			cfg, _ := a.GetK8sConfig()
			convey.So(cfg.Kubeconfig, convey.ShouldEqual, "")
		})

		convey.Convey("ApplyUpdateArgs re-encrypts replaced kubeconfig", func() {
			a := &asset_entity.Asset{Type: "k8s"}
			origEncrypted, _ := credential_svc.Default().Encrypt("old kubeconfig content")
			_ = a.SetK8sConfig(&asset_entity.K8sConfig{Kubeconfig: origEncrypted, Namespace: "old-ns"})

			err := h.ApplyUpdateArgs(context.Background(), a, map[string]any{
				"kubeconfig": sampleKubeconfig,
				"namespace":  "new-ns",
			})
			convey.So(err, convey.ShouldBeNil)
			cfg, _ := a.GetK8sConfig()
			convey.So(cfg.Namespace, convey.ShouldEqual, "new-ns")
			convey.So(cfg.Kubeconfig, convey.ShouldNotEqual, origEncrypted)
			convey.So(strings.Contains(cfg.Kubeconfig, "apiVersion"), convey.ShouldBeFalse)

			decrypted, err := credential_svc.Default().Decrypt(cfg.Kubeconfig)
			convey.So(err, convey.ShouldBeNil)
			convey.So(decrypted, convey.ShouldEqual, sampleKubeconfig)
		})

		convey.Convey("ApplyUpdateArgs without kubeconfig keeps existing ciphertext", func() {
			a := &asset_entity.Asset{Type: "k8s"}
			origEncrypted, _ := credential_svc.Default().Encrypt(sampleKubeconfig)
			_ = a.SetK8sConfig(&asset_entity.K8sConfig{Kubeconfig: origEncrypted, Namespace: "old-ns"})

			err := h.ApplyUpdateArgs(context.Background(), a, map[string]any{
				"namespace": "new-ns",
			})
			convey.So(err, convey.ShouldBeNil)
			cfg, _ := a.GetK8sConfig()
			convey.So(cfg.Kubeconfig, convey.ShouldEqual, origEncrypted)
			convey.So(cfg.Namespace, convey.ShouldEqual, "new-ns")
		})

		convey.Convey("SafeView omits kubeconfig", func() {
			a := &asset_entity.Asset{Type: "k8s", SSHTunnelID: 5}
			_ = a.SetK8sConfig(&asset_entity.K8sConfig{
				Kubeconfig: "ciphertext-blob",
				Namespace:  "default",
				Context:    "ctx",
			})
			view := h.SafeView(a)
			convey.So(view["namespace"], convey.ShouldEqual, "default")
			convey.So(view["context"], convey.ShouldEqual, "ctx")
			convey.So(view["ssh_tunnel_id"], convey.ShouldEqual, int64(5))
			_, hasKubeconfig := view["kubeconfig"]
			convey.So(hasKubeconfig, convey.ShouldBeFalse)
		})

		convey.Convey("ResolvePassword decrypts kubeconfig", func() {
			a := &asset_entity.Asset{Type: "k8s"}
			encrypted, _ := credential_svc.Default().Encrypt(sampleKubeconfig)
			_ = a.SetK8sConfig(&asset_entity.K8sConfig{Kubeconfig: encrypted})

			plain, err := h.ResolvePassword(context.Background(), a)
			convey.So(err, convey.ShouldBeNil)
			convey.So(plain, convey.ShouldEqual, sampleKubeconfig)
		})

		convey.Convey("ValidateCreateArgs requires kubeconfig", func() {
			err := h.ValidateCreateArgs(map[string]any{})
			convey.So(err, convey.ShouldNotBeNil)

			err = h.ValidateCreateArgs(map[string]any{"kubeconfig": sampleKubeconfig})
			convey.So(err, convey.ShouldBeNil)
		})

		convey.Convey("ApplyCreateArgs rejects whitespace in namespace/context", func() {
			a := &asset_entity.Asset{Type: "k8s"}
			err := h.ApplyCreateArgs(context.Background(), a, map[string]any{
				"kubeconfig": sampleKubeconfig,
				"namespace":  "bad name",
			})
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Error(), convey.ShouldContainSubstring, "whitespace")
		})

		convey.Convey("ApplyCreateArgs rejects flag-like context", func() {
			a := &asset_entity.Asset{Type: "k8s"}
			err := h.ApplyCreateArgs(context.Background(), a, map[string]any{
				"kubeconfig": sampleKubeconfig,
				"context":    "--inject",
			})
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Error(), convey.ShouldContainSubstring, "must not start with '-'")
		})

		convey.Convey("ApplyUpdateArgs rejects flag-like namespace", func() {
			a := &asset_entity.Asset{Type: "k8s"}
			_ = a.SetK8sConfig(&asset_entity.K8sConfig{Namespace: "ok"})
			err := h.ApplyUpdateArgs(context.Background(), a, map[string]any{
				"namespace": "-bad",
			})
			convey.So(err, convey.ShouldNotBeNil)
		})

		convey.Convey("Registered", func() {
			h, ok := Get("k8s")
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(h.Type(), convey.ShouldEqual, "k8s")
		})
	})
}
