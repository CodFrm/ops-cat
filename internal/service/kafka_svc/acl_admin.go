package kafka_svc

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
)

type aclFilter struct {
	assetID      int64
	resourceType kmsg.ACLResourceType
	resourceName string
	patternType  kmsg.ACLResourcePatternType
	principal    string
	host         string
	operation    kmsg.ACLOperation
	permission   kmsg.ACLPermissionType
	page         int
	pageSize     int
}

func (s *Service) ListACLs(ctx context.Context, req ListACLsRequest) (ListACLsResponse, error) {
	var out ListACLsResponse
	err := s.withClient(ctx, req.AssetID, func(ctx context.Context, _ *kgo.Client, admin *kadm.Client, _ *asset_entity.Asset, _ *asset_entity.KafkaConfig) error {
		filter, err := normalizeListACLsRequest(req)
		if err != nil {
			return err
		}
		builder, err := aclFilterBuilder(filter)
		if err != nil {
			return err
		}
		results, err := admin.DescribeACLs(ctx, builder)
		if err != nil {
			return fmt.Errorf("读取 Kafka ACL 失败: %w", err)
		}
		acls, err := kafkaACLsFromDescribeResults(results)
		if err != nil {
			return fmt.Errorf("读取 Kafka ACL 失败: %w", err)
		}
		out = listACLsResponse(acls, filter.page, filter.pageSize)
		return nil
	})
	return out, err
}

func (s *Service) CreateACL(ctx context.Context, req CreateACLRequest) (ACLMutationResponse, error) {
	var out ACLMutationResponse
	err := s.withClient(ctx, req.AssetID, func(ctx context.Context, _ *kgo.Client, admin *kadm.Client, _ *asset_entity.Asset, _ *asset_entity.KafkaConfig) error {
		filter, err := normalizeCreateACLRequest(req)
		if err != nil {
			return err
		}
		builder, err := aclCreateBuilder(filter)
		if err != nil {
			return err
		}
		results, err := admin.CreateACLs(ctx, builder)
		if err != nil {
			return fmt.Errorf("创建 Kafka ACL 失败: %w", err)
		}
		acls, err := kafkaACLsFromCreateResults(results)
		if err != nil {
			return fmt.Errorf("创建 Kafka ACL 失败: %w", err)
		}
		out = ACLMutationResponse{ACLs: acls, Count: len(acls)}
		return nil
	})
	return out, err
}

func (s *Service) DeleteACL(ctx context.Context, req DeleteACLRequest) (ACLMutationResponse, error) {
	var out ACLMutationResponse
	err := s.withClient(ctx, req.AssetID, func(ctx context.Context, _ *kgo.Client, admin *kadm.Client, _ *asset_entity.Asset, _ *asset_entity.KafkaConfig) error {
		filter, err := normalizeDeleteACLRequest(req)
		if err != nil {
			return err
		}
		builder, err := aclExactFilterBuilder(filter)
		if err != nil {
			return err
		}
		results, err := admin.DeleteACLs(ctx, builder)
		if err != nil {
			return fmt.Errorf("删除 Kafka ACL 失败: %w", err)
		}
		acls, err := kafkaACLsFromDeleteResults(results)
		if err != nil {
			return fmt.Errorf("删除 Kafka ACL 失败: %w", err)
		}
		out = ACLMutationResponse{ACLs: acls, Count: len(acls)}
		return nil
	})
	return out, err
}

func normalizeListACLsRequest(req ListACLsRequest) (aclFilter, error) {
	resourceName := strings.TrimSpace(req.ResourceName)
	resourceType, err := parseACLResourceType(req.ResourceType, true)
	if err != nil {
		return aclFilter{}, err
	}
	patternFallback := kmsg.ACLResourcePatternTypeAny
	if resourceName != "" {
		patternFallback = kmsg.ACLResourcePatternTypeMatch
	}
	patternType, err := parseACLPatternType(req.PatternType, patternFallback, false)
	if err != nil {
		return aclFilter{}, err
	}
	operation, err := parseACLOperation(req.Operation, true)
	if err != nil {
		return aclFilter{}, err
	}
	permission, err := parseACLPermission(req.Permission, true)
	if err != nil {
		return aclFilter{}, err
	}
	page, pageSize := normalizePage(req.Page, req.PageSize)
	return aclFilter{
		assetID:      req.AssetID,
		resourceType: resourceType,
		resourceName: resourceName,
		patternType:  patternType,
		principal:    strings.TrimSpace(req.Principal),
		host:         strings.TrimSpace(req.Host),
		operation:    operation,
		permission:   permission,
		page:         page,
		pageSize:     pageSize,
	}, nil
}

