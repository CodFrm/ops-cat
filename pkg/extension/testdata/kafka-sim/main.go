package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
)

type config struct {
	brokers           string
	prefix            string
	runID             string
	replicationFactor int
	partitions        int
	increaseTo        int
	messages          int64
	payloadBytes      int
	producers         int
	batchSize         int
	rate              int
	duration          time.Duration
	consumeTarget     int64
	browseLimit       int
	deleteRecords     bool
	cleanup           bool
	cleanupOnly       bool
	timeout           time.Duration
	schemaRegistryURL string
	connectURL        string
}

type topicPlan struct {
	AdminTopic   string
	MessageTopic string
	DeleteTopic  string
	AllTopics    []string
}

type produceStats struct {
	Attempted int64         `json:"attempted"`
	Produced  int64         `json:"produced"`
	Failed    int64         `json:"failed"`
	Duration  time.Duration `json:"duration"`
	Rate      float64       `json:"records_per_second"`
}

type consumeStats struct {
	Group    string `json:"group"`
	Topic    string `json:"topic"`
	Consumed int64  `json:"consumed"`
	Target   int64  `json:"target"`
}

type sampleRecord struct {
	Topic     string `json:"topic"`
	Partition int32  `json:"partition"`
	Offset    int64  `json:"offset"`
	Key       string `json:"key"`
	Value     string `json:"value"`
}

type runSummary struct {
	Brokers          []string       `json:"brokers"`
	Topics           topicPlan      `json:"topics"`
	Group            string         `json:"group"`
	Produced         produceStats   `json:"produced"`
	Consumed         consumeStats   `json:"consumed"`
	BrowseSamples    []sampleRecord `json:"browse_samples"`
	SchemaRegistry   string         `json:"schema_registry,omitempty"`
	KafkaConnect     string         `json:"kafka_connect,omitempty"`
	CleanupPerformed bool           `json:"cleanup_performed"`
}

func main() {
	cfg := parseFlags()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := run(ctx, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "\nFAILED: %v\n", err)
		os.Exit(1)
	}
}

func parseFlags() config {
	defaultBrokers := strings.TrimSpace(os.Getenv("OPSKAT_KAFKA_TEST_BROKERS"))
	if defaultBrokers == "" {
		defaultBrokers = "192.168.100.50:9092"
	}

	cfg := config{}
	flag.StringVar(&cfg.brokers, "brokers", defaultBrokers, "comma separated Kafka bootstrap brokers")
	flag.StringVar(&cfg.prefix, "prefix", "opskat-sim", "prefix for generated topics, groups, and subjects")
	flag.StringVar(&cfg.runID, "run-id", time.Now().Format("20060102-150405"), "run id appended to generated resource names")
	flag.IntVar(&cfg.replicationFactor, "replication-factor", 1, "replication factor for generated topics")
	flag.IntVar(&cfg.partitions, "partitions", 1, "initial partitions for generated topics")
	flag.IntVar(&cfg.increaseTo, "increase-to", 2, "increase admin topic to this partition count; set <= partitions to skip")
	flag.Int64Var(&cfg.messages, "messages", 200, "number of messages to produce when -duration is 0")
	flag.IntVar(&cfg.payloadBytes, "payload-bytes", 512, "approximate payload size for produced messages")
	flag.IntVar(&cfg.producers, "producers", 2, "number of concurrent producers")
	flag.IntVar(&cfg.batchSize, "batch-size", 50, "sync produce batch size per producer")
	flag.IntVar(&cfg.rate, "rate", 0, "global produce rate limit in records/sec; 0 means unlimited")
	flag.DurationVar(&cfg.duration, "duration", 0, "produce for a fixed duration instead of using -messages, for example 30s")
	flag.Int64Var(&cfg.consumeTarget, "consume-target", -1, "records to consume and commit for lag simulation; -1 means half of produced")
	flag.IntVar(&cfg.browseLimit, "browse-limit", 10, "records to read directly from the message topic for UI browse verification")
	flag.BoolVar(&cfg.deleteRecords, "delete-records", false, "also exercise DeleteRecords on a generated delete-only topic")
	flag.BoolVar(&cfg.cleanup, "cleanup", false, "delete generated topics and group at the end")
	flag.BoolVar(&cfg.cleanupOnly, "cleanup-only", false, "only delete generated topics and group for the run id, then exit")
	flag.DurationVar(&cfg.timeout, "timeout", 30*time.Second, "per admin/request timeout")
	flag.StringVar(&cfg.schemaRegistryURL, "schema-registry-url", "", "optional Schema Registry URL to exercise subject list/register/compatibility")
	flag.StringVar(&cfg.connectURL, "connect-url", "", "optional Kafka Connect URL to exercise cluster/connectors/plugin reads")
	flag.Parse()

	return cfg
}

