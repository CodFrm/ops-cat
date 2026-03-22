package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"ops-cat/internal/ai"
	"ops-cat/internal/model/entity/asset_entity"
	"ops-cat/internal/model/entity/conversation_entity"
	"ops-cat/internal/model/entity/group_entity"
	"ops-cat/internal/model/entity/ssh_key_entity"
	"ops-cat/internal/repository/asset_repo"
	"ops-cat/internal/repository/group_repo"
	"ops-cat/internal/service/asset_svc"
	"ops-cat/internal/service/backup_svc"
	"ops-cat/internal/service/conversation_svc"
	"ops-cat/internal/service/credential_svc"
	"ops-cat/internal/service/import_svc"
	"ops-cat/internal/service/sftp_svc"
	"ops-cat/internal/service/ssh_key_svc"
	"ops-cat/internal/service/ssh_svc"

	"github.com/cago-frame/cago/pkg/i18n"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/crypto/ssh"
)

// App Wails应用主结构体，替代controller层
type App struct {
	ctx                   context.Context
	lang                  string
	sshManager            *ssh_svc.Manager
	sftpService           *sftp_svc.Service
	aiAgent               *ai.Agent
	aiProvider            ai.Provider // 保留 provider 引用，用于权限回调注入
	mcpServer             *ai.MCPServer
	githubAuthCancel      context.CancelFunc
	permissionChan        chan ai.PermissionResponse // 前端权限响应 channel
	currentConversationID int64                      // 当前活跃会话ID
	aiProviderType        string                     // 当前 provider 类型
	aiModel               string                     // 当前模型
}

// NewApp 创建App实例
func NewApp() *App {
	mgr := ssh_svc.NewManager()
	return &App{
		lang:           "zh-cn",
		sshManager:     mgr,
		sftpService:    sftp_svc.NewService(mgr),
		permissionChan: make(chan ai.PermissionResponse, 1),
	}
}

// SetAIProvider 设置 AI provider 并创建 agent
func (a *App) SetAIProvider(providerType, apiBase, apiKey, model string) {
	// 停止旧的 MCP Server
	a.stopMCPServer()
	a.aiProviderType = providerType
	a.aiModel = model

	var provider ai.Provider
	switch providerType {
	case "openai":
		provider = ai.NewOpenAIProvider("OpenAI Compatible", apiBase, apiKey, model)
	case "local_cli":
		// apiBase 作为 CLI 路径，model 作为 CLI 类型
		cliProvider := ai.NewLocalCLIProvider("Local CLI", apiBase, model)
		// 注入权限确认回调：转发到前端，等待用户响应
		cliProvider.OnPermissionRequest = func(req ai.PermissionRequest) ai.PermissionResponse {
			wailsRuntime.EventsEmit(a.ctx, "ai:permission", req)
			return <-a.permissionChan
		}
		// 启动 MCP Server（使用应用数据目录作为默认配置目录）
		mcpSrv := ai.NewMCPServer()
		if err := mcpSrv.Start(a.ctx, appDataDir()); err != nil {
			fmt.Printf("MCP Server 启动失败: %v\n", err)
		} else {
			a.mcpServer = mcpSrv
			// 如果有当前会话的工作目录，写入 MCP 配置
			if a.currentConversationID > 0 {
				conv, err := conversation_svc.Conversation().Get(a.langCtx(), a.currentConversationID)
				if err == nil && conv.WorkDir != "" {
					_ = mcpSrv.WriteConfigToDir(conv.WorkDir)
					cliProvider.SetMCPWorkDir(conv.WorkDir)
				} else {
					cliProvider.SetMCPWorkDir(mcpSrv.ConfigDir())
				}
			} else {
				cliProvider.SetMCPWorkDir(mcpSrv.ConfigDir())
			}
		}
		a.aiProvider = cliProvider
		a.aiAgent = ai.NewAgent(cliProvider, nil)
		return
	default:
		provider = ai.NewOpenAIProvider(providerType, apiBase, apiKey, model)
	}
	a.aiAgent = ai.NewAgent(provider, ai.NewDefaultToolExecutor())
}

// stopMCPServer 停止 MCP Server 并清理
func (a *App) stopMCPServer() {
	if a.mcpServer != nil {
		a.mcpServer.Stop()
		a.mcpServer = nil
	}
}

// startup Wails启动回调
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// SetLanguage 前端调用，同步语言设置到后端
func (a *App) SetLanguage(lang string) {
	a.lang = lang
}

// GetLanguage 返回当前语言
func (a *App) GetLanguage() string {
	return a.lang
}

// langCtx 返回带语言设置的context，每个绑定方法内部调用
func (a *App) langCtx() context.Context {
	return i18n.WithLanguage(a.ctx, a.lang)
}

// --- 资产操作 ---

// GetAsset 获取资产详情
func (a *App) GetAsset(id int64) (*asset_entity.Asset, error) {
	return asset_svc.Asset().Get(a.langCtx(), id)
}

