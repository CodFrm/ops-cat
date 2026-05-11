package ai

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/agents/tool"
	. "github.com/smartystreets/goconvey/convey"
)

func TestCagoTools_RegistryShape(t *testing.T) {
	Convey("CagoTools 返回的工具集与既定契约一致", t, func() {
		tools := CagoTools()

		// 不应包含已经删除的 spawn_agent / batch_command（由 cago dispatch_subagent 取代）
		names := make(map[string]tool.Tool, len(tools))
		for _, t := range tools {
			names[t.Name()] = t
		}

		Convey("已删工具不再出现", func() {
			So(names, ShouldNotContainKey, "spawn_agent")
			So(names, ShouldNotContainKey, "batch_command")
		})

		expected := []string{
			// asset
			"list_assets", "get_asset", "add_asset", "update_asset",
			"list_groups", "get_group", "add_group", "update_group",
			// exec
			"run_command", "upload_file", "download_file", "request_permission",
			// data
			"exec_sql", "exec_redis", "exec_mongo", "exec_k8s",
			// kafka
			"kafka_cluster", "kafka_topic", "kafka_consumer_group", "kafka_acl",
			"kafka_schema", "kafka_connect", "kafka_message",
			// extension
			"exec_tool",
		}

		Convey("所有契约里的工具都注册了", func() {
			for _, name := range expected {
				So(names, ShouldContainKey, name)
			}
		})

		Convey("命令类工具标 Serial", func() {
			serialNames := []string{
				"run_command", "upload_file", "download_file", "request_permission",
				"exec_sql", "exec_redis", "exec_mongo", "exec_k8s",
				"exec_tool",
				"kafka_cluster", "kafka_topic", "kafka_consumer_group", "kafka_acl",
				"kafka_schema", "kafka_connect", "kafka_message",
			}
			for _, name := range serialNames {
				st, ok := names[name].(agent.SerialTool)
				So(ok, ShouldBeTrue)
				So(st.Serial(), ShouldBeTrue)
			}
		})

		Convey("Schema 结构合法", func() {
			for name, t := range names {
				schema := t.Schema()
				So(schema.Type, ShouldEqual, "object")
				if len(schema.Properties) > 0 {
					var props map[string]any
					err := json.Unmarshal(schema.Properties, &props)
					So(err, ShouldBeNil)
					_ = name // 仅用于失败时的 SoMsg 上下文
				}
			}
		})
	})
}

func TestTextResult_PacksErrorAsIsError(t *testing.T) {
	Convey("textResult 把 error 包成 IsError=true，把 string 包成单 TextBlock", t, func() {
		blk, err := textResult("hello", nil)
		So(err, ShouldBeNil)
		So(blk.IsError, ShouldBeFalse)
		So(blk.Content, ShouldHaveLength, 1)
		txt, ok := blk.Content[0].(*agent.TextBlock)
		So(ok, ShouldBeTrue)
		So(txt.Text, ShouldEqual, "hello")

		blk2, err := textResult("", errors.New("boom"))
		So(err, ShouldBeNil)
		So(blk2.IsError, ShouldBeTrue)
		txt2, ok := blk2.Content[0].(*agent.TextBlock)
		So(ok, ShouldBeTrue)
		So(txt2.Text, ShouldEqual, "boom")
	})
}
