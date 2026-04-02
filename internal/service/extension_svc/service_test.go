package extension_svc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/opskat/opskat/internal/model/entity/extension_state_entity"
	"github.com/opskat/opskat/internal/repository/extension_data_repo/mock_extension_data_repo"
	"github.com/opskat/opskat/internal/repository/extension_state_repo/mock_extension_state_repo"
	"github.com/opskat/opskat/pkg/extension"

	. "github.com/smartystreets/goconvey/convey"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
)

// minimalWASM is the smallest valid WASM module (magic + version header).
var minimalWASM = []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}

func writeTestExtension(dir, name string) {
	extDir := filepath.Join(dir, name)
	_ = os.MkdirAll(extDir, 0755)
	manifest := map[string]any{
		"name":    name,
		"version": "1.0.0",
		"backend": map[string]any{"runtime": "wasm", "binary": "main.wasm"},
		"assetTypes": []map[string]any{
			{"type": name, "i18n": map[string]any{"name": name + ".name"}},
		},
		"tools": []map[string]any{
			{"name": "test_tool", "i18n": map[string]any{"description": "a tool"}},
		},
	}
	data, _ := json.Marshal(manifest)
	_ = os.WriteFile(filepath.Join(extDir, "manifest.json"), data, 0644)
	_ = os.WriteFile(filepath.Join(extDir, "main.wasm"), minimalWASM, 0644)
}

func newTestManager(dir string) *extension.Manager {
	return extension.NewManager(dir, func(extName string) extension.HostProvider {
		return extension.NewDefaultHostProvider(extension.DefaultHostConfig{Logger: zap.NewNop()})
	}, zap.NewNop())
}

