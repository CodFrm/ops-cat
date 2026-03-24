package sshpool

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// ProxyRequest 代理请求（JSON 握手消息）
type ProxyRequest struct {
	Op         string `json:"op"`                    // "exec" | "upload" | "download" | "copy"
	AssetID    int64  `json:"asset_id"`
	Command    string `json:"command,omitempty"`
	Cols       int    `json:"cols,omitempty"`
	Rows       int    `json:"rows,omitempty"`
	PTY        bool   `json:"pty,omitempty"`
	SrcAssetID int64  `json:"src_asset_id,omitempty"` // copy: 源资产
	SrcPath    string `json:"src_path,omitempty"`
	DstPath    string `json:"dst_path,omitempty"`
}

// ProxyResponse 代理响应（JSON 握手响应）
type ProxyResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// SocketPath 返回 sshpool socket 路径
func SocketPath(dataDir string) string {
	return filepath.Join(dataDir, "sshpool.sock")
}

// Server SSH 代理 Unix socket 服务端
type Server struct {
	pool     *Pool
	listener net.Listener
	done     chan struct{}
	wg       sync.WaitGroup
}

// NewServer 创建代理服务端
func NewServer(pool *Pool) *Server {
	return &Server{
		pool: pool,
		done: make(chan struct{}),
	}
}

// Start 开始监听 Unix socket
func (s *Server) Start(socketPath string) error {
	// 清理 stale socket
	if _, err := os.Stat(socketPath); err == nil {
		conn, err := net.Dial("unix", socketPath)
		if err == nil {
			conn.Close()
			return fmt.Errorf("another instance is already listening on %s", socketPath)
		}
		os.Remove(socketPath)
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", socketPath, err)
	}
	s.listener = listener

	s.wg.Add(1)
	go s.acceptLoop()

	log.Printf("sshpool: server listening on %s", socketPath)
	return nil
}

// Stop 停止服务
func (s *Server) Stop() {
	close(s.done)
	if s.listener != nil {
		s.listener.Close()
	}
	s.wg.Wait()
}

func (s *Server) acceptLoop() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.done:
				return
			default:
				continue
			}
		}
		s.wg.Add(1)
		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer s.wg.Done()
	defer conn.Close()

	reader := bufio.NewReader(conn)

	// 读取 JSON 请求行
	line, err := reader.ReadBytes('\n')
	if err != nil {
		writeJSONResponse(conn, false, "read request failed")
		return
	}

	var req ProxyRequest
	if err := json.Unmarshal(line, &req); err != nil {
		writeJSONResponse(conn, false, "invalid request JSON")
		return
	}

	switch req.Op {
	case "exec":
		s.handleExec(conn, reader, req)
	case "upload":
		s.handleUpload(conn, reader, req)
	case "download":
		s.handleDownload(conn, req)
	case "copy":
		s.handleCopy(conn, req)
	default:
		writeJSONResponse(conn, false, fmt.Sprintf("unknown op: %s", req.Op))
	}
}

