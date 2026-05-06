package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/opskat/opskat/internal/assettype"
	"github.com/opskat/opskat/internal/k8s"
	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/service/asset_svc"
	"github.com/opskat/opskat/internal/sshpool"

	"github.com/cago-frame/cago/pkg/logger"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"go.uber.org/zap"
)

// k8sCallContext 加载 K8S 资产时返回的所有调用上下文。
// kubeconfig 是已解密的 YAML 文本，opts 已带上 SSH 隧道 dial 函数（若配置）。
type k8sCallContext struct {
	asset      *asset_entity.Asset
	cfg        *asset_entity.K8sConfig
	kubeconfig string
	opts       []k8s.ClientOption
}

// loadK8sCall 校验资产、解析 K8S 配置、解密 kubeconfig、构造 ClientOption。
// 所有需要调用 internal/k8s 的 Wails 绑定都从这里获取上下文。
func (a *App) loadK8sCall(ctx context.Context, assetID int64) (*k8sCallContext, error) {
	asset, err := asset_svc.Asset().Get(ctx, assetID)
	if err != nil {
		return nil, fmt.Errorf("get asset: %w", err)
	}
	if !asset.IsK8s() {
		return nil, fmt.Errorf("asset %d is not a K8S cluster", assetID)
	}
	cfg, err := asset.GetK8sConfig()
	if err != nil {
		return nil, fmt.Errorf("get K8S config: %w", err)
	}
	if cfg.Kubeconfig == "" {
		return nil, fmt.Errorf("no kubeconfig configured for this K8S asset")
	}
	h, ok := assettype.Get(asset_entity.AssetTypeK8s)
	if !ok {
		return nil, fmt.Errorf("k8s asset type handler not registered")
	}
	kubeconfig, err := h.ResolvePassword(ctx, asset)
	if err != nil {
		return nil, fmt.Errorf("decrypt kubeconfig: %w", err)
	}
	return &k8sCallContext{
		asset:      asset,
		cfg:        cfg,
		kubeconfig: kubeconfig,
		opts:       a.k8sClientOptions(asset, cfg),
	}, nil
}

