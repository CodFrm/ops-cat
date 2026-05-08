package ai

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKafkaToolCommandMapping(t *testing.T) {
	cmd, err := kafkaClusterCommand("overview")
	require.NoError(t, err)
	assert.Equal(t, "cluster.read *", cmd)

	cmd, err = kafkaClusterCommand("brokers")
	require.NoError(t, err)
	assert.Equal(t, "broker.read *", cmd)

	cmd, err = kafkaTopicCommand("list", "")
	require.NoError(t, err)
	assert.Equal(t, "topic.list *", cmd)

	cmd, err = kafkaTopicCommand("get", "orders")
	require.NoError(t, err)
	assert.Equal(t, "topic.read orders", cmd)

	cmd, err = kafkaTopicCommand("create", "orders")
	require.NoError(t, err)
	assert.Equal(t, "topic.create orders", cmd)

	cmd, err = kafkaTopicCommand("delete", "orders")
	require.NoError(t, err)
	assert.Equal(t, "topic.delete orders", cmd)

	cmd, err = kafkaTopicCommand("update_config", "orders")
	require.NoError(t, err)
	assert.Equal(t, "topic.config.write orders", cmd)

	cmd, err = kafkaTopicCommand("increase_partitions", "orders")
	require.NoError(t, err)
	assert.Equal(t, "topic.partitions.write orders", cmd)

	cmd, err = kafkaTopicCommand("delete_records", "orders")
	require.NoError(t, err)
	assert.Equal(t, "topic.records.delete orders", cmd)

	cmd, err = kafkaConsumerGroupCommand("get", "billing-worker")
	require.NoError(t, err)
	assert.Equal(t, "consumer_group.read billing-worker", cmd)

	cmd, err = kafkaConsumerGroupCommand("reset_offset", "billing-worker")
	require.NoError(t, err)
	assert.Equal(t, "consumer_group.offset.write billing-worker", cmd)

	cmd, err = kafkaConsumerGroupCommand("delete", "billing-worker")
	require.NoError(t, err)
	assert.Equal(t, "consumer_group.delete billing-worker", cmd)

	cmd, err = kafkaACLCommand("list")
	require.NoError(t, err)
	assert.Equal(t, "acl.read *", cmd)

	cmd, err = kafkaACLCommand("create")
	require.NoError(t, err)
	assert.Equal(t, "acl.write *", cmd)

	cmd, err = kafkaACLCommand("delete")
	require.NoError(t, err)
	assert.Equal(t, "acl.write *", cmd)

	cmd, err = kafkaSchemaCommand("list_subjects", "")
	require.NoError(t, err)
	assert.Equal(t, "schema.read *", cmd)

	cmd, err = kafkaSchemaCommand("get", "orders-value")
	require.NoError(t, err)
	assert.Equal(t, "schema.read orders-value", cmd)

	cmd, err = kafkaSchemaCommand("check_compatibility", "orders-value")
	require.NoError(t, err)
	assert.Equal(t, "schema.read orders-value", cmd)

	cmd, err = kafkaSchemaCommand("register", "orders-value")
	require.NoError(t, err)
	assert.Equal(t, "schema.write orders-value", cmd)

	cmd, err = kafkaSchemaCommand("delete", "orders-value")
	require.NoError(t, err)
	assert.Equal(t, "schema.delete orders-value", cmd)

	cmd, err = kafkaConnectCommand("list_connectors", "")
	require.NoError(t, err)
	assert.Equal(t, "connect.read *", cmd)

	cmd, err = kafkaConnectCommand("get_connector", "sink-orders")
	require.NoError(t, err)
	assert.Equal(t, "connect.read sink-orders", cmd)

	cmd, err = kafkaConnectCommand("create", "sink-orders")
	require.NoError(t, err)
	assert.Equal(t, "connect.write sink-orders", cmd)

	cmd, err = kafkaConnectCommand("pause", "sink-orders")
	require.NoError(t, err)
	assert.Equal(t, "connect.state.write sink-orders", cmd)

	cmd, err = kafkaConnectCommand("delete", "sink-orders")
	require.NoError(t, err)
	assert.Equal(t, "connect.delete sink-orders", cmd)

	cmd, err = kafkaMessageCommand("browse", "orders")
	require.NoError(t, err)
	assert.Equal(t, "message.read orders", cmd)

	cmd, err = kafkaMessageCommand("inspect", "orders")
	require.NoError(t, err)
	assert.Equal(t, "message.read orders", cmd)

	cmd, err = kafkaMessageCommand("produce", "orders")
	require.NoError(t, err)
	assert.Equal(t, "message.write orders", cmd)

	_, err = kafkaTopicCommand("get", "")
	assert.Error(t, err)

	_, err = kafkaMessageCommand("browse", "")
	assert.Error(t, err)

	_, err = kafkaMessageCommand("delete", "orders")
	assert.Error(t, err)

	_, err = kafkaACLCommand("grant")
	assert.Error(t, err)

	_, err = kafkaSchemaCommand("get", "")
	assert.Error(t, err)

	_, err = kafkaConnectCommand("get_connector", "")
	assert.Error(t, err)
}