// handleExec 处理命令执行或交互式 SSH
func (s *Server) handleExec(conn net.Conn, reader *bufio.Reader, req ProxyRequest) {
	client, err := s.pool.Get(context.Background(), req.AssetID)
	if err != nil {
		writeJSONResponse(conn, false, fmt.Sprintf("get connection: %v", err))
		return
	}
	defer s.pool.Release(req.AssetID)

	session, err := client.NewSession()
	if err != nil {
		s.handleSSHError(req.AssetID, err)
		writeJSONResponse(conn, false, fmt.Sprintf("create session: %v", err))
		return
	}
	defer session.Close()

	// 如果需要 PTY
	if req.PTY {
		cols, rows := req.Cols, req.Rows
		if cols <= 0 {
			cols = 80
		}
		if rows <= 0 {
			rows = 24
		}
		if err := session.RequestPty("xterm-256color", rows, cols, ssh.TerminalModes{
			ssh.ECHO:          1,
			ssh.TTY_OP_ISPEED: 14400,
			ssh.TTY_OP_OSPEED: 14400,
		}); err != nil {
			writeJSONResponse(conn, false, fmt.Sprintf("request pty: %v", err))
			return
		}
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		writeJSONResponse(conn, false, fmt.Sprintf("stdin pipe: %v", err))
		return
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		writeJSONResponse(conn, false, fmt.Sprintf("stdout pipe: %v", err))
		return
	}

	stderr, err := session.StderrPipe()
	if err != nil {
		writeJSONResponse(conn, false, fmt.Sprintf("stderr pipe: %v", err))
		return
	}

	// PTY 模式用 Shell，否则用 Run
	if req.PTY {
		if err := session.Shell(); err != nil {
			writeJSONResponse(conn, false, fmt.Sprintf("start shell: %v", err))
			return
		}
	} else {
		if err := session.Start(req.Command); err != nil {
			writeJSONResponse(conn, false, fmt.Sprintf("start command: %v", err))
			return
		}
	}

	// 握手成功
	writeJSONResponse(conn, true, "")

	done := make(chan struct{})

	// stdout → FrameStdout
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				WriteFrame(conn, FrameStdout, buf[:n])
			}
			if err != nil {
				break
			}
		}
	}()

	// stderr → FrameStderr（PTY 模式下 stderr 和 stdout 合并，这个 goroutine 会立即结束）
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				WriteFrame(conn, FrameStderr, buf[:n])
			}
			if err != nil {
				break
			}
		}
	}()

	// 读取客户端帧 → stdin / resize
	go func() {
		defer close(done)
		for {
			frameType, payload, err := ReadFrame(reader)
			if err != nil {
				stdin.Close()
				return
			}
			switch frameType {
			case FrameStdin:
				stdin.Write(payload)
			case FrameResize:
				if cols, rows, err := ParseResize(payload); err == nil {
					session.WindowChange(int(rows), int(cols))
				}
			}
		}
	}()

	// 等待命令/shell 结束
	exitCode := 0
	if err := session.Wait(); err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			exitCode = exitErr.ExitStatus()
		} else {
			WriteError(conn, err.Error())
			return
		}
	}

	WriteExitCode(conn, exitCode)
	// 等待客户端读循环结束（连接关闭时自然退出）
	<-done
}

// handleUpload 处理文件上传
func (s *Server) handleUpload(conn net.Conn, reader *bufio.Reader, req ProxyRequest) {
	client, err := s.pool.Get(context.Background(), req.AssetID)
	if err != nil {
		writeJSONResponse(conn, false, fmt.Sprintf("get connection: %v", err))
		return
	}
	defer s.pool.Release(req.AssetID)

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		s.handleSSHError(req.AssetID, err)
		writeJSONResponse(conn, false, fmt.Sprintf("create sftp client: %v", err))
		return
	}
	defer sftpClient.Close()

	writeJSONResponse(conn, true, "")

	remoteFile, err := sftpClient.Create(req.DstPath)
	if err != nil {
		WriteFrame(conn, FrameFileErr, []byte(fmt.Sprintf("create remote file: %v", err)))
		return
	}
	defer remoteFile.Close()

	// 读取 FileData 帧直到 FileEOF
	for {
		frameType, payload, err := ReadFrame(reader)
		if err != nil {
			WriteFrame(conn, FrameFileErr, []byte(fmt.Sprintf("read frame: %v", err)))
			return
		}
		switch frameType {
		case FrameFileData:
			if _, err := remoteFile.Write(payload); err != nil {
				WriteFrame(conn, FrameFileErr, []byte(fmt.Sprintf("write remote file: %v", err)))
				return
			}
		case FrameFileEOF:
			WriteFrame(conn, FrameOK, nil)
			return
		default:
			WriteFrame(conn, FrameFileErr, []byte(fmt.Sprintf("unexpected frame type: 0x%02x", frameType)))
			return
		}
	}
}

