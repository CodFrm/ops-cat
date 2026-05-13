package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/pkg/dirsync"
	"github.com/opskat/opskat/internal/service/asset_svc"
	"github.com/opskat/opskat/internal/service/credential_svc"
	"github.com/opskat/opskat/internal/service/ssh_svc"
	"github.com/opskat/opskat/internal/service/testreg"
	"github.com/opskat/opskat/internal/sshpool"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// SSHConnectRequest 前端 SSH 连接请求
type SSHConnectRequest struct {
	AssetID  int64  `json:"assetId"`
	Password string `json:"password"`
	Key      string `json:"key"`
	Cols     int    `json:"cols"`
	Rows     int    `json:"rows"`
}

// ConnectSSH 连接 SSH 服务器，返回会话 ID
func (a *App) ConnectSSH(req SSHConnectRequest) (string, error) {
	// 获取资产信息
	asset, err := asset_svc.Asset().Get(a.langCtx(), req.AssetID)
	if err != nil {
		return "", fmt.Errorf("资产不存在: %w", err)
	}
	if !asset.IsSSH() {
		return "", fmt.Errorf("资产不是SSH类型")
	}
	sshCfg, err := asset.GetSSHConfig()
	if err != nil {
		return "", err
	}

	// 解析存储的凭证（包含 passphrase）
	storedPassword, storedKey, storedPassphrase := a.resolveSSHCredentialsFull(sshCfg)
	password := req.Password
	key := req.Key
	if password == "" {
		password = storedPassword
	}
	if key == "" {
		key = storedKey
	}

	connectCfg := ssh_svc.ConnectConfig{
		Host:              sshCfg.Host,
		Port:              sshCfg.Port,
		Username:          sshCfg.Username,
		AuthType:          sshCfg.AuthType,
		Password:          password,
		Key:               key,
		KeyPassphrase:     storedPassphrase,
		PrivateKeys:       sshCfg.PrivateKeys,
		AssetID:           req.AssetID,
		Cols:              req.Cols,
		Rows:              req.Rows,
		Proxy:             a.decryptProxyPassword(sshCfg.Proxy),
		HostKeyVerifyFunc: ssh_svc.AutoTrustFirstRejectChangeVerifyFunc(),
		OnData: func(sid string, data []byte) {
			wailsRuntime.EventsEmit(a.ctx, "ssh:data:"+sid, base64.StdEncoding.EncodeToString(data))
		},
		OnClosed: func(sid string) {
			wailsRuntime.EventsEmit(a.ctx, "ssh:closed:"+sid, nil)
		},
		OnSync: func(sid string, state ssh_svc.DirectorySyncState) {
			wailsRuntime.EventsEmit(a.ctx, "ssh:sync:"+sid, state)
		},
	}

	// 解析跳板机链（递归，最大深度 5）
	jumpHostID := asset.SSHTunnelID
	if jumpHostID == 0 {
		jumpHostID = sshCfg.JumpHostID // backward compat
	}
	if jumpHostID > 0 {
		jumpHosts, err := a.resolveJumpHosts(jumpHostID, 5)
		if err != nil {
			return "", fmt.Errorf("解析跳板机失败: %w", err)
		}
		connectCfg.JumpHosts = jumpHosts
	}

	sessionID, err := a.sshManager.Connect(connectCfg)
	if err != nil {
		if isSSHAuthError(err) {
			return "", fmt.Errorf("AUTH_FAILED:%s", err.Error())
		}
		return "", err
	}
	return sessionID, nil
}

// isSSHAuthError 判断是否为 SSH 认证失败错误
func isSSHAuthError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "unable to authenticate") ||
		strings.Contains(msg, "no supported methods remain")
}

