package ai

import (
	"testing"

	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/agents/tool"
	. "github.com/smartystreets/goconvey/convey"
)

func TestTools_RegistryShape(t *testing.T) {
	Convey("Tools 返回的工具集与既定契约一致", t, func() {
		tools := Tools()

		// spawn_agent 由 cago dispatch_subagent 取代，保持已删；batch_command 恢复，对齐 main 上的并行批量能力。
		names := make(map[string]tool.Tool, len(tools))
		for _, t := range tools {
			names[t.Name()] = t
		}

		Convey("spawn_agent 不再出现（cago dispatch_subagent 已取代）", func() {
			So(names, ShouldNotContainKey, "spawn_agent")
		})

		expected := []string{
			// asset
			"list_assets", "get_asset", "add_asset", "update_asset",
			"list_groups", "get_group", "add_group", "update_group",
			// exec
			"run_command", "upload_file", "download_file", "request_permission", "batch_command",
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

		Convey("Schema 结构合法（type=object，必填字段存在）", func() {
			for _, t := range names {
				schema := t.Schema()
				So(schema.Type, ShouldEqual, "object")
				for _, req := range schema.Required {
					So(schema.Properties, ShouldContainKey, req)
				}
			}
		})
	})
}
