package extension

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

// HostABIVersion is the current host ABI contract version.
// Extensions must declare a compatible hostABI in their manifest.
// Bump the minor version when adding new host functions (backward compatible);
// bump the major version when removing or changing existing host function signatures.
const HostABIVersion = "1.0"

// SupportedHostABIs lists all host ABI versions the runtime accepts.
var SupportedHostABIs = []string{"1.0"}

var (
	semverRe         = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	nameRe           = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	policyIDRe       = regexp.MustCompile(`^ext:[a-z0-9][a-z0-9_-]{0,63}(:[a-z0-9][a-z0-9_-]{0,63})*$`)
	credentialRe     = regexp.MustCompile(`^(|read)$`) // "" or "read"
	snippetCatIDRe   = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)
	snippetSeedKeyRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
)

// builtinSnippetCategoryIDs is the immutable set of category ids owned by the app core.
// Extensions may not re-declare these — they are reserved.
// Kept unexported so callers can't mutate it; use IsBuiltinSnippetCategoryID to query.
var builtinSnippetCategoryIDs = map[string]struct{}{
	"shell":  {},
	"sql":    {},
	"redis":  {},
	"mongo":  {},
	"prompt": {},
}

// IsBuiltinSnippetCategoryID reports whether id is reserved by the app core.
func IsBuiltinSnippetCategoryID(id string) bool {
	_, ok := builtinSnippetCategoryIDs[id]
	return ok
}

// CredentialAccessNone means the extension receives only opaque credential handles (default).
// CredentialAccessRead means the extension may request plaintext credentials (discouraged).
const (
	CredentialAccessNone = ""
	CredentialAccessRead = "read"
)

type Manifest struct {
	Name          string          `json:"name"`
	Version       string          `json:"version"`
	Icon          string          `json:"icon"`
	MinAppVersion string          `json:"minAppVersion"`
	HostABI       string          `json:"hostABI"`
	Capabilities  Capabilities    `json:"capabilities"`
	I18n          ManifestI18n    `json:"i18n"`
	Backend       ManifestBackend `json:"backend"`
	AssetTypes    []AssetTypeDef  `json:"assetTypes"`
	Tools         []ToolDef       `json:"tools"`
	Policies      PoliciesDef     `json:"policies"`
	Frontend      FrontendDef     `json:"frontend"`
	Snippets      SnippetsDef     `json:"snippets"`
}

// SnippetsDef declares snippet categories and seed snippets contributed by this extension.
// All fields are optional; an empty block is a no-op.
type SnippetsDef struct {
	Categories []SnippetCategoryDef `json:"categories"`
	Seed       []SeedSnippetDef     `json:"seed"`
}

// SnippetCategoryDef declares a new snippet category id that this extension owns.
//
// Validation rules:
//   - ID must match ^[a-z][a-z0-9-]{0,31}$ (lowercase kebab, <=32 chars).
//   - ID must NOT collide with any builtin category (see IsBuiltinSnippetCategoryID).
//   - ID must be unique within this manifest.
//   - AssetType must be non-empty and must match a type declared in assetTypes[].
type SnippetCategoryDef struct {
	ID        string   `json:"id"`
	AssetType string   `json:"assetType"`
	I18n      I18nName `json:"i18n"`
}

// SeedSnippetDef is a read-only snippet shipped with the extension.
//
// Validation rules:
//   - Key must match ^[a-z0-9][a-z0-9-]{0,63}$ and be unique within this manifest.
//   - Name and Content must be non-empty after trimming.
//   - Category must be either a builtin category id or a category declared in THIS manifest.
type SeedSnippetDef struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Content     string `json:"content"`
	Description string `json:"description"`
}

// Capabilities declares what host resources an extension is permitted to access.
// Default (zero value) is deny-all except an implicit read-only ${EXT_DIR}/** for the
// extension's own directory. Extensions must declare every capability they need;
// the host enforces these at each host_* call site.
type Capabilities struct {
	FS          FSCapability   `json:"fs"`
	HTTP        HTTPCapability `json:"http"`
	Credentials string         `json:"credentials"` // "" (none) | "read"
	Tunnel      bool           `json:"tunnel"`      // allow routing HTTP through the asset's SSH tunnel
}

