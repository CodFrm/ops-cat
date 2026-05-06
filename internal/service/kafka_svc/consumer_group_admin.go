package kafka_svc

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
)

func (s *Service) ResetConsumerGroupOffset(ctx context.Context, req ResetConsumerGroupOffsetRequest) (ResetConsumerGroupOffsetResponse, error) {
	var out ResetConsumerGroupOffsetResponse
	err := s.withClient(ctx, req.AssetID, func(ctx context.Context, _ *kgo.Client, admin *kadm.Client, _ *asset_entity.Asset, _ *asset_entity.KafkaConfig) error {
		normalized, err := normalizeResetConsumerGroupOffsetRequest(req)
		if err != nil {
			return err
		}
		partitions, err := consumerGroupResetPartitions(ctx, admin, normalized.Topic, normalized.Partitions)
		if err != nil {
			return err
		}
		offsets, err := consumerGroupResetOffsets(ctx, admin, normalized, partitions)
		if err != nil {
			return err
		}
		responses, err := admin.CommitOffsets(ctx, normalized.Group, offsets)
		if err != nil {
			return fmt.Errorf("重置 Kafka Consumer Group Offset 失败: %w", err)
		}
		out = ResetConsumerGroupOffsetResponse{
			Group:      normalized.Group,
			Topic:      normalized.Topic,
			Partitions: make([]ConsumerGroupOffsetResetResult, 0, len(partitions)),
		}
		for _, response := range responses.Sorted() {
			if response.Topic != normalized.Topic {
				continue
			}
			out.Partitions = append(out.Partitions, ConsumerGroupOffsetResetResult{
				Partition: response.Partition,
				Offset:    response.At,
				Error:     errorString(response.Err),
			})
		}
		if err := responses.Error(); err != nil {
			return fmt.Errorf("重置 Kafka Consumer Group Offset 失败: %w", err)
		}
		return nil
	})
	return out, err
}

func (s *Service) DeleteConsumerGroup(ctx context.Context, assetID int64, group string) (DeleteConsumerGroupResponse, error) {
	var out DeleteConsumerGroupResponse
	err := s.withClient(ctx, assetID, func(ctx context.Context, _ *kgo.Client, admin *kadm.Client, _ *asset_entity.Asset, _ *asset_entity.KafkaConfig) error {
		name := strings.TrimSpace(group)
		if name == "" {
			return fmt.Errorf("consumer group 不能为空")
		}
		deleted, err := admin.DeleteGroup(ctx, name)
		if err != nil {
			return fmt.Errorf("删除 Kafka Consumer Group 失败: %w", err)
		}
		if deleted.Err != nil {
			return fmt.Errorf("删除 Kafka Consumer Group 失败: %w", deleted.Err)
		}
		out = DeleteConsumerGroupResponse{Group: deleted.Group}
		return nil
	})
	return out, err
}

func normalizeResetConsumerGroupOffsetRequest(req ResetConsumerGroupOffsetRequest) (ResetConsumerGroupOffsetRequest, error) {
	req.Group = strings.TrimSpace(req.Group)
	req.Topic = strings.TrimSpace(req.Topic)
	req.Mode = strings.ToLower(strings.TrimSpace(req.Mode))
	if req.Group == "" {
		return req, fmt.Errorf("consumer group 不能为空")
	}
	if req.Topic == "" {
		return req, fmt.Errorf("topic 不能为空")
	}
	if req.Mode == "" {
		req.Mode = "latest"
	}
	switch req.Mode {
	case "earliest", "latest":
	case "offset":
		if req.Offset < 0 {
			return req, fmt.Errorf("offset 不能小于0")
		}
	case "timestamp":
		if req.TimestampMillis <= 0 {
			return req, fmt.Errorf("timestamp 不能为空")
		}
	default:
		return req, fmt.Errorf("不支持的 Offset 重置模式: %s", req.Mode)
	}
	for _, partition := range req.Partitions {
		if partition < 0 {
			return req, fmt.Errorf("partition 不能小于0")
		}
	}
	return req, nil
}

