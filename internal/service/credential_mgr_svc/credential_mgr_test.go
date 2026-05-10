package credential_mgr_svc

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"sync"
	"testing"

	"github.com/opskat/opskat/internal/model/entity/credential_entity"
	"github.com/opskat/opskat/internal/repository/credential_repo"
	"github.com/opskat/opskat/internal/service/credential_svc"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	gossh "golang.org/x/crypto/ssh"
)

// TestPassphraseReEncrypt 测试 passphrase 重新加密的核心逻辑
// 这是 UpdatePassphrase 的核心逻辑单元测试
func TestPassphraseReEncrypt(t *testing.T) {
	Convey("Passphrase 重新加密逻辑", t, func() {
		// 1. 生成一个测试密钥
		pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
		assert.NoError(t, err)

		// 获取公钥用于验证
		sshPubKey, err := gossh.NewPublicKey(pubKey)
		assert.NoError(t, err)

		Convey("用 passphrase 加密 PEM", func() {
			oldPassphrase := "old-secret-123"
			comment := "test-key"

			// Marshal with old passphrase
			block, err := gossh.MarshalPrivateKeyWithPassphrase(privKey, comment, []byte(oldPassphrase))
			assert.NoError(t, err)
			pemBytes := pem.EncodeToMemory(block)

			Convey("用正确的旧 passphrase 解密成功", func() {
				signer, err := gossh.ParsePrivateKeyWithPassphrase(pemBytes, []byte(oldPassphrase))
				assert.NoError(t, err)
				assert.Equal(t, sshPubKey.Type(), signer.PublicKey().Type())
			})

			Convey("用错误的旧 passphrase 解密失败", func() {
				_, err := gossh.ParsePrivateKeyWithPassphrase(pemBytes, []byte("wrong-passphrase"))
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "decrypt")
			})

			Convey("重新加密流程", func() {
				// Step 1: Parse with old passphrase
				rawKey, err := gossh.ParseRawPrivateKeyWithPassphrase(pemBytes, []byte(oldPassphrase))
				assert.NoError(t, err)

				// Step 2: Re-marshal with new passphrase
				newPassphrase := "new-secret-456"
				newBlock, err := gossh.MarshalPrivateKeyWithPassphrase(rawKey.(crypto.PrivateKey), comment, []byte(newPassphrase))
				assert.NoError(t, err)
				newPemBytes := pem.EncodeToMemory(newBlock)

				// Step 3: Verify new passphrase works
				newSigner, err := gossh.ParsePrivateKeyWithPassphrase(newPemBytes, []byte(newPassphrase))
				assert.NoError(t, err)
				assert.Equal(t, sshPubKey.Type(), newSigner.PublicKey().Type())

				// Step 4: Verify old passphrase no longer works
				_, err = gossh.ParsePrivateKeyWithPassphrase(newPemBytes, []byte(oldPassphrase))
				assert.Error(t, err)
			})

			Convey("移除 passphrase（重新加密为无密码）", func() {
				// Parse with old passphrase
				rawKey, err := gossh.ParseRawPrivateKeyWithPassphrase(pemBytes, []byte(oldPassphrase))
				assert.NoError(t, err)

				// Re-marshal without passphrase
				newBlock, err := gossh.MarshalPrivateKey(rawKey.(crypto.PrivateKey), comment)
				assert.NoError(t, err)
				newPemBytes := pem.EncodeToMemory(newBlock)

				// Verify: can parse without passphrase
				newSigner, err := gossh.ParsePrivateKey(newPemBytes)
				assert.NoError(t, err)
				assert.Equal(t, sshPubKey.Type(), newSigner.PublicKey().Type())

				// Verify: ParsePrivateKeyWithPassphrase returns error because key is not encrypted
				// This is expected behavior - the key is no longer password protected
				_, err = gossh.ParsePrivateKeyWithPassphrase(newPemBytes, []byte("any-passphrase"))
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "not password protected")
			})
		})

		Convey("无 passphrase 的 PEM", func() {
			comment := "test-key-unencrypted"

			// Marshal without passphrase
			block, err := gossh.MarshalPrivateKey(privKey, comment)
			assert.NoError(t, err)
			pemBytes := pem.EncodeToMemory(block)

			Convey("直接解析成功（无 passphrase）", func() {
				signer, err := gossh.ParsePrivateKey(pemBytes)
				assert.NoError(t, err)
				assert.Equal(t, sshPubKey.Type(), signer.PublicKey().Type())
			})

			Convey("添加 passphrase", func() {
				// Parse without passphrase
				rawKey, err := gossh.ParseRawPrivateKey(pemBytes)
				assert.NoError(t, err)

				// Re-marshal with passphrase
				newPassphrase := "new-passphrase-789"
				newBlock, err := gossh.MarshalPrivateKeyWithPassphrase(rawKey.(crypto.PrivateKey), comment, []byte(newPassphrase))
				assert.NoError(t, err)
				newPemBytes := pem.EncodeToMemory(newBlock)

				// Verify: now requires passphrase
				signer, err := gossh.ParsePrivateKeyWithPassphrase(newPemBytes, []byte(newPassphrase))
				assert.NoError(t, err)
				assert.Equal(t, sshPubKey.Type(), signer.PublicKey().Type())

				// Verify: cannot parse without passphrase
				_, err = gossh.ParsePrivateKey(newPemBytes)
				assert.Error(t, err)
			})
		})
	})
}

