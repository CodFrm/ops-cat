# Kafka Management Design

## Background

OpsKat needs Kafka management as a first-class built-in asset capability. The plugin-based direction is not the right implementation path for this feature because complete Kafka management needs a native Kafka client, stable TCP/TLS/SASL support, connection reuse, bounded message browsing, admin APIs, security operations, and AI permission enforcement.

Kafka management will be implemented as a native module, while keeping the architecture compatible with existing asset, query, permission, audit, SSH tunnel, and AI tool patterns.

Backend Kafka protocol client: `github.com/twmb/franz-go`.

UI reference: Kafbat UI information architecture and workflows. The implementation should follow OpsKat's existing visual language instead of copying Kafbat UI styling directly.

## Design Position

This document describes the full target design, not a minimal MVP. Engineering work can still be delivered in milestones, but the data model, service boundaries, permission model, frontend structure, and AI tool contracts should be designed for the complete target from the start.

## Implementation Status

As of 2026-05-02, the Kafka management implementation has reached the Phase 7 target described in this document. This file should be read as both the target design and the current implementation record; sections below still describe the intended architecture, while this section records what has already landed.

| Area | Status | Notes |
| --- | --- | --- |
| Foundation | Implemented | Kafka asset type, config, validation, policy type, group policy migration, `policy.Holder` extension, asset/group policy accessors, asset form, detail card, franz-go client construction, TLS/SASL, and SSH tunnel support are in place. |
| Core read workspace | Implemented | Cluster overview, broker list, topic list/detail, consumer group list/detail, `KafkaPanel`, and `kafkaStore` are available. |
| Message read/write | Implemented | Bounded message browse, offset/timestamp modes, payload truncation metadata, decode modes, produce message, UI confirmation, and AI message tools are implemented. |
| Topic and consumer group admin | Implemented | Topic create/delete/config update, partition increase, record delete, consumer group offset reset/delete, confirmations, service operations, and AI handlers are implemented. |
| ACL management | Implemented | ACL list/create/delete, ACL filter UI, service layer, AI tools, and policy checks are implemented. |
| Schema Registry | Implemented | Subject/version/schema read, compatibility check, register/delete, service layer, frontend panel, companion config, and AI tools are implemented. |
| Kafka Connect | Implemented | Connector list/detail/config/status, create/update/delete, pause/resume/restart, multi-cluster selection, companion config, service layer, frontend panel, and AI tools are implemented. Connector list uses `?expand=status` when supported and falls back to `/connectors` only on Kafka Connect HTTP 4xx compatibility errors. |
| AI permission model | Implemented | Grouped Kafka tools, canonical permission commands, `MatchKafkaRule`, Kafka policy collection, Kafka grant matching, confirmation mapping with `type="kafka"`, and audit summaries are implemented. |
| Testing and tooling | Implemented | Unit tests cover policy matching, permission paths, service behavior, companion HTTP clients, and the Kafka Connect status fallback. `pkg/extension/testdata/kafka-sim` provides a local integration simulator for topic/message/group/ACL/schema/connect workflows. |
| Hardening fixes | Implemented | Rule matching uses `path.Match` semantics for cross-platform consistency, `App.kafkaSvc()` is protected from concurrent lazy initialization, Kafka Connect form JSON errors render inline, restart options use the shared Checkbox component, and bearer companion auth uses a single secret source path. |

Remaining product/design decisions:

- Decide whether ACL editing should be hidden unless the cluster reports ACL support, or always shown with operation-level errors.
- Decide whether selected message export should be added in a later phase.
- Continue validating against additional secured environments as they become available, especially SASL/TLS combinations and SSH tunnel paths.
- Keep this status section updated if later refactors change the connection manager, companion API behavior, or AI tool contract.

## Goals

- Add `kafka` as a built-in asset type.
- Support direct and SSH tunnel connections.
- Support Kafka security modes:
  - plaintext
  - TLS
  - SASL PLAIN
  - SCRAM-SHA-256
  - SCRAM-SHA-512
  - client certificate authentication when supported by the cluster
- Provide complete Kafka cluster management:
  - cluster overview
  - brokers
  - topics
  - partitions
  - topic configs
  - topic create/delete/update
  - partition increase
  - message browse
  - message produce
  - consumer groups
  - consumer group lag
  - consumer group offset reset
  - consumer group delete when supported
  - ACL list/create/delete
  - broker and cluster config inspection
- Provide optional companion integrations:
  - Schema Registry
  - Kafka Connect
- Add AI-callable Kafka tools with policy checks, confirmation, grant, and audit.
- Keep safety by default:
  - metadata reads are allowed by default
  - message reads require explicit permission
  - write/admin/security operations are denied by default

## Non-Goals

- Do not implement Kafka as a WASM extension in this version.
- Do not build a full monitoring or alerting system.
- Do not perform unbounded background consumption.
- Do not persist message payload previews unless explicitly designed later.
- Do not bypass the existing OpsKat permission, confirmation, grant, and audit model.
- Do not expose a generic arbitrary Kafka command executor to AI.