// runK8sCall 是 9 个 GetK8sNamespace*/GetK8sClusterInfo/GetK8sPodDetail 共用的模板：
// 加载上下文 → 调用 fn → JSON 序列化。
func (a *App) runK8sCall(assetID int64, label string, fn func(ctx context.Context, c *k8sCallContext) (any, error)) (string, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()

	c, err := a.loadK8sCall(ctx, assetID)
	if err != nil {
		return "", err
	}
	result, err := fn(ctx, c)
	if err != nil {
		return "", fmt.Errorf("%s: %w", label, err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("marshal %s: %w", label, err)
	}
	return string(data), nil
}

func (a *App) GetK8sClusterInfo(assetID int64) (string, error) {
	return a.runK8sCall(assetID, "get K8S cluster info", func(ctx context.Context, c *k8sCallContext) (any, error) {
		return k8s.GetClusterInfo(ctx, c.kubeconfig, c.opts...)
	})
}

func (a *App) GetK8sNamespaceResources(assetID int64, namespace string) (string, error) {
	return a.runK8sCall(assetID, "get K8S namespace resources", func(ctx context.Context, c *k8sCallContext) (any, error) {
		return k8s.GetNamespaceResources(ctx, c.kubeconfig, namespace, c.opts...)
	})
}

func (a *App) GetK8sNamespacePods(assetID int64, namespace string) (string, error) {
	return a.runK8sCall(assetID, "get K8S namespace pods", func(ctx context.Context, c *k8sCallContext) (any, error) {
		return k8s.GetNamespacePods(ctx, c.kubeconfig, namespace, c.opts...)
	})
}

func (a *App) GetK8sNamespaceDeployments(assetID int64, namespace string) (string, error) {
	return a.runK8sCall(assetID, "get K8S namespace deployments", func(ctx context.Context, c *k8sCallContext) (any, error) {
		return k8s.GetNamespaceDeployments(ctx, c.kubeconfig, namespace, c.opts...)
	})
}

func (a *App) GetK8sNamespaceServices(assetID int64, namespace string) (string, error) {
	return a.runK8sCall(assetID, "get K8S namespace services", func(ctx context.Context, c *k8sCallContext) (any, error) {
		return k8s.GetNamespaceServices(ctx, c.kubeconfig, namespace, c.opts...)
	})
}

func (a *App) GetK8sNamespaceConfigMaps(assetID int64, namespace string) (string, error) {
	return a.runK8sCall(assetID, "get K8S namespace configmaps", func(ctx context.Context, c *k8sCallContext) (any, error) {
		return k8s.GetNamespaceConfigMaps(ctx, c.kubeconfig, namespace, c.opts...)
	})
}

func (a *App) GetK8sNamespaceSecrets(assetID int64, namespace string) (string, error) {
	return a.runK8sCall(assetID, "get K8S namespace secrets", func(ctx context.Context, c *k8sCallContext) (any, error) {
		return k8s.GetNamespaceSecrets(ctx, c.kubeconfig, namespace, c.opts...)
	})
}

func (a *App) GetK8sPodDetail(assetID int64, namespace, podName string) (string, error) {
	return a.runK8sCall(assetID, "get K8S pod detail", func(ctx context.Context, c *k8sCallContext) (any, error) {
		return k8s.GetPodDetail(ctx, c.kubeconfig, namespace, podName, c.opts...)
	})
}

func (a *App) StartK8sPodLogs(assetID int64, namespace, podName, container string, tailLines int64) (string, error) {
	loadCtx, loadCancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer loadCancel()
	c, err := a.loadK8sCall(loadCtx, assetID)
	if err != nil {
		return "", err
	}

	streamID := fmt.Sprintf("k8s-log-%d", atomic.AddInt64(&a.k8sLogStreamCounter, 1))
	ctx, cancel := context.WithCancel(a.ctx)
	a.k8sLogStreams.Store(streamID, cancel)

	reader, err := k8s.StreamPodLogs(ctx, c.kubeconfig, namespace, podName, container, tailLines, c.opts...)
	if err != nil {
		cancel()
		a.k8sLogStreams.Delete(streamID)
		return "", fmt.Errorf("open pod log stream: %w", err)
	}

	go func() {
		defer func() {
			if closeErr := reader.Close(); closeErr != nil {
				logger.Default().Warn("close k8s log reader", zap.Error(closeErr))
			}
		}()
		defer cancel()
		defer a.k8sLogStreams.Delete(streamID)

		buf := make([]byte, 4096)
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				data := base64.StdEncoding.EncodeToString(buf[:n])
				wailsRuntime.EventsEmit(a.ctx, "k8s:log:"+streamID, data)
			}
			if err != nil {
				if err != io.EOF {
					wailsRuntime.EventsEmit(a.ctx, "k8s:logerr:"+streamID, err.Error())
				}
				wailsRuntime.EventsEmit(a.ctx, "k8s:logend:"+streamID, streamID)
				return
			}
		}
	}()

	return streamID, nil
}

func (a *App) StopK8sPodLogs(streamID string) {
	if cancel, ok := a.k8sLogStreams.LoadAndDelete(streamID); ok {
		cancel.(context.CancelFunc)()
	}
}

func (a *App) k8sClientOptions(asset *asset_entity.Asset, cfg *asset_entity.K8sConfig) []k8s.ClientOption {
	opts := make([]k8s.ClientOption, 0, 2)
	if cfg.Context != "" {
		opts = append(opts, k8s.WithContext(cfg.Context))
	}

	tunnelID := asset.SSHTunnelID
	if tunnelID == 0 || a.sshPool == nil {
		return opts
	}

	opts = append(opts, k8s.WithDial(func(ctx context.Context, network, address string) (net.Conn, error) {
		client, err := a.sshPool.Get(ctx, tunnelID)
		if err != nil {
			return nil, fmt.Errorf("get SSH tunnel: %w", err)
		}
		conn, err := client.Dial(network, address)
		if err != nil {
			a.sshPool.Release(tunnelID)
			return nil, fmt.Errorf("dial K8S API through SSH tunnel: %w", err)
		}
		return &k8sTunnelConn{Conn: conn, pool: a.sshPool, assetID: tunnelID}, nil
	}))
	return opts
}

type k8sTunnelConn struct {
	net.Conn
	pool    *sshpool.Pool
	assetID int64
	once    sync.Once
}

func (c *k8sTunnelConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() { c.pool.Release(c.assetID) })
	return err
}