func TestAllToolDefsContainsGroupedKafkaTools(t *testing.T) {
	tools := map[string]ToolDef{}
	for _, def := range AllToolDefs() {
		tools[def.Name] = def
	}

	assert.Contains(t, tools, "kafka_cluster")
	assert.Contains(t, tools, "kafka_topic")
	assert.Contains(t, tools, "kafka_consumer_group")
	assert.Contains(t, tools, "kafka_acl")
	assert.Contains(t, tools, "kafka_schema")
	assert.Contains(t, tools, "kafka_connect")
	assert.Contains(t, tools, "kafka_message")
	assert.NotContains(t, tools, "kafka_topic_delete")

	cmd := tools["kafka_message"].CommandExtractor(map[string]any{
		"operation": "produce",
		"topic":     "orders",
	})
	assert.Equal(t, "message.write orders", cmd)

	cmd = tools["kafka_topic"].CommandExtractor(map[string]any{
		"operation": "delete_records",
		"topic":     "orders",
	})
	assert.Equal(t, "topic.records.delete orders", cmd)

	cmd = tools["kafka_consumer_group"].CommandExtractor(map[string]any{
		"operation": "reset_offset",
		"group":     "billing-worker",
	})
	assert.Equal(t, "consumer_group.offset.write billing-worker", cmd)

	cmd = tools["kafka_acl"].CommandExtractor(map[string]any{
		"operation": "create",
	})
	assert.Equal(t, "acl.write *", cmd)

	cmd = tools["kafka_schema"].CommandExtractor(map[string]any{
		"operation": "register",
		"subject":   "orders-value",
	})
	assert.Equal(t, "schema.write orders-value", cmd)

	cmd = tools["kafka_connect"].CommandExtractor(map[string]any{
		"operation": "restart",
		"connector": "sink-orders",
	})
	assert.Equal(t, "connect.state.write sink-orders", cmd)
}

func TestKafkaMessageArgs(t *testing.T) {
	partition, err := argOptionalInt32(map[string]any{"partition": float64(2)}, "partition")
	require.NoError(t, err)
	require.NotNil(t, partition)
	assert.Equal(t, int32(2), *partition)

	headers, err := kafkaProduceHeadersFromArgs(map[string]any{
		"headers": `[{"key":"trace","value":"abc","encoding":"text"}]`,
	})
	require.NoError(t, err)
	require.Len(t, headers, 1)
	assert.Equal(t, "trace", headers[0].Key)

	_, err = kafkaProduceHeadersFromArgs(map[string]any{"headers": `{"key":"trace"}`})
	assert.Error(t, err)
}

