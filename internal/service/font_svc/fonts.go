package font_svc

import (
	"context"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf16"
)

const fontconfigTimeout = 2 * time.Second

// ListFamilies returns installed font family names. It prefers fontconfig when
// available and falls back to parsing common system font directories.
func ListFamilies(ctx context.Context) ([]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	families := make(map[string]struct{})
	for _, family := range listFontconfigFamilies(ctx) {
		addFamily(families, family)
	}
	for _, family := range scanFontDirectories(defaultFontDirectories()) {
		addFamily(families, family)
	}

	out := make([]string, 0, len(families))
	for family := range families {
		out = append(out, family)
	}
	sort.Strings(out)
	return out, nil
}

func listFontconfigFamilies(ctx context.Context) []string {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return nil
	}

	cmdCtx, cancel := context.WithTimeout(ctx, fontconfigTimeout)
	defer cancel()

	out, err := exec.CommandContext(cmdCtx, "fc-list", ":", "family").Output()
	if err != nil {
		return nil
	}
	return parseFontconfigFamilies(string(out))
}

func parseFontconfigFamilies(out string) []string {
	families := make(map[string]struct{})
	for _, line := range strings.Split(out, "\n") {
		if before, _, ok := strings.Cut(line, ":"); ok {
			line = before
		}
		for _, part := range strings.Split(line, ",") {
			addFamily(families, part)
		}
	}

	result := make([]string, 0, len(families))
	for family := range families {
		result = append(result, family)
	}
	sort.Strings(result)
	return result
}

func scanFontDirectories(dirs []string) []string {
	families := make(map[string]struct{})
	seenDirs := make(map[string]struct{})
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		cleanDir := filepath.Clean(dir)
		if _, ok := seenDirs[cleanDir]; ok {
			continue
		}
		seenDirs[cleanDir] = struct{}{}

		info, err := os.Stat(cleanDir)
		if err != nil || !info.IsDir() {
			continue
		}

		_ = filepath.WalkDir(cleanDir, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if entry.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext != ".ttf" && ext != ".otf" && ext != ".ttc" {
				return nil
			}
			names, err := parseFontFileFamilies(path)
			if err != nil {
				return nil
			}
			for _, name := range names {
				addFamily(families, name)
			}
			return nil
		})
	}

	result := make([]string, 0, len(families))
	for family := range families {
		result = append(result, family)
	}
	sort.Strings(result)
	return result
}

func defaultFontDirectories() []string {
	var dirs []string
	home, _ := os.UserHomeDir()

	switch runtime.GOOS {
	case "darwin":
		dirs = append(dirs,
			"/System/Library/Fonts",
			"/System/Library/Fonts/Supplemental",
			"/Library/Fonts",
			"/Network/Library/Fonts",
		)
		if home != "" {
			dirs = append(dirs, filepath.Join(home, "Library", "Fonts"))
		}
	case "linux":
		dirs = append(dirs, "/usr/share/fonts", "/usr/local/share/fonts")
		if dataHome := os.Getenv("XDG_DATA_HOME"); dataHome != "" {
			dirs = append(dirs, filepath.Join(dataHome, "fonts"))
		} else if home != "" {
			dirs = append(dirs, filepath.Join(home, ".local", "share", "fonts"))
		}
		for _, base := range strings.Split(os.Getenv("XDG_DATA_DIRS"), ":") {
			if base != "" {
				dirs = append(dirs, filepath.Join(base, "fonts"))
			}
		}
		if home != "" {
			dirs = append(dirs, filepath.Join(home, ".fonts"))
		}
	case "windows":
		if windir := os.Getenv("WINDIR"); windir != "" {
			dirs = append(dirs, filepath.Join(windir, "Fonts"))
		}
	default:
		if home != "" {
			dirs = append(dirs, filepath.Join(home, ".fonts"))
		}
	}
	return dirs
}

func parseFontFileFamilies(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseFontFamilies(data), nil
}

func parseFontFamilies(data []byte) []string {
	families := make(map[string]struct{})
	if len(data) < 12 {
		return nil
	}

	if string(data[:4]) == "ttcf" {
		count, ok := readU32(data, 8)
		if !ok || count == 0 || count > 1024 {
			return nil
		}
		for i := uint32(0); i < count; i++ {
			offset, ok := readU32(data, 12+int(i)*4)
			if !ok {
				continue
			}
			for _, name := range parseSFNTFamilies(data, int(offset)) {
				addFamily(families, name)
			}
		}
	} else {
		for _, name := range parseSFNTFamilies(data, 0) {
			addFamily(families, name)
		}
	}

	result := make([]string, 0, len(families))
	for family := range families {
		result = append(result, family)
	}
	sort.Strings(result)
	return result
}