// FSCapability lists filesystem path patterns an extension may access.
// Patterns support one ${EXT_DIR} placeholder (resolved at load time to the extension's directory)
// and a trailing /** for recursive match. All other entries are treated as absolute path prefixes.
type FSCapability struct {
	Read  []string `json:"read"`
	Write []string `json:"write"`
}

// HTTPCapability lists URL prefix patterns an extension may open via host_io_open(type=http).
// Entries are URL prefixes; the request URL must start with at least one allowlist entry.
// Private-network destinations (RFC1918, loopback, link-local) are always rejected
// unless the target also matches an allowlist entry AND Tunnel=true.
type HTTPCapability struct {
	Allowlist []string `json:"allowlist"`
}

type ManifestI18n struct {
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
}

type ManifestBackend struct {
	Runtime string `json:"runtime"`
	Binary  string `json:"binary"`
}

type AssetTypeDef struct {
	Type         string         `json:"type"`
	I18n         I18nName       `json:"i18n"`
	ConfigSchema map[string]any `json:"configSchema"`
}

type I18nName struct {
	Name string `json:"name"`
}

type I18nNameDesc struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type ToolDef struct {
	Name       string         `json:"name"`
	I18n       I18nDesc       `json:"i18n"`
	Parameters map[string]any `json:"parameters"`
}

type I18nDesc struct {
	Description string `json:"description"`
}

type PoliciesDef struct {
	Type    string           `json:"type"`
	Actions []string         `json:"actions"`
	Groups  []PolicyGroupDef `json:"groups"`
	Default []string         `json:"default"`
}

type PolicyGroupDef struct {
	ID     string         `json:"id"`
	I18n   I18nNameDesc   `json:"i18n"`
	Policy map[string]any `json:"policy"`
}

type FrontendDef struct {
	Entry  string    `json:"entry"`
	Styles string    `json:"styles"`
	Pages  []PageDef `json:"pages"`
}

type PageDef struct {
	ID        string   `json:"id"`
	Slot      string   `json:"slot,omitempty"`
	I18n      I18nName `json:"i18n"`
	Component string   `json:"component"`
}

func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if err := m.validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// Localized returns a shallow copy of the manifest with all i18n string fields
// resolved using the provided translate function.
func (m *Manifest) Localized(tr func(key string) string) *Manifest {
	out := *m
	out.I18n = ManifestI18n{
		DisplayName: tr(m.I18n.DisplayName),
		Description: tr(m.I18n.Description),
	}

	if len(m.AssetTypes) > 0 {
		out.AssetTypes = make([]AssetTypeDef, len(m.AssetTypes))
		for i, at := range m.AssetTypes {
			at.I18n = I18nName{Name: tr(at.I18n.Name)}
			at.ConfigSchema = localizeConfigSchema(at.ConfigSchema, tr)
			out.AssetTypes[i] = at
		}
	}

	if len(m.Tools) > 0 {
		out.Tools = make([]ToolDef, len(m.Tools))
		for i, t := range m.Tools {
			t.I18n = I18nDesc{Description: tr(t.I18n.Description)}
			out.Tools[i] = t
		}
	}

	if len(m.Policies.Groups) > 0 {
		out.Policies.Groups = make([]PolicyGroupDef, len(m.Policies.Groups))
		for i, pg := range m.Policies.Groups {
			pg.I18n = I18nNameDesc{
				Name:        tr(pg.I18n.Name),
				Description: tr(pg.I18n.Description),
			}
			out.Policies.Groups[i] = pg
		}
	}

	if len(m.Frontend.Pages) > 0 {
		out.Frontend.Pages = make([]PageDef, len(m.Frontend.Pages))
		for i, p := range m.Frontend.Pages {
			p.I18n = I18nName{Name: tr(p.I18n.Name)}
			out.Frontend.Pages[i] = p
		}
	}

	if len(m.Snippets.Categories) > 0 {
		out.Snippets.Categories = make([]SnippetCategoryDef, len(m.Snippets.Categories))
		for i, c := range m.Snippets.Categories {
			c.I18n = I18nName{Name: tr(c.I18n.Name)}
			out.Snippets.Categories[i] = c
		}
	}

	return &out
}

