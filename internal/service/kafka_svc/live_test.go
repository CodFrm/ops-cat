package kafka_svc

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kadm"
	"go.uber.org/mock/gomock"

	"github.com/opskat/opskat/internal/connpool"
	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/repository/asset_repo"
	"github.com/opskat/opskat/internal/repository/asset_repo/mock_asset_repo"
)

func TestKafkaLiveConnection(t *testing.T) {
	brokersText := strings.TrimSpace(os.Getenv("OPSKAT_KAFKA_TEST_BROKERS"))
	if brokersText == "" {
		t.Skip("set OPSKAT_KAFKA_TEST_BROKERS to run live Kafka connection test")
	}

	cfg := &asset_entity.KafkaConfig{
		Brokers:               splitBrokers(brokersText),
		SASLMechanism:         asset_entity.KafkaSASLNone,
		RequestTimeoutSeconds: 5,
	}
	svc := New(nil)
	defer svc.Close()

	if err := svc.TestConnection(context.Background(), cfg, "", 0); err != nil {
		t.Fatalf("test kafka connection: %v", err)
	}
}

func TestKafkaLiveProduceAndBrowse(t *testing.T) {
	brokersText := strings.TrimSpace(os.Getenv("OPSKAT_KAFKA_TEST_BROKERS"))
	if brokersText == "" {
		t.Skip("set OPSKAT_KAFKA_TEST_BROKERS to run live Kafka message test")
	}

	ctx := context.Background()
	cfg := &asset_entity.KafkaConfig{
		Brokers:               splitBrokers(brokersText),
		SASLMechanism:         asset_entity.KafkaSASLNone,
		RequestTimeoutSeconds: 5,
		MessageFetchLimit:     10,
		MessagePreviewBytes:   1024,
	}
	asset := &asset_entity.Asset{ID: 9001, Name: "live-kafka", Type: asset_entity.AssetTypeKafka}
	require.NoError(t, asset.SetKafkaConfig(cfg))

	mockCtrl := gomock.NewController(t)
	t.Cleanup(mockCtrl.Finish)
	mockRepo := mock_asset_repo.NewMockAssetRepo(mockCtrl)
	mockRepo.EXPECT().Find(gomock.Any(), int64(9001)).Return(asset, nil).AnyTimes()
	origRepo := asset_repo.Asset()
	asset_repo.RegisterAsset(mockRepo)
	t.Cleanup(func() {
		if origRepo != nil {
			asset_repo.RegisterAsset(origRepo)
		}
	})

	adminClient, err := connpool.DialKafka(ctx, asset, cfg, "", nil)
	require.NoError(t, err)
	defer adminClient.Close()
	admin := kadm.NewClient(adminClient)
	topic := fmt.Sprintf("opskat-live-%d", time.Now().UnixNano())
	created, err := admin.CreateTopic(ctx, 1, 1, nil, topic)
	require.NoError(t, err)
	require.NoError(t, created.Err)
	t.Cleanup(func() { _, _ = admin.DeleteTopic(context.Background(), topic) })

	svc := New(nil)
	defer svc.Close()

	partition := int32(0)
	produced, err := svc.ProduceMessage(ctx, ProduceMessageRequest{
		AssetID:   asset.ID,
		Topic:     topic,
		Partition: &partition,
		Key:       "opskat-live-key",
		Value:     "opskat-live-value",
		Headers: []ProduceMessageHeader{
			{Key: "source", Value: "opskat-live"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, topic, produced.Topic)
	assert.Equal(t, partition, produced.Partition)

	browsed, err := svc.BrowseMessages(ctx, BrowseMessagesRequest{
		AssetID:       asset.ID,
		Topic:         topic,
		Partition:     &partition,
		StartMode:     "oldest",
		Limit:         5,
		MaxBytes:      1024,
		DecodeMode:    "text",
		MaxWaitMillis: 5000,
	})
	require.NoError(t, err)
	require.NotEmpty(t, browsed.Records)
	assert.Equal(t, "opskat-live-key", browsed.Records[0].Key)
	assert.Equal(t, "opskat-live-value", browsed.Records[0].Value)
}

func TestKafkaLiveTopicAndConsumerGroupAdmin(t *testing.T) {
	brokersText := strings.TrimSpace(os.Getenv("OPSKAT_KAFKA_TEST_BROKERS"))
	if brokersText == "" {
		t.Skip("set OPSKAT_KAFKA_TEST_BROKERS to run live Kafka admin test")
	}

	ctx := context.Background()
	cfg := &asset_entity.KafkaConfig{
		Brokers:               splitBrokers(brokersText),
		SASLMechanism:         asset_entity.KafkaSASLNone,
		RequestTimeoutSeconds: 5,
		MessageFetchLimit:     10,
		MessagePreviewBytes:   1024,
	}
	asset := &asset_entity.Asset{ID: 9002, Name: "live-kafka-admin", Type: asset_entity.AssetTypeKafka}
	require.NoError(t, asset.SetKafkaConfig(cfg))

	mockCtrl := gomock.NewController(t)
	t.Cleanup(mockCtrl.Finish)
	mockRepo := mock_asset_repo.NewMockAssetRepo(mockCtrl)
	mockRepo.EXPECT().Find(gomock.Any(), int64(9002)).Return(asset, nil).AnyTimes()
	origRepo := asset_repo.Asset()
	asset_repo.RegisterAsset(mockRepo)
	t.Cleanup(func() {
		if origRepo != nil {
			asset_repo.RegisterAsset(origRepo)
		}
	})

	svc := New(nil)
	defer svc.Close()

	topic := fmt.Sprintf("opskat-admin-%d", time.Now().UnixNano())
	_, err := svc.CreateTopic(ctx, CreateTopicRequest{
		AssetID:           asset.ID,
		Topic:             topic,
		Partitions:        1,
		ReplicationFactor: 1,
		Configs:           map[string]string{"retention.ms": "600000"},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = svc.DeleteTopic(context.Background(), asset.ID, topic) })

	_, err = svc.AlterTopicConfig(ctx, AlterTopicConfigRequest{
		AssetID: asset.ID,
		Topic:   topic,
		Configs: []TopicConfigMutation{{Name: "retention.ms", Value: "601000", Op: "set"}},
	})
	require.NoError(t, err)

	_, err = svc.IncreasePartitions(ctx, IncreasePartitionsRequest{AssetID: asset.ID, Topic: topic, Partitions: 2})
	require.NoError(t, err)
	detail, err := svc.GetTopic(ctx, asset.ID, topic)
	require.NoError(t, err)
	assert.Equal(t, 2, detail.PartitionCount)

	partition := int32(0)
	produced, err := svc.ProduceMessage(ctx, ProduceMessageRequest{
		AssetID:   asset.ID,
		Topic:     topic,
		Partition: &partition,
		Value:     "delete-records-target",
	})
	require.NoError(t, err)

	deletedRecords, err := svc.DeleteRecords(ctx, DeleteRecordsRequest{
		AssetID: asset.ID,
		Topic:   topic,
		Partitions: []DeleteRecordsPartition{
			{Partition: partition, Offset: produced.Offset + 1},
		},
	})
	require.NoError(t, err)
	require.Len(t, deletedRecords.Partitions, 1)
	assert.GreaterOrEqual(t, deletedRecords.Partitions[0].LowWatermark, produced.Offset+1)

	group := fmt.Sprintf("opskat-admin-group-%d", time.Now().UnixNano())
	reset, err := svc.ResetConsumerGroupOffset(ctx, ResetConsumerGroupOffsetRequest{
		AssetID:    asset.ID,
		Group:      group,
		Topic:      topic,
		Partitions: []int32{0, 1},
		Mode:       "latest",
	})
	require.NoError(t, err)
	assert.Len(t, reset.Partitions, 2)

	deletedGroup, err := svc.DeleteConsumerGroup(ctx, asset.ID, group)
	require.NoError(t, err)
	assert.Equal(t, group, deletedGroup.Group)

	deletedTopic, err := svc.DeleteTopic(ctx, asset.ID, topic)
	require.NoError(t, err)
	assert.Equal(t, topic, deletedTopic.Topic)
}

func TestKafkaLiveACLAdmin(t *testing.T) {
	brokersText := strings.TrimSpace(os.Getenv("OPSKAT_KAFKA_TEST_BROKERS"))
	if brokersText == "" {
		t.Skip("set OPSKAT_KAFKA_TEST_BROKERS to run live Kafka ACL test")
	}

	ctx := context.Background()
	cfg := &asset_entity.KafkaConfig{
		Brokers:               splitBrokers(brokersText),
		SASLMechanism:         asset_entity.KafkaSASLNone,
		RequestTimeoutSeconds: 5,
	}
	asset := &asset_entity.Asset{ID: 9003, Name: "live-kafka-acl", Type: asset_entity.AssetTypeKafka}
	require.NoError(t, asset.SetKafkaConfig(cfg))

	mockCtrl := gomock.NewController(t)
	t.Cleanup(mockCtrl.Finish)
	mockRepo := mock_asset_repo.NewMockAssetRepo(mockCtrl)
	mockRepo.EXPECT().Find(gomock.Any(), int64(9003)).Return(asset, nil).AnyTimes()
	origRepo := asset_repo.Asset()
	asset_repo.RegisterAsset(mockRepo)
	t.Cleanup(func() {
		if origRepo != nil {
			asset_repo.RegisterAsset(origRepo)
		}
	})

	svc := New(nil)
	defer svc.Close()

	_, err := svc.ListACLs(ctx, ListACLsRequest{AssetID: asset.ID, PageSize: 10})
	if err != nil {
		skipIfKafkaACLUnavailable(t, err)
		require.NoError(t, err)
	}

	topic := fmt.Sprintf("opskat-acl-%d", time.Now().UnixNano())
	principal := "User:opskat-live"
	created, err := svc.CreateACL(ctx, CreateACLRequest{
		AssetID:      asset.ID,
		ResourceType: "topic",
		ResourceName: topic,
		PatternType:  "literal",
		Principal:    principal,
		Host:         "*",
		Operation:    "read",
		Permission:   "allow",
	})
	if err != nil {
		skipIfKafkaACLUnavailable(t, err)
		require.NoError(t, err)
	}
	require.GreaterOrEqual(t, created.Count, 1)
	t.Cleanup(func() {
		_, _ = svc.DeleteACL(context.Background(), DeleteACLRequest{
			AssetID:      asset.ID,
			ResourceType: "topic",
			ResourceName: topic,
			PatternType:  "literal",
			Principal:    principal,
			Host:         "*",
			Operation:    "read",
			Permission:   "allow",
		})
	})

	listed, err := svc.ListACLs(ctx, ListACLsRequest{
		AssetID:      asset.ID,
		ResourceType: "topic",
		ResourceName: topic,
		Principal:    principal,
		Host:         "*",
		Operation:    "read",
		Permission:   "allow",
		PageSize:     10,
	})
	require.NoError(t, err)
	require.NotEmpty(t, listed.ACLs)
	assert.Equal(t, principal, listed.ACLs[0].Principal)

	deleted, err := svc.DeleteACL(ctx, DeleteACLRequest{
		AssetID:      asset.ID,
		ResourceType: "topic",
		ResourceName: topic,
		PatternType:  "literal",
		Principal:    principal,
		Host:         "*",
		Operation:    "read",
		Permission:   "allow",
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, deleted.Count, 1)
}

func splitBrokers(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func skipIfKafkaACLUnavailable(t *testing.T, err error) {
	t.Helper()
	text := strings.ToLower(err.Error())
	for _, marker := range []string{
		"security_disabled",
		"securitydisabled",
		"security disabled",
		"authorization failed",
		"not authorized",
		"unsupported version",
	} {
		if strings.Contains(text, marker) {
			t.Skipf("Kafka ACL admin is unavailable in this live cluster: %v", err)
		}
	}
}
