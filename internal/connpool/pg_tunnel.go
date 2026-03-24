package connpool

import (
	"context"
	"database/sql/driver"
	"net"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// pgTunnelConnector 通过 SSH 隧道连接 PostgreSQL 的 driver.Connector
type pgTunnelConnector struct {
	dsn    string
	tunnel *SSHTunnel
}

func newPgTunnelConnector(dsn string, tunnel *SSHTunnel) driver.Connector {
	return &pgTunnelConnector{dsn: dsn, tunnel: tunnel}
}

func (c *pgTunnelConnector) Connect(ctx context.Context) (driver.Conn, error) {
	connConfig, err := pgx.ParseConfig(c.dsn)
	if err != nil {
		return nil, err
	}
	connConfig.DialFunc = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return c.tunnel.Dial(ctx)
	}
	return stdlib.GetConnector(*connConfig).Connect(ctx)
}

func (c *pgTunnelConnector) Driver() driver.Driver {
	return stdlib.GetDefaultDriver()
}
