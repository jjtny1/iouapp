package money

import "testing"

func TestNormalizeCurrency(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   string
		wantOK bool
	}{
		{name: "lowercase", input: "usd", want: "USD", wantOK: true},
		{name: "uppercase", input: "PLN", want: "PLN", wantOK: true},
		{name: "mixed case with spaces", input: "  eUr ", want: "EUR", wantOK: true},
		{name: "too short", input: "US", wantOK: false},
		{name: "too long", input: "USDC", wantOK: false},
		{name: "empty", input: "", wantOK: false},
		{name: "digits", input: "US1", wantOK: false},
		{name: "symbol", input: "$$$", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := NormalizeCurrency(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got != tt.want {
				t.Errorf("got = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCurrencyOrDefault(t *testing.T) {
	if got := CurrencyOrDefault("jpy"); got != "JPY" {
		t.Errorf("CurrencyOrDefault(jpy) = %q, want JPY", got)
	}
	if got := CurrencyOrDefault(""); got != DefaultCurrency {
		t.Errorf("CurrencyOrDefault(\"\") = %q, want %q", got, DefaultCurrency)
	}
	if got := CurrencyOrDefault("bogus"); got != DefaultCurrency {
		t.Errorf("CurrencyOrDefault(bogus) = %q, want %q", got, DefaultCurrency)
	}
}