## Architecture

Kafka should follow the same broad pattern as existing built-in database, Redis, and MongoDB assets, with a richer service layer.

```text
Kafka Asset
  -> KafkaConfig
  -> credential_resolver
  -> internal/connpool/kafka
  -> internal/service/kafka_svc
  -> internal/app/app_kafka.go
  -> Wails IPC
  -> frontend KafkaPanel
  -> AI tools and policy checker
```

### Main Components

| Layer | Responsibility |
| --- | --- |
| `asset_entity` | Defines Kafka asset type, config, validation, and policy accessors. |
| `credential_resolver` | Resolves Kafka credentials from managed credentials or inline encrypted password. |
| `connpool/kafka` | Builds and reuses franz-go clients with TLS, SASL, and optional SSH tunnel dialer. |
| `kafka_svc` | Provides typed Kafka management operations. |
| `app_kafka.go` | Exposes Wails methods for frontend usage. |
| Frontend asset form | Creates and edits Kafka assets. |
| Frontend Kafka panel | Presents cluster, topic, group, message, ACL, schema, and connect management UI. |
| `internal/ai` | Adds Kafka tools, policy matching, confirmation, grant, and audit support. |

### Current Architecture Constraints

Kafka must fit the current OpsKat architecture instead of introducing parallel systems.

- Asset-specific config continues to live in `Asset.Config` as JSON.
- Group-level type policies continue to live as columns on the `groups` table.
- Built-in policy types are hardcoded in `policy_group_entity.Validate()` and must be extended.
- Policy collection currently depends on `policy.Holder`; adding Kafka changes that interface and therefore both `Asset` and `Group`.
- AI permission checks must go through `CheckPermission()` and `CommandPolicyChecker`.
- Approval item type mapping must include Kafka so the frontend receives `type="kafka"`, not the SSH default `type="exec"`.
- DB Grant storage can be reused, but Kafka matching must use Kafka-specific rule matching rather than `MatchCommandRule`.
- Existing DB/Redis/Mongo services mostly dial per request; any Kafka client cache must be owned by an App/service lifecycle and explicitly closed.

## Asset Model

Add a built-in asset type:

| Field | Value |
| --- | --- |
| Type | `kafka` |
| Default icon | Kafka-specific icon if available, otherwise database/server-style icon |
| Connect action | `query` |
| SSH tunnel support | yes |
| Default port hint | `9092` |

### Kafka Config

Kafka asset config should include:

| Field | Purpose |
| --- | --- |
| `brokers` | One or more bootstrap brokers, stored as host:port values. |
| `client_id` | Client ID used by franz-go. |
| `sasl_mechanism` | `none`, `plain`, `scram-sha-256`, `scram-sha-512`. |
| `username` | SASL username. |
| `password` | Inline encrypted SASL password. |
| `credential_id` | Managed password credential ID. |
| `tls` | Enable TLS. |
| `tls_insecure` | Skip TLS certificate verification. |
| `tls_server_name` | Optional TLS server name override. |
| `tls_ca_file` | Optional CA certificate file path. |
| `tls_cert_file` | Optional client certificate file path. |
| `tls_key_file` | Optional client key file path. |
| `request_timeout_seconds` | Default operation timeout. |
| `message_preview_bytes` | Default max bytes shown per key/value preview. |
| `message_fetch_limit` | Default message browse limit. |
| `ssh_asset_id` | Backward-compatible tunnel field if needed; preferred storage remains `Asset.SSHTunnelID`. |

Validation rules:

- At least one broker is required.
- Broker entries must include host and port.
- SASL username/password is required when SASL is enabled.
- TLS cert and key must be provided together.
- Timeout values must be bounded.
- Message preview and fetch limits must have safe upper bounds.

### Companion Config

Schema Registry and Kafka Connect use HTTP APIs, not the Kafka broker protocol. They should be configured as optional companion endpoints on the Kafka asset.

| Field | Purpose |
| --- | --- |
| `schema_registry.enabled` | Enable Schema Registry integration. |
| `schema_registry.url` | Base URL. |
| `schema_registry.auth_type` | none/basic/bearer. |
| `schema_registry.username` | Basic auth username. |
| `schema_registry.password` | Inline encrypted password. |
| `schema_registry.credential_id` | Managed credential ID. |
| `schema_registry.tls_*` | Optional TLS options. |
| `connect.enabled` | Enable Kafka Connect integration. |
| `connect.clusters` | Named Connect clusters with URL/auth/TLS config. |

Companion integrations should use the same SSH tunnel setting as the Kafka asset when possible.

Companion credential sub-configs must implement the same password-source shape used elsewhere:

- `GetCredentialID() int64`
- `GetPassword() string`

`kafka_svc` should resolve Schema Registry and Kafka Connect credentials with `credential_resolver.Default().ResolvePasswordGeneric()` instead of expanding `AssetTypeHandler.ResolvePassword()` beyond a single primary password.

## Backend Design

### Connection Manager

`internal/connpool/kafka` should provide a client manager, not just one-off dialing.

Responsibilities:

