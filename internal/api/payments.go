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

// paymentRow mirrors a row of the payments table. The table also carries
// vestigial provider/tx_ref columns from the earlier USDC design; they are
// always written as 'venmo'/NULL and never read back, so they are absent here.
type paymentRow struct {
	ID            string
	BillID        string
	ParticipantID string
	AmountCents   int
	Currency      string
	Recipient     string // the host's Venmo handle
	Status        string // "pending" or "paid"
	CreatedAt     int64
	UpdatedAt     int64
}

func (p paymentRow) json() map[string]any {
	return map[string]any{
		"id":             p.ID,
		"participant_id": p.ParticipantID,
		"amount_cents":   p.AmountCents,
		"currency":       p.Currency,
		"status":         p.Status,
		"recipient":      p.Recipient,
	}
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

// participantInBill reports whether participant id belongs to billID.
func (s *Server) participantInBill(ctx context.Context, billID, id string) (bool, error) {
	var partBill string
	err := s.DB.QueryRowContext(ctx,
		`SELECT bill_id FROM participants WHERE id = ?`, id).Scan(&partBill)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return partBill == billID, nil
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
	parts, err := s.loadParticipants(ctx, b.ID)
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
	participantIDs := make([]string, 0, len(parts))
	for _, p := range parts {
		participantIDs = append(participantIDs, p.ID)
	}
	summary := split.Compute(split.Input{
		Items:    splitItems,
		TaxCents: b.TaxCents,
		TipCents: b.TipCents,
		Service: split.ServiceCharge{
			Kind:       b.ServiceChargeKind,
			RateBps:    b.ServiceChargeRateBps,
			FixedCents: b.ServiceChargeCents,
			Headcount:  b.ServiceChargeHeadcount,
		},
		Claims:         claims,
		ParticipantIDs: participantIDs,
	})
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
		        status, created_at, updated_at
		 FROM payments WHERE participant_id = ?`, participantID).
		Scan(&p.ID, &p.BillID, &p.ParticipantID, &p.AmountCents, &p.Currency,
			&p.Recipient, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

// hostVenmoHandle returns the host's Venmo handle for a bill, or "" if unset.
func (s *Server) hostVenmoHandle(ctx context.Context, hostUserID string) (string, error) {
	var handle sql.NullString
	if err := s.DB.QueryRowContext(ctx,
		`SELECT venmo_handle FROM users WHERE id = ?`, hostUserID).Scan(&handle); err != nil {
		return "", err
	}
	if !handle.Valid {
		return "", nil
	}
	return strings.TrimSpace(handle.String), nil
}

// paymentNote is the memo prefilled into the Venmo transfer, written from the
// paying friend's point of view.
func paymentNote(b bill) string {
	name := strings.TrimSpace(b.Restaurant)
	if name == "" {
		return "My share of the bill 🧾"
	}
	return "My share of " + name + " 🧾"
}

// insertPayment creates a payment row, supplying constant values for the
// vestigial provider/tx_ref columns.
func (s *Server) insertPayment(ctx context.Context, p paymentRow) error {
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO payments (id, bill_id, participant_id, amount_cents, currency,
		        recipient, status, provider, tx_ref, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 'venmo', NULL, ?, ?)`,
		p.ID, p.BillID, p.ParticipantID, p.AmountCents, p.Currency,
		p.Recipient, p.Status, p.CreatedAt, p.UpdatedAt)
	return err
}

