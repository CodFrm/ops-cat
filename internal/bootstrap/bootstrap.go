package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"runtime"

	"ops-cat/internal/repository/asset_repo"
	"ops-cat/internal/repository/audit_repo"
	"ops-cat/internal/repository/conversation_repo"
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

const defaultMasterKey = "ops-cat-default-master-key"

// Options 初始化选项
type Options struct {
	DataDir   string // 空则用默认平台目录
	MasterKey string // 空则用默认主密钥
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
	masterKey := opts.MasterKey
	if masterKey == "" {
		masterKey = defaultMasterKey
	}

	if err := os.MkdirAll(filepath.Join(dataDir, "logs"), 0755); err != nil {
		return err
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

	credential_svc.SetDefault(credential_svc.New(masterKey))

	asset_repo.RegisterAsset(asset_repo.NewAsset())
	audit_repo.RegisterAudit(audit_repo.NewAudit())
	conversation_repo.RegisterConversation(conversation_repo.NewConversation())
	group_repo.RegisterGroup(group_repo.NewGroup())
	plan_repo.RegisterPlan(plan_repo.NewPlan())
	ssh_key_repo.RegisterSSHKey(ssh_key_repo.NewSSHKey())
	forward_repo.RegisterForward(forward_repo.NewForward())

	if err := migrations.RunMigrations(db.Default()); err != nil {
		return err
	}

	return nil
}
