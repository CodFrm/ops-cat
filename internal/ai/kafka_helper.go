package ai

import (
	"context"
	"fmt"
	"strings"

	"github.com/opskat/opskat/internal/service/kafka_svc"
)

// --- Kafka service context ---

type kafkaServiceKeyType struct{}

// WithKafkaService 将 Kafka 服务注入 context，让 AI handler 在同一次 Chat 内复用
// franz-go client（避免每次工具调用都重新 dial+ping）。
func WithKafkaService(ctx context.Context, svc *kafka_svc.Service) context.Context {
	if svc == nil {
		return ctx
	}
	return context.WithValue(ctx, kafkaServiceKeyType{}, svc)
}

func getKafkaService(ctx context.Context) *kafka_svc.Service {
	if svc, ok := ctx.Value(kafkaServiceKeyType{}).(*kafka_svc.Service); ok {
		return svc
	}
	return nil
}

// kafkaServiceFromCtx 优先返回 context 中已注入的服务（release 为 no-op），
// 缺省时按旧行为创建一次性 Service 并由调用方在 release 中关闭。
func kafkaServiceFromCtx(ctx context.Context) (*kafka_svc.Service, func()) {
	if svc := getKafkaService(ctx); svc != nil {
		return svc, func() {}
	}
	svc := kafka_svc.New(getSSHPool(ctx))
	return svc, svc.Close
}

// --- Handlers ---

func handleKafkaCluster(ctx context.Context, args map[string]any) (string, error) {
	assetID := argInt64(args, "asset_id")
	operation := normalizeKafkaOperation(argString(args, "operation"), "overview")
	if assetID == 0 {
		return "", fmt.Errorf("missing required parameter: asset_id")
	}
	if _, err := kafkaClusterCommand(operation); err != nil {
		return "", err
	}

	svc, release := kafkaServiceFromCtx(ctx)
	defer release()

	switch operation {
	case "overview":
		result, err := svc.ClusterOverview(ctx, assetID)
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(result)
	case "brokers", "list_brokers":
		brokers, err := svc.ListBrokers(ctx, assetID)
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(map[string]any{"brokers": brokers, "count": len(brokers)})
	case "get_broker_config":
		brokerID := int32(argInt64(args, "broker_id"))
		result, err := svc.GetBrokerConfig(ctx, assetID, brokerID)
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(result)
	case "list_cluster_configs":
		result, err := svc.ListClusterConfigs(ctx, assetID)
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(result)
	default:
		return "", fmt.Errorf("unsupported kafka_cluster operation: %s", operation)
	}
}

func handleKafkaTopic(ctx context.Context, args map[string]any) (string, error) {
	assetID := argInt64(args, "asset_id")
	operation := normalizeKafkaOperation(argString(args, "operation"), "list")
	if assetID == 0 {
		return "", fmt.Errorf("missing required parameter: asset_id")
	}
	if _, err := kafkaTopicCommand(operation, argString(args, "topic")); err != nil {
		return "", err
	}

	svc, release := kafkaServiceFromCtx(ctx)
	defer release()

	switch operation {
	case "list":
		req := kafka_svc.ListTopicsRequest{
			AssetID:         assetID,
			IncludeInternal: argBool(args, "include_internal"),
			Search:          strings.TrimSpace(argString(args, "search")),
			Page:            int(argInt64(args, "page")),
			PageSize:        int(argInt64(args, "page_size")),
		}
		result, err := svc.ListTopics(ctx, req)
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(result)
	case "get", "describe":
		result, err := svc.GetTopic(ctx, assetID, strings.TrimSpace(argString(args, "topic")))
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(result)
	case "create":
		req, err := kafkaCreateTopicRequestFromArgs(assetID, args)
		if err != nil {
			return "", err
		}
		result, err := svc.CreateTopic(ctx, req)
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(result)
	case "delete":
		result, err := svc.DeleteTopic(ctx, assetID, strings.TrimSpace(argString(args, "topic")))
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(result)
	case "update_config":
		req, err := kafkaAlterTopicConfigRequestFromArgs(assetID, args)
		if err != nil {
			return "", err
		}
		result, err := svc.AlterTopicConfig(ctx, req)
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(result)
	case "increase_partitions":
		req, err := kafkaIncreasePartitionsRequestFromArgs(assetID, args)
		if err != nil {
			return "", err
		}
		result, err := svc.IncreasePartitions(ctx, req)
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(result)
	case "delete_records":
		req, err := kafkaDeleteRecordsRequestFromArgs(assetID, args)
		if err != nil {
			return "", err
		}
		result, err := svc.DeleteRecords(ctx, req)
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(result)
	default:
		return "", fmt.Errorf("unsupported kafka_topic operation: %s", operation)
	}
}

