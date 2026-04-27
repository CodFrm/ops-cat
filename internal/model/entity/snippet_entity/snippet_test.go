package snippet_entity

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

func TestSnippet_Validate(t *testing.T) {
	convey.Convey("Snippet.Validate", t, func() {
		convey.Convey("合法片段通过校验", func() {
			s := &Snippet{Name: "ls", Category: CategoryShell, Content: "ls -al", Source: SourceUser}
			assert.NoError(t, s.Validate())
		})
		convey.Convey("名称为空白返回错误", func() {
			s := &Snippet{Name: "  ", Category: CategoryShell, Content: "ls", Source: SourceUser}
			assert.Error(t, s.Validate())
		})
		convey.Convey("category 为空返回错误（格式性校验，合法分类由 svc 层基于注册表判定）", func() {
			s := &Snippet{Name: "x", Category: "  ", Content: "ls", Source: SourceUser}
			assert.Error(t, s.Validate())
		})
		convey.Convey("entity 层不再对非内置分类做拒绝（交由 svc 基于注册表判定）", func() {
			s := &Snippet{Name: "x", Category: "kafka-ext", Content: "ls", Source: "ext:kafka-ext"}
			assert.NoError(t, s.Validate())
		})
		convey.Convey("内容为空白返回错误", func() {
			s := &Snippet{Name: "x", Category: CategoryShell, Content: "   ", Source: SourceUser}
			assert.Error(t, s.Validate())
		})
		convey.Convey("来源为空返回错误", func() {
			s := &Snippet{Name: "x", Category: CategoryShell, Content: "ls", Source: ""}
			assert.Error(t, s.Validate())
		})
	})
}

func TestSnippet_IsReadOnly(t *testing.T) {
	convey.Convey("Snippet.IsReadOnly", t, func() {
		convey.Convey("用户来源可编辑", func() {
			s := &Snippet{Source: SourceUser}
			assert.False(t, s.IsReadOnly())
		})
		convey.Convey("扩展来源只读", func() {
			s := &Snippet{Source: "ext:foo"}
			assert.True(t, s.IsReadOnly())
		})
	})
}

func TestSnippet_TableName(t *testing.T) {
	assert.Equal(t, "snippets", Snippet{}.TableName())
}

func TestAllCategories(t *testing.T) {
	cats := AllCategories()
	assert.Len(t, cats, 5)
	assert.Contains(t, cats, CategoryShell)
	assert.Contains(t, cats, CategorySQL)
	assert.Contains(t, cats, CategoryRedis)
	assert.Contains(t, cats, CategoryMongo)
	assert.Contains(t, cats, CategoryPrompt)
}

func TestIsValidCategory(t *testing.T) {
	assert.True(t, IsValidCategory(CategoryShell))
	assert.True(t, IsValidCategory(CategoryPrompt))
	assert.False(t, IsValidCategory(""))
	assert.False(t, IsValidCategory("bogus"))
}

func TestCategoryAssetType(t *testing.T) {
	assert.Equal(t, "ssh", CategoryAssetType(CategoryShell))
	assert.Equal(t, "database", CategoryAssetType(CategorySQL))
	assert.Equal(t, "redis", CategoryAssetType(CategoryRedis))
	assert.Equal(t, "mongodb", CategoryAssetType(CategoryMongo))
	assert.Equal(t, "", CategoryAssetType(CategoryPrompt))
	assert.Equal(t, "", CategoryAssetType("bogus"))
}
