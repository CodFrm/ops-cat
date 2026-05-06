package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/service/kafka_svc"
)

// --- Handlers ---

func handleKafkaCluster(ctx context.Context, args map[string]any) (string, error) {
	assetID := argInt64(args, "asset_id")
	operation := normalizeKafkaOperation(argString(args, "operation"), "overview")
	if assetID == 0 {
		return "", fmt.Errorf("missing required parameter: asset_id")
	}
	command, err := kafkaClusterCommand(operation)
	if err != nil {
		return "", err
	}
	if result, ok := checkKafkaToolPermission(ctx, assetID, command); !ok {
		return result.Message, nil
	}

	svc := kafka_svc.New(getSSHPool(ctx))
	defer svc.Close()

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
	command, err := kafkaTopicCommand(operation, argString(args, "topic"))
	if err != nil {
		return "", err
	}
	if result, ok := checkKafkaToolPermission(ctx, assetID, command); !ok {
		return result.Message, nil
	}

	svc := kafka_svc.New(getSSHPool(ctx))
	defer svc.Close()

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
	command, err := kafkaConsumerGroupCommand(operation, argString(args, "group"))
	if err != nil {
		return "", err
	}
	if result, ok := checkKafkaToolPermission(ctx, assetID, command); !ok {
		return result.Message, nil
	}

	svc := kafka_svc.New(getSSHPool(ctx))
	defer svc.Close()

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
	command, err := kafkaACLCommand(operation)
	if err != nil {
		return "", err
	}
	if result, ok := checkKafkaToolPermission(ctx, assetID, command); !ok {
		return result.Message, nil
	}

	svc := kafka_svc.New(getSSHPool(ctx))
	defer svc.Close()

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
	command, err := kafkaSchemaCommand(operation, argString(args, "subject"))
	if err != nil {
		return "", err
	}
	if result, ok := checkKafkaToolPermission(ctx, assetID, command); !ok {
		return result.Message, nil
	}

	svc := kafka_svc.New(getSSHPool(ctx))
	defer svc.Close()

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
	command, err := kafkaConnectCommand(operation, argString(args, "connector"))
	if err != nil {
		return "", err
	}
	if result, ok := checkKafkaToolPermission(ctx, assetID, command); !ok {
		return result.Message, nil
	}

	svc := kafka_svc.New(getSSHPool(ctx))
	defer svc.Close()

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
	command, err := kafkaMessageCommand(operation, topic)
	if err != nil {
		return "", err
	}
	if result, ok := checkKafkaToolPermission(ctx, assetID, command); !ok {
		return result.Message, nil
	}

	svc := kafka_svc.New(getSSHPool(ctx))
	defer svc.Close()

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

func checkKafkaToolPermission(ctx context.Context, assetID int64, command string) (CheckResult, bool) {
	if checker := GetPolicyChecker(ctx); checker != nil {
		result := checker.CheckForAsset(ctx, assetID, asset_entity.AssetTypeKafka, command)
		setCheckResult(ctx, result)
		if result.Decision != Allow {
			return result, false
		}
		return result, true
	}
	return CheckResult{Decision: Allow}, true
}

func normalizeKafkaOperation(operation, fallback string) string {
	operation = strings.ToLower(strings.TrimSpace(operation))
	if operation == "" {
		return fallback
	}
	return operation
}

func kafkaClusterCommand(operation string) (string, error) {
	switch operation {
	case "overview":
		return "cluster.read *", nil
	case "brokers", "list_brokers":
		return "broker.read *", nil
	case "get_broker_config":
		return "broker.config.read *", nil
	case "list_cluster_configs":
		return "cluster.config.read *", nil
	default:
		return "", fmt.Errorf("unsupported kafka_cluster operation: %s", operation)
	}
}

func kafkaTopicCommand(operation, topic string) (string, error) {
	switch operation {
	case "list":
		return "topic.list *", nil
	case "get", "describe":
		topic = strings.TrimSpace(topic)
		if topic == "" {
			return "", fmt.Errorf("topic is required for kafka_topic %s", operation)
		}
		return "topic.read " + topic, nil
	case "create":
		topic = strings.TrimSpace(topic)
		if topic == "" {
			return "", fmt.Errorf("topic is required for kafka_topic %s", operation)
		}
		return "topic.create " + topic, nil
	case "delete":
		topic = strings.TrimSpace(topic)
		if topic == "" {
			return "", fmt.Errorf("topic is required for kafka_topic %s", operation)
		}
		return "topic.delete " + topic, nil
	case "update_config":
		topic = strings.TrimSpace(topic)
		if topic == "" {
			return "", fmt.Errorf("topic is required for kafka_topic %s", operation)
		}
		return "topic.config.write " + topic, nil
	case "increase_partitions":
		topic = strings.TrimSpace(topic)
		if topic == "" {
			return "", fmt.Errorf("topic is required for kafka_topic %s", operation)
		}
		return "topic.partitions.write " + topic, nil
	case "delete_records":
		topic = strings.TrimSpace(topic)
		if topic == "" {
			return "", fmt.Errorf("topic is required for kafka_topic %s", operation)
		}
		return "topic.records.delete " + topic, nil
	default:
		return "", fmt.Errorf("unsupported kafka_topic operation: %s", operation)
	}
}

func kafkaCreateTopicRequestFromArgs(assetID int64, args map[string]any) (kafka_svc.CreateTopicRequest, error) {
	configs, err := kafkaStringMapFromJSON(argString(args, "configs"))
	if err != nil {
		return kafka_svc.CreateTopicRequest{}, err
	}
	return kafka_svc.CreateTopicRequest{
		AssetID:           assetID,
		Topic:             argString(args, "topic"),
		Partitions:        int32(argInt(args, "partitions")),
		ReplicationFactor: int16(argInt(args, "replication_factor")),
		Configs:           configs,
	}, nil
}

func kafkaAlterTopicConfigRequestFromArgs(assetID int64, args map[string]any) (kafka_svc.AlterTopicConfigRequest, error) {
	var updates []kafka_svc.TopicConfigMutation
	raw := strings.TrimSpace(argString(args, "config_updates"))
	if raw == "" {
		return kafka_svc.AlterTopicConfigRequest{}, fmt.Errorf("config_updates is required for kafka_topic update_config")
	}
	if err := json.Unmarshal([]byte(raw), &updates); err != nil {
		return kafka_svc.AlterTopicConfigRequest{}, fmt.Errorf("config_updates must be a JSON array: %w", err)
	}
	return kafka_svc.AlterTopicConfigRequest{
		AssetID: assetID,
		Topic:   argString(args, "topic"),
		Configs: updates,
	}, nil
}

func kafkaIncreasePartitionsRequestFromArgs(assetID int64, args map[string]any) (kafka_svc.IncreasePartitionsRequest, error) {
	return kafka_svc.IncreasePartitionsRequest{
		AssetID:    assetID,
		Topic:      argString(args, "topic"),
		Partitions: argInt(args, "partition_count"),
	}, nil
}

func kafkaDeleteRecordsRequestFromArgs(assetID int64, args map[string]any) (kafka_svc.DeleteRecordsRequest, error) {
	raw := strings.TrimSpace(argString(args, "records"))
	if raw == "" {
		return kafka_svc.DeleteRecordsRequest{}, fmt.Errorf("records is required for kafka_topic delete_records")
	}
	var partitions []kafka_svc.DeleteRecordsPartition
	if err := json.Unmarshal([]byte(raw), &partitions); err != nil {
		return kafka_svc.DeleteRecordsRequest{}, fmt.Errorf("records must be a JSON array: %w", err)
	}
	return kafka_svc.DeleteRecordsRequest{
		AssetID:    assetID,
		Topic:      argString(args, "topic"),
		Partitions: partitions,
	}, nil
}

func kafkaStringMapFromJSON(raw string) (map[string]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	configs := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &configs); err != nil {
		return nil, fmt.Errorf("configs must be a JSON object: %w", err)
	}
	return configs, nil
}