func handleKafkaConsumerGroup(ctx context.Context, args map[string]any) (string, error) {
	assetID := argInt64(args, "asset_id")
	operation := normalizeKafkaOperation(argString(args, "operation"), "list")
	if assetID == 0 {
		return "", fmt.Errorf("missing required parameter: asset_id")
	}
	if _, err := kafkaConsumerGroupCommand(operation, argString(args, "group")); err != nil {
		return "", err
	}

	svc, release := kafkaServiceFromCtx(ctx)
	defer release()

	switch operation {
	case "list":
		groups, err := svc.ListConsumerGroups(ctx, assetID)
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(map[string]any{"groups": groups, "count": len(groups)})
	case "get", "describe":
		result, err := svc.GetConsumerGroup(ctx, assetID, strings.TrimSpace(argString(args, "group")))
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(result)
	case "reset_offset":
		req, err := kafkaResetConsumerGroupOffsetRequestFromArgs(assetID, args)
		if err != nil {
			return "", err
		}
		result, err := svc.ResetConsumerGroupOffset(ctx, req)
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(result)
	case "delete":
		result, err := svc.DeleteConsumerGroup(ctx, assetID, strings.TrimSpace(argString(args, "group")))
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(result)
	default:
		return "", fmt.Errorf("unsupported kafka_consumer_group operation: %s", operation)
	}
}

func handleKafkaACL(ctx context.Context, args map[string]any) (string, error) {
	assetID := argInt64(args, "asset_id")
	operation := normalizeKafkaOperation(argString(args, "operation"), "list")
	if assetID == 0 {
		return "", fmt.Errorf("missing required parameter: asset_id")
	}
	if _, err := kafkaACLCommand(operation); err != nil {
		return "", err
	}

	svc, release := kafkaServiceFromCtx(ctx)
	defer release()

	switch operation {
	case "list":
		result, err := svc.ListACLs(ctx, kafkaListACLsRequestFromArgs(assetID, args))
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(result)
	case "create":
		result, err := svc.CreateACL(ctx, kafkaCreateACLRequestFromArgs(assetID, args))
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(result)
	case "delete":
		result, err := svc.DeleteACL(ctx, kafkaDeleteACLRequestFromArgs(assetID, args))
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(result)
	default:
		return "", fmt.Errorf("unsupported kafka_acl operation: %s", operation)
	}
}

func handleKafkaSchema(ctx context.Context, args map[string]any) (string, error) {
	assetID := argInt64(args, "asset_id")
	operation := normalizeKafkaOperation(argString(args, "operation"), "list_subjects")
	if assetID == 0 {
		return "", fmt.Errorf("missing required parameter: asset_id")
	}
	if _, err := kafkaSchemaCommand(operation, argString(args, "subject")); err != nil {
		return "", err
	}

	svc, release := kafkaServiceFromCtx(ctx)
	defer release()

	switch operation {
	case "list_subjects":
		result, err := svc.ListSchemaSubjects(ctx, assetID)
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(map[string]any{"subjects": result, "count": len(result)})
	case "list_versions":
		result, err := svc.GetSchemaSubjectVersions(ctx, assetID, strings.TrimSpace(argString(args, "subject")))
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(result)
	case "get", "describe":
		result, err := svc.GetSchema(ctx, assetID, strings.TrimSpace(argString(args, "subject")), argString(args, "version"))
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(result)
	case "check_compatibility":
		req, err := kafkaCheckSchemaCompatibilityRequestFromArgs(assetID, args)
		if err != nil {
			return "", err
		}
		result, err := svc.CheckSchemaCompatibility(ctx, req)
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(result)
	case "register":
		req, err := kafkaRegisterSchemaRequestFromArgs(assetID, args)
		if err != nil {
			return "", err
		}
		result, err := svc.RegisterSchema(ctx, req)
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(result)
	case "delete":
		result, err := svc.DeleteSchema(ctx, kafkaDeleteSchemaRequestFromArgs(assetID, args))
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(result)
	default:
		return "", fmt.Errorf("unsupported kafka_schema operation: %s", operation)
	}
}

