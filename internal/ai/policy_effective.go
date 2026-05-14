package ai

import (
	"context"
	"strings"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
)

func isWildcardAll(rule string) bool {
	return strings.TrimSpace(rule) == "*"
}

func policyValueMatches(rule, value string) bool {
	return isWildcardAll(rule) || strings.EqualFold(strings.TrimSpace(rule), strings.TrimSpace(value))
}

func containsPolicyValue(rules []string, value string) bool {
	for _, rule := range rules {
		if policyValueMatches(rule, value) {
			return true
		}
	}
	return false
}

func expandQueryPolicy(ctx context.Context, p *asset_entity.QueryPolicy) *asset_entity.QueryPolicy {
	out := &asset_entity.QueryPolicy{}
	if p == nil {
		return out
	}
	out.AllowTypes = append(out.AllowTypes, p.AllowTypes...)
	out.DenyTypes = append(out.DenyTypes, p.DenyTypes...)
	out.DenyFlags = append(out.DenyFlags, p.DenyFlags...)
	if len(p.Groups) > 0 {
		allowTypes, denyTypes, denyFlags := resolveQueryGroups(ctx, p.Groups)
		out.AllowTypes = append(out.AllowTypes, allowTypes...)
		out.DenyTypes = append(out.DenyTypes, denyTypes...)
		out.DenyFlags = append(out.DenyFlags, denyFlags...)
	}
	return out
}

func effectiveQueryPolicy(ctx context.Context, custom *asset_entity.QueryPolicy) *asset_entity.QueryPolicy {
	custom = expandQueryPolicy(ctx, custom)
	defaults := expandQueryPolicy(ctx, asset_entity.DefaultQueryPolicy())

	out := &asset_entity.QueryPolicy{}
	if len(custom.AllowTypes) > 0 {
		out.AllowTypes = appendUnique(out.AllowTypes, custom.AllowTypes...)
	} else {
		out.AllowTypes = appendUnique(out.AllowTypes, defaults.AllowTypes...)
	}
	out.DenyTypes = appendUnique(out.DenyTypes, custom.DenyTypes...)
	out.DenyTypes = appendUnique(out.DenyTypes, defaults.DenyTypes...)
	out.DenyFlags = appendUnique(out.DenyFlags, custom.DenyFlags...)
	out.DenyFlags = appendUnique(out.DenyFlags, defaults.DenyFlags...)
	return out
}

func expandRedisPolicy(ctx context.Context, p *asset_entity.RedisPolicy) *asset_entity.RedisPolicy {
	out := &asset_entity.RedisPolicy{}
	if p == nil {
		return out
	}
	out.AllowList = append(out.AllowList, p.AllowList...)
	out.DenyList = append(out.DenyList, p.DenyList...)
	if len(p.Groups) > 0 {
		allow, deny := resolveRedisGroups(ctx, p.Groups)
		out.AllowList = append(out.AllowList, allow...)
		out.DenyList = append(out.DenyList, deny...)
	}
	return out
}

func effectiveRedisPolicy(ctx context.Context, custom *asset_entity.RedisPolicy) *asset_entity.RedisPolicy {
	custom = expandRedisPolicy(ctx, custom)
	defaults := expandRedisPolicy(ctx, asset_entity.DefaultRedisPolicy())

	out := &asset_entity.RedisPolicy{}
	if len(custom.AllowList) > 0 {
		out.AllowList = appendUnique(out.AllowList, custom.AllowList...)
	} else {
		out.AllowList = appendUnique(out.AllowList, defaults.AllowList...)
	}
	out.DenyList = appendUnique(out.DenyList, custom.DenyList...)
	out.DenyList = appendUnique(out.DenyList, defaults.DenyList...)
	return out
}

