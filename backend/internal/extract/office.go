package extract

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Zip-bomb guards: caps on the uncompressed bytes read per entry and per document.
var (
	maxEntryBytes int64 = 64 << 20
	maxTotalBytes int64 = 256 << 20
)

var ErrTooLarge = errors.New("document is too large to extract")

var slideEntry = regexp.MustCompile(`^ppt/slides/slide(\d+)\.xml$`)

// ExtractOffice reads the text of a DOCX or PPTX file.
func ExtractOffice(filePath string, f Format) ([]Section, error) {
	if f != DOCX && f != PPTX {
		return nil, fmt.Errorf("%w: %q is not an Office format", ErrUnsupportedFormat, f)
	}
	zr, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, fmt.Errorf("open %s archive: %w", f, err)
	}
	defer zr.Close()

	z := &zipEntries{files: make(map[string]*zip.File, len(zr.File)), budget: maxTotalBytes}
	for _, file := range zr.File {
		z.files[file.Name] = file
	}
	if f == DOCX {
		return extractDOCX(z)
	}
	return extractPPTX(z)
}

type zipEntries struct {
	files  map[string]*zip.File
	budget int64
}

// read returns nil data and no error when the entry does not exist.
func (z *zipEntries) read(name string) ([]byte, error) {
	f, ok := z.files[name]
	if !ok {
		return nil, nil
	}
	limit := min(maxEntryBytes, z.budget)
	if f.UncompressedSize64 > uint64(limit) {
		return nil, fmt.Errorf("%w: %s", ErrTooLarge, name)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", name, err)
	}
	defer rc.Close()

	// The size in the zip header can lie, so enforce the limit while reading too.
	data, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%w: %s", ErrTooLarge, name)
	}
	z.budget -= int64(len(data))
	return data, nil
}

func extractDOCX(z *zipEntries) ([]Section, error) {
	doc, err := z.read("word/document.xml")
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, errors.New("invalid docx: missing word/document.xml")
	}
	styles, err := z.read("word/styles.xml")
	if err != nil {
		return nil, err
	}
	headings, err := docxHeadingStyles(styles)
	if err != nil {
		return nil, err
	}
	blocks, err := docxBlocks(doc, headings)
	if err != nil {
		return nil, err
	}
	return toSections(blocks, LabelSection)
}

