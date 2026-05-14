package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/service/kafka_svc"
	"github.com/opskat/opskat/internal/service/testreg"
)

func (a *App) kafkaSvc() *kafka_svc.Service {
	if a.kafkaService == nil {
		a.kafkaService = kafka_svc.New(a.sshPool)
	}
	return a.kafkaService
}

// TestKafkaConnection 测试 Kafka 连接
// testID: 前端生成的本次测试唯一标识，用于配合 CancelTest 中断
// configJSON: KafkaConfig JSON，plainPassword: 明文密码
func (a *App) TestKafkaConnection(testID string, configJSON string, plainPassword string) error {
	var cfg asset_entity.KafkaConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return fmt.Errorf("配置解析失败: %w", err)
	}
	parent, parentCancel := context.WithTimeout(a.langCtx(), 10*time.Second)
	defer parentCancel()
	ctx, release := testreg.Begin(parent, testID)
	defer release()
	return a.kafkaSvc().TestConnection(ctx, &cfg, plainPassword, 0)
}

func (a *App) KafkaClusterOverview(assetID int64) (kafka_svc.ClusterOverview, error) {
	return a.kafkaSvc().ClusterOverview(a.langCtx(), assetID)
}

func (a *App) KafkaListBrokers(assetID int64) ([]kafka_svc.Broker, error) {
	return a.kafkaSvc().ListBrokers(a.langCtx(), assetID)
}

func (a *App) KafkaListTopics(req kafka_svc.ListTopicsRequest) (kafka_svc.ListTopicsResponse, error) {
	return a.kafkaSvc().ListTopics(a.langCtx(), req)
}

func (a *App) KafkaGetTopic(assetID int64, topic string) (kafka_svc.TopicDetail, error) {
	return a.kafkaSvc().GetTopic(a.langCtx(), assetID, topic)
}

func (a *App) KafkaListConsumerGroups(assetID int64) ([]kafka_svc.ConsumerGroup, error) {
	return a.kafkaSvc().ListConsumerGroups(a.langCtx(), assetID)
}

func (a *App) KafkaGetConsumerGroup(assetID int64, group string) (kafka_svc.ConsumerGroupDetail, error) {
	return a.kafkaSvc().GetConsumerGroup(a.langCtx(), assetID, group)
}

func (a *App) KafkaBrowseMessages(req kafka_svc.BrowseMessagesRequest) (kafka_svc.BrowseMessagesResponse, error) {
	return a.kafkaSvc().BrowseMessages(a.langCtx(), req)
}

func (a *App) KafkaProduceMessage(req kafka_svc.ProduceMessageRequest) (kafka_svc.ProduceMessageResponse, error) {
	return a.kafkaSvc().ProduceMessage(a.langCtx(), req)
}

func (a *App) KafkaCreateTopic(req kafka_svc.CreateTopicRequest) (kafka_svc.TopicOperationResponse, error) {
	return a.kafkaSvc().CreateTopic(a.langCtx(), req)
}

func (a *App) KafkaDeleteTopic(assetID int64, topic string) (kafka_svc.TopicOperationResponse, error) {
	return a.kafkaSvc().DeleteTopic(a.langCtx(), assetID, topic)
}

func (a *App) KafkaAlterTopicConfig(req kafka_svc.AlterTopicConfigRequest) (kafka_svc.TopicOperationResponse, error) {
	return a.kafkaSvc().AlterTopicConfig(a.langCtx(), req)
}

func (a *App) KafkaIncreasePartitions(req kafka_svc.IncreasePartitionsRequest) (kafka_svc.TopicOperationResponse, error) {
	return a.kafkaSvc().IncreasePartitions(a.langCtx(), req)
}

func (a *App) KafkaDeleteRecords(req kafka_svc.DeleteRecordsRequest) (kafka_svc.DeleteRecordsResponse, error) {
	return a.kafkaSvc().DeleteRecords(a.langCtx(), req)
}

func (a *App) KafkaGetBrokerConfig(assetID int64, brokerID int32) (kafka_svc.BrokerConfigResponse, error) {
	return a.kafkaSvc().GetBrokerConfig(a.langCtx(), assetID, brokerID)
}

func (a *App) KafkaListClusterConfigs(assetID int64) (kafka_svc.ClusterConfigsResponse, error) {
	return a.kafkaSvc().ListClusterConfigs(a.langCtx(), assetID)
}

func (a *App) KafkaResetConsumerGroupOffset(req kafka_svc.ResetConsumerGroupOffsetRequest) (kafka_svc.ResetConsumerGroupOffsetResponse, error) {
	return a.kafkaSvc().ResetConsumerGroupOffset(a.langCtx(), req)
}

