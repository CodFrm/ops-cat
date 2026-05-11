package ai

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
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

	// 命令摘要提取器在 M6 后改成包级静态表（commandExtractors），
	// 不再挂在 ToolDef 字段上。这里直接走 ExtractCommandForAudit 验证语义未漂移。
	assert.Equal(t, "message.write orders", ExtractCommandForAudit("kafka_message", map[string]any{
		"operation": "produce",
		"topic":     "orders",
	}))
	assert.Equal(t, "topic.records.delete orders", ExtractCommandForAudit("kafka_topic", map[string]any{
		"operation": "delete_records",
		"topic":     "orders",
	}))
	assert.Equal(t, "consumer_group.offset.write billing-worker", ExtractCommandForAudit("kafka_consumer_group", map[string]any{
		"operation": "reset_offset",
		"group":     "billing-worker",
	}))
	assert.Equal(t, "acl.write *", ExtractCommandForAudit("kafka_acl", map[string]any{
		"operation": "create",
	}))
	assert.Equal(t, "schema.write orders-value", ExtractCommandForAudit("kafka_schema", map[string]any{
		"operation": "register",
		"subject":   "orders-value",
	}))
	assert.Equal(t, "connect.state.write sink-orders", ExtractCommandForAudit("kafka_connect", map[string]any{
		"operation": "restart",
		"connector": "sink-orders",
	}))
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

func TestKafkaMessagePermissionStopsBeforeConnection(t *testing.T) {
	ctx, mockAsset, _ := setupPolicyTest(t)
	asset := &asset_entity.Asset{
		ID:   1,
		Name: "kafka-prod",
		Type: asset_entity.AssetTypeKafka,
		CmdPolicy: mustJSON(asset_entity.KafkaPolicy{
			DenyList: []string{"message.write *"},
		}),
	}
	mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

	ctx = WithPolicyChecker(ctx, NewCommandPolicyChecker(nil))
	result, err := handleKafkaMessage(ctx, map[string]any{
		"asset_id":  float64(1),
		"operation": "produce",
		"topic":     "orders",
		"value":     "hello",
	})
	require.NoError(t, err)
	assert.Contains(t, result, "Kafka")
}

func TestKafkaACLPermissionStopsBeforeConnection(t *testing.T) {
	ctx, mockAsset, _ := setupPolicyTest(t)
	asset := &asset_entity.Asset{
		ID:   1,
		Name: "kafka-prod",
		Type: asset_entity.AssetTypeKafka,
		CmdPolicy: mustJSON(asset_entity.KafkaPolicy{
			DenyList: []string{"acl.write *"},
		}),
	}
	mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

	ctx = WithPolicyChecker(ctx, NewCommandPolicyChecker(nil))
	result, err := handleKafkaACL(ctx, map[string]any{
		"asset_id":      float64(1),
		"operation":     "create",
		"resource_type": "topic",
		"resource_name": "orders",
		"principal":     "User:alice",
		"host":          "*",
		"acl_operation": "read",
		"permission":    "allow",
	})
	require.NoError(t, err)
	assert.Contains(t, result, "Kafka")
}

func TestKafkaSchemaPermissionStopsBeforeConnection(t *testing.T) {
	ctx, mockAsset, _ := setupPolicyTest(t)
	asset := &asset_entity.Asset{
		ID:   1,
		Name: "kafka-prod",
		Type: asset_entity.AssetTypeKafka,
		CmdPolicy: mustJSON(asset_entity.KafkaPolicy{
			DenyList: []string{"schema.write *"},
		}),
	}
	mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

	ctx = WithPolicyChecker(ctx, NewCommandPolicyChecker(nil))
	result, err := handleKafkaSchema(ctx, map[string]any{
		"asset_id":      float64(1),
		"operation":     "register",
		"subject":       "orders-value",
		"schema":        `{"type":"record","name":"Order","fields":[]}`,
		"schema_type":   "AVRO",
		"compatibility": "FULL",
	})
	require.NoError(t, err)
	assert.Contains(t, result, "Kafka")
}

func TestKafkaConnectPermissionStopsBeforeConnection(t *testing.T) {
	ctx, mockAsset, _ := setupPolicyTest(t)
	asset := &asset_entity.Asset{
		ID:   1,
		Name: "kafka-prod",
		Type: asset_entity.AssetTypeKafka,
		CmdPolicy: mustJSON(asset_entity.KafkaPolicy{
			DenyList: []string{"connect.state.write *"},
		}),
	}
	mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

	ctx = WithPolicyChecker(ctx, NewCommandPolicyChecker(nil))
	result, err := handleKafkaConnect(ctx, map[string]any{
		"asset_id":  float64(1),
		"operation": "restart",
		"cluster":   "local",
		"connector": "sink-orders",
	})
	require.NoError(t, err)
	assert.Contains(t, result, "Kafka")
}

func TestKafkaTopicAdminPermissionStopsBeforeConnection(t *testing.T) {
	ctx, mockAsset, _ := setupPolicyTest(t)
	asset := &asset_entity.Asset{
		ID:   1,
		Name: "kafka-prod",
		Type: asset_entity.AssetTypeKafka,
		CmdPolicy: mustJSON(asset_entity.KafkaPolicy{
			DenyList: []string{"topic.delete *"},
		}),
	}
	mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

	ctx = WithPolicyChecker(ctx, NewCommandPolicyChecker(nil))
	result, err := handleKafkaTopic(ctx, map[string]any{
		"asset_id":  float64(1),
		"operation": "delete",
		"topic":     "orders",
	})
	require.NoError(t, err)
	assert.Contains(t, result, "Kafka")
}

func TestKafkaConsumerGroupAdminPermissionStopsBeforeConnection(t *testing.T) {
	ctx, mockAsset, _ := setupPolicyTest(t)
	asset := &asset_entity.Asset{
		ID:   1,
		Name: "kafka-prod",
		Type: asset_entity.AssetTypeKafka,
		CmdPolicy: mustJSON(asset_entity.KafkaPolicy{
			DenyList: []string{"consumer_group.offset.write *"},
		}),
	}
	mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

	ctx = WithPolicyChecker(ctx, NewCommandPolicyChecker(nil))
	result, err := handleKafkaConsumerGroup(ctx, map[string]any{
		"asset_id":  float64(1),
		"operation": "reset_offset",
		"group":     "billing",
		"topic":     "orders",
		"mode":      "latest",
	})
	require.NoError(t, err)
	assert.Contains(t, result, "Kafka")
}
