package backup_svc

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

const (
	webDAVBackupFilename   = gistBackupFilename
	webDAVDefaultDirectory = "opskat"
)

// WebDAVAuthType 描述 WebDAV 服务器接受的鉴权方式。
type WebDAVAuthType string

const (
	WebDAVAuthNone   WebDAVAuthType = "none"
	WebDAVAuthBasic  WebDAVAuthType = "basic"
	WebDAVAuthBearer WebDAVAuthType = "bearer"
)

// webDAVHTTPClient 不跟随重定向：3xx 会把 PUT/PROPFIND/MKCOL/DELETE 改写成 GET，导致难以排查的失败；
// 由 webDAVRequest 显式处理 3xx 并要求用户修正 URL。
var webDAVHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// ValidateWebDAVURL 校验给定的 WebDAV URL 是否符合保存条件：scheme 为 http/https、有 host、不内嵌账号密码。
// 在 App 层保存配置之前调用，确保不会把含明文凭据的 URL 写到 config.json。
func ValidateWebDAVURL(raw string) error {
	_, err := parseWebDAVBaseURL(raw)
	return err
}

// ValidateWebDAVConfig 校验 URL 与 AuthType 必填字段。
// app 层在 Save / Test 入口处调用，避免发出无意义请求。
func ValidateWebDAVConfig(cfg WebDAVConfig) error {
	if err := ValidateWebDAVURL(cfg.URL); err != nil {
		return err
	}
	switch cfg.AuthType {
	case WebDAVAuthNone, "":
		return nil
	case WebDAVAuthBasic:
		if strings.TrimSpace(cfg.Username) == "" {
			return fmt.Errorf("WebDAV username is required for basic auth")
		}
		if cfg.Password == "" {
			return fmt.Errorf("WebDAV password is required for basic auth")
		}
		return nil
	case WebDAVAuthBearer:
		if strings.TrimSpace(cfg.Token) == "" {
			return fmt.Errorf("WebDAV token is required for bearer auth")
		}
		return nil
	default:
		return fmt.Errorf("unsupported WebDAV auth type %q", cfg.AuthType)
	}
}

// WebDAVConfig contains the connection details used for WebDAV backup transport.
type WebDAVConfig struct {
	URL      string         `json:"url"`
	AuthType WebDAVAuthType `json:"authType"`
	Username string         `json:"username,omitempty"` // 仅 basic
	Password string         `json:"password,omitempty"` // 仅 basic
	Token    string         `json:"token,omitempty"`    // 仅 bearer
}

// WebDAVBackupInfo is the frontend-facing metadata for a remote backup file.
type WebDAVBackupInfo struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	UpdatedAt string `json:"updatedAt"`
	Size      int64  `json:"size"`
}

