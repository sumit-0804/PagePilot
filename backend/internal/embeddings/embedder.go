package embeddings

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/pagepilot/backend/internal/config"
)

// Embedder turns text into vectors. Documents and queries are embedded separately because
// retrieval models tune each side differently.
type Embedder interface {
	EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error)
	EmbedQuery(ctx context.Context, text string) ([]float32, error)
	Dimension() int
	Model() string
}

// New builds the configured embedder and probes it once so a bad key or wrong dimension fails at startup.
func New(ctx context.Context, cfg *config.Config) (Embedder, error) {
	var emb Embedder
	switch cfg.EmbeddingProvider {
	case "gemini":
		g, err := NewGeminiEmbedder(ctx, GeminiConfig{
			APIKey:    cfg.GeminiAPIKey,
			Model:     cfg.EmbeddingModel,
			Dimension: cfg.EmbeddingDim,
			RPM:       cfg.EmbeddingRPM,
			TPM:       cfg.EmbeddingTPM,
			RPD:       cfg.EmbeddingRPD,
		})
		if err != nil {
			return nil, err
		}
		emb = g
	case "openai":
		emb = NewOpenAIEmbedder(cfg.EmbeddingBaseURL, cfg.EmbeddingAPIKey, cfg.EmbeddingModel, cfg.EmbeddingDim)
	default:
		return nil, fmt.Errorf("unknown embedding provider %q", cfg.EmbeddingProvider)
	}

	vec, err := emb.EmbedQuery(ctx, "startup check")
	if err != nil {
		return nil, fmt.Errorf("embedding provider check failed: %w", err)
	}
	if len(vec) != cfg.EmbeddingDim {
		return nil, fmt.Errorf("embedding model %s returned %d dimensions, EMBEDDING_DIM is %d", emb.Model(), len(vec), cfg.EmbeddingDim)
	}
	return emb, nil
}

// BuiltinEmbedder makes deterministic hash-based vectors. Not semantic: tests only.
type BuiltinEmbedder struct {
	dim int
}

func NewBuiltinEmbedder(dim int) *BuiltinEmbedder {
	if dim <= 0 {
		dim = 384
	}
	return &BuiltinEmbedder{dim: dim}
}

func (b *BuiltinEmbedder) Dimension() int { return b.dim }
func (b *BuiltinEmbedder) Model() string  { return "builtin-hash" }

func (b *BuiltinEmbedder) EmbedDocuments(_ context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, text := range texts {
		results[i] = b.generateVector(text)
	}
	return results, nil
}

func (b *BuiltinEmbedder) EmbedQuery(_ context.Context, text string) ([]float32, error) {
	return b.generateVector(text), nil
}

// generateVector maps words to a normalized vector using feature hashing
func (b *BuiltinEmbedder) generateVector(text string) []float32 {
	vec := make([]float32, b.dim)
	for _, word := range strings.Fields(strings.ToLower(text)) {
		word = strings.Trim(word, ".,!?:;\"'()[]{}")
		if word == "" {
			continue
		}
		h := sha256.Sum256([]byte(word))
		for j := 0; j < 4; j++ {
			val := binary.BigEndian.Uint32(h[j*4 : (j+1)*4])
			sign := float32(1.0)
			if val>>31 == 1 {
				sign = -1.0
			}
			vec[int(val%uint32(b.dim))] += sign * float32(math.Log(float64(len(word))+1.0))
		}
	}

	var norm float64
	for _, val := range vec {
		norm += float64(val * val)
	}
	if norm > 0 {
		sqrtNorm := float32(math.Sqrt(norm))
		for k := range vec {
			vec[k] /= sqrtNorm
		}
	}
	return vec
}

// OpenAIEmbedder calls an OpenAI-compatible /embeddings endpoint (e.g. Ollama for offline work).
type OpenAIEmbedder struct {
	url        string
	apiKey     string
	model      string
	dimension  int
	httpClient *http.Client
}

func NewOpenAIEmbedder(baseURL, apiKey, model string, dimension int) *OpenAIEmbedder {
	url := strings.TrimRight(baseURL, "/")
	if !strings.HasSuffix(url, "/embeddings") {
		url += "/embeddings"
	}
	return &OpenAIEmbedder{
		url:        url,
		apiKey:     apiKey,
		model:      model,
		dimension:  dimension,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

func (o *OpenAIEmbedder) Dimension() int { return o.dimension }
func (o *OpenAIEmbedder) Model() string  { return o.model }

func (o *OpenAIEmbedder) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	vecs, err := o.EmbedDocuments(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return vecs[0], nil
}

func (o *OpenAIEmbedder) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	reqBody, err := json.Marshal(map[string]any{"model": o.model, "input": texts})
	if err != nil {
		return nil, fmt.Errorf("serializing embedding request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.apiKey)
	}

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading embedding response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding provider returned status %d: %s", resp.StatusCode, truncate(string(body), 300))
	}

	var parsed struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parsing embedding response: %w", err)
	}

	vectors := make([][]float32, len(texts))
	for _, item := range parsed.Data {
		if item.Index >= 0 && item.Index < len(vectors) {
			vectors[item.Index] = item.Embedding
		}
	}
	for i, v := range vectors {
		if len(v) == 0 {
			return nil, fmt.Errorf("embedding provider returned no vector for input %d", i)
		}
	}
	return vectors, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
