package payment

import (
	"context"
	"errors"
)

// X402Provider is a SKELETON for real on-chain settlement and is intentionally
// not implemented.
//
// A real implementation would replace MockProvider as follows:
//
//   - Initiation: instead of the mock returning immediately, the server would
//     respond to POST /pay with an x402-compatible HTTP 402 carrying an
//     `accepts` payment-requirements object — the USDC asset on Base, the
//     Recipient address, and the AmountCents converted to USDC base units.
//   - Settlement: the payer's wallet (or an x402 client) signs and broadcasts
//     a USDC transfer to Recipient on the Base network, then submits the
//     resulting payload as `proof` to POST /pay/confirm.
//   - Verify: this method would parse that proof, confirm on Base (via an RPC
//     node or a facilitator service) that a USDC transfer of the exact amount
//     to Recipient was mined and finalized, and return the on-chain
//     transaction hash as txRef. Mismatched amount, recipient, or an
//     unconfirmed transaction would return an error so the payment stays
//     'pending'.
//
// No on-chain logic is implemented here.
type X402Provider struct{}

func (X402Provider) Name() string { return "x402" }

func (X402Provider) Verify(_ context.Context, _ Challenge, _ string) (string, error) {
	return "", errors.New("x402 settlement not implemented")
}
