package connpool

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/cago-frame/cago/pkg/logger"
	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/sshpool"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
)

// MongoClientCloser wraps *mongo.Client to satisfy io.Closer.
type MongoClientCloser struct {
	*mongo.Client
}

// Close disconnects the underlying MongoDB client.
func (m *MongoClientCloser) Close() error {
	return m.Disconnect(context.Background())
}

// mongoTunnelDialer implements options.ContextDialer for SSH tunnel routing.
type mongoTunnelDialer struct {
	tunnel *SSHTunnel
}

func (d *mongoTunnelDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return d.tunnel.Dial(ctx)
}

// DialMongoDB 创建 MongoDB 连接（直连或通过 SSH 隧道）
// password 为已解析的明文密码，由调用方负责解密
func DialMongoDB(ctx context.Context, asset *asset_entity.Asset, cfg *asset_entity.MongoDBConfig, password string, sshPool *sshpool.Pool) (*mongo.Client, io.Closer, error) {
	var uri string
	if cfg.ConnectionURI != "" {
		uri = injectPassword(cfg.ConnectionURI, password)
	} else {
		uri = buildMongoURI(cfg, password)
	}

	clientOpts := options.Client().ApplyURI(uri)

	if cfg.TLS {
		clientOpts.SetTLSConfig(&tls.Config{})
	}

	var tunnel *SSHTunnel
	tunnelID := asset.SSHTunnelID
	if tunnelID == 0 {
		tunnelID = cfg.SSHAssetID // backward compat
	}
	if tunnelID > 0 && sshPool != nil {
		var host string
		var port int
		var err error
		if cfg.ConnectionURI != "" {
			host, port, err = parseHostFromURI(cfg.ConnectionURI)
			if err != nil {
				return nil, nil, fmt.Errorf("解析 MongoDB URI 失败: %w", err)
			}
		} else {
			host = cfg.Host
			port = cfg.Port
		}
		tunnel = NewSSHTunnel(tunnelID, host, port, sshPool)
		clientOpts.SetDialer(&mongoTunnelDialer{tunnel: tunnel})
		// 禁止副本集发现，强制直连，避免驱动尝试连接副本集其他节点
		clientOpts.SetDirect(true)
	}

	client, err := mongo.Connect(clientOpts)
	if err != nil {
		if tunnel != nil {
			if closeErr := tunnel.Close(); closeErr != nil {
				logger.Default().Warn("close ssh tunnel", zap.Error(closeErr))
			}
		}
		return nil, nil, fmt.Errorf("MongoDB 连接失败: %w", err)
	}

	if pingErr := client.Ping(ctx, nil); pingErr != nil {
		if disconnectErr := client.Disconnect(context.Background()); disconnectErr != nil {
			logger.Default().Warn("disconnect mongodb client", zap.Error(disconnectErr))
		}
		if tunnel != nil {
			if closeErr := tunnel.Close(); closeErr != nil {
				logger.Default().Warn("close ssh tunnel", zap.Error(closeErr))
			}
		}
		return nil, nil, fmt.Errorf("MongoDB 连接失败: %w", pingErr)
	}

	if tunnel == nil {
		return client, nil, nil
	}
	return client, tunnel, nil
}

// buildMongoURI 从 MongoDBConfig 字段组装 MongoDB URI
func buildMongoURI(cfg *asset_entity.MongoDBConfig, password string) string {
	host := cfg.Host
	if host == "" {
		host = "localhost"
	}
	port := cfg.Port
	if port == 0 {
		port = 27017
	}

	var userInfo string
	if cfg.Username != "" {
		userInfo = url.QueryEscape(cfg.Username) + ":" + url.QueryEscape(password) + "@"
	}

	uri := fmt.Sprintf("mongodb://%s%s:%d", userInfo, host, port)

	if cfg.Database != "" {
		uri += "/" + url.PathEscape(cfg.Database)
	}

	params := url.Values{}
	if cfg.AuthSource != "" {
		params.Set("authSource", cfg.AuthSource)
	}
	if cfg.ReplicaSet != "" {
		params.Set("replicaSet", cfg.ReplicaSet)
	}
	if len(params) > 0 {
		if cfg.Database == "" {
			uri += "/"
		}
		uri += "?" + params.Encode()
	}

	return uri
}

// injectPassword 将密码注入已有 URI（仅在 URI 包含用户名且密码为空时注入）
func injectPassword(uri, password string) string {
	if password == "" {
		return uri
	}
	parsed, err := url.Parse(uri)
	if err != nil || parsed.User == nil {
		return uri
	}
	existing, _ := parsed.User.Password()
	if existing != "" {
		// URI 中已有密码，不覆盖
		return uri
	}
	username := parsed.User.Username()
	parsed.User = url.UserPassword(username, password)
	return parsed.String()
}

// parseHostFromURI 从 MongoDB URI 中提取 host 和 port（仅取第一个主机）
func parseHostFromURI(uri string) (string, int, error) {
	parsed, err := url.Parse(uri)
	if err != nil {
		return "", 0, fmt.Errorf("无效的 MongoDB URI: %w", err)
	}

	hostPort := parsed.Host
	// 对于 mongodb+srv 或 replica set，只取第一个主机
	if idx := strings.IndexByte(hostPort, ','); idx >= 0 {
		hostPort = hostPort[:idx]
	}

	hostStr, portStr, splitErr := net.SplitHostPort(hostPort)
	if splitErr != nil {
		// SplitHostPort 失败说明没有端口部分，使用默认 27017
		return hostPort, 27017, nil //nolint:nilerr // no port in host is expected, not an error
	}

	portNum, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, fmt.Errorf("无效的端口号 %q: %w", portStr, err)
	}

	return hostStr, portNum, nil
}