// ListAssets 列出资产
func (a *App) ListAssets(assetType string, groupID int64) ([]*asset_entity.Asset, error) {
	return asset_svc.Asset().List(a.langCtx(), assetType, groupID)
}

// CreateAsset 创建资产
func (a *App) CreateAsset(asset *asset_entity.Asset) error {
	return asset_svc.Asset().Create(a.langCtx(), asset)
}

// UpdateAsset 更新资产
func (a *App) UpdateAsset(asset *asset_entity.Asset) error {
	return asset_svc.Asset().Update(a.langCtx(), asset)
}

// DeleteAsset 删除资产
func (a *App) DeleteAsset(id int64) error {
	return asset_svc.Asset().Delete(a.langCtx(), id)
}

// --- 分组操作 ---

// ListGroups 列出所有分组
func (a *App) ListGroups() ([]*group_entity.Group, error) {
	return group_repo.Group().List(a.langCtx())
}

// CreateGroup 创建分组
func (a *App) CreateGroup(group *group_entity.Group) error {
	if err := group.Validate(); err != nil {
		return err
	}
	return group_repo.Group().Create(a.langCtx(), group)
}

// UpdateGroup 更新分组
func (a *App) UpdateGroup(group *group_entity.Group) error {
	if err := group.Validate(); err != nil {
		return err
	}
	return group_repo.Group().Update(a.langCtx(), group)
}

// DeleteGroup 删除分组
// deleteAssets: true 删除分组下的资产，false 移动到未分组
func (a *App) DeleteGroup(id int64, deleteAssets bool) error {
	ctx := a.langCtx()
	// 获取分组信息，用于将子分组挂到父分组
	group, err := group_repo.Group().Find(ctx, id)
	if err != nil {
		return err
	}
	// 子分组挂到被删分组的父级
	if err := group_repo.Group().ReparentChildren(ctx, id, group.ParentID); err != nil {
		return err
	}
	// 处理分组下的资产
	if deleteAssets {
		if err := asset_repo.Asset().DeleteByGroupID(ctx, id); err != nil {
			return err
		}
	} else {
		if err := asset_repo.Asset().MoveToGroup(ctx, id, 0); err != nil {
			return err
		}
	}
	return group_repo.Group().Delete(ctx, id)
}

// --- SSH 操作 ---

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

	// 解析存储的凭证
	password := req.Password
	key := req.Key
	if password == "" && sshCfg.AuthType == "password" && sshCfg.Password != "" {
		decrypted, err := credential_svc.Default().Decrypt(sshCfg.Password)
		if err == nil {
			password = decrypted
		}
	}
	if key == "" && sshCfg.AuthType == "key" && sshCfg.KeySource == "managed" && sshCfg.KeyID > 0 {
		privKey, err := ssh_key_svc.GetPrivateKey(a.langCtx(), sshCfg.KeyID)
		if err == nil {
			key = privKey
		}
	}

	connectCfg := ssh_svc.ConnectConfig{
		Host:        sshCfg.Host,
		Port:        sshCfg.Port,
		Username:    sshCfg.Username,
		AuthType:    sshCfg.AuthType,
		Password:    password,
		Key:         key,
		PrivateKeys: sshCfg.PrivateKeys,
		AssetID:     req.AssetID,
		Cols:        req.Cols,
		Rows:        req.Rows,
		Proxy:       sshCfg.Proxy,
		OnData: func(sid string, data []byte) {
			wailsRuntime.EventsEmit(a.ctx, "ssh:data:"+sid, base64.StdEncoding.EncodeToString(data))
		},
		OnClosed: func(sid string) {
			wailsRuntime.EventsEmit(a.ctx, "ssh:closed:"+sid, nil)
		},
	}

	// 解析跳板机链（递归，最大深度 5）
	if sshCfg.JumpHostID > 0 {
		jumpHosts, err := a.resolveJumpHosts(sshCfg.JumpHostID, 5)
		if err != nil {
			return "", fmt.Errorf("解析跳板机失败: %w", err)
		}
		connectCfg.JumpHosts = jumpHosts
	}

	sessionID, err := a.sshManager.Connect(connectCfg)
	if err != nil {
		return "", err
	}
	return sessionID, nil
}

