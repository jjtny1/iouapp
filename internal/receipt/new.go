package receipt

import (
	"log"

	"github.com/jjtny1/iouapp/internal/config"
)

// New selects a Parser based on configuration: ClaudeParser when an
// Anthropic API key is present, otherwise the StubParser fallback.
func New(cfg config.Config) Parser {
	if cfg.AnthropicKey != "" {
		log.Printf("receipt: using ClaudeParser (Anthropic API)")
		return NewClaudeParser(cfg.AnthropicKey)
	}
	log.Printf("receipt: no ANTHROPIC_API_KEY set, using StubParser")
	return StubParser{}
}
