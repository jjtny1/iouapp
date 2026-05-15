package receipt

import "context"

// StubParser returns a fixed sample receipt, ignoring the image bytes.
// It keeps the app usable when no Anthropic API key is configured.
type StubParser struct{}

func (StubParser) Parse(_ context.Context, _ []byte, _ string) (ParsedReceipt, error) {
	return ParsedReceipt{
		Restaurant: "Sample Diner",
		Currency:   "USD",
		Items: []ParsedItem{
			{Name: "Cheeseburger", PriceCents: 1395, Qty: 2},
			{Name: "Caesar Salad", PriceCents: 1050, Qty: 1},
			{Name: "Fries", PriceCents: 595, Qty: 1},
			{Name: "Iced Tea", PriceCents: 350, Qty: 3},
		},
		TaxCents:      412,
		TipCents:      900,
		ServiceCharge: ParsedServiceCharge{Kind: "percent", Percent: 10},
	}, nil
}
