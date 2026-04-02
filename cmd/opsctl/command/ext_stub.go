//go:build no_wasm

package command

import (
	"encoding/json"
	"fmt"
	"os"
)

// localExtExec is a stub when building without WASM support.
// Only delegate mode (desktop app running) is available.
func localExtExec(extName string, toolName string, toolArgs json.RawMessage) int {
	fmt.Fprintf(os.Stderr, "Error: local extension execution not available (built without WASM support)\n")
	fmt.Fprintf(os.Stderr, "Hint: start the desktop app and try again — delegate mode will be used automatically\n")
	return 1
}
