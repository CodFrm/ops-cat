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

func (s *Service) GetBrokerConfig(ctx context.Context, assetID int64, brokerID int32) (BrokerConfigResponse, error) {
	var out BrokerConfigResponse
	err := s.withClient(ctx, assetID, func(ctx context.Context, _ *kgo.Client, admin *kadm.Client, _ *asset_entity.Asset, _ *asset_entity.KafkaConfig) error {
		out.BrokerID = brokerID
		resources, err := admin.DescribeBrokerConfigs(ctx, brokerID)
		if err != nil {
			return fmt.Errorf("读取 Broker 配置失败: %w", err)
		}
		if len(resources) == 0 {
			return fmt.Errorf("broker 配置响应为空: %d", brokerID)
		}
		rc := resources[0]
		if rc.Err != nil {
			out.Error = rc.Err.Error()
			return nil //nolint:nilerr // Kafka per-resource errors are exposed in the response payload.
		}
		out.Configs = configEntriesFromKadm(rc.Configs)
		return nil
	})
	return out, err
}

func (s *Service) ListClusterConfigs(ctx context.Context, assetID int64) (ClusterConfigsResponse, error) {
	var out ClusterConfigsResponse
	err := s.withClient(ctx, assetID, func(ctx context.Context, _ *kgo.Client, admin *kadm.Client, _ *asset_entity.Asset, _ *asset_entity.KafkaConfig) error {
		metadata, err := admin.Metadata(ctx)
		if err != nil {
			return fmt.Errorf("读取集群元数据失败: %w", err)
		}
		resources, err := admin.DescribeBrokerConfigs(ctx, metadata.Controller)
		if err != nil {
			return fmt.Errorf("读取集群配置失败: %w", err)
		}
		if len(resources) == 0 {
			return fmt.Errorf("集群配置响应为空")
		}
		rc := resources[0]
		if rc.Err != nil {
			out.Error = rc.Err.Error()
			return nil //nolint:nilerr // Kafka per-resource errors are exposed in the response payload.
		}
		out.Configs = configEntriesFromKadm(rc.Configs)
		return nil
	})
	return out, err
}

