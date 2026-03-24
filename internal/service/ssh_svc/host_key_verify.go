package ssh_svc

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"time"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"

	"github.com/opskat/opskat/internal/model/entity/host_key_entity"
	"github.com/opskat/opskat/internal/repository/host_key_repo"

	"golang.org/x/crypto/ssh"
	"gorm.io/gorm"
)

// HostKeyAction 主机密钥校验操作
type HostKeyAction int

const (
	HostKeyAcceptAndSave HostKeyAction = iota // 接受并记住
	HostKeyAcceptOnce                         // 仅本次接受
	HostKeyReject                             // 取消/拒绝
)

// HostKeyEvent 主机密钥校验事件
type HostKeyEvent struct {
	Host           string `json:"host"`
	Port           int    `json:"port"`
	KeyType        string `json:"keyType"`
	Fingerprint    string `json:"fingerprint"`
	IsChanged      bool   `json:"isChanged"`      // true=密钥已变更（危险）
	OldFingerprint string `json:"oldFingerprint"` // 变更时的旧指纹
}

// HostKeyVerifyFunc 主机密钥校验回调，由调用方实现不同的交互方式
type HostKeyVerifyFunc func(event HostKeyEvent) HostKeyAction

// MakeHostKeyCallback 创建 SSH HostKeyCallback
func MakeHostKeyCallback(host string, port int, verifyFn HostKeyVerifyFunc) ssh.HostKeyCallback {
	if verifyFn == nil {
		return ssh.InsecureIgnoreHostKey()
	}

	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		fingerprint := ssh.FingerprintSHA256(key)
		keyType := key.Type()
		pubKeyBase64 := base64.StdEncoding.EncodeToString(key.Marshal())
		now := time.Now().Unix()

		ctx := context.Background()
		stored, err := host_key_repo.HostKey().FindByHostPort(ctx, host, port)
		if err != nil && err != gorm.ErrRecordNotFound {
			// 数据库错误，回退到询问用户（视为首次连接）
			stored = nil
		}

		if stored != nil {
			// 已有记录：检查是否匹配
			if stored.PublicKey == pubKeyBase64 {
				// 密钥匹配，更新 last_seen
				stored.LastSeen = now
				if err := host_key_repo.HostKey().Upsert(ctx, stored); err != nil {
					logger.Default().Warn("upsert host key", zap.String("host", host), zap.Int("port", port), zap.Error(err))
				}
				return nil
			}

			// 密钥变更！
			action := verifyFn(HostKeyEvent{
				Host:           host,
				Port:           port,
				KeyType:        keyType,
				Fingerprint:    fingerprint,
				IsChanged:      true,
				OldFingerprint: stored.Fingerprint,
			})

			switch action {
			case HostKeyAcceptAndSave:
				stored.KeyType = keyType
				stored.PublicKey = pubKeyBase64
				stored.Fingerprint = fingerprint
				stored.LastSeen = now
				if err := host_key_repo.HostKey().Upsert(ctx, stored); err != nil {
					logger.Default().Warn("upsert host key", zap.String("host", host), zap.Int("port", port), zap.Error(err))
				}
				return nil
			case HostKeyAcceptOnce:
				return nil
			default:
				return fmt.Errorf("主机密钥已变更，连接被用户拒绝 (host=%s:%d)", host, port)
			}
		}

		// 首次连接
		action := verifyFn(HostKeyEvent{
			Host:        host,
			Port:        port,
			KeyType:     keyType,
			Fingerprint: fingerprint,
			IsChanged:   false,
		})

		switch action {
		case HostKeyAcceptAndSave:
			newKey := &host_key_entity.HostKey{
				Host:        host,
				Port:        port,
				KeyType:     keyType,
				PublicKey:   pubKeyBase64,
				Fingerprint: fingerprint,
				FirstSeen:   now,
				LastSeen:    now,
			}
			if err := host_key_repo.HostKey().Upsert(ctx, newKey); err != nil {
					logger.Default().Warn("upsert new host key", zap.String("host", host), zap.Int("port", port), zap.Error(err))
				}
			return nil
		case HostKeyAcceptOnce:
			return nil
		default:
			return fmt.Errorf("首次连接被用户拒绝 (host=%s:%d)", host, port)
		}
	}
}

// AutoTrustFirstRejectChangeVerifyFunc AI agent 使用：首次自动信任，变更拒绝
func AutoTrustFirstRejectChangeVerifyFunc() HostKeyVerifyFunc {
	return func(event HostKeyEvent) HostKeyAction {
		if event.IsChanged {
			return HostKeyReject
		}
		return HostKeyAcceptAndSave
	}
}
