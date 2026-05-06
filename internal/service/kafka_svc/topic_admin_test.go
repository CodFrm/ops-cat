package kafka_svc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kadm"
)

func TestTopicConfigMap(t *testing.T) {
	configs := topicConfigMap(map[string]string{
		" retention.ms ": "60000",
		"":               "ignored",
	})
	require.Len(t, configs, 1)
	require.NotNil(t, configs["retention.ms"])
	assert.Equal(t, "60000", *configs["retention.ms"])
}

func TestTopicConfigMutations(t *testing.T) {
	configs, err := topicConfigMutations([]TopicConfigMutation{
		{Name: "retention.ms", Value: "60000"},
		{Name: "cleanup.policy", Op: "delete"},
		{Name: "segment.bytes", Value: "1048576", Op: "append"},
	})
	require.NoError(t, err)
	require.Len(t, configs, 3)
	assert.Equal(t, kadm.SetConfig, configs[0].Op)
	require.NotNil(t, configs[0].Value)
	assert.Equal(t, "60000", *configs[0].Value)
	assert.Equal(t, kadm.DeleteConfig, configs[1].Op)
	assert.Nil(t, configs[1].Value)
	assert.Equal(t, kadm.AppendConfig, configs[2].Op)

	_, err = topicConfigMutations(nil)
	assert.Error(t, err)

	_, err = topicConfigMutations([]TopicConfigMutation{{Name: "x", Op: "replace"}})
	assert.Error(t, err)

	_, err = topicConfigMutations([]TopicConfigMutation{{Name: ""}})
	assert.Error(t, err)
}
