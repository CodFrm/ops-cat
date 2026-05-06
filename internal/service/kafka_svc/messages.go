package kafka_svc

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
)

const (
	defaultKafkaMessageFetchLimit   = 50
	maxKafkaMessageFetchLimit       = 1000
	defaultKafkaMessagePreviewBytes = 4096
	maxKafkaMessagePreviewBytes     = 1024 * 1024
	defaultKafkaMessageWaitMillis   = 1000
	maxKafkaMessageWaitMillis       = 30000
)

type normalizedBrowseRequest struct {
	BrowseMessagesRequest
	topic         string
	startMode     string
	limit         int
	maxBytes      int
	decodeMode    string
	maxWaitMillis int
}

// BrowseMessages reads a bounded message preview from a topic. It uses a one-off
// franz-go client so direct partition consumption state never leaks into the UI
// metadata client cache.
func (s *Service) BrowseMessages(ctx context.Context, req BrowseMessagesRequest) (BrowseMessagesResponse, error) {
	var out BrowseMessagesResponse
	err := s.withOneOffClient(ctx, req.AssetID, []kgo.Opt{
		kgo.ConsumePartitions(map[string]map[int32]kgo.Offset{}),
	}, func(ctx context.Context, client *kgo.Client, admin *kadm.Client, _ *asset_entity.Asset, cfg *asset_entity.KafkaConfig) error {
		normalized, err := normalizeBrowseMessagesRequest(req, cfg)
		if err != nil {
			return err
		}
		partitions, err := browseTopicPartitions(ctx, admin, normalized.topic, normalized.Partition)
		if err != nil {
			return err
		}
		offsets, err := browseStartOffsets(ctx, admin, normalized, partitions)
		if err != nil {
			return err
		}
		client.AddConsumePartitions(map[string]map[int32]kgo.Offset{normalized.topic: offsets})

		records, nextOffsets, pollErrors := pollKafkaRecords(ctx, client, normalized)
		out = BrowseMessagesResponse{
			Topic:      normalized.topic,
			Partitions: partitions,
			StartMode:  normalized.startMode,
			Limit:      normalized.limit,
			MaxBytes:   normalized.maxBytes,
			Records:    records,
			NextOffset: nextOffsets,
			Errors:     pollErrors,
		}
		return nil
	})
	return out, err
}

func (s *Service) ProduceMessage(ctx context.Context, req ProduceMessageRequest) (ProduceMessageResponse, error) {
	var out ProduceMessageResponse
	topic := strings.TrimSpace(req.Topic)
	if topic == "" {
		return out, fmt.Errorf("topic 不能为空")
	}
	if req.Partition != nil && *req.Partition < 0 {
		return out, fmt.Errorf("partition 不能小于0")
	}

	extraOpts := []kgo.Opt(nil)
	if req.Partition != nil {
		extraOpts = append(extraOpts, kgo.RecordPartitioner(kgo.ManualPartitioner()))
	}

	err := s.withOneOffClient(ctx, req.AssetID, extraOpts, func(ctx context.Context, client *kgo.Client, _ *kadm.Client, _ *asset_entity.Asset, _ *asset_entity.KafkaConfig) error {
		key, err := decodeKafkaInput(req.Key, req.KeyEncoding)
		if err != nil {
			return fmt.Errorf("解析消息 key 失败: %w", err)
		}
		value, err := decodeKafkaInput(req.Value, req.ValueEncoding)
		if err != nil {
			return fmt.Errorf("解析消息 value 失败: %w", err)
		}
		headers, err := produceHeaders(req.Headers)
		if err != nil {
			return err
		}

		record := &kgo.Record{
			Topic:   topic,
			Key:     key,
			Value:   value,
			Headers: headers,
		}
		if req.Partition != nil {
			record.Partition = *req.Partition
		}
		if req.TimestampMillis > 0 {
			record.Timestamp = time.UnixMilli(req.TimestampMillis)
		}

		produced, err := client.ProduceSync(ctx, record).First()
		if err != nil {
			return fmt.Errorf("写入 Kafka 消息失败: %w", err)
		}
		out = ProduceMessageResponse{
			Topic:           produced.Topic,
			Partition:       produced.Partition,
			Offset:          produced.Offset,
			Timestamp:       produced.Timestamp.Format(time.RFC3339Nano),
			TimestampMillis: produced.Timestamp.UnixMilli(),
		}
		return nil
	})
	return out, err
}

