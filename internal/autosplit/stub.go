package autosplit

import "context"

// StubAssigner returns a fixed Assignment, ignoring the transcript and items.
// It keeps the audio-split flow usable when no Anthropic API key is configured,
// and is kept consistent with transcribe.StubTranscriber's sample transcript:
// the host Sam takes a cheeseburger and an iced tea, Riley takes the other
// cheeseburger and the caesar salad, Jordan takes the fries and two iced teas.
//
// The indices match the StubParser receipt after multi-quantity expansion:
//
//	1 Cheeseburger (1 of 2)   2 Cheeseburger (2 of 2)   3 Caesar Salad
//	4 Fries   5 Iced Tea (1 of 3)   6 Iced Tea (2 of 3)   7 Iced Tea (3 of 3)
type StubAssigner struct{}

func (StubAssigner) Assign(_ context.Context, _ []Item, _, _ string) (Assignment, error) {
	return Assignment{
		People: []string{"Sam", "Riley", "Jordan"},
		Items: []ItemAssignment{
			{Index: 1, People: []string{"Sam"}},
			{Index: 2, People: []string{"Riley"}},
			{Index: 3, People: []string{"Riley"}},
			{Index: 4, People: []string{"Jordan"}},
			{Index: 5, People: []string{"Sam"}},
			{Index: 6, People: []string{"Jordan"}},
			{Index: 7, People: []string{"Jordan"}},
		},
		Notes: "Assigned from a sample recording (no Anthropic API key configured).",
	}, nil
}
