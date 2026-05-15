package api

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jjtny1/splitit/internal/payment"
	"github.com/jjtny1/splitit/internal/split"
)

type paymentRow struct {
	ID            string
	BillID        string
	ParticipantID string
	AmountCents   int
	Currency      string
	Recipient     string
	Status        string
	Provider      string
	TxRef         sql.NullString
	CreatedAt     int64
	UpdatedAt     int64
}

func (p paymentRow) json() map[string]any {
	out := map[string]any{
		"id":             p.ID,
		"participant_id": p.ParticipantID,
		"amount_cents":   p.AmountCents,
		"currency":       p.Currency,
		"status":         p.Status,
		"provider":       p.Provider,
		"recipient":      p.Recipient,
	}
	if p.TxRef.Valid {
		out["tx_ref"] = p.TxRef.String
	} else {
		out["tx_ref"] = nil
	}
	return out
}

// participantByToken resolves a participant_token to its id, verifying the
// participant belongs to the given bill.
func (s *Server) participantByToken(ctx context.Context, billID, token string) (string, error) {
	var id, partBill string
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, bill_id FROM participants WHERE participant_token = ?`, token).
		Scan(&id, &partBill)
	if err != nil {
		return "", err
	}
	if partBill != billID {
		return "", sql.ErrNoRows
	}
	return id, nil
}

// amountOwed computes a participant's current share of a bill by reusing the
// split computation. It returns 0 if the participant has claimed nothing.
func (s *Server) amountOwed(ctx context.Context, b bill, participantID string) (int, error) {
	items, err := s.loadItems(ctx, b.ID)
	if err != nil {
		return 0, err
	}
	claims, err := s.loadClaims(ctx, b.ID)
	if err != nil {
		return 0, err
	}
	splitItems := make([]split.Item, 0, len(items))
	for _, it := range items {
		splitItems = append(splitItems, split.Item{
			ID:         it.ID,
			TotalCents: it.PriceCents,
		})
	}
	summary := split.Compute(splitItems, b.TaxCents, b.TipCents, claims)
	for _, ps := range summary.Participants {
		if ps.ParticipantID == participantID {
			return ps.TotalCents, nil
		}
	}
	return 0, nil
}

// loadPayment fetches the payment row for a participant, if any.
func (s *Server) loadPayment(ctx context.Context, participantID string) (paymentRow, error) {
	var p paymentRow
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, bill_id, participant_id, amount_cents, currency, recipient,
		        status, provider, tx_ref, created_at, updated_at
		 FROM payments WHERE participant_id = ?`, participantID).
		Scan(&p.ID, &p.BillID, &p.ParticipantID, &p.AmountCents, &p.Currency,
			&p.Recipient, &p.Status, &p.Provider, &p.TxRef, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

// hostWallet returns the host's payout wallet address for a bill.
func (s *Server) hostWallet(ctx context.Context, hostUserID string) (string, error) {
	var wallet sql.NullString
	if err := s.DB.QueryRowContext(ctx,
		`SELECT wallet_address FROM users WHERE id = ?`, hostUserID).Scan(&wallet); err != nil {
		return "", err
	}
	if !wallet.Valid || strings.TrimSpace(wallet.String) == "" {
		return "", nil
	}
	return strings.TrimSpace(wallet.String), nil
}

// handlePay initiates a payment for a friend and responds with an HTTP 402
// payment challenge. If the friend has already paid it returns the existing
// paid payment with HTTP 200 (idempotent).
func (s *Server) handlePay(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req struct {
		ParticipantToken string `json:"participant_token"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	b, err := s.loadBill(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "bill not found"})
		return
	}
	if err != nil {
		log.Printf("pay: load bill: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	participantID, err := s.participantByToken(r.Context(), b.ID, req.ParticipantToken)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "participant not found"})
		return
	}
	if err != nil {
		log.Printf("pay: participant: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	existing, err := s.loadPayment(r.Context(), participantID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.Printf("pay: load payment: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	hasExisting := err == nil
	if hasExisting && existing.Status == "paid" {
		writeJSON(w, http.StatusOK, existing.json())
		return
	}

	wallet, err := s.hostWallet(r.Context(), b.hostUserID)
	if err != nil {
		log.Printf("pay: host wallet: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if wallet == "" {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "the host has not set a payout address yet",
		})
		return
	}

	amount, err := s.amountOwed(r.Context(), b, participantID)
	if err != nil {
		log.Printf("pay: amount: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	now := time.Now().Unix()
	var p paymentRow
	if !hasExisting {
		// The payment settles in the stablecoin advertised by payment.Currency
		// (USDC). When the bill's own currency is not USD, amount is still its
		// raw owed value — FX conversion from the bill currency to the
		// settlement currency is intentionally deferred to the planned x402
		// work and is not applied here.
		p = paymentRow{
			ID:            uuid.NewString(),
			BillID:        b.ID,
			ParticipantID: participantID,
			AmountCents:   amount,
			Currency:      payment.Currency,
			Recipient:     wallet,
			Status:        "pending",
			Provider:      s.Payment.Name(),
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if _, err := s.DB.ExecContext(r.Context(),
			`INSERT INTO payments (id, bill_id, participant_id, amount_cents, currency,
			        recipient, status, provider, tx_ref, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?)`,
			p.ID, p.BillID, p.ParticipantID, p.AmountCents, p.Currency,
			p.Recipient, p.Status, p.Provider, p.CreatedAt, p.UpdatedAt); err != nil {
			log.Printf("pay: insert: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
	} else {
		// Reuse the pending row, refreshing the amount and recipient in case
		// claims or the host wallet changed since it was created.
		p = existing
		p.AmountCents = amount
		p.Recipient = wallet
		p.UpdatedAt = now
		if _, err := s.DB.ExecContext(r.Context(),
			`UPDATE payments SET amount_cents = ?, recipient = ?, updated_at = ?
			 WHERE id = ?`,
			p.AmountCents, p.Recipient, p.UpdatedAt, p.ID); err != nil {
			log.Printf("pay: update: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
	}

	writeJSON(w, http.StatusPaymentRequired, payment.Challenge{
		PaymentID:   p.ID,
		AmountCents: p.AmountCents,
		Currency:    p.Currency,
		Recipient:   p.Recipient,
		Network:     payment.Network,
	})
}

// handlePayConfirm verifies submitted payment proof and marks the payment
// paid on success.
func (s *Server) handlePayConfirm(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req struct {
		ParticipantToken string `json:"participant_token"`
		PaymentID        string `json:"payment_id"`
		Proof            string `json:"proof"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	b, err := s.loadBill(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "bill not found"})
		return
	}
	if err != nil {
		log.Printf("pay confirm: load bill: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	participantID, err := s.participantByToken(r.Context(), b.ID, req.ParticipantToken)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "participant not found"})
		return
	}
	if err != nil {
		log.Printf("pay confirm: participant: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	p, err := s.loadPayment(r.Context(), participantID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil &&
		(p.ID != req.PaymentID || p.BillID != b.ID)) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "payment not found"})
		return
	}
	if err != nil {
		log.Printf("pay confirm: load payment: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if p.Status != "pending" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "payment is not pending"})
		return
	}

	ch := payment.Challenge{
		PaymentID:   p.ID,
		AmountCents: p.AmountCents,
		Currency:    p.Currency,
		Recipient:   p.Recipient,
		Network:     payment.Network,
	}
	txRef, err := s.Payment.Verify(r.Context(), ch, req.Proof)
	if err != nil {
		log.Printf("pay confirm: verify: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "payment verification failed"})
		return
	}

	now := time.Now().Unix()
	if _, err := s.DB.ExecContext(r.Context(),
		`UPDATE payments SET status = 'paid', tx_ref = ?, provider = ?, updated_at = ?
		 WHERE id = ?`,
		txRef, s.Payment.Name(), now, p.ID); err != nil {
		log.Printf("pay confirm: update: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	p.Status = "paid"
	p.TxRef = sql.NullString{String: txRef, Valid: true}
	p.Provider = s.Payment.Name()
	p.UpdatedAt = now
	writeJSON(w, http.StatusOK, p.json())
}

// handleListPayments returns the payment rows for a bill, for the host or a
// friend holding the share token.
func (s *Server) handleListPayments(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	b, err := s.loadBill(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "bill not found"})
		return
	}
	if err != nil {
		log.Printf("list payments: load: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	u, authed := s.currentUser(r)
	host := authed && u.ID == b.hostUserID
	if !host && r.URL.Query().Get("t") != b.friendToken {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "bill not found"})
		return
	}

	payments, err := s.loadPayments(r.Context(), b.ID)
	if err != nil {
		log.Printf("list payments: query: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	out := make([]map[string]any, 0, len(payments))
	for _, p := range payments {
		out = append(out, p.json())
	}
	writeJSON(w, http.StatusOK, out)
}

// loadPayments returns all payment rows for a bill.
func (s *Server) loadPayments(ctx context.Context, billID string) ([]paymentRow, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, bill_id, participant_id, amount_cents, currency, recipient,
		        status, provider, tx_ref, created_at, updated_at
		 FROM payments WHERE bill_id = ? ORDER BY created_at`, billID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var payments []paymentRow
	for rows.Next() {
		var p paymentRow
		if err := rows.Scan(&p.ID, &p.BillID, &p.ParticipantID, &p.AmountCents,
			&p.Currency, &p.Recipient, &p.Status, &p.Provider, &p.TxRef,
			&p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		payments = append(payments, p)
	}
	return payments, rows.Err()
}