func normalizeBrowseMessagesRequest(req BrowseMessagesRequest, cfg *asset_entity.KafkaConfig) (normalizedBrowseRequest, error) {
	topic := strings.TrimSpace(req.Topic)
	if topic == "" {
		return normalizedBrowseRequest{}, fmt.Errorf("topic 不能为空")
	}
	if req.Partition != nil && *req.Partition < 0 {
		return normalizedBrowseRequest{}, fmt.Errorf("partition 不能小于0")
	}

	startMode := strings.ToLower(strings.TrimSpace(req.StartMode))
	if startMode == "" {
		startMode = "newest"
	}
	switch startMode {
	case "newest", "oldest", "offset", "timestamp":
	default:
		return normalizedBrowseRequest{}, fmt.Errorf("不支持的 Kafka 消息起始模式: %s", req.StartMode)
	}
	if startMode == "offset" && req.Offset < 0 {
		return normalizedBrowseRequest{}, fmt.Errorf("offset 不能小于0")
	}
	if startMode == "timestamp" && req.TimestampMillis <= 0 {
		return normalizedBrowseRequest{}, fmt.Errorf("timestamp 不能为空")
	}

	limitDefault := defaultKafkaMessageFetchLimit
	maxBytesDefault := defaultKafkaMessagePreviewBytes
	if cfg != nil {
		if cfg.MessageFetchLimit > 0 {
			limitDefault = cfg.MessageFetchLimit
		}
		if cfg.MessagePreviewBytes > 0 {
			maxBytesDefault = cfg.MessagePreviewBytes
		}
	}

	limit := req.Limit
	if limit <= 0 {
		limit = limitDefault
	}
	if limit <= 0 {
		limit = defaultKafkaMessageFetchLimit
	}
	if limit > maxKafkaMessageFetchLimit {
		limit = maxKafkaMessageFetchLimit
	}

	maxBytes := req.MaxBytes
	if maxBytes <= 0 {
		maxBytes = maxBytesDefault
	}
	if maxBytes <= 0 {
		maxBytes = defaultKafkaMessagePreviewBytes
	}
	if maxBytes > maxKafkaMessagePreviewBytes {
		maxBytes = maxKafkaMessagePreviewBytes
	}

	decodeMode := strings.ToLower(strings.TrimSpace(req.DecodeMode))
	if decodeMode == "" {
		decodeMode = "text"
	}
	switch decodeMode {
	case "text", "json", "hex", "base64":
	default:
		return normalizedBrowseRequest{}, fmt.Errorf("不支持的 Kafka 消息解码模式: %s", req.DecodeMode)
	}

	maxWaitMillis := req.MaxWaitMillis
	if maxWaitMillis <= 0 {
		maxWaitMillis = defaultKafkaMessageWaitMillis
	}
	if maxWaitMillis > maxKafkaMessageWaitMillis {
		maxWaitMillis = maxKafkaMessageWaitMillis
	}

	return normalizedBrowseRequest{
		BrowseMessagesRequest: req,
		topic:                 topic,
		startMode:             startMode,
		limit:                 limit,
		maxBytes:              maxBytes,
		decodeMode:            decodeMode,
		maxWaitMillis:         maxWaitMillis,
	}, nil
}

func browseTopicPartitions(ctx context.Context, admin *kadm.Client, topic string, selected *int32) ([]int32, error) {
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

	partitions := make([]int32, 0, len(detail.Partitions))
	exists := map[int32]bool{}
	for _, partition := range detail.Partitions {
		exists[partition.Partition] = true
		if selected == nil {
			partitions = append(partitions, partition.Partition)
		}
	}
	if selected != nil {
		if !exists[*selected] {
			return nil, fmt.Errorf("topic %s 不存在 partition %d", topic, *selected)
		}
		partitions = append(partitions, *selected)
	}
	sort.Slice(partitions, func(i, j int) bool { return partitions[i] < partitions[j] })
	return partitions, nil
}

func browseStartOffsets(ctx context.Context, admin *kadm.Client, req normalizedBrowseRequest, partitions []int32) (map[int32]kgo.Offset, error) {
	out := make(map[int32]kgo.Offset, len(partitions))
	switch req.startMode {
	case "oldest":
		for _, partition := range partitions {
			out[partition] = kgo.NewOffset().AtStart()
		}
	case "offset":
		for _, partition := range partitions {
			out[partition] = kgo.NewOffset().At(req.Offset)
		}
	case "timestamp":
		for _, partition := range partitions {
			out[partition] = kgo.NewOffset().AfterMilli(req.TimestampMillis)
		}
	case "newest":
		starts, err := admin.ListStartOffsets(ctx, req.topic)
		if err != nil {
			return nil, fmt.Errorf("读取 Kafka 起始 offset 失败: %w", err)
		}
		ends, err := admin.ListEndOffsets(ctx, req.topic)
		if err != nil {
			return nil, fmt.Errorf("读取 Kafka 最新 offset 失败: %w", err)
		}
		for _, partition := range partitions {
			startOffset := int64(0)
			if start, ok := starts.Lookup(req.topic, partition); ok {
				if start.Err != nil {
					return nil, fmt.Errorf("读取 Partition %d 起始 offset 失败: %w", partition, start.Err)
				}
				startOffset = start.Offset
			}
			end, ok := ends.Lookup(req.topic, partition)
			if !ok {
				return nil, fmt.Errorf("读取 Partition %d 最新 offset 失败", partition)
			}
			if end.Err != nil {
				return nil, fmt.Errorf("读取 Partition %d 最新 offset 失败: %w", partition, end.Err)
			}
			offset := end.Offset - int64(req.limit)
			if offset < startOffset {
				offset = startOffset
			}
			out[partition] = kgo.NewOffset().At(offset)
		}
	default:
		return nil, fmt.Errorf("不支持的 Kafka 消息起始模式: %s", req.startMode)
	}
	return out, nil
}

