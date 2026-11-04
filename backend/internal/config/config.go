package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	APIPort         string
	FrontendOrigins []string

	GroqAPIKey string
	GroqModel  string

	DatabaseURL string

	SupabaseURL       string
	SupabaseSecretKey string
	SupabaseJWTSecret string // only for projects still signing tokens with HS256
	StorageBucket     string

	EmbeddingProvider  string
	EmbeddingModel     string
	EmbeddingDim       int
	EmbeddingBatchSize int
	EmbeddingRPM       int
	EmbeddingTPM       int
	EmbeddingRPD       int
	GeminiAPIKey       string
	EmbeddingAPIKey    string // openai-compatible provider only
	EmbeddingBaseURL   string // openai-compatible provider only

	// OCRProvider is "gemini" (read scanned pages with a Gemini model) or "none".
	OCRProvider     string
	OCRModel        string
	OCRRPM          int
	OCRTPM          int
	OCRRPD          int
	OCRPagesPerCall int
	OCRMaxPages     int // scanned pages read per document, to protect the daily quota

	// WebSearchProvider is "gemini" (Gemini with Google Search grounding) or "none".
	WebSearchProvider string
	WebSearchModel    string
	WebSearchRPM      int
	WebSearchRPD      int
}

// Load reads .env (cwd or repo root) and validates required settings.
func Load() (*Config, error) {
	for _, f := range []string{".env", "../.env"} {
		if _, err := os.Stat(f); err == nil {
			if err := godotenv.Load(f); err != nil {
				return nil, fmt.Errorf("loading %s: %w", f, err)
			}
			break
		}
	}

	var errs []error
	intVar := func(key string, def int) int {
		raw := getEnv(key, "")
		if raw == "" {
			return def
		}
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			errs = append(errs, fmt.Errorf("%s must be a positive integer, got %q", key, raw))
			return def
		}
		return n
	}

	cfg := &Config{
		APIPort:            getEnv("PORT", getEnv("API_PORT", "8080")), // hosts like Render set PORT
		FrontendOrigins:    splitList(getEnv("FRONTEND_URL", "http://localhost:5173")),
		GroqAPIKey:         getEnv("GROQ_API_KEY", ""),
		GroqModel:          getEnv("GROQ_MODEL", "openai/gpt-oss-120b"),
		DatabaseURL:        getEnv("DATABASE_URL", ""),
		SupabaseURL:        strings.TrimRight(getEnv("SUPABASE_URL", ""), "/"),
		SupabaseSecretKey:  getEnv("SUPABASE_SECRET_KEY", ""),
		SupabaseJWTSecret:  getEnv("SUPABASE_JWT_SECRET", ""),
		StorageBucket:      getEnv("STORAGE_BUCKET", "pdfs"),
		EmbeddingProvider:  strings.ToLower(getEnv("EMBEDDING_PROVIDER", "gemini")),
		EmbeddingModel:     getEnv("EMBEDDING_MODEL", "gemini-embedding-2"),
		GeminiAPIKey:       getEnv("GEMINI_API_KEY", ""),
		EmbeddingAPIKey:    getEnv("EMBEDDING_API_KEY", ""),
		EmbeddingBaseURL:   getEnv("EMBEDDING_BASE_URL", ""),
		EmbeddingDim:       intVar("EMBEDDING_DIM", 1536),
		EmbeddingBatchSize: intVar("EMBEDDING_BATCH_SIZE", 100),
		EmbeddingRPM:       intVar("EMBEDDING_RPM", 0),
		EmbeddingTPM:       intVar("EMBEDDING_TPM", 0),
		EmbeddingRPD:       intVar("EMBEDDING_RPD", 0),
		OCRProvider:        strings.ToLower(getEnv("OCR_PROVIDER", "none")),
		OCRModel:           getEnv("OCR_MODEL", ""),
		OCRRPM:             intVar("OCR_RPM", 0),
		OCRTPM:             intVar("OCR_TPM", 0),
		OCRRPD:             intVar("OCR_RPD", 0),
		OCRPagesPerCall:    intVar("OCR_PAGES_PER_CALL", 10),
		OCRMaxPages:        intVar("OCR_MAX_PAGES", 100),
		WebSearchProvider:  strings.ToLower(getEnv("WEB_SEARCH_PROVIDER", "none")),
		WebSearchModel:     getEnv("WEB_SEARCH_MODEL", ""),
		WebSearchRPM:       intVar("WEB_SEARCH_RPM", 0),
		WebSearchRPD:       intVar("WEB_SEARCH_RPD", 0),
	}

	required := map[string]string{
		"GROQ_API_KEY":        cfg.GroqAPIKey,
		"DATABASE_URL":        cfg.DatabaseURL,
		"SUPABASE_URL":        cfg.SupabaseURL,
		"SUPABASE_SECRET_KEY": cfg.SupabaseSecretKey,
	}
	switch cfg.EmbeddingProvider {
	case "gemini":
		required["GEMINI_API_KEY"] = cfg.GeminiAPIKey
		required["EMBEDDING_RPM"] = positive(cfg.EmbeddingRPM)
		required["EMBEDDING_TPM"] = positive(cfg.EmbeddingTPM)
		required["EMBEDDING_RPD"] = positive(cfg.EmbeddingRPD)
	case "openai":
		required["EMBEDDING_BASE_URL"] = cfg.EmbeddingBaseURL
	default:
		errs = append(errs, fmt.Errorf("EMBEDDING_PROVIDER must be gemini or openai, got %q", cfg.EmbeddingProvider))
	}
	switch cfg.OCRProvider {
	case "gemini":
		required["GEMINI_API_KEY"] = cfg.GeminiAPIKey
		required["OCR_MODEL"] = cfg.OCRModel
		required["OCR_RPM"] = positive(cfg.OCRRPM)
		required["OCR_TPM"] = positive(cfg.OCRTPM)
		required["OCR_RPD"] = positive(cfg.OCRRPD)
	case "none":
	default:
		errs = append(errs, fmt.Errorf("OCR_PROVIDER must be gemini or none, got %q", cfg.OCRProvider))
	}
	switch cfg.WebSearchProvider {
	case "gemini":
		required["GEMINI_API_KEY"] = cfg.GeminiAPIKey
		required["WEB_SEARCH_MODEL"] = cfg.WebSearchModel
		required["WEB_SEARCH_RPM"] = positive(cfg.WebSearchRPM)
		required["WEB_SEARCH_RPD"] = positive(cfg.WebSearchRPD)
	case "none":
	default:
		errs = append(errs, fmt.Errorf("WEB_SEARCH_PROVIDER must be gemini or none, got %q", cfg.WebSearchProvider))
	}
	for key, val := range required {
		if val == "" {
			errs = append(errs, fmt.Errorf("%s is required", key))
		}
	}

	if err := errors.Join(errs...); err != nil {
		return nil, fmt.Errorf("invalid configuration:\n%w", err)
	}
	return cfg, nil
}

func getEnv(key, def string) string {
	if val, ok := os.LookupEnv(key); ok && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	return def
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, strings.TrimRight(p, "/"))
		}
	}
	return out
}

func positive(n int) string {
	if n > 0 {
		return strconv.Itoa(n)
	}
	return ""
}
