package credential_svc

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"
)

// Argon2id 参数（与 backup_svc 保持一致）
const (
	argon2Time    = 3
	argon2Memory  = 64 * 1024 // 64 MiB
	argon2Threads = 4
	argon2KeyLen  = 32 // AES-256
	saltLen       = 16
)

// CredentialSvc 凭证加解密服务（Argon2id + AES-256-GCM）
type CredentialSvc struct {
	gcm cipher.AEAD
}

// New 创建凭证服务，使用 Argon2id(masterKey, salt) 派生 AES-256 密钥
func New(masterKey string, salt []byte) *CredentialSvc {
	key := argon2.IDKey([]byte(masterKey), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(fmt.Sprintf("创建 AES cipher 失败: %v", err))
	}
	// 清除密钥材料
	for i := range key {
		key[i] = 0
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		panic(fmt.Sprintf("创建 GCM 失败: %v", err))
	}
	return &CredentialSvc{gcm: gcm}
}

// GenerateSalt 生成随机 salt
func GenerateSalt() ([]byte, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("生成 salt 失败: %w", err)
	}
	return salt, nil
}

// Encrypt 加密明文，返回 base64 编码的密文
func (s *CredentialSvc) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, s.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("生成 nonce 失败: %w", err)
	}
	ciphertext := s.gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt 解密 base64 编码的密文
func (s *CredentialSvc) Decrypt(ciphertextB64 string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return "", fmt.Errorf("base64 解码失败: %w", err)
	}
	nonceSize := s.gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("密文太短")
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := s.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("解密失败: %w", err)
	}
	return string(plaintext), nil
}

// 全局单例
var defaultSvc *CredentialSvc

// SetDefault 设置全局实例
func SetDefault(svc *CredentialSvc) {
	defaultSvc = svc
}

// Default 获取全局实例
func Default() *CredentialSvc {
	return defaultSvc
}
