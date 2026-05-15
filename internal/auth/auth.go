// Package auth provides magic-link authentication primitives: token
// generation, email delivery, and TTL constants.
package auth

import (
	"crypto/rand"
	"encoding/base64"
	"log"
	"time"
)

const (
	// MagicLinkTTL is how long a magic link remains valid.
	MagicLinkTTL = 15 * time.Minute
	// SessionTTL is how long a session cookie remains valid.
	SessionTTL = 30 * 24 * time.Hour
	// SessionCookie is the name of the session cookie.
	SessionCookie = "iou_session"
)

// NewToken returns a cryptographically random URL-safe token.
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// EmailSender delivers a magic-link URL to an email address.
type EmailSender interface {
	Send(email, link string) error
}

// LogSender is an EmailSender that writes the magic link to stdout.
// It is intended for development; in production a real sender is used.
type LogSender struct{}

func (LogSender) Send(email, link string) error {
	log.Printf("magic link for %s: %s", email, link)
	return nil
}
