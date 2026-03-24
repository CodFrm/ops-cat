package bootstrap

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"ops-cat/internal/repository/asset_repo"
	"ops-cat/internal/repository/audit_repo"
	"ops-cat/internal/repository/conversation_repo"
	"ops-cat/internal/repository/credential_repo"
	"ops-cat/internal/repository/forward_repo"
	"ops-cat/internal/repository/group_repo"
	"ops-cat/internal/repository/plan_repo"
	"ops-cat/internal/repository/ssh_key_repo"
	"ops-cat/internal/service/credential_svc"
	"ops-cat/migrations"

	"github.com/cago-frame/cago"
	"github.com/cago-frame/cago/configs"
	"github.com/cago-frame/cago/configs/memory"
	"github.com/cago-frame/cago/database/db"

	_ "ops-cat/internal/pkg/code"

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
		return filepath.Join(home, "Library", "Application Support", "ops-cat")
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "ops-cat")
	default:
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "ops-cat")
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

	cfg, err := configs.NewConfig("ops-cat", configs.WithSource(memory.NewSource(map[string]interface{}{
		"db": map[string]interface{}{
			"driver": "sqlite",
			"dsn":    filepath.Join(dataDir, "ops-cat.db"),
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

	asset_repo.RegisterAsset(asset_repo.NewAsset())
	audit_repo.RegisterAudit(audit_repo.NewAudit())
	conversation_repo.RegisterConversation(conversation_repo.NewConversation())
	group_repo.RegisterGroup(group_repo.NewGroup())
	plan_repo.RegisterPlan(plan_repo.NewPlan())
	ssh_key_repo.RegisterSSHKey(ssh_key_repo.NewSSHKey())
	credential_repo.RegisterCredential(credential_repo.NewCredential())
	forward_repo.RegisterForward(forward_repo.NewForward())

	if err := migrations.RunMigrations(db.Default()); err != nil {
		return err
	}

	return nil
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
