package api

import (
	_ "embed"
	"log"
	"net/http"
)

//go:embed privacy.html
var privacyHTML string

// handlePrivacy serves the IOU privacy policy as a self-contained HTML page.
// It is a public, unauthenticated route registered before the SPA fallback so
// that the App Store listing can point its Privacy Policy URL at
// https://iouapp.ai/privacy.
func (s *Server) handlePrivacy(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(privacyHTML)); err != nil {
		log.Printf("handlePrivacy: %v", err)
	}
}
