package ai

import (
	"context"

	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/agents/tool"
)

// kafkaTools Kafka 操作工具集（7 个）。
// 全部 Serial：Kafka client / Schema Registry / Connect 是按 asset_id 缓存的有状态客户端，
// 同会话内串行执行避免请求乱序导致的偏移混乱与配置写竞争。
func kafkaTools() []tool.Tool {
	return []tool.Tool{
		&tool.RawTool{
			NameStr: "kafka_cluster",
			DescStr: "Read Kafka cluster metadata and configuration for a Kafka asset. Grouped operations: overview, brokers, get_broker_config, list_cluster_configs. Credentials are resolved automatically.",
			SchemaVal: agent.Schema{
				Type: "object",
				Properties: map[string]*agent.Property{
					"asset_id":  {Type: "number", Description: "Kafka asset ID. Use list_assets with asset_type='kafka' to find."},
					"operation": {Type: "string", Description: "Operation: overview, brokers, get_broker_config, list_cluster_configs. Defaults to overview."},
					"broker_id": {Type: "number", Description: "Broker node ID for operation=get_broker_config."},
				},
				Required: []string{"asset_id"},
			},
			IsSerial: true,
			Handler: func(ctx context.Context, in map[string]any) (*agent.ToolResultBlock, error) {
				out, err := handleKafkaCluster(ctx, in)
				if err != nil {
					return nil, err
				}
				return &agent.ToolResultBlock{Content: []agent.ContentBlock{agent.TextBlock{Text: out}}}, nil
			},
		},
		&tool.RawTool{
			NameStr: "kafka_topic",
			DescStr: "Read and manage Kafka topics for a Kafka asset. Grouped operations: list, get, create, delete, update_config, increase_partitions, delete_records.",
			SchemaVal: agent.Schema{
				Type: "object",
				Properties: map[string]*agent.Property{
					"asset_id":           {Type: "number", Description: "Kafka asset ID. Use list_assets with asset_type='kafka' to find."},
					"operation":          {Type: "string", Description: "Operation: list, get, create, delete, update_config, increase_partitions, delete_records. Defaults to list."},
					"topic":              {Type: "string", Description: "Topic name. Required except operation=list."},
					"include_internal":   {Type: "string", Description: `Set to "true" to include internal topics when operation=list.`},
					"search":             {Type: "string", Description: "Optional case-insensitive topic name filter for operation=list."},
					"page":               {Type: "number", Description: "Page number for operation=list. Defaults to 1."},
					"page_size":          {Type: "number", Description: "Page size for operation=list. Defaults to 50, max 500."},
					"partitions":         {Type: "number", Description: "Partition count for operation=create."},
					"replication_factor": {Type: "number", Description: "Replication factor for operation=create."},
					"configs":            {Type: "string", Description: `Topic configs for operation=create as JSON object, e.g. {"cleanup.policy":"compact"}. Optional.`},
					"config_updates":     {Type: "string", Description: `Config mutations for operation=update_config as JSON array, e.g. [{"name":"retention.ms","value":"60000","op":"set"}]. op can be set, delete, append, subtract.`},
					"partition_count":    {Type: "number", Description: "Final partition count for operation=increase_partitions. Must be greater than the current count."},
					"records":            {Type: "string", Description: `Partition offsets for operation=delete_records as JSON array, e.g. [{"partition":0,"offset":123}]. Deletes records before each offset.`},
				},
				Required: []string{"asset_id"},
			},
			IsSerial: true,
			Handler: func(ctx context.Context, in map[string]any) (*agent.ToolResultBlock, error) {
				out, err := handleKafkaTopic(ctx, in)
				if err != nil {
					return nil, err
				}
				return &agent.ToolResultBlock{Content: []agent.ContentBlock{agent.TextBlock{Text: out}}}, nil
			},
		},
		&tool.RawTool{
			NameStr: "kafka_consumer_group",
			DescStr: "Read and manage Kafka consumer groups for a Kafka asset. Grouped operations: list, get, reset_offset, delete.",
			SchemaVal: agent.Schema{
				Type: "object",
				Properties: map[string]*agent.Property{
					"asset_id":         {Type: "number", Description: "Kafka asset ID. Use list_assets with asset_type='kafka' to find."},
					"operation":        {Type: "string", Description: "Operation: list, get, reset_offset, delete. Defaults to list."},
					"group":            {Type: "string", Description: "Consumer group name. Required except operation=list."},
					"topic":            {Type: "string", Description: "Topic name for operation=reset_offset."},
					"partitions":       {Type: "string", Description: "Optional JSON array of partitions for operation=reset_offset. Omit to reset all partitions in the topic."},
					"mode":             {Type: "string", Description: "Offset reset mode: earliest, latest, offset, timestamp. Defaults to latest."},
					"offset":           {Type: "number", Description: "Offset for mode=offset."},
					"timestamp_millis": {Type: "number", Description: "Unix milliseconds for mode=timestamp."},
				},
				Required: []string{"asset_id"},
			},
			IsSerial: true,
			Handler: func(ctx context.Context, in map[string]any) (*agent.ToolResultBlock, error) {
				out, err := handleKafkaConsumerGroup(ctx, in)
				if err != nil {
					return nil, err
				}
				return &agent.ToolResultBlock{Content: []agent.ContentBlock{agent.TextBlock{Text: out}}}, nil
			},
		},
		&tool.RawTool{
			NameStr: "kafka_acl",
			DescStr: "Read and manage Kafka ACLs for a Kafka asset. Grouped operations: list, create, delete. ACL create/delete are security-admin operations and require explicit policy approval.",
			SchemaVal: agent.Schema{
				Type: "object",
				Properties: map[string]*agent.Property{
					"asset_id":      {Type: "number", Description: "Kafka asset ID. Use list_assets with asset_type='kafka' to find."},
					"operation":     {Type: "string", Description: "Operation: list, create, delete. Defaults to list."},
					"resource_type": {Type: "string", Description: "ACL resource type: topic, group, cluster, transactional_id, delegation_token, or any for list only."},
					"resource_name": {Type: "string", Description: "ACL resource name. Required for create/delete except resource_type=cluster."},
					"pattern_type":  {Type: "string", Description: "ACL pattern type: literal, prefixed, match, any. create/delete only allow literal or prefixed."},
					"principal":     {Type: "string", Description: "ACL principal, e.g. User:alice. Required for create/delete."},
					"host":          {Type: "string", Description: "ACL host, e.g. * or 192.168.1.10. Required for delete; create defaults to * when omitted."},
					"acl_operation": {Type: "string", Description: "Kafka ACL operation: read, write, create, delete, alter, describe, describe_configs, alter_configs, all, etc."},
					"permission":    {Type: "string", Description: "ACL permission: allow, deny, or any for list only."},
					"page":          {Type: "number", Description: "Page number for operation=list. Defaults to 1."},
					"page_size":     {Type: "number", Description: "Page size for operation=list. Defaults to 50, max 500."},
				},
				Required: []string{"asset_id"},
			},
			IsSerial: true,
			Handler: func(ctx context.Context, in map[string]any) (*agent.ToolResultBlock, error) {
				out, err := handleKafkaACL(ctx, in)
				if err != nil {
					return nil, err
				}
				return &agent.ToolResultBlock{Content: []agent.ContentBlock{agent.TextBlock{Text: out}}}, nil
			},
		},
		&tool.RawTool{
			NameStr: "kafka_schema",
			DescStr: "Read and manage Schema Registry subjects for a Kafka asset when Schema Registry is configured. Grouped operations: list_subjects, list_versions, get, check_compatibility, register, delete.",
			SchemaVal: agent.Schema{
				Type: "object",
				Properties: map[string]*agent.Property{
					"asset_id":    {Type: "number", Description: "Kafka asset ID. Use list_assets with asset_type='kafka' to find."},
					"operation":   {Type: "string", Description: "Operation: list_subjects, list_versions, get, check_compatibility, register, delete. Defaults to list_subjects."},
					"subject":     {Type: "string", Description: "Schema subject. Required except operation=list_subjects."},
					"version":     {Type: "string", Description: "Schema version number or latest. Defaults to latest for get/check_compatibility. Optional for delete; omitted deletes the subject."},
					"schema":      {Type: "string", Description: "Schema content for register/check_compatibility."},
					"schema_type": {Type: "string", Description: "Schema type such as AVRO, JSON, or PROTOBUF. Optional."},
					"references":  {Type: "string", Description: `Schema references as JSON array, e.g. [{"name":"Common","subject":"common-value","version":1}]. Optional.`},
					"permanent":   {Type: "string", Description: `Set to "true" for permanent delete where supported.`},
				},
				Required: []string{"asset_id"},
			},
			IsSerial: true,
			Handler: func(ctx context.Context, in map[string]any) (*agent.ToolResultBlock, error) {
				out, err := handleKafkaSchema(ctx, in)
				if err != nil {
					return nil, err
				}
				return &agent.ToolResultBlock{Content: []agent.ContentBlock{agent.TextBlock{Text: out}}}, nil
			},
		},
		&tool.RawTool{
			NameStr: "kafka_connect",
			DescStr: "Read and manage Kafka Connect connectors for a Kafka asset when Kafka Connect is configured. Grouped operations: list_clusters, list_connectors, get_connector, create, update_config, pause, resume, restart, delete.",
			SchemaVal: agent.Schema{
				Type: "object",
				Properties: map[string]*agent.Property{
					"asset_id":      {Type: "number", Description: "Kafka asset ID. Use list_assets with asset_type='kafka' to find."},
					"operation":     {Type: "string", Description: "Operation: list_clusters, list_connectors, get_connector, create, update_config, pause, resume, restart, delete. Defaults to list_connectors."},
					"cluster":       {Type: "string", Description: "Kafka Connect cluster name. Optional when the asset has exactly one Connect cluster."},
					"connector":     {Type: "string", Description: "Connector name. Required except list_clusters/list_connectors."},
					"config":        {Type: "string", Description: "Connector config as JSON object for create/update_config."},
					"include_tasks": {Type: "string", Description: `Set to "true" for restart to include tasks.`},
					"only_failed":   {Type: "string", Description: `Set to "true" for restart to restart only failed tasks.`},
				},
				Required: []string{"asset_id"},
			},
			IsSerial: true,
			Handler: func(ctx context.Context, in map[string]any) (*agent.ToolResultBlock, error) {
				out, err := handleKafkaConnect(ctx, in)
				if err != nil {
					return nil, err
				}
				return &agent.ToolResultBlock{Content: []agent.ContentBlock{agent.TextBlock{Text: out}}}, nil
			},
		},
		&tool.RawTool{
			NameStr: "kafka_message",
			DescStr: "Browse or produce bounded Kafka messages for a Kafka asset. Grouped operations: browse, inspect, produce. Message reads and writes are policy-controlled; returned payload previews are truncated.",
			SchemaVal: agent.Schema{
				Type: "object",
				Properties: map[string]*agent.Property{
					"asset_id":         {Type: "number", Description: "Kafka asset ID. Use list_assets with asset_type='kafka' to find."},
					"operation":        {Type: "string", Description: "Operation: browse, inspect, produce. Defaults to browse."},
					"topic":            {Type: "string", Description: "Topic name."},
					"partition":        {Type: "number", Description: "Optional partition. Required for inspect."},
					"start_mode":       {Type: "string", Description: "Browse start mode: newest, oldest, offset, timestamp. Defaults to newest."},
					"offset":           {Type: "number", Description: "Start offset for browse start_mode=offset, or exact offset for inspect."},
					"timestamp_millis": {Type: "number", Description: "Unix milliseconds for browse start_mode=timestamp, or produce timestamp override."},
					"limit":            {Type: "number", Description: "Browse record limit. Defaults to asset settings; max 1000."},
					"max_bytes":        {Type: "number", Description: "Max key/value/header preview bytes per field. Defaults to asset settings."},
					"decode_mode":      {Type: "string", Description: "Browse decode mode: text, json, hex, base64. Defaults to text; binary data is returned as base64."},
					"max_wait_millis":  {Type: "number", Description: "Browse poll wait in milliseconds. Defaults to 1000; max 30000."},
					"key":              {Type: "string", Description: "Produce key. Optional."},
					"key_encoding":     {Type: "string", Description: "Produce key encoding: text, json, hex, base64. Defaults to text."},
					"value":            {Type: "string", Description: "Produce value. Empty string is allowed."},
					"value_encoding":   {Type: "string", Description: "Produce value encoding: text, json, hex, base64. Defaults to text."},
					"headers":          {Type: "string", Description: `Produce headers as JSON array, e.g. [{"key":"trace","value":"abc","encoding":"text"}]. Optional.`},
				},
				Required: []string{"asset_id", "topic"},
			},
			IsSerial: true,
			Handler: func(ctx context.Context, in map[string]any) (*agent.ToolResultBlock, error) {
				out, err := handleKafkaMessage(ctx, in)
				if err != nil {
					return nil, err
				}
				return &agent.ToolResultBlock{Content: []agent.ContentBlock{agent.TextBlock{Text: out}}}, nil
			},
		},
	}
}
