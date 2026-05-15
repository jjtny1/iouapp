package payment

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

// MockProvider simulates instant stablecoin settlement. Verify always
// succeeds, ignoring the proof, and returns a synthetic transaction
// reference. It exercises the full payment code path without real funds.
type MockProvider struct{}

func (MockProvider) Name() string { return "mock" }

func (MockProvider) Verify(_ context.Context, _ Challenge, _ string) (string, error) {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "mock-tx-" + hex.EncodeToString(buf), nil
}