// localizeConfigSchema translates title, placeholder, description fields in a JSON Schema.
func localizeConfigSchema(schema map[string]any, tr func(string) string) map[string]any {
	if schema == nil {
		return nil
	}
	out := make(map[string]any, len(schema))
	for k, v := range schema {
		out[k] = v
	}
	// Translate top-level title/placeholder/description
	for _, field := range []string{"title", "placeholder", "description"} {
		if s, ok := out[field].(string); ok && s != "" {
			out[field] = tr(s)
		}
	}
	// Recurse into properties
	if props, ok := out["properties"].(map[string]any); ok {
		newProps := make(map[string]any, len(props))
		for name, propVal := range props {
			if propMap, ok := propVal.(map[string]any); ok {
				newProps[name] = localizeConfigSchema(propMap, tr)
			} else {
				newProps[name] = propVal
			}
		}
		out["properties"] = newProps
	}
	return out
}

func (m *Manifest) validate() error {
	if m.Name == "" {
		return fmt.Errorf("manifest: name is required")
	}
	if !nameRe.MatchString(m.Name) {
		return fmt.Errorf("manifest: name must match %s (got %q)", nameRe.String(), m.Name)
	}
	if m.Version == "" {
		return fmt.Errorf("manifest: version is required")
	}
	if !semverRe.MatchString(m.Version) {
		return fmt.Errorf("manifest: version must be semver (got %q)", m.Version)
	}
	if m.MinAppVersion != "" && !semverRe.MatchString(m.MinAppVersion) {
		return fmt.Errorf("manifest: minAppVersion must be semver (got %q)", m.MinAppVersion)
	}
	if m.HostABI == "" {
		return fmt.Errorf("manifest: hostABI is required (expected one of %v)", SupportedHostABIs)
	}
	if !isSupportedHostABI(m.HostABI) {
		return fmt.Errorf("manifest: hostABI %q not supported by this runtime (supported: %v)", m.HostABI, SupportedHostABIs)
	}
	if !credentialRe.MatchString(m.Capabilities.Credentials) {
		return fmt.Errorf("manifest: capabilities.credentials must be \"\" or \"read\" (got %q)", m.Capabilities.Credentials)
	}
	for _, g := range m.Policies.Groups {
		if !strings.HasPrefix(g.ID, "ext:") {
			return fmt.Errorf("manifest: policy group ID must start with ext: (got %q)", g.ID)
		}
		if !policyIDRe.MatchString(g.ID) {
			return fmt.Errorf("manifest: policy group ID has invalid characters (got %q)", g.ID)
		}
	}
	if err := m.validateSnippets(); err != nil {
		return err
	}
	return nil
}

