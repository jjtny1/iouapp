// Package money holds shared money helpers. IOU tracks every amount as
// integer cents — hundredths of the currency's major unit — and this package
// centralizes currency-code handling so the receipt parser and the HTTP API
// agree on what a valid currency looks like.
package money

import "strings"

// DefaultCurrency is used when no currency is detected or supplied.
const DefaultCurrency = "USD"

// NormalizeCurrency upper-cases and trims code, returning it with ok=true when
// it is a well-formed ISO 4217 alphabetic code: exactly three ASCII letters.
// It does not check the code against the official ISO 4217 registry — any
// three-letter code is accepted, which keeps the app open to new currencies
// without a list to maintain.
func NormalizeCurrency(code string) (normalized string, ok bool) {
	c := strings.ToUpper(strings.TrimSpace(code))
	if len(c) != 3 {
		return "", false
	}
	for _, r := range c {
		if r < 'A' || r > 'Z' {
			return "", false
		}
	}
	return c, true
}

// CurrencyOrDefault returns the normalized form of code, falling back to
// DefaultCurrency when code is empty or malformed.
func CurrencyOrDefault(code string) string {
	if c, ok := NormalizeCurrency(code); ok {
		return c
	}
	return DefaultCurrency
}
