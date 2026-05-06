package kafka_svc

import (
	"context"
	"fmt"
	"strings"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
)

func (s *Service) CreateTopic(ctx context.Context, req CreateTopicRequest) (TopicOperationResponse, error) {
	var out TopicOperationResponse
	err := s.withClient(ctx, req.AssetID, func(ctx context.Context, client *kgo.Client, admin *kadm.Client, _ *asset_entity.Asset, _ *asset_entity.KafkaConfig) error {
		topic := strings.TrimSpace(req.Topic)
		if topic == "" {
			return fmt.Errorf("topic 不能为空")
		}
		if req.Partitions <= 0 {
			return fmt.Errorf("分区数必须大于0")
		}
		if req.ReplicationFactor <= 0 {
			return fmt.Errorf("副本因子必须大于0")
		}

		created, err := admin.CreateTopic(ctx, req.Partitions, req.ReplicationFactor, topicConfigMap(req.Configs), topic)
		if err != nil {
			return fmt.Errorf("创建 Kafka Topic 失败: %w", err)
		}
		if created.Err != nil {
			return fmt.Errorf("创建 Kafka Topic 失败: %w", created.Err)
		}
		client.PurgeTopicsFromClient(topic)
		out = TopicOperationResponse{Topic: topic, Message: created.ErrMessage}
		return nil
	})
	return out, err
}

func (s *Service) DeleteTopic(ctx context.Context, assetID int64, topic string) (TopicOperationResponse, error) {
	var out TopicOperationResponse
	err := s.withClient(ctx, assetID, func(ctx context.Context, client *kgo.Client, admin *kadm.Client, _ *asset_entity.Asset, _ *asset_entity.KafkaConfig) error {
		name := strings.TrimSpace(topic)
		if name == "" {
			return fmt.Errorf("topic 不能为空")
		}
		deleted, err := admin.DeleteTopic(ctx, name)
		if err != nil {
			return fmt.Errorf("删除 Kafka Topic 失败: %w", err)
		}
		if deleted.Err != nil {
			return fmt.Errorf("删除 Kafka Topic 失败: %w", deleted.Err)
		}
		client.PurgeTopicsFromClient(name)
		out = TopicOperationResponse{Topic: name, Message: deleted.ErrMessage}
		return nil
	})
	return out, err
}

func (s *Service) AlterTopicConfig(ctx context.Context, req AlterTopicConfigRequest) (TopicOperationResponse, error) {
	var out TopicOperationResponse
	err := s.withClient(ctx, req.AssetID, func(ctx context.Context, client *kgo.Client, admin *kadm.Client, _ *asset_entity.Asset, _ *asset_entity.KafkaConfig) error {
		topic := strings.TrimSpace(req.Topic)
		if topic == "" {
			return fmt.Errorf("topic 不能为空")
		}
		configs, err := topicConfigMutations(req.Configs)
		if err != nil {
			return err
		}
		responses, err := admin.AlterTopicConfigs(ctx, configs, topic)
		if err != nil {
			return fmt.Errorf("修改 Kafka Topic 配置失败: %w", err)
		}
		response, err := responses.On(topic, nil)
		if err != nil {
			return fmt.Errorf("修改 Kafka Topic 配置失败: %w", err)
		}
		if response.Err != nil {
			return fmt.Errorf("修改 Kafka Topic 配置失败: %w", response.Err)
		}
		client.PurgeTopicsFromClient(topic)
		out = TopicOperationResponse{Topic: topic, Message: response.ErrMessage}
		return nil
	})
	return out, err
}