// resolveJumpHosts 递归解析跳板机链，返回从第一跳到最后一跳的顺序
func (a *App) resolveJumpHosts(jumpHostID int64, maxDepth int) ([]ssh_svc.JumpHostEntry, error) {
	if maxDepth <= 0 {
		return nil, fmt.Errorf("跳板机链过深，可能存在循环引用")
	}

	jumpAsset, err := asset_svc.Asset().Get(a.langCtx(), jumpHostID)
	if err != nil {
		return nil, fmt.Errorf("跳板机资产不存在(ID=%d): %w", jumpHostID, err)
	}
	jumpCfg, err := jumpAsset.GetSSHConfig()
	if err != nil {
		return nil, err
	}

	entry := ssh_svc.JumpHostEntry{
		Host:     jumpCfg.Host,
		Port:     jumpCfg.Port,
		Username: jumpCfg.Username,
		AuthType: jumpCfg.AuthType,
	}

	// 如果跳板机自身也有跳板机，递归解析
	if jumpCfg.JumpHostID > 0 {
		parentHosts, err := a.resolveJumpHosts(jumpCfg.JumpHostID, maxDepth-1)
		if err != nil {
			return nil, err
		}
		// 父级跳板机在前，当前在后
		return append(parentHosts, entry), nil
	}

	return []ssh_svc.JumpHostEntry{entry}, nil
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

// DisconnectSSH 断开 SSH 连接
func (a *App) DisconnectSSH(sessionID string) {
	a.sshManager.Disconnect(sessionID)
}

// --- SFTP 文件传输 ---

// SFTPGetwd 获取远程工作目录（用户 home）
func (a *App) SFTPGetwd(sessionID string) (string, error) {
	return a.sftpService.Getwd(sessionID)
}

// SFTPListDir 列出远程目录内容
func (a *App) SFTPListDir(sessionID, dirPath string) ([]sftp_svc.FileEntry, error) {
	return a.sftpService.ListDir(sessionID, dirPath)
}

// SFTPUpload 上传文件：弹出本地文件选择 → 上传到 remotePath
func (a *App) SFTPUpload(sessionID, remotePath string) (string, error) {
	localPath, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "选择上传文件",
	})
	if err != nil {
		return "", fmt.Errorf("打开文件对话框失败: %w", err)
	}
	if localPath == "" {
		return "", nil // 用户取消
	}

	// 如果 remotePath 以 / 结尾，则拼接本地文件名
	if strings.HasSuffix(remotePath, "/") {
		remotePath += filepath.Base(localPath)
	}

	transferID := a.sftpService.GenerateTransferID()
	go func() {
		err := a.sftpService.Upload(a.ctx, transferID, sessionID, localPath, remotePath, func(p sftp_svc.TransferProgress) {
			wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, p)
		})
		if err != nil {
			wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, sftp_svc.TransferProgress{
				TransferID: transferID,
				Status:     "error",
				Error:      err.Error(),
			})
			return
		}
		wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, sftp_svc.TransferProgress{
			TransferID: transferID,
			Status:     "done",
		})
	}()
	return transferID, nil
}

// SFTPUploadDir 上传目录：弹出本地目录选择 → 上传到 remotePath
func (a *App) SFTPUploadDir(sessionID, remotePath string) (string, error) {
	localDir, err := wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "选择上传文件夹",
	})
	if err != nil {
		return "", fmt.Errorf("打开目录对话框失败: %w", err)
	}
	if localDir == "" {
		return "", nil
	}

	// remotePath 拼接本地目录名
	if strings.HasSuffix(remotePath, "/") {
		remotePath += filepath.Base(localDir)
	} else {
		remotePath += "/" + filepath.Base(localDir)
	}

	transferID := a.sftpService.GenerateTransferID()
	go func() {
		err := a.sftpService.UploadDir(a.ctx, transferID, sessionID, localDir, remotePath, func(p sftp_svc.TransferProgress) {
			wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, p)
		})
		if err != nil {
			wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, sftp_svc.TransferProgress{
				TransferID: transferID,
				Status:     "error",
				Error:      err.Error(),
			})
			return
		}
		wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, sftp_svc.TransferProgress{
			TransferID: transferID,
			Status:     "done",
		})
	}()
	return transferID, nil
}

// SFTPDownload 下载文件：remotePath → 弹出本地保存对话框
func (a *App) SFTPDownload(sessionID, remotePath string) (string, error) {
	// 以远程文件名作为默认文件名
	defaultName := filepath.Base(remotePath)
	localPath, err := wailsRuntime.SaveFileDialog(a.ctx, wailsRuntime.SaveDialogOptions{
		DefaultFilename: defaultName,
		Title:           "保存到本地",
	})
	if err != nil {
		return "", fmt.Errorf("保存文件对话框失败: %w", err)
	}
	if localPath == "" {
		return "", nil
	}

	transferID := a.sftpService.GenerateTransferID()
	go func() {
		err := a.sftpService.Download(a.ctx, transferID, sessionID, remotePath, localPath, func(p sftp_svc.TransferProgress) {
			wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, p)
		})
		if err != nil {
			wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, sftp_svc.TransferProgress{
				TransferID: transferID,
				Status:     "error",
				Error:      err.Error(),
			})
			return
		}
		wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, sftp_svc.TransferProgress{
			TransferID: transferID,
			Status:     "done",
		})
	}()
	return transferID, nil
}

