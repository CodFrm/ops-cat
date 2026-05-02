package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/cago-frame/cago/pkg/logger"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"

	"github.com/opskat/opskat/internal/connpool"
	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/service/asset_svc"
	"github.com/opskat/opskat/internal/service/credential_resolver"
	"github.com/opskat/opskat/internal/service/kafka_svc"
)

// --- Kafka connection cache ---

type kafkaCacheKeyType struct{}

type kafkaClientCloser struct {
	client *kgo.Client
}

func (c *kafkaClientCloser) Close() error {
	if c != nil && c.client != nil {
		c.client.Close()
	}
	return nil
}

type KafkaClientCache = ConnCache[*kafkaClientCloser]

func NewKafkaClientCache() *KafkaClientCache {
	return NewConnCache[*kafkaClientCloser]("Kafka")
}

func WithKafkaCache(ctx context.Context, cache *KafkaClientCache) context.Context {
	return context.WithValue(ctx, kafkaCacheKeyType{}, cache)
}

func getKafkaCache(ctx context.Context) *KafkaClientCache {
	if cache, ok := ctx.Value(kafkaCacheKeyType{}).(*KafkaClientCache); ok {
		return cache
	}
	return nil
}

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

	client, closeFn, err := openKafkaClient(ctx, assetID)
	if err != nil {
		return "", err
	}
	defer closeFn()

	admin := kadm.NewClient(client)
	switch operation {
	case "overview":
		metadata, err := admin.Metadata(ctx)
		if err != nil {
			return "", fmt.Errorf("read Kafka cluster overview: %w", err)
		}
		return marshalResult(kafkaOverviewResult(assetID, metadata))
	case "brokers", "list_brokers":
		brokers, err := admin.ListBrokers(ctx)
		if err != nil {
			return "", fmt.Errorf("list Kafka brokers: %w", err)
		}
		return marshalResult(map[string]any{"brokers": kafkaBrokersResult(brokers), "count": len(brokers)})
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

	client, closeFn, err := openKafkaClient(ctx, assetID)
	if err != nil {
		return "", err
	}
	defer closeFn()

	admin := kadm.NewClient(client)
	switch operation {
	case "list":
		topics, err := listKafkaTopics(ctx, admin, argBool(args, "include_internal"))
		if err != nil {
			return "", fmt.Errorf("list Kafka topics: %w", err)
		}
		return marshalResult(kafkaTopicListResult(topics, args))
	case "get", "describe":
		topic := strings.TrimSpace(argString(args, "topic"))
		topics, err := admin.ListTopicsWithInternal(ctx, topic)
		if err != nil {
			return "", fmt.Errorf("describe Kafka topic: %w", err)
		}
		detail, ok := topics[topic]
		if !ok {
			return "", fmt.Errorf("Kafka topic not found: %s", topic)
		}
		return marshalResult(map[string]any{"topic": kafkaTopicDetailResult(detail)})
	case "create":
		req, err := kafkaCreateTopicRequestFromArgs(assetID, args)
		if err != nil {
			return "", err
		}
		svc := kafka_svc.New(getSSHPool(ctx))
		defer svc.Close()
		result, err := svc.CreateTopic(ctx, req)
		if err != nil {
			return "", err
		}
		return marshalKafkaResult(result)
	case "delete":
		svc := kafka_svc.New(getSSHPool(ctx))
		defer svc.Close()
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
		svc := kafka_svc.New(getSSHPool(ctx))
		defer svc.Close()
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
		svc := kafka_svc.New(getSSHPool(ctx))
		defer svc.Close()
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
		svc := kafka_svc.New(getSSHPool(ctx))
		defer svc.Close()
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

	client, closeFn, err := openKafkaClient(ctx, assetID)
	if err != nil {
		return "", err
	}
	defer closeFn()

	admin := kadm.NewClient(client)
	switch operation {
	case "list":
		groups, err := admin.ListGroups(ctx)
		if err != nil {
			return "", fmt.Errorf("list Kafka consumer groups: %w", err)
		}
		return marshalResult(map[string]any{"groups": kafkaConsumerGroupsResult(groups), "count": len(groups)})
	case "get", "describe":
		group := strings.TrimSpace(argString(args, "group"))
		groups, err := admin.DescribeGroups(ctx, group)
		if err != nil {
			return "", fmt.Errorf("describe Kafka consumer group: %w", err)
		}
		detail, ok := groups[group]
		if !ok {
			return "", fmt.Errorf("Kafka consumer group not found: %s", group)
		}
		result := kafkaConsumerGroupDetailResult(detail)
		if lags, lagErr := admin.Lag(ctx, group); lagErr != nil {
			result["lag_error"] = lagErr.Error()
		} else if lag, ok := lags[group]; ok {
			result["lag"] = kafkaConsumerGroupLagResult(lag)
			result["total_lag"] = lag.Lag.Total()
		}
		return marshalResult(map[string]any{"group": result})
	default:
		return "", fmt.Errorf("unsupported kafka_consumer_group operation: %s", operation)
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

func openKafkaClient(ctx context.Context, assetID int64) (*kgo.Client, func(), error) {
	asset, err := asset_svc.Asset().Get(ctx, assetID)
	if err != nil {
		return nil, func() {}, fmt.Errorf("asset not found: %w", err)
	}
	if !asset.IsKafka() {
		return nil, func() {}, fmt.Errorf("asset is not Kafka type")
	}
	cfg, err := asset.GetKafkaConfig()
	if err != nil {
		return nil, func() {}, fmt.Errorf("failed to get Kafka config: %w", err)
	}

	wrapper, err := getOrDialKafka(ctx, asset, cfg)
	if err != nil {
		return nil, func() {}, fmt.Errorf("failed to connect to Kafka: %w", err)
	}
	if getKafkaCache(ctx) != nil {
		return wrapper.client, func() {}, nil
	}
	return wrapper.client, func() {
		if err := wrapper.Close(); err != nil {
			logger.Default().Warn("close Kafka connection", zap.Error(err))
		}
	}, nil
}

func getOrDialKafka(ctx context.Context, asset *asset_entity.Asset, cfg *asset_entity.KafkaConfig) (*kafkaClientCloser, error) {
	dialFn := func() (*kafkaClientCloser, io.Closer, error) {
		password, err := credential_resolver.Default().ResolvePasswordGeneric(ctx, cfg)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to resolve credentials: %w", err)
		}
		client, err := connpool.DialKafka(ctx, asset, cfg, password, getSSHPool(ctx))
		if err != nil {
			return nil, nil, err
		}
		return &kafkaClientCloser{client: client}, nil, nil
	}
	if cache := getKafkaCache(ctx); cache != nil {
		wrapper, _, err := cache.GetOrDial(asset.ID, dialFn)
		return wrapper, err
	}
	wrapper, _, err := dialFn()
	return wrapper, err
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
	default:
		return "", fmt.Errorf("unsupported kafka_consumer_group operation: %s", operation)
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

func listKafkaTopics(ctx context.Context, admin *kadm.Client, includeInternal bool) (kadm.TopicDetails, error) {
	if includeInternal {
		return admin.ListTopicsWithInternal(ctx)
	}
	return admin.ListTopics(ctx)
}

func kafkaOverviewResult(assetID int64, metadata kadm.Metadata) map[string]any {
	out := map[string]any{
		"asset_id":        assetID,
		"cluster_id":      metadata.Cluster,
		"controller_id":   metadata.Controller,
		"broker_count":    len(metadata.Brokers),
		"topic_count":     len(metadata.Topics),
		"brokers":         kafkaBrokersResult(metadata.Brokers),
		"partition_count": 0,
	}
	internalTopics := 0
	offlinePartitions := 0
	underReplicatedPartitions := 0
	partitionCount := 0
	for _, topic := range metadata.Topics {
		if topic.IsInternal {
			internalTopics++
		}
		for _, partition := range topic.Partitions {
			partitionCount++
			if partition.Leader < 0 {
				offlinePartitions++
			}
			if len(partition.Replicas) > 0 && len(partition.ISR) < len(partition.Replicas) {
				underReplicatedPartitions++
			}
		}
	}
	out["internal_topic_count"] = internalTopics
	out["partition_count"] = partitionCount
	out["offline_partition_count"] = offlinePartitions
	out["under_replicated_partition_count"] = underReplicatedPartitions
	return out
}

func kafkaBrokersResult(details kadm.BrokerDetails) []map[string]any {
	out := make([]map[string]any, 0, len(details))
	for _, detail := range details {
		broker := map[string]any{
			"node_id": detail.NodeID,
			"host":    detail.Host,
			"port":    detail.Port,
		}
		if detail.Rack != nil {
			broker["rack"] = *detail.Rack
		}
		out = append(out, broker)
	}
	return out
}

func kafkaTopicListResult(topics kadm.TopicDetails, args map[string]any) map[string]any {
	search := strings.ToLower(strings.TrimSpace(argString(args, "search")))
	page, pageSize := normalizeKafkaPage(argInt(args, "page"), argInt(args, "page_size"))
	items := make([]map[string]any, 0, len(topics))
	for _, topic := range topics.Sorted() {
		item := kafkaTopicSummaryResult(topic)
		name, _ := item["name"].(string)
		if search != "" && !strings.Contains(strings.ToLower(name), search) {
			continue
		}
		items = append(items, item)
	}
	total := len(items)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return map[string]any{
		"topics":    items[start:end],
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}
}

func kafkaTopicSummaryResult(topic kadm.TopicDetail) map[string]any {
	replicationFactor := 0
	offlinePartitions := 0
	underReplicatedPartitions := 0
	for _, partition := range topic.Partitions {
		if len(partition.Replicas) > replicationFactor {
			replicationFactor = len(partition.Replicas)
		}
		if partition.Leader < 0 {
			offlinePartitions++
		}
		if len(partition.Replicas) > 0 && len(partition.ISR) < len(partition.Replicas) {
			underReplicatedPartitions++
		}
	}
	out := map[string]any{
		"name":                             topic.Topic,
		"id":                               topic.ID.String(),
		"internal":                         topic.IsInternal,
		"partition_count":                  len(topic.Partitions),
		"replication_factor":               replicationFactor,
		"offline_partition_count":          offlinePartitions,
		"under_replicated_partition_count": underReplicatedPartitions,
	}
	if topic.Err != nil {
		out["error"] = topic.Err.Error()
	}
	return out
}

func kafkaTopicDetailResult(topic kadm.TopicDetail) map[string]any {
	out := kafkaTopicSummaryResult(topic)
	partitions := make([]map[string]any, 0, len(topic.Partitions))
	for _, partition := range topic.Partitions.Sorted() {
		item := map[string]any{
			"partition":        partition.Partition,
			"leader":           partition.Leader,
			"leader_epoch":     partition.LeaderEpoch,
			"replicas":         partition.Replicas,
			"isr":              partition.ISR,
			"offline_replicas": partition.OfflineReplicas,
		}
		if partition.Err != nil {
			item["error"] = partition.Err.Error()
		}
		partitions = append(partitions, item)
	}
	out["partitions"] = partitions
	out["authorized_operations"] = kafkaACLOperations(topic.AuthorizedOperations)
	return out
}

func kafkaConsumerGroupsResult(groups kadm.ListedGroups) []map[string]any {
	out := make([]map[string]any, 0, len(groups))
	for _, group := range groups.Sorted() {
		out = append(out, map[string]any{
			"group":         group.Group,
			"coordinator":   group.Coordinator,
			"protocol_type": group.ProtocolType,
			"state":         group.State,
		})
	}
	return out
}

func kafkaConsumerGroupDetailResult(group kadm.DescribedGroup) map[string]any {
	out := map[string]any{
		"group":         group.Group,
		"coordinator":   kafkaBrokersResult(kadm.BrokerDetails{group.Coordinator})[0],
		"state":         group.State,
		"protocol_type": group.ProtocolType,
		"protocol":      group.Protocol,
		"members":       kafkaConsumerGroupMembersResult(group.Members),
	}
	if group.Err != nil {
		out["error"] = group.Err.Error()
	}
	if group.ErrMessage != "" {
		out["error_message"] = group.ErrMessage
	}
	out["authorized_operations"] = kafkaACLOperations(group.AuthorizedOperations)
	return out
}

func kafkaConsumerGroupMembersResult(members []kadm.DescribedGroupMember) []map[string]any {
	out := make([]map[string]any, 0, len(members))
	for _, member := range members {
		item := map[string]any{
			"member_id":   member.MemberID,
			"client_id":   member.ClientID,
			"client_host": member.ClientHost,
		}
		if member.InstanceID != nil {
			item["instance_id"] = *member.InstanceID
		}
		if assignment, ok := member.Assigned.AsConsumer(); ok {
			assigned := make([]map[string]any, 0, len(assignment.Topics))
			for _, topic := range assignment.Topics {
				partitions := append([]int32(nil), topic.Partitions...)
				sort.Slice(partitions, func(i, j int) bool { return partitions[i] < partitions[j] })
				assigned = append(assigned, map[string]any{
					"topic":      topic.Topic,
					"partitions": partitions,
				})
			}
			item["assigned_partitions"] = assigned
		}
		out = append(out, item)
	}
	return out
}

func kafkaConsumerGroupLagResult(lag kadm.DescribedGroupLag) []map[string]any {
	out := make([]map[string]any, 0)
	for _, partitionLag := range lag.Lag.Sorted() {
		item := map[string]any{
			"topic":            partitionLag.Topic,
			"partition":        partitionLag.Partition,
			"committed_offset": partitionLag.Commit.At,
			"end_offset":       partitionLag.End.Offset,
			"lag":              partitionLag.Lag,
		}
		if partitionLag.Member != nil {
			item["member_id"] = partitionLag.Member.MemberID
		}
		if partitionLag.Err != nil {
			item["error"] = partitionLag.Err.Error()
		}
		out = append(out, item)
	}
	return out
}

func kafkaACLOperations(ops []kadm.ACLOperation) []string {
	if len(ops) == 0 {
		return nil
	}
	out := make([]string, 0, len(ops))
	for _, op := range ops {
		out = append(out, op.String())
	}
	return out
}

func normalizeKafkaPage(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 500 {
		pageSize = 500
	}
	return page, pageSize
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
