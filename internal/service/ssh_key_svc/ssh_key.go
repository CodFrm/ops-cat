package ssh_key_svc

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/opskat/opskat/internal/model/entity/ssh_key_entity"
	"github.com/opskat/opskat/internal/repository/ssh_key_repo"
	"github.com/opskat/opskat/internal/service/credential_svc"

	gossh "golang.org/x/crypto/ssh"
)

// GenerateRequest 密钥生成请求
type GenerateRequest struct {
	Name    string `json:"name"`
	Comment string `json:"comment"`
	KeyType string `json:"keyType"` // rsa, ed25519, ecdsa
	KeySize int    `json:"keySize"` // RSA: 2048/4096; ECDSA: 256/384/521; ED25519 忽略
}

// UpdateRequest 密钥更新请求
type UpdateRequest struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Comment string `json:"comment"`
}

// List 列出所有 SSH 密钥
func List(ctx context.Context) ([]*ssh_key_entity.SSHKey, error) {
	return ssh_key_repo.SSHKey().List(ctx)
}

// Get 获取 SSH 密钥
func Get(ctx context.Context, id int64) (*ssh_key_entity.SSHKey, error) {
	return ssh_key_repo.SSHKey().Find(ctx, id)
}

// GetPrivateKey 获取解密后的私钥 PEM
func GetPrivateKey(ctx context.Context, id int64) (string, error) {
	key, err := ssh_key_repo.SSHKey().Find(ctx, id)
	if err != nil {
		return "", fmt.Errorf("密钥不存在: %w", err)
	}
	plaintext, err := credential_svc.Default().Decrypt(key.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("解密私钥失败: %w", err)
	}
	return plaintext, nil
}

// Delete 删除 SSH 密钥
func Delete(ctx context.Context, id int64) error {
	return ssh_key_repo.SSHKey().Delete(ctx, id)
}

// Generate 生成新的 SSH 密钥对
func Generate(ctx context.Context, req GenerateRequest) (*ssh_key_entity.SSHKey, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("密钥名称不能为空")
	}

	var privateKeyPEM []byte
	var publicKeyStr string
	var fingerprint string

	switch req.KeyType {
	case ssh_key_entity.KeyTypeRSA:
		if req.KeySize != 2048 && req.KeySize != 4096 {
			req.KeySize = 4096
		}
		privateKey, err := rsa.GenerateKey(rand.Reader, req.KeySize)
		if err != nil {
			return nil, fmt.Errorf("生成 RSA 密钥失败: %w", err)
		}
		privateKeyPEM = pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
		})
		pub, err := gossh.NewPublicKey(&privateKey.PublicKey)
		if err != nil {
			return nil, err
		}
		publicKeyStr = string(gossh.MarshalAuthorizedKey(pub))
		fingerprint = gossh.FingerprintSHA256(pub)

	case ssh_key_entity.KeyTypeED25519:
		req.KeySize = 256
		pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("生成 ED25519 密钥失败: %w", err)
		}
		privBytes, err := x509.MarshalPKCS8PrivateKey(privKey)
		if err != nil {
			return nil, err
		}
		privateKeyPEM = pem.EncodeToMemory(&pem.Block{
			Type:  "PRIVATE KEY",
			Bytes: privBytes,
		})
		pub, err := gossh.NewPublicKey(pubKey)
		if err != nil {
			return nil, err
		}
		publicKeyStr = string(gossh.MarshalAuthorizedKey(pub))
		fingerprint = gossh.FingerprintSHA256(pub)

	case ssh_key_entity.KeyTypeECDSA:
		var curve elliptic.Curve
		switch req.KeySize {
		case 384:
			curve = elliptic.P384()
		case 521:
			curve = elliptic.P521()
		default:
			req.KeySize = 256
			curve = elliptic.P256()
		}
		privateKey, err := ecdsa.GenerateKey(curve, rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("生成 ECDSA 密钥失败: %w", err)
		}
		privBytes, err := x509.MarshalECPrivateKey(privateKey)
		if err != nil {
			return nil, err
		}
		privateKeyPEM = pem.EncodeToMemory(&pem.Block{
			Type:  "EC PRIVATE KEY",
			Bytes: privBytes,
		})
		pub, err := gossh.NewPublicKey(&privateKey.PublicKey)
		if err != nil {
			return nil, err
		}
		publicKeyStr = string(gossh.MarshalAuthorizedKey(pub))
		fingerprint = gossh.FingerprintSHA256(pub)

	default:
		return nil, fmt.Errorf("不支持的密钥类型: %s", req.KeyType)
	}

	// comment 为空则用 name
	comment := req.Comment
	if comment == "" {
		comment = req.Name
	}
	publicKeyStr = appendComment(publicKeyStr, comment)

	// 加密私钥
	encryptedPrivateKey, err := credential_svc.Default().Encrypt(string(privateKeyPEM))
	if err != nil {
		return nil, fmt.Errorf("加密私钥失败: %w", err)
	}

	now := time.Now().Unix()
	key := &ssh_key_entity.SSHKey{
		Name:        req.Name,
		Comment:     comment,
		KeyType:     req.KeyType,
		KeySize:     req.KeySize,
		PrivateKey:  encryptedPrivateKey,
		PublicKey:   publicKeyStr,
		Fingerprint: fingerprint,
		Createtime:  now,
		Updatetime:  now,
	}

	if err := ssh_key_repo.SSHKey().Create(ctx, key); err != nil {
		return nil, fmt.Errorf("保存密钥失败: %w", err)
	}
	return key, nil
}

