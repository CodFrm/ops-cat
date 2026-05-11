package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opskat/opskat/internal/ai"
)

// TestMentionsToStash_PreservesStartEnd 回归 bug2b：刷新后用户消息里的 mention chip
// 渲染成空 button——根因是 mentionsToStash 把前端的 start/end 偏移丢了，DB 里只剩
// {assetId, name, type, host, groupPath}。GetMentions 反序列化时 start/end 默认成 0，
// 前端 UserMessage.buildSegments 用 content.slice(0,0) 切出空字符串 → 空 chip。
func TestMentionsToStash_PreservesStartEnd(t *testing.T) {
	ms := []ai.MentionedAsset{{
		AssetID:   42,
		Name:      "local-docker",
		Type:      "ssh",
		Host:      "192.168.8.141",
		GroupPath: "本地",
		Start:     0,
		End:       13, // "@local-docker"
	}}

	stash := mentionsToStash(ms)
	require.Len(t, stash, 1)
	m := stash[0]

	assert.EqualValues(t, 42, m["assetId"])
	assert.Equal(t, "local-docker", m["name"])
	assert.Equal(t, 0, m["start"], "start 必须保留以便 UserMessage 渲染 chip")
	assert.Equal(t, 13, m["end"], "end 必须保留以便 UserMessage 渲染 chip")
}

// TestMentionsToStash_MultipleMentionsKeepIndividualOffsets 多个 mention 时
// 各自的 offset 要分别保留——前端按 mention.start 排序后切片，offset 错位会让
// 整个气泡渲染崩坏。
func TestMentionsToStash_MultipleMentionsKeepIndividualOffsets(t *testing.T) {
	ms := []ai.MentionedAsset{
		{AssetID: 1, Name: "a", Start: 0, End: 2},
		{AssetID: 2, Name: "bb", Start: 10, End: 13},
	}
	stash := mentionsToStash(ms)
	require.Len(t, stash, 2)
	assert.Equal(t, 0, stash[0]["start"])
	assert.Equal(t, 2, stash[0]["end"])
	assert.Equal(t, 10, stash[1]["start"])
	assert.Equal(t, 13, stash[1]["end"])
}
