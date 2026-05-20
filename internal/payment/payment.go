// Package payment builds Venmo deep links so a friend can settle their share
// of a bill.
//
// Venmo gives the app no settlement callback — once a friend is handed off to
// the Venmo app or website the app cannot observe whether the transfer
// completed. There is therefore no verification step: the server hands the
// friend a payment intent (the host's handle, the amount, and ready-made
// links) and the payment is marked paid by the friend's self-report or by the
// host confirming it.
package payment

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// handlePattern matches a valid Venmo username: 5–30 characters of letters,
// digits, underscores and hyphens. (Venmo also accepts phone numbers and
// emails as recipients, but the host always supplies a username here.)
var handlePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{5,30}$`)

// NormalizeHandle trims surrounding space and a leading "@" from a Venmo
// username. ok is false when what remains is not a valid handle.
func NormalizeHandle(raw string) (handle string, ok bool) {
	h := strings.TrimSpace(raw)
	h = strings.TrimPrefix(h, "@")
	h = strings.TrimSpace(h)
	if !handlePattern.MatchString(h) {
		return "", false
	}
	return h, true
}

// amountParam formats integer cents as a plain decimal string (1234 → "12.34")
// for the Venmo "amount" query parameter.
func amountParam(cents int) string {
	if cents < 0 {
		cents = 0
	}
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}

// payURL builds the Venmo Universal Link that prefills a payment to handle for
// amountCents with note. Venmo's iOS and Android apps claim venmo.com, so the
// same https URL opens the app when installed and falls back to venmo.com web
// otherwise — one link covers phones, desktop browsers, and QR scans.
//
// The legacy venmo://paycharge?txn=pay&recipients=… scheme this replaced
// stopped working in 2024: the current Venmo app treats it as an unknown
// "Venmo Code" and shows "We don't recognize that code. Recheck and try
// again." The path-based handle (venmo.com/<user>) is the format Venmo's own
// share links and docs use today.
func payURL(handle string, amountCents int, note string) string {
	q := url.Values{}
	q.Set("txn", "pay")
	q.Set("amount", amountParam(amountCents))
	// Venmo's note display field renders both "+" and "%20" as a literal "+"
	// in the prefilled payment — both standard space encodings are mangled.
	// Substitute regular spaces with U+00A0 (non-breaking space) before
	// encoding: it's visually identical to a regular space, but its URL
	// encoding (%C2%A0) doesn't go through Venmo's space-mangling code path.
	// Verified manually with a beta tester after two attempts at the standard
	// encodings both shipped a visible "+" between every word.
	q.Set("note", strings.ReplaceAll(note, " ", " "))
	return "https://venmo.com/" + handle + "?" + q.Encode()
}

// AppURL builds the link used to hand a phone off to Venmo and the value
// encoded into the desktop QR code. It is a Universal Link, so iOS / Android
// open the Venmo app prefilled when installed.
func AppURL(handle string, amountCents int, note string) string {
	return payURL(handle, amountCents, note)
}

// WebURL is the same Universal Link, exposed under a separate name for the
// payment intent's web fallback field. On a desktop with no Venmo app it
// opens venmo.com's web pay flow.
func WebURL(handle string, amountCents int, note string) string {
	return payURL(handle, amountCents, note)
}
