package credential_svc

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

func testSalt() []byte {
	return []byte("test-salt-16byte")
}

func TestCredentialSvc_EncryptDecrypt(t *testing.T) {
	convey.Convey("凭证加解密", t, func() {
		svc := New("test-master-key-1234567890abcdef", testSalt())

		convey.Convey("加密后解密应返回原文", func() {
			plain := "my-secret-password"
			encrypted, err := svc.Encrypt(plain)
			assert.NoError(t, err)
			assert.NotEqual(t, plain, encrypted)

			decrypted, err := svc.Decrypt(encrypted)
			assert.NoError(t, err)
			assert.Equal(t, plain, decrypted)
		})

		convey.Convey("空字符串加解密", func() {
			encrypted, err := svc.Encrypt("")
			assert.NoError(t, err)

			decrypted, err := svc.Decrypt(encrypted)
			assert.NoError(t, err)
			assert.Equal(t, "", decrypted)
		})

		convey.Convey("同一明文每次加密结果不同（随机 nonce）", func() {
			plain := "same-password"
			enc1, _ := svc.Encrypt(plain)
			enc2, _ := svc.Encrypt(plain)
			assert.NotEqual(t, enc1, enc2)
		})

		convey.Convey("错误密钥解密失败", func() {
			svc2 := New("wrong-master-key-xxxxxxxxxxxxxxxxx", testSalt())
			encrypted, _ := svc.Encrypt("secret")

			_, err := svc2.Decrypt(encrypted)
			assert.Error(t, err)
		})

		convey.Convey("不同 salt 解密失败", func() {
			svc2 := New("test-master-key-1234567890abcdef", []byte("different-salt!!"))
			encrypted, _ := svc.Encrypt("secret")

			_, err := svc2.Decrypt(encrypted)
			assert.Error(t, err)
		})

		convey.Convey("无效密文解密失败", func() {
			_, err := svc.Decrypt("not-valid-base64!!!")
			assert.Error(t, err)
		})
	})
}

func TestGenerateSalt(t *testing.T) {
	convey.Convey("生成 salt", t, func() {
		salt, err := GenerateSalt()
		assert.NoError(t, err)
		assert.Len(t, salt, saltLen)

		convey.Convey("每次生成的 salt 不同", func() {
			salt2, err := GenerateSalt()
			assert.NoError(t, err)
			assert.NotEqual(t, salt, salt2)
		})
	})
}