func normalizeCreateACLRequest(req CreateACLRequest) (aclFilter, error) {
	resourceType, err := parseACLResourceType(req.ResourceType, false)
	if err != nil {
		return aclFilter{}, err
	}
	resourceName, err := normalizeACLResourceName(resourceType, req.ResourceName, true)
	if err != nil {
		return aclFilter{}, err
	}
	patternType, err := parseACLPatternType(req.PatternType, kmsg.ACLResourcePatternTypeLiteral, true)
	if err != nil {
		return aclFilter{}, err
	}
	operation, err := parseACLOperation(req.Operation, false)
	if err != nil {
		return aclFilter{}, err
	}
	permission, err := parseACLPermission(req.Permission, false)
	if err != nil {
		return aclFilter{}, err
	}
	principal := strings.TrimSpace(req.Principal)
	if principal == "" {
		return aclFilter{}, fmt.Errorf("principal 不能为空")
	}
	return aclFilter{
		assetID:      req.AssetID,
		resourceType: resourceType,
		resourceName: resourceName,
		patternType:  patternType,
		principal:    principal,
		host:         strings.TrimSpace(req.Host),
		operation:    operation,
		permission:   permission,
	}, nil
}

func normalizeDeleteACLRequest(req DeleteACLRequest) (aclFilter, error) {
	resourceType, err := parseACLResourceType(req.ResourceType, false)
	if err != nil {
		return aclFilter{}, err
	}
	resourceName, err := normalizeACLResourceName(resourceType, req.ResourceName, true)
	if err != nil {
		return aclFilter{}, err
	}
	patternType, err := parseACLPatternType(req.PatternType, kmsg.ACLResourcePatternTypeLiteral, true)
	if err != nil {
		return aclFilter{}, err
	}
	operation, err := parseACLOperation(req.Operation, false)
	if err != nil {
		return aclFilter{}, err
	}
	permission, err := parseACLPermission(req.Permission, false)
	if err != nil {
		return aclFilter{}, err
	}
	principal := strings.TrimSpace(req.Principal)
	if principal == "" {
		return aclFilter{}, fmt.Errorf("principal 不能为空")
	}
	host := strings.TrimSpace(req.Host)
	if host == "" {
		return aclFilter{}, fmt.Errorf("host 不能为空")
	}
	return aclFilter{
		assetID:      req.AssetID,
		resourceType: resourceType,
		resourceName: resourceName,
		patternType:  patternType,
		principal:    principal,
		host:         host,
		operation:    operation,
		permission:   permission,
	}, nil
}

func aclFilterBuilder(filter aclFilter) (*kadm.ACLBuilder, error) {
	builder := kadm.NewACLs().ResourcePatternType(kadm.ACLPattern(filter.patternType)).Operations(kadm.ACLOperation(filter.operation))
	if err := applyACLResource(builder, filter.resourceType, filter.resourceName); err != nil {
		return nil, err
	}
	switch filter.permission {
	case kmsg.ACLPermissionTypeAny:
		applyACLPermissionFilter(builder.Allow, builder.AllowHosts, filter.principal, filter.host)
		applyACLPermissionFilter(builder.Deny, builder.DenyHosts, filter.principal, filter.host)
	case kmsg.ACLPermissionTypeAllow:
		applyACLPermissionFilter(builder.Allow, builder.AllowHosts, filter.principal, filter.host)
	case kmsg.ACLPermissionTypeDeny:
		applyACLPermissionFilter(builder.Deny, builder.DenyHosts, filter.principal, filter.host)
	default:
		return nil, fmt.Errorf("不支持的 ACL Permission: %s", filter.permission.String())
	}
	return builder, nil
}

func aclExactFilterBuilder(filter aclFilter) (*kadm.ACLBuilder, error) {
	builder := kadm.NewACLs().ResourcePatternType(kadm.ACLPattern(filter.patternType)).Operations(kadm.ACLOperation(filter.operation))
	if err := applyACLResource(builder, filter.resourceType, filter.resourceName); err != nil {
		return nil, err
	}
	switch filter.permission {
	case kmsg.ACLPermissionTypeAllow:
		builder.Allow(filter.principal).AllowHosts(filter.host)
	case kmsg.ACLPermissionTypeDeny:
		builder.Deny(filter.principal).DenyHosts(filter.host)
	default:
		return nil, fmt.Errorf("删除 ACL 必须指定 ALLOW 或 DENY")
	}
	return builder, nil
}

func aclCreateBuilder(filter aclFilter) (*kadm.ACLBuilder, error) {
	builder := kadm.NewACLs().ResourcePatternType(kadm.ACLPattern(filter.patternType)).Operations(kadm.ACLOperation(filter.operation))
	if err := applyACLResource(builder, filter.resourceType, filter.resourceName); err != nil {
		return nil, err
	}
	switch filter.permission {
	case kmsg.ACLPermissionTypeAllow:
		builder.Allow(filter.principal)
		if filter.host != "" {
			builder.AllowHosts(filter.host)
		}
	case kmsg.ACLPermissionTypeDeny:
		builder.Deny(filter.principal)
		if filter.host != "" {
			builder.DenyHosts(filter.host)
		}
	default:
		return nil, fmt.Errorf("创建 ACL 必须指定 ALLOW 或 DENY")
	}
	return builder, nil
}

