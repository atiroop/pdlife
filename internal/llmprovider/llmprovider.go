// Package llmprovider resolves which chat-completion provider(s) a
// command should use (openai and/or groq, selected via the PROVIDER /
// FALLBACK_PROVIDER env vars) and makes the actual HTTP call, trying
// providers in order until one succeeds. Shared by cmd/news_ingest_pubmed,
// cmd/news_ingest_nephrothai, and internal/newsimage's topic
// summarization step, so provider-selection/fallback behavior can't drift
// between call sites.
package llmprovider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type Provider struct {
	Name     string
	Endpoint string
	APIKey   string
	Model    string
}

var httpClient = &http.Client{Timeout: 30 * time.Second}

// Resolve returns the config for a named provider ("openai" or "groq"),
// or an error naming exactly which env var is missing. Never fabricates a
// default API key.
func Resolve(name string) (*Provider, error) {
	switch name {
	case "openai":
		apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
		model := strings.TrimSpace(os.Getenv("OPENAI_MODEL"))
		var missing []string
		if apiKey == "" {
			missing = append(missing, "OPENAI_API_KEY")
		}
		if model == "" {
			missing = append(missing, "OPENAI_MODEL")
		}
		if len(missing) > 0 {
			return nil, fmt.Errorf("provider %q missing required env var(s): %s", name, strings.Join(missing, ", "))
		}
		return &Provider{Name: name, Endpoint: "https://api.openai.com/v1/chat/completions", APIKey: apiKey, Model: model}, nil

	case "groq":
		apiKey := strings.TrimSpace(os.Getenv("GROQ_API_KEY"))
		if apiKey == "" {
			return nil, fmt.Errorf("provider %q missing required env var: GROQ_API_KEY", name)
		}
		model := strings.TrimSpace(os.Getenv("GROQ_MODEL"))
		if model == "" {
			model = "qwen/qwen3-32b"
		}
		return &Provider{Name: name, Endpoint: "https://api.groq.com/openai/v1/chat/completions", APIKey: apiKey, Model: model}, nil

	default:
		return nil, fmt.Errorf("unsupported provider %q (supported: openai, groq)", name)
	}
}

// Require reads PROVIDER + FALLBACK_PROVIDER and resolves both up front.
// cmdName prefixes the fatal log line if config is incomplete — this
// exits the process (per project convention: never guess or proceed
// partially configured).
func Require(cmdName string) (primary, fallback *Provider) {
	providerName := strings.TrimSpace(os.Getenv("PROVIDER"))
	fallbackName := strings.TrimSpace(os.Getenv("FALLBACK_PROVIDER"))

	var errs []string
	if providerName == "" {
		errs = append(errs, `PROVIDER is not set (expected "openai" or "groq")`)
	}
	if fallbackName == "" {
		errs = append(errs, `FALLBACK_PROVIDER is not set (expected "openai" or "groq")`)
	}
	if len(errs) > 0 {
		fatalMissingConfig(cmdName, errs)
	}

	var err error
	primary, err = Resolve(providerName)
	if err != nil {
		errs = append(errs, err.Error())
	}
	fallback, err = Resolve(fallbackName)
	if err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		fatalMissingConfig(cmdName, errs)
	}
	return primary, fallback
}

func fatalMissingConfig(cmdName string, errs []string) {
	log.Fatalf("%s: missing configuration, aborting (no guessing):\n  - %s", cmdName, strings.Join(errs, "\n  - "))
}

// List returns [primary] or [primary, fallback] (deduped by name) for use
// with CallChatJSON's try-in-order loop.
func List(primary, fallback *Provider) []*Provider {
	if fallback.Name == primary.Name {
		return []*Provider{primary}
	}
	return []*Provider{primary, fallback}
}

// CallChatJSON sends systemPrompt+userContent to each provider in order
// until one responds successfully. Returns the message content
// (extracted from any markdown fencing) for the caller to json.Unmarshal
// into whatever shape it expects, plus which provider succeeded.
func CallChatJSON(providers []*Provider, systemPrompt, userContent string, temperature float64) (content string, used *Provider, err error) {
	var lastErr error
	for _, p := range providers {
		c, cerr := callChatCompletion(p, systemPrompt, userContent, temperature)
		if cerr == nil {
			return c, p, nil
		}
		lastErr = cerr
	}
	return "", nil, fmt.Errorf("all providers failed, last error: %w", lastErr)
}

func callChatCompletion(p *Provider, systemMsg, userMsg string, temperature float64) (string, error) {
	payload, _ := json.Marshal(map[string]interface{}{
		"model": p.Model,
		"messages": []map[string]string{
			{"role": "system", "content": systemMsg},
			{"role": "user", "content": userMsg},
		},
		"temperature": temperature,
	})

	req, err := http.NewRequest(http.MethodPost, p.Endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "pdlife.app-news-ingest/1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%s returned HTTP %d: %s", p.Name, resp.StatusCode, string(body))
	}

	var raw struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &raw); err != nil || len(raw.Choices) == 0 {
		return "", fmt.Errorf("%s: invalid response shape", p.Name)
	}
	return ExtractJSONObject(raw.Choices[0].Message.Content), nil
}

// ExtractJSONObject strips markdown code fences and surrounding prose,
// returning just the {...} substring — models sometimes wrap JSON in
// ```json fences or add commentary despite instructions not to.
func ExtractJSONObject(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		return raw[start : end+1]
	}
	return raw
}