func (s *Service) IncreasePartitions(ctx context.Context, req IncreasePartitionsRequest) (TopicOperationResponse, error) {
	var out TopicOperationResponse
	err := s.withClient(ctx, req.AssetID, func(ctx context.Context, client *kgo.Client, admin *kadm.Client, _ *asset_entity.Asset, _ *asset_entity.KafkaConfig) error {
		topic := strings.TrimSpace(req.Topic)
		if topic == "" {
			return fmt.Errorf("topic 不能为空")
		}
		if req.Partitions <= 0 {
			return fmt.Errorf("分区数必须大于0")
		}
		responses, err := admin.UpdatePartitions(ctx, req.Partitions, topic)
		if err != nil {
			return fmt.Errorf("增加 Kafka Topic 分区失败: %w", err)
		}
		response, ok := responses[topic]
		if !ok {
			return fmt.Errorf("增加 Kafka Topic 分区失败: 无响应")
		}
		if response.Err != nil {
			return fmt.Errorf("增加 Kafka Topic 分区失败: %w", response.Err)
		}
		client.PurgeTopicsFromClient(topic)
		out = TopicOperationResponse{Topic: topic, Message: response.ErrMessage}
		return nil
	})
	return out, err
}

func (s *Service) DeleteRecords(ctx context.Context, req DeleteRecordsRequest) (DeleteRecordsResponse, error) {
	var out DeleteRecordsResponse
	err := s.withClient(ctx, req.AssetID, func(ctx context.Context, _ *kgo.Client, admin *kadm.Client, _ *asset_entity.Asset, _ *asset_entity.KafkaConfig) error {
		topic := strings.TrimSpace(req.Topic)
		if topic == "" {
			return fmt.Errorf("topic 不能为空")
		}
		if len(req.Partitions) == 0 {
			return fmt.Errorf("至少需要一个 Partition offset")
		}
		offsets := kadm.Offsets{}
		for _, partition := range req.Partitions {
			if partition.Partition < 0 {
				return fmt.Errorf("partition 不能小于0")
			}
			if partition.Offset < 0 {
				return fmt.Errorf("offset 不能小于0")
			}
			offsets.Add(kadm.Offset{Topic: topic, Partition: partition.Partition, At: partition.Offset})
		}
		responses, err := admin.DeleteRecords(ctx, offsets)
		if err != nil {
			return fmt.Errorf("删除 Kafka Topic 记录失败: %w", err)
		}
		out = DeleteRecordsResponse{Topic: topic, Partitions: make([]DeleteRecordsPartitionResult, 0, len(req.Partitions))}
		for _, response := range responses.Sorted() {
			result := DeleteRecordsPartitionResult{
				Partition:    response.Partition,
				LowWatermark: response.LowWatermark,
				Error:        errorString(response.Err),
			}
			out.Partitions = append(out.Partitions, result)
		}
		if err := responses.Error(); err != nil {
			return fmt.Errorf("删除 Kafka Topic 记录失败: %w", err)
		}
		return nil
	})
	return out, err
}

func topicConfigMap(configs map[string]string) map[string]*string {
	if len(configs) == 0 {
		return nil
	}
	out := make(map[string]*string, len(configs))
	for name, value := range configs {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		v := value
		out[name] = &v
	}
	return out
}

func topicConfigMutations(configs []TopicConfigMutation) ([]kadm.AlterConfig, error) {
	if len(configs) == 0 {
		return nil, fmt.Errorf("至少需要一个配置变更")
	}
	out := make([]kadm.AlterConfig, 0, len(configs))
	for _, cfg := range configs {
		name := strings.TrimSpace(cfg.Name)
		if name == "" {
			return nil, fmt.Errorf("配置名称不能为空")
		}
		op, err := topicConfigOp(cfg.Op)
		if err != nil {
			return nil, err
		}
		var value *string
		if op != kadm.DeleteConfig {
			v := cfg.Value
			value = &v
		}
		out = append(out, kadm.AlterConfig{Op: op, Name: name, Value: value})
	}
	return out, nil
}

func topicConfigOp(op string) (kadm.IncrementalOp, error) {
	switch strings.ToLower(strings.TrimSpace(op)) {
	case "", "set":
		return kadm.SetConfig, nil
	case "delete", "unset":
		return kadm.DeleteConfig, nil
	case "append":
		return kadm.AppendConfig, nil
	case "subtract":
		return kadm.SubtractConfig, nil
	default:
		return 0, fmt.Errorf("不支持的配置操作: %s", op)
	}
}
