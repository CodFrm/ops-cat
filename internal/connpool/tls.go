package connpool

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

// TLSFields 描述构建 *tls.Config 所需的最小字段集，
// 由各资产类型 *Config 通过适配方法返回。
type TLSFields struct {
	ServerName string
	Insecure   bool
	CAFile     string
	CertFile   string
	KeyFile    string
}

// BuildTLSConfig 根据通用 TLS 字段构建 *tls.Config。
// name 用于错误消息（例如 "Kafka"、"Redis"），方便用户定位是哪种资产的 TLS 配置出错。
func BuildTLSConfig(name string, f TLSFields) (*tls.Config, error) {
	cfg := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		ServerName:         f.ServerName,
		InsecureSkipVerify: f.Insecure,
	}
	if f.CAFile != "" {
		ca, err := os.ReadFile(f.CAFile)
		if err != nil {
			return nil, fmt.Errorf("读取 %s TLS CA 证书失败: %w", name, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(ca) {
			return nil, fmt.Errorf("解析 %s TLS CA 证书失败", name)
		}
		cfg.RootCAs = pool
	}
	if f.CertFile != "" || f.KeyFile != "" {
		if f.CertFile == "" || f.KeyFile == "" {
			return nil, fmt.Errorf("%s TLS 客户端证书和私钥必须同时配置", name)
		}
		cert, err := tls.LoadX509KeyPair(f.CertFile, f.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("加载 %s TLS 客户端证书失败: %w", name, err)
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	return cfg, nil
}