func applyACLResource(builder *kadm.ACLBuilder, resourceType kmsg.ACLResourceType, resourceName string) error {
	switch resourceType {
	case kmsg.ACLResourceTypeAny:
		if resourceName == "" {
			builder.AnyResource()
		} else {
			builder.AnyResource(resourceName)
		}
	case kmsg.ACLResourceTypeTopic:
		if resourceName == "" {
			builder.Topics()
		} else {
			builder.Topics(resourceName)
		}
	case kmsg.ACLResourceTypeGroup:
		if resourceName == "" {
			builder.Groups()
		} else {
			builder.Groups(resourceName)
		}
	case kmsg.ACLResourceTypeCluster:
		builder.Clusters()
	case kmsg.ACLResourceTypeTransactionalId:
		if resourceName == "" {
			builder.TransactionalIDs()
		} else {
			builder.TransactionalIDs(resourceName)
		}
	case kmsg.ACLResourceTypeDelegationToken:
		if resourceName == "" {
			builder.DelegationTokens()
		} else {
			builder.DelegationTokens(resourceName)
		}
	default:
		return fmt.Errorf("不支持的 ACL Resource Type: %s", resourceType.String())
	}
	return nil
}

func applyACLPermissionFilter(principalFn func(...string) *kadm.ACLBuilder, hostFn func(...string) *kadm.ACLBuilder, principal, host string) {
	if principal == "" {
		principalFn()
	} else {
		principalFn(principal)
	}
	if host == "" {
		hostFn()
	} else {
		hostFn(host)
	}
}

func normalizeACLResourceName(resourceType kmsg.ACLResourceType, value string, required bool) (string, error) {
	name := strings.TrimSpace(value)
	if resourceType == kmsg.ACLResourceTypeCluster {
		if name == "" {
			return "kafka-cluster", nil
		}
		if !strings.EqualFold(name, "kafka-cluster") {
			return "", fmt.Errorf("cluster resource name 必须为 kafka-cluster")
		}
		return "kafka-cluster", nil
	}
	if required && name == "" {
		return "", fmt.Errorf("resource name 不能为空")
	}
	return name, nil
}

func parseACLResourceType(value string, allowAny bool) (kmsg.ACLResourceType, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		if allowAny {
			return kmsg.ACLResourceTypeAny, nil
		}
		return kmsg.ACLResourceTypeUnknown, fmt.Errorf("resource type 不能为空")
	}
	resourceType, err := kmsg.ParseACLResourceType(value)
	if err != nil {
		return kmsg.ACLResourceTypeUnknown, fmt.Errorf("不支持的 ACL Resource Type: %s", value)
	}
	if resourceType == kmsg.ACLResourceTypeAny && !allowAny {
		return kmsg.ACLResourceTypeUnknown, fmt.Errorf("resource type 不能为 ANY")
	}
	if resourceType == kmsg.ACLResourceTypeUser {
		return kmsg.ACLResourceTypeUnknown, fmt.Errorf("暂不支持 USER Resource Type")
	}
	return resourceType, nil
}

func parseACLPatternType(value string, fallback kmsg.ACLResourcePatternType, createOrExact bool) (kmsg.ACLResourcePatternType, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, nil
	}
	patternType, err := kmsg.ParseACLResourcePatternType(value)
	if err != nil {
		return kmsg.ACLResourcePatternTypeUnknown, fmt.Errorf("不支持的 ACL Pattern Type: %s", value)
	}
	if createOrExact {
		switch patternType {
		case kmsg.ACLResourcePatternTypeLiteral, kmsg.ACLResourcePatternTypePrefixed:
			return patternType, nil
		default:
			return kmsg.ACLResourcePatternTypeUnknown, fmt.Errorf("pattern type 必须为 LITERAL 或 PREFIXED")
		}
	}
	return patternType, nil
}

func parseACLOperation(value string, allowAny bool) (kmsg.ACLOperation, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		if allowAny {
			return kmsg.ACLOperationAny, nil
		}
		return kmsg.ACLOperationUnknown, fmt.Errorf("operation不能为空")
	}
	operation, err := kmsg.ParseACLOperation(value)
	if err != nil {
		return kmsg.ACLOperationUnknown, fmt.Errorf("不支持的 ACL Operation: %s", value)
	}
	if operation == kmsg.ACLOperationAny && !allowAny {
		return kmsg.ACLOperationUnknown, fmt.Errorf("operation不能为 ANY")
	}
	if operation == kmsg.ACLOperationUnknown {
		return kmsg.ACLOperationUnknown, fmt.Errorf("operation不能为 UNKNOWN")
	}
	return operation, nil
}

