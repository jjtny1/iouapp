package transcribe

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

const whisperURL = "https://api.openai.com/v1/audio/transcriptions"

// WhisperTranscriber transcribes audio using the OpenAI Whisper API.
type WhisperTranscriber struct {
	APIKey string
	Client *http.Client
}

// NewWhisperTranscriber builds a WhisperTranscriber with a 60s HTTP timeout.
func NewWhisperTranscriber(apiKey string) WhisperTranscriber {
	return WhisperTranscriber{
		APIKey: apiKey,
		Client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (t WhisperTranscriber) Transcribe(ctx context.Context, audio []byte, filename string) (string, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)

	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("create form file: %w", err)
	}
	if _, err := fw.Write(audio); err != nil {
		return "", fmt.Errorf("write audio: %w", err)
	}
	if err := mw.WriteField("model", "whisper-1"); err != nil {
		return "", fmt.Errorf("write model field: %w", err)
	}
	if err := mw.WriteField("response_format", "text"); err != nil {
		return "", fmt.Errorf("write response_format field: %w", err)
	}
	if err := mw.Close(); err != nil {
		return "", fmt.Errorf("close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, whisperURL, &body)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+t.APIKey)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := t.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("call openai: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openai status %d: %s", resp.StatusCode, string(raw))
	}

	// With response_format=text the body is the plain transcript itself.
	transcript := strings.TrimSpace(string(raw))
	if transcript == "" {
		return "", fmt.Errorf("empty transcript")
	}
	return transcript, nil
}
