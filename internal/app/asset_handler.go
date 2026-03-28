package app

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// ExtensionAssetHandler 处理 /extensions/* 路径的静态文件请求
type ExtensionAssetHandler struct {
	extensionsDir string
}

// NewExtensionAssetHandler 创建扩展文件服务 handler
func NewExtensionAssetHandler(extensionsDir string) *ExtensionAssetHandler {
	return &ExtensionAssetHandler{extensionsDir: extensionsDir}
}

// ServeHTTP 处理请求：仅服务 /extensions/ 前缀的路径
func (h *ExtensionAssetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/extensions/") {
		http.NotFound(w, r)
		return
	}

	// 提取相对路径并清理
	rel := strings.TrimPrefix(r.URL.Path, "/extensions/")
	rel = filepath.Clean(rel)

	// 安全检查：防止路径穿越
	if strings.Contains(rel, "..") {
		http.NotFound(w, r)
		return
	}

	// 映射规则：/extensions/{extName}/frontend/* → {extensionsDir}/{extName}/dist/frontend/*
	parts := strings.SplitN(rel, "/", 2)
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	extName := parts[0]
	subPath := parts[1]

	filePath := filepath.Join(h.extensionsDir, extName, "dist", subPath)
	filePath = filepath.Clean(filePath)

	// 确保路径在 extensions 目录内
	if !strings.HasPrefix(filePath, filepath.Clean(h.extensionsDir)) {
		http.NotFound(w, r)
		return
	}

	// 检查文件是否存在
	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	// 设置正确的 Content-Type
	if strings.HasSuffix(filePath, ".js") {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	} else if strings.HasSuffix(filePath, ".css") {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	}

	http.ServeFile(w, r, filePath)
}
