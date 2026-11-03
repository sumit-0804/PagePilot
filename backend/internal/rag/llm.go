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

type ChatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature float32       `json:"temperature"`
	Stream      bool          `json:"stream"`
}

type ChatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		Delta struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type LLMClient interface {
	Generate(ctx context.Context, messages []ChatMessage) (string, error)
	Stream(ctx context.Context, messages []ChatMessage, onChunk func(token string) error) (string, error)
}

type GroqClient struct {
	apiKey     string
	model      string
	baseURL    string
	httpClient *http.Client
}

func NewGroqClient(cfg *config.Config) *GroqClient {
	return &GroqClient{
		apiKey:     cfg.GroqAPIKey,
		model:      cfg.GroqModel,
		baseURL:    "https://api.groq.com/openai/v1/chat/completions",
		httpClient: &http.Client{Timeout: 90 * time.Second},
	}
}

func (g *GroqClient) Generate(ctx context.Context, messages []ChatMessage) (string, error) {
	if g.apiKey == "" {
		return g.mockFallback(messages), nil
	}

	reqBody, err := json.Marshal(ChatCompletionRequest{
		Model:       g.model,
		Messages:    messages,
		Temperature: 0.2,
		Stream:      false,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", g.baseURL, bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.apiKey)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("groq API request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("groq API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var parsed ChatCompletionResponse
	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		return "", fmt.Errorf("failed to parse groq response: %w", err)
	}

	if parsed.Error != nil && parsed.Error.Message != "" {
		return "", fmt.Errorf("groq API error: %s", parsed.Error.Message)
	}

	if len(parsed.Choices) == 0 {
		return "", errors.New("groq API returned no choices")
	}

	return parsed.Choices[0].Message.Content, nil
}

func (g *GroqClient) Stream(ctx context.Context, messages []ChatMessage, onChunk func(token string) error) (string, error) {
	if g.apiKey == "" {
		// Simulate streaming with mock
		mockText := g.mockFallback(messages)
		words := strings.Fields(mockText)
		var full strings.Builder
		for i, w := range words {
			token := w
			if i > 0 {
				token = " " + w
			}
			full.WriteString(token)
			if err := onChunk(token); err != nil {
				return full.String(), err
			}
			time.Sleep(25 * time.Millisecond)
		}
		return full.String(), nil
	}

	reqBody, err := json.Marshal(ChatCompletionRequest{
		Model:       g.model,
		Messages:    messages,
		Temperature: 0.2,
		Stream:      true,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", g.baseURL, bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.apiKey)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("groq streaming request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("groq API error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	reader := bufio.NewReader(resp.Body)
	var fullContent strings.Builder

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fullContent.String(), err
		}

		lineStr := strings.TrimSpace(string(line))
		if lineStr == "" || !strings.HasPrefix(lineStr, "data:") {
			continue
		}

		data := strings.TrimSpace(strings.TrimPrefix(lineStr, "data:"))
		if data == "[DONE]" {
			break
		}

		var chunk ChatCompletionResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if len(chunk.Choices) > 0 {
			token := chunk.Choices[0].Delta.Content
			if token != "" {
				fullContent.WriteString(token)
				if err := onChunk(token); err != nil {
					return fullContent.String(), err
				}
			}
		}
	}

	return fullContent.String(), nil
}

func (g *GroqClient) mockFallback(messages []ChatMessage) string {
	lastUserMsg := ""
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			lastUserMsg = messages[i].Content
			break
		}
	}
	return fmt.Sprintf("Grounded Response: Based on the provided document excerpts, here is the answer regarding '%s'. (Note: Please set GROQ_API_KEY in .env for full Groq LLM inference).", lastUserMsg)
}