// ConnectSSHAsync 异步连接 SSH 服务器，立即返回 connectionId，通过事件推送进度
func (a *App) ConnectSSHAsync(req SSHConnectRequest) (string, error) {
	// 前置校验（同步）
	asset, err := asset_svc.Asset().Get(a.langCtx(), req.AssetID)
	if err != nil {
		return "", fmt.Errorf("资产不存在: %w", err)
	}
	if !asset.IsSSH() {
		return "", fmt.Errorf("资产不是SSH类型")
	}

	// 生成 connectionId
	a.mu.Lock()
	a.connCounter++
	connectionId := fmt.Sprintf("conn-%d", a.connCounter)
	a.mu.Unlock()

	// 创建可取消的 context
	connCtx, cancel := context.WithCancel(a.ctx)
	a.pendingConnections.Store(connectionId, cancel)

	eventName := "ssh:connect:" + connectionId

	emitEvent := func(event SSHConnectEvent) {
		wailsRuntime.EventsEmit(a.ctx, eventName, event)
	}

	go func() {
		defer func() {
			a.pendingConnections.Delete(connectionId)
		}()

		emitEvent(SSHConnectEvent{Type: "progress", Step: "resolve", Message: "正在解析凭证..."})

		sshCfg, err := asset.GetSSHConfig()
		if err != nil {
			emitEvent(SSHConnectEvent{Type: "error", Error: err.Error()})
			return
		}

		// 检查是否已取消
		if connCtx.Err() != nil {
			return
		}

		// 解析凭证
		storedPassword, storedKey, storedPassphrase := a.resolveSSHCredentialsFull(sshCfg)
		password := req.Password
		key := req.Key
		if password == "" {
			password = storedPassword
		}
		if key == "" {
			key = storedKey
		}

		connectCfg := ssh_svc.ConnectConfig{
			Host:          sshCfg.Host,
			Port:          sshCfg.Port,
			Username:      sshCfg.Username,
			AuthType:      sshCfg.AuthType,
			Password:      password,
			Key:           key,
			KeyPassphrase: storedPassphrase,
			PrivateKeys:   sshCfg.PrivateKeys,
			AssetID:       req.AssetID,
			Cols:          req.Cols,
			Rows:          req.Rows,
			Proxy:         a.decryptProxyPassword(sshCfg.Proxy),
			OnData: func(sid string, data []byte) {
				wailsRuntime.EventsEmit(a.ctx, "ssh:data:"+sid, base64.StdEncoding.EncodeToString(data))
			},
			OnClosed: func(sid string) {
				wailsRuntime.EventsEmit(a.ctx, "ssh:closed:"+sid, nil)
			},
			OnSync: func(sid string, state ssh_svc.DirectorySyncState) {
				wailsRuntime.EventsEmit(a.ctx, "ssh:sync:"+sid, state)
			},
			OnProgress: func(step, message string) {
				emitEvent(SSHConnectEvent{Type: "progress", Step: step, Message: message})
			},
			OnAuthChallenge: func(prompts []string, echo []bool) ([]string, error) {
				challengeID := fmt.Sprintf("auth_%s_%d", connectionId, time.Now().UnixNano())
				emitEvent(SSHConnectEvent{
					Type:        "auth_challenge",
					ChallengeID: challengeID,
					Prompts:     prompts,
					Echo:        echo,
				})

				ch := make(chan []string, 1)
				a.pendingAuthResponses.Store(challengeID, ch)
				defer a.pendingAuthResponses.Delete(challengeID)

				select {
				case answers := <-ch:
					return answers, nil
				case <-connCtx.Done():
					return nil, fmt.Errorf("连接已取消")
				}
			},
			HostKeyVerifyFunc: func(event ssh_svc.HostKeyEvent) ssh_svc.HostKeyAction {
				verifyID := fmt.Sprintf("hk_%s_%d", connectionId, time.Now().UnixNano())
				emitEvent(SSHConnectEvent{
					Type:            "host_key_verify",
					HostKeyVerifyID: verifyID,
					HostKeyEvent:    &event,
				})

				ch := make(chan ssh_svc.HostKeyAction, 1)
				a.pendingHostKeyResponses.Store(verifyID, ch)
				defer a.pendingHostKeyResponses.Delete(verifyID)

				select {
				case action := <-ch:
					return action
				case <-connCtx.Done():
					return ssh_svc.HostKeyReject
				case <-a.shutdownCh:
					return ssh_svc.HostKeyReject
				}
			},
		}

		// 解析跳板机链
		jumpHostID := asset.SSHTunnelID
		if jumpHostID == 0 {
			jumpHostID = sshCfg.JumpHostID // backward compat
		}
		if jumpHostID > 0 {
			emitEvent(SSHConnectEvent{Type: "progress", Step: "resolve", Message: "正在解析跳板机链..."})
			jumpHosts, err := a.resolveJumpHosts(jumpHostID, 5)
			if err != nil {
				emitEvent(SSHConnectEvent{Type: "error", Error: fmt.Sprintf("解析跳板机失败: %s", err.Error())})
				return
			}
			connectCfg.JumpHosts = jumpHosts
		}

		// 检查是否已取消
		if connCtx.Err() != nil {
			return
		}

		sessionID, err := a.sshManager.Connect(connectCfg)
		if err != nil {
			emitEvent(SSHConnectEvent{
				Type:       "error",
				Error:      err.Error(),
				AuthFailed: isSSHAuthError(err),
			})
			return
		}

		emitEvent(SSHConnectEvent{Type: "connected", SessionID: sessionID})
	}()

	return connectionId, nil
}

