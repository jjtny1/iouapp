package transcribe

import (
	"context"
	"strings"
	"testing"

	"github.com/jjtny1/iouapp/internal/config"
)

func TestStubTranscriber(t *testing.T) {
	tr := StubTranscriber{}
	inputs := [][]byte{
		nil,
		{},
		[]byte("not really audio"),
		[]byte{0x00, 0x01, 0x02},
	}
	for _, in := range inputs {
		got, err := tr.Transcribe(context.Background(), in, "clip.m4a")
		if err != nil {
			t.Fatalf("StubTranscriber.Transcribe error: %v", err)
		}
		if strings.TrimSpace(got) == "" {
			t.Error("expected a non-empty transcript")
		}
		// The transcript must name the host so "I"/"me" resolve correctly.
		if !strings.Contains(got, "Sam") {
			t.Errorf("transcript should mention host Sam, got %q", got)
		}
	}
}

func TestNewSelectsStubWithoutKey(t *testing.T) {
	if _, ok := New(config.Config{}).(StubTranscriber); !ok {
		t.Error("New with no OpenAI key should return a StubTranscriber")
	}
	if _, ok := New(config.Config{OpenAIKey: "sk-test"}).(WhisperTranscriber); !ok {
		t.Error("New with an OpenAI key should return a WhisperTranscriber")
	}
}
