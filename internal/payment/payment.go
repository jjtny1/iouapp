// Package payment models settling a bill share in stablecoin.
//
// The design mirrors x402 ("HTTP 402 Payment Required"): the server issues a
// Challenge describing what to pay, the payer settles out of band and submits
// proof, and the server Verifies that proof, recording a transaction
// reference. A Provider abstracts the settlement backend so the mock used in
// development can be swapped for a real on-chain provider without touching the
// HTTP handlers.
package payment

import "context"

// Challenge describes a payment the payer must settle. It is the body of the
// HTTP 402 response returned when a friend initiates a payment.
type Challenge struct {
	PaymentID   string `json:"payment_id"`
	AmountCents int    `json:"amount_cents"`
	Currency    string `json:"currency"`
	Recipient   string `json:"recipient"`
	Network     string `json:"network"`
}

// Provider settles and verifies payments against a backend.
type Provider interface {
	// Name identifies the provider; it is persisted on each payment row.
	Name() string
	// Verify confirms that proof settles the given Challenge and returns a
	// transaction reference for the settled payment.
	Verify(ctx context.Context, ch Challenge, proof string) (txRef string, err error)
}

// Network is the settlement network advertised in challenges.
const Network = "base"

// Currency is the stablecoin advertised in challenges.
const Currency = "USDC"

// NewProvider selects a Provider by name. Anything other than "x402" yields
// the MockProvider, so development and tests settle instantly.
func NewProvider(name string) Provider {
	if name == "x402" {
		return X402Provider{}
	}
	return MockProvider{}
}
