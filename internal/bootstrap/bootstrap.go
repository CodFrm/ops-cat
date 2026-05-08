package bootstrap

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/opskat/opskat/internal/repository/ai_provider_repo"
	"github.com/opskat/opskat/internal/repository/asset_repo"
	"github.com/opskat/opskat/internal/repository/audit_repo"
	"github.com/opskat/opskat/internal/repository/conversation_repo"
	"github.com/opskat/opskat/internal/repository/credential_repo"
	"github.com/opskat/opskat/internal/repository/extension_data_repo"
	"github.com/opskat/opskat/internal/repository/extension_state_repo"
	"github.com/opskat/opskat/internal/repository/forward_repo"
	"github.com/opskat/opskat/internal/repository/grant_repo"
	"github.com/opskat/opskat/internal/repository/group_repo"
	"github.com/opskat/opskat/internal/repository/host_key_repo"
	"github.com/opskat/opskat/internal/repository/local_tool_grant_repo"
	"github.com/opskat/opskat/internal/repository/policy_group_repo"
	"github.com/opskat/opskat/internal/repository/snippet_repo"
	"github.com/opskat/opskat/internal/service/credential_svc"
	"github.com/opskat/opskat/internal/service/snippet_svc"
	"github.com/opskat/opskat/migrations"

	"github.com/cago-frame/cago"
	"github.com/cago-frame/cago/configs"
	"github.com/cago-frame/cago/configs/memory"
	"github.com/cago-frame/cago/database/db"

	_ "github.com/opskat/opskat/internal/pkg/code"

	_ "github.com/cago-frame/cago/database/db/sqlite"
)

// Options 初始化选项
type Options struct {
	DataDir   string // 空则用默认平台目录
	MasterKey string // 空则从 Keychain/文件自动获取或生成
}

// AppDataDir 返回应用数据目录
func AppDataDir() string {
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "opskat")
	case "windows":
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			home, _ := os.UserHomeDir()
			localAppData = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(localAppData, "opskat")
	default:
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "opskat")
	}
}

// Init 初始化数据库、凭证服务、注册 Repository、运行迁移
func Init(ctx context.Context, opts Options) error {
	dataDir := opts.DataDir
	if dataDir == "" {
		dataDir = AppDataDir()
	}

	if err := os.MkdirAll(filepath.Join(dataDir, "logs"), 0755); err != nil {
		return err
	}

	// 获取 master key：CLI 参数 > Keychain > 文件 > 自动生成
	masterKey, err := credential_svc.ResolveMasterKey(opts.MasterKey, dataDir)
	if err != nil {
		return fmt.Errorf("获取 master key 失败: %w", err)
	}

	cfg, err := configs.NewConfig("opskat", configs.WithSource(memory.NewSource(map[string]interface{}{
		"db": map[string]interface{}{
			"driver": "sqlite",
			"dsn":    filepath.Join(dataDir, "opskat.db"),
		},
	})))
	if err != nil {
		return err
	}

	cago.New(ctx, cfg).
		Registry(db.Database())

	// 获取或生成 KDF salt
	salt, err := resolveKDFSalt(dataDir)
	if err != nil {
		return fmt.Errorf("获取 KDF salt 失败: %w", err)
	}

	credential_svc.SetDefault(credential_svc.New(masterKey, salt))

	registerRepositories()

	if err := migrations.RunMigrations(db.Default()); err != nil {
		return err
	}

	snippet_svc.Register(snippet_svc.NewSnippetSvc(snippet_svc.NewCategoryRegistry()))

	return nil
}

// registerRepositories 注册所有 Repository 单例
func registerRepositories() {
	asset_repo.RegisterAsset(asset_repo.NewAsset())
	audit_repo.RegisterAudit(audit_repo.NewAudit())
	conversation_repo.RegisterConversation(conversation_repo.NewConversation())
	group_repo.RegisterGroup(group_repo.NewGroup())
	grant_repo.RegisterGrant(grant_repo.NewGrant())
	local_tool_grant_repo.Register(local_tool_grant_repo.New())
	credential_repo.RegisterCredential(credential_repo.NewCredential())
	host_key_repo.RegisterHostKey(host_key_repo.NewHostKey())
	forward_repo.RegisterForward(forward_repo.NewForward())
	policy_group_repo.RegisterPolicyGroup(policy_group_repo.NewPolicyGroup())
	ai_provider_repo.RegisterAIProvider(ai_provider_repo.NewAIProvider())
	extension_data_repo.RegisterExtensionData(extension_data_repo.NewExtensionData())
	extension_state_repo.RegisterExtensionState(extension_state_repo.NewExtensionState())
	snippet_repo.RegisterSnippet(snippet_repo.NewSnippet())
}

// resolveKDFSalt 从 config.json 获取 salt，不存在则生成并持久化
func resolveKDFSalt(dataDir string) ([]byte, error) {
	appCfg, err := LoadConfig(dataDir)
	if err != nil {
		return nil, err
	}

	if appCfg.KDFSalt != "" {
		salt, err := base64.StdEncoding.DecodeString(appCfg.KDFSalt)
		if err != nil {
			return nil, fmt.Errorf("解码 KDF salt 失败: %w", err)
		}
		return salt, nil
	}

	// 首次启动，生成 salt
	salt, err := credential_svc.GenerateSalt()
	if err != nil {
		return nil, err
	}

	appCfg.KDFSalt = base64.StdEncoding.EncodeToString(salt)
	if err := SaveConfig(appCfg); err != nil {
		return nil, fmt.Errorf("保存 KDF salt 失败: %w", err)
	}

	return salt, nil
}