func (a *App) KafkaDeleteConsumerGroup(assetID int64, group string) (kafka_svc.DeleteConsumerGroupResponse, error) {
	return a.kafkaSvc().DeleteConsumerGroup(a.langCtx(), assetID, group)
}

func (a *App) KafkaListACLs(req kafka_svc.ListACLsRequest) (kafka_svc.ListACLsResponse, error) {
	return a.kafkaSvc().ListACLs(a.langCtx(), req)
}

func (a *App) KafkaCreateACL(req kafka_svc.CreateACLRequest) (kafka_svc.ACLMutationResponse, error) {
	return a.kafkaSvc().CreateACL(a.langCtx(), req)
}

func (a *App) KafkaDeleteACL(req kafka_svc.DeleteACLRequest) (kafka_svc.ACLMutationResponse, error) {
	return a.kafkaSvc().DeleteACL(a.langCtx(), req)
}

func (a *App) KafkaListSchemaSubjects(assetID int64) ([]string, error) {
	return a.kafkaSvc().ListSchemaSubjects(a.langCtx(), assetID)
}

func (a *App) KafkaGetSchemaSubjectVersions(assetID int64, subject string) (kafka_svc.SchemaSubjectVersions, error) {
	return a.kafkaSvc().GetSchemaSubjectVersions(a.langCtx(), assetID, subject)
}

func (a *App) KafkaGetSchema(assetID int64, subject string, version string) (kafka_svc.SchemaVersionDetail, error) {
	return a.kafkaSvc().GetSchema(a.langCtx(), assetID, subject, version)
}

func (a *App) KafkaCheckSchemaCompatibility(req kafka_svc.CheckSchemaCompatibilityRequest) (kafka_svc.CheckSchemaCompatibilityResponse, error) {
	return a.kafkaSvc().CheckSchemaCompatibility(a.langCtx(), req)
}

func (a *App) KafkaRegisterSchema(req kafka_svc.RegisterSchemaRequest) (kafka_svc.RegisterSchemaResponse, error) {
	return a.kafkaSvc().RegisterSchema(a.langCtx(), req)
}

func (a *App) KafkaDeleteSchema(req kafka_svc.DeleteSchemaRequest) (kafka_svc.DeleteSchemaResponse, error) {
	return a.kafkaSvc().DeleteSchema(a.langCtx(), req)
}

func (a *App) KafkaListConnectClusters(assetID int64) ([]kafka_svc.KafkaConnectCluster, error) {
	return a.kafkaSvc().ListConnectClusters(a.langCtx(), assetID)
}

func (a *App) KafkaListConnectors(req kafka_svc.ListConnectorsRequest) ([]kafka_svc.KafkaConnectorSummary, error) {
	return a.kafkaSvc().ListConnectors(a.langCtx(), req)
}

func (a *App) KafkaGetConnector(assetID int64, cluster string, name string) (kafka_svc.KafkaConnectorDetail, error) {
	return a.kafkaSvc().GetConnector(a.langCtx(), assetID, cluster, name)
}

func (a *App) KafkaCreateConnector(req kafka_svc.ConnectorConfigRequest) (kafka_svc.ConnectorOperationResponse, error) {
	return a.kafkaSvc().CreateConnector(a.langCtx(), req)
}

func (a *App) KafkaUpdateConnectorConfig(req kafka_svc.ConnectorConfigRequest) (kafka_svc.ConnectorOperationResponse, error) {
	return a.kafkaSvc().UpdateConnectorConfig(a.langCtx(), req)
}

func (a *App) KafkaPauseConnector(assetID int64, cluster string, name string) (kafka_svc.ConnectorOperationResponse, error) {
	return a.kafkaSvc().PauseConnector(a.langCtx(), assetID, cluster, name)
}

func (a *App) KafkaResumeConnector(assetID int64, cluster string, name string) (kafka_svc.ConnectorOperationResponse, error) {
	return a.kafkaSvc().ResumeConnector(a.langCtx(), assetID, cluster, name)
}

func (a *App) KafkaRestartConnector(req kafka_svc.RestartConnectorRequest) (kafka_svc.ConnectorOperationResponse, error) {
	return a.kafkaSvc().RestartConnector(a.langCtx(), req)
}

func (a *App) KafkaDeleteConnector(assetID int64, cluster string, name string) (kafka_svc.ConnectorOperationResponse, error) {
	return a.kafkaSvc().DeleteConnector(a.langCtx(), assetID, cluster, name)
}
