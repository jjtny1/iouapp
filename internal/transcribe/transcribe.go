// Package transcribe turns a host's audio recording into a text transcript.
package transcribe

import (
	"context"
	"log"

	"github.com/jjtny1/iouapp/internal/config"
)

// Transcriber converts an audio file into a plain-text transcript.
type Transcriber interface {
	Transcribe(ctx context.Context, audio []byte, filename string) (string, error)
}

// New selects a Transcriber based on configuration: WhisperTranscriber when an
// OpenAI API key is present, otherwise the StubTranscriber fallback.
func New(cfg config.Config) Transcriber {
	if cfg.OpenAIKey != "" {
		log.Printf("transcribe: using WhisperTranscriber (OpenAI API)")
		return NewWhisperTranscriber(cfg.OpenAIKey)
	}
	log.Printf("transcribe: no OPENAI_API_KEY set, using StubTranscriber")
	return StubTranscriber{}
}