func run(ctx context.Context, cfg config) error {
	if err := validateConfig(cfg); err != nil {
		return err
	}

	brokers := splitCSV(cfg.brokers)
	plan := buildTopicPlan(cfg)
	group := fmt.Sprintf("%s-%s-group", safeName(cfg.prefix), safeName(cfg.runID))
	summary := runSummary{Brokers: brokers, Topics: plan, Group: group}

	fmt.Printf("Kafka simulator run: brokers=%s prefix=%s run=%s\n", strings.Join(brokers, ","), cfg.prefix, cfg.runID)
	fmt.Printf("Generated resources:\n  admin topic:   %s\n  message topic: %s\n  group:         %s\n", plan.AdminTopic, plan.MessageTopic, group)
	if cfg.deleteRecords {
		fmt.Printf("  delete topic:  %s\n", plan.DeleteTopic)
	}

	client, admin, err := newAdminClient(ctx, brokers, cfg.timeout)
	if err != nil {
		return err
	}
	defer client.Close()

	if cfg.cleanupOnly {
		if err := phase("cleanup only", func() error {
			return cleanupGenerated(ctx, admin, plan.AllTopics, group)
		}); err != nil {
			return err
		}
		summary.CleanupPerformed = true
		printSummary(summary)
		return nil
	}

	if err := phase("cluster metadata", func() error {
		return printClusterMetadata(ctx, admin)
	}); err != nil {
		return err
	}

	if err := phase("topic create/list/config/partitions", func() error {
		return prepareTopics(ctx, admin, cfg, plan)
	}); err != nil {
		return err
	}

	produced, err := runProducePhase(ctx, cfg, brokers, plan.MessageTopic)
	if err != nil {
		return err
	}
	summary.Produced = produced

	consumed, err := runConsumerGroupPhase(ctx, cfg, brokers, group, plan.MessageTopic, produced.Produced)
	if err != nil {
		return err
	}
	summary.Consumed = consumed

	samples, err := runBrowsePhase(ctx, cfg, brokers, plan.MessageTopic)
	if err != nil {
		return err
	}
	summary.BrowseSamples = samples

	if err := phase("consumer group lag", func() error {
		return printGroupLag(ctx, admin, group)
	}); err != nil {
		return err
	}

	if err := phase("acl read probe", func() error {
		return probeACLs(ctx, admin)
	}); err != nil {
		fmt.Printf("ACL probe returned expected/handled error: %v\n", err)
	}

	if cfg.deleteRecords {
		if err := phase("delete-records exercise", func() error {
			return runDeleteRecordsExercise(ctx, cfg, brokers, admin, plan.DeleteTopic)
		}); err != nil {
			return err
		}
	}

	if strings.TrimSpace(cfg.schemaRegistryURL) != "" {
		if err := phase("schema registry exercise", func() error {
			return exerciseSchemaRegistry(ctx, cfg)
		}); err != nil {
			return err
		}
		summary.SchemaRegistry = strings.TrimRight(cfg.schemaRegistryURL, "/")
	}

	if strings.TrimSpace(cfg.connectURL) != "" {
		if err := phase("kafka connect exercise", func() error {
			return exerciseKafkaConnect(ctx, cfg.connectURL)
		}); err != nil {
			return err
		}
		summary.KafkaConnect = strings.TrimRight(cfg.connectURL, "/")
	}

	if cfg.cleanup {
		if err := phase("cleanup", func() error {
			return cleanupGenerated(ctx, admin, plan.AllTopics, group)
		}); err != nil {
			return err
		}
		summary.CleanupPerformed = true
	}

	printSummary(summary)

	fmt.Println("\nUI verification checklist:")
	fmt.Printf("  1. Open Kafka asset -> Topics, search %q.\n", safeName(cfg.runID))
	fmt.Printf("  2. Open %s, verify partitions/config and use Browse Messages from oldest.\n", plan.MessageTopic)
	fmt.Printf("  3. Open Consumer Groups, verify %s and lag details.\n", group)
	if cfg.deleteRecords {
		fmt.Printf("  4. DeleteRecords was exercised on %s; verify low watermark / empty reads.\n", plan.DeleteTopic)
	} else {
		fmt.Println("  4. Run with -delete-records to exercise record deletion on a generated topic.")
	}
	if !cfg.cleanup {
		fmt.Println("  Cleanup is OFF. Re-run with the same -run-id -cleanup to delete generated topics/group.")
	}

	return nil
}