- Normalize bootstrap brokers.
- Resolve TLS config.
- Resolve SASL config.
- Attach an SSH tunnel dialer when the asset has `SSHTunnelID`.
- Maintain a bounded TTL client cache keyed by asset ID and config fingerprint.
- Close stale clients when config changes or app shuts down.
- Provide explicit close/reload behavior for asset updates.

Implementation shape:

- `internal/connpool/kafka` owns the low-level client builder and optional manager type.
- `internal/service/kafka_svc.Service` owns a manager instance for UI/Wails calls.
- `App.Startup` creates `kafka_svc.New(a.sshPool)`.
- `App.Cleanup` closes the Kafka service/manager.
- The manager key is `assetID + configFingerprint`.
- The fingerprint is computed from connection-relevant config fields: `brokers` (sorted), `saslMechanism`, `username`, `tls`, `tlsInsecure`, `tlsServerName`, `tlsCaFile`, `tlsCertFile`, `tlsKeyFile`, `sshTunnelID`, and the credential identifier (`credentialID` if non-zero, otherwise a stable hash of the encrypted inline password bytes). Plaintext secrets must never appear in the fingerprint, logs, or debug output. Serialize these fields into a deterministic string and hash with `fnv.New64a`.
- On checkout, the manager compares the latest fingerprint with the cached entry and closes stale clients.
- A background TTL eviction goroutine is acceptable only if it is owned by the service and stopped in `Close()`.
- Lazy eviction on checkout is acceptable for the first implementation if the close path is deterministic.
- If a generic asset-update hook is added later, Kafka should call `CloseAsset(assetID)` from that hook. Until then, fingerprint comparison is the required invalidation path.

AI handlers should not depend on an App singleton. They should follow the current `exec_sql` / `exec_redis` pattern:

- resolve asset and credentials inside `internal/ai`
- use `getSSHPool(ctx)` for tunnel support
- use a session-scoped `ConnCache[*kgo.Client]` when long enough tool sequences benefit from reuse
- close non-cached clients after the tool call

### Service Layer

Create `internal/service/kafka_svc`.

Cluster and broker methods:

| Method | Description |
| --- | --- |
| `TestConnection` | Connects and performs a lightweight metadata/API versions request. |
| `ClusterOverview` | Returns cluster ID, controller, broker count, topic count, partition count. |
| `ListBrokers` | Returns broker ID, host, port, rack when available. |
| `GetBrokerConfig` | Reads broker configs when authorized and supported. |
| `ListClusterConfigs` | Reads cluster-level configs when available. |

Topic methods:

| Method | Description |
| --- | --- |
| `ListTopics` | Returns topic name, partition count, replication factor, internal flag, config summary. |
| `GetTopic` | Returns partitions, leader, replicas, ISR, and configs. |
| `CreateTopic` | Creates topic with partitions, replication factor, and configs. |
| `DeleteTopic` | Deletes a topic. |
| `AlterTopicConfig` | Updates topic configs. |
| `IncreasePartitions` | Increases partition count. |
| `DeleteRecords` | Deletes records before offsets when supported. |

Message methods:

| Method | Description |
| --- | --- |
| `BrowseMessages` | Reads bounded records by topic, partition, offset, or timestamp. |
| `ProduceMessage` | Produces a single message with key, value, headers, partition, and timestamp options. |
| `InspectRecord` | Returns a single record detail with bounded payload rendering. |

Consumer group methods:

| Method | Description |
| --- | --- |
| `ListConsumerGroups` | Returns group IDs and states. |
| `GetConsumerGroup` | Returns members, assignments, committed offsets, lag summary. |
| `ResetConsumerGroupOffset` | Resets offsets by offset, timestamp, earliest, or latest. |
| `DeleteConsumerGroup` | Deletes a group when supported and safe. |

ACL methods:

| Method | Description |
| --- | --- |
| `ListACLs` | Lists ACLs with filters. |
| `CreateACL` | Creates ACL entries. |
| `DeleteACL` | Deletes ACL entries by exact filter. |

Schema Registry methods:

| Method | Description |
| --- | --- |
| `ListSubjects` | Lists subjects. |
| `GetSubjectVersions` | Lists versions for a subject. |
| `GetSchema` | Reads schema details. |
| `CheckCompatibility` | Checks compatibility for a schema. |
| `RegisterSchema` | Registers a new schema version. |
| `DeleteSubject` | Deletes subject or version according to API support. |

Kafka Connect methods:

| Method | Description |
| --- | --- |
| `ListConnectors` | Lists connectors for a Connect cluster. |
| `GetConnector` | Returns config and status. |
| `CreateConnector` | Creates a connector. |
| `UpdateConnectorConfig` | Updates connector config. |
| `PauseConnector` | Pauses connector. |
| `ResumeConnector` | Resumes connector. |
| `RestartConnector` | Restarts connector or tasks. |
| `DeleteConnector` | Deletes connector. |

### Message Browser

Message browsing must be bounded and explicit.

Request controls:

- topic
- partition
- start mode: newest, oldest, offset, timestamp
- offset
- timestamp
- limit
- max bytes per key/value
- key/value decode mode: text, json preview, hex, base64
- header display mode

Safety defaults:

- limit defaults to a small number.
- payload preview is truncated.
- binary payloads are not rendered as plain text unless safely detected.
- UI can browse messages after user action.
- AI message reads require explicit permission.

## Frontend Design

Kafka UI should be an operational workspace, not a landing page.

### Asset Form

Add `KafkaConfigSection`.

Sections:

- Bootstrap brokers
- Client ID
- SASL
- TLS
- SSH tunnel
- Message browser defaults
- Schema Registry companion config
- Kafka Connect companion config
- Advanced timeout settings
- Test connection

The form should reuse existing patterns from Redis, MongoDB, and database config sections:

- `PasswordSourceField` for inline or managed password.
- `AssetSelect` for SSH tunnel.
- existing toast and loading behavior for test connection.

### Kafka Panel

Create `KafkaPanel` under query components.

Layout:

- Left sidebar: Overview, Brokers, Topics, Consumer Groups, ACLs, Schema Registry, Kafka Connect.
- Main area: dense tables and detail tabs.
- Table-first interactions similar to Kafbat UI.
- No marketing hero, no decorative cards.

Core views:

| View | Main Content |
| --- | --- |
| Overview | cluster status, broker count, topic count, partition count, controller, security mode. |
| Brokers | broker table, broker config detail. |
| Topics | searchable topic table, create topic action. |
| Topic Detail | partitions, configs, messages, produce, danger zone. |
| Consumer Groups | group table with state and lag summary. |
| Consumer Group Detail | members, assignments, offsets, lag by topic/partition, reset offsets. |
| ACLs | ACL filters, ACL list, create/delete ACL. |
| Schema Registry | subjects, versions, schema content, compatibility, register/delete. |
| Kafka Connect | connector list, status, config, pause/resume/restart/delete. |

Admin and destructive UI actions must use confirmation dialogs. Confirmation copy should name the target resource and the exact action.

### Frontend State

Use a dedicated Kafka store instead of expanding `queryStore` too much.

Suggested store:

- `kafkaStates[tabId]`
- active view
- topic list state
- selected topic
- selected group
- selected ACL filters
- selected schema subject/version
- selected connect cluster/connector
- message browser request/result
- admin operation loading state
- loading and error states per panel

`queryStore` only needs to know that `kafka` is a query asset type and open a tab.

## AI Tool Design

Kafka AI tools must be structured, not a generic free-form Kafka command executor.

Do not add a separate static AI tool for every Kafka operation. `AllToolDefs()` serializes every tool definition into every AI conversation, so 20+ Kafka tools would create a large prompt tax even when no Kafka asset is in use.

Use a small set of grouped tools with explicit `operation` values. Each handler maps `operation + args` to one canonical permission command.

| Tool | Operations |
| --- | --- |
| `kafka_cluster` | `overview`, `list_brokers`, `get_broker_config`, `list_cluster_configs` |
| `kafka_topic` | `list`, `describe`, `create`, `delete`, `update_config`, `increase_partitions`, `delete_records` |
| `kafka_message` | `browse`, `inspect`, `produce` |
| `kafka_consumer_group` | `list`, `describe`, `reset_offset`, `delete` |
| `kafka_acl` | `list`, `create`, `delete` |
| `kafka_schema` | `list_subjects`, `list_versions`, `get`, `check_compatibility`, `register`, `delete` |
| `kafka_connect` | `list_connectors`, `get_connector`, `create`, `update_config`, `pause`, `resume`, `restart`, `delete` |

Examples of operation-to-permission mapping:

| Tool operation | Permission Command |
| --- | --- |
| `kafka_cluster.overview` | `cluster.read *` |
| `kafka_cluster.list_brokers` | `broker.read *` |
| `kafka_topic.list` | `topic.list *` |
| `kafka_topic.describe` | `topic.read <topic>` |
| `kafka_topic.create` | `topic.create <topic>` |
| `kafka_topic.delete` | `topic.delete <topic>` |
| `kafka_topic.update_config` | `topic.config.write <topic>` |
| `kafka_message.browse` | `message.read <topic>` |
| `kafka_message.produce` | `message.write <topic>` |
| `kafka_consumer_group.describe` | `consumer_group.read <group>` |
| `kafka_consumer_group.reset_offset` | `consumer_group.offset.write <group>` |
| `kafka_acl.list` | `acl.read *` |
| `kafka_acl.create` | `acl.write *` |
| `kafka_schema.get` | `schema.read <subject>` |
| `kafka_schema.register` | `schema.write <subject>` |
| `kafka_connect.get_connector` | `connect.read <connector>` |
| `kafka_connect.pause` | `connect.state.write <connector>` |
| `kafka_connect.delete` | `connect.delete <connector>` |

Tool behavior:

- Every tool must require `asset_id`.
- Every tool must require `operation`.
- Operation args should be a JSON object, not free-form command text.
- Tools must verify the target asset is Kafka.
- Tools must run through `CommandPolicyChecker`.
- Tools must set the check result for audit.
- Tools should return compact JSON suitable for model consumption.
- Each grouped tool handler dispatches on `operation` first; the tool's `CommandExtractor` function must also inspect `operation` to build the audit command string (e.g., for `kafka_topic` with `operation=describe`, produce `"topic.read " + topicName`).
- Message browsing must truncate payloads and include truncation metadata.
- Write/admin tools must return exact target summaries before and after execution where practical.

## Permission Model

Kafka should reuse the existing policy chain:

```text
asset policy
  -> group policy chain
  -> builtin policy groups
  -> DB grant
  -> user confirmation
  -> audit
```

### Kafka Policy Shape

Use the existing allow/deny list style:

| Field | Meaning |
| --- | --- |
| `allow_list` | Allowed Kafka action/resource patterns. |
| `deny_list` | Denied Kafka action/resource patterns. |
| `groups` | Referenced policy group IDs. |

Pattern examples:

- `cluster.read *`
- `broker.read *`
- `cluster.config.read *`
- `cluster.config.write *`
- `topic.list *`
- `topic.read orders-*`
- `topic.create orders-*`
- `topic.delete orders-*`
- `topic.config.write orders-*`
- `topic.partitions.write orders-*`
- `topic.records.delete orders-*`
- `message.read orders-*`
- `message.write orders-*`
- `consumer_group.read payment-*`
- `consumer_group.offset.write payment-*`
- `consumer_group.delete payment-*`
- `acl.read *`
- `acl.write *`
- `schema.read orders-*`
- `schema.write orders-*`
- `schema.delete orders-*`
- `connect.read jdbc-*`
- `connect.write jdbc-*`
- `connect.state.write jdbc-*`
- `connect.delete jdbc-*`

### Kafka Rule Grammar

Kafka must use a dedicated matcher, `MatchKafkaRule(rule, command string)`. Do not use `MatchCommandRule` for Kafka because it treats the first token as an exact program name and does not support wildcard matching in the action token.

Canonical permission command:

```text
<action> <resource>
```

Rule:

```text
<action-pattern> <resource-pattern>
```

Matching rules:

- Both rule and command must contain exactly two fields after trimming whitespace.
- `action` is matched as a complete string, not split into category and operation.
- `topic.config.write` is one action.
- `topic.*` matches `topic.read`, `topic.create`, `topic.config.write`, and other topic actions via glob matching.
- `resource` is matched separately with glob semantics.
- Actions are normalized to lowercase.
- Resources remain case-sensitive because Kafka topic names and consumer group IDs can be case-sensitive.
- Extra audit details such as partition, offset, config key, or connector action must not be appended to the permission command. Put them in approval detail and audit detail fields instead.

`MatchKafkaRule` is structurally similar to `MatchRedisRule` in `internal/ai/redis_policy.go`: split on whitespace into exactly two fields, then apply `path.Match` glob semantics to each field independently. Key differences from both existing matchers: unlike `MatchCommandRule` there is no program-name concept, subcommand list, or flag parsing; unlike `MatchRedisRule` the first field (action) is glob-matched after lowercase normalization rather than exact-matched after uppercase normalization, and the second field (resource) is case-sensitive. Both rule and command must parse to exactly two whitespace-separated fields — inputs that do not are a non-match.

Examples:

| Rule | Command | Result |
| --- | --- | --- |
| `topic.* *` | `topic.config.write orders` | match |
| `topic.read orders-*` | `topic.read orders-v1` | match |
| `topic.read orders-*` | `message.read orders-v1` | no match |
| `consumer_group.offset.write payment-*` | `consumer_group.offset.write payment-api` | match |

Kafka Grant matching must use the same matcher. If the existing grant path only accepts `MatchCommandRule`, add a match-function variant rather than forcing Kafka rules into command-rule syntax.

### Built-in Policy Groups

Add policy type: `kafka`.

Built-in groups:

| ID | Description |
| --- | --- |
| `builtin:kafka-metadata-readonly` | Allows cluster, broker, topic metadata, and consumer group metadata reads. |
| `builtin:kafka-message-read` | Allows reading message payloads. Not enabled by default. |
| `builtin:kafka-schema-readonly` | Allows Schema Registry read operations. |
| `builtin:kafka-connect-readonly` | Allows Kafka Connect read operations. |
| `builtin:kafka-operator` | Allows non-security admin operations such as topic create/config update/offset reset. Not enabled by default. |
| `builtin:kafka-security-admin` | Allows ACL changes. Not enabled by default. |
| `builtin:kafka-dangerous-deny` | Denies destructive and high-risk operations by default. |

Default Kafka policy:

- includes `builtin:kafka-metadata-readonly`
- includes `builtin:kafka-schema-readonly` only when Schema Registry is enabled and user opts in
- includes `builtin:kafka-connect-readonly` only when Kafka Connect is enabled and user opts in
- includes `builtin:kafka-dangerous-deny`
- does not include `builtin:kafka-message-read`
- does not include operator or security-admin groups

