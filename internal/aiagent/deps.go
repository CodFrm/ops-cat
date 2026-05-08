package aiagent

import (
	"github.com/opskat/opskat/internal/ai"
	"github.com/opskat/opskat/internal/service/kafka_svc"
	"github.com/opskat/opskat/internal/sshpool"
)

// Deps is the per-Conversation (per-coding.System) dependency bag.
// Tools, hooks, and observers borrow from it.
//
// Lifetime invariants:
//   - sshPool is App-shared (one process-wide instance) — provided by the App.
//   - sshCache, mongoCache, kafkaSvc are per-Conversation; constructed per Deps,
//     closed via Deps.Close() when the System closes.
type Deps struct {
	// App-shared (do NOT close from Deps)
	SSHPool *sshpool.Pool

	// Per-Conversation (closed by Deps.Close)
	SSHCache     *ai.SSHClientCache
	MongoCache   *ai.MongoDBClientCache
	KafkaService *kafka_svc.Service

	// Reused references
	PolicyChecker *ai.CommandPolicyChecker
}

// NewDeps constructs a fresh per-Conversation Deps bag.
// pool MUST be the App-shared SSH pool; checker MUST be the App-shared checker.
func NewDeps(pool *sshpool.Pool, checker *ai.CommandPolicyChecker) *Deps {
	return &Deps{
		SSHPool:       pool,
		SSHCache:      ai.NewSSHClientCache(),
		MongoCache:    ai.NewMongoDBClientCache(),
		KafkaService:  kafka_svc.New(pool),
		PolicyChecker: checker,
	}
}

// Close releases per-Conversation resources. Idempotent.
func (d *Deps) Close() error {
	if d.KafkaService != nil {
		d.KafkaService.Close()
		d.KafkaService = nil
	}
	if d.MongoCache != nil {
		_ = d.MongoCache.Close()
		d.MongoCache = nil
	}
	if d.SSHCache != nil {
		_ = d.SSHCache.Close()
		d.SSHCache = nil
	}
	return nil
}