func TestService(t *testing.T) {
	Convey("Service", t, func() {
		ctrl := gomock.NewController(t)
		ctx := context.Background()
		dir := t.TempDir()

		stateRepo := mock_extension_state_repo.NewMockExtensionStateRepo(ctrl)
		dataRepo := mock_extension_data_repo.NewMockExtensionDataRepo(ctrl)
		logger := zap.NewNop()

		var bridgeChanged int
		var reloadCalled int
		svc := New(
			newTestManager(dir),
			stateRepo, dataRepo, logger,
			func(b *extension.Bridge) { bridgeChanged++ },
			func() { reloadCalled++ },
		)

		Convey("Init with no extensions", func() {
			stateRepo.EXPECT().FindAll(gomock.Any()).Return(nil, nil)

			err := svc.Init(ctx)
			So(err, ShouldBeNil)
			So(svc.Bridge().GetAssetTypes(), ShouldBeEmpty)
			So(bridgeChanged, ShouldEqual, 1)
		})

		Convey("Init loads extension and applies DB disabled state", func() {
			writeTestExtension(dir, "ext-a")

			stateRepo.EXPECT().FindAll(gomock.Any()).Return([]*extension_state_entity.ExtensionState{
				{Name: "ext-a", Enabled: false},
			}, nil)

			err := svc.Init(ctx)
			So(err, ShouldBeNil)

			// ext-a should be unloaded because DB says disabled
			So(svc.Bridge().GetAssetTypes(), ShouldBeEmpty)
			So(svc.Manager().GetExtension("ext-a"), ShouldBeNil)
		})

		Convey("Init loads enabled extension", func() {
			writeTestExtension(dir, "ext-b")

			stateRepo.EXPECT().FindAll(gomock.Any()).Return(nil, nil)

			err := svc.Init(ctx)
			So(err, ShouldBeNil)

			types := svc.Bridge().GetAssetTypes()
			So(len(types), ShouldEqual, 1)
			So(types[0].Type, ShouldEqual, "ext-b")
		})

		Convey("Reload closes and reinitializes", func() {
			writeTestExtension(dir, "ext-c")
			stateRepo.EXPECT().FindAll(gomock.Any()).Return(nil, nil).Times(2)

			err := svc.Init(ctx)
			So(err, ShouldBeNil)
			So(len(svc.Bridge().GetAssetTypes()), ShouldEqual, 1)

			bridgeChanged = 0
			reloadCalled = 0

			err = svc.Reload(ctx)
			So(err, ShouldBeNil)
			So(len(svc.Bridge().GetAssetTypes()), ShouldEqual, 1)
			So(bridgeChanged, ShouldEqual, 1)
			So(reloadCalled, ShouldEqual, 1)
		})

		Convey("Disable unregisters and unloads", func() {
			writeTestExtension(dir, "ext-d")
			stateRepo.EXPECT().FindAll(gomock.Any()).Return(nil, nil)

			err := svc.Init(ctx)
			So(err, ShouldBeNil)
			So(len(svc.Bridge().GetAssetTypes()), ShouldEqual, 1)

			stateRepo.EXPECT().Find(gomock.Any(), "ext-d").Return(nil, fmt.Errorf("not found"))
			stateRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)

			err = svc.Disable(ctx, "ext-d")
			So(err, ShouldBeNil)
			So(svc.Bridge().GetAssetTypes(), ShouldBeEmpty)
			So(svc.Manager().GetExtension("ext-d"), ShouldBeNil)
		})

		Convey("Enable loads and registers", func() {
			writeTestExtension(dir, "ext-e")
			stateRepo.EXPECT().FindAll(gomock.Any()).Return([]*extension_state_entity.ExtensionState{
				{Name: "ext-e", Enabled: false},
			}, nil)

			err := svc.Init(ctx)
			So(err, ShouldBeNil)
			So(svc.Bridge().GetAssetTypes(), ShouldBeEmpty)

			stateRepo.EXPECT().Find(gomock.Any(), "ext-e").Return(nil, fmt.Errorf("not found"))
			stateRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)

			err = svc.Enable(ctx, "ext-e")
			So(err, ShouldBeNil)
			So(len(svc.Bridge().GetAssetTypes()), ShouldEqual, 1)
		})

		Convey("Uninstall removes extension", func() {
			writeTestExtension(dir, "ext-f")
			stateRepo.EXPECT().FindAll(gomock.Any()).Return(nil, nil)

			err := svc.Init(ctx)
			So(err, ShouldBeNil)

			stateRepo.EXPECT().Delete(gomock.Any(), "ext-f").Return(nil)
			dataRepo.EXPECT().DeleteAll(gomock.Any(), "ext-f").Return(nil)

			err = svc.Uninstall(ctx, "ext-f", true)
			So(err, ShouldBeNil)
			So(svc.Bridge().GetAssetTypes(), ShouldBeEmpty)
		})

		Convey("Uninstall without cleanData skips data deletion", func() {
			writeTestExtension(dir, "ext-g")
			stateRepo.EXPECT().FindAll(gomock.Any()).Return(nil, nil)

			err := svc.Init(ctx)
			So(err, ShouldBeNil)

			stateRepo.EXPECT().Delete(gomock.Any(), "ext-g").Return(nil)
			// dataRepo.DeleteAll should NOT be called

			err = svc.Uninstall(ctx, "ext-g", false)
			So(err, ShouldBeNil)
		})

		Convey("ListInstalled returns enabled and disabled", func() {
			writeTestExtension(dir, "ext-h")
			stateRepo.EXPECT().FindAll(gomock.Any()).Return([]*extension_state_entity.ExtensionState{
				{Name: "ext-h", Enabled: false},
			}, nil)

			err := svc.Init(ctx)
			So(err, ShouldBeNil)

			infos := svc.ListInstalled("en")
			So(len(infos), ShouldEqual, 1)
			So(infos[0].Name, ShouldEqual, "ext-h")
			So(infos[0].Enabled, ShouldBeFalse)
		})

		Reset(func() {
			svc.Close(ctx)
		})
	})
}
