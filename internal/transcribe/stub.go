package transcribe

import (
	"context"
	"strings"
)

// stubTranscript is a fixed sample recording describing how the StubParser's
// sample receipt splits. It names a host ("Sam") and two friends ("Riley",
// "Jordan"), and is kept consistent with autosplit.StubAssigner so the offline
// audio-split flow produces a sensible split end to end.
//
// The StubParser receipt, after multi-quantity expansion, has these items:
// two Cheeseburgers, one Caesar Salad, one Fries, three Iced Teas.
const stubTranscript = "Okay so this is Sam recording the split. " +
	"I had a cheeseburger and one of the iced teas. " +
	"Riley had the other cheeseburger and the caesar salad. " +
	"Jordan got the fries and the other two iced teas."

// StubTranscriber returns a fixed transcript, ignoring the audio bytes.
// It keeps the audio-split flow usable when no OpenAI API key is configured.
type StubTranscriber struct{}

func (StubTranscriber) Transcribe(_ context.Context, _ []byte, _ string) (string, error) {
	return strings.TrimSpace(stubTranscript), nil
}
