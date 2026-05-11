package credential_mgr_svc

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/opskat/opskat/internal/model/entity/credential_entity"
	"github.com/opskat/opskat/internal/repository/credential_repo"
	"github.com/opskat/opskat/internal/service/credential_svc"

	gossh "golang.org/x/crypto/ssh"
)

// GenerateKeyRequest SSH 密钥生成请求
type GenerateKeyRequest struct {
	Name       string `json:"name"`
	Comment    string `json:"comment"`
	Username   string `json:"username"`   // 关联用户名（可选），用于资产表单自动填充
	KeyType    string `json:"keyType"`    // rsa, ed25519, ecdsa
	KeySize    int    `json:"keySize"`    // RSA: 2048/4096; ECDSA: 256/384/521; ED25519 忽略
	Passphrase string `json:"passphrase"` // 私钥密码（可选）
}

// CreatePasswordRequest 密码凭证创建请求
type CreatePasswordRequest struct {
	Name        string `json:"name"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	Description string `json:"description"`
}

// UpdateRequest 凭证更新请求
type UpdateRequest struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Comment     string `json:"comment"`     // SSH 密钥专用
	Description string `json:"description"` // 密码凭证专用
	Username    string `json:"username"`    // 关联用户名，SSH 密钥与密码凭证均可使用
}

// List 列出所有凭证
func List(ctx context.Context) ([]*credential_entity.Credential, error) {
	return credential_repo.Credential().List(ctx)
}

// ListByType 按类型列出凭证
func ListByType(ctx context.Context, credType string) ([]*credential_entity.Credential, error) {
	return credential_repo.Credential().ListByType(ctx, credType)
}

// Get 获取凭证
func Get(ctx context.Context, id int64) (*credential_entity.Credential, error) {
	return credential_repo.Credential().Find(ctx, id)
}

// GetDecryptedPassword 获取解密后的密码
func GetDecryptedPassword(ctx context.Context, id int64) (string, error) {
	cred, err := credential_repo.Credential().Find(ctx, id)
	if err != nil {
		return "", fmt.Errorf("凭证不存在: %w", err)
	}
	if !cred.IsPassword() {
		return "", fmt.Errorf("凭证类型不是密码")
	}
	plaintext, err := credential_svc.Default().Decrypt(cred.Password)
	if err != nil {
		return "", fmt.Errorf("解密密码失败: %w", err)
	}
	return plaintext, nil
}

// GetDecryptedPrivateKey 获取解密后的私钥 PEM
func GetDecryptedPrivateKey(ctx context.Context, id int64) (string, error) {
	cred, err := credential_repo.Credential().Find(ctx, id)
	if err != nil {
		return "", fmt.Errorf("凭证不存在: %w", err)
	}
	if !cred.IsSSHKey() {
		return "", fmt.Errorf("凭证类型不是 SSH 密钥")
	}
	plaintext, err := credential_svc.Default().Decrypt(cred.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("解密私钥失败: %w", err)
	}
	return plaintext, nil
}

// GetDecryptedPassphrase 获取解密后的私钥密码
func GetDecryptedPassphrase(ctx context.Context, id int64) (string, error) {
	cred, err := credential_repo.Credential().Find(ctx, id)
	if err != nil {
		return "", fmt.Errorf("凭证不存在: %w", err)
	}
	if !cred.IsSSHKey() {
		return "", fmt.Errorf("凭证类型不是 SSH 密钥")
	}
	if cred.Passphrase == "" {
		return "", nil // 无密码，返回空字符串
	}
	plaintext, err := credential_svc.Default().Decrypt(cred.Passphrase)
	if err != nil {
		return "", fmt.Errorf("解密 passphrase 失败: %w", err)
	}
	return plaintext, nil
}

// Delete 删除凭证
func Delete(ctx context.Context, id int64) error {
	return credential_repo.Credential().Delete(ctx, id)
}

// CreatePassword 创建密码凭证
func CreatePassword(ctx context.Context, req CreatePasswordRequest) (*credential_entity.Credential, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("凭证名称不能为空")
	}
	if req.Password == "" {
		return nil, fmt.Errorf("密码不能为空")
	}

	encryptedPassword, err := credential_svc.Default().Encrypt(req.Password)
	if err != nil {
		return nil, fmt.Errorf("加密密码失败: %w", err)
	}

	now := time.Now().Unix()
	cred := &credential_entity.Credential{
		Name:        req.Name,
		Type:        credential_entity.TypePassword,
		Username:    req.Username,
		Password:    encryptedPassword,
		Description: req.Description,
		Createtime:  now,
		Updatetime:  now,
	}

	if err := credential_repo.Credential().Create(ctx, cred); err != nil {
		return nil, fmt.Errorf("保存凭证失败: %w", err)
	}
	return cred, nil
}

// GenerateSSHKey 生成新的 SSH 密钥对
func GenerateSSHKey(ctx context.Context, req GenerateKeyRequest) (*credential_entity.Credential, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("密钥名称不能为空")
	}

	var privateKey crypto.PrivateKey
	var publicKeyStr string
	var fingerprint string

	switch req.KeyType {
	case credential_entity.KeyTypeRSA:
		if req.KeySize != 2048 && req.KeySize != 4096 {
			req.KeySize = 4096
		}
		rsaKey, err := rsa.GenerateKey(rand.Reader, req.KeySize)
		if err != nil {
			return nil, fmt.Errorf("生成 RSA 密钥失败: %w", err)
		}
		privateKey = rsaKey
		pub, err := gossh.NewPublicKey(&rsaKey.PublicKey)
		if err != nil {
			return nil, err
		}
		publicKeyStr = string(gossh.MarshalAuthorizedKey(pub))
		fingerprint = gossh.FingerprintSHA256(pub)

	case credential_entity.KeyTypeED25519:
		req.KeySize = 256
		pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("生成 ED25519 密钥失败: %w", err)
		}
		privateKey = privKey
		pub, err := gossh.NewPublicKey(pubKey)
		if err != nil {
			return nil, err
		}
		publicKeyStr = string(gossh.MarshalAuthorizedKey(pub))
		fingerprint = gossh.FingerprintSHA256(pub)

	case credential_entity.KeyTypeECDSA:
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
		ecdsaKey, err := ecdsa.GenerateKey(curve, rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("生成 ECDSA 密钥失败: %w", err)
		}
		privateKey = ecdsaKey
		pub, err := gossh.NewPublicKey(&ecdsaKey.PublicKey)
		if err != nil {
			return nil, err
		}
		publicKeyStr = string(gossh.MarshalAuthorizedKey(pub))
		fingerprint = gossh.FingerprintSHA256(pub)

	default:
		return nil, fmt.Errorf("不支持的密钥类型: %s", req.KeyType)
	}

	comment := req.Comment
	if comment == "" {
		comment = req.Name
	}
	publicKeyStr = appendComment(publicKeyStr, comment)

	// 根据 passphrase 是否存在生成私钥 PEM
	var privateKeyBlock *pem.Block
	var err error
	if req.Passphrase != "" {
		privateKeyBlock, err = gossh.MarshalPrivateKeyWithPassphrase(privateKey, comment, []byte(req.Passphrase))
	} else {
		privateKeyBlock, err = gossh.MarshalPrivateKey(privateKey, comment)
	}
	if err != nil {
		return nil, fmt.Errorf("序列化私钥失败: %w", err)
	}
	privateKeyPEM := pem.EncodeToMemory(privateKeyBlock)

	encryptedPrivateKey, err := credential_svc.Default().Encrypt(string(privateKeyPEM))
	if err != nil {
		return nil, fmt.Errorf("加密私钥失败: %w", err)
	}

	now := time.Now().Unix()
	cred := &credential_entity.Credential{
		Name:        req.Name,
		Type:        credential_entity.TypeSSHKey,
		Comment:     comment,
		Username:    req.Username,
		KeyType:     req.KeyType,
		KeySize:     req.KeySize,
		PrivateKey:  encryptedPrivateKey,
		PublicKey:   publicKeyStr,
		Fingerprint: fingerprint,
		Createtime:  now,
		Updatetime:  now,
	}

	// 如果提供了 passphrase，加密存储
	if req.Passphrase != "" {
		encryptedPassphrase, err := credential_svc.Default().Encrypt(req.Passphrase)
		if err != nil {
			return nil, fmt.Errorf("加密 passphrase 失败: %w", err)
		}
		cred.Passphrase = encryptedPassphrase
	}

	if err := credential_repo.Credential().Create(ctx, cred); err != nil {
		return nil, fmt.Errorf("保存密钥失败: %w", err)
	}
	return cred, nil
}

// ImportSSHKeyFromFile 从文件导入私钥
func ImportSSHKeyFromFile(ctx context.Context, name, comment, filePath, passphrase, username string) (*credential_entity.Credential, error) {
	if name == "" {
		return nil, fmt.Errorf("密钥名称不能为空")
	}

	data, err := os.ReadFile(filePath) //nolint:gosec // file path from user config
	if err != nil {
		return nil, fmt.Errorf("读取密钥文件失败: %w", err)
	}

	return ImportSSHKeyFromPEM(ctx, name, comment, string(data), passphrase, username)
}

// ImportSSHKeyFromPEM 从 PEM 字符串导入私钥
func ImportSSHKeyFromPEM(ctx context.Context, name, comment, pemData, passphrase, username string) (*credential_entity.Credential, error) {
	if name == "" {
		return nil, fmt.Errorf("密钥名称不能为空")
	}

	var signer gossh.Signer
	var err error

	// 先尝试无 passphrase 解析
	signer, err = gossh.ParsePrivateKey([]byte(pemData))
	if err != nil {
		// 如果失败且提供了 passphrase，尝试带 passphrase 解析
		if passphrase != "" {
			signer, err = gossh.ParsePrivateKeyWithPassphrase([]byte(pemData), []byte(passphrase))
			if err != nil {
				return nil, fmt.Errorf("解析私钥失败（可能 passphrase 不正确）: %w", err)
			}
		} else {
			// 无 passphrase 但解析失败，可能密钥有加密
			if strings.Contains(err.Error(), "password protected") || strings.Contains(err.Error(), "encrypted") {
				return nil, fmt.Errorf("私钥已加密，请提供 passphrase")
			}
			return nil, fmt.Errorf("解析私钥失败: %w", err)
		}
	}

	pub := signer.PublicKey()
	publicKeyStr := string(gossh.MarshalAuthorizedKey(pub))
	fingerprint := gossh.FingerprintSHA256(pub)

	if comment == "" {
		comment = name
	}
	publicKeyStr = appendComment(publicKeyStr, comment)

	keyType, keySize := detectKeyTypeAndSize(signer)

	encryptedPrivateKey, err := credential_svc.Default().Encrypt(pemData)
	if err != nil {
		return nil, fmt.Errorf("加密私钥失败: %w", err)
	}

	// 如果提供了 passphrase，加密后存储
	var encryptedPassphrase string
	if passphrase != "" {
		encryptedPassphrase, err = credential_svc.Default().Encrypt(passphrase)
		if err != nil {
			return nil, fmt.Errorf("加密 passphrase 失败: %w", err)
		}
	}

	now := time.Now().Unix()
	cred := &credential_entity.Credential{
		Name:        name,
		Type:        credential_entity.TypeSSHKey,
		Comment:     comment,
		Username:    username,
		KeyType:     keyType,
		KeySize:     keySize,
		PrivateKey:  encryptedPrivateKey,
		Passphrase:  encryptedPassphrase,
		PublicKey:   publicKeyStr,
		Fingerprint: fingerprint,
		Createtime:  now,
		Updatetime:  now,
	}

	if err := credential_repo.Credential().Create(ctx, cred); err != nil {
		return nil, fmt.Errorf("保存密钥失败: %w", err)
	}
	return cred, nil
}

// Update 更新凭证
func Update(ctx context.Context, req UpdateRequest) (*credential_entity.Credential, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("凭证名称不能为空")
	}

	cred, err := credential_repo.Credential().Find(ctx, req.ID)
	if err != nil {
		return nil, fmt.Errorf("凭证不存在: %w", err)
	}

	cred.Name = req.Name
	cred.Username = req.Username
	cred.Updatetime = time.Now().Unix()

	if cred.IsSSHKey() {
		comment := req.Comment
		if comment == "" {
			comment = req.Name
		}
		// 更新公钥中的 comment
		if cred.Comment != comment {
			parts := strings.SplitN(strings.TrimSpace(cred.PublicKey), " ", 3)
			if len(parts) >= 2 {
				cred.PublicKey = parts[0] + " " + parts[1] + " " + comment + "\n"
			}
		}
		cred.Comment = comment
	} else {
		cred.Description = req.Description
	}

	if err := credential_repo.Credential().Update(ctx, cred); err != nil {
		return nil, fmt.Errorf("更新凭证失败: %w", err)
	}
	return cred, nil
}

// UpdatePassword 更新密码凭证的密码
func UpdatePassword(ctx context.Context, id int64, password string) error {
	if password == "" {
		return fmt.Errorf("密码不能为空")
	}

	cred, err := credential_repo.Credential().Find(ctx, id)
	if err != nil {
		return fmt.Errorf("凭证不存在: %w", err)
	}
	if !cred.IsPassword() {
		return fmt.Errorf("凭证类型不是密码")
	}

	encryptedPassword, err := credential_svc.Default().Encrypt(password)
	if err != nil {
		return fmt.Errorf("加密密码失败: %w", err)
	}

	cred.Password = encryptedPassword
	cred.Updatetime = time.Now().Unix()

	return credential_repo.Credential().Update(ctx, cred)
}

// UpdatePassphrase 更新 SSH 密钥的 passphrase
// 需要同时重新加密 PEM 内容，否则旧 passphrase 加密的 PEM 无法用新 passphrase 解密
func UpdatePassphrase(ctx context.Context, id int64, oldPassphrase, newPassphrase string) error {
	cred, err := credential_repo.Credential().Find(ctx, id)
	if err != nil {
		return fmt.Errorf("凭证不存在: %w", err)
	}
	if !cred.IsSSHKey() {
		return fmt.Errorf("凭证类型不是 SSH 密钥")
	}

	// 1. 解密存储的 PrivateKey (AES 解密得到 PEM)
	decryptedPEM, err := credential_svc.Default().Decrypt(cred.PrivateKey)
	if err != nil {
		return fmt.Errorf("解密私钥存储失败: %w", err)
	}

	// 2. 用旧 passphrase 解析 PEM 得到 crypto.PrivateKey (interface{})
	var privateKey crypto.PrivateKey
	if oldPassphrase != "" {
		privateKey, err = gossh.ParseRawPrivateKeyWithPassphrase([]byte(decryptedPEM), []byte(oldPassphrase))
	} else {
		privateKey, err = gossh.ParseRawPrivateKey([]byte(decryptedPEM))
	}
	if err != nil {
		return fmt.Errorf("解析私钥失败（旧 passphrase 可能不正确）: %w", err)
	}

	// 3. 重新 marshal 私钥，使用新的 passphrase
	comment := cred.Comment
	if comment == "" {
		comment = cred.Name
	}

	var privateKeyBlock *pem.Block
	if newPassphrase != "" {
		privateKeyBlock, err = gossh.MarshalPrivateKeyWithPassphrase(privateKey, comment, []byte(newPassphrase))
	} else {
		privateKeyBlock, err = gossh.MarshalPrivateKey(privateKey, comment)
	}
	if err != nil {
		return fmt.Errorf("重新序列化私钥失败: %w", err)
	}
	newPEM := pem.EncodeToMemory(privateKeyBlock)

	// 4. 加密新 PEM 和新 passphrase
	encryptedPrivateKey, err := credential_svc.Default().Encrypt(string(newPEM))
	if err != nil {
		return fmt.Errorf("加密私钥失败: %w", err)
	}

	var encryptedPassphrase string
	if newPassphrase != "" {
		encryptedPassphrase, err = credential_svc.Default().Encrypt(newPassphrase)
		if err != nil {
			return fmt.Errorf("加密 passphrase 失败: %w", err)
		}
	}

	// 5. 更新凭证
	cred.PrivateKey = encryptedPrivateKey
	cred.Passphrase = encryptedPassphrase
	cred.Updatetime = time.Now().Unix()

	return credential_repo.Credential().Update(ctx, cred)
}

// appendComment 在公钥末尾追加 comment
func appendComment(publicKey, comment string) string {
	trimmed := strings.TrimSpace(publicKey)
	return trimmed + " " + comment + "\n"
}

// detectKeyTypeAndSize 根据 signer 推断密钥类型和大小
func detectKeyTypeAndSize(signer gossh.Signer) (string, int) {
	pub := signer.PublicKey()
	switch pub.Type() {
	case "ssh-rsa":
		return credential_entity.KeyTypeRSA, 0
	case "ssh-ed25519":
		return credential_entity.KeyTypeED25519, 256
	case "ecdsa-sha2-nistp256":
		return credential_entity.KeyTypeECDSA, 256
	case "ecdsa-sha2-nistp384":
		return credential_entity.KeyTypeECDSA, 384
	case "ecdsa-sha2-nistp521":
		return credential_entity.KeyTypeECDSA, 521
	default:
		return pub.Type(), 0
	}
}