// CreateOrUpdateWebDAVBackup uploads the canonical encrypted backup file to WebDAV.
func CreateOrUpdateWebDAVBackup(cfg WebDAVConfig, content []byte) (*WebDAVBackupInfo, error) {
	dirURL, err := webDAVDirectoryURL(cfg.URL)
	if err != nil {
		return nil, err
	}
	if err := ensureWebDAVDirectory(cfg, dirURL); err != nil {
		return nil, err
	}

	fileURL, err := webDAVFileURL(cfg.URL, webDAVBackupFilename)
	if err != nil {
		return nil, err
	}
	status, body, err := webDAVRequest(cfg, http.MethodPut, fileURL, content, nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK && status != http.StatusCreated && status != http.StatusNoContent {
		return nil, fmt.Errorf("WebDAV upload failed: HTTP %d: %s", status, string(body))
	}
	return &WebDAVBackupInfo{
		Name:      webDAVBackupFilename,
		Path:      fileURL,
		UpdatedAt: time.Now().Format(time.RFC3339),
		Size:      int64(len(content)),
	}, nil
}

// ListWebDAVBackups lists OpsKat backup files from the configured WebDAV directory.
func ListWebDAVBackups(cfg WebDAVConfig) ([]*WebDAVBackupInfo, error) {
	dirURL, err := webDAVDirectoryURL(cfg.URL)
	if err != nil {
		return nil, err
	}
	body := []byte(`<?xml version="1.0" encoding="utf-8"?><propfind xmlns="DAV:"><prop><getlastmodified/><getcontentlength/><resourcetype/></prop></propfind>`)
	status, respBody, err := webDAVRequest(cfg, "PROPFIND", dirURL, body, map[string]string{
		"Depth":        "1",
		"Content-Type": "application/xml; charset=utf-8",
	})
	if err != nil {
		return nil, err
	}
	if status != 207 && status != http.StatusOK {
		if status == http.StatusNotFound {
			return []*WebDAVBackupInfo{}, nil
		}
		return nil, fmt.Errorf("WebDAV list failed: HTTP %d: %s", status, string(respBody))
	}
	return parseWebDAVBackupList(respBody)
}

// GetWebDAVBackupContent downloads a selected OpsKat backup file from WebDAV.
func GetWebDAVBackupContent(cfg WebDAVConfig, name string) ([]byte, error) {
	fileURL, err := webDAVFileURL(cfg.URL, name)
	if err != nil {
		return nil, err
	}
	status, body, err := webDAVRequest(cfg, http.MethodGet, fileURL, nil, nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("WebDAV download failed: HTTP %d: %s", status, string(body))
	}
	return body, nil
}

// TestWebDAVConnection verifies that the configured WebDAV directory is reachable AND writable.
// 仅靠 PROPFIND/列表无法验证写权限：只读只读账号或路径配置错误时也可能返回 200/207 或 404，
// 因此这里走一遍 MKCOL → PUT(probe) → DELETE(probe) 探测，让 UI 的“测试连接”能给出真实结果。
func TestWebDAVConnection(cfg WebDAVConfig) error {
	dirURL, err := webDAVDirectoryURL(cfg.URL)
	if err != nil {
		return err
	}
	if err := ensureWebDAVDirectory(cfg, dirURL); err != nil {
		return err
	}

	probeName := ".opskat-webdav-connection-test-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	probeURL, err := webDAVFileURL(cfg.URL, probeName)
	if err != nil {
		return err
	}

	status, body, err := webDAVRequest(cfg, http.MethodPut, probeURL, []byte("ok"), nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusCreated && status != http.StatusNoContent {
		return fmt.Errorf("WebDAV connection test upload failed: HTTP %d: %s", status, string(body))
	}

	status, body, err = webDAVRequest(cfg, http.MethodDelete, probeURL, nil, nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK &&
		status != http.StatusAccepted &&
		status != http.StatusNoContent &&
		status != http.StatusNotFound {
		return fmt.Errorf("WebDAV connection test cleanup failed: HTTP %d: %s", status, string(body))
	}

	return nil
}

func ensureWebDAVDirectory(cfg WebDAVConfig, dirURL string) error {
	status, body, err := webDAVRequest(cfg, "MKCOL", dirURL, nil, nil)
	if err != nil {
		return err
	}
	// 405 = MKCOL on existing collection (RFC 4918 §9.3.1)。
	// 部分服务器（Nextcloud/ownCloud）在目录已存在时会返回 409 Conflict，也视作成功。
	if status == http.StatusOK ||
		status == http.StatusCreated ||
		status == http.StatusNoContent ||
		status == http.StatusMethodNotAllowed ||
		status == http.StatusConflict {
		return nil
	}
	return fmt.Errorf("WebDAV create directory failed: HTTP %d: %s", status, string(body))
}

// webDAVRequest 只暴露状态码 + 响应体，避免把已 Close 的 *http.Response 抛回给调用方造成误用。
func webDAVRequest(cfg WebDAVConfig, method, target string, body []byte, headers map[string]string) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, target, reader)
	if err != nil {
		return 0, nil, fmt.Errorf("create WebDAV request: %w", err)
	}
	applyWebDAVAuth(req, cfg)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := webDAVHTTPClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("request WebDAV: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("read WebDAV response: %w", err)
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		location := strings.TrimSpace(resp.Header.Get("Location"))
		if location == "" {
			return resp.StatusCode, respBody, fmt.Errorf("WebDAV server returned redirect HTTP %d; please configure the final WebDAV URL directly", resp.StatusCode)
		}
		return resp.StatusCode, respBody, fmt.Errorf("WebDAV server redirected to %q (HTTP %d); please configure the final WebDAV URL directly", location, resp.StatusCode)
	}
	return resp.StatusCode, respBody, nil
}

// applyWebDAVAuth 按 cfg.AuthType 给 req 注入鉴权头。
// 抽成函数：单测可直接验证 header；新增 Digest 等鉴权方式时仅改这一处。
func applyWebDAVAuth(req *http.Request, cfg WebDAVConfig) {
	switch cfg.AuthType {
	case WebDAVAuthBasic:
		req.SetBasicAuth(cfg.Username, cfg.Password)
	case WebDAVAuthBearer:
		if cfg.Token != "" {
			req.Header.Set("Authorization", "Bearer "+cfg.Token)
		}
	case WebDAVAuthNone, "":
		// no-op
	}
}

func webDAVDirectoryURL(raw string) (string, error) {
	u, err := parseWebDAVBaseURL(raw)
	if err != nil {
		return "", err
	}
	u.Path = webDAVStoragePath(u.Path)
	return u.String(), nil
}

func webDAVFileURL(raw, name string) (string, error) {
	if name == "" {
		name = webDAVBackupFilename
	}
	// 额外拒绝 "."、".." 以及 path.Base 之后才能识别的 "\\" 路径分隔符，
	// 防止任何形式的路径穿越被拼到目录 URL 后面。
	if name == "." || name == ".." || strings.ContainsAny(name, `/\`) || name != path.Base(name) {
		return "", fmt.Errorf("invalid WebDAV backup name %q", name)
	}
	u, err := parseWebDAVBaseURL(raw)
	if err != nil {
		return "", err
	}
	u.Path = strings.TrimRight(webDAVStoragePath(u.Path), "/") + "/" + url.PathEscape(name)
	return u.String(), nil
}

func webDAVStoragePath(rawPath string) string {
	cleanPath := strings.TrimRight(rawPath, "/")
	if path.Base(cleanPath) == webDAVDefaultDirectory {
		return cleanPath + "/"
	}
	if cleanPath == "" {
		return "/" + webDAVDefaultDirectory + "/"
	}
	return cleanPath + "/" + webDAVDefaultDirectory + "/"
}

func parseWebDAVBaseURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("WebDAV URL is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse WebDAV URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("WebDAV URL must start with http:// or https://")
	}
	if u.Host == "" {
		return nil, fmt.Errorf("WebDAV URL must include a host")
	}
	// 拒绝形如 https://user:pass@host/path 的 URL：
	// 这里保存进 config 的是 URL 本身（明文），同时账号密码已经有专门的 Username/Password 字段，
	// 否则用户的密码会以明文写入 config.json，并且会被 BasicAuth 重复发送。
	if u.User != nil {
		return nil, fmt.Errorf("WebDAV URL must not include credentials; use the username and password fields instead")
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u, nil
}

type webDAVMultiStatus struct {
	Responses []webDAVResponse `xml:"response"`
}

type webDAVResponse struct {
	Href     string             `xml:"href"`
	Propstat []webDAVPropStatus `xml:"propstat"`
}

type webDAVPropStatus struct {
	Status string     `xml:"status"`
	Prop   webDAVProp `xml:"prop"`
}

type webDAVProp struct {
	GetContentLength string             `xml:"getcontentlength"`
	GetLastModified  string             `xml:"getlastmodified"`
	ResourceType     webDAVResourceType `xml:"resourcetype"`
}

type webDAVResourceType struct {
	Collection *struct{} `xml:"collection"`
}

func parseWebDAVBackupList(data []byte) ([]*WebDAVBackupInfo, error) {
	var ms webDAVMultiStatus
	if err := xml.Unmarshal(data, &ms); err != nil {
		return nil, fmt.Errorf("parse WebDAV list: %w", err)
	}
	result := make([]*WebDAVBackupInfo, 0)
	for _, response := range ms.Responses {
		if response.Href == "" {
			continue
		}
		prop := response.bestProp()
		if prop.ResourceType.Collection != nil {
			continue
		}
		name := webDAVNameFromHref(response.Href)
		if !isOpsKatBackupName(name) {
			continue
		}
		size, _ := strconv.ParseInt(strings.TrimSpace(prop.GetContentLength), 10, 64)
		result = append(result, &WebDAVBackupInfo{
			Name:      name,
			Path:      response.Href,
			UpdatedAt: strings.TrimSpace(prop.GetLastModified),
			Size:      size,
		})
	}
	return result, nil
}

func (r webDAVResponse) bestProp() webDAVProp {
	for _, ps := range r.Propstat {
		if ps.Status == "" || strings.Contains(ps.Status, " 200 ") {
			return ps.Prop
		}
	}
	if len(r.Propstat) > 0 {
		return r.Propstat[0].Prop
	}
	return webDAVProp{}
}

func webDAVNameFromHref(href string) string {
	// 关键点：url.Parse 后用 EscapedPath() 而不是 .Path——后者会把 %2F 还原成字面上的 '/'，
	// 让下面的 path.Base 错误地把其当成路径分隔符，从而绕过对路径穿越名字的过滤。
	raw := href
	if u, err := url.Parse(href); err == nil {
		raw = u.EscapedPath()
	}
	rawName := path.Base(strings.TrimRight(raw, "/"))
	name, err := url.PathUnescape(rawName)
	if err != nil {
		return ""
	}
	if strings.ContainsAny(name, `/\`) {
		return ""
	}
	return name
}

// isOpsKatBackupName 仅匹配 OpsKat 上传到 WebDAV 的加密备份文件。
// ExportToFile 也会产生 opskat-backup-YYYYMMDD.json（明文）但只会出现在本地，
// 这里限定 .encrypted.json，避免用户把明文备份手动放进同一个目录后被当成可解密的备份导入。
func isOpsKatBackupName(name string) bool {
	if name == "" {
		return false
	}
	return name == webDAVBackupFilename ||
		(strings.HasPrefix(name, "opskat-backup-") && strings.HasSuffix(name, ".encrypted.json"))
}
