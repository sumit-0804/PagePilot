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
	"strings"
	"time"

	"github.com/pagepilot/backend/internal/config"
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
}

type responseFormat struct {
	Type string `json:"type"`
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
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
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

func (g *GroqClient) Generate(ctx context.Context, messages []ChatMessage) (string, error) {
	return g.complete(ctx, messages, nil)
}

func (g *GroqClient) GenerateJSON(ctx context.Context, messages []ChatMessage) (string, error) {
	return g.complete(ctx, messages, &responseFormat{Type: "json_object"})
}

func (g *GroqClient) complete(ctx context.Context, messages []ChatMessage, format *responseFormat) (string, error) {
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
	if len(parsed.Choices) == 0 {
		return "", errors.New("groq returned no choices")
	}
	return parsed.Choices[0].Message.Content, nil
}

func (g *GroqClient) Stream(ctx context.Context, messages []ChatMessage, onChunk func(token string) error) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, streamTimeout)
	defer cancel()

	resp, err := g.send(ctx, chatCompletionRequest{Model: g.model, Messages: messages, Temperature: 0.2, Stream: true})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	var full strings.Builder
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
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return nil, fmt.Errorf("groq returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return resp, nil
}
