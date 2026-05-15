package receipt

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	anthropicURL     = "https://api.anthropic.com/v1/messages"
	anthropicVersion = "2023-06-01"
	anthropicModel   = "claude-sonnet-4-6"
)

const parsePrompt = `Extract the restaurant bill from this receipt image. ` +
	`Identify the restaurant name, every line item with its name, price, and quantity, ` +
	`the tax, and the tip. Express all monetary amounts as integer cents (e.g. $13.95 -> 1395). ` +
	`If a value is missing use 0 or an empty string. ` +
	`Respond with ONLY a single JSON object and no prose, code fences, or explanation, ` +
	`matching exactly this shape: ` +
	`{"restaurant":string,"items":[{"name":string,"price_cents":integer,"qty":integer}],` +
	`"tax_cents":integer,"tip_cents":integer}`

// ClaudeParser parses receipts using the Anthropic Messages API.
type ClaudeParser struct {
	APIKey string
	Client *http.Client
}

// NewClaudeParser builds a ClaudeParser with a 60s HTTP timeout.
func NewClaudeParser(apiKey string) ClaudeParser {
	return ClaudeParser{
		APIKey: apiKey,
		Client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (p ClaudeParser) Parse(ctx context.Context, image []byte, mediaType string) (ParsedReceipt, error) {
	reqBody := map[string]any{
		"model":      anthropicModel,
		"max_tokens": 2048,
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{
						"type": "image",
						"source": map[string]any{
							"type":       "base64",
							"media_type": mediaType,
							"data":       base64.StdEncoding.EncodeToString(image),
						},
					},
					map[string]any{"type": "text", "text": parsePrompt},
				},
			},
		},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return ParsedReceipt{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicURL, bytes.NewReader(payload))
	if err != nil {
		return ParsedReceipt{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("x-api-key", p.APIKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("content-type", "application/json")

	resp, err := p.Client.Do(req)
	if err != nil {
		return ParsedReceipt{}, fmt.Errorf("call anthropic: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return ParsedReceipt{}, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return ParsedReceipt{}, fmt.Errorf("anthropic status %d: %s", resp.StatusCode, string(body))
	}

	var apiResp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return ParsedReceipt{}, fmt.Errorf("decode response: %w", err)
	}

	var text string
	for _, c := range apiResp.Content {
		if c.Type == "text" {
			text += c.Text
		}
	}
	if strings.TrimSpace(text) == "" {
		return ParsedReceipt{}, fmt.Errorf("empty model response")
	}

	parsed, err := parseReceiptJSON(text)
	if err != nil {
		return ParsedReceipt{}, fmt.Errorf("parse model output: %w", err)
	}
	return parsed, nil
}

// parseReceiptJSON extracts a ParsedReceipt from model text, tolerating
// code fences and surrounding prose.
func parseReceiptJSON(text string) (ParsedReceipt, error) {
	s := strings.TrimSpace(text)
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			s = s[i+1:]
		}
		if i := strings.LastIndex(s, "```"); i >= 0 {
			s = s[:i]
		}
		s = strings.TrimSpace(s)
	}
	if start := strings.IndexByte(s, '{'); start >= 0 {
		if end := strings.LastIndexByte(s, '}'); end > start {
			s = s[start : end+1]
		}
	}

	var r ParsedReceipt
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		return ParsedReceipt{}, err
	}
	for i := range r.Items {
		if r.Items[i].Qty < 1 {
			r.Items[i].Qty = 1
		}
	}
	return r, nil
}
