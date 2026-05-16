package autosplit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	anthropicURL     = "https://api.anthropic.com/v1/messages"
	anthropicVersion = "2023-06-01"
	anthropicModel   = "claude-sonnet-4-6"
)

// ClaudeAssigner maps a transcript onto bill items using the Anthropic
// Messages API.
type ClaudeAssigner struct {
	APIKey string
	Client *http.Client
}

// NewClaudeAssigner builds a ClaudeAssigner with a 60s HTTP timeout.
func NewClaudeAssigner(apiKey string) ClaudeAssigner {
	return ClaudeAssigner{
		APIKey: apiKey,
		Client: &http.Client{Timeout: 60 * time.Second},
	}
}

// buildPrompt renders the numbered item list and transcript into the
// instruction text sent to the model.
func buildPrompt(items []Item, transcript, hostName string) string {
	var b strings.Builder
	b.WriteString("You are splitting a restaurant bill from a host's spoken description.\n\n")
	b.WriteString("The host (the speaker) is named ")
	b.WriteString(hostName)
	b.WriteString(". When the transcript says \"I\", \"me\", \"my\", or \"mine\", that refers to ")
	b.WriteString(hostName)
	b.WriteString(".\n\nThe bill has these line items (one claimable unit each):\n")
	for _, it := range items {
		b.WriteString(strconv.Itoa(it.Index))
		b.WriteString(". ")
		b.WriteString(it.Name)
		b.WriteString(" (")
		b.WriteString(strconv.Itoa(it.PriceCents))
		b.WriteString(" cents)\n")
	}
	b.WriteString("\nTranscript of the host describing the split:\n")
	b.WriteString(transcript)
	b.WriteString("\n\nDecide who claimed each item. An item shared by several people lists ")
	b.WriteString("all of them. Use the item's \"index\" number. Put every person you name ")
	b.WriteString("into the top-level \"people\" array. Use \"notes\" for anything ambiguous ")
	b.WriteString("or items you could not assign.\n")
	b.WriteString("Respond with ONLY a single JSON object and no prose, code fences, or ")
	b.WriteString("explanation, matching exactly this shape: ")
	b.WriteString(`{"people":[string],"items":[{"index":integer,"people":[string]}],"notes":string}`)
	return b.String()
}

func (a ClaudeAssigner) Assign(ctx context.Context, items []Item, transcript, hostName string) (Assignment, error) {
	reqBody := map[string]any{
		"model":      anthropicModel,
		"max_tokens": 2048,
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": buildPrompt(items, transcript, hostName)},
				},
			},
		},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return Assignment{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicURL, bytes.NewReader(payload))
	if err != nil {
		return Assignment{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("x-api-key", a.APIKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("content-type", "application/json")

	resp, err := a.Client.Do(req)
	if err != nil {
		return Assignment{}, fmt.Errorf("call anthropic: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Assignment{}, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return Assignment{}, fmt.Errorf("anthropic status %d: %s", resp.StatusCode, string(body))
	}

	var apiResp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return Assignment{}, fmt.Errorf("decode response: %w", err)
	}

	var text string
	for _, c := range apiResp.Content {
		if c.Type == "text" {
			text += c.Text
		}
	}
	if strings.TrimSpace(text) == "" {
		return Assignment{}, fmt.Errorf("empty model response")
	}

	assignment, err := parseAssignmentJSON(text)
	if err != nil {
		return Assignment{}, fmt.Errorf("parse model output: %w", err)
	}
	return assignment, nil
}

// parseAssignmentJSON extracts an Assignment from model text, tolerating code
// fences and surrounding prose.
func parseAssignmentJSON(text string) (Assignment, error) {
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

	var a Assignment
	if err := json.Unmarshal([]byte(s), &a); err != nil {
		return Assignment{}, err
	}
	return a, nil
}