// docxHeadingStyles maps style IDs whose display name is a heading or title.
func docxHeadingStyles(data []byte) (map[string]bool, error) {
	headings := map[string]bool{}
	if data == nil {
		return headings, nil
	}
	var doc struct {
		Styles []struct {
			ID   string `xml:"styleId,attr"`
			Name struct {
				Val string `xml:"val,attr"`
			} `xml:"name"`
		} `xml:"style"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse docx styles: %w", err)
	}
	for _, s := range doc.Styles {
		if isHeadingName(s.Name.Val) {
			headings[s.ID] = true
		}
	}
	return headings, nil
}

func isHeadingName(name string) bool {
	n := strings.ToLower(name)
	return strings.HasPrefix(n, "heading") || n == "title"
}

type docxParagraph struct {
	text    strings.Builder
	heading bool
}

// docxBlocks returns the body text split into blocks that each start at a heading.
func docxBlocks(data []byte, headings map[string]bool) ([]string, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var blocks []string
	var cur strings.Builder
	var stack []*docxParagraph // text boxes nest paragraphs inside paragraphs
	inText := false
	fallback := 0 // mc:Fallback repeats content already given in mc:Choice

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse docx: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "Fallback" {
				fallback++
			}
			if fallback > 0 {
				continue
			}
			if t.Name.Local == "p" {
				stack = append(stack, &docxParagraph{})
				continue
			}
			if len(stack) == 0 {
				continue
			}
			top := stack[len(stack)-1]
			switch t.Name.Local {
			case "pStyle":
				id := attr(t, "val")
				top.heading = headings[id] || isHeadingName(id)
			case "t":
				inText = true
			case "tab":
				top.text.WriteByte('\t')
			case "br", "cr":
				top.text.WriteByte('\n')
			}

		case xml.EndElement:
			if t.Name.Local == "Fallback" {
				fallback--
				continue
			}
			if fallback > 0 {
				continue
			}
			switch t.Name.Local {
			case "t":
				inText = false
			case "p":
				if len(stack) == 0 {
					continue
				}
				p := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				text := strings.TrimSpace(p.text.String())
				if text == "" {
					continue
				}
				if p.heading && cur.Len() > 0 {
					blocks = append(blocks, cur.String())
					cur.Reset()
				}
				cur.WriteString(text)
				cur.WriteString("\n\n")
			}

		case xml.CharData:
			if inText && fallback == 0 && len(stack) > 0 {
				stack[len(stack)-1].text.Write(t)
			}
		}
	}
	return append(blocks, cur.String()), nil
}

func extractPPTX(z *zipEntries) ([]Section, error) {
	type slide struct {
		num  int
		name string
	}
	var slides []slide
	for name := range z.files {
		m := slideEntry.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		slides = append(slides, slide{num: n, name: name})
	}
	sort.Slice(slides, func(i, j int) bool { return slides[i].num < slides[j].num })

	var sections []Section
	for i, s := range slides {
		data, err := z.read(s.name)
		if err != nil {
			return nil, err
		}
		text, err := drawingText(data)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", s.name, err)
		}
		notes, err := slideNotes(z, s.name)
		if err != nil {
			return nil, err
		}
		if notes != "" {
			text = strings.TrimSpace(text + "\n\nNotes:\n" + notes)
		}
		if text == "" {
			continue
		}
		sections = append(sections, Section{Number: i + 1, Label: LabelSlide, Text: text})
	}
	if len(sections) == 0 {
		return nil, ErrNoText
	}
	return sections, nil
}

// slideNotes follows the slide's relationships to its notes slide, if it has one.
func slideNotes(z *zipEntries, slideName string) (string, error) {
	relsName := path.Join(path.Dir(slideName), "_rels", path.Base(slideName)+".rels")
	data, err := z.read(relsName)
	if err != nil || data == nil {
		return "", err
	}
	var rels struct {
		Items []struct {
			Type   string `xml:"Type,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	if err := xml.Unmarshal(data, &rels); err != nil {
		return "", fmt.Errorf("parse %s: %w", relsName, err)
	}
	for _, r := range rels.Items {
		if !strings.HasSuffix(r.Type, "/notesSlide") {
			continue
		}
		target := path.Join(path.Dir(slideName), r.Target)
		if strings.HasPrefix(r.Target, "/") {
			target = strings.TrimPrefix(path.Clean(r.Target), "/")
		}
		notes, err := z.read(target)
		if err != nil || notes == nil {
			return "", err
		}
		text, err := drawingText(notes)
		if err != nil {
			return "", fmt.Errorf("parse %s: %w", target, err)
		}
		return text, nil
	}
	return "", nil
}

// drawingText collects a:t runs, one line per a:p, skipping slide-number fields.
func drawingText(data []byte) (string, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var lines []string
	var line strings.Builder
	inText := false
	depth := 0
	skipAt := 0 // depth of the element being skipped, 0 when not skipping

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if skipAt > 0 {
				continue
			}
			switch {
			case t.Name.Local == "Fallback",
				t.Name.Local == "fld" && attr(t, "type") == "slidenum":
				skipAt = depth
			case t.Name.Local == "t":
				inText = true
			case t.Name.Local == "br":
				line.WriteByte('\n')
			}
		case xml.EndElement:
			depth--
			if skipAt > 0 {
				if depth < skipAt {
					skipAt = 0
				}
				continue
			}
			switch t.Name.Local {
			case "t":
				inText = false
			case "p":
				if s := strings.TrimSpace(line.String()); s != "" {
					lines = append(lines, s)
				}
				line.Reset()
			}
		case xml.CharData:
			if inText && skipAt == 0 {
				line.Write(t)
			}
		}
	}
	return strings.Join(lines, "\n"), nil
}

func attr(e xml.StartElement, local string) string {
	for _, a := range e.Attr {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}
