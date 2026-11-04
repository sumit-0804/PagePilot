package extract

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestDetectFormat(t *testing.T) {
	zipHeader := []byte("PK\x03\x04rest")
	tests := []struct {
		name     string
		header   []byte
		filename string
		want     Format
		wantErr  bool
	}{
		{"pdf", []byte("%PDF-1.7\n"), "a.pdf", PDF, false},
		{"pdf magic wins over name", []byte("%PDF-1.4"), "notes.txt", PDF, false},
		{"docx", zipHeader, "Report.DOCX", DOCX, false},
		{"pptx", zipHeader, "deck.pptx", PPTX, false},
		{"zip without office name", zipHeader, "bundle.zip", "", true},
		{"xlsx is not supported", zipHeader, "sheet.xlsx", "", true},
		{"docx name without zip magic", []byte("hello"), "fake.docx", "", true},
		{"txt", []byte("hello world"), "a.txt", TXT, false},
		{"markdown", []byte("# Title"), "README.md", MD, false},
		{"csv", []byte("a,b\n1,2"), "data.csv", CSV, false},
		{"htm", []byte("<html>"), "page.htm", HTML, false},
		{"go code as text", []byte("package main"), "main.go", TXT, false},
		{"tsx code as text", []byte("export {}"), "App.tsx", TXT, false},
		{"empty text file", []byte{}, "empty.txt", TXT, false},
		{"header cut mid-rune", []byte("caf\xc3"), "a.txt", TXT, false},
		{"invalid utf-8", []byte("bad \xff\xfe bytes"), "a.txt", "", true},
		{"nul byte", []byte("a\x00b"), "a.txt", "", true},
		{"unknown extension", []byte("hello"), "a.exe", "", true},
		{"no extension", []byte("hello"), "Makefile", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DetectFormat(tt.header, tt.filename)
			if tt.wantErr {
				if !errors.Is(err, ErrUnsupportedFormat) {
					t.Fatalf("want ErrUnsupportedFormat, got %v (%q)", err, got)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("got %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestExtractText(t *testing.T) {
	para := strings.Repeat("word ", 200) // ~1000 bytes
	longTxt := strings.Join([]string{para, para, para, para, para}, "\n\n")
	var csv strings.Builder
	for i := 0; i < 300; i++ {
		csv.WriteString("id,name,some longer value here\n")
	}

	tests := []struct {
		name    string
		data    string
		format  Format
		want    []string // expected section texts; nil means only check count
		count   int
		wantErr error
	}{
		{
			name:   "short txt is one section with CRLF normalised",
			data:   "\uFEFFline one\r\nline two\r\n\r\nnext para",
			format: TXT,
			want:   []string{"line one\nline two\n\nnext para"},
		},
		{
			name:   "long txt splits on paragraphs",
			data:   longTxt,
			format: TXT,
			count:  3,
		},
		{
			name:   "csv without blank lines splits on lines",
			data:   csv.String(),
			format: CSV,
			count:  4,
		},
		{
			name:   "markdown splits on headings outside code fences",
			data:   "intro text\n\n# One\nbody one\n```\n# not a heading\n```\n## Two\nbody two\n#hashtag stays",
			format: MD,
			want: []string{
				"intro text",
				"# One\nbody one\n```\n# not a heading\n```",
				"## Two\nbody two\n#hashtag stays",
			},
		},
		{
			name: "html drops scripts, styles and tags",
			data: `<html><head><title>T</title><style>p{color:red}</style>
<script>alert("x")</script></head><body><h1>Hello &amp; welcome</h1>
<p>First   para
continues</p><p>Second<br>line</p><noscript>enable js</noscript><!-- note --></body></html>`,
			format: HTML,
			want:   []string{"T\n\nHello & welcome\n\nFirst para continues\n\nSecond\nline"},
		},
		{name: "whitespace only", data: " \n\n\t\n", format: TXT, wantErr: ErrNoText},
		{name: "pdf is not handled here", data: "%PDF-", format: PDF, wantErr: ErrUnsupportedFormat},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractText([]byte(tt.data), tt.format)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("want %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			checkNumbering(t, got, LabelSection)
			if tt.want != nil {
				assertTexts(t, got, tt.want)
				return
			}
			if len(got) != tt.count {
				t.Fatalf("got %d sections, want %d", len(got), tt.count)
			}
			for _, s := range got {
				if len(s.Text) > sectionChars {
					t.Errorf("section %d is %d bytes, over %d", s.Number, len(s.Text), sectionChars)
				}
			}
		})
	}
}

func TestChunkTextHardSplit(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		limit int
	}{
		{"multi-byte runes, no spaces", strings.Repeat("é日", 50), 17},
		{"long words with spaces", strings.Repeat("abcdefgh ", 30), 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := chunkText(tt.text, tt.limit)
			if len(chunks) < 2 {
				t.Fatalf("expected several chunks, got %d", len(chunks))
			}
			for _, c := range chunks {
				if len(c) > tt.limit {
					t.Errorf("chunk %q is over the %d byte limit", c, tt.limit)
				}
				if !utf8.ValidString(c) {
					t.Errorf("chunk %q split a rune", c)
				}
			}
		})
	}
}

const (
	wNS = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006"`
	pNS = `xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"`
)

func wPara(style, text string) string {
	ppr := ""
	if style != "" {
		ppr = `<w:pPr><w:pStyle w:val="` + style + `"/></w:pPr>`
	}
	return `<w:p>` + ppr + `<w:r><w:t xml:space="preserve">` + text + `</w:t></w:r></w:p>`
}

func docxDocument(body string) string {
	return `<?xml version="1.0" encoding="UTF-8"?><w:document ` + wNS + `><w:body>` + body + `</w:body></w:document>`
}

func slideXML(paragraphs ...string) string {
	var b strings.Builder
	b.WriteString(`<p:sld ` + pNS + `><p:cSld><p:spTree><p:sp><p:txBody>`)
	for _, p := range paragraphs {
		b.WriteString(`<a:p><a:r><a:t>` + p + `</a:t></a:r></a:p>`)
	}
	b.WriteString(`</p:txBody></p:sp></p:spTree></p:cSld></p:sld>`)
	return b.String()
}

func TestExtractOffice(t *testing.T) {
	longBody := strings.Repeat(wPara("", strings.Repeat("filler ", 100)), 10) // ~7000 bytes

	docx := map[string]string{
		"word/styles.xml": `<w:styles ` + wNS + `><w:style w:styleId="MyHead"><w:name w:val="heading 2"/></w:style>` +
			`<w:style w:styleId="Normal"><w:name w:val="Normal"/></w:style></w:styles>`,
		"word/document.xml": docxDocument(
			wPara("", "Intro paragraph") +
				wPara("Heading1", "Chapter One") +
				`<w:p><w:r><w:t>Split </w:t></w:r><w:r><w:t>runs</w:t><w:tab/><w:t>tabbed</w:t><w:br/><w:t>broken</w:t></w:r></w:p>` +
				`<w:p><w:r><mc:AlternateContent><mc:Choice><w:t>Box text</w:t></mc:Choice>` +
				`<mc:Fallback><w:p><w:r><w:t>Box text</w:t></w:r></w:p></mc:Fallback></mc:AlternateContent></w:r></w:p>` +
				wPara("MyHead", "Custom Heading") +
				wPara("", "Under custom")),
	}

	pptx := map[string]string{
		"ppt/slides/slide10.xml": slideXML("Tenth slide"),
		"ppt/slides/slide1.xml":  slideXML("Title slide", "Subtitle"),
		"ppt/slides/slide2.xml":  slideXML("Second"),
		"ppt/slides/slide3.xml":  slideXML(),
		"ppt/slides/_rels/slide2.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout" Target="../slideLayouts/slideLayout1.xml"/>` +
			`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/notesSlide" Target="../notesSlides/notesSlide7.xml"/></Relationships>`,
		"ppt/notesSlides/notesSlide7.xml": `<p:notes ` + pNS + `><p:cSld><p:spTree><p:sp><p:txBody>` +
			`<a:p><a:r><a:t>Remember the demo</a:t></a:r></a:p>` +
			`<a:p><a:fld type="slidenum"><a:t>2</a:t></a:fld></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:notes>`,
	}

	tests := []struct {
		name    string
		files   map[string]string
		format  Format
		label   string
		want    []string
		numbers []int
		count   int
		wantErr error
	}{
		{
			name:   "docx splits at heading styles",
			files:  docx,
			format: DOCX,
			label:  LabelSection,
			want: []string{
				"Intro paragraph",
				"Chapter One\n\nSplit runs\ttabbed\nbroken\n\nBox text",
				"Custom Heading\n\nUnder custom",
			},
		},
		{
			name:   "docx without headings splits by size",
			files:  map[string]string{"word/document.xml": docxDocument(longBody)},
			format: DOCX,
			label:  LabelSection,
			count:  3,
		},
		{
			name:    "pptx sorts slides numerically and appends notes",
			files:   pptx,
			format:  PPTX,
			label:   LabelSlide,
			want:    []string{"Title slide\nSubtitle", "Second\n\nNotes:\nRemember the demo", "Tenth slide"},
			numbers: []int{1, 2, 4},
		},
		{name: "docx missing document.xml", files: map[string]string{"x.xml": "<x/>"}, format: DOCX},
		{name: "empty docx body", files: map[string]string{"word/document.xml": docxDocument("")}, format: DOCX, wantErr: ErrNoText},
		{name: "pptx with no slides", files: map[string]string{"ppt/presentation.xml": "<p/>"}, format: PPTX, wantErr: ErrNoText},
		{name: "text format rejected", files: docx, format: TXT, wantErr: ErrUnsupportedFormat},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractOffice(writeZip(t, tt.files), tt.format)
			if tt.wantErr != nil || (tt.want == nil && tt.count == 0) {
				if err == nil || (tt.wantErr != nil && !errors.Is(err, tt.wantErr)) {
					t.Fatalf("want error %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tt.count > 0 && len(got) != tt.count {
				t.Fatalf("got %d sections, want %d", len(got), tt.count)
			}
			if tt.want != nil {
				assertTexts(t, got, tt.want)
			}
			for i, s := range got {
				if s.Label != tt.label {
					t.Errorf("section %d label %q, want %q", i, s.Label, tt.label)
				}
				wantNum := i + 1
				if tt.numbers != nil {
					wantNum = tt.numbers[i]
				}
				if s.Number != wantNum {
					t.Errorf("section %d number %d, want %d", i, s.Number, wantNum)
				}
			}
		})
	}
}

func TestExtractOfficeZipBombGuard(t *testing.T) {
	path := writeZip(t, map[string]string{"word/document.xml": docxDocument(strings.Repeat(wPara("", "zzzz"), 2000))})
	tests := []struct {
		name         string
		entry, total int64
	}{
		{"per-entry cap", 1024, 1 << 20},
		{"per-document cap", 1 << 20, 1024},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldEntry, oldTotal := maxEntryBytes, maxTotalBytes
			maxEntryBytes, maxTotalBytes = tt.entry, tt.total
			t.Cleanup(func() { maxEntryBytes, maxTotalBytes = oldEntry, oldTotal })

			if _, err := ExtractOffice(path, DOCX); !errors.Is(err, ErrTooLarge) {
				t.Fatalf("want ErrTooLarge, got %v", err)
			}
		})
	}
}

func writeZip(t *testing.T, files map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "doc.zip")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertTexts(t *testing.T, got []Section, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d sections %+v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i].Text != want[i] {
			t.Errorf("section %d:\n got %q\nwant %q", i, got[i].Text, want[i])
		}
	}
}

func checkNumbering(t *testing.T, got []Section, label string) {
	t.Helper()
	for i, s := range got {
		if s.Number != i+1 || s.Label != label {
			t.Errorf("section %d has number %d label %q", i, s.Number, s.Label)
		}
	}
}