func validateConfig(cfg config) error {
	if len(splitCSV(cfg.brokers)) == 0 {
		return errors.New("-brokers is required")
	}
	if cfg.replicationFactor <= 0 {
		return errors.New("-replication-factor must be > 0")
	}
	if cfg.partitions <= 0 {
		return errors.New("-partitions must be > 0")
	}
	if cfg.messages < 0 {
		return errors.New("-messages must be >= 0")
	}
	if cfg.payloadBytes < 0 {
		return errors.New("-payload-bytes must be >= 0")
	}
	if cfg.producers <= 0 {
		return errors.New("-producers must be > 0")
	}
	if cfg.batchSize <= 0 {
		return errors.New("-batch-size must be > 0")
	}
	if cfg.rate < 0 {
		return errors.New("-rate must be >= 0")
	}
	if cfg.duration < 0 {
		return errors.New("-duration must be >= 0")
	}
	if cfg.browseLimit < 0 {
		return errors.New("-browse-limit must be >= 0")
	}
	if cfg.timeout <= 0 {
		return errors.New("-timeout must be > 0")
	}
	return nil
}

func buildTopicPlan(cfg config) topicPlan {
	base := safeName(cfg.prefix) + "-" + safeName(cfg.runID)
	plan := topicPlan{
		AdminTopic:   base + "-admin",
		MessageTopic: base + "-messages",
		DeleteTopic:  base + "-delete-records",
	}
	plan.AllTopics = []string{plan.AdminTopic, plan.MessageTopic}
	if cfg.deleteRecords || cfg.cleanupOnly {
		plan.AllTopics = append(plan.AllTopics, plan.DeleteTopic)
	}
	return plan
}

func printSummary(summary runSummary) {
	fmt.Println("\nSummary JSON:")
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(summary)
}