// RespondAuthChallenge 前端响应 keyboard-interactive 认证质询
func (a *App) RespondAuthChallenge(challengeID string, answers []string) {
	if v, ok := a.pendingAuthResponses.Load(challengeID); ok {
		ch := v.(chan []string)
		select {
		case ch <- answers:
		default:
		}
	}
}

// RespondHostKeyVerify 前端响应主机密钥校验
// action: 0=AcceptAndSave, 1=AcceptOnce, 2=Reject
func (a *App) RespondHostKeyVerify(verifyID string, action int) {
	if v, ok := a.pendingHostKeyResponses.Load(verifyID); ok {
		ch := v.(chan ssh_svc.HostKeyAction)
		select {
		case ch <- ssh_svc.HostKeyAction(action):
		default:
		}
	}
}

// CancelSSHConnect 取消异步 SSH 连接
func (a *App) CancelSSHConnect(connectionId string) {
	if v, ok := a.pendingConnections.Load(connectionId); ok {
		cancel := v.(context.CancelFunc)
		cancel()
	}
}

// UpdateAssetPassword 更新资产的保存密码
func (a *App) UpdateAssetPassword(assetID int64, password string) error {
	asset, err := asset_svc.Asset().Get(a.langCtx(), assetID)
	if err != nil {
		return err
	}
	sshCfg, err := asset.GetSSHConfig()
	if err != nil {
		return err
	}
	encrypted, err := credential_svc.Default().Encrypt(password)
	if err != nil {
		return err
	}
	sshCfg.Password = encrypted
	if err := asset.SetSSHConfig(sshCfg); err != nil {
		return err
	}
	return asset_svc.Asset().Update(a.langCtx(), asset)
}

// TestSSHConnection 测试 SSH 连接（不创建终端会话）
// testID: 前端生成的本次测试唯一标识，用于配合 CancelTest 中断
// configJSON: SSHConfig JSON，plainPassword: 明文密码（前端表单直接传入）
func (a *App) TestSSHConnection(testID string, configJSON string, plainPassword string) error {
	var sshCfg asset_entity.SSHConfig
	if err := json.Unmarshal([]byte(configJSON), &sshCfg); err != nil {
		return fmt.Errorf("配置解析失败: %w", err)
	}

	parent, parentCancel := context.WithTimeout(a.langCtx(), 10*time.Second)
	defer parentCancel()
	ctx, release := testreg.Begin(parent, testID)
	defer release()

	storedPassword, key, passphrase := a.resolveSSHCredentialsFull(&sshCfg)
	password := plainPassword
	if password == "" {
		password = storedPassword
	}

	// 处理 passphrase：
	// 1. 如果前端传入明文 passphrase（用户新输入），直接使用
	// 2. 如果前端传入加密的 passphrase（存储的），尝试解密
	// 3. 否则使用 resolver 解析出的 passphrase（托管密钥）
	var keyPassphrase string
	if sshCfg.PrivateKeyPassphrase != "" {
		// 尝试解密，如果解密失败则认为是明文（用户刚输入的）
		decrypted, err := credential_svc.Default().Decrypt(sshCfg.PrivateKeyPassphrase)
		if err == nil {
			keyPassphrase = decrypted
		} else {
			// 解密失败，可能是明文 passphrase（用户刚输入）
			keyPassphrase = sshCfg.PrivateKeyPassphrase
		}
	} else {
		keyPassphrase = passphrase
	}

	connectCfg := ssh_svc.ConnectConfig{
		Host:              sshCfg.Host,
		Port:              sshCfg.Port,
		Username:          sshCfg.Username,
		AuthType:          sshCfg.AuthType,
		Password:          password,
		Key:               key,
		KeyPassphrase:     keyPassphrase,
		PrivateKeys:       sshCfg.PrivateKeys,
		Proxy:             sshCfg.Proxy,
		HostKeyVerifyFunc: ssh_svc.AutoTrustFirstRejectChangeVerifyFunc(),
	}

	// 解析跳板机
	if sshCfg.JumpHostID > 0 {
		jumpHosts, err := a.resolveJumpHosts(sshCfg.JumpHostID, 5)
		if err != nil {
			return fmt.Errorf("解析跳板机失败: %w", err)
		}
		connectCfg.JumpHosts = jumpHosts
	}

	return a.sshManager.TestConnection(ctx, connectCfg)
}