func parseACLPermission(value string, allowAny bool) (kmsg.ACLPermissionType, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		if allowAny {
			return kmsg.ACLPermissionTypeAny, nil
		}
		return kmsg.ACLPermissionTypeUnknown, fmt.Errorf("permission不能为空")
	}
	permission, err := kmsg.ParseACLPermissionType(value)
	if err != nil {
		return kmsg.ACLPermissionTypeUnknown, fmt.Errorf("不支持的 ACL Permission: %s", value)
	}
	if permission == kmsg.ACLPermissionTypeAny && !allowAny {
		return kmsg.ACLPermissionTypeUnknown, fmt.Errorf("permission不能为 ANY")
	}
	if permission == kmsg.ACLPermissionTypeUnknown {
		return kmsg.ACLPermissionTypeUnknown, fmt.Errorf("permission不能为 UNKNOWN")
	}
	return permission, nil
}

func listACLsResponse(acls []KafkaACL, page, pageSize int) ListACLsResponse {
	sortKafkaACLs(acls)
	total := len(acls)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return ListACLsResponse{
		ACLs:     acls[start:end],
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}
}

func kafkaACLsFromDescribeResults(results kadm.DescribeACLsResults) ([]KafkaACL, error) {
	acls := make([]KafkaACL, 0)
	for _, result := range results {
		if err := aclResultError(result.Err, result.ErrMessage); err != nil {
			return nil, err
		}
		for _, acl := range result.Described {
			acls = append(acls, kafkaACLFromFields(acl.Type, acl.Name, acl.Pattern, acl.Principal, acl.Host, acl.Operation, acl.Permission, nil, ""))
		}
	}
	return acls, nil
}

func kafkaACLsFromCreateResults(results kadm.CreateACLsResults) ([]KafkaACL, error) {
	acls := make([]KafkaACL, 0, len(results))
	for _, result := range results {
		if err := aclResultError(result.Err, result.ErrMessage); err != nil {
			return nil, err
		}
		acls = append(acls, kafkaACLFromFields(result.Type, result.Name, result.Pattern, result.Principal, result.Host, result.Operation, result.Permission, result.Err, result.ErrMessage))
	}
	sortKafkaACLs(acls)
	return acls, nil
}

func kafkaACLsFromDeleteResults(results kadm.DeleteACLsResults) ([]KafkaACL, error) {
	acls := make([]KafkaACL, 0)
	for _, result := range results {
		if err := aclResultError(result.Err, result.ErrMessage); err != nil {
			return nil, err
		}
		for _, acl := range result.Deleted {
			if err := aclResultError(acl.Err, acl.ErrMessage); err != nil {
				return nil, err
			}
			acls = append(acls, kafkaACLFromFields(acl.Type, acl.Name, acl.Pattern, acl.Principal, acl.Host, acl.Operation, acl.Permission, acl.Err, acl.ErrMessage))
		}
	}
	sortKafkaACLs(acls)
	return acls, nil
}

func kafkaACLFromFields(resourceType kmsg.ACLResourceType, resourceName string, patternType kmsg.ACLResourcePatternType, principal string, host string, operation kmsg.ACLOperation, permission kmsg.ACLPermissionType, err error, errMessage string) KafkaACL {
	return KafkaACL{
		ResourceType: resourceType.String(),
		ResourceName: resourceName,
		PatternType:  patternType.String(),
		Principal:    principal,
		Host:         host,
		Operation:    operation.String(),
		Permission:   permission.String(),
		Error:        aclErrorString(err, errMessage),
	}
}

func aclResultError(err error, message string) error {
	if err == nil {
		return nil
	}
	if strings.TrimSpace(message) == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, message)
}

func aclErrorString(err error, message string) string {
	message = strings.TrimSpace(message)
	if err == nil {
		return message
	}
	if message == "" {
		return err.Error()
	}
	return err.Error() + ": " + message
}

func sortKafkaACLs(acls []KafkaACL) {
	sort.Slice(acls, func(i, j int) bool {
		left := []string{
			acls[i].ResourceType,
			acls[i].ResourceName,
			acls[i].PatternType,
			acls[i].Principal,
			acls[i].Host,
			acls[i].Operation,
			acls[i].Permission,
		}
		right := []string{
			acls[j].ResourceType,
			acls[j].ResourceName,
			acls[j].PatternType,
			acls[j].Principal,
			acls[j].Host,
			acls[j].Operation,
			acls[j].Permission,
		}
		for idx := range left {
			if left[idx] == right[idx] {
				continue
			}
			return left[idx] < right[idx]
		}
		return false
	})
}
