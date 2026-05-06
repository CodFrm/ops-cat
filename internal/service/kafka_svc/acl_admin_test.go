package kafka_svc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kmsg"
)

func TestNormalizeListACLsRequest(t *testing.T) {
	filter, err := normalizeListACLsRequest(ListACLsRequest{Page: -1, PageSize: 0})
	require.NoError(t, err)
	assert.Equal(t, kmsg.ACLResourceTypeAny, filter.resourceType)
	assert.Equal(t, kmsg.ACLResourcePatternTypeAny, filter.patternType)
	assert.Equal(t, kmsg.ACLOperationAny, filter.operation)
	assert.Equal(t, kmsg.ACLPermissionTypeAny, filter.permission)
	assert.Equal(t, 1, filter.page)
	assert.Equal(t, 50, filter.pageSize)

	filter, err = normalizeListACLsRequest(ListACLsRequest{
		ResourceType: "topic",
		ResourceName: " orders ",
		Principal:    " User:alice ",
		Operation:    "read",
		Permission:   "allow",
	})
	require.NoError(t, err)
	assert.Equal(t, kmsg.ACLResourceTypeTopic, filter.resourceType)
	assert.Equal(t, "orders", filter.resourceName)
	assert.Equal(t, kmsg.ACLResourcePatternTypeMatch, filter.patternType)
	assert.Equal(t, "User:alice", filter.principal)
	assert.Equal(t, kmsg.ACLOperationRead, filter.operation)
	assert.Equal(t, kmsg.ACLPermissionTypeAllow, filter.permission)
}

func TestNormalizeCreateACLRequest(t *testing.T) {
	filter, err := normalizeCreateACLRequest(CreateACLRequest{
		ResourceType: "topic",
		ResourceName: "orders",
		Principal:    "User:alice",
		Operation:    "read",
		Permission:   "allow",
	})
	require.NoError(t, err)
	assert.Equal(t, kmsg.ACLResourceTypeTopic, filter.resourceType)
	assert.Equal(t, kmsg.ACLResourcePatternTypeLiteral, filter.patternType)
	assert.Equal(t, kmsg.ACLOperationRead, filter.operation)
	assert.Equal(t, kmsg.ACLPermissionTypeAllow, filter.permission)

	filter, err = normalizeCreateACLRequest(CreateACLRequest{
		ResourceType: "cluster",
		Principal:    "User:admin",
		Operation:    "describe",
		Permission:   "allow",
	})
	require.NoError(t, err)
	assert.Equal(t, "kafka-cluster", filter.resourceName)

	_, err = normalizeCreateACLRequest(CreateACLRequest{
		ResourceType: "topic",
		Principal:    "User:alice",
		Operation:    "read",
		Permission:   "allow",
	})
	assert.Error(t, err)

	_, err = normalizeCreateACLRequest(CreateACLRequest{
		ResourceType: "topic",
		ResourceName: "orders",
		Principal:    "User:alice",
		Operation:    "any",
		Permission:   "allow",
	})
	assert.Error(t, err)

	_, err = normalizeCreateACLRequest(CreateACLRequest{
		ResourceType: "topic",
		ResourceName: "orders",
		Principal:    "User:alice",
		Operation:    "read",
		Permission:   "any",
	})
	assert.Error(t, err)
}

func TestNormalizeDeleteACLRequestRequiresExactFilter(t *testing.T) {
	filter, err := normalizeDeleteACLRequest(DeleteACLRequest{
		ResourceType: "group",
		ResourceName: "billing",
		Principal:    "User:alice",
		Host:         "*",
		Operation:    "read",
		Permission:   "deny",
	})
	require.NoError(t, err)
	assert.Equal(t, kmsg.ACLResourceTypeGroup, filter.resourceType)
	assert.Equal(t, kmsg.ACLPermissionTypeDeny, filter.permission)

	_, err = normalizeDeleteACLRequest(DeleteACLRequest{
		ResourceType: "group",
		ResourceName: "billing",
		Principal:    "User:alice",
		Operation:    "read",
		Permission:   "deny",
	})
	assert.Error(t, err)

	_, err = normalizeDeleteACLRequest(DeleteACLRequest{
		ResourceType: "any",
		ResourceName: "billing",
		Principal:    "User:alice",
		Host:         "*",
		Operation:    "read",
		Permission:   "deny",
	})
	assert.Error(t, err)
}

func TestListACLsResponseSortsAndPaginates(t *testing.T) {
	response := listACLsResponse([]KafkaACL{
		{ResourceType: "TOPIC", ResourceName: "b", Principal: "User:b"},
		{ResourceType: "TOPIC", ResourceName: "a", Principal: "User:a"},
	}, 1, 1)
	require.Len(t, response.ACLs, 1)
	assert.Equal(t, 2, response.Total)
	assert.Equal(t, "a", response.ACLs[0].ResourceName)
}