func phase(name string, fn func() error) error {
	start := time.Now()
	fmt.Printf("\n== %s ==\n", name)
	if err := fn(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	fmt.Printf("OK %s (%s)\n", name, time.Since(start).Round(time.Millisecond))
	return nil
}

func newAdminClient(ctx context.Context, brokers []string, timeout time.Duration) (*kgo.Client, *kadm.Client, error) {
	client, err := connectKafkaClient(ctx, brokers, "opskat-kafka-sim-admin", timeout)
	if err != nil {
		return nil, nil, err
	}
	admin := kadm.NewClient(client)
	admin.SetTimeoutMillis(int32(timeout / time.Millisecond))
	return client, admin, nil
}

func connectKafkaClient(ctx context.Context, brokers []string, clientID string, timeout time.Duration, extra ...kgo.Opt) (*kgo.Client, error) {
	deadline := time.Now().Add(timeout)
	attempt := 0
	var lastErr error
	for {
		attempt++
		attemptTimeout := timeout / 3
		if attemptTimeout < 2*time.Second {
			attemptTimeout = 2 * time.Second
		}
		if attemptTimeout > 5*time.Second {
			attemptTimeout = 5 * time.Second
		}
		if remaining := time.Until(deadline); remaining < attemptTimeout {
			attemptTimeout = remaining
		}
		if attemptTimeout <= 0 {
			break
		}

		client, err := kgo.NewClient(baseKafkaOpts(brokers, clientID, attemptTimeout, extra...)...)
		if err != nil {
			lastErr = fmt.Errorf("create Kafka client: %w", err)
		} else {
			pingCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
			err = client.Ping(pingCtx)
			cancel()
			if err == nil {
				if attempt > 1 {
					fmt.Printf("Kafka connected after %d attempts\n", attempt)
				}
				return client, nil
			}
			client.Close()
			lastErr = fmt.Errorf("ping Kafka: %w", err)
		}

		if time.Now().Add(500 * time.Millisecond).After(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	if lastErr == nil {
		lastErr = context.DeadlineExceeded
	}
	return nil, lastErr
}

func baseKafkaOpts(brokers []string, clientID string, timeout time.Duration, extra ...kgo.Opt) []kgo.Opt {
	opts := []kgo.Opt{
		kgo.SeedBrokers(brokers...),
		kgo.ClientID(clientID),
		kgo.DisableClientMetrics(),
		kgo.DialTimeout(timeout),
		kgo.RetryTimeout(timeout),
		kgo.RequestTimeoutOverhead(timeout),
	}
	return append(opts, extra...)
}

func printClusterMetadata(ctx context.Context, admin *kadm.Client) error {
	meta, err := admin.Metadata(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("Cluster ID: %s\n", meta.Cluster)
	fmt.Printf("Controller: %d\n", meta.Controller)
	brokers := meta.Brokers.NodeIDs()
	sort.Slice(brokers, func(i, j int) bool { return brokers[i] < brokers[j] })
	for _, id := range brokers {
		for _, b := range meta.Brokers {
			if b.NodeID == id {
				fmt.Printf("Broker %d: %s:%d\n", id, b.Host, b.Port)
				break
			}
		}
	}
	return nil
}

func prepareTopics(ctx context.Context, admin *kadm.Client, cfg config, plan topicPlan) error {
	configs := map[string]*string{
		"cleanup.policy": ptr("delete"),
		"retention.ms":   ptr("3600000"),
	}
	for _, topic := range plan.AllTopics {
		if err := createTopic(ctx, admin, topic, int32(cfg.partitions), int16(cfg.replicationFactor), configs); err != nil {
			return err
		}
	}

	if cfg.increaseTo > cfg.partitions {
		responses, err := admin.UpdatePartitions(ctx, cfg.increaseTo, plan.AdminTopic)
		if err != nil {
			return fmt.Errorf("increase partitions for %s: %w", plan.AdminTopic, err)
		}
		if response, ok := responses[plan.AdminTopic]; ok && response.Err != nil {
			if !strings.Contains(response.Err.Error(), "INVALID_PARTITIONS") {
				return fmt.Errorf("increase partitions for %s: %w", plan.AdminTopic, response.Err)
			}
			fmt.Printf("Partition increase skipped: %v\n", response.Err)
		}
	}

	altered, err := admin.AlterTopicConfigs(ctx, []kadm.AlterConfig{
		{Op: kadm.SetConfig, Name: "retention.ms", Value: ptr("7200000")},
	}, plan.AdminTopic)
	if err != nil {
		return fmt.Errorf("alter topic config for %s: %w", plan.AdminTopic, err)
	}
	if response, err := altered.On(plan.AdminTopic, nil); err != nil {
		return fmt.Errorf("alter topic config response for %s: %w", plan.AdminTopic, err)
	} else if response.Err != nil {
		return fmt.Errorf("alter topic config for %s: %w", plan.AdminTopic, response.Err)
	}

	details, err := admin.ListTopics(ctx, plan.AllTopics...)
	if err != nil {
		return err
	}
	for _, detail := range details.Sorted() {
		if detail.Err != nil {
			return fmt.Errorf("topic %s detail error: %w", detail.Topic, detail.Err)
		}
		fmt.Printf("Topic %s: partitions=%d internal=%v\n", detail.Topic, len(detail.Partitions), detail.IsInternal)
	}
	return nil
}

func createTopic(ctx context.Context, admin *kadm.Client, topic string, partitions int32, rf int16, configs map[string]*string) error {
	created, err := admin.CreateTopic(ctx, partitions, rf, configs, topic)
	if err != nil {
		return fmt.Errorf("create topic %s: %w", topic, err)
	}
	if created.Err != nil {
		if errors.Is(created.Err, kerr.TopicAlreadyExists) {
			fmt.Printf("Topic %s already exists, reusing it\n", topic)
			return nil
		}
		return fmt.Errorf("create topic %s: %w", topic, created.Err)
	}
	fmt.Printf("Created topic %s\n", topic)
	return nil
}

func runProducePhase(ctx context.Context, cfg config, brokers []string, topic string) (produceStats, error) {
	var stats produceStats
	err := phase("produce messages", func() error {
		start := time.Now()
		produceCtx := ctx
		var cancel context.CancelFunc
		if cfg.duration > 0 {
			produceCtx, cancel = context.WithTimeout(ctx, cfg.duration)
			defer cancel()
		}

		var seq atomic.Int64
		var produced atomic.Int64
		var failed atomic.Int64
		var firstErr atomic.Value
		var wg sync.WaitGroup

		var limiter <-chan time.Time
		var stopLimiter func()
		if cfg.rate > 0 {
			interval := time.Second / time.Duration(cfg.rate)
			if interval <= 0 {
				interval = time.Nanosecond
			}
			ticker := time.NewTicker(interval)
			limiter = ticker.C
			stopLimiter = ticker.Stop
		}
		if stopLimiter != nil {
			defer stopLimiter()
		}

		for producer := 0; producer < cfg.producers; producer++ {
			producerID := producer
			wg.Add(1)
			go func() {
				defer wg.Done()
				client, err := kgo.NewClient(baseKafkaOpts(
					brokers,
					fmt.Sprintf("opskat-kafka-sim-producer-%d", producerID),
					cfg.timeout,
					kgo.ProducerLinger(5*time.Millisecond),
				)...)
				if err != nil {
					firstErr.CompareAndSwap(nil, err)
					return
				}
				defer client.Close()

				for {
					batch := make([]*kgo.Record, 0, cfg.batchSize)
					for len(batch) < cfg.batchSize {
						n := nextSequence(produceCtx, &seq, cfg)
						if n < 0 {
							break
						}
						if limiter != nil {
							select {
							case <-produceCtx.Done():
								return
							case <-limiter:
							}
						}
						batch = append(batch, makeRecord(topic, n, producerID, cfg.payloadBytes))
					}
					if len(batch) == 0 {
						return
					}
					results := client.ProduceSync(produceCtx, batch...)
					for _, result := range results {
						if result.Err != nil {
							failed.Add(1)
							firstErr.CompareAndSwap(nil, result.Err)
							continue
						}
						produced.Add(1)
					}
				}
			}()
		}
		wg.Wait()

		stats.Attempted = seq.Load()
		stats.Produced = produced.Load()
		stats.Failed = failed.Load()
		stats.Duration = time.Since(start).Round(time.Millisecond)
		if stats.Duration > 0 {
			stats.Rate = float64(stats.Produced) / stats.Duration.Seconds()
		}
		fmt.Printf("Produced %d/%d records in %s (%.1f records/s), failed=%d\n", stats.Produced, stats.Attempted, stats.Duration, stats.Rate, stats.Failed)

		if v := firstErr.Load(); v != nil && stats.Produced == 0 {
			return v.(error)
		}
		return nil
	})
	return stats, err
}

func nextSequence(ctx context.Context, seq *atomic.Int64, cfg config) int64 {
	select {
	case <-ctx.Done():
		return -1
	default:
	}
	for {
		current := seq.Load()
		if cfg.duration == 0 && current >= cfg.messages {
			return -1
		}
		next := current + 1
		if seq.CompareAndSwap(current, next) {
			return next
		}
	}
}

func makeRecord(topic string, seq int64, producerID int, payloadBytes int) *kgo.Record {
	body := map[string]any{
		"seq":       seq,
		"producer":  producerID,
		"source":    "opskat-kafka-sim",
		"createdAt": time.Now().Format(time.RFC3339Nano),
	}
	raw, _ := json.Marshal(body)
	if payloadBytes > len(raw) {
		padding := strings.Repeat("x", payloadBytes-len(raw))
		body["padding"] = padding
		raw, _ = json.Marshal(body)
	}
	return &kgo.Record{
		Topic: topic,
		Key:   []byte(fmt.Sprintf("key-%06d", seq%1000)),
		Value: raw,
		Headers: []kgo.RecordHeader{
			{Key: "source", Value: []byte("opskat-kafka-sim")},
			{Key: "producer", Value: []byte(fmt.Sprintf("%d", producerID))},
			{Key: "seq", Value: []byte(fmt.Sprintf("%d", seq))},
		},
	}
}

func runConsumerGroupPhase(ctx context.Context, cfg config, brokers []string, group string, topic string, produced int64) (consumeStats, error) {
	stats := consumeStats{Group: group, Topic: topic}
	if cfg.consumeTarget >= 0 {
		stats.Target = cfg.consumeTarget
	} else {
		stats.Target = produced / 2
	}
	if stats.Target <= 0 || produced <= 0 {
		return stats, nil
	}
	if stats.Target > produced {
		stats.Target = produced
	}

	err := phase("consumer group consume/commit", func() error {
		client, err := kgo.NewClient(baseKafkaOpts(
			brokers,
			"opskat-kafka-sim-consumer",
			cfg.timeout,
			kgo.ConsumerGroup(group),
			kgo.ConsumeTopics(topic),
			kgo.ConsumeStartOffset(kgo.NewOffset().AtStart()),
			kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
			kgo.DisableAutoCommit(),
			kgo.BlockRebalanceOnPoll(),
		)...)
		if err != nil {
			return err
		}
		defer client.Close()

		for stats.Consumed < stats.Target {
			pollCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			fetches := client.PollRecords(pollCtx, int(minInt64(100, stats.Target-stats.Consumed)))
			cancel()
			if err := fetches.Err(); err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					break
				}
				return err
			}
			records := fetches.Records()
			if len(records) == 0 {
				break
			}
			if err := client.CommitRecords(ctx, records...); err != nil {
				return err
			}
			stats.Consumed += int64(len(records))
			client.AllowRebalance()
		}
		client.AllowRebalance()
		fmt.Printf("Consumer group %s committed %d records, leaving %d+ lag for UI checks\n", group, stats.Consumed, maxInt64(0, produced-stats.Consumed))
		return nil
	})
	return stats, err
}

func runBrowsePhase(ctx context.Context, cfg config, brokers []string, topic string) ([]sampleRecord, error) {
	var samples []sampleRecord
	err := phase("direct browse sample", func() error {
		if cfg.browseLimit == 0 {
			return nil
		}
		client, err := kgo.NewClient(baseKafkaOpts(
			brokers,
			"opskat-kafka-sim-browser",
			cfg.timeout,
			kgo.ConsumePartitions(map[string]map[int32]kgo.Offset{
				topic: {0: kgo.NewOffset().AtStart()},
			}),
			kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		)...)
		if err != nil {
			return err
		}
		defer client.Close()

		for len(samples) < cfg.browseLimit {
			pollCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			fetches := client.PollRecords(pollCtx, cfg.browseLimit-len(samples))
			cancel()
			if err := fetches.Err(); err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					break
				}
				return err
			}
			records := fetches.Records()
			if len(records) == 0 {
				break
			}
			for _, record := range records {
				samples = append(samples, sampleRecord{
					Topic:     record.Topic,
					Partition: record.Partition,
					Offset:    record.Offset,
					Key:       string(record.Key),
					Value:     truncate(string(record.Value), 160),
				})
				if len(samples) >= cfg.browseLimit {
					break
				}
			}
		}
		fmt.Printf("Browsed %d sample records from %s partition 0\n", len(samples), topic)
		return nil
	})
	return samples, err
}

