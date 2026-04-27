package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/opskat/opskat/internal/k8s"
	"github.com/opskat/opskat/internal/service/asset_svc"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) GetK8sClusterInfo(assetID int64) (string, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()

	asset, err := asset_svc.Asset().Get(ctx, assetID)
	if err != nil {
		return "", fmt.Errorf("get asset: %w", err)
	}
	if !asset.IsK8s() {
		return "", fmt.Errorf("asset %d is not a K8S cluster", assetID)
	}

	cfg, err := asset.GetK8sConfig()
	if err != nil {
		return "", fmt.Errorf("get K8S config: %w", err)
	}

	token := cfg.Token
	if token == "" && cfg.Kubeconfig == "" && cfg.ApiServer == "" {
		return "", fmt.Errorf("no kubeconfig or api_server configured for this K8S asset")
	}

	info, err := k8s.GetClusterInfo(ctx, cfg.Kubeconfig, cfg.ApiServer, token)
	if err != nil {
		return "", fmt.Errorf("get K8S cluster info: %w", err)
	}

	result, err := json.Marshal(info)
	if err != nil {
		return "", fmt.Errorf("marshal cluster info: %w", err)
	}
	return string(result), nil
}

func (a *App) GetK8sNamespaceResources(assetID int64, namespace string) (string, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()

	asset, err := asset_svc.Asset().Get(ctx, assetID)
	if err != nil {
		return "", fmt.Errorf("get asset: %w", err)
	}
	if !asset.IsK8s() {
		return "", fmt.Errorf("asset %d is not a K8S cluster", assetID)
	}

	cfg, err := asset.GetK8sConfig()
	if err != nil {
		return "", fmt.Errorf("get K8S config: %w", err)
	}

	token := cfg.Token
	if token == "" && cfg.Kubeconfig == "" && cfg.ApiServer == "" {
		return "", fmt.Errorf("no kubeconfig or api_server configured for this K8S asset")
	}

	res, err := k8s.GetNamespaceResources(ctx, cfg.Kubeconfig, cfg.ApiServer, token, namespace)
	if err != nil {
		return "", fmt.Errorf("get K8S namespace resources: %w", err)
	}

	result, err := json.Marshal(res)
	if err != nil {
		return "", fmt.Errorf("marshal namespace resources: %w", err)
	}
	return string(result), nil
}

func (a *App) GetK8sNamespacePods(assetID int64, namespace string) (string, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()

	asset, err := asset_svc.Asset().Get(ctx, assetID)
	if err != nil {
		return "", fmt.Errorf("get asset: %w", err)
	}
	if !asset.IsK8s() {
		return "", fmt.Errorf("asset %d is not a K8S cluster", assetID)
	}

	cfg, err := asset.GetK8sConfig()
	if err != nil {
		return "", fmt.Errorf("get K8S config: %w", err)
	}

	token := cfg.Token
	if token == "" && cfg.Kubeconfig == "" && cfg.ApiServer == "" {
		return "", fmt.Errorf("no kubeconfig or api_server configured for this K8S asset")
	}

	pods, err := k8s.GetNamespacePods(ctx, cfg.Kubeconfig, cfg.ApiServer, token, namespace)
	if err != nil {
		return "", fmt.Errorf("get K8S namespace pods: %w", err)
	}

	result, err := json.Marshal(pods)
	if err != nil {
		return "", fmt.Errorf("marshal namespace pods: %w", err)
	}
	return string(result), nil
}

func (a *App) GetK8sPodDetail(assetID int64, namespace, podName string) (string, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()

	asset, err := asset_svc.Asset().Get(ctx, assetID)
	if err != nil {
		return "", fmt.Errorf("get asset: %w", err)
	}
	if !asset.IsK8s() {
		return "", fmt.Errorf("asset %d is not a K8S cluster", assetID)
	}

	cfg, err := asset.GetK8sConfig()
	if err != nil {
		return "", fmt.Errorf("get K8S config: %w", err)
	}

	token := cfg.Token
	if token == "" && cfg.Kubeconfig == "" && cfg.ApiServer == "" {
		return "", fmt.Errorf("no kubeconfig or api_server configured for this K8S asset")
	}

	detail, err := k8s.GetPodDetail(ctx, cfg.Kubeconfig, cfg.ApiServer, token, namespace, podName)
	if err != nil {
		return "", fmt.Errorf("get K8S pod detail: %w", err)
	}

	result, err := json.Marshal(detail)
	if err != nil {
		return "", fmt.Errorf("marshal pod detail: %w", err)
	}
	return string(result), nil
}

func (a *App) StartK8sPodLogs(assetID int64, namespace, podName, container string, tailLines int64) (string, error) {
	asset, err := asset_svc.Asset().Get(a.ctx, assetID)
	if err != nil {
		return "", fmt.Errorf("get asset: %w", err)
	}
	if !asset.IsK8s() {
		return "", fmt.Errorf("asset %d is not a K8S cluster", assetID)
	}

	cfg, err := asset.GetK8sConfig()
	if err != nil {
		return "", fmt.Errorf("get K8S config: %w", err)
	}

	token := cfg.Token
	if token == "" && cfg.Kubeconfig == "" && cfg.ApiServer == "" {
		return "", fmt.Errorf("no kubeconfig or api_server configured for this K8S asset")
	}

	streamID := fmt.Sprintf("k8s-log-%d", atomic.AddInt64(&a.k8sLogStreamCounter, 1))

	ctx, cancel := context.WithCancel(a.ctx)
	a.k8sLogStreams.Store(streamID, cancel)

	reader, err := k8s.StreamPodLogs(ctx, cfg.Kubeconfig, cfg.ApiServer, token, namespace, podName, container, tailLines)
	if err != nil {
		cancel()
		a.k8sLogStreams.Delete(streamID)
		return "", fmt.Errorf("open pod log stream: %w", err)
	}

	go func() {
		defer reader.Close()
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