func TestKafkaTopicAdminArgs(t *testing.T) {
	createReq, err := kafkaCreateTopicRequestFromArgs(7, map[string]any{
		"topic":              "orders",
		"partitions":         float64(3),
		"replication_factor": float64(1),
		"configs":            `{"cleanup.policy":"compact"}`,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(7), createReq.AssetID)
	assert.Equal(t, int32(3), createReq.Partitions)
	assert.Equal(t, int16(1), createReq.ReplicationFactor)
	assert.Equal(t, "compact", createReq.Configs["cleanup.policy"])

	updateReq, err := kafkaAlterTopicConfigRequestFromArgs(7, map[string]any{
		"topic":          "orders",
		"config_updates": `[{"name":"retention.ms","value":"60000","op":"set"}]`,
	})
	require.NoError(t, err)
	require.Len(t, updateReq.Configs, 1)
	assert.Equal(t, "retention.ms", updateReq.Configs[0].Name)

	recordsReq, err := kafkaDeleteRecordsRequestFromArgs(7, map[string]any{
		"topic":   "orders",
		"records": `[{"partition":0,"offset":123}]`,
	})
	require.NoError(t, err)
	require.Len(t, recordsReq.Partitions, 1)
	assert.Equal(t, int32(0), recordsReq.Partitions[0].Partition)
	assert.Equal(t, int64(123), recordsReq.Partitions[0].Offset)

	_, err = kafkaStringMapFromJSON(`[{"bad":true}]`)
	assert.Error(t, err)

	_, err = kafkaAlterTopicConfigRequestFromArgs(7, map[string]any{"topic": "orders"})
	assert.Error(t, err)

	_, err = kafkaDeleteRecordsRequestFromArgs(7, map[string]any{"topic": "orders", "records": `{"partition":0}`})
	assert.Error(t, err)
}

func TestKafkaConsumerGroupAdminArgs(t *testing.T) {
	req, err := kafkaResetConsumerGroupOffsetRequestFromArgs(7, map[string]any{
		"group":      "billing",
		"topic":      "orders",
		"mode":       "offset",
		"offset":     float64(123),
		"partitions": `[0,1]`,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(7), req.AssetID)
	assert.Equal(t, "billing", req.Group)
	assert.Equal(t, "orders", req.Topic)
	assert.Equal(t, int64(123), req.Offset)
	assert.Equal(t, []int32{0, 1}, req.Partitions)

	partitions, err := kafkaInt32SliceFromJSON(`[2,3]`)
	require.NoError(t, err)
	assert.Equal(t, []int32{2, 3}, partitions)

	_, err = kafkaInt32SliceFromJSON(`{"bad":true}`)
	assert.Error(t, err)
}

func TestKafkaACLArgs(t *testing.T) {
	listReq := kafkaListACLsRequestFromArgs(7, map[string]any{
		"resource_type": "topic",
		"resource_name": "orders",
		"pattern_type":  "match",
		"principal":     "User:alice",
		"host":          "*",
		"acl_operation": "read",
		"permission":    "allow",
		"page":          float64(2),
		"page_size":     float64(10),
	})
	assert.Equal(t, int64(7), listReq.AssetID)
	assert.Equal(t, "topic", listReq.ResourceType)
	assert.Equal(t, "orders", listReq.ResourceName)
	assert.Equal(t, "read", listReq.Operation)
	assert.Equal(t, 2, listReq.Page)
	assert.Equal(t, 10, listReq.PageSize)

	createReq := kafkaCreateACLRequestFromArgs(7, map[string]any{
		"resource_type": "group",
		"resource_name": "billing",
		"principal":     "User:alice",
		"host":          "*",
		"acl_operation": "read",
		"permission":    "deny",
	})
	assert.Equal(t, "group", createReq.ResourceType)
	assert.Equal(t, "deny", createReq.Permission)

	deleteReq := kafkaDeleteACLRequestFromArgs(7, map[string]any{
		"resource_type": "cluster",
		"principal":     "User:admin",
		"host":          "*",
		"acl_operation": "describe",
		"permission":    "allow",
	})
	assert.Equal(t, "cluster", deleteReq.ResourceType)
	assert.Equal(t, "describe", deleteReq.Operation)
}

func TestKafkaSchemaArgs(t *testing.T) {
	registerReq, err := kafkaRegisterSchemaRequestFromArgs(7, map[string]any{
		"subject":     "orders-value",
		"schema":      `{"type":"record","name":"Order","fields":[]}`,
		"schema_type": "AVRO",
		"references":  `[{"name":"Common","subject":"common-value","version":1}]`,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(7), registerReq.AssetID)
	assert.Equal(t, "orders-value", registerReq.Subject)
	assert.Equal(t, "AVRO", registerReq.SchemaType)
	require.Len(t, registerReq.References, 1)
	assert.Equal(t, "common-value", registerReq.References[0].Subject)

	checkReq, err := kafkaCheckSchemaCompatibilityRequestFromArgs(7, map[string]any{
		"subject": "orders-value",
		"version": "latest",
		"schema":  `{"type":"record","name":"Order","fields":[]}`,
	})
	require.NoError(t, err)
	assert.Equal(t, "latest", checkReq.Version)

	deleteReq := kafkaDeleteSchemaRequestFromArgs(7, map[string]any{
		"subject":   "orders-value",
		"version":   "2",
		"permanent": "true",
	})
	assert.True(t, deleteReq.Permanent)
	assert.Equal(t, "2", deleteReq.Version)

	_, err = kafkaSchemaReferencesFromJSON(`{"bad":true}`)
	assert.Error(t, err)
}

func TestKafkaConnectArgs(t *testing.T) {
	configReq, err := kafkaConnectorConfigRequestFromArgs(7, map[string]any{
		"cluster":   "local",
		"connector": "sink-orders",
		"config":    `{"connector.class":"FileStreamSink"}`,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(7), configReq.AssetID)
	assert.Equal(t, "local", configReq.Cluster)
	assert.Equal(t, "sink-orders", configReq.Name)
	assert.Equal(t, "FileStreamSink", configReq.Config["connector.class"])

	restartReq := kafkaRestartConnectorRequestFromArgs(7, map[string]any{
		"cluster":       "local",
		"connector":     "sink-orders",
		"include_tasks": "true",
		"only_failed":   "true",
	})
	assert.True(t, restartReq.IncludeTasks)
	assert.True(t, restartReq.OnlyFailed)

	_, err = kafkaConnectorConfigRequestFromArgs(7, map[string]any{"config": `[]`})
	assert.Error(t, err)
}

// 之前这里有 6 个 TestKafkaXxxPermissionStopsBeforeConnection，验证 in-handler
// checkKafkaToolPermission 在策略 deny 时会在连 kafka 前返回。
//
// 那条 in-handler 防御已经从架构里删了：cago 迁移之后 PreToolUse aiagent.policyHook
// 负责所有 ops tool 的 deny gate（走 ai.CheckPermission + gw.RequestSingle，convID
// 走 emitter 闭包）；in-handler 再 check 会触发 legacy makeCommandConfirmFunc 弹第二张
// 审批卡。
//
// 策略分类本身（DenyList / AllowList / NeedConfirm 判定）由 kafka_policy_test.go 的
// TestCheckKafkaPolicy 直接测；端到端 "deny 立即停" 由 aiagent.TestPolicyHook_DenyShortCircuits
// 看住。所以这几个 wrapper 删掉，避免「测试已删除的代码层」。
