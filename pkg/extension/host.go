// pkg/extension/host.go
package extension

import "encoding/json"

// HostProvider defines the capabilities that the host provides to extensions.
// Main App and DevServer each provide their own implementation.
type HostProvider interface {
	IOOpen(params IOOpenParams) (uint32, IOMeta, error)
	IORead(handleID uint32, size int) ([]byte, error)
	IOWrite(handleID uint32, data []byte) (int, error)
	IOFlush(handleID uint32) (*IOMeta, error)
	IOClose(handleID uint32) error
	GetAssetConfig(assetID int64) (json.RawMessage, error)
	FileDialog(dialogType string, opts DialogOptions) (string, error)
	Log(level, msg string)
	KVGet(key string) ([]byte, error)
	KVSet(key string, value []byte) error
	ActionEvent(eventType string, data json.RawMessage) error
	CloseAll()
}

type IOOpenParams struct {
	Type    string            `json:"type"`
	Path    string            `json:"path"`
	Mode    string            `json:"mode"`
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

type DialogOptions struct {
	Title       string   `json:"title"`
	DefaultName string   `json:"defaultName"`
	Filters     []string `json:"filters"`
}
