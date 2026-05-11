package aiagent

import (
	"github.com/opskat/opskat/internal/ai"
	"github.com/opskat/opskat/internal/service/kafka_svc"
	"github.com/opskat/opskat/internal/sshpool"
)

// Deps 是 per-ConvHandle 的依赖包：tools 共用同一组 SSH/Mongo/Kafka 缓存
// （同一会话内复用连接），policy checker 走 App 共享实例。
//
// 生命周期：
//   - SSHPool / PolicyChecker：App 共享，外部传入，Close 不动。
//   - SSHCache / MongoCache / KafkaService：per-ConvHandle，NewDeps 新建，
//     Close 时释放。
type Deps struct {
	// App-shared (do NOT close from Deps)
	SSHPool       *sshpool.Pool
	PolicyChecker *ai.CommandPolicyChecker

	// Per-ConvHandle (closed by Deps.Close)
	SSHCache     *ai.SSHClientCache
	MongoCache   *ai.MongoDBClientCache
	KafkaService *kafka_svc.Service
}

// NewDeps 新建 per-ConvHandle Deps。pool / checker 是 App 共享实例。
func NewDeps(pool *sshpool.Pool, checker *ai.CommandPolicyChecker) *Deps {
	return &Deps{
		SSHPool:       pool,
		PolicyChecker: checker,
		SSHCache:      ai.NewSSHClientCache(),
		MongoCache:    ai.NewMongoDBClientCache(),
		KafkaService:  kafka_svc.New(pool),
	}
}

// Close 释放 per-ConvHandle 资源。幂等。
func (d *Deps) Close() error {
	if d == nil {
		return nil
	}
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