func printGroupLag(ctx context.Context, admin *kadm.Client, group string) error {
	groups, err := admin.ListGroups(ctx)
	if err != nil {
		return err
	}
	found := false
	for _, g := range groups.Sorted() {
		if g.Group == group {
			found = true
			fmt.Printf("Group listed: %s state=%s protocol=%s\n", g.Group, g.State, g.ProtocolType)
		}
	}
	if !found {
		fmt.Printf("Group %s was not returned by ListGroups yet; offsets may still be visible after refresh.\n", group)
	}

	lags, err := admin.Lag(ctx, group)
	if err != nil {
		return err
	}
	lag, ok := lags[group]
	if !ok {
		fmt.Printf("No lag response for group %s\n", group)
		return nil
	}
	fmt.Printf("Total lag: %d\n", lag.Lag.Total())
	for _, part := range lag.Lag.Sorted() {
		if part.Err != nil {
			fmt.Printf("  %s[%d] lag error: %v\n", part.Topic, part.Partition, part.Err)
			continue
		}
		fmt.Printf("  %s[%d] committed=%d end=%d lag=%d\n", part.Topic, part.Partition, part.Commit.At, part.End.Offset, part.Lag)
	}
	return nil
}

func probeACLs(ctx context.Context, admin *kadm.Client) error {
	builder := kadm.NewACLs().
		ResourcePatternType(kadm.ACLPatternAny).
		AnyResource("*").
		Allow("*").
		AllowHosts("*").
		Operations(kadm.OpAny)
	results, err := admin.DescribeACLs(ctx, builder)
	if err != nil {
		return err
	}
	total := 0
	for _, result := range results {
		if result.Err != nil {
			return result.Err
		}
		total += len(result.Described)
	}
	fmt.Printf("ACL describe succeeded: %d ACL entries\n", total)
	return nil
}