// SFTPDownloadDir 下载目录：remotePath → 弹出本地目录选择
func (a *App) SFTPDownloadDir(sessionID, remotePath string) (string, error) {
	localDir, err := wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "选择保存目录",
	})
	if err != nil {
		return "", fmt.Errorf("打开目录对话框失败: %w", err)
	}
	if localDir == "" {
		return "", nil
	}

	// 本地目录 + 远程目录名
	localDir = filepath.Join(localDir, filepath.Base(remotePath))

	transferID := a.sftpService.GenerateTransferID()
	go func() {
		err := a.sftpService.DownloadDir(a.ctx, transferID, sessionID, remotePath, localDir, func(p sftp_svc.TransferProgress) {
			wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, p)
		})
		if err != nil {
			wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, sftp_svc.TransferProgress{
				TransferID: transferID,
				Status:     "error",
				Error:      err.Error(),
			})
			return
		}
		wailsRuntime.EventsEmit(a.ctx, "sftp:progress:"+transferID, sftp_svc.TransferProgress{
			TransferID: transferID,
			Status:     "done",
		})
	}()
	return transferID, nil
}

// SFTPCancelTransfer 取消传输
func (a *App) SFTPCancelTransfer(transferID string) {
	a.sftpService.Cancel(transferID)
}

// --- 本地 SSH 密钥发现 ---

// LocalSSHKeyInfo 本地 SSH 密钥信息
type LocalSSHKeyInfo struct {
	Path        string `json:"path"`
	KeyType     string `json:"keyType"`
	Fingerprint string `json:"fingerprint"`
}

// ListLocalSSHKeys 扫描 ~/.ssh 目录，返回有效的私钥列表
func (a *App) ListLocalSSHKeys() ([]LocalSSHKeyInfo, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("获取用户目录失败: %w", err)
	}
	sshDir := filepath.Join(homeDir, ".ssh")

	entries, err := os.ReadDir(sshDir)
	if err != nil {
		// ~/.ssh 不存在时返回空列表
		if os.IsNotExist(err) {
			return []LocalSSHKeyInfo{}, nil
		}
		return nil, fmt.Errorf("读取 .ssh 目录失败: %w", err)
	}

	// 需要跳过的文件
	skipFiles := map[string]bool{
		"known_hosts":     true,
		"known_hosts.old": true,
		"config":          true,
		"authorized_keys": true,
		"environment":     true,
	}

	var keys []LocalSSHKeyInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// 跳过公钥、已知文件和隐藏文件
		if strings.HasSuffix(name, ".pub") || skipFiles[name] || strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".sock") {
			continue
		}

		fullPath := filepath.Join(sshDir, name)
		info, err := parseLocalSSHKey(fullPath)
		if err != nil {
			continue // 不是有效私钥，跳过
		}
		keys = append(keys, *info)
	}

	if keys == nil {
		keys = []LocalSSHKeyInfo{}
	}
	return keys, nil
}

// SelectSSHKeyFile 打开文件选择框选择密钥文件，默认定位到 ~/.ssh
func (a *App) SelectSSHKeyFile() (*LocalSSHKeyInfo, error) {
	homeDir, _ := os.UserHomeDir()
	defaultDir := filepath.Join(homeDir, ".ssh")

	filePath, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title:            "选择 SSH 私钥文件",
		DefaultDirectory: defaultDir,
	})
	if err != nil {
		return nil, fmt.Errorf("打开文件对话框失败: %w", err)
	}
	if filePath == "" {
		return nil, nil // 用户取消
	}

	info, err := parseLocalSSHKey(filePath)
	if err != nil {
		return nil, fmt.Errorf("所选文件不是有效的 SSH 私钥: %w", err)
	}
	return info, nil
}

// parseLocalSSHKey 解析本地私钥文件，返回密钥信息
func parseLocalSSHKey(path string) (*LocalSSHKeyInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// 快速检查：私钥文件通常以 "-----BEGIN" 开头或是 OpenSSH 格式
	if len(data) == 0 {
		return nil, fmt.Errorf("empty file")
	}

	signer, err := ssh.ParsePrivateKey(data)
	if err != nil {
		return nil, err
	}

	pubKey := signer.PublicKey()
	fingerprint := ssh.FingerprintSHA256(pubKey)
	keyType := pubKey.Type()

	return &LocalSSHKeyInfo{
		Path:        path,
		KeyType:     keyType,
		Fingerprint: fingerprint,
	}, nil
}

// --- AI 操作 ---

// ConversationDisplayMessage 返回给前端的会话消息（用于恢复显示）
type ConversationDisplayMessage struct {
	Role    string                          `json:"role"`
	Content string                          `json:"content"`
	Blocks  []conversation_entity.ContentBlock `json:"blocks"`
}

