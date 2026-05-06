package ai

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/model/entity/grant_entity"
	"github.com/opskat/opskat/internal/repository/grant_repo"
)

func TestMatchKafkaRule(t *testing.T) {
	tests := []struct {
		name    string
		rule    string
		command string
		want    bool
	}{
		{name: "exact action and resource", rule: "topic.read orders", command: "topic.read orders", want: true},
		{name: "action wildcard matches three level action", rule: "topic.* *", command: "topic.config.write orders", want: true},
		{name: "resource wildcard", rule: "message.read orders-*", command: "message.read orders-2026", want: true},
		{name: "action is case insensitive", rule: "TOPIC.READ orders", command: "topic.read orders", want: true},
		{name: "resource is case sensitive", rule: "topic.read Orders", command: "topic.read orders", want: false},
		{name: "invalid rule shape", rule: "topic.read", command: "topic.read orders", want: false},
		{name: "invalid command shape", rule: "topic.read *", command: "topic.read orders extra", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, MatchKafkaRule(tt.rule, tt.command))
		})
	}
}

func TestCheckKafkaPolicy(t *testing.T) {
	ctx := context.Background()
	p := &asset_entity.KafkaPolicy{
		AllowList: []string{"topic.* *", "message.read orders-*", "message.write *"},
		DenyList:  []string{"topic.delete *", "message.write prod-*"},
	}

	denied := CheckKafkaPolicy(ctx, p, "topic.delete orders")
	assert.Equal(t, Deny, denied.Decision)
	assert.Equal(t, SourcePolicyDeny, denied.DecisionSource)
	assert.Equal(t, "topic.delete *", denied.MatchedPattern)

	allowed := CheckKafkaPolicy(ctx, p, "topic.config.write orders")
	assert.Equal(t, Allow, allowed.Decision)
	assert.Equal(t, SourcePolicyAllow, allowed.DecisionSource)
	assert.Equal(t, "topic.* *", allowed.MatchedPattern)

	allowed = CheckKafkaPolicy(ctx, p, "message.write orders")
	assert.Equal(t, Allow, allowed.Decision)
	assert.Equal(t, SourcePolicyAllow, allowed.DecisionSource)
	assert.Equal(t, "message.write *", allowed.MatchedPattern)

	denied = CheckKafkaPolicy(ctx, p, "message.write prod-orders")
	assert.Equal(t, Deny, denied.Decision)
	assert.Equal(t, SourcePolicyDeny, denied.DecisionSource)
	assert.Equal(t, "message.write prod-*", denied.MatchedPattern)

	needConfirm := CheckKafkaPolicy(ctx, p, "message.read invoices")
	assert.Equal(t, NeedConfirm, needConfirm.Decision)
}

func TestCheckKafkaPolicy_TopicAdminDenyWins(t *testing.T) {
	ctx := context.Background()
	p := &asset_entity.KafkaPolicy{
		AllowList: []string{
			"topic.create *",
			"topic.config.write *",
			"topic.partitions.write *",
			"topic.records.delete *",
			"consumer_group.offset.write *",
			"consumer_group.delete *",
		},
		DenyList: []string{"topic.delete *", "topic.records.delete *", "consumer_group.delete *"},
	}

	created := CheckKafkaPolicy(ctx, p, "topic.create orders")
	assert.Equal(t, Allow, created.Decision)

	config := CheckKafkaPolicy(ctx, p, "topic.config.write orders")
	assert.Equal(t, Allow, config.Decision)

	partitions := CheckKafkaPolicy(ctx, p, "topic.partitions.write orders")
	assert.Equal(t, Allow, partitions.Decision)

	deleted := CheckKafkaPolicy(ctx, p, "topic.delete orders")
	assert.Equal(t, Deny, deleted.Decision)

	records := CheckKafkaPolicy(ctx, p, "topic.records.delete orders")
	assert.Equal(t, Deny, records.Decision)
	assert.Equal(t, "topic.records.delete *", records.MatchedPattern)

	reset := CheckKafkaPolicy(ctx, p, "consumer_group.offset.write billing")
	assert.Equal(t, Allow, reset.Decision)

	deleteGroup := CheckKafkaPolicy(ctx, p, "consumer_group.delete billing")
	assert.Equal(t, Deny, deleteGroup.Decision)
	assert.Equal(t, "consumer_group.delete *", deleteGroup.MatchedPattern)
}