func kafkaConsumerGroupCommand(operation, group string) (string, error) {
	switch operation {
	case "list":
		return "consumer_group.list *", nil
	case "get", "describe":
		group = strings.TrimSpace(group)
		if group == "" {
			return "", fmt.Errorf("group is required for kafka_consumer_group %s", operation)
		}
		return "consumer_group.read " + group, nil
	case "reset_offset":
		group = strings.TrimSpace(group)
		if group == "" {
			return "", fmt.Errorf("group is required for kafka_consumer_group %s", operation)
		}
		return "consumer_group.offset.write " + group, nil
	case "delete":
		group = strings.TrimSpace(group)
		if group == "" {
			return "", fmt.Errorf("group is required for kafka_consumer_group %s", operation)
		}
		return "consumer_group.delete " + group, nil
	default:
		return "", fmt.Errorf("unsupported kafka_consumer_group operation: %s", operation)
	}
}

func kafkaResetConsumerGroupOffsetRequestFromArgs(assetID int64, args map[string]any) (kafka_svc.ResetConsumerGroupOffsetRequest, error) {
	partitions, err := kafkaInt32SliceFromJSON(argString(args, "partitions"))
	if err != nil {
		return kafka_svc.ResetConsumerGroupOffsetRequest{}, err
	}
	return kafka_svc.ResetConsumerGroupOffsetRequest{
		AssetID:         assetID,
		Group:           argString(args, "group"),
		Topic:           argString(args, "topic"),
		Partitions:      partitions,
		Mode:            argString(args, "mode"),
		Offset:          argInt64(args, "offset"),
		TimestampMillis: argInt64(args, "timestamp_millis"),
	}, nil
}

