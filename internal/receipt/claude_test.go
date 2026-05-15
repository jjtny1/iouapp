package receipt

import (
	"context"
	"testing"
)

// wantReceipt is the structured receipt the test JSON inputs all encode.
// The inputs carry no currency field, so parseReceiptJSON defaults it to USD.
func wantReceipt() ParsedReceipt {
	return ParsedReceipt{
		Restaurant: "The Test Kitchen",
		Currency:   "USD",
		Items: []ParsedItem{
			{Name: "Soup", PriceCents: 800, Qty: 1},
			{Name: "Steak", PriceCents: 2995, Qty: 2},
		},
		TaxCents:      350,
		TipCents:      600,
		ServiceCharge: ParsedServiceCharge{Kind: "none"},
	}
}

const bareJSON = `{"restaurant":"The Test Kitchen","items":[` +
	`{"name":"Soup","price_cents":800,"qty":1},` +
	`{"name":"Steak","price_cents":2995,"qty":2}],` +
	`"tax_cents":350,"tip_cents":600}`

func assertReceiptEqual(t *testing.T, got, want ParsedReceipt) {
	t.Helper()
	if got.Restaurant != want.Restaurant {
		t.Errorf("restaurant = %q, want %q", got.Restaurant, want.Restaurant)
	}
	if got.Currency != want.Currency {
		t.Errorf("currency = %q, want %q", got.Currency, want.Currency)
	}
	if got.TaxCents != want.TaxCents {
		t.Errorf("tax_cents = %d, want %d", got.TaxCents, want.TaxCents)
	}
	if got.TipCents != want.TipCents {
		t.Errorf("tip_cents = %d, want %d", got.TipCents, want.TipCents)
	}
	if got.ServiceCharge != want.ServiceCharge {
		t.Errorf("service_charge = %+v, want %+v", got.ServiceCharge, want.ServiceCharge)
	}
	if len(got.Items) != len(want.Items) {
		t.Fatalf("items len = %d, want %d", len(got.Items), len(want.Items))
	}
	for i := range want.Items {
		if got.Items[i] != want.Items[i] {
			t.Errorf("item %d = %+v, want %+v", i, got.Items[i], want.Items[i])
		}
	}
}

func TestParseReceiptJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:  "bare JSON object",
			input: bareJSON,
		},
		{
			name:  "fenced json code block",
			input: "```json\n" + bareJSON + "\n```",
		},
		{
			name:  "fenced code block without language",
			input: "```\n" + bareJSON + "\n```",
		},
		{
			name:  "surrounding prose",
			input: "Here is the receipt:\n" + bareJSON + "\nLet me know if you need anything else.",
		},
		{
			name:  "fenced block with leading prose",
			input: "Sure! Here is the parsed data:\n```json\n" + bareJSON + "\n```\nThanks!",
		},
		{
			name:    "no JSON at all",
			input:   "I could not read the receipt image, sorry.",
			wantErr: true,
		},
		{
			name:    "malformed JSON",
			input:   `{"restaurant":"Broken","items":[{"name":`,
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseReceiptJSON(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got receipt %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertReceiptEqual(t, got, wantReceipt())
		})
	}
}

// TestParseReceiptJSONQtyDefault verifies a zero/missing qty is coerced to 1.
func TestParseReceiptJSONQtyDefault(t *testing.T) {
	input := `{"restaurant":"Q","items":[{"name":"Water","price_cents":0,"qty":0},` +
		`{"name":"Bread","price_cents":300}],"tax_cents":0,"tip_cents":0}`
	got, err := parseReceiptJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i, it := range got.Items {
		if it.Qty != 1 {
			t.Errorf("item %d qty = %d, want 1", i, it.Qty)
		}
	}
}

// TestParseReceiptJSONCurrency verifies the currency is read and normalized,
// defaulting to USD when missing or malformed.
func TestParseReceiptJSONCurrency(t *testing.T) {
	tests := []struct {
		name     string
		currency string // value of the "currency" field, or "" to omit it
		want     string
	}{
		{name: "explicit PLN", currency: `"currency":"PLN",`, want: "PLN"},
		{name: "lowercase normalized", currency: `"currency":"eur",`, want: "EUR"},
		{name: "omitted defaults to USD", currency: "", want: "USD"},
		{name: "empty defaults to USD", currency: `"currency":"",`, want: "USD"},
		{name: "malformed defaults to USD", currency: `"currency":"dollars",`, want: "USD"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := `{"restaurant":"C",` + tt.currency +
				`"items":[{"name":"X","price_cents":100,"qty":1}],` +
				`"tax_cents":0,"tip_cents":0}`
			got, err := parseReceiptJSON(input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Currency != tt.want {
				t.Errorf("currency = %q, want %q", got.Currency, tt.want)
			}
		})
	}
}

// TestParseReceiptJSONServiceCharge verifies service charges are read and
// normalized: a valid percent or fixed charge is kept, anything else (an
// unknown kind, a non-positive amount, or an omitted field) becomes "none".
func TestParseReceiptJSONServiceCharge(t *testing.T) {
	tests := []struct {
		name string
		sc   string // value of the "service_charge" field, or "" to omit it
		want ParsedServiceCharge
	}{
		{
			name: "percent charge",
			sc:   `"service_charge":{"kind":"percent","percent":12.5},`,
			want: ParsedServiceCharge{Kind: "percent", Percent: 12.5},
		},
		{
			name: "fixed charge",
			sc:   `"service_charge":{"kind":"fixed","amount_cents":500},`,
			want: ParsedServiceCharge{Kind: "fixed", AmountCents: 500},
		},
		{
			name: "omitted defaults to none",
			sc:   "",
			want: ParsedServiceCharge{Kind: "none"},
		},
		{
			name: "zero percent degrades to none",
			sc:   `"service_charge":{"kind":"percent","percent":0},`,
			want: ParsedServiceCharge{Kind: "none"},
		},
		{
			name: "unknown kind degrades to none",
			sc:   `"service_charge":{"kind":"tip"},`,
			want: ParsedServiceCharge{Kind: "none"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := `{"restaurant":"S",` + tt.sc +
				`"items":[{"name":"X","price_cents":100,"qty":1}],` +
				`"tax_cents":0,"tip_cents":0}`
			got, err := parseReceiptJSON(input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.ServiceCharge != tt.want {
				t.Errorf("service_charge = %+v, want %+v", got.ServiceCharge, tt.want)
			}
		})
	}
}

func TestStubParser(t *testing.T) {
	p := StubParser{}
	inputs := [][]byte{
		nil,
		{},
		[]byte("not really an image"),
		[]byte{0xFF, 0xD8, 0xFF},
	}
	for _, in := range inputs {
		got, err := p.Parse(context.Background(), in, "image/jpeg")
		if err != nil {
			t.Fatalf("StubParser.Parse error: %v", err)
		}
		if got.Restaurant == "" {
			t.Error("expected non-empty restaurant")
		}
		if got.Currency != "USD" {
			t.Errorf("currency = %q, want USD", got.Currency)
		}
		if len(got.Items) == 0 {
			t.Error("expected at least one item")
		}
		if got.TaxCents <= 0 {
			t.Errorf("expected positive tax, got %d", got.TaxCents)
		}
		if got.TipCents <= 0 {
			t.Errorf("expected positive tip, got %d", got.TipCents)
		}
		for i, it := range got.Items {
			if it.Name == "" || it.PriceCents <= 0 || it.Qty < 1 {
				t.Errorf("item %d looks invalid: %+v", i, it)
			}
		}
	}
}
