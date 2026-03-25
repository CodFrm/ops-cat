package update_svc

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

func TestCompareVersions(t *testing.T) {
	convey.Convey("版本比较", t, func() {
		convey.Convey("基础版本号比较", func() {
			assert.Equal(t, 0, compareVersions("1.0.0", "1.0.0"))
			assert.Greater(t, compareVersions("2.0.0", "1.0.0"), 0)
			assert.Less(t, compareVersions("1.0.0", "2.0.0"), 0)
			assert.Greater(t, compareVersions("1.1.0", "1.0.0"), 0)
			assert.Greater(t, compareVersions("1.0.1", "1.0.0"), 0)
		})

		convey.Convey("稳定版 > 同版本预发布", func() {
			assert.Greater(t, compareVersions("1.0.0", "1.0.0-beta.1"), 0)
			assert.Greater(t, compareVersions("1.0.0", "1.0.0-rc.1"), 0)
			assert.Less(t, compareVersions("1.0.0-beta.1", "1.0.0"), 0)
		})

		convey.Convey("预发布标识符排序", func() {
			// beta < rc (字母序)
			assert.Less(t, compareVersions("1.0.0-beta.1", "1.0.0-rc.1"), 0)
			// 同类型数字递增
			assert.Less(t, compareVersions("1.0.0-beta.1", "1.0.0-beta.2"), 0)
			assert.Greater(t, compareVersions("1.0.0-rc.2", "1.0.0-rc.1"), 0)
		})

		convey.Convey("nightly 版本比较", func() {
			// 同基线 nightly 按日期排序
			assert.Greater(t, compareVersions("1.0.0-nightly.20260326", "1.0.0-nightly.20260325"), 0)
			assert.Equal(t, 0, compareVersions("1.0.0-nightly.20260325", "1.0.0-nightly.20260325"))
			// 基于预发布的 nightly
			assert.Greater(t, compareVersions("1.0.0-beta.1.nightly.20260326", "1.0.0-beta.1.nightly.20260325"), 0)
		})

		convey.Convey("跨类型比较", func() {
			// nightly 基于 beta 的 vs 纯 beta
			assert.Greater(t, compareVersions("1.0.0-beta.1.nightly.20260325", "1.0.0-beta.1"), 0)
			// 更高基线版本胜出
			assert.Greater(t, compareVersions("1.1.0-beta.1", "1.0.0"), 0)
			assert.Less(t, compareVersions("1.0.0-nightly.20260325", "1.1.0-beta.1"), 0)
		})

		convey.Convey("不同长度版本号", func() {
			assert.Equal(t, 0, compareVersions("1.0", "1.0.0"))
			assert.Greater(t, compareVersions("1.0.1", "1.0"), 0)
		})
	})
}

func TestIsNightlyVersion(t *testing.T) {
	convey.Convey("nightly 版本判断", t, func() {
		convey.Convey("新格式（语义化）", func() {
			assert.True(t, isNightlyVersion("v1.0.0-nightly.20260325"))
			assert.True(t, isNightlyVersion("1.0.0-beta.1.nightly.20260325"))
		})

		convey.Convey("旧格式", func() {
			assert.True(t, isNightlyVersion("nightly-20260325-abc1234"))
		})

		convey.Convey("非 nightly", func() {
			assert.False(t, isNightlyVersion("v1.0.0"))
			assert.False(t, isNightlyVersion("1.0.0-beta.1"))
			assert.False(t, isNightlyVersion("1.0.0-rc.1"))
		})
	})
}

func TestHasUpdate(t *testing.T) {
	convey.Convey("更新判断", t, func() {
		convey.Convey("dev 或空版本始终有更新", func() {
			assert.True(t, hasUpdate(ChannelStable, "dev", "v1.0.0"))
			assert.True(t, hasUpdate(ChannelStable, "", "v1.0.0"))
		})

		convey.Convey("stable 通道", func() {
			convey.Convey("有新版本", func() {
				assert.True(t, hasUpdate(ChannelStable, "v1.0.0", "v1.0.1"))
				assert.True(t, hasUpdate(ChannelStable, "v1.0.0", "v2.0.0"))
			})

			convey.Convey("同版本无更新", func() {
				assert.False(t, hasUpdate(ChannelStable, "v1.0.0", "v1.0.0"))
			})

			convey.Convey("远端版本更旧无更新", func() {
				assert.False(t, hasUpdate(ChannelStable, "v1.0.1", "v1.0.0"))
			})

			convey.Convey("当前是 nightly 切换到 stable 始终更新", func() {
				assert.True(t, hasUpdate(ChannelStable, "v1.0.0-nightly.20260325", "v1.0.0"))
			})
		})

		convey.Convey("beta 通道", func() {
			convey.Convey("有新 beta 版本", func() {
				assert.True(t, hasUpdate(ChannelBeta, "v1.0.0-beta.1", "v1.0.0-beta.2"))
			})

			convey.Convey("当前是 nightly 切换到 beta 始终更新", func() {
				assert.True(t, hasUpdate(ChannelBeta, "v1.0.0-nightly.20260325", "v1.0.0-beta.1"))
			})
		})

		convey.Convey("nightly 通道", func() {
			convey.Convey("从 stable 切换到 nightly 始终更新", func() {
				assert.True(t, hasUpdate(ChannelNightly, "v1.0.0", "v1.0.0-nightly.20260325"))
			})

			convey.Convey("旧格式 nightly 字符串比较", func() {
				assert.True(t, hasUpdate(ChannelNightly, "nightly-20260324-abc", "nightly-20260325-def"))
				assert.False(t, hasUpdate(ChannelNightly, "nightly-20260325-abc", "nightly-20260325-abc"))
			})

			convey.Convey("新格式 nightly 语义化比较", func() {
				assert.True(t, hasUpdate(ChannelNightly, "v1.0.0-nightly.20260324", "v1.0.0-nightly.20260325"))
				assert.False(t, hasUpdate(ChannelNightly, "v1.0.0-nightly.20260325", "v1.0.0-nightly.20260325"))
				assert.False(t, hasUpdate(ChannelNightly, "v1.0.0-nightly.20260326", "v1.0.0-nightly.20260325"))
			})
		})
	})
}

func TestSplitPreRelease(t *testing.T) {
	convey.Convey("分离预发布后缀", t, func() {
		base, pre := splitPreRelease("1.0.0")
		assert.Equal(t, "1.0.0", base)
		assert.Equal(t, "", pre)

		base, pre = splitPreRelease("1.0.0-beta.1")
		assert.Equal(t, "1.0.0", base)
		assert.Equal(t, "beta.1", pre)

		base, pre = splitPreRelease("1.0.0-beta.1.nightly.20260325")
		assert.Equal(t, "1.0.0", base)
		assert.Equal(t, "beta.1.nightly.20260325", pre)
	})
}
