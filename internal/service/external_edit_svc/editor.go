package external_edit_svc

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/opskat/opskat/internal/bootstrap"
)

func (s *Service) detectEditors(customEditors []bootstrap.ExternalEditorConfig, defaultID string) []Editor {
	editors := make([]Editor, 0, 8)
	seen := make(map[string]struct{})

	for _, editor := range builtInEditors() {
		if _, err := validateExecutable(editor.Path); err == nil {
			editor.Available = true
		}
		editor.Default = editor.ID == defaultID
		editors = append(editors, editor)
		seen[editor.ID] = struct{}{}
	}

	for _, editor := range customEditors {
		id := strings.TrimSpace(editor.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		available := validateCustomEditor(editor) == nil
		editors = append(editors, Editor{
			ID:        id,
			Name:      strings.TrimSpace(editor.Name),
			Path:      strings.TrimSpace(editor.Path),
			Args:      cloneArgs(editor.Args),
			BuiltIn:   false,
			Available: available,
			Default:   id == defaultID,
		})
		seen[id] = struct{}{}
	}

	sort.SliceStable(editors, func(i, j int) bool {
		if editors[i].Available != editors[j].Available {
			return editors[i].Available
		}
		if editors[i].BuiltIn != editors[j].BuiltIn {
			return editors[i].BuiltIn
		}
		return editors[i].Name < editors[j].Name
	})
	return editors
}

func (s *Service) resolveEditor(requestedID string) (*Editor, error) {
	settings, err := s.GetSettings()
	if err != nil {
		return nil, err
	}
	targetID := strings.TrimSpace(requestedID)
	if targetID == "" {
		targetID = settings.DefaultEditorID
	}
	if targetID == "" {
		targetID = firstAvailableEditorID(settings.Editors)
	}
	for _, editor := range settings.Editors {
		if editor.ID != targetID {
			continue
		}
		if !editor.Available {
			return nil, fmt.Errorf("外部编辑器不可用: %s", editor.Name)
		}
		return &editor, nil
	}
	return nil, fmt.Errorf("未找到外部编辑器配置")
}

func (s *Service) normalizeCustomEditors(customEditors []bootstrap.ExternalEditorConfig) ([]bootstrap.ExternalEditorConfig, error) {
	normalized := make([]bootstrap.ExternalEditorConfig, 0, len(customEditors))
	seenNames := make(map[string]struct{})
	seenPaths := make(map[string]struct{})
	seenIDs := make(map[string]struct{})

	for _, editor := range builtInEditors() {
		if editor.ID != "" {
			seenIDs[editor.ID] = struct{}{}
		}
		if name := strings.TrimSpace(editor.Name); name != "" {
			seenNames[strings.ToLower(name)] = struct{}{}
		}
		if path := strings.TrimSpace(editor.Path); path != "" {
			seenPaths[strings.ToLower(path)] = struct{}{}
		}
	}

	for idx, editor := range customEditors {
		editor.ID = strings.TrimSpace(editor.ID)
		editor.Name = strings.TrimSpace(editor.Name)
		editor.Path = strings.TrimSpace(editor.Path)
		editor.Args = trimArgs(editor.Args)
		if editor.ID == "" {
			editor.ID = fmt.Sprintf("custom-%d", idx+1)
		}
		if editor.Name == "" {
			return nil, fmt.Errorf("自定义编辑器名称不能为空")
		}
		if editor.Path == "" {
			return nil, fmt.Errorf("自定义编辑器路径不能为空")
		}
		if _, ok := seenIDs[editor.ID]; ok {
			return nil, fmt.Errorf("存在重复的编辑器 ID: %s", editor.ID)
		}
		if _, ok := seenNames[strings.ToLower(editor.Name)]; ok {
			return nil, fmt.Errorf("存在重复的编辑器名称: %s", editor.Name)
		}
		if _, ok := seenPaths[strings.ToLower(editor.Path)]; ok {
			return nil, fmt.Errorf("存在重复的编辑器路径: %s", editor.Path)
		}
		if err := validateCustomEditor(editor); err != nil {
			return nil, err
		}
		seenIDs[editor.ID] = struct{}{}
		seenNames[strings.ToLower(editor.Name)] = struct{}{}
		seenPaths[strings.ToLower(editor.Path)] = struct{}{}
		normalized = append(normalized, editor)
	}

	return normalized, nil
}

func builtInEditors() []Editor {
	switch {
	case isWindows():
		windir := os.Getenv("WINDIR")
		if windir == "" {
			windir = `C:\Windows`
		}
		localAppData := os.Getenv("LOCALAPPDATA")
		programFiles := os.Getenv("ProgramFiles")
		programFilesX86 := os.Getenv("ProgramFiles(x86)")
		return []Editor{
			{
				ID:      "cursor",
				Name:    "Cursor",
				Path:    firstExistingPath([]string{filepath.Join(localAppData, "Programs", "Cursor", "Cursor.exe"), filepath.Join(programFiles, "Cursor", "Cursor.exe")}),
				BuiltIn: true,
			},
			{
				ID:      "vscode",
				Name:    "VS Code",
				Path:    firstExistingPath([]string{filepath.Join(localAppData, "Programs", "Microsoft VS Code", "Code.exe"), filepath.Join(programFiles, "Microsoft VS Code", "Code.exe"), filepath.Join(programFilesX86, "Microsoft VS Code", "Code.exe")}),
				BuiltIn: true,
			},
			{
				ID:      "typora",
				Name:    "Typora",
				Path:    firstExistingPath([]string{filepath.Join(localAppData, "Programs", "Typora", "Typora.exe"), filepath.Join(programFiles, "Typora", "Typora.exe"), filepath.Join(programFilesX86, "Typora", "Typora.exe")}),
				BuiltIn: true,
			},
			{
				ID:      "system-text",
				Name:    "System Text Editor",
				Path:    filepath.Join(windir, "System32", "notepad.exe"),
				BuiltIn: true,
			},
		}
	default:
		return []Editor{
			{
				ID:      "cursor",
				Name:    "Cursor",
				Path:    firstExistingPath([]string{"/Applications/Cursor.app/Contents/MacOS/Cursor", "/usr/bin/cursor"}),
				BuiltIn: true,
			},
			{
				ID:      "vscode",
				Name:    "VS Code",
				Path:    firstExistingPath([]string{"/Applications/Visual Studio Code.app/Contents/MacOS/Electron", "/usr/bin/code"}),
				BuiltIn: true,
			},
			{
				ID:      "typora",
				Name:    "Typora",
				Path:    firstExistingPath([]string{"/Applications/Typora.app/Contents/MacOS/Typora", "/usr/bin/typora"}),
				BuiltIn: true,
			},
			{
				ID:      "system-text",
				Name:    "System Text Editor",
				Path:    firstExistingPath([]string{"/usr/bin/open", "/usr/bin/xdg-open", "/bin/xdg-open"}),
				Args:    nil,
				BuiltIn: true,
			},
		}
	}
}

func validateCustomEditor(editor bootstrap.ExternalEditorConfig) error {
	if _, err := validateExecutable(editor.Path); err != nil {
		return err
	}
	return nil
}

func validateExecutable(execPath string) (string, error) {
	execPath = strings.TrimSpace(execPath)
	if execPath == "" {
		return "", fmt.Errorf("编辑器路径不能为空")
	}
	if !filepath.IsAbs(execPath) {
		return "", fmt.Errorf("编辑器路径必须是绝对路径")
	}
	ext := strings.ToLower(filepath.Ext(execPath))
	if ext == ".bat" || ext == ".cmd" {
		return "", fmt.Errorf("不允许使用 .bat 或 .cmd 作为外部编辑器")
	}
	info, err := os.Stat(execPath)
	if err != nil {
		return "", fmt.Errorf("外部编辑器不可访问: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("外部编辑器路径不能是目录")
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("外部编辑器路径必须是常规文件")
	}
	if isWindows() {
		if ext != ".exe" {
			return "", fmt.Errorf("仅支持 .exe 格式的 Windows 外部编辑器")
		}
		return execPath, nil
	}
	if info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("外部编辑器缺少执行权限")
	}
	return execPath, nil
}

func containsAvailableEditor(editors []Editor, editorID string) bool {
	for _, editor := range editors {
		if editor.ID == editorID && editor.Available {
			return true
		}
	}
	return false
}

func containsEditorID(editors []Editor, editorID string) bool {
	for _, editor := range editors {
		if editor.ID == editorID {
			return true
		}
	}
	return false
}

func firstAvailableEditorID(editors []Editor) string {
	for _, editor := range editors {
		if editor.Available {
			return editor.ID
		}
	}
	return ""
}

func firstExistingPath(paths []string) string {
	for _, candidate := range paths {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		if !filepath.IsAbs(candidate) {
			continue
		}
		if _, err := os.Stat(candidate); err == nil { //nolint:gosec // built-in candidates are static absolute paths
			return candidate
		}
	}
	return ""
}
