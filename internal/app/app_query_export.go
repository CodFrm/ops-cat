package app

import (
	"fmt"
	"os"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// SelectTableExportFile opens a native save dialog for table exports.
func (a *App) SelectTableExportFile(defaultFilename, filterName, pattern string) (string, error) {
	if filterName == "" {
		filterName = "Export Files"
	}
	if pattern == "" {
		pattern = "*.*"
	}
	filePath, err := wailsRuntime.SaveFileDialog(a.ctx, wailsRuntime.SaveDialogOptions{
		Title:           "Export Data",
		DefaultFilename: defaultFilename,
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: filterName, Pattern: pattern},
		},
	})
	if err != nil {
		return "", fmt.Errorf("save file dialog failed: %w", err)
	}
	return filePath, nil
}

// WriteTableExportFile writes exported table content to the path chosen by the user.
func (a *App) WriteTableExportFile(filePath, content string) error {
	if filePath == "" {
		return fmt.Errorf("export file path is empty")
	}
	return os.WriteFile(filePath, []byte(content), 0644)
}