// CreateConversation 创建新会话
func (a *App) CreateConversation() (*conversation_entity.Conversation, error) {
	if a.aiAgent == nil {
		return nil, fmt.Errorf("请先配置 AI Provider")
	}

	ctx := a.langCtx()
	conv := &conversation_entity.Conversation{
		Title:        "新对话",
		ProviderType: a.aiProviderType,
		Model:        a.aiModel,
	}

	// 本地 CLI 模式创建工作目录
	if a.aiProviderType == "local_cli" {
		workDir := filepath.Join(appDataDir(), "workspaces", fmt.Sprintf("conv-%d", time.Now().UnixMilli()))
		conv.WorkDir = workDir
	}

	if err := conversation_svc.Conversation().Create(ctx, conv); err != nil {
		return nil, err
	}

	// 如果有工作目录，更新路径为带 ID 的稳定路径
	if conv.WorkDir != "" {
		stableDir := filepath.Join(appDataDir(), "workspaces", fmt.Sprintf("%d", conv.ID))
		if err := os.Rename(conv.WorkDir, stableDir); err == nil {
			conv.WorkDir = stableDir
			_ = conversation_svc.Conversation().Update(ctx, conv)
		}
	}

	// 切换到新会话
	a.switchToConversation(conv)

	return conv, nil
}

// ListConversations 获取会话列表
func (a *App) ListConversations() ([]*conversation_entity.Conversation, error) {
	return conversation_svc.Conversation().List(a.langCtx())
}

// SwitchConversation 切换到指定会话，返回显示消息
func (a *App) SwitchConversation(id int64) ([]ConversationDisplayMessage, error) {
	ctx := a.langCtx()
	conv, err := conversation_svc.Conversation().Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("会话不存在: %w", err)
	}

	a.switchToConversation(conv)

	// 加载消息用于前端显示
	msgs, err := conversation_svc.Conversation().LoadMessages(ctx, id)
	if err != nil {
		return nil, err
	}

	var displayMsgs []ConversationDisplayMessage
	for _, msg := range msgs {
		blocks, _ := msg.GetBlocks()
		displayMsgs = append(displayMsgs, ConversationDisplayMessage{
			Role:    msg.Role,
			Content: msg.Content,
			Blocks:  blocks,
		})
	}
	return displayMsgs, nil
}

// switchToConversation 内部切换会话逻辑
func (a *App) switchToConversation(conv *conversation_entity.Conversation) {
	a.currentConversationID = conv.ID

	if p, ok := a.aiProvider.(*ai.LocalCLIProvider); ok {
		// 恢复 CLI session
		info, err := conv.GetSessionInfo()
		if err == nil && info.SessionID != "" {
			p.SetSessionID(info.SessionID)
		} else {
			p.SetSessionID("")
		}

		// 切换工作目录
		if conv.WorkDir != "" {
			if a.mcpServer != nil {
				_ = a.mcpServer.WriteConfigToDir(conv.WorkDir)
			}
			p.SetMCPWorkDir(conv.WorkDir)
		}
	}
}

// DeleteConversation 删除会话
func (a *App) DeleteConversation(id int64) error {
	err := conversation_svc.Conversation().Delete(a.langCtx(), id)
	if err != nil {
		return err
	}
	// 如果删的是当前会话，清空当前会话ID
	if a.currentConversationID == id {
		a.currentConversationID = 0
	}
	return nil
}

// SendAIMessage 发送 AI 消息，通过 Wails Events 流式返回
func (a *App) SendAIMessage(messages []ai.Message) error {
	if a.aiAgent == nil {
		return fmt.Errorf("请先配置 AI Provider")
	}

	ctx := a.langCtx()

	// 自动创建会话（首次发消息时）
	if a.currentConversationID == 0 {
		conv, err := a.CreateConversation()
		if err != nil {
			return fmt.Errorf("创建会话失败: %w", err)
		}
		// 用首条用户消息作为标题
		for _, msg := range messages {
			if msg.Role == ai.RoleUser {
				title := string(msg.Content)
				if len([]rune(title)) > 50 {
					title = string([]rune(title)[:50])
				}
				conv.Title = title
				_ = conversation_svc.Conversation().Update(ctx, conv)
				break
			}
		}
	}

	convID := a.currentConversationID
	eventName := fmt.Sprintf("ai:event:%d", convID)

	// 添加系统提示
	fullMessages := []ai.Message{
		{
			Role:    ai.RoleSystem,
			Content: "You are the Ops Cat AI assistant, helping users manage IT assets. You can list assets, view details, add assets, and run commands on SSH servers. Respond in the same language the user uses.",
		},
	}
	fullMessages = append(fullMessages, messages...)

	go func() {
		err := a.aiAgent.Chat(a.ctx, fullMessages, func(event ai.StreamEvent) {
			wailsRuntime.EventsEmit(a.ctx, eventName, event)
		})
		if err != nil {
			wailsRuntime.EventsEmit(a.ctx, eventName, ai.StreamEvent{
				Type:  "error",
				Error: err.Error(),
			})
		}

		// 消息完成后持久化
		a.persistConversationState(convID, messages)
	}()

	return nil
}