func expandMongoPolicy(ctx context.Context, p *asset_entity.MongoPolicy) *asset_entity.MongoPolicy {
	out := &asset_entity.MongoPolicy{}
	if p == nil {
		return out
	}
	out.AllowTypes = append(out.AllowTypes, p.AllowTypes...)
	out.DenyTypes = append(out.DenyTypes, p.DenyTypes...)
	if len(p.Groups) > 0 {
		allowTypes, denyTypes := resolveMongoGroups(ctx, p.Groups)
		out.AllowTypes = append(out.AllowTypes, allowTypes...)
		out.DenyTypes = append(out.DenyTypes, denyTypes...)
	}
	return out
}

func effectiveMongoPolicy(ctx context.Context, custom *asset_entity.MongoPolicy) *asset_entity.MongoPolicy {
	custom = expandMongoPolicy(ctx, custom)
	defaults := expandMongoPolicy(ctx, asset_entity.DefaultMongoPolicy())

	out := &asset_entity.MongoPolicy{}
	if len(custom.AllowTypes) > 0 {
		out.AllowTypes = appendUnique(out.AllowTypes, custom.AllowTypes...)
	} else {
		out.AllowTypes = appendUnique(out.AllowTypes, defaults.AllowTypes...)
	}
	out.DenyTypes = appendUnique(out.DenyTypes, custom.DenyTypes...)
	out.DenyTypes = appendUnique(out.DenyTypes, defaults.DenyTypes...)
	return out
}

func expandKafkaPolicy(ctx context.Context, p *asset_entity.KafkaPolicy) *asset_entity.KafkaPolicy {
	out := &asset_entity.KafkaPolicy{}
	if p == nil {
		return out
	}
	out.AllowList = append(out.AllowList, p.AllowList...)
	out.DenyList = append(out.DenyList, p.DenyList...)
	if len(p.Groups) > 0 {
		allow, deny := resolveKafkaGroups(ctx, p.Groups)
		out.AllowList = append(out.AllowList, allow...)
		out.DenyList = append(out.DenyList, deny...)
	}
	return out
}

func effectiveKafkaPolicy(ctx context.Context, custom *asset_entity.KafkaPolicy) *asset_entity.KafkaPolicy {
	custom = expandKafkaPolicy(ctx, custom)
	defaults := expandKafkaPolicy(ctx, asset_entity.DefaultKafkaPolicy())

	out := &asset_entity.KafkaPolicy{}
	if len(custom.AllowList) > 0 {
		out.AllowList = appendUnique(out.AllowList, custom.AllowList...)
	} else {
		out.AllowList = appendUnique(out.AllowList, defaults.AllowList...)
	}
	out.DenyList = appendUnique(out.DenyList, custom.DenyList...)
	out.DenyList = appendUnique(out.DenyList, defaults.DenyList...)
	return out
}

func expandK8sPolicy(ctx context.Context, p *asset_entity.K8sPolicy) *asset_entity.K8sPolicy {
	out := &asset_entity.K8sPolicy{}
	if p == nil {
		return out
	}
	out.AllowList = append(out.AllowList, p.AllowList...)
	out.DenyList = append(out.DenyList, p.DenyList...)
	if len(p.Groups) > 0 {
		allow, deny := resolveCommandGroups(ctx, p.Groups)
		out.AllowList = append(out.AllowList, allow...)
		out.DenyList = append(out.DenyList, deny...)
	}
	return out
}

func effectiveK8sPolicy(ctx context.Context, custom *asset_entity.K8sPolicy) *asset_entity.K8sPolicy {
	custom = expandK8sPolicy(ctx, custom)
	defaults := expandK8sPolicy(ctx, asset_entity.DefaultK8sPolicy())

	out := &asset_entity.K8sPolicy{}
	if len(custom.AllowList) > 0 {
		out.AllowList = appendUnique(out.AllowList, custom.AllowList...)
	} else {
		out.AllowList = appendUnique(out.AllowList, defaults.AllowList...)
	}
	out.DenyList = appendUnique(out.DenyList, custom.DenyList...)
	out.DenyList = appendUnique(out.DenyList, defaults.DenyList...)
	return out
}
