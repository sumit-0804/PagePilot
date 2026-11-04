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

// AskParams describes one grounded question about a single document.
type AskParams struct {
	UserID     string
	DocumentID string
	Question   string // what the user typed; shown to the model
	// SearchQuery is a standalone rewrite used for retrieval, so follow-ups like
	// "explain that more simply" still find the right pages. Defaults to Question.
	SearchQuery string
	Style       string // one of ExplanationStyles, or "" for the default
	History     []ChatMessage
}

// ExplanationStyles maps a style name to the instruction given to the model.
var ExplanationStyles = map[string]string{
	"simple":       "Explain in very simple words: short sentences, no jargon, and define any term you must use.",
	"detailed":     "Give a thorough, detailed explanation covering nuances and connections between ideas.",
	"step_by_step": "Explain step by step as a numbered sequence, one idea per step.",
	"beginner":     "Assume no prior knowledge. Build up from the basics and check each idea is clear before the next.",
	"technical":    "Use precise technical language and formal definitions, as in a textbook.",
	"analogy":      "Explain mainly through a real-world analogy, then map the analogy back to the concept.",
	"example":      "Explain mainly through concrete worked examples.",
	"exam":         "Focus on what matters for an exam: key definitions, must-know points, common mistakes and likely questions.",
}

// GroundingRules are shared by every engine that answers from document excerpts.
const GroundingRules = `STRICT GROUNDING RULES:
1. Ground your answers in the document excerpts you were given.
2. Every claim supported by the excerpts MUST cite its page in square brackets, exactly like [Page X].
3. Never invent page numbers or references; cite only pages that appear in the excerpts.
4. If the excerpts do not answer the question, say clearly that the document does not contain this information. You may then add general knowledge, clearly labeled as not from the document.
5. Use an encouraging, clear teaching tone, Markdown headings and bullet points, and KaTeX ($...$ or $$...$$) for math.

When explaining a concept, use this structure where it helps: a simple definition, the main idea, a step-by-step explanation, a real-world analogy, a practical example, key points to remember, and the page references.`

// StyleRule returns the prompt line for a requested explanation style, or "" for the default.
func StyleRule(style string) string {
	if instruction, ok := ExplanationStyles[style]; ok {
		return "\nREQUESTED EXPLANATION STYLE: " + instruction + "\n"
	}
	return ""
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
func (e *Engine) Query(ctx context.Context, p AskParams) (*RAGResult, error) {
	return e.answer(ctx, p, nil)
}

// StreamQuery is Query with token-by-token streaming
func (e *Engine) StreamQuery(ctx context.Context, p AskParams, onChunk func(token string) error) (*RAGResult, error) {
	return e.answer(ctx, p, onChunk)
}

func (e *Engine) answer(ctx context.Context, p AskParams, onChunk func(token string) error) (*RAGResult, error) {
	search := p.SearchQuery
	if strings.TrimSpace(search) == "" {
		search = p.Question
	}
	results, err := e.Retrieve(ctx, search, p.UserID, p.DocumentID, 5)
	if err != nil {
		return nil, err
	}
	messages, candidates := e.BuildMessages(p.Question, p.Style, results, p.History)

	var answer string
	if onChunk != nil {
		answer, err = e.llmClient.Stream(ctx, messages, onChunk)
	} else {
		answer, err = e.llmClient.Generate(ctx, messages)
	}
	if err != nil {
		return nil, fmt.Errorf("LLM generation failed: %w", err)
	}

	// Streamed tokens went out raw; the saved answer is normalized, and the UI accepts both forms.
	answer = NormalizeCitations(answer)
	return &RAGResult{Answer: answer, Citations: FilterCitations(answer, candidates)}, nil
}

// BuildMessages creates the grounding prompt, injected context chunks, and conversation history
func (e *Engine) BuildMessages(question, style string, chunks []vectorstore.SearchResult, history []ChatMessage) ([]ChatMessage, []models.Citation) {
	var excerpts strings.Builder
	if len(chunks) == 0 {
		excerpts.WriteString("(No specific excerpts found for this query in the document.)\n")
	}
	for i, c := range chunks {
		fmt.Fprintf(&excerpts, "--- EXCERPT %d (Page %d, Document: %s) ---\n%s\n\n", i+1, c.PageNumber, c.DocumentName, c.ChunkText)
	}

	systemPrompt := fmt.Sprintf(`You are PagePilot, an AI study companion and tutor.
Help the student understand their uploaded PDF, grounded in the document.

%s
%s
DOCUMENT EXCERPTS:
%s`, GroundingRules, StyleRule(style), excerpts.String())

	messages := []ChatMessage{{Role: "system", Content: systemPrompt}}
	// Up to 6 recent turns keep follow-ups in context without crowding out the excerpts.
	start := max(0, len(history)-6)
	messages = append(messages, history[start:]...)
	messages = append(messages, ChatMessage{Role: "user", Content: question})

	return messages, CitationsFor(chunks)
}

// CitationsFor turns retrieved chunks into one candidate citation per page.
func CitationsFor(chunks []vectorstore.SearchResult) []models.Citation {
	var citations []models.Citation
	seen := make(map[int]bool)
	for _, c := range chunks {
		if seen[c.PageNumber] {
			continue
		}
		seen[c.PageNumber] = true
		citations = append(citations, models.Citation{
			DocumentID:   c.DocumentID,
			DocumentName: c.DocumentName,
			PageNumber:   c.PageNumber,
			Snippet:      Snippet(c.ChunkText, 160),
		})
	}
	return citations
}

// Snippet shortens text to n characters without splitting a multi-byte character.
func Snippet(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

// Some models (e.g. gpt-oss) write 【Page 2】 instead of [Page 2].
var bracketNormalizer = strings.NewReplacer("【", "[", "】", "]", "［", "[", "］", "]")

func NormalizeCitations(answer string) string {
	return bracketNormalizer.Replace(answer)
}

// Matches [Page 3], [page 3], [Pages 3, 5] and [Pages 3 and 5]. Spaces may be Unicode ones like
// U+202F, which gpt-oss emits and Go's \s does not cover.
var citationRe = regexp.MustCompile(`(?i)\[[\s\p{Zs}]*pages?[\s\p{Zs}]*(\d+(?:[\s\p{Zs}]*(?:,|and|&)[\s\p{Zs}]*\d+)*)[\s\p{Zs}]*\]`)
var digitsRe = regexp.MustCompile(`\d+`)

// FilterCitations keeps the candidates whose pages the answer actually cites.
func FilterCitations(answer string, candidates []models.Citation) []models.Citation {
	if len(candidates) == 0 {
		return candidates
	}

	matches := citationRe.FindAllStringSubmatch(NormalizeCitations(answer), -1)
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