func kafkaInt32SliceFromJSON(raw string) ([]int32, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var values []int32
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, fmt.Errorf("partitions must be a JSON array of integers: %w", err)
	}
	return values, nil
}

func kafkaACLCommand(operation string) (string, error) {
	switch operation {
	case "list":
		return "acl.read *", nil
	case "create", "delete":
		return "acl.write *", nil
	default:
		return "", fmt.Errorf("unsupported kafka_acl operation: %s", operation)
	}
}

func kafkaListACLsRequestFromArgs(assetID int64, args map[string]any) kafka_svc.ListACLsRequest {
	return kafka_svc.ListACLsRequest{
		AssetID:      assetID,
		ResourceType: argString(args, "resource_type"),
		ResourceName: argString(args, "resource_name"),
		PatternType:  argString(args, "pattern_type"),
		Principal:    argString(args, "principal"),
		Host:         argString(args, "host"),
		Operation:    argString(args, "acl_operation"),
		Permission:   argString(args, "permission"),
		Page:         argInt(args, "page"),
		PageSize:     argInt(args, "page_size"),
	}
}

func kafkaCreateACLRequestFromArgs(assetID int64, args map[string]any) kafka_svc.CreateACLRequest {
	return kafka_svc.CreateACLRequest{
		AssetID:      assetID,
		ResourceType: argString(args, "resource_type"),
		ResourceName: argString(args, "resource_name"),
		PatternType:  argString(args, "pattern_type"),
		Principal:    argString(args, "principal"),
		Host:         argString(args, "host"),
		Operation:    argString(args, "acl_operation"),
		Permission:   argString(args, "permission"),
	}
}

func kafkaDeleteACLRequestFromArgs(assetID int64, args map[string]any) kafka_svc.DeleteACLRequest {
	return kafka_svc.DeleteACLRequest{
		AssetID:      assetID,
		ResourceType: argString(args, "resource_type"),
		ResourceName: argString(args, "resource_name"),
		PatternType:  argString(args, "pattern_type"),
		Principal:    argString(args, "principal"),
		Host:         argString(args, "host"),
		Operation:    argString(args, "acl_operation"),
		Permission:   argString(args, "permission"),
	}
}

func kafkaSchemaCommand(operation, subject string) (string, error) {
	switch operation {
	case "list_subjects":
		return "schema.read *", nil
	case "list_versions", "get", "describe", "check_compatibility":
		subject = strings.TrimSpace(subject)
		if subject == "" {
			return "", fmt.Errorf("subject is required for kafka_schema %s", operation)
		}
		return "schema.read " + subject, nil
	case "register":
		subject = strings.TrimSpace(subject)
		if subject == "" {
			return "", fmt.Errorf("subject is required for kafka_schema %s", operation)
		}
		return "schema.write " + subject, nil
	case "delete":
		subject = strings.TrimSpace(subject)
		if subject == "" {
			return "", fmt.Errorf("subject is required for kafka_schema %s", operation)
		}
		return "schema.delete " + subject, nil
	default:
		return "", fmt.Errorf("unsupported kafka_schema operation: %s", operation)
	}
}

func kafkaRegisterSchemaRequestFromArgs(assetID int64, args map[string]any) (kafka_svc.RegisterSchemaRequest, error) {
	references, err := kafkaSchemaReferencesFromJSON(argString(args, "references"))
	if err != nil {
		return kafka_svc.RegisterSchemaRequest{}, err
	}
	return kafka_svc.RegisterSchemaRequest{
		AssetID:    assetID,
		Subject:    argString(args, "subject"),
		Schema:     argString(args, "schema"),
		SchemaType: argString(args, "schema_type"),
		References: references,
	}, nil
}