// validateSnippets validates the snippets.categories and snippets.seed blocks.
// Both are optional; an empty block passes.
func (m *Manifest) validateSnippets() error {
	// assetTypes declared by this manifest; used to validate category.assetType.
	assetTypeSet := make(map[string]struct{}, len(m.AssetTypes))
	for _, at := range m.AssetTypes {
		if at.Type != "" {
			assetTypeSet[at.Type] = struct{}{}
		}
	}

	// Validate categories.
	catIDs := make(map[string]struct{}, len(m.Snippets.Categories))
	for _, c := range m.Snippets.Categories {
		if c.ID == "" {
			return fmt.Errorf("manifest: snippets.categories[].id is required")
		}
		if !snippetCatIDRe.MatchString(c.ID) {
			return fmt.Errorf("manifest: snippets.categories[].id must match %s (got %q)", snippetCatIDRe.String(), c.ID)
		}
		if IsBuiltinSnippetCategoryID(c.ID) {
			return fmt.Errorf("manifest: snippets.categories[].id %q collides with a builtin category", c.ID)
		}
		if _, dup := catIDs[c.ID]; dup {
			return fmt.Errorf("manifest: duplicate snippets.categories[].id %q", c.ID)
		}
		catIDs[c.ID] = struct{}{}
		if c.AssetType == "" {
			return fmt.Errorf("manifest: snippets.categories[%q].assetType is required", c.ID)
		}
		if _, ok := assetTypeSet[c.AssetType]; !ok {
			return fmt.Errorf("manifest: snippets.categories[%q].assetType %q must be declared in assetTypes[]", c.ID, c.AssetType)
		}
	}

	// Validate seed snippets.
	seedKeys := make(map[string]struct{}, len(m.Snippets.Seed))
	for _, s := range m.Snippets.Seed {
		if s.Key == "" {
			return fmt.Errorf("manifest: snippets.seed[].key is required")
		}
		if !snippetSeedKeyRe.MatchString(s.Key) {
			return fmt.Errorf("manifest: snippets.seed[].key must match %s (got %q)", snippetSeedKeyRe.String(), s.Key)
		}
		if _, dup := seedKeys[s.Key]; dup {
			return fmt.Errorf("manifest: duplicate snippets.seed[].key %q", s.Key)
		}
		seedKeys[s.Key] = struct{}{}
		if strings.TrimSpace(s.Name) == "" {
			return fmt.Errorf("manifest: snippets.seed[%q].name is required", s.Key)
		}
		if strings.TrimSpace(s.Content) == "" {
			return fmt.Errorf("manifest: snippets.seed[%q].content is required", s.Key)
		}
		if s.Category == "" {
			return fmt.Errorf("manifest: snippets.seed[%q].category is required", s.Key)
		}
		isBuiltin := IsBuiltinSnippetCategoryID(s.Category)
		_, isLocal := catIDs[s.Category]
		if !isBuiltin && !isLocal {
			return fmt.Errorf("manifest: snippets.seed[%q].category %q is neither builtin nor declared in this manifest", s.Key, s.Category)
		}
	}
	return nil
}

func isSupportedHostABI(abi string) bool {
	for _, s := range SupportedHostABIs {
		if s == abi {
			return true
		}
	}
	return false
}

// -- Capability enforcement --------------------------------------------------

// CheckFSRead returns nil if reading the given absolute path is permitted
// by the extension's fs.read capabilities. extDir is the extension's own
// directory (used to resolve ${EXT_DIR} placeholders).
func (m *Manifest) CheckFSRead(path, extDir string) error {
	return checkFSPath(path, extDir, m.Capabilities.FS.Read, "read")
}

// CheckFSWrite returns nil if writing to the given absolute path is permitted
// by the extension's fs.write capabilities.
func (m *Manifest) CheckFSWrite(path, extDir string) error {
	return checkFSPath(path, extDir, m.Capabilities.FS.Write, "write")
}

func checkFSPath(path, extDir string, patterns []string, kind string) error {
	if path == "" {
		return fmt.Errorf("fs %s denied: empty path", kind)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("fs %s denied: resolve path: %w", kind, err)
	}
	// Reject parent-traversal leftovers after absolute resolution.
	if strings.Contains(abs, "..") {
		return fmt.Errorf("fs %s denied: path contains traversal (%q)", kind, path)
	}
	for _, pat := range patterns {
		resolved := resolvePattern(pat, extDir)
		if resolved == "" {
			continue
		}
		if matchPathPrefix(abs, resolved) {
			return nil
		}
	}
	return fmt.Errorf("fs %s denied: %q not in capabilities.fs.%s", kind, abs, kind)
}