func pollKafkaRecords(ctx context.Context, client *kgo.Client, req normalizedBrowseRequest) ([]KafkaRecord, map[int32]int64, []string) {
	records := make([]KafkaRecord, 0, req.limit)
	nextOffsets := make(map[int32]int64)
	var pollErrors []string

	pollCtx, cancel := context.WithTimeout(ctx, time.Duration(req.maxWaitMillis)*time.Millisecond)
	defer cancel()

	for len(records) < req.limit {
		fetches := client.PollRecords(pollCtx, req.limit-len(records))
		fetches.EachError(func(topic string, partition int32, err error) {
			if err == nil || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				return
			}
			pollErrors = append(pollErrors, fmt.Sprintf("%s[%d]: %s", topic, partition, err.Error()))
		})
		fetches.EachRecord(func(record *kgo.Record) {
			if len(records) >= req.limit || record.Topic != req.topic {
				return
			}
			records = append(records, kafkaRecordPreview(record, req.maxBytes, req.decodeMode))
			nextOffsets[record.Partition] = record.Offset + 1
		})
		if pollCtx.Err() != nil || fetches.Empty() {
			break
		}
	}
	if len(nextOffsets) == 0 {
		nextOffsets = nil
	}
	return records, nextOffsets, pollErrors
}

func kafkaRecordPreview(record *kgo.Record, maxBytes int, decodeMode string) KafkaRecord {
	key, keyEncoding, keyTruncated := renderKafkaBytes(record.Key, maxBytes, decodeMode)
	value, valueEncoding, valueTruncated := renderKafkaBytes(record.Value, maxBytes, decodeMode)
	out := KafkaRecord{
		Topic:           record.Topic,
		Partition:       record.Partition,
		Offset:          record.Offset,
		Timestamp:       record.Timestamp.Format(time.RFC3339Nano),
		TimestampMillis: record.Timestamp.UnixMilli(),
		Key:             key,
		KeyBytes:        len(record.Key),
		KeyEncoding:     keyEncoding,
		KeyTruncated:    keyTruncated,
		Value:           value,
		ValueBytes:      len(record.Value),
		ValueEncoding:   valueEncoding,
		ValueTruncated:  valueTruncated,
	}
	if len(record.Headers) > 0 {
		out.Headers = make([]KafkaRecordHeader, 0, len(record.Headers))
		for _, header := range record.Headers {
			headerValue, headerEncoding, headerTruncated := renderKafkaBytes(header.Value, maxBytes, decodeMode)
			out.Headers = append(out.Headers, KafkaRecordHeader{
				Key:            header.Key,
				Value:          headerValue,
				ValueBytes:     len(header.Value),
				ValueEncoding:  headerEncoding,
				ValueTruncated: headerTruncated,
			})
		}
	}
	return out
}

func renderKafkaBytes(value []byte, maxBytes int, decodeMode string) (string, string, bool) {
	if maxBytes <= 0 {
		maxBytes = defaultKafkaMessagePreviewBytes
	}
	truncated := false
	preview := value
	if len(preview) > maxBytes {
		preview = preview[:maxBytes]
		truncated = true
	}

	switch strings.ToLower(strings.TrimSpace(decodeMode)) {
	case "hex":
		return hex.EncodeToString(preview), "hex", truncated
	case "base64":
		return base64.StdEncoding.EncodeToString(preview), "base64", truncated
	case "json":
		if !truncated && json.Valid(preview) {
			var buf bytes.Buffer
			if err := json.Compact(&buf, preview); err == nil {
				return buf.String(), "json", false
			}
		}
	}

	if kafkaSafeText(preview) {
		return string(preview), "text", truncated
	}
	return base64.StdEncoding.EncodeToString(preview), "base64", truncated
}

func kafkaSafeText(value []byte) bool {
	if !utf8.Valid(value) {
		return false
	}
	for len(value) > 0 {
		r, size := utf8.DecodeRune(value)
		if r < 32 && r != '\n' && r != '\r' && r != '\t' {
			return false
		}
		value = value[size:]
	}
	return true
}

func decodeKafkaInput(value, encoding string) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "", "text", "json":
		return []byte(value), nil
	case "base64":
		return base64.StdEncoding.DecodeString(strings.TrimSpace(value))
	case "hex":
		return hex.DecodeString(strings.TrimSpace(value))
	default:
		return nil, fmt.Errorf("不支持的编码模式: %s", encoding)
	}
}

func produceHeaders(headers []ProduceMessageHeader) ([]kgo.RecordHeader, error) {
	if len(headers) == 0 {
		return nil, nil
	}
	out := make([]kgo.RecordHeader, 0, len(headers))
	for _, header := range headers {
		key := strings.TrimSpace(header.Key)
		if key == "" {
			return nil, fmt.Errorf("header key 不能为空")
		}
		value, err := decodeKafkaInput(header.Value, header.Encoding)
		if err != nil {
			return nil, fmt.Errorf("解析 Header %s 失败: %w", key, err)
		}
		out = append(out, kgo.RecordHeader{Key: key, Value: value})
	}
	return out, nil
}
