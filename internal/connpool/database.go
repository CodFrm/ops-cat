package connpool

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net"
	"net/url"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/sshpool"

	"github.com/cago-frame/cago/pkg/logger"
	"github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver
	"go.uber.org/zap"
)

// DialDatabase 创建数据库连接（直连或通过 SSH 隧道）
// password 为已解析的明文密码，由调用方负责解密
func DialDatabase(ctx context.Context, asset *asset_entity.Asset, cfg *asset_entity.DatabaseConfig, password string, sshPool *sshpool.Pool) (*sql.DB, io.Closer, error) {

	var db *sql.DB
	var tunnel *SSHTunnel
	var err error

	tunnelID := asset.SSHTunnelID
	if tunnelID == 0 {
		tunnelID = cfg.SSHAssetID // backward compat
	}
	if tunnelID > 0 && sshPool != nil {
		tunnel = NewSSHTunnel(tunnelID, cfg.Host, cfg.Port, sshPool)
		db, err = openWithTunnel(cfg, password, tunnel)
	} else {
		db, err = openDirect(cfg, password)
	}
	if err != nil {
		if tunnel != nil {
			if err := tunnel.Close(); err != nil {
				logger.Default().Warn("close ssh tunnel", zap.Error(err))
			}
		}
		return nil, nil, err
	}

	// 连接级只读
	if cfg.ReadOnly {
		if roErr := setReadOnly(db, cfg.Driver); roErr != nil {
			if err := db.Close(); err != nil {
				logger.Default().Warn("close db", zap.Error(err))
			}
			if tunnel != nil {
				if err := tunnel.Close(); err != nil {
					logger.Default().Warn("close ssh tunnel", zap.Error(err))
				}
			}
			return nil, nil, fmt.Errorf("设置只读模式失败: %w", roErr)
		}
	}

	// 测试连接
	if pingErr := db.PingContext(ctx); pingErr != nil {
		if err := db.Close(); err != nil {
			logger.Default().Warn("close db", zap.Error(err))
		}
		if tunnel != nil {
			if err := tunnel.Close(); err != nil {
				logger.Default().Warn("close ssh tunnel", zap.Error(err))
			}
		}
		return nil, nil, fmt.Errorf("数据库连接失败: %w", pingErr)
	}

	return db, tunnel, nil
}

func openDirect(cfg *asset_entity.DatabaseConfig, password string) (*sql.DB, error) {
	driverName, dsn := buildDSN(cfg, password)
	return sql.Open(driverName, dsn)
}

func openWithTunnel(cfg *asset_entity.DatabaseConfig, password string, tunnel *SSHTunnel) (*sql.DB, error) {
	switch cfg.Driver {
	case asset_entity.DriverMySQL:
		return openMySQLWithTunnel(cfg, password, tunnel)
	case asset_entity.DriverPostgreSQL:
		return openPgWithTunnel(cfg, password, tunnel)
	default:
		return nil, fmt.Errorf("不支持的数据库驱动: %s", cfg.Driver)
	}
}

func openMySQLWithTunnel(cfg *asset_entity.DatabaseConfig, password string, tunnel *SSHTunnel) (*sql.DB, error) {
	dialer := fmt.Sprintf("ssh-tunnel-%d", cfg.SSHAssetID)
	mysql.RegisterDialContext(dialer, func(ctx context.Context, addr string) (net.Conn, error) {
		return tunnel.Dial(ctx)
	})
	mysqlCfg := mysql.NewConfig()
	mysqlCfg.User = cfg.Username
	mysqlCfg.Passwd = password
	mysqlCfg.Net = dialer
	mysqlCfg.Addr = fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	mysqlCfg.DBName = cfg.Database
	if cfg.TLS {
		mysqlCfg.TLSConfig = "skip-verify"
	}
	if cfg.Params != "" {
		mysqlCfg.Params = parseParams(cfg.Params)
	}
	return sql.Open("mysql", mysqlCfg.FormatDSN())
}

func openPgWithTunnel(cfg *asset_entity.DatabaseConfig, password string, tunnel *SSHTunnel) (*sql.DB, error) {
	// pgx 支持通过 DialFunc 自定义连接方式
	_, dsn := buildDSN(cfg, password)
	// 对于隧道模式，使用 pgx 的 connector API
	db := sql.OpenDB(newPgTunnelConnector(dsn, tunnel))
	return db, nil
}

func buildDSN(cfg *asset_entity.DatabaseConfig, password string) (driverName string, dsn string) {
	switch cfg.Driver {
	case asset_entity.DriverMySQL:
		mysqlCfg := mysql.NewConfig()
		mysqlCfg.User = cfg.Username
		mysqlCfg.Passwd = password
		mysqlCfg.Net = "tcp"
		mysqlCfg.Addr = fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
		mysqlCfg.DBName = cfg.Database
		if cfg.TLS {
			mysqlCfg.TLSConfig = "skip-verify"
		}
		if cfg.Params != "" {
			mysqlCfg.Params = parseParams(cfg.Params)
		}
		return "mysql", mysqlCfg.FormatDSN()
	case asset_entity.DriverPostgreSQL:
		sslMode := cfg.SSLMode
		if sslMode == "" {
			sslMode = "disable"
		}
		dsn = fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
			url.QueryEscape(cfg.Username), url.QueryEscape(password),
			cfg.Host, cfg.Port, url.PathEscape(cfg.Database), sslMode)
		if cfg.Params != "" {
			dsn += "&" + cfg.Params
		}
		return "pgx", dsn
	default:
		return "", ""
	}
}

func setReadOnly(db *sql.DB, driver asset_entity.DatabaseDriver) error {
	switch driver {
	case asset_entity.DriverMySQL:
		_, err := db.Exec("SET SESSION TRANSACTION READ ONLY")
		return err
	case asset_entity.DriverPostgreSQL:
		_, err := db.Exec("SET default_transaction_read_only = on")
		return err
	}
	return nil
}

func parseParams(params string) map[string]string {
	m := make(map[string]string)
	values, err := url.ParseQuery(params)
	if err != nil {
		return m
	}
	for k, v := range values {
		if len(v) > 0 {
			m[k] = v[0]
		}
	}
	return m
}
