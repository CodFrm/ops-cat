package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/mark3labs/mcp-go/server"
)

// MCPServer 管理 MCP Server 生命周期
type MCPServer struct {
	mcpServer  *server.MCPServer
	httpServer *server.StreamableHTTPServer
	listener   net.Listener
	configDir  string   // MCP 配置文件存放目录（应用数据目录）
	configFiles []string // 生成的配置文件路径列表，Stop 时清理
}

// mcpInstructions MCP Server 级别的指令，告知 AI 整体能力和工作流
const mcpInstructions = `You are connected to Ops Cat, a desktop application for managing remote server assets via SSH.

## Core Concepts
- **Asset**: A remote server (currently SSH only) with connection info (host, port, username, auth method). Each asset has a unique numeric ID.
- **Group**: An organizational unit for assets, supporting nested hierarchies via parent_id.
- **Credentials**: SSH passwords and keys are encrypted and stored in the app. All tools resolve credentials automatically — you never need to provide passwords or keys.

## Typical Workflow
1. Use list_assets or list_groups to discover available servers and their organization.
2. Use get_asset to inspect a specific server's connection details.
3. Use run_command to execute shell commands on a remote server (diagnostics, deployment, maintenance).
4. Use upload_file / download_file to transfer files via SFTP.
5. Use add_asset / update_asset to manage the asset inventory.

## Important
- run_command executes on the **remote server**, not locally.
- All operations requiring asset_id expect an ID from list_assets. Always list first if you don't know the ID.
- Credentials are resolved automatically from the app's encrypted store — do not ask the user for passwords.`

// NewMCPServer 创建并注册所有工具的 MCP Server
func NewMCPServer() *MCPServer {
	mcpSrv := server.NewMCPServer("ops-cat", "1.0.0",
		server.WithInstructions(mcpInstructions),
	)
	RegisterToMCP(mcpSrv, AllToolDefs())
	return &MCPServer{
		mcpServer: mcpSrv,
	}
}

// Start 启动 MCP Server，监听 127.0.0.1 随机端口，生成 CLI 配置文件
// configDir 为应用数据目录，MCP 配置文件写入此目录
func (s *MCPServer) Start(ctx context.Context, configDir string) error {
	s.configDir = configDir

	// 监听随机端口
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("MCP Server 监听失败: %w", err)
	}
	s.listener = listener

	// 生成 CLI MCP 配置文件到应用数据目录
	if err := s.writeConfigFiles(); err != nil {
		listener.Close()
		return fmt.Errorf("生成 MCP 配置失败: %w", err)
	}

	// 启动 HTTP Server，用 mux 正确路由 /mcp 路径
	s.httpServer = server.NewStreamableHTTPServer(s.mcpServer)
	mux := http.NewServeMux()
	mux.Handle("/mcp", s.httpServer)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	httpSrv := &http.Server{Handler: mux}
	go func() {
		log.Printf("MCP HTTP server starting on %s", s.listener.Addr().String())
		if err := httpSrv.Serve(s.listener); err != nil && err != http.ErrServerClosed {
			log.Printf("MCP HTTP server error: %v", err)
		}
		log.Printf("MCP HTTP server goroutine exited")
	}()

	// 自检：确认 HTTP server 真正在接受连接
	selfCheckURL := fmt.Sprintf("http://%s/health", s.listener.Addr().String())
	resp, err := http.Get(selfCheckURL)
	if err != nil {
		log.Printf("MCP Server 自检失败: %v", err)
		// 不 return error，让服务继续尝试运行
	} else {
		resp.Body.Close()
		log.Printf("MCP Server 自检通过，URL: %s", s.URL())
	}

	return nil
}

// Stop 停止 MCP Server 并清理配置文件
func (s *MCPServer) Stop() {
	if s.listener != nil {
		s.listener.Close()
		s.listener = nil
	}
	// 清理生成的配置文件
	for _, f := range s.configFiles {
		os.Remove(f)
	}
	s.configFiles = nil
}

// URL 返回 MCP Server 的 URL
func (s *MCPServer) URL() string {
	if s.listener == nil {
		return ""
	}
	return fmt.Sprintf("http://%s/mcp", s.listener.Addr().String())
}

// ConfigDir 返回 MCP 配置文件所在目录（作为 CLI 工作目录）
func (s *MCPServer) ConfigDir() string {
	return s.configDir
}

// WriteConfigToDir 将 MCP 配置文件写入指定目录（切换会话时使用）
func (s *MCPServer) WriteConfigToDir(workDir string) error {
	url := s.URL()
	if url == "" {
		return fmt.Errorf("MCP Server 未启动")
	}

	// Claude CLI: .mcp.json
	claudeConfig := map[string]any{
		"mcpServers": map[string]any{
			"ops-cat": map[string]any{
				"type": "http",
				"url":  url,
			},
		},
	}
	data, _ := json.MarshalIndent(claudeConfig, "", "  ")
	if err := os.WriteFile(filepath.Join(workDir, ".mcp.json"), data, 0644); err != nil {
		return err
	}

	// Codex CLI: .codex/config.toml
	codexDir := filepath.Join(workDir, ".codex")
	if err := os.MkdirAll(codexDir, 0755); err != nil {
		return err
	}
	codexConfig := fmt.Sprintf("[mcp_servers.ops-cat]\nurl = %q\n", url)
	return os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(codexConfig), 0644)
}

// writeConfigFiles 在应用数据目录下生成 Claude CLI 和 Codex CLI 的 MCP 配置文件
func (s *MCPServer) writeConfigFiles() error {
	url := s.URL()

	// Claude CLI: .mcp.json
	claudeConfigPath := filepath.Join(s.configDir, ".mcp.json")
	claudeConfig := map[string]any{
		"mcpServers": map[string]any{
			"ops-cat": map[string]any{
				"type": "http",
				"url":  url,
			},
		},
	}
	data, _ := json.MarshalIndent(claudeConfig, "", "  ")
	if err := os.WriteFile(claudeConfigPath, data, 0644); err != nil {
		return err
	}
	s.configFiles = append(s.configFiles, claudeConfigPath)

	// Codex CLI: .codex/config.toml
	codexDir := filepath.Join(s.configDir, ".codex")
	os.MkdirAll(codexDir, 0755)
	codexConfigPath := filepath.Join(codexDir, "config.toml")
	codexConfig := fmt.Sprintf("[mcp_servers.ops-cat]\nurl = %q\n", url)
	if err := os.WriteFile(codexConfigPath, []byte(codexConfig), 0644); err != nil {
		return err
	}
	s.configFiles = append(s.configFiles, codexConfigPath)

	return nil
}
