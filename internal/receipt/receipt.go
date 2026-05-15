// Package receipt extracts structured bill data from receipt photos.
package receipt

import "context"

// ParsedItem is a single line item read from a receipt.
type ParsedItem struct {
	Name       string `json:"name"`
	PriceCents int    `json:"price_cents"`
	Qty        int    `json:"qty"`
}

// ParsedServiceCharge is a mandatory service fee read from a receipt,
// distinct from a voluntary tip. Kind is "percent", "fixed", or "none".
// For a percent charge Percent is the rate (e.g. 12.5 for 12.5%); for a
// fixed charge AmountCents is the flat amount in integer cents.
type ParsedServiceCharge struct {
	Kind        string  `json:"kind"`
	Percent     float64 `json:"percent"`
	AmountCents int     `json:"amount_cents"`
}

// ParsedReceipt is the structured result of parsing a receipt image.
//
// Currency is the ISO 4217 code of the amounts on the receipt. All *Cents
// fields are hundredths of that currency's major unit (e.g. ¥4100 -> 410000).
//
// GrandTotalCents is the final total printed on the receipt. It is used to
// reconcile the parsed parts (see reconcile) and is not stored on the bill.
type ParsedReceipt struct {
	Restaurant      string              `json:"restaurant"`
	Currency        string              `json:"currency"`
	Items           []ParsedItem        `json:"items"`
	TaxCents        int                 `json:"tax_cents"`
	TipCents        int                 `json:"tip_cents"`
	ServiceCharge   ParsedServiceCharge `json:"service_charge"`
	GrandTotalCents int                 `json:"grand_total_cents"`
}

// Parser turns a receipt image into structured bill data.
type Parser interface {
	Parse(ctx context.Context, image []byte, mediaType string) (ParsedReceipt, error)
}
