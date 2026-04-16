package assettype

import (
	"testing"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/smartystreets/goconvey/convey"
)

func TestMongoDBHandler(t *testing.T) {
	convey.Convey("MongoDB Handler", t, func() {
		h := &mongodbHandler{}
		convey.Convey("Type and DefaultPort", func() {
			convey.So(h.Type(), convey.ShouldEqual, "mongodb")
			convey.So(h.DefaultPort(), convey.ShouldEqual, 27017)
		})
		convey.Convey("SafeView", func() {
			a := &asset_entity.Asset{Type: "mongodb", Status: 1}
			_ = a.SetMongoDBConfig(&asset_entity.MongoDBConfig{
				Host: "10.0.0.1", Port: 27017, Username: "admin",
				Password: "secret", Database: "mydb",
			})
			view := h.SafeView(a)
			convey.So(view["host"], convey.ShouldEqual, "10.0.0.1")
			convey.So(view["port"], convey.ShouldEqual, 27017)
			convey.So(view["username"], convey.ShouldEqual, "admin")
			convey.So(view["database"], convey.ShouldEqual, "mydb")
			_, hasPassword := view["password"]
			convey.So(hasPassword, convey.ShouldBeFalse)
		})
		convey.Convey("ApplyCreateArgs", func() {
			a := &asset_entity.Asset{Type: "mongodb"}
			err := h.ApplyCreateArgs(a, map[string]any{
				"host": "10.0.0.1", "port": float64(27017),
				"username": "admin", "database": "mydb",
				"ssh_asset_id": float64(99),
			})
			convey.So(err, convey.ShouldBeNil)
			cfg, _ := a.GetMongoDBConfig()
			convey.So(cfg.Host, convey.ShouldEqual, "10.0.0.1")
			convey.So(cfg.Port, convey.ShouldEqual, 27017)
			convey.So(cfg.Username, convey.ShouldEqual, "admin")
			convey.So(cfg.Database, convey.ShouldEqual, "mydb")
			convey.So(cfg.AuthSource, convey.ShouldEqual, "admin")
			convey.So(a.SSHTunnelID, convey.ShouldEqual, 99)
		})
		convey.Convey("ApplyUpdateArgs", func() {
			a := &asset_entity.Asset{Type: "mongodb"}
			_ = a.SetMongoDBConfig(&asset_entity.MongoDBConfig{
				Host: "10.0.0.1", Port: 27017, Username: "admin", Database: "mydb",
			})
			err := h.ApplyUpdateArgs(a, map[string]any{"host": "10.0.0.2", "database": "newdb"})
			convey.So(err, convey.ShouldBeNil)
			cfg, _ := a.GetMongoDBConfig()
			convey.So(cfg.Host, convey.ShouldEqual, "10.0.0.2")
			convey.So(cfg.Port, convey.ShouldEqual, 27017)
			convey.So(cfg.Username, convey.ShouldEqual, "admin")
			convey.So(cfg.Database, convey.ShouldEqual, "newdb")
		})
		convey.Convey("Registered", func() {
			h, ok := Get("mongodb")
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(h.Type(), convey.ShouldEqual, "mongodb")
		})
	})
}
