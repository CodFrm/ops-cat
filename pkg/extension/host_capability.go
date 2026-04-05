// pkg/extension/host_capability.go
package extension

import "encoding/json"

// capHost decorates a HostProvider with per-call capability enforcement.
type capHost struct {
	inner    HostProvider
	manifest *Manifest
	extDir   string
}

// NewCapabilityHost wraps inner with capability enforcement.
func NewCapabilityHost(inner HostProvider, manifest *Manifest, extDir string) HostProvider {
	return &capHost{inner: inner, manifest: manifest, extDir: extDir}
}

func (c *capHost) IOOpen(params IOOpenParams) (uint32, IOMeta, error) {
	switch params.Type {
	case "file":
		switch params.Mode {
		case "read":
			if err := c.manifest.CheckFSRead(params.Path, c.extDir); err != nil {
				return 0, IOMeta{}, err
			}
		case "write":
			if err := c.manifest.CheckFSWrite(params.Path, c.extDir); err != nil {
				return 0, IOMeta{}, err
			}
		}
	case "http":
		if err := c.manifest.CheckHTTPURL(params.URL, c.manifest.Capabilities.Tunnel); err != nil {
			return 0, IOMeta{}, err
		}
		// Pass tunnel capability to dial-time guard.
		params.AllowPrivate = c.manifest.Capabilities.Tunnel
	}
	return c.inner.IOOpen(params)
}

// Delegate all other HostProvider methods unchanged.
func (c *capHost) IORead(handleID uint32, size int) ([]byte, error) {
	return c.inner.IORead(handleID, size)
}
func (c *capHost) IOWrite(handleID uint32, data []byte) (int, error) {
	return c.inner.IOWrite(handleID, data)
}
func (c *capHost) IOFlush(handleID uint32) (*IOMeta, error) { return c.inner.IOFlush(handleID) }
func (c *capHost) IOClose(handleID uint32) error            { return c.inner.IOClose(handleID) }
func (c *capHost) GetAssetConfig(assetID int64) (json.RawMessage, error) {
	return c.inner.GetAssetConfig(assetID)
}
func (c *capHost) FileDialog(dialogType string, opts DialogOptions) (string, error) {
	return c.inner.FileDialog(dialogType, opts)
}
func (c *capHost) Log(level, msg string) { c.inner.Log(level, msg) }
func (c *capHost) KVGet(key string) ([]byte, error) {
	return c.inner.KVGet(key)
}
func (c *capHost) KVSet(key string, value []byte) error { return c.inner.KVSet(key, value) }
func (c *capHost) ActionEvent(eventType string, data json.RawMessage) error {
	return c.inner.ActionEvent(eventType, data)
}
func (c *capHost) CloseAll() { c.inner.CloseAll() }
