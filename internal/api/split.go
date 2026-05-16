package api

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jjtny1/splitit/internal/auth"
	"github.com/jjtny1/splitit/internal/split"
)

type participant struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	HostManaged bool   `json:"host_managed"`
	IsHost      bool   `json:"is_host"`

	token string
}

// handleBillByToken resolves a friend share token to its bill (friend view).
func (s *Server) handleBillByToken(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")

	var id string
	err := s.DB.QueryRowContext(r.Context(),
		`SELECT id FROM bills WHERE friend_token = ?`, token).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "bill not found"})
		return
	}
	if err != nil {
		log.Printf("bill by token: lookup: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	b, err := s.loadBill(r.Context(), id)
	if err != nil {
		log.Printf("bill by token: load: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	items, err := s.loadItems(r.Context(), b.ID)
	if err != nil {
		log.Printf("bill by token: items: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	b.Items = items
	writeJSON(w, http.StatusOK, s.billJSON(b, false))
}

// handleJoinBill creates a participant for a friend opening the share link.
func (s *Server) handleJoinBill(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req struct {
		DisplayName string `json:"display_name"`
		T           string `json:"t"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	name := strings.TrimSpace(req.DisplayName)
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name required"})
		return
	}

	b, err := s.loadBill(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "bill not found"})
		return
	}
	if err != nil {
		log.Printf("join bill: load: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if req.T != b.friendToken {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "bill not found"})
		return
	}
	if b.SplitMode == "host" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "this bill was split by the host"})
		return
	}

	token, err := auth.NewToken()
	if err != nil {
		log.Printf("join bill: token: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	p := participant{ID: uuid.NewString(), DisplayName: name}
	if _, err := s.DB.ExecContext(r.Context(),
		`INSERT INTO participants (id, bill_id, display_name, participant_token, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		p.ID, b.ID, p.DisplayName, token, time.Now().Unix()); err != nil {
		log.Printf("join bill: insert: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"participant":       p,
		"participant_token": token,
	})
}

// handleSetClaims replaces a participant's claimed items.
func (s *Server) handleSetClaims(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req struct {
		ParticipantToken string   `json:"participant_token"`
		ItemIDs          []string `json:"item_ids"`
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
		log.Printf("set claims: load: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	var participantID, participantBill string
	err = s.DB.QueryRowContext(r.Context(),
		`SELECT id, bill_id FROM participants WHERE participant_token = ?`, req.ParticipantToken).
		Scan(&participantID, &participantBill)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && participantBill != b.ID) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "participant not found"})
		return
	}
	if err != nil {
		log.Printf("set claims: participant: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	items, err := s.loadItems(r.Context(), b.ID)
	if err != nil {
		log.Printf("set claims: items: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	valid := map[string]bool{}
	for _, it := range items {
		valid[it.ID] = true
	}
	wanted := map[string]bool{}
	for _, itemID := range req.ItemIDs {
		if valid[itemID] {
			wanted[itemID] = true
		}
	}

	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		log.Printf("set claims: tx: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(r.Context(),
		`DELETE FROM claims WHERE participant_id = ?`, participantID); err != nil {
		log.Printf("set claims: delete: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	for itemID := range wanted {
		if _, err := tx.ExecContext(r.Context(),
			`INSERT INTO claims (item_id, participant_id) VALUES (?, ?)`,
			itemID, participantID); err != nil {
			log.Printf("set claims: insert: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
	}
	if err := tx.Commit(); err != nil {
		log.Printf("set claims: commit: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	resp, err := s.buildSummary(r.Context(), b)
	if err != nil {
		log.Printf("set claims: summary: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleSummary returns the full split summary for host or friend.
func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	b, err := s.loadBill(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "bill not found"})
		return
	}
	if err != nil {
		log.Printf("summary: load: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	u, authed := s.currentUser(r)
	host := authed && u.ID == b.hostUserID
	if !host && r.URL.Query().Get("t") != b.friendToken {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "bill not found"})
		return
	}

	resp, err := s.buildSummary(r.Context(), b)
	if err != nil {
		log.Printf("summary: build: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// buildSummary loads items, participants and claims for a bill and computes
// the split, returning the friend-view JSON payload.
func (s *Server) buildSummary(ctx context.Context, b bill) (map[string]any, error) {
	items, err := s.loadItems(ctx, b.ID)
	if err != nil {
		return nil, err
	}
	b.Items = items

	parts, err := s.loadParticipants(ctx, b.ID)
	if err != nil {
		return nil, err
	}

	claims, err := s.loadClaims(ctx, b.ID)
	if err != nil {
		return nil, err
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

	payments, err := s.loadPayments(ctx, b.ID)
	if err != nil {
		return nil, err
	}
	byParticipant := map[string]paymentRow{}
	for _, p := range payments {
		byParticipant[p.ParticipantID] = p
	}
	partsOut := make([]map[string]any, 0, len(parts))
	for _, p := range parts {
		entry := map[string]any{
			"id":             p.ID,
			"display_name":   p.DisplayName,
			"host_managed":   p.HostManaged,
			"is_host":        p.IsHost,
			"payment_status": "none",
		}
		if pay, ok := byParticipant[p.ID]; ok {
			entry["payment_status"] = pay.Status
		}
		// The per-participant token lets a host-split bill identify a
		// participant for payment; it is exposed only on a host-split bill.
		if b.SplitMode == "host" {
			entry["participant_token"] = p.token
		}
		partsOut = append(partsOut, entry)
	}

	return map[string]any{
		"bill":         s.billJSON(b, false),
		"items":        items,
		"participants": partsOut,
		"claims":       claims,
		"split":        summary,
	}, nil
}

// loadParticipants returns a bill's participants ordered by creation time.
func (s *Server) loadParticipants(ctx context.Context, billID string) ([]participant, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, display_name, host_managed, is_host, participant_token FROM participants
		 WHERE bill_id = ? ORDER BY created_at, id`, billID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	parts := []participant{}
	for rows.Next() {
		var p participant
		if err := rows.Scan(&p.ID, &p.DisplayName, &p.HostManaged, &p.IsHost, &p.token); err != nil {
			return nil, err
		}
		parts = append(parts, p)
	}
	return parts, rows.Err()
}

// loadClaims returns a map of itemID to the sorted participant IDs claiming it.
func (s *Server) loadClaims(ctx context.Context, billID string) (map[string][]string, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT c.item_id, c.participant_id FROM claims c
		 JOIN items i ON i.id = c.item_id
		 WHERE i.bill_id = ?`, billID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	claims := map[string][]string{}
	for rows.Next() {
		var itemID, participantID string
		if err := rows.Scan(&itemID, &participantID); err != nil {
			return nil, err
		}
		claims[itemID] = append(claims[itemID], participantID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for k := range claims {
		sort.Strings(claims[k])
	}
	return claims, nil
}
