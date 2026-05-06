package kafka_svc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeResetConsumerGroupOffsetRequest(t *testing.T) {
	req, err := normalizeResetConsumerGroupOffsetRequest(ResetConsumerGroupOffsetRequest{
		Group: " billing ",
		Topic: " orders ",
	})
	require.NoError(t, err)
	assert.Equal(t, "billing", req.Group)
	assert.Equal(t, "orders", req.Topic)
	assert.Equal(t, "latest", req.Mode)

	req, err = normalizeResetConsumerGroupOffsetRequest(ResetConsumerGroupOffsetRequest{
		Group:  "billing",
		Topic:  "orders",
		Mode:   "offset",
		Offset: 42,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(42), req.Offset)

	_, err = normalizeResetConsumerGroupOffsetRequest(ResetConsumerGroupOffsetRequest{Group: "", Topic: "orders"})
	assert.Error(t, err)

	_, err = normalizeResetConsumerGroupOffsetRequest(ResetConsumerGroupOffsetRequest{Group: "billing", Topic: ""})
	assert.Error(t, err)

	_, err = normalizeResetConsumerGroupOffsetRequest(ResetConsumerGroupOffsetRequest{
		Group:  "billing",
		Topic:  "orders",
		Mode:   "offset",
		Offset: -1,
	})
	assert.Error(t, err)

	_, err = normalizeResetConsumerGroupOffsetRequest(ResetConsumerGroupOffsetRequest{
		Group: "billing",
		Topic: "orders",
		Mode:  "timestamp",
	})
	assert.Error(t, err)

	_, err = normalizeResetConsumerGroupOffsetRequest(ResetConsumerGroupOffsetRequest{
		Group:      "billing",
		Topic:      "orders",
		Mode:       "latest",
		Partitions: []int32{-1},
	})
	assert.Error(t, err)
}
