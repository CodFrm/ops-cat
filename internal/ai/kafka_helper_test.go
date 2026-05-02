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
}

func TestAllToolDefsContainsGroupedKafkaTools(t *testing.T) {
	tools := map[string]ToolDef{}
	for _, def := range AllToolDefs() {
		tools[def.Name] = def
	}

	assert.Contains(t, tools, "kafka_cluster")
	assert.Contains(t, tools, "kafka_topic")
	assert.Contains(t, tools, "kafka_consumer_group")
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