func consumerGroupResetPartitions(ctx context.Context, admin *kadm.Client, topic string, selected []int32) ([]int32, error) {
	topics, err := admin.ListTopicsWithInternal(ctx, topic)
	if err != nil {
		return nil, fmt.Errorf("读取 Kafka Topic 详情失败: %w", err)
	}
	detail, ok := topics[topic]
	if !ok {
		return nil, fmt.Errorf("topic 不存在: %s", topic)
	}
	if detail.Err != nil {
		return nil, fmt.Errorf("读取 Topic 失败: %w", detail.Err)
	}

	exists := map[int32]bool{}
	for _, partition := range detail.Partitions {
		exists[partition.Partition] = true
	}
	if len(selected) == 0 {
		partitions := make([]int32, 0, len(exists))
		for partition := range exists {
			partitions = append(partitions, partition)
		}
		sort.Slice(partitions, func(i, j int) bool { return partitions[i] < partitions[j] })
		return partitions, nil
	}

	partitions := make([]int32, 0, len(selected))
	seen := map[int32]bool{}
	for _, partition := range selected {
		if !exists[partition] {
			return nil, fmt.Errorf("topic %s 不存在 partition %d", topic, partition)
		}
		if seen[partition] {
			continue
		}
		seen[partition] = true
		partitions = append(partitions, partition)
	}
	sort.Slice(partitions, func(i, j int) bool { return partitions[i] < partitions[j] })
	return partitions, nil
}

func consumerGroupResetOffsets(ctx context.Context, admin *kadm.Client, req ResetConsumerGroupOffsetRequest, partitions []int32) (kadm.Offsets, error) {
	offsets := kadm.Offsets{}
	switch req.Mode {
	case "offset":
		for _, partition := range partitions {
			offsets.Add(kadm.Offset{Topic: req.Topic, Partition: partition, At: req.Offset})
		}
	case "earliest", "latest", "timestamp":
		listed, err := listedOffsetsForReset(ctx, admin, req)
		if err != nil {
			return nil, err
		}
		for _, partition := range partitions {
			offset, ok := listed.Lookup(req.Topic, partition)
			if !ok {
				return nil, fmt.Errorf("读取 Partition %d offset 失败", partition)
			}
			if offset.Err != nil {
				return nil, fmt.Errorf("读取 Partition %d offset 失败: %w", partition, offset.Err)
			}
			offsets.Add(kadm.Offset{Topic: req.Topic, Partition: partition, At: offset.Offset, LeaderEpoch: offset.LeaderEpoch})
		}
	default:
		return nil, fmt.Errorf("不支持的 Offset 重置模式: %s", req.Mode)
	}
	return offsets, nil
}

func listedOffsetsForReset(ctx context.Context, admin *kadm.Client, req ResetConsumerGroupOffsetRequest) (kadm.ListedOffsets, error) {
	switch req.Mode {
	case "earliest":
		offsets, err := admin.ListStartOffsets(ctx, req.Topic)
		if err != nil {
			return nil, fmt.Errorf("读取 Kafka 起始 offset 失败: %w", err)
		}
		return offsets, nil
	case "latest":
		offsets, err := admin.ListEndOffsets(ctx, req.Topic)
		if err != nil {
			return nil, fmt.Errorf("读取 Kafka 最新 offset 失败: %w", err)
		}
		return offsets, nil
	case "timestamp":
		offsets, err := admin.ListOffsetsAfterMilli(ctx, req.TimestampMillis, req.Topic)
		if err != nil {
			return nil, fmt.Errorf("按时间读取 Kafka offset 失败: %w", err)
		}
		return offsets, nil
	default:
		return nil, fmt.Errorf("不支持的 Offset 重置模式: %s", req.Mode)
	}
}