// handlePay prepares a Venmo payment for a friend and responds with a payment
// intent: the host's handle, the amount owed, and ready-made app/web links.
// Venmo cannot report settlement back, so this never blocks — the friend
// settles in Venmo and the payment is marked paid separately (handlePayConfirm
// or handleMarkPayment). If the friend has already paid the paid intent is
// returned unchanged (idempotent).
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
		writeJSON(w, http.StatusOK, s.paymentIntent(existing, b))
		return
	}

	handle, err := s.hostVenmoHandle(r.Context(), b.hostUserID)
	if err != nil {
		log.Printf("pay: host handle: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if handle == "" {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "the host hasn't set their Venmo handle yet",
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
		p = paymentRow{
			ID:            uuid.NewString(),
			BillID:        b.ID,
			ParticipantID: participantID,
			AmountCents:   amount,
			Currency:      b.Currency,
			Recipient:     handle,
			Status:        "pending",
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := s.insertPayment(r.Context(), p); err != nil {
			log.Printf("pay: insert: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
	} else {
		// Reuse the pending row, refreshing the amount and handle in case
		// claims or the host's handle changed since it was created.
		p = existing
		p.AmountCents = amount
		p.Currency = b.Currency
		p.Recipient = handle
		p.UpdatedAt = now
		if _, err := s.DB.ExecContext(r.Context(),
			`UPDATE payments SET amount_cents = ?, currency = ?, recipient = ?, updated_at = ?
			 WHERE id = ?`,
			p.AmountCents, p.Currency, p.Recipient, p.UpdatedAt, p.ID); err != nil {
			log.Printf("pay: update: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
	}

	writeJSON(w, http.StatusOK, s.paymentIntent(p, b))
}

// paymentIntent renders the JSON a friend's client needs to hand off to Venmo.
func (s *Server) paymentIntent(p paymentRow, b bill) map[string]any {
	note := paymentNote(b)
	return map[string]any{
		"payment_id":   p.ID,
		"status":       p.Status,
		"amount_cents": p.AmountCents,
		"currency":     p.Currency,
		"venmo_handle": p.Recipient,
		"note":         note,
		"app_url":      payment.AppURL(p.Recipient, p.AmountCents, note),
		"web_url":      payment.WebURL(p.Recipient, p.AmountCents, note),
	}
}

// handlePayConfirm marks a friend's payment paid. Venmo provides no proof of
// settlement, so this records the friend's self-report; it is idempotent if
// the payment is already paid.
func (s *Server) handlePayConfirm(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req struct {
		ParticipantToken string `json:"participant_token"`
		PaymentID        string `json:"payment_id"`
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

	if p.Status != "paid" {
		now := time.Now().Unix()
		if _, err := s.DB.ExecContext(r.Context(),
			`UPDATE payments SET status = 'paid', updated_at = ? WHERE id = ?`,
			now, p.ID); err != nil {
			log.Printf("pay confirm: update: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		p.Status = "paid"
		p.UpdatedAt = now
	}
	writeJSON(w, http.StatusOK, p.json())
}

// handleMarkPayment lets the host confirm or undo a friend's payment from the
// bill editor. paid=true records the friend as paid (creating a payment row if
// none exists); paid=false removes the payment row, returning the friend to
// "not paid". It returns the refreshed bill summary.
func (s *Server) handleMarkPayment(w http.ResponseWriter, r *http.Request) {
	u := r.Context().Value(userCtxKey).(user)
	id := r.PathValue("id")
	pid := r.PathValue("pid")

	var req struct {
		Paid bool `json:"paid"`
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
		log.Printf("mark payment: load bill: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if b.hostUserID != u.ID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	inBill, err := s.participantInBill(r.Context(), b.ID, pid)
	if err != nil {
		log.Printf("mark payment: participant: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if !inBill {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "participant not found"})
		return
	}

	now := time.Now().Unix()
	if req.Paid {
		existing, err := s.loadPayment(r.Context(), pid)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			log.Printf("mark payment: load: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			amount, err := s.amountOwed(r.Context(), b, pid)
			if err != nil {
				log.Printf("mark payment: amount: %v", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
				return
			}
			handle, err := s.hostVenmoHandle(r.Context(), b.hostUserID)
			if err != nil {
				log.Printf("mark payment: handle: %v", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
				return
			}
			if err := s.insertPayment(r.Context(), paymentRow{
				ID:            uuid.NewString(),
				BillID:        b.ID,
				ParticipantID: pid,
				AmountCents:   amount,
				Currency:      b.Currency,
				Recipient:     handle,
				Status:        "paid",
				CreatedAt:     now,
				UpdatedAt:     now,
			}); err != nil {
				log.Printf("mark payment: insert: %v", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
				return
			}
		} else if existing.Status != "paid" {
			if _, err := s.DB.ExecContext(r.Context(),
				`UPDATE payments SET status = 'paid', updated_at = ? WHERE id = ?`,
				now, existing.ID); err != nil {
				log.Printf("mark payment: update: %v", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
				return
			}
		}
	} else {
		if _, err := s.DB.ExecContext(r.Context(),
			`DELETE FROM payments WHERE participant_id = ?`, pid); err != nil {
			log.Printf("mark payment: delete: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
	}

	resp, err := s.buildSummary(r.Context(), b)
	if err != nil {
		log.Printf("mark payment: summary: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, resp)
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
		        status, created_at, updated_at
		 FROM payments WHERE bill_id = ? ORDER BY created_at`, billID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var payments []paymentRow
	for rows.Next() {
		var p paymentRow
		if err := rows.Scan(&p.ID, &p.BillID, &p.ParticipantID, &p.AmountCents,
			&p.Currency, &p.Recipient, &p.Status, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		payments = append(payments, p)
	}
	return payments, rows.Err()
}