// resolvePattern substitutes ${EXT_DIR} and cleans the pattern.
// Returns empty string on failure.
func resolvePattern(pattern, extDir string) string {
	if pattern == "" {
		return ""
	}
	p := strings.ReplaceAll(pattern, "${EXT_DIR}", extDir)
	// Strip trailing "/**" if present — we match by prefix.
	p = strings.TrimSuffix(p, "/**")
	p = strings.TrimSuffix(p, "**")
	if p == "" {
		return ""
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return ""
	}
	return filepath.Clean(abs)
}

// matchPathPrefix returns true if path is within root (root is a prefix,
// separated by a path separator).
func matchPathPrefix(path, root string) bool {
	if path == root {
		return true
	}
	sep := string(filepath.Separator)
	if !strings.HasSuffix(root, sep) {
		root += sep
	}
	return strings.HasPrefix(path, root)
}

// CheckHTTPURL returns nil if the given URL is permitted by the extension's
// http capabilities. Enforces:
//  1. The URL prefix must match an allowlist entry.
//  2. Private/loopback/link-local destinations are rejected unless tunnelAllowed is true.
//  3. The scheme must be http or https.
//
// tunnelAllowed should be true ONLY when the host has an active SSH tunnel
// configured AND the extension declared capabilities.tunnel=true.
func (m *Manifest) CheckHTTPURL(urlStr string, tunnelAllowed bool) error {
	if urlStr == "" {
		return fmt.Errorf("http denied: empty url")
	}
	u, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("http denied: parse url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("http denied: unsupported scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("http denied: missing host")
	}

	// Enforce allowlist prefix match.
	matched := false
	for _, allowed := range m.Capabilities.HTTP.Allowlist {
		if allowed == "" {
			continue
		}
		if strings.HasPrefix(urlStr, allowed) {
			matched = true
			break
		}
	}
	if !matched {
		return fmt.Errorf("http denied: %q not in capabilities.http.allowlist", urlStr)
	}

	// Reject private-network destinations unless tunnel routing is allowed.
	if !tunnelAllowed && isPrivateHost(u.Hostname()) {
		return fmt.Errorf("http denied: %q resolves to private/loopback/link-local address (tunnel=%v, capabilities.tunnel=%v)",
			u.Hostname(), tunnelAllowed, m.Capabilities.Tunnel)
	}
	return nil
}

// isPrivateHost returns true if the host is a literal private IP (RFC1918,
// loopback, link-local, or IPv6 unique-local / link-local).
// For DNS hostnames it returns false — the HTTP stack will resolve at dial time,
// and the private-network check must also run at dial time (see DialGuard).
func isPrivateHost(host string) bool {
	if host == "" {
		return true
	}
	// Strip brackets from IPv6 literals.
	host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		// DNS hostname — dial-time guard handles this.
		return false
	}
	return isPrivateIP(ip)
}

// IsPrivateIP returns true for RFC1918, loopback, link-local, unspecified,
// IPv6 ULA, IPv6 link-local, and AWS/GCP metadata endpoints.
// Exported so that the dial-time guard in io_http.go can reuse it.
func IsPrivateIP(ip net.IP) bool {
	return isPrivateIP(ip)
}

// isPrivateIP returns true for RFC1918, loopback, link-local, unspecified,
// IPv6 ULA, IPv6 link-local, and AWS/GCP metadata endpoints.
func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsPrivate() {
		return true
	}
	// IPv6 unique-local is covered by IsPrivate() in Go 1.17+.
	// AWS metadata: 169.254.169.254 (IsLinkLocalUnicast covers 169.254/16).
	return false
}

// CheckCredentialRead returns nil if the extension is allowed to request
// plaintext credential values. Default is deny.
func (m *Manifest) CheckCredentialRead() error {
	if m.Capabilities.Credentials != CredentialAccessRead {
		return fmt.Errorf("credentials denied: extension must declare capabilities.credentials=\"read\" to access plaintext passwords")
	}
	return nil
}

// CheckTunnel returns nil if the extension is allowed to use SSH tunnel routing.
func (m *Manifest) CheckTunnel() error {
	if !m.Capabilities.Tunnel {
		return fmt.Errorf("tunnel denied: extension must declare capabilities.tunnel=true")
	}
	return nil
}
