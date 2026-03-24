package credential_svc

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"
)

const (
	keychainService = "ops-cat"
	keychainAccount = "master-key"
	masterKeyLen    = 32 // 256-bit
	masterKeyFile   = "master.key"
)

// ResolveMasterKey 按优先级获取 master key:
//  1. 传入的 explicit（CLI --master-key / 环境变量）
//  2. OS Keychain
//  3. 文件回退 (<dataDir>/master.key)
//
// 如果所有来源都没有，自动生成并存储。
func ResolveMasterKey(explicit string, dataDir string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}

	// 尝试从 Keychain 读取
	key, err := keyring.Get(keychainService, keychainAccount)
	if err == nil && key != "" {
		return key, nil
	}

	// 尝试从文件读取
	filePath := filepath.Join(dataDir, masterKeyFile)
	data, err := os.ReadFile(filePath) //nolint:gosec // path from app data directory
	if err == nil && len(data) > 0 {
		key = string(data)
		// 尝试同步到 Keychain（best-effort）
		_ = keyring.Set(keychainService, keychainAccount, key)
		return key, nil
	}

	// 自动生成新的 master key
	key, err = generateMasterKey()
	if err != nil {
		return "", fmt.Errorf("生成 master key 失败: %w", err)
	}

	// 存储到 Keychain
	if err := keyring.Set(keychainService, keychainAccount, key); err != nil {
		// Keychain 不可用，回退到文件存储
		if writeErr := os.WriteFile(filePath, []byte(key), 0600); writeErr != nil {
			return "", fmt.Errorf("存储 master key 失败（Keychain: %v, 文件: %w）", err, writeErr)
		}
	}

	return key, nil
}

// generateMasterKey 生成 32 字节随机密钥，返回 base64 编码
func generateMasterKey() (string, error) {
	buf := make([]byte, masterKeyLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}
