package extract

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	LabelPage    = "Page"
	LabelSlide   = "Slide"
	LabelSection = "Section"
)

// sectionChars is the rough size (in bytes) at which long text is split into sections.
const sectionChars = 3000

// ErrNoText means the document parsed fine but contained no readable text.
var ErrNoText = errors.New("no extractable text")

type Section struct {
	Number int
	Label  string
	Text   string
}

var (
	blankLines = regexp.MustCompile(`\n[ \t]*\n`)
	mdHeading  = regexp.MustCompile(`^ {0,3}#{1,6}(\s|$)`)
)

// ExtractText splits a TXT, MD, CSV or HTML document into sections.
func ExtractText(data []byte, f Format) ([]Section, error) {
	text := normalize(string(data))

	var blocks []string
	switch f {
	case TXT, CSV:
		blocks = []string{text}
	case MD:
		blocks = splitMarkdown(text)
	case HTML:
		plain, err := htmlToText(text)
		if err != nil {
			return nil, err
		}
		blocks = []string{plain}
	default:
		return nil, fmt.Errorf("%w: %q is not a text format", ErrUnsupportedFormat, f)
	}
	return toSections(blocks, LabelSection)
}

func toSections(blocks []string, label string) ([]Section, error) {
	var sections []Section
	for _, block := range blocks {
		for _, chunk := range chunkText(block, sectionChars) {
			sections = append(sections, Section{Number: len(sections) + 1, Label: label, Text: chunk})
		}
	}
	if len(sections) == 0 {
		return nil, ErrNoText
	}
	return sections, nil
}

// normalize makes text safe to store in Postgres and uses \n line endings.
func normalize(s string) string {
	s = strings.ToValidUTF8(s, "�")
	s = strings.TrimPrefix(s, "\xef\xbb\xbf") // UTF-8 BOM
	s = strings.ReplaceAll(s, "\x00", "")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

// splitMarkdown starts a new block at every heading outside fenced code.
func splitMarkdown(text string) []string {
	var blocks []string
	var cur strings.Builder
	inFence := false
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~"):
			inFence = !inFence
		case !inFence && mdHeading.MatchString(line) && cur.Len() > 0:
			blocks = append(blocks, cur.String())
			cur.Reset()
		}
		cur.WriteString(line)
		cur.WriteByte('\n')
	}
	return append(blocks, cur.String())
}

// chunkText packs paragraphs into chunks of about limit bytes.
// Oversized paragraphs fall back to line breaks, then to word or rune boundaries.
func chunkText(text string, limit int) []string {
	var chunks []string
	var buf strings.Builder
	flush := func() {
		if s := strings.Trim(buf.String(), "\n"); strings.TrimSpace(s) != "" {
			chunks = append(chunks, s)
		}
		buf.Reset()
	}
	add := func(piece, sep string) {
		if buf.Len() > 0 && buf.Len()+len(sep)+len(piece) > limit {
			flush()
		}
		if buf.Len() > 0 {
			buf.WriteString(sep)
		}
		buf.WriteString(piece)
	}

	for _, para := range blankLines.Split(text, -1) {
		para = strings.Trim(para, "\n")
		if strings.TrimSpace(para) == "" {
			continue
		}
		if len(para) <= limit {
			add(para, "\n\n")
			continue
		}
		sep := "\n\n"
		for _, line := range strings.Split(para, "\n") {
			for _, piece := range hardSplit(line, limit) {
				add(piece, sep)
				sep = "\n"
			}
		}
	}
	flush()
	return chunks
}

func hardSplit(s string, limit int) []string {
	var parts []string
	for len(s) > limit {
		cut := strings.LastIndexAny(s[:limit], " \t")
		if cut < limit/2 {
			cut = limit
			for cut > 1 && !utf8.RuneStart(s[cut]) {
				cut--
			}
		}
		parts = append(parts, s[:cut])
		s = strings.TrimLeft(s[cut:], " \t")
	}
	if s != "" {
		parts = append(parts, s)
	}
	return parts
}
