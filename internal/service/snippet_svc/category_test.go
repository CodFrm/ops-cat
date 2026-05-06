package snippet_svc

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/opskat/opskat/internal/model/entity/snippet_entity"
)

func TestCategoryRegistry_List(t *testing.T) {
	r := NewCategoryRegistry()
	list := r.List()
	assert.Len(t, list, 6)

	ids := make([]string, len(list))
	for i, c := range list {
		ids[i] = c.ID
	}
	assert.Contains(t, ids, snippet_entity.CategoryShell)
	assert.Contains(t, ids, snippet_entity.CategorySQL)
	assert.Contains(t, ids, snippet_entity.CategoryRedis)
	assert.Contains(t, ids, snippet_entity.CategoryMongo)
	assert.Contains(t, ids, snippet_entity.CategoryPrompt)
}

func TestCategoryRegistry_Get(t *testing.T) {
	r := NewCategoryRegistry()
	c, ok := r.Get(snippet_entity.CategoryShell)
	assert.True(t, ok)
	assert.Equal(t, "ssh", c.AssetType)
	assert.Equal(t, CategorySourceBuiltin, c.Source)
	assert.Equal(t, "Shell", c.Label)

	_, ok = r.Get("bogus")
	assert.False(t, ok)
}

func TestCategoryRegistry_AssetTypeMapping(t *testing.T) {
	r := NewCategoryRegistry()
	cases := map[string]string{
		snippet_entity.CategoryShell:  "ssh",
		snippet_entity.CategorySQL:    "database",
		snippet_entity.CategoryRedis:  "redis",
		snippet_entity.CategoryMongo:  "mongodb",
		snippet_entity.CategoryK8s:    "k8s",
		snippet_entity.CategoryPrompt: "",
	}
	for id, want := range cases {
		c, ok := r.Get(id)
		assert.True(t, ok, id)
		assert.Equal(t, want, c.AssetType, id)
	}
}
