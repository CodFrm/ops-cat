package ai

import (
	"context"
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"

	"github.com/opskat/opskat/internal/connpool"
)

func TestMongoDBCacheContext(t *testing.T) {
	Convey("MongoDB 连接缓存 context 注入", t, func() {
		Convey("WithMongoDBCache / getMongoDBCache 往返正常", func() {
			cache := NewMongoDBClientCache()
			ctx := WithMongoDBCache(context.Background(), cache)
			got := getMongoDBCache(ctx)
			assert.NotNil(t, got)
			assert.Equal(t, cache, got)
		})

		Convey("空 context 返回 nil", func() {
			got := getMongoDBCache(context.Background())
			assert.Nil(t, got)
		})

		Convey("NewMongoDBClientCache 创建可用的缓存实例", func() {
			cache := NewMongoDBClientCache()
			assert.NotNil(t, cache)
			// 确保可以关闭一个空缓存而不报错
			err := cache.Close()
			assert.NoError(t, err)
		})
	})
}

func TestMongoDBCacheType(t *testing.T) {
	Convey("MongoDBClientCache 类型别名与 ConnCache 兼容", t, func() {
		cache := NewMongoDBClientCache()

		// 验证类型别名是否匹配 ConnCache[*connpool.MongoClientCloser]
		_ = (*ConnCache[*connpool.MongoClientCloser])(cache)
		assert.NotNil(t, cache)
	})
}

func TestParseQueryMap(t *testing.T) {
	Convey("parseQueryMap 解析 query JSON", t, func() {
		Convey("空字符串返回空 map", func() {
			m, err := parseQueryMap("")
			assert.NoError(t, err)
			assert.NotNil(t, m)
			assert.Empty(t, m)
		})

		Convey("空对象字符串返回空 map", func() {
			m, err := parseQueryMap("{}")
			assert.NoError(t, err)
			assert.NotNil(t, m)
			assert.Empty(t, m)
		})

		Convey("合法 JSON 正确解析", func() {
			m, err := parseQueryMap(`{"filter": {"name": "test"}, "limit": 10}`)
			assert.NoError(t, err)
			assert.Len(t, m, 2)
			assert.Contains(t, m, "filter")
			assert.Contains(t, m, "limit")
		})

		Convey("非法 JSON 返回错误", func() {
			_, err := parseQueryMap("not json")
			assert.Error(t, err)
		})
	})
}

func TestToBSONDoc(t *testing.T) {
	Convey("toBSONDoc 从 queryMap 提取 BSON 文档", t, func() {
		Convey("key 不存在返回 nil", func() {
			m := map[string]json.RawMessage{}
			doc, err := toBSONDoc(m, "filter")
			assert.NoError(t, err)
			assert.Nil(t, doc)
		})

		Convey("合法 JSON 对象转为 bson.D", func() {
			m := map[string]json.RawMessage{
				"filter": json.RawMessage(`{"name": "test", "age": 18}`),
			}
			doc, err := toBSONDoc(m, "filter")
			assert.NoError(t, err)
			assert.NotNil(t, doc)
			assert.Len(t, doc, 2)
		})

		Convey("非法 JSON 返回错误", func() {
			m := map[string]json.RawMessage{
				"filter": json.RawMessage(`not-json`),
			}
			_, err := toBSONDoc(m, "filter")
			assert.Error(t, err)
		})
	})
}

func TestMarshalResult(t *testing.T) {
	Convey("marshalResult 将 map 序列化为 JSON 字符串", t, func() {
		Convey("正常序列化", func() {
			result, err := marshalResult(map[string]any{
				"count":     5,
				"documents": []string{"a", "b"},
			})
			assert.NoError(t, err)
			assert.Contains(t, result, `"count":5`)
			assert.Contains(t, result, `"documents"`)
		})

		Convey("空 map 序列化为空对象", func() {
			result, err := marshalResult(map[string]any{})
			assert.NoError(t, err)
			assert.Equal(t, "{}", result)
		})
	})
}