func TestCheckPermission_Kafka(t *testing.T) {
	ctx, mockAsset, _ := setupPolicyTest(t)
	asset := &asset_entity.Asset{
		ID:   1,
		Type: asset_entity.AssetTypeKafka,
		CmdPolicy: mustJSON(asset_entity.KafkaPolicy{
			AllowList: []string{"topic.read orders-*"},
			DenyList:  []string{"topic.delete *"},
		}),
	}
	mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

	allowed := CheckPermission(ctx, asset_entity.AssetTypeKafka, 1, "topic.read orders-2026")
	assert.Equal(t, Allow, allowed.Decision)
	assert.Equal(t, SourcePolicyAllow, allowed.DecisionSource)

	denied := CheckPermission(ctx, asset_entity.AssetTypeKafka, 1, "topic.delete orders")
	assert.Equal(t, Deny, denied.Decision)
	assert.Equal(t, SourcePolicyDeny, denied.DecisionSource)

	needConfirm := CheckPermission(ctx, asset_entity.AssetTypeKafka, 1, "message.read orders")
	assert.Equal(t, NeedConfirm, needConfirm.Decision)
	assert.Contains(t, needConfirm.HintRules, "topic.read orders-*")
}

func TestCheckPermission_KafkaGrantUsesKafkaMatcher(t *testing.T) {
	ctx, mockAsset, _ := setupPolicyTest(t)
	asset := &asset_entity.Asset{
		ID:   1,
		Type: asset_entity.AssetTypeKafka,
		CmdPolicy: mustJSON(asset_entity.KafkaPolicy{
			AllowList: []string{"topic.read *"},
		}),
	}
	mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

	stubGrant := newStubGrantRepo()
	origGrant := grant_repo.Grant()
	grant_repo.RegisterGrant(stubGrant)
	t.Cleanup(func() {
		if origGrant != nil {
			grant_repo.RegisterGrant(origGrant)
		}
	})

	stubGrant.sessions["sess-1"] = &grant_entity.GrantSession{
		ID:     "sess-1",
		Status: grant_entity.GrantStatusApproved,
	}
	stubGrant.items["sess-1"] = []*grant_entity.GrantItem{
		{GrantSessionID: "sess-1", AssetID: 1, Command: "message.* orders"},
	}

	result := CheckPermission(WithSessionID(ctx, "sess-1"), asset_entity.AssetTypeKafka, 1, "message.read orders")
	assert.Equal(t, Allow, result.Decision)
	assert.Equal(t, SourceGrantAllow, result.DecisionSource)
	assert.Equal(t, "message.* orders", result.MatchedPattern)
}

func TestCommandPolicyChecker_KafkaApprovalType(t *testing.T) {
	ctx, mockAsset, _ := setupPolicyTest(t)
	asset := &asset_entity.Asset{
		ID:   1,
		Name: "local-kafka",
		Type: asset_entity.AssetTypeKafka,
		CmdPolicy: mustJSON(asset_entity.KafkaPolicy{
			AllowList: []string{"topic.read *"},
		}),
	}
	mockAsset.EXPECT().Find(gomock.Any(), int64(1)).Return(asset, nil).AnyTimes()

	var approvalType string
	checker := NewCommandPolicyChecker(func(_ context.Context, _ string, items []ApprovalItem, _ string) ApprovalResponse {
		if len(items) > 0 {
			approvalType = items[0].Type
		}
		return ApprovalResponse{Decision: "allow"}
	})

	result := checker.CheckForAsset(ctx, 1, asset_entity.AssetTypeKafka, "message.read orders")
	assert.Equal(t, Allow, result.Decision)
	assert.Equal(t, SourceUserAllow, result.DecisionSource)
	assert.Equal(t, "kafka", approvalType)
}