This means AI can inspect Kafka structure by default, but cannot read message contents or perform write/admin/security operations unless policy is changed.

### Confirmation and Grant

Kafka should use existing confirmation and grant behavior.

Examples:

- `message.read orders` can ask for confirmation.
- `message.read orders-*` can be granted for the session.
- `topic.create orders-*` can be granted only if no deny rule blocks it.
- `topic.delete orders` should be denied by dangerous-deny unless a user deliberately removes or overrides the deny policy.

Deny policy must win over grant. This prevents temporary grants from bypassing hard safety rules.

Implementation touchpoints:

- `CheckPermission()` must dispatch `asset_entity.AssetTypeKafka` to `checkKafkaPermission()`.
- `checkKafkaPermission()` must run group generic policy, Kafka policy, Grant matching, then confirmation in the same order as existing Database/Redis/MongoDB checks.
- `collectKafkaPolicies()` must collect Kafka policies from asset and group chain.
- `resolveKafkaGroups()` must read Kafka policy groups.
- `CommandPolicyChecker.handleConfirm()` must map Kafka to approval item `type="kafka"`.
- Grant matching must use `MatchKafkaRule`. The existing grant path (`matchGrantForAsset`) uses `MatchCommandRule` and must be extended: add `matchGrantForAssetWith(ctx context.Context, assetID int64, command string, matchFn MatchFunc) *CheckResult` alongside the existing function, and call this variant from `checkKafkaPermission`. No change is needed to the grant entity or grant storage schema.
- The approval item command should be the canonical permission command, while the detail field can show richer context such as partition, offset, config key, connector name, or schema subject.

### UI Permissions

AI operations must always use policy checks. UI operations should also distinguish safe and dangerous actions:

- Read-only UI actions execute directly.
- Message browse UI actions execute after explicit user interaction.
- Write/admin/security UI actions require confirmation dialogs.
- Destructive UI actions should never be hidden behind bulk shortcuts without confirmation.

### Audit

Audit entries should record concise Kafka operation summaries.

Examples:

- `topic.read orders`
- `message.read orders partition=0 offset=123 limit=20`
- `message.write orders key=user-1 headers=2 value_bytes=128`
- `consumer_group.offset.write payment-service topic=orders partition=0 offset=latest`
- `topic.config.write orders retention.ms`
- `acl.write principal=User:alice operation=READ resource=Topic:orders`
- `schema.write subject=orders-value`
- `connect.state.write connector=jdbc-sink action=pause`

Sensitive values must not be logged:

- SASL password
- message value body
- full message headers if they may contain secrets
- TLS key material
- bearer tokens
- Schema Registry or Connect credentials

## Policy UI

Frontend policy management needs Kafka support.

Changes:

- Add Kafka tab in policy group manager.
- Add Kafka policy selector mapping.
- Add Kafka policy fields to asset type definition:
  - allow list
  - deny list
- Add i18n for:
  - Kafka policy title
  - Kafka policy hints
  - Kafka built-in groups
  - placeholders for action/resource patterns
- Update hardcoded policy type maps in frontend policy components so `kafka` resolves to backend policy type `kafka`.
- Ensure policy group creation uses `policyType="kafka"` and passes backend validation.

The UI can reuse `PolicyTagEditor` because Kafka rules are simple strings.

## Integration Points

### Backend Files

Expected additions or modifications:

| File or Area | Required Change |
| --- | --- |
| `migrations/` | Add migration for `groups.kafka_policy`, following the existing MongoDB group-policy migration pattern. |
| `internal/model/entity/policy/policy.go` | Add `KafkaPolicy`, `DefaultKafkaPolicy()`, Kafka built-in policy IDs, and `GetKafkaPolicy()` to `policy.Holder`. |
| `internal/model/entity/policy/registry.go` | Register `RegisterDefaultPolicy("kafka", ...)` in `init()`. |
| `internal/model/entity/group_entity/group.go` | Add `KfkPolicy`/`kafka_policy` column field plus `GetKafkaPolicy()` and `SetKafkaPolicy()`. |
| `internal/model/entity/asset_entity/asset.go` | Add `AssetTypeKafka`, `KafkaConfig`, `KafkaPolicy` alias, config getter/setter, `IsKafka()`, `GetKafkaPolicy()`, and `SetKafkaPolicy()`. `KafkaPolicy` is stored in the existing `CmdPolicy` text field on `Asset` (no new column on the `assets` table), consistent with how `RedisPolicy` and `MongoPolicy` are stored for their respective asset types. |
| `internal/model/entity/policy_group_entity/policy_group.go` | Add `PolicyTypeKafka`, include it in `Validate()`, add Kafka built-in groups. |
| `internal/assettype/kafka.go` | Register Kafka asset handler and default policy; implement safe view, default port, password resolution, and create/update args. |
| `internal/connpool/kafka.go` | Build franz-go clients with TLS/SASL/SSH tunnel and optional TTL manager. |
| `internal/service/credential_resolver/resolver.go` | Add Kafka password resolver if useful; companion sub-configs can use `ResolvePasswordGeneric()`. |
| `internal/service/kafka_svc/*` | Implement typed Kafka, Schema Registry, and Kafka Connect service operations. |
| `internal/app/app.go` | Own and close `kafka_svc.Service` in App lifecycle. |
| `internal/app/app_kafka.go` | Expose Wails methods for Kafka UI. |
| `internal/app/app_approval.go` and approval UI | Ensure approval payloads with `type="kafka"` render with Kafka labels and details. |
| `internal/ai/kafka_policy.go` | Add `MatchKafkaRule`, merge/check policy functions, tests. |
| `internal/ai/kafka_helper.go` | Add grouped Kafka AI tool handlers and optional session-scoped Kafka connection cache. |
| `internal/ai/permission.go` | Add Kafka case in `CheckPermission()` and implement `checkKafkaPermission()`. |
| `internal/ai/command_policy.go` | Add `collectKafkaPolicies()`, map Kafka confirmation to approval type `kafka`, and add `matchGrantForAssetWith(ctx, assetID, command string, matchFn MatchFunc) *CheckResult` alongside the existing grant matching path. |
| `internal/ai/policy_group_resolve.go` | Add `resolveKafkaGroups()`. |
| `internal/ai/tool_registry.go` | Add grouped Kafka tools, not one static tool per operation; update existing asset-type descriptions for list/add/update asset tools. |