func handleKafkaConnect(ctx context.Context, args map[string]any) (string, error) {
	assetID := argInt64(args, "asset_id")
	operation := normalizeKafkaOperation(argString(args, "operation"), "list_connectors")
	if assetID == 0 {
		return "", fmt.Errorf("missing required parameter: asset_id")
	}
	if _, err := kafkaConnectCommand(operation, argString(args, "connector")); err != nil {
		return "", err
	}

	svc, release := kafkaServiceFromCtx(ctx)
	defer release()

	cluster := argString(args, "cluster")
	switch operation {
	case "list_clusters":
		result, err := svc.ListConnectClusters(ctx, assetID)
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(map[string]any{"clusters": result, "count": len(result)})
	case "list_connectors":
		result, err := svc.ListConnectors(ctx, kafka_svc.ListConnectorsRequest{AssetID: assetID, Cluster: cluster})
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(map[string]any{"connectors": result, "count": len(result)})
	case "get_connector", "get", "describe":
		result, err := svc.GetConnector(ctx, assetID, cluster, strings.TrimSpace(argString(args, "connector")))
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(result)
	case "create":
		req, err := kafkaConnectorConfigRequestFromArgs(assetID, args)
		if err != nil {
			return "", err
		}
		result, err := svc.CreateConnector(ctx, req)
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(result)
	case "update_config":
		req, err := kafkaConnectorConfigRequestFromArgs(assetID, args)
		if err != nil {
			return "", err
		}
		result, err := svc.UpdateConnectorConfig(ctx, req)
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(result)
	case "pause":
		result, err := svc.PauseConnector(ctx, assetID, cluster, strings.TrimSpace(argString(args, "connector")))
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(result)
	case "resume":
		result, err := svc.ResumeConnector(ctx, assetID, cluster, strings.TrimSpace(argString(args, "connector")))
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(result)
	case "restart":
		result, err := svc.RestartConnector(ctx, kafkaRestartConnectorRequestFromArgs(assetID, args))
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(result)
	case "delete":
		result, err := svc.DeleteConnector(ctx, assetID, cluster, strings.TrimSpace(argString(args, "connector")))
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(result)
	default:
		return "", fmt.Errorf("unsupported kafka_connect operation: %s", operation)
	}
}

func handleKafkaMessage(ctx context.Context, args map[string]any) (string, error) {
	assetID := argInt64(args, "asset_id")
	operation := normalizeKafkaOperation(argString(args, "operation"), "browse")
	topic := argString(args, "topic")
	if assetID == 0 {
		return "", fmt.Errorf("missing required parameter: asset_id")
	}
	if _, err := kafkaMessageCommand(operation, topic); err != nil {
		return "", err
	}

	svc, release := kafkaServiceFromCtx(ctx)
	defer release()

	switch operation {
	case "browse":
		req, err := kafkaBrowseRequestFromArgs(assetID, args)
		if err != nil {
			return "", err
		}
		result, err := svc.BrowseMessages(ctx, req)
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(result)
	case "inspect":
		req, err := kafkaInspectRequestFromArgs(assetID, args)
		if err != nil {
			return "", err
		}
		result, err := svc.BrowseMessages(ctx, req)
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(result)
	case "produce":
		req, err := kafkaProduceRequestFromArgs(assetID, args)
		if err != nil {
			return "", err
		}
		result, err := svc.ProduceMessage(ctx, req)
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(result)
	default:
		return "", fmt.Errorf("unsupported kafka_message operation: %s", operation)
	}
}

func normalizeKafkaOperation(operation, fallback string) string {
	operation = strings.ToLower(strings.TrimSpace(operation))
	if operation == "" {
		return fallback
	}
	return operation
}
