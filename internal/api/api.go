package api

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/jjtny1/iouapp/internal/auth"
	"github.com/jjtny1/iouapp/internal/autosplit"
	"github.com/jjtny1/iouapp/internal/config"
	"github.com/jjtny1/iouapp/internal/db"
	"github.com/jjtny1/iouapp/internal/receipt"
	"github.com/jjtny1/iouapp/internal/transcribe"
)

type Server struct {
	DB          *db.DB
	Cfg         config.Config
	Mailer      auth.EmailSender
	Parser      receipt.Parser
	Transcriber transcribe.Transcriber
	Assigner    autosplit.Assigner
}

func NewRouter(database *db.DB, cfg config.Config, mailer auth.EmailSender) http.Handler {
	s := &Server{
		DB:          database,
		Cfg:         cfg,
		Mailer:      mailer,
		Parser:      receipt.New(cfg),
		Transcriber: transcribe.New(cfg),
		Assigner:    autosplit.New(cfg),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /.well-known/apple-app-site-association", s.handleAASA)
	mux.HandleFunc("GET /privacy", s.handlePrivacy)
	mux.HandleFunc("POST /api/auth/request", s.handleAuthRequest)
	mux.HandleFunc("POST /api/auth/verify", s.handleAuthVerify)
	mux.HandleFunc("GET /api/auth/me", s.handleAuthMe)
	mux.HandleFunc("POST /api/auth/logout", s.handleAuthLogout)
	mux.HandleFunc("PATCH /api/users/me", s.requireAuth(s.handleUpdateMe))
	mux.HandleFunc("POST /api/bills", s.requireAuth(s.handleCreateBill))
	mux.HandleFunc("GET /api/bills", s.requireAuth(s.handleListBills))
	mux.HandleFunc("GET /api/bills/joined", s.requireAuth(s.handleListJoinedBills))
	mux.HandleFunc("GET /api/bills/{id}", s.handleGetBill)
	mux.HandleFunc("POST /api/bills/{id}/receipt", s.requireAuth(s.handleBillReceipt))
	mux.HandleFunc("POST /api/bills/{id}/auto-split", s.requireAuth(s.handleAutoSplit))
	mux.HandleFunc("PATCH /api/bills/{id}", s.requireAuth(s.handleUpdateBill))
	mux.HandleFunc("DELETE /api/bills/{id}", s.requireAuth(s.handleDeleteBill))
	mux.HandleFunc("GET /api/by-token/{token}", s.handleBillByToken)
	mux.HandleFunc("POST /api/bills/{id}/participants", s.handleJoinBill)
	mux.HandleFunc("POST /api/bills/{id}/participants/{pid}/link", s.requireAuth(s.handleLinkIdentity))
	mux.HandleFunc("GET /api/bills/{id}/my-participant", s.requireAuth(s.handleMyParticipant))
	mux.HandleFunc("PUT /api/bills/{id}/claims", s.handleSetClaims)
	mux.HandleFunc("GET /api/bills/{id}/summary", s.handleSummary)
	mux.HandleFunc("POST /api/bills/{id}/pay", s.handlePay)
	mux.HandleFunc("POST /api/bills/{id}/pay/confirm", s.handlePayConfirm)
	mux.HandleFunc("GET /api/bills/{id}/payments", s.handleListPayments)
	mux.HandleFunc("POST /api/bills/{id}/payments/{pid}", s.requireAuth(s.handleMarkPayment))
	mux.Handle("/", spaHandler("web/dist"))

	return logging(cors(mux))
}

// allowedOrigins are the cross-origin callers permitted by CORS. The web app
// is served by this same server, so it is same-origin and never triggers
// CORS; the only cross-origin caller is the native iOS app, whose Capacitor
// WebView is served from capacitor://localhost. (If a CORS request is wrongly
// rejected, check the request's actual Origin header against this set.)
var allowedOrigins = map[string]bool{
	"capacitor://localhost": true,
}

// cors answers CORS preflight requests and tags allowed cross-origin
// responses. Native-app requests authenticate with a bearer token, not a
// cookie, so Access-Control-Allow-Credentials is intentionally not sent.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); allowedOrigins[origin] {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Add("Vary", "Origin")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			h.Set("Access-Control-Max-Age", "86400")
		}
		// Preflight requests carry no auth and match no API route — answer
		// them here before the mux sends them to the SPA fallback.
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleAASA serves the Apple App Site Association file. iOS fetches it over
// HTTPS to authorize the native app to handle iouapp.ai Universal Links —
// specifically the magic-link sign-in path /auth/verify, so tapping the email
// link opens the app instead of Safari. It is published only when
// IOU_APPLE_APP_ID is configured; until then it 404s, the correct
// "no association" state.
func (s *Server) handleAASA(w http.ResponseWriter, r *http.Request) {
	if s.Cfg.AppleAppID == "" {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"applinks": map[string]any{
			"details": []any{
				map[string]any{
					"appIDs": []string{s.Cfg.AppleAppID},
					"components": []any{
						map[string]any{
							"/":       "/auth/verify",
							"comment": "magic-link sign-in",
						},
					},
				},
			},
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON: %v", err)
	}
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}