// persistConversationState 持久化会话状态（消息+session）
func (a *App) persistConversationState(convID int64, messages []ai.Message) {
	ctx := a.langCtx()

	// 保存 local CLI session ID
	if p, ok := a.aiProvider.(*ai.LocalCLIProvider); ok {
		conv, err := conversation_svc.Conversation().Get(ctx, convID)
		if err == nil {
			sessionID := p.GetSessionID()
			_ = conv.SetSessionInfo(&conversation_entity.SessionInfo{
				SessionID: sessionID,
			})
			conv.Updatetime = time.Now().Unix()
			_ = conversation_svc.Conversation().Update(ctx, conv)
		}
	}
}

// SaveConversationMessages 前端调用，保存显示消息到数据库
func (a *App) SaveConversationMessages(displayMsgs []ConversationDisplayMessage) error {
	if a.currentConversationID == 0 {
		return nil
	}
	ctx := a.langCtx()
	var msgs []*conversation_entity.Message
	for i, dm := range displayMsgs {
		msg := &conversation_entity.Message{
			ConversationID: a.currentConversationID,
			Role:           dm.Role,
			Content:        dm.Content,
			SortOrder:      i,
			Createtime:     time.Now().Unix(),
		}
		_ = msg.SetBlocks(dm.Blocks)
		msgs = append(msgs, msg)
	}
	return conversation_svc.Conversation().SaveMessages(ctx, a.currentConversationID, msgs)
}

// GetCurrentConversationID 获取当前会话ID
func (a *App) GetCurrentConversationID() int64 {
	return a.currentConversationID
}

// DetectLocalCLIs 检测本地 AI CLI 工具
func (a *App) DetectLocalCLIs() []ai.CLIInfo {
	return ai.DetectLocalCLIs()
}

// RespondPermission 前端响应权限确认请求
func (a *App) RespondPermission(behavior, message string) {
	select {
	case a.permissionChan <- ai.PermissionResponse{Behavior: behavior, Message: message}:
	default:
	}
}

// ResetAISession 重置 AI 会话（创建新会话）
func (a *App) ResetAISession() {
	if p, ok := a.aiProvider.(*ai.LocalCLIProvider); ok {
		p.ResetSession()
	}
	a.currentConversationID = 0
}

// GetInitContext 获取 /init 命令的资产上下文信息
func (a *App) GetInitContext(assetID int64, groupID int64) (string, error) {
	ctx := a.langCtx()
	var sb strings.Builder

	if assetID > 0 {
		asset, err := asset_svc.Asset().Get(ctx, assetID)
		if err != nil {
			return "", fmt.Errorf("获取资产失败: %w", err)
		}
		sb.WriteString("=== 资产初始化分析 ===\n\n")
		sb.WriteString(a.formatAssetContext(asset))
	} else if groupID > 0 {
		group, err := group_repo.Group().Find(ctx, groupID)
		if err != nil {
			return "", fmt.Errorf("获取分组失败: %w", err)
		}
		assets, err := asset_svc.Asset().List(ctx, "", groupID)
		if err != nil {
			return "", fmt.Errorf("获取资产列表失败: %w", err)
		}
		sb.WriteString(fmt.Sprintf("=== 分组「%s」初始化分析 ===\n\n", group.Name))
		if len(assets) == 0 {
			sb.WriteString("该分组下没有资产。\n")
		}
		for _, asset := range assets {
			sb.WriteString(a.formatAssetContext(asset))
			sb.WriteString("\n")
		}
	} else {
		return "", fmt.Errorf("请选择一个资产或分组")
	}

	sb.WriteString("\n请分析以上服务器环境，执行以下发现命令：\n")
	sb.WriteString("1. uname -a（操作系统信息）\n")
	sb.WriteString("2. cat /etc/os-release（发行版信息）\n")
	sb.WriteString("3. hostname（主机名）\n")
	sb.WriteString("4. df -h（磁盘使用）\n")
	sb.WriteString("5. free -h（内存使用）\n")
	sb.WriteString("6. nproc（CPU核心数）\n")
	sb.WriteString("7. ip addr 或 ifconfig（网络接口）\n")
	sb.WriteString("8. docker ps 2>/dev/null（Docker容器）\n")
	sb.WriteString("9. systemctl list-units --type=service --state=running 2>/dev/null | head -20（运行中的服务）\n")

	return sb.String(), nil
}

// formatAssetContext 格式化单个资产的上下文信息
func (a *App) formatAssetContext(asset *asset_entity.Asset) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("- 名称: %s (ID: %d)\n", asset.Name, asset.ID))
	sb.WriteString(fmt.Sprintf("  类型: %s\n", asset.Type))

	if asset.IsSSH() {
		cfg, err := asset.GetSSHConfig()
		if err == nil {
			sb.WriteString(fmt.Sprintf("  地址: %s:%d\n", cfg.Host, cfg.Port))
			sb.WriteString(fmt.Sprintf("  用户: %s\n", cfg.Username))
			sb.WriteString(fmt.Sprintf("  认证: %s\n", cfg.AuthType))
		}
	}

	if asset.Description != "" {
		sb.WriteString(fmt.Sprintf("  描述: %s\n", asset.Description))
	}
	return sb.String()
}