func kafkaCheckSchemaCompatibilityRequestFromArgs(assetID int64, args map[string]any) (kafka_svc.CheckSchemaCompatibilityRequest, error) {
	references, err := kafkaSchemaReferencesFromJSON(argString(args, "references"))
	if err != nil {
		return kafka_svc.CheckSchemaCompatibilityRequest{}, err
	}
	return kafka_svc.CheckSchemaCompatibilityRequest{
		AssetID:    assetID,
		Subject:    argString(args, "subject"),
		Version:    argString(args, "version"),
		Schema:     argString(args, "schema"),
		SchemaType: argString(args, "schema_type"),
		References: references,
	}, nil
}

func kafkaDeleteSchemaRequestFromArgs(assetID int64, args map[string]any) kafka_svc.DeleteSchemaRequest {
	return kafka_svc.DeleteSchemaRequest{
		AssetID:   assetID,
		Subject:   argString(args, "subject"),
		Version:   argString(args, "version"),
		Permanent: argBool(args, "permanent"),
	}
}

func kafkaSchemaReferencesFromJSON(raw string) ([]kafka_svc.SchemaReference, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var references []kafka_svc.SchemaReference
	if err := json.Unmarshal([]byte(raw), &references); err != nil {
		return nil, fmt.Errorf("references must be a JSON array: %w", err)
	}
	return references, nil
}

func kafkaConnectCommand(operation, connector string) (string, error) {
	switch operation {
	case "list_clusters", "list_connectors":
		return "connect.read *", nil
	case "get_connector", "get", "describe":
		connector = strings.TrimSpace(connector)
		if connector == "" {
			return "", fmt.Errorf("connector is required for kafka_connect %s", operation)
		}
		return "connect.read " + connector, nil
	case "create", "update_config":
		connector = strings.TrimSpace(connector)
		if connector == "" {
			return "", fmt.Errorf("connector is required for kafka_connect %s", operation)
		}
		return "connect.write " + connector, nil
	case "pause", "resume", "restart":
		connector = strings.TrimSpace(connector)
		if connector == "" {
			return "", fmt.Errorf("connector is required for kafka_connect %s", operation)
		}
		return "connect.state.write " + connector, nil
	case "delete":
		connector = strings.TrimSpace(connector)
		if connector == "" {
			return "", fmt.Errorf("connector is required for kafka_connect %s", operation)
		}
		return "connect.delete " + connector, nil
	default:
		return "", fmt.Errorf("unsupported kafka_connect operation: %s", operation)
	}
}

func kafkaConnectorConfigRequestFromArgs(assetID int64, args map[string]any) (kafka_svc.ConnectorConfigRequest, error) {
	config, err := kafkaStringMapFromJSON(argString(args, "config"))
	if err != nil {
		return kafka_svc.ConnectorConfigRequest{}, err
	}
	if len(config) == 0 {
		return kafka_svc.ConnectorConfigRequest{}, fmt.Errorf("config is required for kafka_connect connector config operation")
	}
	return kafka_svc.ConnectorConfigRequest{
		AssetID: assetID,
		Cluster: argString(args, "cluster"),
		Name:    argString(args, "connector"),
		Config:  config,
	}, nil
}

func kafkaRestartConnectorRequestFromArgs(assetID int64, args map[string]any) kafka_svc.RestartConnectorRequest {
	return kafka_svc.RestartConnectorRequest{
		AssetID:      assetID,
		Cluster:      argString(args, "cluster"),
		Name:         argString(args, "connector"),
		IncludeTasks: argBool(args, "include_tasks"),
		OnlyFailed:   argBool(args, "only_failed"),
	}
}

func kafkaMessageCommand(operation, topic string) (string, error) {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return "", fmt.Errorf("topic is required for kafka_message %s", operation)
	}
	switch operation {
	case "browse", "inspect":
		return "message.read " + topic, nil
	case "produce":
		return "message.write " + topic, nil
	default:
		return "", fmt.Errorf("unsupported kafka_message operation: %s", operation)
	}
}

func kafkaBrowseRequestFromArgs(assetID int64, args map[string]any) (kafka_svc.BrowseMessagesRequest, error) {
	partition, err := argOptionalInt32(args, "partition")
	if err != nil {
		return kafka_svc.BrowseMessagesRequest{}, err
	}
	return kafka_svc.BrowseMessagesRequest{
		AssetID:         assetID,
		Topic:           argString(args, "topic"),
		Partition:       partition,
		StartMode:       argString(args, "start_mode"),
		Offset:          argInt64(args, "offset"),
		TimestampMillis: argInt64(args, "timestamp_millis"),
		Limit:           argInt(args, "limit"),
		MaxBytes:        argInt(args, "max_bytes"),
		DecodeMode:      argString(args, "decode_mode"),
		MaxWaitMillis:   argInt(args, "max_wait_millis"),
	}, nil
}

