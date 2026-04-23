package assettype

import (
	"context"
	"testing"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/smartystreets/goconvey/convey"
)

func TestRedisHandler(t *testing.T) {
	convey.Convey("Redis Handler", t, func() {
		h := &redisHandler{}
		convey.Convey("Type and DefaultPort", func() {
			convey.So(h.Type(), convey.ShouldEqual, "redis")
			convey.So(h.DefaultPort(), convey.ShouldEqual, 6379)
		})
		convey.Convey("SafeView", func() {
			a := &asset_entity.Asset{Type: "redis", Status: 1}
			_ = a.SetRedisConfig(&asset_entity.RedisConfig{
				Host: "10.0.0.1", Port: 6379, Username: "default",
				Password: "secret", Database: 3,
			})
			view := h.SafeView(a)
			convey.So(view["host"], convey.ShouldEqual, "10.0.0.1")
			convey.So(view["port"], convey.ShouldEqual, 6379)
			convey.So(view["username"], convey.ShouldEqual, "default")
			convey.So(view["redis_db"], convey.ShouldEqual, 3)
			_, hasPassword := view["password"]
			convey.So(hasPassword, convey.ShouldBeFalse)
		})
		convey.Convey("ApplyCreateArgs", func() {
			a := &asset_entity.Asset{Type: "redis"}
			err := h.ApplyCreateArgs(context.Background(), a, map[string]any{
				"host": "10.0.0.1", "port": float64(6379),
				"username": "default", "ssh_asset_id": float64(7),
			})
			convey.So(err, convey.ShouldBeNil)
			cfg, _ := a.GetRedisConfig()
			convey.So(cfg.Host, convey.ShouldEqual, "10.0.0.1")
			convey.So(cfg.Port, convey.ShouldEqual, 6379)
			convey.So(cfg.Username, convey.ShouldEqual, "default")
			convey.So(cfg.SSHAssetID, convey.ShouldEqual, 7)
		})
		convey.Convey("ApplyUpdateArgs", func() {
			a := &asset_entity.Asset{Type: "redis"}
			_ = a.SetRedisConfig(&asset_entity.RedisConfig{
				Host: "10.0.0.1", Port: 6379, Username: "default",
			})
			err := h.ApplyUpdateArgs(context.Background(), a, map[string]any{"host": "10.0.0.2"})
			convey.So(err, convey.ShouldBeNil)
			cfg, _ := a.GetRedisConfig()
			convey.So(cfg.Host, convey.ShouldEqual, "10.0.0.2")
			convey.So(cfg.Port, convey.ShouldEqual, 6379)
			convey.So(cfg.Username, convey.ShouldEqual, "default")
		})
		convey.Convey("Registered", func() {
			h, ok := Get("redis")
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(h.Type(), convey.ShouldEqual, "redis")
		})
	})
}
