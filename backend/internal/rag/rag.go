package rag

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/pagepilot/backend/internal/embeddings"
	"github.com/pagepilot/backend/internal/models"
	"github.com/pagepilot/backend/internal/vectorstore"
)

type RAGResult struct {
	Answer    string            `json:"answer"`
	Citations []models.Citation `json:"citations"`
}

type Engine struct {
	embedder    embeddings.Embedder
	vectorStore vectorstore.VectorStore
	llmClient   LLMClient
}

func NewEngine(embedder embeddings.Embedder, vs vectorstore.VectorStore, llm LLMClient) *Engine {
	return &Engine{
		embedder:    embedder,
		vectorStore: vs,
		llmClient:   llm,
	}
}

// Retrieve finds the chunks of one user's document most relevant to the query.
func (e *Engine) Retrieve(ctx context.Context, query, userID, docID string, limit int) ([]vectorstore.SearchResult, error) {
	queryVec, err := e.embedder.EmbedQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}
	results, err := e.vectorStore.Search(ctx, queryVec, userID, docID, limit)
	if err != nil {
		return nil, fmt.Errorf("vector search failed: %w", err)
	}
	return results, nil
}

// Query performs end-to-end grounded RAG retrieval and answer generation
func (e *Engine) Query(ctx context.Context, query string, docID, userID string, chatHistory []ChatMessage) (*RAGResult, error) {
	searchResults, err := e.Retrieve(ctx, query, userID, docID, 5)
	if err != nil {
		return nil, err
	}

	// 3. Assemble prompt and messages
	messages, citations := e.BuildMessages(query, searchResults, chatHistory)

	// 4. Generate response using LLM
	answer, err := e.llmClient.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("LLM generation failed: %w", err)
	}

	answer = normalizeCitations(answer)
	return &RAGResult{
		Answer:    answer,
		Citations: filterRelevantCitations(answer, citations),
	}, nil
}

// StreamQuery performs RAG with token-by-token streaming
func (e *Engine) StreamQuery(
	ctx context.Context,
	query string,
	docID, userID string,
	chatHistory []ChatMessage,
	onChunk func(token string) error,
) (*RAGResult, error) {
	searchResults, err := e.Retrieve(ctx, query, userID, docID, 5)
	if err != nil {
		return nil, err
	}

	// 3. Build messages
	messages, citations := e.BuildMessages(query, searchResults, chatHistory)

	// 4. Stream tokens
	answer, err := e.llmClient.Stream(ctx, messages, onChunk)
	if err != nil {
		return nil, fmt.Errorf("streaming error: %w", err)
	}

	// Streamed tokens went out raw; the saved answer is normalized, and the UI accepts both forms.
	answer = normalizeCitations(answer)
	return &RAGResult{
		Answer:    answer,
		Citations: filterRelevantCitations(answer, citations),
	}, nil
}

// BuildMessages creates the grounding prompt, injected context chunks, and conversation history
func (e *Engine) BuildMessages(
	query string,
	chunks []vectorstore.SearchResult,
	history []ChatMessage,
) ([]ChatMessage, []models.Citation) {
	var contextBuilder strings.Builder
	var citations []models.Citation
	seenPages := make(map[int]bool)

	if len(chunks) == 0 {
		contextBuilder.WriteString("(No specific excerpts found for this query in the document.)\n")
	} else {
		for i, c := range chunks {
			contextBuilder.WriteString(fmt.Sprintf("--- EXCERPT %d (Page %d, Document: %s) ---\n%s\n\n",
				i+1, c.PageNumber, c.DocumentName, c.ChunkText))

			if !seenPages[c.PageNumber] {
				seenPages[c.PageNumber] = true
				snippet := c.ChunkText
				if len(snippet) > 160 {
					snippet = snippet[:160] + "..."
				}
				citations = append(citations, models.Citation{
					DocumentID:   c.DocumentID,
					DocumentName: c.DocumentName,
					PageNumber:   c.PageNumber,
					Snippet:      snippet,
				})
			}
		}
	}

	systemPrompt := fmt.Sprintf(`You are PagePilot, an elite AI Study Companion and academic tutor.
Your mission is to help the student learn and understand concepts deeply, grounded directly in their uploaded PDF.

STRICT GROUNDING RULES:
1. Ground your answers strictly in the provided DOCUMENT EXCERPTS below.
2. Prioritize factual correctness and clarity based on the excerpts.
3. Every claim supported by the excerpts MUST cite the exact page number in square brackets, exactly like [Page X].
4. Never invent page numbers or references.
5. If the user's question cannot be answered using the provided excerpts, clearly state that the document does not contain this information, then offer what you know generally while clearly labeling it as general knowledge outside the text.
6. Adopt an encouraging, articulate, pedagogical tone. Use clear headings, bullet points, and KaTeX math formatting ($...$ or $$...$$) where applicable.

DOCUMENT EXCERPTS:
%s
`, contextBuilder.String())

	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
	}

	// Add up to 6 recent history turns for conversational context
	startIdx := 0
	if len(history) > 6 {
		startIdx = len(history) - 6
	}
	for i := startIdx; i < len(history); i++ {
		messages = append(messages, history[i])
	}

	// Add current query
	messages = append(messages, ChatMessage{
		Role:    "user",
		Content: query,
	})

	return messages, citations
}

// Some models (e.g. gpt-oss) write 【Page 2】 instead of [Page 2].
var bracketNormalizer = strings.NewReplacer("【", "[", "】", "]", "［", "[", "］", "]")

func normalizeCitations(answer string) string {
	return bracketNormalizer.Replace(answer)
}

// Matches [Page 3], [page 3], [Pages 3, 5] and [Pages 3 and 5]. Spaces may be Unicode ones like
// U+202F, which gpt-oss emits and Go's \s does not cover.
var citationRe = regexp.MustCompile(`(?i)\[[\s\p{Zs}]*pages?[\s\p{Zs}]*(\d+(?:[\s\p{Zs}]*(?:,|and|&)[\s\p{Zs}]*\d+)*)[\s\p{Zs}]*\]`)
var digitsRe = regexp.MustCompile(`\d+`)

// filterRelevantCitations matches cited pages in the text to return accurate references
func filterRelevantCitations(answer string, candidates []models.Citation) []models.Citation {
	if len(candidates) == 0 {
		return candidates
	}

	matches := citationRe.FindAllStringSubmatch(normalizeCitations(answer), -1)
	if len(matches) == 0 {
		// The answer cites nothing, so attaching sources would claim support it doesn't have.
		return nil
	}

	citedPages := make(map[int]bool)
	for _, m := range matches {
		for _, num := range digitsRe.FindAllString(m[1], -1) {
			if p, err := strconv.Atoi(num); err == nil {
				citedPages[p] = true
			}
		}
	}

	// Only pages that were actually retrieved can be cited; invented page numbers are dropped.
	var filtered []models.Citation
	for _, c := range candidates {
		if citedPages[c.PageNumber] {
			filtered = append(filtered, c)
		}
	}
	return filtered
}
