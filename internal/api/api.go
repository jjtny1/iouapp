package api

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/jjtny1/splitit/internal/auth"
	"github.com/jjtny1/splitit/internal/autosplit"
	"github.com/jjtny1/splitit/internal/config"
	"github.com/jjtny1/splitit/internal/db"
	"github.com/jjtny1/splitit/internal/receipt"
	"github.com/jjtny1/splitit/internal/transcribe"
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
	mux.HandleFunc("POST /api/auth/request", s.handleAuthRequest)
	mux.HandleFunc("POST /api/auth/verify", s.handleAuthVerify)
	mux.HandleFunc("GET /api/auth/me", s.handleAuthMe)
	mux.HandleFunc("POST /api/auth/logout", s.handleAuthLogout)
	mux.HandleFunc("PATCH /api/users/me", s.requireAuth(s.handleUpdateMe))
	mux.HandleFunc("POST /api/bills", s.requireAuth(s.handleCreateBill))
	mux.HandleFunc("GET /api/bills", s.requireAuth(s.handleListBills))
	mux.HandleFunc("GET /api/bills/{id}", s.handleGetBill)
	mux.HandleFunc("POST /api/bills/{id}/receipt", s.requireAuth(s.handleBillReceipt))
	mux.HandleFunc("POST /api/bills/{id}/audio-split", s.requireAuth(s.handleAudioSplit))
	mux.HandleFunc("PATCH /api/bills/{id}", s.requireAuth(s.handleUpdateBill))
	mux.HandleFunc("DELETE /api/bills/{id}", s.requireAuth(s.handleDeleteBill))
	mux.HandleFunc("GET /api/by-token/{token}", s.handleBillByToken)
	mux.HandleFunc("POST /api/bills/{id}/participants", s.handleJoinBill)
	mux.HandleFunc("PUT /api/bills/{id}/claims", s.handleSetClaims)
	mux.HandleFunc("GET /api/bills/{id}/summary", s.handleSummary)
	mux.HandleFunc("POST /api/bills/{id}/pay", s.handlePay)
	mux.HandleFunc("POST /api/bills/{id}/pay/confirm", s.handlePayConfirm)
	mux.HandleFunc("GET /api/bills/{id}/payments", s.handleListPayments)
	mux.HandleFunc("POST /api/bills/{id}/payments/{pid}", s.requireAuth(s.handleMarkPayment))
	mux.Handle("/", spaHandler("web/dist"))

	return logging(mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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