// --- 凭证操作 ---

// SaveCredential 加密保存凭证（密码或密钥），返回加密后的字符串
func (a *App) SaveCredential(plaintext string) (string, error) {
	return credential_svc.Default().Encrypt(plaintext)
}

// LoadCredential 解密凭证
func (a *App) LoadCredential(ciphertext string) (string, error) {
	return credential_svc.Default().Decrypt(ciphertext)
}

// --- 导入导出 ---

// PreviewTabbyConfig 预览 Tabby 配置（不写入数据库）
// 自动检测默认路径，找不到则弹出文件选择框
func (a *App) PreviewTabbyConfig() (*import_svc.PreviewResult, error) {
	data, err := a.readTabbyConfig()
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	return import_svc.PreviewTabbyConfig(a.langCtx(), data)
}

// ImportTabbySelected 导入用户选中的 Tabby 连接
func (a *App) ImportTabbySelected(selectedIndexes []int) (*import_svc.ImportResult, error) {
	data, err := a.readTabbyConfig()
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	return import_svc.ImportTabbySelected(a.langCtx(), data, selectedIndexes)
}

// readTabbyConfig 读取 Tabby 配置文件内容
func (a *App) readTabbyConfig() ([]byte, error) {
	filePath := detectTabbyConfigPath()
	if filePath == "" {
		var err error
		filePath, err = wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
			Title: "选择 Tabby 配置文件",
			Filters: []wailsRuntime.FileFilter{
				{DisplayName: "YAML Files", Pattern: "*.yaml;*.yml"},
				{DisplayName: "All Files", Pattern: "*"},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("打开文件对话框失败: %w", err)
		}
		if filePath == "" {
			return nil, nil
		}
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}
	return data, nil
}

// detectTabbyConfigPath 检测 Tabby 配置文件默认路径
func detectTabbyConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		candidates = []string{
			filepath.Join(homeDir, "Library", "Application Support", "tabby", "config.yaml"),
		}
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData != "" {
			candidates = []string{
				filepath.Join(appData, "Tabby", "config.yaml"),
			}
		}
	case "linux":
		candidates = []string{
			filepath.Join(homeDir, ".config", "tabby", "config.yaml"),
		}
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// ExportData 导出所有资产和分组为 JSON
func (a *App) ExportData() (string, error) {
	data, err := backup_svc.Export(a.langCtx())
	if err != nil {
		return "", err
	}
	result, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

// --- 备份操作 ---

// ExportToFile 导出备份到文件，password 为空则不加密
func (a *App) ExportToFile(password string) error {
	data, err := backup_svc.Export(a.langCtx())
	if err != nil {
		return err
	}
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	var output []byte
	var defaultName string
	if password != "" {
		output, err = backup_svc.EncryptBackup(jsonData, password)
		if err != nil {
			return err
		}
		defaultName = fmt.Sprintf("ops-cat-backup-%s.encrypted.json", time.Now().Format("20060102"))
	} else {
		output = jsonData
		defaultName = fmt.Sprintf("ops-cat-backup-%s.json", time.Now().Format("20060102"))
	}

	filePath, err := wailsRuntime.SaveFileDialog(a.ctx, wailsRuntime.SaveDialogOptions{
		DefaultFilename: defaultName,
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "JSON Files", Pattern: "*.json"},
		},
	})
	if err != nil {
		return fmt.Errorf("保存文件对话框失败: %w", err)
	}
	if filePath == "" {
		return nil
	}

	return os.WriteFile(filePath, output, 0644)
}

// ImportFileInfo 导入文件信息
type ImportFileInfo struct {
	FilePath  string `json:"filePath"`
	Encrypted bool   `json:"encrypted"`
}

// SelectImportFile 选择备份文件并检测是否加密
func (a *App) SelectImportFile() (*ImportFileInfo, error) {
	filePath, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "导入备份",
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "JSON Files", Pattern: "*.json"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("打开文件对话框失败: %w", err)
	}
	if filePath == "" {
		return nil, nil
	}

	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}

	return &ImportFileInfo{
		FilePath:  filePath,
		Encrypted: backup_svc.IsEncryptedBackup(fileData),
	}, nil
}

// ExecuteImportFile 执行文件导入
func (a *App) ExecuteImportFile(filePath, password string) error {
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("读取文件失败: %w", err)
	}

	var jsonData []byte
	if backup_svc.IsEncryptedBackup(fileData) {
		jsonData, err = backup_svc.DecryptBackup(fileData, password)
		if err != nil {
			return err
		}
	} else {
		jsonData = fileData
	}

	var data backup_svc.BackupData
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return fmt.Errorf("解析备份数据失败: %w", err)
	}

	return backup_svc.Import(a.langCtx(), &data)
}

// --- GitHub 认证 ---

