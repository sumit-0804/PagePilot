// Package extract detects upload formats and pulls text out of non-PDF documents.
package extract

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type Format string

const (
	PDF  Format = "pdf"
	DOCX Format = "docx"
	PPTX Format = "pptx"
	TXT  Format = "txt"
	MD   Format = "md"
	CSV  Format = "csv"
	HTML Format = "html"
)

var ErrUnsupportedFormat = errors.New("unsupported file format")

var (
	pdfMagic = []byte("%PDF-")
	zipMagic = []byte("PK\x03\x04")
)

// Source code and config files are indexed as plain text.
var textExtensions = map[string]Format{
	".txt": TXT, ".text": TXT, ".log": TXT, ".tsv": TXT,
	".md": MD, ".markdown": MD,
	".csv":  CSV,
	".html": HTML, ".htm": HTML,

	".go": TXT, ".py": TXT, ".js": TXT, ".jsx": TXT, ".mjs": TXT, ".ts": TXT, ".tsx": TXT,
	".java": TXT, ".kt": TXT, ".kts": TXT, ".scala": TXT, ".swift": TXT, ".dart": TXT,
	".c": TXT, ".h": TXT, ".cc": TXT, ".cpp": TXT, ".hpp": TXT, ".cs": TXT,
	".rb": TXT, ".rs": TXT, ".php": TXT, ".lua": TXT, ".r": TXT, ".sql": TXT,
	".sh": TXT, ".bash": TXT, ".ps1": TXT,
	".json": TXT, ".yaml": TXT, ".yml": TXT, ".toml": TXT, ".xml": TXT, ".ini": TXT, ".cfg": TXT,
	".css": TXT, ".scss": TXT, ".vue": TXT, ".svelte": TXT,
}

// DetectFormat identifies an upload from its first bytes and file name.
// Binary formats are matched by magic bytes; text formats need a known extension and UTF-8 content.
func DetectFormat(header []byte, filename string) (Format, error) {
	ext := strings.ToLower(filepath.Ext(filename))

	if bytes.HasPrefix(header, pdfMagic) {
		return PDF, nil
	}
	if bytes.HasPrefix(header, zipMagic) {
		switch ext {
		case ".docx":
			return DOCX, nil
		case ".pptx":
			return PPTX, nil
		}
		return "", ErrUnsupportedFormat
	}
	if f, ok := textExtensions[ext]; ok && looksLikeText(header) {
		return f, nil
	}
	return "", ErrUnsupportedFormat
}

func looksLikeText(b []byte) bool {
	if bytes.IndexByte(b, 0) >= 0 {
		return false
	}
	// The header may end in the middle of a multi-byte character; drop that partial rune.
	for i := len(b) - 1; i >= 0 && i >= len(b)-utf8.UTFMax; i-- {
		if utf8.RuneStart(b[i]) {
			if !utf8.FullRune(b[i:]) {
				b = b[:i]
			}
			break
		}
	}
	return utf8.Valid(b)
}
