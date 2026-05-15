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

// payQuery builds the query string shared by the app and web pay links.
// Spaces are percent-encoded as %20 rather than "+": Venmo's deep-link
// parser does not treat "+" as a space, so form-style encoding would show a
// literal "+" between every word of the note.
func payQuery(handle string, amountCents int, note string) string {
	q := url.Values{}
	q.Set("txn", "pay")
	q.Set("recipients", handle)
	q.Set("amount", amountParam(amountCents))
	q.Set("note", note)
	return strings.ReplaceAll(q.Encode(), "+", "%20")
}

// AppURL builds a venmo:// deep link that opens the Venmo app prefilled to pay
// handle the given amount with note. It is the link offered on phones and
// encoded into the desktop QR code.
func AppURL(handle string, amountCents int, note string) string {
	return "venmo://paycharge?" + payQuery(handle, amountCents, note)
}

// WebURL builds an https://venmo.com pay link for desktop browsers. On a
// desktop with no Venmo app it opens Venmo's web pay flow.
func WebURL(handle string, amountCents int, note string) string {
	return "https://account.venmo.com/pay?" + payQuery(handle, amountCents, note)
}
