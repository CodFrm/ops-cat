package conversation_entity

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMessage_CagoFieldsRoundTrip(t *testing.T) {
	Convey("Message cago 平铺字段读写", t, func() {
		m := &Message{}
		m.CagoID = "c-1"
		m.ParentID = "p-1"
		m.Kind = "assistant"
		m.Origin = "model"
		m.Thinking = "thinking content"
		m.ToolCallJSON = `{"name":"foo"}`
		m.ToolResultJSON = `{"ok":true}`
		m.Persist = true
		m.Raw = `{"raw":"value"}`
		m.MsgTime = 1715155200

		Convey("所有 cago 字段能读回", func() {
			So(m.CagoID, ShouldEqual, "c-1")
			So(m.ParentID, ShouldEqual, "p-1")
			So(m.Kind, ShouldEqual, "assistant")
			So(m.Origin, ShouldEqual, "model")
			So(m.Thinking, ShouldEqual, "thinking content")
			So(m.ToolCallJSON, ShouldEqual, `{"name":"foo"}`)
			So(m.ToolResultJSON, ShouldEqual, `{"ok":true}`)
			So(m.Persist, ShouldBeTrue)
			So(m.Raw, ShouldEqual, `{"raw":"value"}`)
			So(m.MsgTime, ShouldEqual, int64(1715155200))
		})
	})
}

func TestConversation_StateRoundTrip(t *testing.T) {
	Convey("Conversation StateValues 往返", t, func() {
		c := &Conversation{ThreadID: "t-1"}
		Convey("非空写入后能读回", func() {
			So(c.SetStateValues(map[string]string{"k": "v"}), ShouldBeNil)
			got, err := c.GetStateValues()
			So(err, ShouldBeNil)
			So(got["k"], ShouldEqual, "v")
		})
		Convey("nil/空 map 写入清空列", func() {
			c.StateValues = `{"x":"y"}`
			So(c.SetStateValues(nil), ShouldBeNil)
			So(c.StateValues, ShouldEqual, "")
		})
		Convey("空列读取返回 nil", func() {
			c.StateValues = ""
			got, err := c.GetStateValues()
			So(err, ShouldBeNil)
			So(got, ShouldBeNil)
		})
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