func runDeleteRecordsExercise(ctx context.Context, cfg config, brokers []string, admin *kadm.Client, topic string) error {
	produceCfg := cfg
	produceCfg.messages = minInt64(50, maxInt64(1, cfg.messages/4))
	produceCfg.duration = 0
	produceCfg.producers = 1
	produceCfg.rate = 0
	if _, err := runProducePhase(ctx, produceCfg, brokers, topic); err != nil {
		return err
	}
	end, err := admin.ListEndOffsets(ctx, topic)
	if err != nil {
		return err
	}
	offsets := kadm.Offsets{}
	end.Each(func(o kadm.ListedOffset) {
		if o.Err == nil && o.Offset > 0 {
			offsets.Add(kadm.Offset{Topic: o.Topic, Partition: o.Partition, At: o.Offset})
		}
	})
	if len(offsets) == 0 {
		return nil
	}
	responses, err := admin.DeleteRecords(ctx, offsets)
	if err != nil {
		return err
	}
	for _, response := range responses.Sorted() {
		if response.Err != nil {
			return response.Err
		}
		fmt.Printf("DeleteRecords %s[%d] lowWatermark=%d\n", response.Topic, response.Partition, response.LowWatermark)
	}
	return nil
}

func exerciseSchemaRegistry(ctx context.Context, cfg config) error {
	base := strings.TrimRight(cfg.schemaRegistryURL, "/")
	subject := safeName(cfg.prefix) + "-" + safeName(cfg.runID) + "-value"
	schema := `{"type":"record","name":"OpskatKafkaSim","fields":[{"name":"id","type":"string"},{"name":"ok","type":"boolean"}]}`
	payload := map[string]any{"schemaType": "AVRO", "schema": schema}

	if _, err := httpJSON(ctx, http.MethodGet, base+"/subjects", nil); err != nil {
		return fmt.Errorf("list subjects: %w", err)
	}
	registered, err := httpJSON(ctx, http.MethodPost, base+"/subjects/"+subject+"/versions", payload)
	if err != nil {
		return fmt.Errorf("register subject %s: %w", subject, err)
	}
	fmt.Printf("Registered schema subject %s: %s\n", subject, truncate(registered, 240))
	if compat, err := httpJSON(ctx, http.MethodPost, base+"/compatibility/subjects/"+subject+"/versions/latest", payload); err != nil {
		return fmt.Errorf("check compatibility: %w", err)
	} else {
		fmt.Printf("Compatibility: %s\n", truncate(compat, 240))
	}
	if cfg.cleanup {
		if deleted, err := httpJSON(ctx, http.MethodDelete, base+"/subjects/"+subject, nil); err != nil {
			return fmt.Errorf("delete subject %s: %w", subject, err)
		} else {
			fmt.Printf("Deleted schema subject %s: %s\n", subject, truncate(deleted, 240))
		}
	}
	return nil
}

