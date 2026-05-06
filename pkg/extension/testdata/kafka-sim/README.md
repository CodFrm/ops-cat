# Kafka Simulator

`pkg/extension/testdata/kafka-sim` is a local Kafka smoke, coverage, and
stress-test helper for validating OpsKat Kafka management screens against a
real Kafka cluster.

It creates only generated resources with a configurable prefix. Cleanup and
record deletion are opt-in so existing topics are not touched by default.

## Quick Start

```powershell
go run ./pkg/extension/testdata/kafka-sim `
  -brokers 192.168.100.50:9092 `
  -run-id ui-smoke-20260502 `
  -messages 200 `
  -payload-bytes 512 `
  -producers 2 `
  -batch-size 50 `
  -browse-limit 10 `
  -increase-to 2 `
  -timeout 30s
```

After the run, open the Kafka asset in the app and verify:

- `Topics`: search the run id, for example `ui-smoke-20260502`.
- `Topics -> <run>-messages`: browse from oldest and confirm generated records.
- `Consumer Groups`: find `<prefix>-<run>-group` and confirm lag.
- `ACLs`: a PLAINTEXT local broker without authorizer should show the expected
  security-disabled error.

## Coverage

The simulator exercises:

- Cluster metadata and broker listing.
- Topic create, list, describe, config update, and partition increase.
- Message production with configurable concurrency, batch size, payload size,
  and optional rate limit.
- Direct message browsing from the beginning of a generated topic.
- Consumer group consumption, offset commits, and lag inspection.
- ACL describe probe, including the expected `SECURITY_DISABLED` path on local
  Kafka without an authorizer.
- Optional DeleteRecords on a generated delete-only topic.
- Optional Schema Registry subject register, compatibility check, and cleanup.
- Optional Kafka Connect root, connector list, and plugin list reads.

## Stress Profiles

Short stress run:

```powershell
go run ./pkg/extension/testdata/kafka-sim `
  -brokers 192.168.100.50:9092 `
  -run-id stress-1 `
  -messages 10000 `
  -payload-bytes 1024 `
  -producers 8 `
  -batch-size 200 `
  -browse-limit 20 `
  -timeout 60s
```

Time-boxed run:

```powershell
go run ./pkg/extension/testdata/kafka-sim `
  -brokers 192.168.100.50:9092 `
  -run-id stress-30s `
  -duration 30s `
  -payload-bytes 512 `
  -producers 8 `
  -batch-size 200 `
  -rate 2000 `
  -timeout 60s
```

## Destructive Coverage

DeleteRecords is disabled by default. Enable it only for generated topics:

```powershell
go run ./pkg/extension/testdata/kafka-sim `
  -brokers 192.168.100.50:9092 `
  -run-id delete-records-smoke `
  -messages 100 `
  -delete-records `
  -cleanup `
  -timeout 60s
```

Cleanup deletes generated topics and the generated consumer group without
running a new simulation:

```powershell
go run ./pkg/extension/testdata/kafka-sim `
  -brokers 192.168.100.50:9092 `
  -run-id ui-smoke-20260502 `
  -cleanup-only `
  -timeout 60s
```

## Optional Companion Services

Schema Registry:

```powershell
go run ./pkg/extension/testdata/kafka-sim `
  -brokers 192.168.100.50:9092 `
  -run-id schema-smoke `
  -schema-registry-url http://localhost:8081 `
  -cleanup
```

Kafka Connect:

```powershell
go run ./pkg/extension/testdata/kafka-sim `
  -brokers 192.168.100.50:9092 `
  -run-id connect-smoke `
  -connect-url http://localhost:8083 `
  -cleanup
```

## Notes

- Use `-timeout 30s` or higher for local clusters that occasionally take longer
  to accept Kafka protocol handshakes.
- The default brokers come from `OPSKAT_KAFKA_TEST_BROKERS`; if unset, the
  script defaults to `192.168.100.50:9092`.
- The script prints a summary JSON at the end so run output can be attached to
  bug reports or compared with UI state.