// ImportFromFile 从文件导入私钥
func ImportFromFile(ctx context.Context, name, comment, filePath string) (*ssh_key_entity.SSHKey, error) {
	if name == "" {
		return nil, fmt.Errorf("密钥名称不能为空")
	}

	data, err := os.ReadFile(filePath) //nolint:gosec // file path from user config
	if err != nil {
		return nil, fmt.Errorf("读取密钥文件失败: %w", err)
	}

	return ImportFromPEM(ctx, name, comment, string(data))
}

// ImportFromPEM 从 PEM 字符串导入私钥
func ImportFromPEM(ctx context.Context, name, comment, pemData string) (*ssh_key_entity.SSHKey, error) {
	if name == "" {
		return nil, fmt.Errorf("密钥名称不能为空")
	}

	// 解析私钥以验证格式并提取公钥
	signer, err := gossh.ParsePrivateKey([]byte(pemData))
	if err != nil {
		return nil, fmt.Errorf("解析私钥失败: %w", err)
	}

	pub := signer.PublicKey()
	publicKeyStr := string(gossh.MarshalAuthorizedKey(pub))
	fingerprint := gossh.FingerprintSHA256(pub)

	// comment 为空则用 name
	if comment == "" {
		comment = name
	}
	publicKeyStr = appendComment(publicKeyStr, comment)

	// 推断密钥类型和大小
	keyType, keySize := detectKeyTypeAndSize(signer)

	// 加密私钥
	encryptedPrivateKey, err := credential_svc.Default().Encrypt(pemData)
	if err != nil {
		return nil, fmt.Errorf("加密私钥失败: %w", err)
	}

	now := time.Now().Unix()
	key := &ssh_key_entity.SSHKey{
		Name:        name,
		Comment:     comment,
		KeyType:     keyType,
		KeySize:     keySize,
		PrivateKey:  encryptedPrivateKey,
		PublicKey:   publicKeyStr,
		Fingerprint: fingerprint,
		Createtime:  now,
		Updatetime:  now,
	}

	if err := ssh_key_repo.SSHKey().Create(ctx, key); err != nil {
		return nil, fmt.Errorf("保存密钥失败: %w", err)
	}
	return key, nil
}

// Update 更新 SSH 密钥名称和注释
func Update(ctx context.Context, req UpdateRequest) (*ssh_key_entity.SSHKey, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("密钥名称不能为空")
	}

	key, err := ssh_key_repo.SSHKey().Find(ctx, req.ID)
	if err != nil {
		return nil, fmt.Errorf("密钥不存在: %w", err)
	}

	comment := req.Comment
	if comment == "" {
		comment = req.Name
	}

	// 如果 comment 变了，需要更新公钥中的 comment 部分
	if key.Comment != comment {
		// 去掉旧的 comment，重新追加新的
		parts := strings.SplitN(strings.TrimSpace(key.PublicKey), " ", 3)
		if len(parts) >= 2 {
			key.PublicKey = parts[0] + " " + parts[1] + " " + comment + "\n"
		}
	}

	key.Name = req.Name
	key.Comment = comment
	key.Updatetime = time.Now().Unix()

	if err := ssh_key_repo.SSHKey().Update(ctx, key); err != nil {
		return nil, fmt.Errorf("更新密钥失败: %w", err)
	}
	return key, nil
}

// appendComment 在公钥末尾追加 comment
func appendComment(publicKey, comment string) string {
	// gossh.MarshalAuthorizedKey 输出格式: "type base64\n"
	// 我们需要变成: "type base64 comment\n"
	trimmed := strings.TrimSpace(publicKey)
	return trimmed + " " + comment + "\n"
}

// detectKeyTypeAndSize 根据 signer 推断密钥类型和大小
func detectKeyTypeAndSize(signer gossh.Signer) (string, int) {
	pub := signer.PublicKey()
	switch pub.Type() {
	case "ssh-rsa":
		// 无法直接获取位数，默认标记为 0
		return ssh_key_entity.KeyTypeRSA, 0
	case "ssh-ed25519":
		return ssh_key_entity.KeyTypeED25519, 256
	case "ecdsa-sha2-nistp256":
		return ssh_key_entity.KeyTypeECDSA, 256
	case "ecdsa-sha2-nistp384":
		return ssh_key_entity.KeyTypeECDSA, 384
	case "ecdsa-sha2-nistp521":
		return ssh_key_entity.KeyTypeECDSA, 521
	default:
		return pub.Type(), 0
	}
}