func kafkaInspectRequestFromArgs(assetID int64, args map[string]any) (kafka_svc.BrowseMessagesRequest, error) {
	partition, err := argOptionalInt32(args, "partition")
	if err != nil {
		return kafka_svc.BrowseMessagesRequest{}, err
	}
	if partition == nil {
		return kafka_svc.BrowseMessagesRequest{}, fmt.Errorf("partition is required for kafka_message inspect")
	}
	if _, ok := args["offset"]; !ok {
		return kafka_svc.BrowseMessagesRequest{}, fmt.Errorf("offset is required for kafka_message inspect")
	}
	return kafka_svc.BrowseMessagesRequest{
		AssetID:       assetID,
		Topic:         argString(args, "topic"),
		Partition:     partition,
		StartMode:     "offset",
		Offset:        argInt64(args, "offset"),
		Limit:         1,
		MaxBytes:      argInt(args, "max_bytes"),
		DecodeMode:    argString(args, "decode_mode"),
		MaxWaitMillis: argInt(args, "max_wait_millis"),
	}, nil
}

func kafkaProduceRequestFromArgs(assetID int64, args map[string]any) (kafka_svc.ProduceMessageRequest, error) {
	partition, err := argOptionalInt32(args, "partition")
	if err != nil {
		return kafka_svc.ProduceMessageRequest{}, err
	}
	headers, err := kafkaProduceHeadersFromArgs(args)
	if err != nil {
		return kafka_svc.ProduceMessageRequest{}, err
	}
	return kafka_svc.ProduceMessageRequest{
		AssetID:         assetID,
		Topic:           argString(args, "topic"),
		Partition:       partition,
		Key:             argString(args, "key"),
		KeyEncoding:     argString(args, "key_encoding"),
		Value:           argString(args, "value"),
		ValueEncoding:   argString(args, "value_encoding"),
		Headers:         headers,
		TimestampMillis: argInt64(args, "timestamp_millis"),
	}, nil
}

func kafkaProduceHeadersFromArgs(args map[string]any) ([]kafka_svc.ProduceMessageHeader, error) {
	raw := strings.TrimSpace(argString(args, "headers"))
	if raw == "" {
		return nil, nil
	}
	var headers []kafka_svc.ProduceMessageHeader
	if err := json.Unmarshal([]byte(raw), &headers); err != nil {
		return nil, fmt.Errorf("headers must be a JSON array: %w", err)
	}
	return headers, nil
}

func marshalKafkaResult(result any) (string, error) {
	data, err := json.Marshal(result)
	if err != nil {
		logger.Default().Error("marshal Kafka result", zap.Error(err))
		return "", fmt.Errorf("序列化 Kafka 结果失败: %w", err)
	}
	return string(data), nil
}

func argOptionalInt32(args map[string]any, key string) (*int32, error) {
	value, ok := args[key]
	if !ok || value == nil {
		return nil, nil
	}

	var n int64
	switch v := value.(type) {
	case int:
		n = int64(v)
	case int32:
		n = int64(v)
	case int64:
		n = v
	case float64:
		if math.Trunc(v) != v {
			return nil, fmt.Errorf("%s must be an integer", key)
		}
		n = int64(v)
	case json.Number:
		parsed, err := v.Int64()
		if err != nil {
			return nil, fmt.Errorf("%s must be an integer: %w", key, err)
		}
		n = parsed
	case string:
		if strings.TrimSpace(v) == "" {
			return nil, nil
		}
		parsed, err := strconv.ParseInt(strings.TrimSpace(v), 10, 32)
		if err != nil {
			return nil, fmt.Errorf("%s must be an integer: %w", key, err)
		}
		n = parsed
	default:
		return nil, fmt.Errorf("%s must be an integer", key)
	}
	const (
		minInt32 = -1 << 31
		maxInt32 = 1<<31 - 1
	)
	if n < minInt32 || n > maxInt32 {
		return nil, fmt.Errorf("%s is out of int32 range", key)
	}
	out := int32(n)
	return &out, nil
}

func argBool(args map[string]any, key string) bool {
	if v, ok := args[key]; ok {
		switch b := v.(type) {
		case bool:
			return b
		case string:
			return strings.EqualFold(strings.TrimSpace(b), "true")
		}
	}
	return false
}