// fakeCredentialRepo: 内存实现，仅用于测试。Create 累积保存到 creds slice 用于断言。
type fakeCredentialRepo struct {
	mu     sync.Mutex
	creds  []*credential_entity.Credential
	nextID int64
}

func (r *fakeCredentialRepo) Find(_ context.Context, id int64) (*credential_entity.Credential, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.creds {
		if c.ID == id {
			return c, nil
		}
	}
	return nil, fmt.Errorf("not found")
}
func (r *fakeCredentialRepo) List(_ context.Context) ([]*credential_entity.Credential, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*credential_entity.Credential(nil), r.creds...), nil
}
func (r *fakeCredentialRepo) ListByType(_ context.Context, t string) ([]*credential_entity.Credential, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*credential_entity.Credential
	for _, c := range r.creds {
		if c.Type == t {
			out = append(out, c)
		}
	}
	return out, nil
}
func (r *fakeCredentialRepo) Create(_ context.Context, cred *credential_entity.Credential) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	cred.ID = r.nextID
	r.creds = append(r.creds, cred)
	return nil
}
func (r *fakeCredentialRepo) Update(_ context.Context, cred *credential_entity.Credential) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, c := range r.creds {
		if c.ID == cred.ID {
			r.creds[i] = cred
			return nil
		}
	}
	return fmt.Errorf("not found")
}
func (r *fakeCredentialRepo) Delete(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, c := range r.creds {
		if c.ID == id {
			r.creds = append(r.creds[:i], r.creds[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("not found")
}

func setupCredentialTestEnv(t *testing.T) *fakeCredentialRepo {
	t.Helper()
	credential_svc.SetDefault(credential_svc.New("test-master-key-1234567890abcdef", []byte("test-salt-16byte")))
	repo := &fakeCredentialRepo{}
	credential_repo.RegisterCredential(repo)
	return repo
}

func TestGenerateSSHKey_PersistsUsername(t *testing.T) {
	Convey("GenerateSSHKey 写入 username 字段", t, func() {
		repo := setupCredentialTestEnv(t)
		ctx := context.Background()

		Convey("提供 username", func() {
			cred, err := GenerateSSHKey(ctx, GenerateKeyRequest{
				Name:     "test-key",
				KeyType:  credential_entity.KeyTypeED25519,
				Username: "alice",
			})
			assert.NoError(t, err)
			assert.Equal(t, "alice", cred.Username)
			assert.Len(t, repo.creds, 1)
			assert.Equal(t, "alice", repo.creds[0].Username)
		})

		Convey("username 留空保持空字符串", func() {
			cred, err := GenerateSSHKey(ctx, GenerateKeyRequest{
				Name:    "test-key-2",
				KeyType: credential_entity.KeyTypeED25519,
			})
			assert.NoError(t, err)
			assert.Equal(t, "", cred.Username)
			assert.Len(t, repo.creds, 1)
			assert.Equal(t, "", repo.creds[0].Username)
		})
	})
}

func TestImportSSHKeyFromPEM_PersistsUsername(t *testing.T) {
	Convey("ImportSSHKeyFromPEM 写入 username 字段", t, func() {
		repo := setupCredentialTestEnv(t)
		ctx := context.Background()

		// 用 gossh 现场生成一个无 passphrase 的 ed25519 PEM，避免 fixture
		_, privKey, err := ed25519.GenerateKey(rand.Reader)
		assert.NoError(t, err)
		block, err := gossh.MarshalPrivateKey(privKey, "test")
		assert.NoError(t, err)
		pemData := string(pem.EncodeToMemory(block))

		cred, err := ImportSSHKeyFromPEM(ctx, "imported", "comment", pemData, "", "bob")
		assert.NoError(t, err)
		assert.Equal(t, "bob", cred.Username)
		assert.Len(t, repo.creds, 1)
		assert.Equal(t, "bob", repo.creds[0].Username)
	})
}

// TestCredentialTypeCheck 测试凭证类型检查
func TestCredentialTypeCheck(t *testing.T) {
	Convey("凭证类型检查", t, func() {
		Convey("SSH 密钥类型", func() {
			cred := &credential_entity.Credential{
				Name:       "test-key",
				Type:       credential_entity.TypeSSHKey,
				PrivateKey: "priv",
				PublicKey:  "pub",
				KeyType:    credential_entity.KeyTypeED25519,
			}
			So(cred.IsSSHKey(), ShouldBeTrue)
			So(cred.IsPassword(), ShouldBeFalse)
		})

		Convey("密码类型", func() {
			cred := &credential_entity.Credential{
				Name:     "test-password",
				Type:     credential_entity.TypePassword,
				Password: "secret",
			}
			So(cred.IsSSHKey(), ShouldBeFalse)
			So(cred.IsPassword(), ShouldBeTrue)
		})
	})
}
