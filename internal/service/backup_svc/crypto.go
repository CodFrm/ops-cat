package backup_svc

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"golang.org/x/crypto/argon2"
)

// Argon2id 参数
const (
	argon2Time    = 3
	argon2Memory  = 64 * 1024 // 64 MiB
	argon2Threads = 4
	argon2KeyLen  = 32 // AES-256
	saltLen       = 16
)

// EncryptedBackup 加密备份信封
type EncryptedBackup struct {
	Format     string           `json:"format"`
	Version    int              `json:"version"`
	KDF        KDFParams        `json:"kdf"`
	Encryption EncryptionParams `json:"encryption"`
	Ciphertext string           `json:"ciphertext"`
}

// KDFParams 密钥派生参数
type KDFParams struct {
	Algorithm string `json:"algorithm"`
	Time      uint32 `json:"time"`
	Memory    uint32 `json:"memory"`
	Threads   uint8  `json:"threads"`
	Salt      string `json:"salt"`
}

// EncryptionParams 加密参数
type EncryptionParams struct {
	Algorithm string `json:"algorithm"`
	Nonce     string `json:"nonce"`
}

// EncryptBackup 使用密码加密备份数据
func EncryptBackup(plainJSON []byte, password string) ([]byte, error) {
	// 生成随机 salt
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("生成 salt 失败: %w", err)
	}

	// Argon2id 派生密钥
	key := argon2.IDKey([]byte(password), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)
	defer func() {
		for i := range key {
			key[i] = 0
		}
	}()

	// AES-256-GCM 加密
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("创建 AES cipher 失败: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("创建 GCM 失败: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("生成 nonce 失败: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plainJSON, nil)

	// 构建信封
	envelope := EncryptedBackup{
		Format:  "opskat-encrypted-backup",
		Version: 1,
		KDF: KDFParams{
			Algorithm: "argon2id",
			Time:      argon2Time,
			Memory:    argon2Memory,
			Threads:   argon2Threads,
			Salt:      base64.StdEncoding.EncodeToString(salt),
		},
		Encryption: EncryptionParams{
			Algorithm: "aes-256-gcm",
			Nonce:     base64.StdEncoding.EncodeToString(nonce),
		},
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
	}

	return json.MarshalIndent(envelope, "", "  ")
}

// DecryptBackup 使用密码解密备份数据
func DecryptBackup(envelopeJSON []byte, password string) ([]byte, error) {
	var envelope EncryptedBackup
	if err := json.Unmarshal(envelopeJSON, &envelope); err != nil {
		return nil, fmt.Errorf("解析加密信封失败: %w", err)
	}

	if envelope.Version != 1 {
		return nil, fmt.Errorf("不支持的加密版本: %d", envelope.Version)
	}

	salt, err := base64.StdEncoding.DecodeString(envelope.KDF.Salt)
	if err != nil {
		return nil, fmt.Errorf("解码 salt 失败: %w", err)
	}
	nonce, err := base64.StdEncoding.DecodeString(envelope.Encryption.Nonce)
	if err != nil {
		return nil, fmt.Errorf("解码 nonce 失败: %w", err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(envelope.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("解码密文失败: %w", err)
	}

	// 使用存储的 KDF 参数派生密钥
	key := argon2.IDKey([]byte(password), salt, envelope.KDF.Time, envelope.KDF.Memory, envelope.KDF.Threads, argon2KeyLen)
	defer func() {
		for i := range key {
			key[i] = 0
		}
	}()

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("创建 AES cipher 失败: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("创建 GCM 失败: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("解密失败，密码错误或数据损坏")
	}

	return plaintext, nil
}

// IsEncryptedBackup 检测数据是否为加密备份
func IsEncryptedBackup(data []byte) bool {
	var probe struct {
		Format string `json:"format"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return false
	}
	return probe.Format == "opskat-encrypted-backup"
}
