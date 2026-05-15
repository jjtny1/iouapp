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

	"github.com/jjtny1/splitit/internal/money"
)

const (
	anthropicURL     = "https://api.anthropic.com/v1/messages"
	anthropicVersion = "2023-06-01"
	anthropicModel   = "claude-sonnet-4-6"
)

const parsePrompt = `Extract the restaurant bill from this receipt image. ` +
	`Identify the restaurant name, every line item with its name, per-unit price, and quantity, ` +
	`and the tip. The price is the price of a single unit, not the line total. ` +
	`Use the prices printed on the receipt (which already include any tax baked into them). ` +
	`Also identify the currency as a 3-letter ISO 4217 code ` +
	`(e.g. USD, EUR, PLN, JPY) inferred from currency symbols or text on the receipt; ` +
	`if you cannot tell, use "USD". ` +
	`For tax_cents, report ONLY tax that is ADDED ON TOP of the listed item prices, ` +
	`such as US-style sales tax. Many receipts — especially European VAT receipts — ` +
	`instead print tax that is ALREADY INCLUDED in the item prices (often broken out ` +
	`into several rate lines such as "VAT 23%" and "VAT 8%" for information only); ` +
	`there the item prices, tip and service charge by themselves already sum to the ` +
	`grand total, so set tax_cents to 0 — do NOT add included VAT again. ` +
	`When tax is genuinely added on top, sum every added tax line into tax_cents. ` +
	`Identify any mandatory service charge (a "service charge", "service", "servis", ` +
	`"gratuity", or "coperto" line) — this is separate from a voluntary tip. ` +
	`If the receipt shows it as a percentage, set service_charge.kind to "percent" and ` +
	`service_charge.percent to that rate as a number (e.g. 12.5 for "12.5%"). ` +
	`If it is a flat amount, set service_charge.kind to "fixed" and ` +
	`service_charge.amount_cents to that amount. ` +
	`If there is no service charge, set service_charge.kind to "none". ` +
	`Read grand_total_cents: the final total, amount due, balance, or payment ` +
	`line printed on the receipt — the exact amount the customer paid. ` +
	`Express all monetary amounts as integer cents — hundredths of the currency's ` +
	`major unit, regardless of currency (e.g. $13.95 -> 1395, ¥4100 -> 410000). ` +
	`If a value is missing use 0 or an empty string. ` +
	`*** CRITICAL — THIS RECONCILIATION STEP IS MANDATORY AND OVERRIDES EVERY ` +
	`OTHER INSTRUCTION. *** Before you respond you MUST verify that the sum of ` +
	`every item line total (per-unit price times quantity) plus tax_cents plus ` +
	`tip_cents plus the service charge amount EQUALS grand_total_cents EXACTLY. ` +
	`If they are not equal your answer is WRONG — you must fix it before ` +
	`responding. The usual cause is tax: VAT printed on a receipt is almost ` +
	`always already included in the item prices, so set tax_cents to 0 and the ` +
	`bill reconciles; also recheck the item prices, quantities, tip and service ` +
	`charge. Repeat the check until the parts add up to grand_total_cents to the ` +
	`exact cent. NEVER return a result whose parts do not sum exactly to ` +
	`grand_total_cents — the amounts must match. ` +
	`Respond with ONLY a single JSON object and no prose, code fences, or explanation, ` +
	`matching exactly this shape: ` +
	`{"restaurant":string,"currency":string,` +
	`"items":[{"name":string,"price_cents":integer,"qty":integer}],` +
	`"tax_cents":integer,"tip_cents":integer,` +
	`"service_charge":{"kind":"none"|"percent"|"fixed","percent":number,"amount_cents":integer},` +
	`"grand_total_cents":integer}`

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
	r.Currency = money.CurrencyOrDefault(r.Currency)
	r.ServiceCharge = normalizeServiceCharge(r.ServiceCharge)
	r = reconcile(r)
	return r, nil
}

// normalizeServiceCharge clamps a parsed service charge to a known kind with
// non-negative amounts; anything unrecognized degrades to "none".
func normalizeServiceCharge(sc ParsedServiceCharge) ParsedServiceCharge {
	switch sc.Kind {
	case "percent":
		if sc.Percent <= 0 {
			return ParsedServiceCharge{Kind: "none"}
		}
		return ParsedServiceCharge{Kind: "percent", Percent: sc.Percent}
	case "fixed":
		if sc.AmountCents <= 0 {
			return ParsedServiceCharge{Kind: "none"}
		}
		return ParsedServiceCharge{Kind: "fixed", AmountCents: sc.AmountCents}
	default:
		return ParsedServiceCharge{Kind: "none"}
	}
}
