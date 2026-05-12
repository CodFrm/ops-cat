package conversation_entity

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMessageBlocksRoundtrip(t *testing.T) {
	Convey("SetBlocks/GetBlocks 往返", t, func() {
		msg := &Message{}
		blocks := []ContentBlock{
			{Type: "text", Content: "hi"},
			{Type: "tool", ToolName: "run_command", ToolCallID: "call_1", Status: "completed"},
		}

		Convey("非空写入后能读回", func() {
			So(msg.SetBlocks(blocks), ShouldBeNil)
			got, err := msg.GetBlocks()
			So(err, ShouldBeNil)
			So(got, ShouldResemble, blocks)
		})

		Convey("空数组写入后 Blocks 列为空字符串", func() {
			So(msg.SetBlocks(nil), ShouldBeNil)
			So(msg.Blocks, ShouldEqual, "")
			got, err := msg.GetBlocks()
			So(err, ShouldBeNil)
			So(got, ShouldBeNil)
		})
	})
}
