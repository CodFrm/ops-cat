package import_svc

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"golang.org/x/crypto/pbkdf2"
)

const (
	pbkdfIterations = 100000
	cryptKeyLength  = 32
)

// tabbyStoredVault Tabby vault 存储结构
type tabbyStoredVault struct {
	Version  int    `yaml:"version"`
	Contents string `yaml:"contents"` // base64 编码的密文
	KeySalt  string `yaml:"keySalt"`  // hex 编码的 PBKDF2 salt
	IV       string `yaml:"iv"`       // hex 编码的 AES IV
}

// tabbyVault 解密后的 vault 内容
type tabbyVault struct {
	Config  json.RawMessage `json:"config"`
	Secrets []tabbySecret   `json:"secrets"`
}

// tabbySecret vault 中的单条密钥
type tabbySecret struct {
	Type  string          `json:"type"`
	Key   json.RawMessage `json:"key"` // 可能是 string 或 object
	Value string          `json:"value"`
}

// secretKey 获取 secret 的 key（profile UUID）
// key 可能是纯字符串，也可能是 {"id": "...", "description": "..."} 对象
func (s *tabbySecret) secretKey() string {
	// 先尝试解析为字符串
	var str string
	if err := json.Unmarshal(s.Key, &str); err == nil {
		return str
	}
	// 尝试解析为对象
	var obj struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(s.Key, &obj); err == nil {
		return obj.ID
	}
	return ""
}

// decryptTabbyVault 解密 Tabby vault
// 使用 PBKDF2 (SHA-512, 100000 次迭代) 派生密钥，AES-256-CBC 解密
func decryptTabbyVault(vault *tabbyStoredVault, passphrase string) (*tabbyVault, error) {
	if vault == nil || vault.Contents == "" {
		return nil, fmt.Errorf("vault 内容为空")
	}

	salt, err := hex.DecodeString(vault.KeySalt)
	if err != nil {
		return nil, fmt.Errorf("解析 vault salt 失败: %w", err)
	}

	iv, err := hex.DecodeString(vault.IV)
	if err != nil {
		return nil, fmt.Errorf("解析 vault IV 失败: %w", err)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(vault.Contents)
	if err != nil {
		return nil, fmt.Errorf("解析 vault 密文失败: %w", err)
	}

	// PBKDF2 派生密钥
	key := pbkdf2.Key([]byte(passphrase), salt, pbkdfIterations, cryptKeyLength, sha512.New)

	// AES-256-CBC 解密
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("创建 AES cipher 失败: %w", err)
	}

	if len(ciphertext) < aes.BlockSize {
		return nil, fmt.Errorf("密文长度不足")
	}
	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("密文长度不是块大小的整数倍")
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	plaintext := make([]byte, len(ciphertext))
	mode.CryptBlocks(plaintext, ciphertext)

	// 去除 PKCS7 padding
	plaintext, err = pkcs7Unpad(plaintext)
	if err != nil {
		return nil, fmt.Errorf("vault 密码错误或数据损坏: %w", err)
	}

	var result tabbyVault
	if err := json.Unmarshal(plaintext, &result); err != nil {
		return nil, fmt.Errorf("vault 密码错误或数据损坏: %w", err)
	}

	return &result, nil
}

// vaultSecretInfo 包含密码类型信息
type vaultSecretInfo struct {
	Type  string
	Value string
}

// buildVaultSecretMap 从解密后的 vault 构建 profileID → secretInfo 映射
func buildVaultSecretMap(vault *tabbyVault) map[string]vaultSecretInfo {
	secrets := make(map[string]vaultSecretInfo)
	for _, secret := range vault.Secrets {
		if secret.Type == "ssh:password" || secret.Type == "ssh:key-passphrase" {
			key := secret.secretKey()
			if key != "" && secret.Value != "" {
				secrets[key] = vaultSecretInfo{Type: secret.Type, Value: secret.Value}
			}
		}
	}
	return secrets
}

// buildVaultPasswordMap 从解密后的 vault 构建 profileID → password 映射（向后兼容）
func buildVaultPasswordMap(vault *tabbyVault) map[string]string {
	secrets := buildVaultSecretMap(vault)
	passwords := make(map[string]string)
	for k, v := range secrets {
		passwords[k] = v.Value
	}
	return passwords
}

// pkcs7Unpad 去除 PKCS7 填充
func pkcs7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("数据为空")
	}
	padLen := int(data[len(data)-1])
	if padLen == 0 || padLen > aes.BlockSize || padLen > len(data) {
		return nil, fmt.Errorf("无效的 padding")
	}
	for i := len(data) - padLen; i < len(data); i++ {
		if data[i] != byte(padLen) {
			return nil, fmt.Errorf("无效的 padding")
		}
	}
	return data[:len(data)-padLen], nil
}