// StartGitHubDeviceFlow 发起 GitHub Device Flow 认证
func (a *App) StartGitHubDeviceFlow() (*backup_svc.DeviceFlowInfo, error) {
	return backup_svc.StartDeviceFlow()
}

// WaitGitHubDeviceAuth 等待用户完成 GitHub 授权，返回 access_token
func (a *App) WaitGitHubDeviceAuth(deviceCode string, interval int) (string, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Minute)
	a.githubAuthCancel = cancel
	defer func() {
		cancel()
		a.githubAuthCancel = nil
	}()
	return backup_svc.PollDeviceAuth(ctx, deviceCode, interval)
}

// CancelGitHubAuth 取消 GitHub 授权等待
func (a *App) CancelGitHubAuth() {
	if a.githubAuthCancel != nil {
		a.githubAuthCancel()
	}
}

// GetGitHubUser 获取 GitHub 用户信息
func (a *App) GetGitHubUser(token string) (*backup_svc.GitHubUser, error) {
	return backup_svc.GetGitHubUser(token)
}

// --- Gist 备份 ---

// ExportToGist 加密并上传备份到 Gist
func (a *App) ExportToGist(password, token, gistID string) (*backup_svc.GistInfo, error) {
	data, err := backup_svc.Export(a.langCtx())
	if err != nil {
		return nil, err
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	encrypted, err := backup_svc.EncryptBackup(jsonData, password)
	if err != nil {
		return nil, err
	}

	return backup_svc.CreateOrUpdateGist(token, gistID, encrypted)
}

// ListBackupGists 列出用户的备份 Gist
func (a *App) ListBackupGists(token string) ([]*backup_svc.GistInfo, error) {
	return backup_svc.ListBackupGists(token)
}

// --- SSH 密钥管理 ---

// ListSSHKeys 列出所有 SSH 托管密钥
func (a *App) ListSSHKeys() ([]*ssh_key_entity.SSHKey, error) {
	return ssh_key_svc.List(a.langCtx())
}

// GenerateSSHKey 生成新的 SSH 密钥对
func (a *App) GenerateSSHKey(name, comment, keyType string, keySize int) (*ssh_key_entity.SSHKey, error) {
	return ssh_key_svc.Generate(a.langCtx(), ssh_key_svc.GenerateRequest{
		Name:    name,
		Comment: comment,
		KeyType: keyType,
		KeySize: keySize,
	})
}

// ImportSSHKeyFile 通过文件选择框导入 SSH 密钥
func (a *App) ImportSSHKeyFile(name, comment string) (*ssh_key_entity.SSHKey, error) {
	filePath, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "选择 SSH 私钥文件",
	})
	if err != nil {
		return nil, fmt.Errorf("打开文件对话框失败: %w", err)
	}
	if filePath == "" {
		return nil, nil
	}
	return ssh_key_svc.ImportFromFile(a.langCtx(), name, comment, filePath)
}

// ImportSSHKeyPEM 通过粘贴 PEM 内容导入 SSH 密钥
func (a *App) ImportSSHKeyPEM(name, comment, pemData string) (*ssh_key_entity.SSHKey, error) {
	return ssh_key_svc.ImportFromPEM(a.langCtx(), name, comment, pemData)
}

// UpdateSSHKey 更新 SSH 密钥名称和注释
func (a *App) UpdateSSHKey(id int64, name, comment string) (*ssh_key_entity.SSHKey, error) {
	return ssh_key_svc.Update(a.langCtx(), ssh_key_svc.UpdateRequest{
		ID:      id,
		Name:    name,
		Comment: comment,
	})
}

// GetSSHKeyUsage 获取引用此 SSH 密钥的资产名称列表
func (a *App) GetSSHKeyUsage(id int64) ([]string, error) {
	assets, err := asset_repo.Asset().FindBySSHKeyID(a.langCtx(), id)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(assets))
	for i, asset := range assets {
		names[i] = asset.Name
	}
	return names, nil
}

// DeleteSSHKey 删除 SSH 密钥
func (a *App) DeleteSSHKey(id int64) error {
	return ssh_key_svc.Delete(a.langCtx(), id)
}

// GetSSHKeyPublicKey 获取密钥公钥（用于复制）
func (a *App) GetSSHKeyPublicKey(id int64) (string, error) {
	key, err := ssh_key_svc.Get(a.langCtx(), id)
	if err != nil {
		return "", err
	}
	return key.PublicKey, nil
}

// ImportFromGist 从 Gist 导入备份
func (a *App) ImportFromGist(gistID, password, token string) error {
	content, err := backup_svc.GetGistContent(token, gistID)
	if err != nil {
		return err
	}

	jsonData, err := backup_svc.DecryptBackup(content, password)
	if err != nil {
		return err
	}

	var data backup_svc.BackupData
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return fmt.Errorf("解析备份数据失败: %w", err)
	}

	return backup_svc.Import(a.langCtx(), &data)
}

