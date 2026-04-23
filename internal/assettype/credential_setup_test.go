package assettype

import (
	"os"
	"testing"

	"github.com/opskat/opskat/internal/service/credential_svc"
)

// TestMain 初始化 credential_svc，使各 handler 中的 password 加密路径可在单测中执行。
func TestMain(m *testing.M) {
	credential_svc.SetDefault(credential_svc.New("test-master-key-1234567890abcdef", []byte("test-salt-16byte")))
	os.Exit(m.Run())
}