func exerciseKafkaConnect(ctx context.Context, connectURL string) error {
	base := strings.TrimRight(connectURL, "/")
	root, err := httpJSON(ctx, http.MethodGet, base+"/", nil)
	if err != nil {
		return fmt.Errorf("connect root: %w", err)
	}
	fmt.Printf("Connect root: %s\n", truncate(root, 240))
	connectors, err := httpJSON(ctx, http.MethodGet, base+"/connectors", nil)
	if err != nil {
		return fmt.Errorf("list connectors: %w", err)
	}
	fmt.Printf("Connectors: %s\n", truncate(connectors, 240))
	plugins, err := httpJSON(ctx, http.MethodGet, base+"/connector-plugins", nil)
	if err != nil {
		return fmt.Errorf("list connector plugins: %w", err)
	}
	fmt.Printf("Connector plugins: %s\n", truncate(plugins, 240))
	return nil
}

func httpJSON(ctx context.Context, method, url string, body any) (string, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return "", err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return "", err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/vnd.schemaregistry.v1+json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%s %s returned %s: %s", method, url, resp.Status, truncate(string(raw), 500))
	}
	return string(raw), nil
}

func cleanupGenerated(ctx context.Context, admin *kadm.Client, topics []string, group string) error {
	if group != "" {
		deleted, err := admin.DeleteGroup(ctx, group)
		if err != nil {
			if !strings.Contains(err.Error(), "GROUP_ID_NOT_FOUND") {
				return fmt.Errorf("delete group %s: %w", group, err)
			}
		} else if deleted.Err != nil && !errors.Is(deleted.Err, kerr.GroupIDNotFound) {
			return fmt.Errorf("delete group %s: %w", group, deleted.Err)
		} else {
			fmt.Printf("Deleted group %s\n", group)
		}
	}
	deleted, err := admin.DeleteTopics(ctx, topics...)
	if err != nil {
		return fmt.Errorf("delete topics: %w", err)
	}
	for _, response := range deleted.Sorted() {
		if response.Err != nil {
			if errors.Is(response.Err, kerr.UnknownTopicOrPartition) {
				continue
			}
			return fmt.Errorf("delete topic %s: %w", response.Topic, response.Err)
		}
		fmt.Printf("Deleted topic %s\n", response.Topic)
	}
	return nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func safeName(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "run"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-'
		if ok {
			b.WriteRune(r)
			lastDash = r == '-'
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "run"
	}
	return out
}

func ptr(s string) *string {
	return &s
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
