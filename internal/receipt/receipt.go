// Package receipt extracts structured bill data from receipt photos.
package receipt

import "context"

// ParsedItem is a single line item read from a receipt.
type ParsedItem struct {
	Name       string `json:"name"`
	PriceCents int    `json:"price_cents"`
	Qty        int    `json:"qty"`
}

// ParsedReceipt is the structured result of parsing a receipt image.
type ParsedReceipt struct {
	Restaurant string       `json:"restaurant"`
	Items      []ParsedItem `json:"items"`
	TaxCents   int          `json:"tax_cents"`
	TipCents   int          `json:"tip_cents"`
}

// Parser turns a receipt image into structured bill data.
type Parser interface {
	Parse(ctx context.Context, image []byte, mediaType string) (ParsedReceipt, error)
}
