package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jjtny1/splitit/internal/auth"
)

type user struct {
	ID            string  `json:"id"`
	Email         string  `json:"email"`
	WalletAddress *string `json:"wallet_address"`
}

type ctxKey string

const userCtxKey ctxKey = "user"

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return false
	}
	return true
}

func (s *Server) handleAuthRequest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" || !strings.Contains(email, "@") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "valid email required"})
		return
	}

	token, err := auth.NewToken()
	if err != nil {
		log.Printf("auth request: token: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	now := time.Now()
	_, err = s.DB.ExecContext(r.Context(),
		`INSERT INTO magic_links (token, email, expires_at, used, created_at) VALUES (?, ?, ?, 0, ?)`,
		token, email, now.Add(auth.MagicLinkTTL).Unix(), now.Unix())
	if err != nil {
		log.Printf("auth request: insert: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	link := s.Cfg.BaseURL + "/auth/verify?token=" + token
	if err := s.Mailer.Send(email, link); err != nil {
		log.Printf("auth request: send: %v", err)
	}

	resp := map[string]string{"message": "If that email is registered, a magic link has been sent."}
	if s.Cfg.DevMode {
		resp["link"] = link
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleAuthVerify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token required"})
		return
	}

	now := time.Now()
	var email string
	var expiresAt, used int64
	err := s.DB.QueryRowContext(r.Context(),
		`SELECT email, expires_at, used FROM magic_links WHERE token = ?`, req.Token).
		Scan(&email, &expiresAt, &used)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid or expired link"})
		return
	}
	if err != nil {
		log.Printf("auth verify: select: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if used != 0 || now.Unix() > expiresAt {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid or expired link"})
		return
	}

	if _, err := s.DB.ExecContext(r.Context(),
		`UPDATE magic_links SET used = 1 WHERE token = ?`, req.Token); err != nil {
		log.Printf("auth verify: mark used: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	u, err := s.upsertUser(r.Context(), email)
	if err != nil {
		log.Printf("auth verify: upsert user: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	sessionToken, err := auth.NewToken()
	if err != nil {
		log.Printf("auth verify: session token: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if _, err := s.DB.ExecContext(r.Context(),
		`INSERT INTO sessions (token, user_id, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		sessionToken, u.ID, now.Add(auth.SessionTTL).Unix(), now.Unix()); err != nil {
		log.Printf("auth verify: insert session: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookie,
		Value:    sessionToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   !s.Cfg.DevMode,
		SameSite: http.SameSiteLaxMode,
		Expires:  now.Add(auth.SessionTTL),
		MaxAge:   int(auth.SessionTTL.Seconds()),
	})
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	u, ok := s.currentUser(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(auth.SessionCookie); err == nil {
		if _, err := s.DB.ExecContext(r.Context(),
			`DELETE FROM sessions WHERE token = ?`, c.Value); err != nil {
			log.Printf("logout: delete session: %v", err)
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   !s.Cfg.DevMode,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]string{"message": "signed out"})
}

func (s *Server) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	u, _ := r.Context().Value(userCtxKey).(user)
	var req struct {
		WalletAddress string `json:"wallet_address"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	wallet := strings.TrimSpace(req.WalletAddress)
	var walletVal any
	if wallet != "" {
		walletVal = wallet
	}
	if _, err := s.DB.ExecContext(r.Context(),
		`UPDATE users SET wallet_address = ? WHERE id = ?`, walletVal, u.ID); err != nil {
		log.Printf("update me: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if wallet != "" {
		u.WalletAddress = &wallet
	} else {
		u.WalletAddress = nil
	}
	writeJSON(w, http.StatusOK, u)
}

// upsertUser returns the user for email, creating one if it does not exist.
func (s *Server) upsertUser(ctx context.Context, email string) (user, error) {
	u, err := s.userByEmail(ctx, email)
	if err == nil {
		return u, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return user{}, err
	}
	id := uuid.NewString()
	if _, err := s.DB.ExecContext(ctx,
		`INSERT INTO users (id, email, created_at) VALUES (?, ?, ?)`,
		id, email, time.Now().Unix()); err != nil {
		return user{}, err
	}
	return user{ID: id, Email: email}, nil
}

func (s *Server) userByEmail(ctx context.Context, email string) (user, error) {
	var u user
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, email, wallet_address FROM users WHERE email = ?`, email).
		Scan(&u.ID, &u.Email, &u.WalletAddress)
	return u, err
}

// currentUser resolves the authenticated user from the session cookie.
func (s *Server) currentUser(r *http.Request) (user, bool) {
	c, err := r.Cookie(auth.SessionCookie)
	if err != nil || c.Value == "" {
		return user{}, false
	}
	var u user
	var expiresAt int64
	err = s.DB.QueryRowContext(r.Context(),
		`SELECT u.id, u.email, u.wallet_address, sessions.expires_at
		 FROM sessions JOIN users u ON u.id = sessions.user_id
		 WHERE sessions.token = ?`, c.Value).
		Scan(&u.ID, &u.Email, &u.WalletAddress, &expiresAt)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("currentUser: %v", err)
		}
		return user{}, false
	}
	if time.Now().Unix() > expiresAt {
		return user{}, false
	}
	return u, true
}

// requireAuth wraps h, rejecting unauthenticated requests with 401 and
// otherwise placing the user in the request context.
func (s *Server) requireAuth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := s.currentUser(r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
			return
		}
		ctx := context.WithValue(r.Context(), userCtxKey, u)
		h(w, r.WithContext(ctx))
	}
}
