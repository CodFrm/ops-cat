package kafka_svc

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
)

func TestNormalizeBrowseMessagesRequest(t *testing.T) {
	cfg := &asset_entity.KafkaConfig{MessageFetchLimit: 7, MessagePreviewBytes: 128}

	req, err := normalizeBrowseMessagesRequest(BrowseMessagesRequest{Topic: " orders "}, cfg)
	require.NoError(t, err)
	assert.Equal(t, "orders", req.topic)
	assert.Equal(t, "newest", req.startMode)
	assert.Equal(t, 7, req.limit)
	assert.Equal(t, 128, req.maxBytes)
	assert.Equal(t, "text", req.decodeMode)
	assert.Equal(t, defaultKafkaMessageWaitMillis, req.maxWaitMillis)

	req, err = normalizeBrowseMessagesRequest(BrowseMessagesRequest{
		Topic:         "orders",
		StartMode:     "oldest",
		Limit:         maxKafkaMessageFetchLimit + 10,
		MaxBytes:      maxKafkaMessagePreviewBytes + 10,
		DecodeMode:    "hex",
		MaxWaitMillis: maxKafkaMessageWaitMillis + 10,
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, maxKafkaMessageFetchLimit, req.limit)
	assert.Equal(t, maxKafkaMessagePreviewBytes, req.maxBytes)
	assert.Equal(t, "hex", req.decodeMode)
	assert.Equal(t, maxKafkaMessageWaitMillis, req.maxWaitMillis)

	_, err = normalizeBrowseMessagesRequest(BrowseMessagesRequest{Topic: "orders", StartMode: "offset", Offset: -1}, nil)
	assert.Error(t, err)

	_, err = normalizeBrowseMessagesRequest(BrowseMessagesRequest{Topic: "orders", DecodeMode: "yaml"}, nil)
	assert.Error(t, err)
}

func TestKafkaRecordPreviewTruncatesAndAvoidsBinaryText(t *testing.T) {
	record := &kgo.Record{
		Topic:     "orders",
		Partition: 2,
		Offset:    42,
		Timestamp: time.UnixMilli(1710000000000),
		Key:       []byte("abcdef"),
		Value:     []byte{0x01, 0x02, 0x03, 0x04, 0x05},
		Headers: []kgo.RecordHeader{
			{Key: "trace", Value: []byte("123456")},
		},
	}

	out := kafkaRecordPreview(record, 4, "text")
	assert.Equal(t, int32(2), out.Partition)
	assert.Equal(t, int64(42), out.Offset)
	assert.Equal(t, "abcd", out.Key)
	assert.Equal(t, 6, out.KeyBytes)
	assert.True(t, out.KeyTruncated)
	assert.Equal(t, "base64", out.ValueEncoding)
	assert.Equal(t, base64.StdEncoding.EncodeToString([]byte{0x01, 0x02, 0x03, 0x04}), out.Value)
	assert.True(t, out.ValueTruncated)
	require.Len(t, out.Headers, 1)
	assert.Equal(t, "1234", out.Headers[0].Value)
	assert.True(t, out.Headers[0].ValueTruncated)
}

func TestRenderKafkaBytesJSONAndProduceInput(t *testing.T) {
	value, encoding, truncated := renderKafkaBytes([]byte("{\"b\": 2, \"a\": 1}"), 128, "json")
	assert.Equal(t, "{\"b\":2,\"a\":1}", value)
	assert.Equal(t, "json", encoding)
	assert.False(t, truncated)

	decoded, err := decodeKafkaInput("68656c6c6f", "hex")
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), decoded)

	headers, err := produceHeaders([]ProduceMessageHeader{{Key: "trace", Value: "aGVsbG8=", Encoding: "base64"}})
	require.NoError(t, err)
	require.Len(t, headers, 1)
	assert.Equal(t, "trace", headers[0].Key)
	assert.Equal(t, []byte("hello"), headers[0].Value)

	_, err = produceHeaders([]ProduceMessageHeader{{Key: "", Value: "x"}})
	assert.Error(t, err)
}
