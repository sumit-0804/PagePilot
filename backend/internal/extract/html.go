package extract

import (
	"fmt"
	"io"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Raw-text elements whose content is never shown to a reader.
var hiddenTags = map[atom.Atom]bool{
	atom.Script: true, atom.Style: true, atom.Noscript: true,
	atom.Iframe: true, atom.Noembed: true, atom.Noframes: true,
}

var paragraphTags = map[atom.Atom]bool{
	atom.P: true, atom.Div: true, atom.Section: true, atom.Article: true, atom.Main: true,
	atom.Header: true, atom.Footer: true, atom.Nav: true, atom.Aside: true,
	atom.H1: true, atom.H2: true, atom.H3: true, atom.H4: true, atom.H5: true, atom.H6: true,
	atom.Blockquote: true, atom.Pre: true, atom.Table: true, atom.Ul: true, atom.Ol: true,
	atom.Dl: true, atom.Figure: true, atom.Form: true, atom.Hr: true, atom.Title: true,
}

var lineTags = map[atom.Atom]bool{
	atom.Br: true, atom.Li: true, atom.Tr: true, atom.Dt: true, atom.Dd: true, atom.Figcaption: true,
}

// htmlToText strips tags, scripts and styles, keeping paragraph and line breaks.
func htmlToText(src string) (string, error) {
	z := html.NewTokenizer(strings.NewReader(src))
	var b strings.Builder
	hidden := false
	preDepth := 0

	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			if err := z.Err(); err != io.EOF {
				return "", fmt.Errorf("parse html: %w", err)
			}
			return tidyLines(b.String()), nil

		case html.TextToken:
			if hidden {
				continue
			}
			text := string(z.Text())
			if preDepth == 0 {
				text = strings.ReplaceAll(text, "\n", " ")
			}
			b.WriteString(text)
			continue

		case html.StartTagToken, html.SelfClosingTagToken, html.EndTagToken:
			name, _ := z.TagName()
			a := atom.Lookup(name)
			if a == atom.Pre {
				if tt == html.StartTagToken {
					preDepth++
				} else if tt == html.EndTagToken && preDepth > 0 {
					preDepth--
				}
			}
			switch {
			case paragraphTags[a]:
				b.WriteString("\n\n")
			case lineTags[a]:
				b.WriteString("\n")
			case a == atom.Td || a == atom.Th:
				b.WriteString(" ")
			}
			// A raw-text element's content arrives as the very next text token.
			hidden = tt == html.StartTagToken && hiddenTags[a]
			continue
		}
		hidden = false
	}
}

// tidyLines collapses runs of spaces and keeps at most one blank line between paragraphs.
func tidyLines(s string) string {
	var b strings.Builder
	blank := false
	for _, line := range strings.Split(s, "\n") {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			blank = true
			continue
		}
		if b.Len() > 0 {
			if blank {
				b.WriteString("\n\n")
			} else {
				b.WriteString("\n")
			}
		}
		b.WriteString(line)
		blank = false
	}
	return b.String()
}