// handleDownload 处理文件下载
func (s *Server) handleDownload(conn net.Conn, req ProxyRequest) {
	client, err := s.pool.Get(context.Background(), req.AssetID)
	if err != nil {
		writeJSONResponse(conn, false, fmt.Sprintf("get connection: %v", err))
		return
	}
	defer s.pool.Release(req.AssetID)

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		s.handleSSHError(req.AssetID, err)
		writeJSONResponse(conn, false, fmt.Sprintf("create sftp client: %v", err))
		return
	}
	defer sftpClient.Close()

	remoteFile, err := sftpClient.Open(req.SrcPath)
	if err != nil {
		writeJSONResponse(conn, false, fmt.Sprintf("open remote file: %v", err))
		return
	}
	defer remoteFile.Close()

	writeJSONResponse(conn, true, "")

	// 分块读取远程文件发送 FileData 帧
	buf := make([]byte, 32*1024)
	for {
		n, err := remoteFile.Read(buf)
		if n > 0 {
			if writeErr := WriteFrame(conn, FrameFileData, buf[:n]); writeErr != nil {
				return
			}
		}
		if err == io.EOF {
			WriteFrame(conn, FrameFileEOF, nil)
			return
		}
		if err != nil {
			WriteFrame(conn, FrameFileErr, []byte(fmt.Sprintf("read remote file: %v", err)))
			return
		}
	}
}

// handleCopy 处理远程到远程复制
func (s *Server) handleCopy(conn net.Conn, req ProxyRequest) {
	srcClient, err := s.pool.Get(context.Background(), req.SrcAssetID)
	if err != nil {
		writeJSONResponse(conn, false, fmt.Sprintf("get source connection: %v", err))
		return
	}
	defer s.pool.Release(req.SrcAssetID)

	dstClient, err := s.pool.Get(context.Background(), req.AssetID)
	if err != nil {
		writeJSONResponse(conn, false, fmt.Sprintf("get destination connection: %v", err))
		return
	}
	defer s.pool.Release(req.AssetID)

	srcSFTP, err := sftp.NewClient(srcClient)
	if err != nil {
		s.handleSSHError(req.SrcAssetID, err)
		writeJSONResponse(conn, false, fmt.Sprintf("create source sftp: %v", err))
		return
	}
	defer srcSFTP.Close()

	dstSFTP, err := sftp.NewClient(dstClient)
	if err != nil {
		s.handleSSHError(req.AssetID, err)
		writeJSONResponse(conn, false, fmt.Sprintf("create destination sftp: %v", err))
		return
	}
	defer dstSFTP.Close()

	srcFile, err := srcSFTP.Open(req.SrcPath)
	if err != nil {
		writeJSONResponse(conn, false, fmt.Sprintf("open source file: %v", err))
		return
	}
	defer srcFile.Close()

	dstFile, err := dstSFTP.Create(req.DstPath)
	if err != nil {
		writeJSONResponse(conn, false, fmt.Sprintf("create destination file: %v", err))
		return
	}
	defer dstFile.Close()

	writeJSONResponse(conn, true, "")

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		WriteFrame(conn, FrameError, []byte(fmt.Sprintf("copy: %v", err)))
		return
	}

	WriteFrame(conn, FrameOK, nil)
}

// handleSSHError 处理 SSH 连接错误，移除可能已断开的连接
func (s *Server) handleSSHError(assetID int64, err error) {
	// 如果是连接层面的错误，移除缓存的连接
	if isConnectionError(err) {
		s.pool.Remove(assetID)
	}
}

func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	// SSH session 创建失败或 SFTP 创建失败通常意味着底层连接有问题
	if err == io.EOF {
		return true
	}
	if _, ok := err.(*net.OpError); ok {
		return true
	}
	return false
}

func writeJSONResponse(conn net.Conn, ok bool, errMsg string) {
	resp := ProxyResponse{OK: ok, Error: errMsg}
	data, _ := json.Marshal(resp)
	data = append(data, '\n')
	conn.Write(data)
}