func parseSFNTFamilies(data []byte, base int) []string {
	if base < 0 || base+12 > len(data) || !isSFNT(data[base:base+4]) {
		return nil
	}

	numTables, ok := readU16(data, base+4)
	if !ok {
		return nil
	}
	tableDirEnd := base + 12 + int(numTables)*16
	if tableDirEnd > len(data) {
		return nil
	}

	for i := 0; i < int(numTables); i++ {
		record := base + 12 + i*16
		if string(data[record:record+4]) != "name" {
			continue
		}
		offset, ok := readU32(data, record+8)
		if !ok {
			return nil
		}
		length, ok := readU32(data, record+12)
		if !ok {
			return nil
		}
		tableOffset := int(offset)
		tableLength := int(length)
		if tableOffset < 0 || tableLength < 0 || tableOffset+tableLength > len(data) {
			return nil
		}
		return parseNameTableFamilies(data[tableOffset : tableOffset+tableLength])
	}
	return nil
}

func isSFNT(tag []byte) bool {
	if len(tag) != 4 {
		return false
	}
	switch string(tag) {
	case "\x00\x01\x00\x00", "OTTO", "true", "typ1":
		return true
	default:
		return false
	}
}

func parseNameTableFamilies(table []byte) []string {
	count, ok := readU16(table, 2)
	if !ok {
		return nil
	}
	stringOffset, ok := readU16(table, 4)
	if !ok {
		return nil
	}

	recordsEnd := 6 + int(count)*12
	if recordsEnd > len(table) || int(stringOffset) > len(table) {
		return nil
	}

	preferred16 := make(map[string]struct{})
	all16 := make(map[string]struct{})
	preferred1 := make(map[string]struct{})
	all1 := make(map[string]struct{})

	for i := 0; i < int(count); i++ {
		record := 6 + i*12
		platformID, _ := readU16(table, record)
		encodingID, _ := readU16(table, record+2)
		languageID, _ := readU16(table, record+4)
		nameID, _ := readU16(table, record+6)
		length, _ := readU16(table, record+8)
		offset, _ := readU16(table, record+10)

		if nameID != 1 && nameID != 16 {
			continue
		}
		start := int(stringOffset) + int(offset)
		end := start + int(length)
		if start < 0 || end > len(table) || start > end {
			continue
		}
		name := cleanFamilyName(decodeName(platformID, encodingID, table[start:end]))
		if name == "" {
			continue
		}

		targetAll := all1
		targetPreferred := preferred1
		if nameID == 16 {
			targetAll = all16
			targetPreferred = preferred16
		}
		targetAll[name] = struct{}{}
		if isPreferredLanguage(platformID, languageID) {
			targetPreferred[name] = struct{}{}
		}
	}

	for _, names := range []map[string]struct{}{preferred16, all16, preferred1, all1} {
		if len(names) > 0 {
			return sortedNames(names)
		}
	}
	return nil
}

func decodeName(platformID, encodingID uint16, raw []byte) string {
	if platformID == 0 || platformID == 3 || (platformID == 2 && encodingID == 1) {
		if len(raw)%2 != 0 {
			return ""
		}
		u16 := make([]uint16, 0, len(raw)/2)
		for i := 0; i < len(raw); i += 2 {
			u16 = append(u16, binary.BigEndian.Uint16(raw[i:i+2]))
		}
		return string(utf16.Decode(u16))
	}
	return string(raw)
}

func isPreferredLanguage(platformID, languageID uint16) bool {
	switch platformID {
	case 0:
		return true
	case 1:
		return languageID == 0
	case 3:
		return languageID == 0 || languageID == 0x0409
	default:
		return false
	}
}

func cleanFamilyName(name string) string {
	name = strings.ToValidUTF8(name, "")
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	return strings.TrimSpace(name)
}

func addFamily(families map[string]struct{}, family string) {
	family = cleanFamilyName(family)
	if family == "" {
		return
	}
	families[family] = struct{}{}
}

func sortedNames(names map[string]struct{}) []string {
	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func readU16(data []byte, offset int) (uint16, bool) {
	if offset < 0 || offset+2 > len(data) {
		return 0, false
	}
	return binary.BigEndian.Uint16(data[offset : offset+2]), true
}

func readU32(data []byte, offset int) (uint32, bool) {
	if offset < 0 || offset+4 > len(data) {
		return 0, false
	}
	return binary.BigEndian.Uint32(data[offset : offset+4]), true
}