// WriteSSH 向 SSH 终端写入数据（base64 编码）
func (a *App) WriteSSH(sessionID string, dataB64 string) error {
	sess, ok := a.sshManager.GetSession(sessionID)
	if !ok {
		return fmt.Errorf("会话不存在: %s", sessionID)
	}
	data, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		return fmt.Errorf("解码数据失败: %w", err)
	}
	return sess.Write(data)
}

// ResizeSSH 调整终端尺寸
func (a *App) ResizeSSH(sessionID string, cols int, rows int) error {
	sess, ok := a.sshManager.GetSession(sessionID)
	if !ok {
		return fmt.Errorf("会话不存在: %s", sessionID)
	}
	return sess.Resize(cols, rows)
}

// GetSSHSyncState 返回会话当前的目录同步状态。
func (a *App) GetSSHSyncState(sessionID string) (ssh_svc.DirectorySyncState, error) {
	return a.sshManager.GetSessionSyncState(sessionID)
}

// ChangeSSHDirectory 请求当前终端切换到指定目录。
// 若目录同步尚未启用，会自动注入钩子（一次性，会话内后续切换不再注入）。
func (a *App) ChangeSSHDirectory(sessionID, targetPath string) error {
	sess, ok := a.sshManager.GetSession(sessionID)
	if !ok {
		return dirsync.Error(dirsync.CodeSessionNotFound)
	}

	state := sess.GetSyncState()
	if !state.Supported {
		if err := sess.EnableSync(); err != nil {
			return err
		}
		state = sess.GetSyncState()
	}
	if !state.CwdKnown {
		// EnableSync resolved init:pid; the first prompt nonce may not have
		// arrived yet. Surface a typed retry error so the frontend can debounce.
		return dirsync.Error(dirsync.CodeCwdUnknown)
	}

	resolvedPath := targetPath
	if !strings.HasPrefix(resolvedPath, "/") {
		resolvedPath = path.Join(state.Cwd, resolvedPath)
	}
	resolvedPath = path.Clean(resolvedPath)

	expectedPath, err := a.sftpService.ResolveDirectory(sessionID, resolvedPath)
	if err != nil {
		return err
	}

	return sess.ChangeDirectoryTo(resolvedPath, expectedPath)
}

// EnableSSHSync 显式启用目录同步（用于面板"跟随终端"按钮的首次点击）。
// 注入钩子需要终端处于 shell 提示符状态；若用户处于 vim/less/tmux 等前台
// 程序中，会在 ~3s 后超时返回 DIRSYNC_TIMEOUT，前端应提示用户先退出该程序。
func (a *App) EnableSSHSync(sessionID string) error {
	sess, ok := a.sshManager.GetSession(sessionID)
	if !ok {
		return dirsync.Error(dirsync.CodeSessionNotFound)
	}
	return sess.EnableSync()
}

// SplitSSH 在已有会话的连接上创建新会话（分割窗格复用连接）
func (a *App) SplitSSH(existingSessionID string, cols, rows int) (string, error) {
	return a.sshManager.NewSessionFrom(existingSessionID, cols, rows,
		func(sid string, data []byte) {
			wailsRuntime.EventsEmit(a.ctx, "ssh:data:"+sid, base64.StdEncoding.EncodeToString(data))
		},
		func(sid string) {
			wailsRuntime.EventsEmit(a.ctx, "ssh:closed:"+sid, nil)
		},
		func(sid string, state ssh_svc.DirectorySyncState) {
			wailsRuntime.EventsEmit(a.ctx, "ssh:sync:"+sid, state)
		},
	)
}

// DisconnectSSH 断开 SSH 连接
func (a *App) DisconnectSSH(sessionID string) {
	a.sshManager.Disconnect(sessionID)
}

// GetSSHPoolConnections 返回连接池中的活跃连接信息（供前端展示）
func (a *App) GetSSHPoolConnections() []sshpool.PoolEntryInfo {
	if a.sshPool == nil {
		return nil
	}
	return a.sshPool.List()
}
