package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// LangfuseClient calls the Langfuse public API. Today it only manages user-feedback scores.
type LangfuseClient struct {
	baseURL, publicKey, secretKey, environment string
	http                                       *http.Client
}

// NewLangfuseClient returns nil when the keys are missing, so callers can skip Langfuse entirely.
func NewLangfuseClient(cfg Config) *LangfuseClient {
	if cfg.LangfusePublicKey == "" || cfg.LangfuseSecretKey == "" {
		return nil
	}
	base := strings.TrimRight(cfg.LangfuseBaseURL, "/")
	if base == "" {
		base = "https://cloud.langfuse.com"
	}
	return &LangfuseClient{
		baseURL: base, publicKey: cfg.LangfusePublicKey, secretKey: cfg.LangfuseSecretKey,
		environment: cfg.Environment,
		http:        &http.Client{Timeout: 10 * time.Second},
	}
}

// FeedbackScoreName is the Langfuse score that holds thumbs up (1) / down (0).
const FeedbackScoreName = "user_feedback"

// feedbackScoreID is fixed per message, so re-rating overwrites the score instead of adding one.
func feedbackScoreID(messageID string) string { return "feedback-" + messageID }

type scoreRequest struct {
	ID          string  `json:"id"`
	TraceID     string  `json:"traceId"`
	Name        string  `json:"name"`
	Value       float64 `json:"value"`
	DataType    string  `json:"dataType"`
	Comment     string  `json:"comment,omitempty"`
	Environment string  `json:"environment,omitempty"`
}

// Score records a thumbs up or down on the answer's trace. The reason, if any, is the comment.
func (c *LangfuseClient) Score(ctx context.Context, traceID, messageID string, rating int, reason string) error {
	value := 0.0
	if rating > 0 {
		value = 1
	}
	body, err := json.Marshal(scoreRequest{
		ID: feedbackScoreID(messageID), TraceID: traceID, Name: FeedbackScoreName,
		Value: value, DataType: "BOOLEAN", Comment: reason, Environment: c.environment,
	})
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodPost, "/api/public/scores", body)
}

// Remove deletes the feedback score of a message whose rating was cleared.
func (c *LangfuseClient) Remove(ctx context.Context, messageID string) error {
	return c.do(ctx, http.MethodDelete, "/api/public/scores/"+url.PathEscape(feedbackScoreID(messageID)), nil)
}

func (c *LangfuseClient) do(ctx context.Context, method, path string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.publicKey, c.secretKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("langfuse %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("langfuse %s %s: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}