func configEntriesFromKadm(configs []kadm.Config) []ConfigEntry {
	out := make([]ConfigEntry, 0, len(configs))
	for _, c := range configs {
		entry := ConfigEntry{
			Name:        c.Key,
			IsSensitive: c.Sensitive,
			Source:      c.Source.String(),
		}
		if c.Value != nil {
			entry.Value = *c.Value
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *Service) ClusterOverview(ctx context.Context, assetID int64) (ClusterOverview, error) {
	var out ClusterOverview
	err := s.withClient(ctx, assetID, func(ctx context.Context, _ *kgo.Client, admin *kadm.Client, _ *asset_entity.Asset, _ *asset_entity.KafkaConfig) error {
		metadata, err := admin.Metadata(ctx)
		if err != nil {
			return fmt.Errorf("读取 Kafka 集群元数据失败: %w", err)
		}
		out = clusterOverviewFromMetadata(assetID, metadata)
		return nil
	})
	return out, err
}

func (s *Service) ListBrokers(ctx context.Context, assetID int64) ([]Broker, error) {
	var out []Broker
	err := s.withClient(ctx, assetID, func(ctx context.Context, _ *kgo.Client, admin *kadm.Client, _ *asset_entity.Asset, _ *asset_entity.KafkaConfig) error {
		brokers, err := admin.ListBrokers(ctx)
		if err != nil {
			return fmt.Errorf("读取 Kafka Broker 列表失败: %w", err)
		}
		out = brokersFromKadm(brokers)
		return nil
	})
	return out, err
}

func (s *Service) ListTopics(ctx context.Context, req ListTopicsRequest) (ListTopicsResponse, error) {
	var out ListTopicsResponse
	err := s.withClient(ctx, req.AssetID, func(ctx context.Context, _ *kgo.Client, admin *kadm.Client, _ *asset_entity.Asset, _ *asset_entity.KafkaConfig) error {
		topics, err := listTopicDetails(ctx, admin, req.IncludeInternal)
		if err != nil {
			return fmt.Errorf("读取 Kafka Topic 列表失败: %w", err)
		}
		out = listTopicsResponse(topics, req)
		return nil
	})
	return out, err
}

func (s *Service) GetTopic(ctx context.Context, assetID int64, topic string) (TopicDetail, error) {
	var out TopicDetail
	err := s.withClient(ctx, assetID, func(ctx context.Context, _ *kgo.Client, admin *kadm.Client, _ *asset_entity.Asset, _ *asset_entity.KafkaConfig) error {
		name := strings.TrimSpace(topic)
		if name == "" {
			return fmt.Errorf("topic 不能为空")
		}
		topics, err := admin.ListTopicsWithInternal(ctx, name)
		if err != nil {
			return fmt.Errorf("读取 Kafka Topic 详情失败: %w", err)
		}
		detail, ok := topics[name]
		if !ok {
			return fmt.Errorf("topic 不存在: %s", name)
		}
		out = topicDetailFromKadm(detail)
		return nil
	})
	return out, err
}

func (s *Service) ListConsumerGroups(ctx context.Context, assetID int64) ([]ConsumerGroup, error) {
	var out []ConsumerGroup
	err := s.withClient(ctx, assetID, func(ctx context.Context, _ *kgo.Client, admin *kadm.Client, _ *asset_entity.Asset, _ *asset_entity.KafkaConfig) error {
		groups, err := admin.ListGroups(ctx)
		if err != nil {
			return fmt.Errorf("读取 Kafka Consumer Group 列表失败: %w", err)
		}
		out = consumerGroupsFromKadm(groups)
		return nil
	})
	return out, err
}

func (s *Service) GetConsumerGroup(ctx context.Context, assetID int64, group string) (ConsumerGroupDetail, error) {
	var out ConsumerGroupDetail
	err := s.withClient(ctx, assetID, func(ctx context.Context, _ *kgo.Client, admin *kadm.Client, _ *asset_entity.Asset, _ *asset_entity.KafkaConfig) error {
		name := strings.TrimSpace(group)
		if name == "" {
			return fmt.Errorf("consumer group 不能为空")
		}
		groups, err := admin.DescribeGroups(ctx, name)
		if err != nil {
			return fmt.Errorf("读取 Kafka Consumer Group 详情失败: %w", err)
		}
		detail, ok := groups[name]
		if !ok {
			return fmt.Errorf("consumer group 不存在: %s", name)
		}
		out = consumerGroupDetailFromKadm(detail)

		lags, lagErr := admin.Lag(ctx, name)
		if lagErr != nil {
			out.LagError = lagErr.Error()
			return nil //nolint:nilerr // lag lookup is optional; expose the per-operation error in the response payload.
		}
		if lag, ok := lags[name]; ok {
			applyConsumerGroupLag(&out, lag)
		}
		return nil
	})
	return out, err
}

func clusterOverviewFromMetadata(assetID int64, metadata kadm.Metadata) ClusterOverview {
	out := ClusterOverview{
		AssetID:      assetID,
		ClusterID:    metadata.Cluster,
		ControllerID: metadata.Controller,
		BrokerCount:  len(metadata.Brokers),
		TopicCount:   len(metadata.Topics),
	}
	for _, topic := range metadata.Topics {
		if topic.IsInternal {
			out.InternalTopicCount++
		}
		for _, partition := range topic.Partitions {
			out.PartitionCount++
			if partition.Leader < 0 {
				out.OfflinePartitionCount++
			}
			if len(partition.Replicas) > 0 && len(partition.ISR) < len(partition.Replicas) {
				out.UnderReplicatedPartitionCount++
			}
		}
	}
	return out
}

func brokersFromKadm(details kadm.BrokerDetails) []Broker {
	out := make([]Broker, 0, len(details))
	for _, detail := range details {
		out = append(out, brokerFromKadm(detail))
	}
	return out
}

func brokerFromKadm(detail kadm.BrokerDetail) Broker {
	broker := Broker{
		NodeID: detail.NodeID,
		Host:   detail.Host,
		Port:   detail.Port,
	}
	if detail.Rack != nil {
		broker.Rack = *detail.Rack
	}
	return broker
}

func listTopicDetails(ctx context.Context, admin *kadm.Client, includeInternal bool) (kadm.TopicDetails, error) {
	if includeInternal {
		return admin.ListTopicsWithInternal(ctx)
	}
	return admin.ListTopics(ctx)
}

func listTopicsResponse(topics kadm.TopicDetails, req ListTopicsRequest) ListTopicsResponse {
	page, pageSize := normalizePage(req.Page, req.PageSize)
	search := strings.ToLower(strings.TrimSpace(req.Search))
	summaries := make([]TopicSummary, 0, len(topics))
	for _, detail := range topics.Sorted() {
		summary := topicSummaryFromKadm(detail)
		if search != "" && !strings.Contains(strings.ToLower(summary.Name), search) {
			continue
		}
		summaries = append(summaries, summary)
	}

	total := len(summaries)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return ListTopicsResponse{
		Topics:   summaries[start:end],
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}
}

func topicSummaryFromKadm(detail kadm.TopicDetail) TopicSummary {
	summary := TopicSummary{
		Name:           detail.Topic,
		ID:             detail.ID.String(),
		Internal:       detail.IsInternal,
		PartitionCount: len(detail.Partitions),
		Error:          errorString(detail.Err),
	}
	for _, partition := range detail.Partitions {
		if len(partition.Replicas) > summary.ReplicationFactor {
			summary.ReplicationFactor = len(partition.Replicas)
		}
		if partition.Leader < 0 {
			summary.OfflinePartitionCount++
		}
		if len(partition.Replicas) > 0 && len(partition.ISR) < len(partition.Replicas) {
			summary.UnderReplicatedPartitionCount++
		}
	}
	return summary
}

func topicDetailFromKadm(detail kadm.TopicDetail) TopicDetail {
	out := TopicDetail{
		TopicSummary:         topicSummaryFromKadm(detail),
		Partitions:           make([]TopicPartition, 0, len(detail.Partitions)),
		AuthorizedOperations: aclOperationsToStrings(detail.AuthorizedOperations),
	}
	for _, partition := range detail.Partitions.Sorted() {
		out.Partitions = append(out.Partitions, TopicPartition{
			Partition:       partition.Partition,
			Leader:          partition.Leader,
			LeaderEpoch:     partition.LeaderEpoch,
			Replicas:        append([]int32(nil), partition.Replicas...),
			ISR:             append([]int32(nil), partition.ISR...),
			OfflineReplicas: append([]int32(nil), partition.OfflineReplicas...),
			Error:           errorString(partition.Err),
		})
	}
	return out
}

func consumerGroupsFromKadm(groups kadm.ListedGroups) []ConsumerGroup {
	out := make([]ConsumerGroup, 0, len(groups))
	for _, group := range groups.Sorted() {
		out = append(out, ConsumerGroup{
			Group:        group.Group,
			Coordinator:  group.Coordinator,
			ProtocolType: group.ProtocolType,
			State:        group.State,
		})
	}
	return out
}

func consumerGroupDetailFromKadm(group kadm.DescribedGroup) ConsumerGroupDetail {
	out := ConsumerGroupDetail{
		Group:        group.Group,
		Coordinator:  brokerFromKadm(group.Coordinator),
		State:        group.State,
		ProtocolType: group.ProtocolType,
		Protocol:     group.Protocol,
		Error:        errorString(group.Err),
		Members:      make([]ConsumerGroupMember, 0, len(group.Members)),
	}
	for _, member := range group.Members {
		out.Members = append(out.Members, consumerGroupMemberFromKadm(member))
	}
	return out
}

func consumerGroupMemberFromKadm(member kadm.DescribedGroupMember) ConsumerGroupMember {
	out := ConsumerGroupMember{
		MemberID:   member.MemberID,
		ClientID:   member.ClientID,
		ClientHost: member.ClientHost,
	}
	if member.InstanceID != nil {
		out.InstanceID = *member.InstanceID
	}
	if assignment, ok := member.Assigned.AsConsumer(); ok {
		out.AssignedPartitions = make([]TopicPartitionAssignment, 0, len(assignment.Topics))
		for _, topic := range assignment.Topics {
			partitions := append([]int32(nil), topic.Partitions...)
			sort.Slice(partitions, func(i, j int) bool { return partitions[i] < partitions[j] })
			out.AssignedPartitions = append(out.AssignedPartitions, TopicPartitionAssignment{
				Topic:      topic.Topic,
				Partitions: partitions,
			})
		}
		sort.Slice(out.AssignedPartitions, func(i, j int) bool {
			return out.AssignedPartitions[i].Topic < out.AssignedPartitions[j].Topic
		})
	}
	return out
}

func applyConsumerGroupLag(out *ConsumerGroupDetail, lag kadm.DescribedGroupLag) {
	out.TotalLag = lag.Lag.Total()
	out.Lag = make([]ConsumerGroupPartitionLag, 0)
	for _, partitionLag := range lag.Lag.Sorted() {
		entry := ConsumerGroupPartitionLag{
			Topic:           partitionLag.Topic,
			Partition:       partitionLag.Partition,
			CommittedOffset: partitionLag.Commit.At,
			EndOffset:       partitionLag.End.Offset,
			Lag:             partitionLag.Lag,
			Error:           errorString(partitionLag.Err),
		}
		if partitionLag.Member != nil {
			entry.MemberID = partitionLag.Member.MemberID
		}
		out.Lag = append(out.Lag, entry)
	}
	if lag.Error() != nil {
		out.LagError = lag.Error().Error()
	}
}

func aclOperationsToStrings(ops []kadm.ACLOperation) []string {
	if len(ops) == 0 {
		return nil
	}
	out := make([]string, 0, len(ops))
	for _, op := range ops {
		out = append(out, op.String())
	}
	return out
}

func normalizePage(page, pageSize int) (int, int) {
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

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
