package api

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jjtny1/splitit/internal/auth"
)

const maxReceiptBytes = 10 << 20

type billItem struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	PriceCents int    `json:"price_cents"`
	Qty        int    `json:"qty"`
	Position   int    `json:"position"`
}

type bill struct {
	ID         string     `json:"id"`
	Restaurant string     `json:"restaurant"`
	Currency   string     `json:"currency"`
	TaxCents   int        `json:"tax_cents"`
	TipCents   int        `json:"tip_cents"`
	Status     string     `json:"status"`
	Items      []billItem `json:"items"`
	CreatedAt  int64      `json:"created_at"`

	hostUserID  string
	friendToken string
}

// billJSON renders a bill, including host-only fields only when host is true.
func (s *Server) billJSON(b bill, host bool) map[string]any {
	out := map[string]any{
		"id":         b.ID,
		"restaurant": b.Restaurant,
		"currency":   b.Currency,
		"tax_cents":  b.TaxCents,
		"tip_cents":  b.TipCents,
		"status":     b.Status,
		"items":      b.Items,
		"created_at": b.CreatedAt,
	}
	if host {
		out["friend_token"] = b.friendToken
		out["share_url"] = s.Cfg.BaseURL + "/b/" + b.friendToken
	}
	return out
}

func (s *Server) handleCreateBill(w http.ResponseWriter, r *http.Request) {
	u := r.Context().Value(userCtxKey).(user)

	token, err := auth.NewToken()
	if err != nil {
		log.Printf("create bill: token: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	b := bill{
		ID:          uuid.NewString(),
		Currency:    "USD",
		Status:      "draft",
		CreatedAt:   time.Now().Unix(),
		Items:       []billItem{},
		hostUserID:  u.ID,
		friendToken: token,
	}
	if _, err := s.DB.ExecContext(r.Context(),
		`INSERT INTO bills (id, host_user_id, restaurant, currency, tax_cents, tip_cents, status, friend_token, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		b.ID, b.hostUserID, "", b.Currency, b.TaxCents, b.TipCents, b.Status, b.friendToken, b.CreatedAt); err != nil {
		log.Printf("create bill: insert: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusCreated, s.billJSON(b, true))
}

func (s *Server) handleListBills(w http.ResponseWriter, r *http.Request) {
	u := r.Context().Value(userCtxKey).(user)

	rows, err := s.DB.QueryContext(r.Context(),
		`SELECT id, restaurant, currency, tax_cents, tip_cents, status, friend_token, created_at
		 FROM bills WHERE host_user_id = ? ORDER BY created_at DESC`, u.ID)
	if err != nil {
		log.Printf("list bills: query: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	defer rows.Close()

	var bills []bill
	for rows.Next() {
		var b bill
		if err := rows.Scan(&b.ID, &b.Restaurant, &b.Currency, &b.TaxCents, &b.TipCents,
			&b.Status, &b.friendToken, &b.CreatedAt); err != nil {
			log.Printf("list bills: scan: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		b.hostUserID = u.ID
		bills = append(bills, b)
	}
	if err := rows.Err(); err != nil {
		log.Printf("list bills: rows: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	out := make([]map[string]any, 0, len(bills))
	for i := range bills {
		items, err := s.loadItems(r.Context(), bills[i].ID)
		if err != nil {
			log.Printf("list bills: items: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		bills[i].Items = items
		out = append(out, s.billJSON(bills[i], true))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetBill(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	b, err := s.loadBill(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "bill not found"})
		return
	}
	if err != nil {
		log.Printf("get bill: load: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	u, authed := s.currentUser(r)
	host := authed && u.ID == b.hostUserID
	if !host {
		if r.URL.Query().Get("t") != b.friendToken {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "bill not found"})
			return
		}
	}

	items, err := s.loadItems(r.Context(), b.ID)
	if err != nil {
		log.Printf("get bill: items: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	b.Items = items
	writeJSON(w, http.StatusOK, s.billJSON(b, host))
}

func (s *Server) handleBillReceipt(w http.ResponseWriter, r *http.Request) {
	u := r.Context().Value(userCtxKey).(user)
	id := r.PathValue("id")

	b, err := s.loadBill(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "bill not found"})
		return
	}
	if err != nil {
		log.Printf("bill receipt: load: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if b.hostUserID != u.ID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxReceiptBytes)
	file, header, err := r.FormFile("receipt")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "receipt image required"})
		return
	}
	defer file.Close()

	image, err := io.ReadAll(file)
	if err != nil {
		log.Printf("bill receipt: read: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "could not read upload"})
		return
	}

	mediaType := header.Header.Get("Content-Type")
	if mediaType == "" {
		mediaType = http.DetectContentType(image)
	}

	parsed, err := s.Parser.Parse(r.Context(), image, mediaType)
	if err != nil {
		log.Printf("bill receipt: parse: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not parse receipt"})
		return
	}

	items := make([]billItem, 0, len(parsed.Items))
	for i, it := range parsed.Items {
		qty := it.Qty
		if qty < 1 {
			qty = 1
		}
		price := it.PriceCents
		if price < 0 {
			price = 0
		}
		items = append(items, billItem{Name: it.Name, PriceCents: price, Qty: qty, Position: i})
	}

	b.Restaurant = parsed.Restaurant
	b.TaxCents = parsed.TaxCents
	if b.TaxCents < 0 {
		b.TaxCents = 0
	}
	b.TipCents = parsed.TipCents
	if b.TipCents < 0 {
		b.TipCents = 0
	}

	if err := s.saveBillAndItems(r.Context(), b, items); err != nil {
		log.Printf("bill receipt: save: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	b.Items = items
	writeJSON(w, http.StatusOK, s.billJSON(b, true))
}

func (s *Server) handleUpdateBill(w http.ResponseWriter, r *http.Request) {
	u := r.Context().Value(userCtxKey).(user)
	id := r.PathValue("id")

	b, err := s.loadBill(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "bill not found"})
		return
	}
	if err != nil {
		log.Printf("update bill: load: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if b.hostUserID != u.ID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	var req struct {
		Restaurant string `json:"restaurant"`
		TaxCents   int    `json:"tax_cents"`
		TipCents   int    `json:"tip_cents"`
		Status     string `json:"status"`
		Items      []struct {
			Name       string `json:"name"`
			PriceCents int    `json:"price_cents"`
			Qty        int    `json:"qty"`
		} `json:"items"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.TaxCents < 0 || req.TipCents < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tax and tip must be non-negative"})
		return
	}
	if req.Status != "" && req.Status != "draft" && req.Status != "open" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid status"})
		return
	}

	items := make([]billItem, 0, len(req.Items))
	for i, it := range req.Items {
		if it.PriceCents < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "item prices must be non-negative"})
			return
		}
		if it.Qty < 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "item quantity must be at least 1"})
			return
		}
		items = append(items, billItem{Name: it.Name, PriceCents: it.PriceCents, Qty: it.Qty, Position: i})
	}

	b.Restaurant = req.Restaurant
	b.TaxCents = req.TaxCents
	b.TipCents = req.TipCents
	if req.Status != "" {
		b.Status = req.Status
	}

	if err := s.saveBillAndItems(r.Context(), b, items); err != nil {
		log.Printf("update bill: save: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	b.Items = items
	writeJSON(w, http.StatusOK, s.billJSON(b, true))
}

// loadBill fetches a bill row by id without its items.
func (s *Server) loadBill(ctx context.Context, id string) (bill, error) {
	var b bill
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, host_user_id, restaurant, currency, tax_cents, tip_cents, status, friend_token, created_at
		 FROM bills WHERE id = ?`, id).
		Scan(&b.ID, &b.hostUserID, &b.Restaurant, &b.Currency, &b.TaxCents, &b.TipCents,
			&b.Status, &b.friendToken, &b.CreatedAt)
	return b, err
}

// loadItems returns a bill's items ordered by position.
func (s *Server) loadItems(ctx context.Context, billID string) ([]billItem, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, name, price_cents, qty, position FROM items
		 WHERE bill_id = ? ORDER BY position`, billID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []billItem{}
	for rows.Next() {
		var it billItem
		if err := rows.Scan(&it.ID, &it.Name, &it.PriceCents, &it.Qty, &it.Position); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// saveBillAndItems updates the bill fields and replaces all of its items
// within a single transaction.
func (s *Server) saveBillAndItems(ctx context.Context, b bill, items []billItem) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`UPDATE bills SET restaurant = ?, tax_cents = ?, tip_cents = ?, status = ? WHERE id = ?`,
		b.Restaurant, b.TaxCents, b.TipCents, b.Status, b.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM items WHERE bill_id = ?`, b.ID); err != nil {
		return err
	}
	for i := range items {
		items[i].ID = uuid.NewString()
		items[i].Position = i
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO items (id, bill_id, name, price_cents, qty, position) VALUES (?, ?, ?, ?, ?, ?)`,
			items[i].ID, b.ID, items[i].Name, items[i].PriceCents, items[i].Qty, items[i].Position); err != nil {
			return err
		}
	}
	return tx.Commit()
}
