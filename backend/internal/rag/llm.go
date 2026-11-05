package rag

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pagepilot/backend/internal/config"
	"github.com/pagepilot/backend/internal/observability"
	"go.opentelemetry.io/otel/trace"
)

type ChatMessage struct {
	Role    string `json:"role"` // "system", "user", "assistant"
	Content string `json:"content"`
}

type chatCompletionRequest struct {
	Model            string          `json:"model"`
	Messages         []ChatMessage   `json:"messages"`
	Temperature      float32         `json:"temperature"`
	Stream           bool            `json:"stream"`
	IncludeReasoning *bool           `json:"include_reasoning,omitempty"`
	ResponseFormat   *responseFormat `json:"response_format,omitempty"`
	StreamOptions    *streamOptions  `json:"stream_options,omitempty"`
}

type responseFormat struct {
	Type string `json:"type"`
}

// streamOptions asks for token usage in the final stream chunk.
type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type tokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *tokenUsage `json:"usage,omitempty"`
	// Groq also reports streaming usage here.
	XGroq *struct {
		Usage *tokenUsage `json:"usage,omitempty"`
	} `json:"x_groq,omitempty"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (r *chatCompletionResponse) usage() *tokenUsage {
	if r.Usage != nil {
		return r.Usage
	}
	if r.XGroq != nil {
		return r.XGroq.Usage
	}
	return nil
}

type LLMClient interface {
	Generate(ctx context.Context, messages []ChatMessage) (string, error)
	// GenerateJSON asks the model for a single JSON object (JSON mode).
	GenerateJSON(ctx context.Context, messages []ChatMessage) (string, error)
	Stream(ctx context.Context, messages []ChatMessage, onChunk func(token string) error) (string, error)
}

const (
	groqURL         = "https://api.groq.com/openai/v1/chat/completions"
	generateTimeout = 90 * time.Second
	streamTimeout   = 5 * time.Minute
)

type GroqClient struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

func NewGroqClient(cfg *config.Config) *GroqClient {
	// No client-wide timeout: it would also cap how long a streamed body may take. Each call sets its own deadline.
	return &GroqClient{apiKey: cfg.GroqAPIKey, model: cfg.GroqModel, httpClient: &http.Client{}}
}

// WithTransport sends requests through t (the eval paces calls to Groq's per-minute limits this way).
func (g *GroqClient) WithTransport(t http.RoundTripper) *GroqClient {
	g.httpClient = &http.Client{Transport: t}
	return g
}

func (g *GroqClient) Generate(ctx context.Context, messages []ChatMessage) (string, error) {
	return g.complete(ctx, messages, nil)
}

func (g *GroqClient) GenerateJSON(ctx context.Context, messages []ChatMessage) (string, error) {
	return g.complete(ctx, messages, &responseFormat{Type: "json_object"})
}

func (g *GroqClient) complete(ctx context.Context, messages []ChatMessage, format *responseFormat) (answer string, err error) {
	ctx, span := g.startGeneration(ctx, messages, false, format)
	var usage *tokenUsage
	defer func() { endGeneration(span, usage, answer, err) }()

	ctx, cancel := context.WithTimeout(ctx, generateTimeout)
	defer cancel()

	resp, err := g.send(ctx, chatCompletionRequest{Model: g.model, Messages: messages, Temperature: 0.2, ResponseFormat: format})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading groq response: %w", err)
	}
	var parsed chatCompletionResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("parsing groq response: %w", err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return "", fmt.Errorf("groq error: %s", parsed.Error.Message)
	}
	usage = parsed.usage()
	if len(parsed.Choices) == 0 {
		return "", errors.New("groq returned no choices")
	}
	return parsed.Choices[0].Message.Content, nil
}

func (g *GroqClient) Stream(ctx context.Context, messages []ChatMessage, onChunk func(token string) error) (_ string, err error) {
	ctx, span := g.startGeneration(ctx, messages, true, nil)
	var full strings.Builder
	var usage *tokenUsage
	defer func() { endGeneration(span, usage, full.String(), err) }()

	ctx, cancel := context.WithTimeout(ctx, streamTimeout)
	defer cancel()

	resp, err := g.send(ctx, chatCompletionRequest{
		Model: g.model, Messages: messages, Temperature: 0.2, Stream: true,
		StreamOptions: &streamOptions{IncludeUsage: true},
	})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return full.String(), fmt.Errorf("reading groq stream: %w", err)
		}

		data, ok := strings.CutPrefix(strings.TrimSpace(string(line)), "data:")
		if !ok {
			continue
		}
		data = strings.TrimSpace(data)
		if data == "[DONE]" {
			break
		}

		var chunk chatCompletionResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return full.String(), fmt.Errorf("parsing groq stream chunk: %w", err)
		}
		if chunk.Error != nil && chunk.Error.Message != "" {
			return full.String(), fmt.Errorf("groq stream error: %s", chunk.Error.Message)
		}
		if u := chunk.usage(); u != nil {
			usage = u
		}
		if len(chunk.Choices) == 0 || chunk.Choices[0].Delta.Content == "" {
			continue
		}
		token := chunk.Choices[0].Delta.Content
		full.WriteString(token)
		if err := onChunk(token); err != nil {
			return full.String(), err
		}
	}
	return full.String(), nil
}

func (g *GroqClient) startGeneration(ctx context.Context, messages []ChatMessage, streaming bool, format *responseFormat) (context.Context, trace.Span) {
	responseFormat := "text"
	if format != nil {
		responseFormat = format.Type
	}
	ctx, span := observability.Start(ctx, "chat "+g.model, observability.TypeGeneration,
		observability.AttrGenAIOperation.String("chat"),
		observability.AttrGenAISystem.String("groq"),
		observability.AttrGenAIRequestModel.String(g.model),
		observability.AttrStreaming.Bool(streaming),
		observability.AttrResponseFormat.String(responseFormat),
	)
	if observability.CaptureContent() && len(messages) > 0 {
		observability.SetContent(span, observability.AttrInput, messages[len(messages)-1].Content)
	}
	return ctx, span
}

// endGeneration records token usage only when Groq reported it; it never estimates.
func endGeneration(span trace.Span, usage *tokenUsage, output string, err error) {
	if usage != nil {
		span.SetAttributes(
			observability.AttrGenAIInputTokens.Int(usage.PromptTokens),
			observability.AttrGenAIOutputTokens.Int(usage.CompletionTokens),
			observability.AttrUsageAvailable.Bool(true),
		)
	} else {
		span.SetAttributes(observability.AttrUsageAvailable.Bool(false))
	}
	observability.SetContent(span, observability.AttrOutput, output)
	observability.End(span, err)
}

// send posts a chat completion request and returns the response if it succeeded.
func (g *GroqClient) send(ctx context.Context, payload chatCompletionRequest) (*http.Response, error) {
	// gpt-oss reasons before answering; the reasoning text isn't shown, so don't send it back.
	noReasoning := false
	if strings.Contains(g.model, "gpt-oss") {
		payload.IncludeReasoning = &noReasoning
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, groqURL, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+g.apiKey)

		resp, err := g.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("groq request failed: %w", err)
		}
		if resp.StatusCode == http.StatusOK {
			return resp, nil
		}

		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		resp.Body.Close()
		if resp.StatusCode != http.StatusTooManyRequests || attempt >= maxRateLimitRetries {
			return nil, fmt.Errorf("groq returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
		}
		if err := sleepCtx(ctx, retryAfter(resp.Header.Get("Retry-After"), attempt)); err != nil {
			return nil, err
		}
	}
}

const maxRateLimitRetries = 4

// retryAfter uses Groq's Retry-After seconds when given, otherwise backs off exponentially.
func retryAfter(header string, attempt int) time.Duration {
	if secs, err := strconv.ParseFloat(strings.TrimSpace(header), 64); err == nil && secs > 0 {
		return time.Duration(secs * float64(time.Second))
	}
	return min(2*time.Second<<attempt, time.Minute)
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
