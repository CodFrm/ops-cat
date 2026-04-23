package assettype

import (
	"context"
	"testing"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/smartystreets/goconvey/convey"
)

func TestSSHHandler(t *testing.T) {
	convey.Convey("SSH Handler", t, func() {
		h := &sshHandler{}
		convey.Convey("Type and DefaultPort", func() {
			convey.So(h.Type(), convey.ShouldEqual, "ssh")
			convey.So(h.DefaultPort(), convey.ShouldEqual, 22)
		})
		convey.Convey("SafeView", func() {
			a := &asset_entity.Asset{Type: "ssh", Status: 1}
			_ = a.SetSSHConfig(&asset_entity.SSHConfig{
				Host: "10.0.0.1", Port: 22, Username: "root",
				AuthType: "password", Password: "secret",
			})
			view := h.SafeView(a)
			convey.So(view["host"], convey.ShouldEqual, "10.0.0.1")
			convey.So(view["port"], convey.ShouldEqual, 22)
			convey.So(view["username"], convey.ShouldEqual, "root")
			convey.So(view["auth_type"], convey.ShouldEqual, "password")
			_, hasPassword := view["password"]
			convey.So(hasPassword, convey.ShouldBeFalse)
		})
		convey.Convey("ApplyCreateArgs", func() {
			a := &asset_entity.Asset{Type: "ssh"}
			err := h.ApplyCreateArgs(context.Background(), a, map[string]any{
				"host": "10.0.0.1", "port": float64(22), "username": "root",
			})
			convey.So(err, convey.ShouldBeNil)
			cfg, _ := a.GetSSHConfig()
			convey.So(cfg.Host, convey.ShouldEqual, "10.0.0.1")
			convey.So(cfg.Port, convey.ShouldEqual, 22)
			convey.So(cfg.AuthType, convey.ShouldEqual, "password") // default
		})
		convey.Convey("ApplyUpdateArgs", func() {
			a := &asset_entity.Asset{Type: "ssh"}
			_ = a.SetSSHConfig(&asset_entity.SSHConfig{
				Host: "10.0.0.1", Port: 22, Username: "root", AuthType: "password",
			})
			err := h.ApplyUpdateArgs(context.Background(), a, map[string]any{"host": "10.0.0.2"})
			convey.So(err, convey.ShouldBeNil)
			cfg, _ := a.GetSSHConfig()
			convey.So(cfg.Host, convey.ShouldEqual, "10.0.0.2")
			convey.So(cfg.Port, convey.ShouldEqual, 22)
			convey.So(cfg.Username, convey.ShouldEqual, "root")
		})
		convey.Convey("Registered", func() {
			h, ok := Get("ssh")
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(h.Type(), convey.ShouldEqual, "ssh")
		})
	})
}
