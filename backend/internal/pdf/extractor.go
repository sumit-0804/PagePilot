package pdf

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/ledongthuc/pdf"
)

// ExtractedPage holds the text content and page number
type ExtractedPage struct {
	PageNumber int
	Text       string
}

// ExtractionResult holds extracted pages and statistics
type ExtractionResult struct {
	TotalPages int
	Pages      []ExtractedPage
	TotalText  string
	IsScanned  bool // true if very little text found
}

// ExtractText extracts text page-by-page from a PDF file
func ExtractText(filePath string) (*ExtractionResult, error) {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("file does not exist: %s", filePath)
	}

	f, r, err := pdf.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open PDF: %w", err)
	}
	defer f.Close()

	numPages := r.NumPage()
	if numPages <= 0 {
		return nil, errors.New("PDF contains no pages or is empty")
	}

	result := &ExtractionResult{
		TotalPages: numPages,
		Pages:      make([]ExtractedPage, 0, numPages),
	}

	var allTextBuilder strings.Builder

	for pageNum := 1; pageNum <= numPages; pageNum++ {
		page := r.Page(pageNum)
		if page.V.IsNull() {
			result.Pages = append(result.Pages, ExtractedPage{
				PageNumber: pageNum,
				Text:       "",
			})
			continue
		}

		text, err := page.GetPlainText(nil)
		if err != nil {
			// Some pages might fail while others succeed; preserve page index
			text = ""
		}

		cleaned := cleanText(text)
		result.Pages = append(result.Pages, ExtractedPage{
			PageNumber: pageNum,
			Text:       cleaned,
		})

		allTextBuilder.WriteString(cleaned)
		allTextBuilder.WriteString(" ")
	}

	result.TotalText = strings.TrimSpace(allTextBuilder.String())

	// Check if PDF has very little text (e.g. scanned images without OCR)
	if len(result.TotalText) < 20*numPages && len(result.TotalText) < 50 {
		result.IsScanned = true
	}

	return result, nil
}

// cleanText normalizes whitespace and line breaks
func cleanText(text string) string {
	lines := strings.Split(text, "\n")
	var cleanedLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			cleanedLines = append(cleanedLines, trimmed)
		}
	}
	return strings.Join(cleanedLines, "\n")
}
