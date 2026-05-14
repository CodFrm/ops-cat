package font_svc

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"unicode/utf16"
)

func TestListFamiliesCachesResult(t *testing.T) {
	InvalidateCache()
	t.Cleanup(InvalidateCache)

	first, err := ListFamilies(context.Background())
	if err != nil {
		t.Fatalf("first ListFamilies: %v", err)
	}

	// Mutating the cached slice should not affect a fresh call — the cache
	// returns the same underlying slice, so we instead check that a second
	// call is identity-equal (no new computation).
	second, err := ListFamilies(context.Background())
	if err != nil {
		t.Fatalf("second ListFamilies: %v", err)
	}
	if len(first) > 0 && &first[0] != &second[0] {
		t.Fatalf("expected cached slice to be reused, got fresh allocation")
	}

	InvalidateCache()
	third, err := ListFamilies(context.Background())
	if err != nil {
		t.Fatalf("third ListFamilies: %v", err)
	}
	if len(third) > 0 && len(second) > 0 && &third[0] == &second[0] {
		t.Fatalf("expected InvalidateCache to force a re-scan")
	}
}

type testNameRecord struct {
	platformID uint16
	encodingID uint16
	languageID uint16
	nameID     uint16
	text       string
}

func TestParseFontFamiliesPrefersTypographicFamily(t *testing.T) {
	font := makeTestFont([]testNameRecord{
		{platformID: 3, encodingID: 1, languageID: 0x0409, nameID: 1, text: "Example Mono Bold"},
		{platformID: 3, encodingID: 1, languageID: 0x0409, nameID: 16, text: "Example Mono"},
	})

	got := parseFontFamilies(font)
	want := []string{"Example Mono"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseFontFamilies() = %#v, want %#v", got, want)
	}
}

func TestParseFontFamiliesFallsBackToFamilyName(t *testing.T) {
	font := makeTestFont([]testNameRecord{
		{platformID: 3, encodingID: 1, languageID: 0x0409, nameID: 1, text: "Fallback Mono"},
	})

	got := parseFontFamilies(font)
	want := []string{"Fallback Mono"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseFontFamilies() = %#v, want %#v", got, want)
	}
}

func TestParseFontconfigFamilies(t *testing.T) {
	got := parseFontconfigFamilies("Zed Mono, Zed Mono Book\nMenlo\nFira Code:style=Regular\n")
	want := []string{"Fira Code", "Menlo", "Zed Mono", "Zed Mono Book"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseFontconfigFamilies() = %#v, want %#v", got, want)
	}
}

func TestScanFontDirectories(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Example.ttf"), makeTestFont([]testNameRecord{
		{platformID: 3, encodingID: 1, languageID: 0x0409, nameID: 1, text: "Scanned Mono"},
	}), 0o644); err != nil {
		t.Fatal(err)
	}

	got := scanFontDirectories([]string{dir})
	want := []string{"Scanned Mono"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("scanFontDirectories() = %#v, want %#v", got, want)
	}
}

func makeTestFont(records []testNameRecord) []byte {
	nameTableHeaderLen := 6
	nameRecordLen := 12
	nameRecordsLen := len(records) * nameRecordLen
	stringOffset := nameTableHeaderLen + nameRecordsLen
	rawStrings := make([][]byte, len(records))
	stringDataLen := 0
	for i, record := range records {
		raw := utf16BE(record.text)
		rawStrings[i] = raw
		stringDataLen += len(raw)
	}

	nameTable := make([]byte, stringOffset+stringDataLen)
	binary.BigEndian.PutUint16(nameTable[0:2], 0)
	binary.BigEndian.PutUint16(nameTable[2:4], uint16(len(records)))
	binary.BigEndian.PutUint16(nameTable[4:6], uint16(stringOffset))

	stringDataOffset := 0
	for i, record := range records {
		raw := rawStrings[i]
		offset := nameTableHeaderLen + i*nameRecordLen
		binary.BigEndian.PutUint16(nameTable[offset:offset+2], record.platformID)
		binary.BigEndian.PutUint16(nameTable[offset+2:offset+4], record.encodingID)
		binary.BigEndian.PutUint16(nameTable[offset+4:offset+6], record.languageID)
		binary.BigEndian.PutUint16(nameTable[offset+6:offset+8], record.nameID)
		binary.BigEndian.PutUint16(nameTable[offset+8:offset+10], uint16(len(raw)))
		binary.BigEndian.PutUint16(nameTable[offset+10:offset+12], uint16(stringDataOffset))
		copy(nameTable[stringOffset+stringDataOffset:], raw)
		stringDataOffset += len(raw)
	}

	nameTableOffset := 12 + 16
	font := make([]byte, nameTableOffset+len(nameTable))
	copy(font[0:4], []byte{0x00, 0x01, 0x00, 0x00})
	binary.BigEndian.PutUint16(font[4:6], 1)
	copy(font[12:16], "name")
	binary.BigEndian.PutUint32(font[20:24], uint32(nameTableOffset))
	binary.BigEndian.PutUint32(font[24:28], uint32(len(nameTable)))
	copy(font[nameTableOffset:], nameTable)
	return font
}

func utf16BE(text string) []byte {
	encoded := utf16.Encode([]rune(text))
	out := make([]byte, len(encoded)*2)
	for i, value := range encoded {
		binary.BigEndian.PutUint16(out[i*2:i*2+2], value)
	}
	return out
}
