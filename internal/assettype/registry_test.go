package assettype

import (
	"context"
	"testing"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/smartystreets/goconvey/convey"
)

type stubHandler struct {
	typ  string
	port int
}

func (s *stubHandler) Type() string     { return s.typ }
func (s *stubHandler) DefaultPort() int { return s.port }
func (s *stubHandler) SafeView(_ *asset_entity.Asset) map[string]any {
	return map[string]any{"stub": true}
}
func (s *stubHandler) ResolvePassword(_ context.Context, _ *asset_entity.Asset) (string, error) {
	return "", nil
}
func (s *stubHandler) DefaultPolicy() any                       { return nil }
func (s *stubHandler) ValidateCreateArgs(_ map[string]any) error { return nil }
func (s *stubHandler) ApplyCreateArgs(_ context.Context, _ *asset_entity.Asset, _ map[string]any) error {
	return nil
}
func (s *stubHandler) ApplyUpdateArgs(_ context.Context, _ *asset_entity.Asset, _ map[string]any) error {
	return nil
}

func TestRegistry(t *testing.T) {
	convey.Convey("AssetType Registry", t, func() {
		mu.Lock()
		orig := registry
		registry = map[string]AssetTypeHandler{}
		mu.Unlock()
		defer func() {
			mu.Lock()
			registry = orig
			mu.Unlock()
		}()

		convey.Convey("Get returns false for unregistered type", func() {
			_, ok := Get("nonexistent")
			convey.So(ok, convey.ShouldBeFalse)
		})

		convey.Convey("Register and Get works", func() {
			Register(&stubHandler{typ: "test", port: 9999})
			h, ok := Get("test")
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(h.Type(), convey.ShouldEqual, "test")
			convey.So(h.DefaultPort(), convey.ShouldEqual, 9999)
		})

		convey.Convey("All returns all registered handlers", func() {
			Register(&stubHandler{typ: "a", port: 1})
			Register(&stubHandler{typ: "b", port: 2})
			convey.So(len(All()), convey.ShouldEqual, 2)
		})
	})
}
