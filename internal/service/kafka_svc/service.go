package kafka_svc

import (
	"context"
	"fmt"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/opskat/opskat/internal/connpool"
	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/service/asset_svc"
	"github.com/opskat/opskat/internal/service/credential_resolver"
	"github.com/opskat/opskat/internal/sshpool"
)

const (
	defaultKafkaOperationTimeout = 30 * time.Second
	defaultKafkaTestTimeout      = 10 * time.Second
)

type Service struct {
	sshPool *sshpool.Pool
	clients *connpool.KafkaClientManager
}

func New(sshPool *sshpool.Pool) *Service {
	return &Service{
		sshPool: sshPool,
		clients: connpool.NewKafkaClientManager(sshPool),
	}
}

func (s *Service) Close() {
	if s == nil || s.clients == nil {
		return
	}
	s.clients.Close()
}

func (s *Service) CloseAsset(assetID int64) {
	if s == nil || s.clients == nil {
		return
	}
	s.clients.CloseAsset(assetID)
}

func (s *Service) TestConnection(ctx context.Context, cfg *asset_entity.KafkaConfig, plainPassword string, tunnelID int64) error {
	if cfg == nil {
		return fmt.Errorf("kafka 配置为空")
	}
	password := plainPassword
	if password == "" {
		var err error
		password, err = credential_resolver.Default().ResolvePasswordGeneric(ctx, cfg)
		if err != nil {
			return fmt.Errorf("解析 Kafka 凭据失败: %w", err)
		}
	}

	timeout := kafkaTimeout(cfg, defaultKafkaTestTimeout)
	opCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	testAsset := &asset_entity.Asset{Type: asset_entity.AssetTypeKafka, SSHTunnelID: tunnelID}
	client, err := connpool.DialKafka(opCtx, testAsset, cfg, password, s.sshPool)
	if err != nil {
		return err
	}
	client.Close()
	return nil
}

func (s *Service) withClient(ctx context.Context, assetID int64, fn func(context.Context, *kgo.Client, *kadm.Client, *asset_entity.Asset, *asset_entity.KafkaConfig) error) error {
	asset, cfg, password, err := resolveKafkaAsset(ctx, assetID)
	if err != nil {
		return err
	}

	opCtx, cancel := context.WithTimeout(ctx, kafkaTimeout(cfg, defaultKafkaOperationTimeout))
	defer cancel()

	client, release, err := s.clients.Acquire(opCtx, asset, cfg, password)
	if err != nil {
		return fmt.Errorf("连接 Kafka 失败: %w", err)
	}
	defer release()
	admin := kadm.NewClient(client)
	applyKafkaAdminTimeout(admin, cfg)
	return fn(opCtx, client, admin, asset, cfg)
}

func (s *Service) withOneOffClient(ctx context.Context, assetID int64, extraOpts []kgo.Opt, fn func(context.Context, *kgo.Client, *kadm.Client, *asset_entity.Asset, *asset_entity.KafkaConfig) error) error {
	asset, cfg, password, err := resolveKafkaAsset(ctx, assetID)
	if err != nil {
		return err
	}

	opCtx, cancel := context.WithTimeout(ctx, kafkaTimeout(cfg, defaultKafkaOperationTimeout))
	defer cancel()

	opts, err := connpool.BuildKafkaOptions(asset, cfg, password, s.sshPool)
	if err != nil {
		return err
	}
	opts = append(opts, extraOpts...)
	client, err := kgo.NewClient(opts...)
	if err != nil {
		return fmt.Errorf("创建 Kafka 客户端失败: %w", err)
	}
	defer client.Close()
	if err := client.Ping(opCtx); err != nil {
		return fmt.Errorf("kafka 连接失败: %w", err)
	}

	admin := kadm.NewClient(client)
	applyKafkaAdminTimeout(admin, cfg)
	return fn(opCtx, client, admin, asset, cfg)
}

func resolveKafkaAsset(ctx context.Context, assetID int64) (*asset_entity.Asset, *asset_entity.KafkaConfig, string, error) {
	asset, cfg, err := resolveKafkaAssetConfig(ctx, assetID)
	if err != nil {
		return nil, nil, "", err
	}
	password, err := credential_resolver.Default().ResolvePasswordGeneric(ctx, cfg)
	if err != nil {
		return nil, nil, "", fmt.Errorf("解析 Kafka 凭据失败: %w", err)
	}
	return asset, cfg, password, nil
}

func resolveKafkaAssetConfig(ctx context.Context, assetID int64) (*asset_entity.Asset, *asset_entity.KafkaConfig, error) {
	asset, err := asset_svc.Asset().Get(ctx, assetID)
	if err != nil {
		return nil, nil, fmt.Errorf("资产不存在: %w", err)
	}
	if !asset.IsKafka() {
		return nil, nil, fmt.Errorf("资产不是 Kafka 类型")
	}
	cfg, err := asset.GetKafkaConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("获取 Kafka 配置失败: %w", err)
	}
	return asset, cfg, nil
}

func applyKafkaAdminTimeout(admin *kadm.Client, cfg *asset_entity.KafkaConfig) {
	if admin == nil || cfg == nil {
		return
	}
	if cfg.RequestTimeoutSeconds > 0 {
		admin.SetTimeoutMillis(int32(time.Duration(cfg.RequestTimeoutSeconds) * time.Second / time.Millisecond))
	}
}

func kafkaTimeout(cfg *asset_entity.KafkaConfig, fallback time.Duration) time.Duration {
	if cfg != nil && cfg.RequestTimeoutSeconds > 0 {
		return time.Duration(cfg.RequestTimeoutSeconds) * time.Second
	}
	return fallback
}
