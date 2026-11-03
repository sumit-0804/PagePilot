package pdf

import (
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/pagepilot/backend/internal/models"
)

// ChunkerConfig specifies chunk size and overlap parameters
type ChunkerConfig struct {
	TargetChunkSize int // Target characters per chunk (e.g. 600)
	OverlapSize     int // Overlap characters between chunks (e.g. 100)
}

func DefaultChunkerConfig() ChunkerConfig {
	return ChunkerConfig{
		TargetChunkSize: 650,
		OverlapSize:     100,
	}
}

// ChunkDocument takes the extracted pages of a document and breaks them into
// semantically coherent chunks respecting page boundaries and sentence ends.
func ChunkDocument(docID, userID, docName string, pages []ExtractedPage, cfg ChunkerConfig) []models.DocumentChunk {
	if cfg.TargetChunkSize <= 0 {
		cfg.TargetChunkSize = 650
	}
	if cfg.OverlapSize < 0 || cfg.OverlapSize >= cfg.TargetChunkSize {
		cfg.OverlapSize = 100
	}

	var chunks []models.DocumentChunk
	globalChunkIndex := 0

	for _, page := range pages {
		pageText := strings.TrimSpace(page.Text)
		if pageText == "" {
			continue
		}

		pageChunks := chunkText(pageText, cfg.TargetChunkSize, cfg.OverlapSize)
		for _, ct := range pageChunks {
			trimmed := strings.TrimSpace(ct)
			if len(trimmed) < 20 { // skip trivial empty fragments
				continue
			}

			chunks = append(chunks, models.DocumentChunk{
				ID:           uuid.New().String(),
				DocumentID:   docID,
				UserID:       userID,
				PageNumber:   page.PageNumber,
				ChunkIndex:   globalChunkIndex,
				ChunkText:    trimmed,
				DocumentName: docName,
			})
			globalChunkIndex++
		}
	}

	return chunks
}

// chunkText divides text into blocks keeping paragraph/sentence integrity where possible
func chunkText(text string, targetSize, overlap int) []string {
	if len(text) <= targetSize {
		return []string{text}
	}

	var chunks []string
	paragraphs := strings.Split(text, "\n")
	var currentChunk strings.Builder

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}

		// If paragraph alone is longer than targetSize, split by sentences
		if len(para) > targetSize {
			if currentChunk.Len() > 0 {
				chunks = append(chunks, currentChunk.String())
				currentChunk.Reset()
			}

			sentences := splitSentences(para)
			var subChunk strings.Builder

			for _, s := range sentences {
				if subChunk.Len()+len(s) > targetSize && subChunk.Len() > 0 {
					chunks = append(chunks, strings.TrimSpace(subChunk.String()))
					// Keep overlap from tail of previous subChunk
					overlapText := extractOverlap(subChunk.String(), overlap)
					subChunk.Reset()
					subChunk.WriteString(overlapText)
				}
				if subChunk.Len() > 0 {
					subChunk.WriteString(" ")
				}
				subChunk.WriteString(s)
			}
			if subChunk.Len() > 0 {
				chunks = append(chunks, strings.TrimSpace(subChunk.String()))
			}
			continue
		}

		// Check if adding this paragraph exceeds target size
		if currentChunk.Len()+len(para) > targetSize && currentChunk.Len() > 0 {
			chunks = append(chunks, strings.TrimSpace(currentChunk.String()))
			overlapText := extractOverlap(currentChunk.String(), overlap)
			currentChunk.Reset()
			currentChunk.WriteString(overlapText)
		}

		if currentChunk.Len() > 0 {
			currentChunk.WriteString("\n")
		}
		currentChunk.WriteString(para)
	}

	if currentChunk.Len() > 0 {
		chunks = append(chunks, strings.TrimSpace(currentChunk.String()))
	}

	return chunks
}

func splitSentences(text string) []string {
	var sentences []string
	var current strings.Builder

	runes := []rune(text)
	n := len(runes)

	for i := 0; i < n; i++ {
		r := runes[i]
		current.WriteRune(r)

		if (r == '.' || r == '!' || r == '?') && i+1 < n && unicode.IsSpace(runes[i+1]) {
			sentences = append(sentences, strings.TrimSpace(current.String()))
			current.Reset()
			i++ // skip following space
		}
	}

	if current.Len() > 0 {
		sentences = append(sentences, strings.TrimSpace(current.String()))
	}

	return sentences
}

func extractOverlap(text string, overlap int) string {
	if len(text) <= overlap {
		return text
	}
	// Try to find a space or word boundary in the overlap window
	start := len(text) - overlap
	spaceIdx := strings.Index(text[start:], " ")
	if spaceIdx != -1 && spaceIdx < overlap/2 {
		return text[start+spaceIdx+1:]
	}
	return text[start:]
}