### Frontend Files

Expected additions or modifications:

- `frontend/src/lib/assetTypes/kafka.ts`
- `frontend/src/lib/assetTypes/index.ts`
- `frontend/src/stores/queryStore.ts`
- `frontend/src/stores/kafkaStore.ts`
- `frontend/src/components/layout/MainPanel.tsx`
- `frontend/src/components/asset/AssetForm.tsx`
- `frontend/src/components/asset/KafkaConfigSection.tsx`
- `frontend/src/components/asset/detail/KafkaDetailInfoCard.tsx`
- `frontend/src/components/query/KafkaPanel.tsx`
- `frontend/src/components/query/KafkaBrokerList.tsx`
- `frontend/src/components/query/KafkaTopicList.tsx`
- `frontend/src/components/query/KafkaTopicDetail.tsx`
- `frontend/src/components/query/KafkaConsumerGroups.tsx`
- `frontend/src/components/query/KafkaMessageBrowser.tsx`
- `frontend/src/components/query/KafkaAclPanel.tsx`
- `frontend/src/components/query/KafkaSchemaRegistryPanel.tsx`
- `frontend/src/components/query/KafkaConnectPanel.tsx`
- frontend approval components that display AI approval item types
- frontend i18n files

## Delivery Plan

The target scope is complete Kafka management. The phases below are engineering order, not product scope cuts. Each phase should preserve the final API and state model so later capabilities do not require reshaping earlier work.

### Phase 1: Foundation

- Add Kafka asset type and config.
- Add Kafka credential resolution.
- Add Kafka connection manager with franz-go.
- Support TLS, SASL, and SSH tunnel.
- Add test connection Wails method.
- Add Kafka asset form and detail card.
- Add Kafka policy type, built-in groups, and frontend policy UI from the start.
- Add `groups.kafka_policy` migration.
- Extend `policy.Holder`, `Asset`, and `Group` together.
- Add `PolicyTypeKafka` to policy group validation.
- Add Kafka default-policy registration in `policy/registry.go` and `assettype/kafka.go`.
- Add `MatchKafkaRule` before Kafka AI tools or Grant support are implemented.

Exit criteria:

- User can create, edit, test, and assign policies to a Kafka asset.
- Direct and SSH tunnel connection paths are covered.
- Permission model exists before AI or admin actions are added.
- Existing tests and all generated mock implementations compile after the `policy.Holder` interface change. Run `go generate ./...` as part of this step to regenerate all `mock_*/` files before verifying compilation.

### Phase 2: Core Read and Workspace

- Add `kafka_svc`.
- Implement cluster overview.
- Implement broker list and broker config view.
- Implement topic list and topic detail.
- Implement consumer group list and detail.
- Add `KafkaPanel` and dedicated Kafka frontend store.

Exit criteria:

- Kafka assets open a usable management workspace.
- UI can render overview, brokers, topics, and consumer groups from real clusters.

### Phase 3: Message Read and Produce

- Implement bounded message browsing.
- Add offset and timestamp modes.
- Add payload preview and truncation.
- Add decode modes.
- Implement produce message.
- Add UI confirmation for produce.
- Add AI permission checks for message read/write.
- Use grouped AI tool handlers and canonical permission commands.

Exit criteria:

- User can inspect and produce messages safely.
- AI message reads and writes are policy-controlled and audited.

### Phase 4: Topic and Consumer Group Admin

- Create topic.
- Delete topic.
- Alter topic configs.
- Increase partitions.
- Delete records.
- Reset consumer group offsets.
- Delete consumer groups when supported.
- Add UI confirmations and audit summaries.
- Add AI tools and policy checks.

Exit criteria:

- Core Kafka admin workflows are available and guarded.

### Phase 5: ACLs and Security Admin

- List ACLs.
- Create ACLs.
- Delete ACLs.
- Add ACL filter UI.
- Add AI ACL tools with strict policy checks.

Exit criteria:

- Kafka security operations are possible but never allowed by default.

### Phase 6: Schema Registry

- Add Schema Registry HTTP client.
- List subjects.
- Show versions and schema content.
- Check compatibility.
- Register schema.
- Delete subject/version.
- Add AI schema tools with policy checks.

Exit criteria:

- Schema Registry is managed from the Kafka workspace when configured.

### Phase 7: Kafka Connect

- Add Kafka Connect HTTP client.
- List connectors.
- Show connector config and status.
- Create/update/delete connectors.
- Pause/resume/restart connectors and tasks.
- Add AI Connect tools with policy checks.

Exit criteria:

- Kafka Connect is managed from the Kafka workspace when configured.

## Testing Strategy

Backend tests:

- Kafka config validation.
- SASL option construction.
- TLS option construction.
- SSH tunnel dialer behavior.
- Kafka client cache invalidation.
- `policy.Holder` compile-time coverage for `Asset` and `Group`.
- `groups.kafka_policy` migration.
- `PolicyGroup.Validate()` accepts `kafka`.
- Kafka policy matching.
- Kafka Grant matching uses `MatchKafkaRule`.
- Built-in policy groups.
- AI tool permission checks.
- Kafka confirmation uses approval type `kafka`.
- Audit summary generation.
- Companion HTTP client auth and TLS behavior.

Frontend tests:

- Kafka asset form create/edit behavior.
- Kafka policy UI rendering.
- Kafka panel empty/loading/error states.
- Message browser request validation.
- Admin confirmation dialogs.
- ACL, Schema Registry, and Connect panel state.

Integration tests:

- Local Kafka cluster without auth.
- Kafka cluster with SASL.
- Kafka cluster with TLS when practical.
- SSH tunnel path when practical.
- Topic create/delete/config update.
- Produce and consume round trip.
- Consumer group offset reset with a test group.
- Schema Registry when configured.
- Kafka Connect when configured.

Manual checks:

- Direct connection.
- SSH tunnel connection.
- Metadata reads.
- Consumer group lag.
- Message browsing truncation.
- Produce message confirmation.
- Topic admin confirmation.
- ACL admin confirmation.
- AI permission confirmation.
- Grant behavior.
- Audit entry content.

## Risks and Decisions

### Connection Pooling

Kafka clients are stateful and expensive enough that one-off clients will become painful.

Decision: design and implement a small TTL client manager from the start, owned by `kafka_svc.Service` and closed from App cleanup. Do not introduce an unmanaged global singleton. Until there is a generic asset-update hook, config fingerprint comparison is the required stale-client invalidation mechanism.

### Policy Interface Change

Adding Kafka changes `policy.Holder`, which is implemented by both `Asset` and `Group`.

Decision: update `policy.Holder`, `asset_entity.Asset`, and `group_entity.Group` in the same implementation step, with compile-time tests or existing tests verifying both implementations.

### AI Tool Prompt Cost

Adding one static AI tool per Kafka operation would increase every conversation's tool prompt, even when no Kafka asset is relevant.

Decision: use grouped Kafka tools with explicit `operation` fields. Keep permission checks precise by mapping every operation to a canonical Kafka permission command.

### Message Sensitivity

Kafka messages may contain secrets or user data.

Decision: AI message reads are not allowed by default and payloads are truncated.

### Admin Operations

Kafka admin operations can be destructive.

Decision: include admin operations in the target design, but deny them by default and require explicit UI confirmation or policy changes.

### Companion APIs

Schema Registry and Kafka Connect are HTTP APIs, not part of franz-go.

Decision: keep franz-go as the Kafka broker protocol backend and add small typed HTTP clients for companion APIs. Companion credential sub-configs implement the same password-source interface so they can use `ResolvePasswordGeneric()`.

### Plugin Future

Kafka is not implemented as a plugin now, but the design should not block future plugin capabilities.

Decision: build Kafka as a native first-party module and later use lessons from Kafka to improve the extension system.

## Resolved Design Decisions

The following questions were open during design and have been closed:

- **Message payload storage**: In-memory only. Message payloads are not persisted or exported unless explicitly designed in a later phase.
- **Clusters per asset**: One cluster per asset. `KafkaConfig.brokers` covers a single cluster's bootstrap brokers; users who manage multiple clusters create multiple Kafka assets.
- **Companion credentials**: Always configured separately. Schema Registry and Kafka Connect each have their own credential sub-config and are never inherited from the Kafka asset credentials.
- **Read-only UI actions and policy**: Read-only UI actions execute directly without policy checks. Policy checks apply to AI tools, message browse actions (explicit user trigger), and dangerous/write/admin UI actions.

## Open Questions

- Should ACL editing be hidden unless the cluster reports ACL support, or always shown?
- Should users be able to explicitly export selected messages to a file in a later phase?
