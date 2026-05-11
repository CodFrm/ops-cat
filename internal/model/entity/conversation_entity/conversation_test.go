package conversation_entity

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

const (
	testMentionsJSON   = `[]`
	testTokenUsageJSON = `{"prompt":10,"completion":5}` //nolint:gosec // false positive: test fixture JSON, not a credential
)

func TestMessage_V2Fields(t *testing.T) {
	Convey("Message v2 字段", t, func() {
		m := Message{
			ID:             0,
			ConversationID: 42,
			Role:           "assistant",
			Blocks:         `[{"type":"text","text":"hi"}]`,
			Mentions:       testMentionsJSON,
			TokenUsage:     testTokenUsageJSON,
			PartialReason:  "errored",
			SortOrder:      3,
		}
		So(m.PartialReason, ShouldEqual, "errored")
		So(m.SortOrder, ShouldEqual, 3)
	})
}

func TestMessageMentionsRoundtrip(t *testing.T) {
	Convey("SetMentions/GetMentions 往返", t, func() {
		msg := &Message{}
		refs := []MentionRef{
			{AssetID: 42, Name: "prod-db", Start: 6, End: 14},
			{AssetID: 43, Name: "redis-cache", Start: 20, End: 32},
		}

		Convey("非空写入后能读回", func() {
			So(msg.SetMentions(refs), ShouldBeNil)
			got, err := msg.GetMentions()
			So(err, ShouldBeNil)
			So(got, ShouldResemble, refs)
		})

		Convey("空数组写入后 Mentions 列为空字符串", func() {
			So(msg.SetMentions(nil), ShouldBeNil)
			So(msg.Mentions, ShouldEqual, "")
			got, err := msg.GetMentions()
			So(err, ShouldBeNil)
			So(got, ShouldBeNil)
		})

		Convey("空列读取返回 nil", func() {
			msg.Mentions = ""
			got, err := msg.GetMentions()
			So(err, ShouldBeNil)
			So(got, ShouldBeNil)
		})
	})
}
