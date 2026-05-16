package api

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jjtny1/iouapp/internal/auth"
	"github.com/jjtny1/iouapp/internal/money"
	"github.com/jjtny1/iouapp/internal/receipt"
)

const maxReceiptBytes = 10 << 20

// supportedReceiptTypes are the image media types the receipt parser (the
// Anthropic vision API) can read. HEIC is intentionally absent: iPhone HEIC
// photos are converted to JPEG client-side before upload.
var supportedReceiptTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
}

type billItem struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	PriceCents int    `json:"price_cents"`
	Position   int    `json:"position"`
}

type bill struct {
	ID         string     `json:"id"`
	Restaurant string     `json:"restaurant"`
	Currency   string     `json:"currency"`
	TaxCents   int        `json:"tax_cents"`
	TipCents   int        `json:"tip_cents"`
	Status     string     `json:"status"`
	SplitMode  string     `json:"split_mode"`
	Items      []billItem `json:"items"`
	CreatedAt  int64      `json:"created_at"`

	// Service charge: ServiceChargeKind is "none", "percent", or "fixed".
	// RateBps (basis points) applies to a percent charge; Cents is the flat
	// amount of a fixed charge; Headcount is the fixed charge's diner count
	// (0 means split across the joined participants).
	ServiceChargeKind      string `json:"service_charge_kind"`
	ServiceChargeRateBps   int    `json:"service_charge_rate_bps"`
	ServiceChargeCents     int    `json:"service_charge_cents"`
	ServiceChargeHeadcount int    `json:"service_charge_headcount"`

	hostUserID  string
	friendToken string
}

// billJSON renders a bill, including host-only fields only when host is true.
func (s *Server) billJSON(b bill, host bool) map[string]any {
	out := map[string]any{
		"id":                       b.ID,
		"restaurant":               b.Restaurant,
		"currency":                 b.Currency,
		"tax_cents":                b.TaxCents,
		"tip_cents":                b.TipCents,
		"service_charge_kind":      b.ServiceChargeKind,
		"service_charge_rate_bps":  b.ServiceChargeRateBps,
		"service_charge_cents":     b.ServiceChargeCents,
		"service_charge_headcount": b.ServiceChargeHeadcount,
		"status":                   b.Status,
		"split_mode":               b.SplitMode,
		"items":                    b.Items,
		"created_at":               b.CreatedAt,
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
		ID:                uuid.NewString(),
		Currency:          "USD",
		Status:            "draft",
		CreatedAt:         time.Now().Unix(),
		Items:             []billItem{},
		ServiceChargeKind: "none",
		hostUserID:        u.ID,
		friendToken:       token,
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
		`SELECT id, restaurant, currency, tax_cents, tip_cents,
		        service_charge_kind, service_charge_rate_bps, service_charge_cents, service_charge_headcount,
		        status, split_mode, friend_token, created_at
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
			&b.ServiceChargeKind, &b.ServiceChargeRateBps, &b.ServiceChargeCents, &b.ServiceChargeHeadcount,
			&b.Status, &b.SplitMode, &b.friendToken, &b.CreatedAt); err != nil {
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
	if mediaType == "" || mediaType == "application/octet-stream" {
		mediaType = http.DetectContentType(image)
	}
	base, _, _ := strings.Cut(mediaType, ";")
	if !supportedReceiptTypes[strings.TrimSpace(base)] {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{
			"error": "unsupported image type; upload a JPEG, PNG, GIF, or WebP",
		})
		return
	}

	parsed, err := s.Parser.Parse(r.Context(), image, mediaType)
	if err != nil {
		log.Printf("bill receipt: parse: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not parse receipt"})
		return
	}

	// Expand multi-quantity lines into one item per unit so each unit can be
	// claimed independently in the friend split.
	flat := receipt.Flatten(parsed.Items)
	items := make([]billItem, 0, len(flat))
	for i, it := range flat {
		items = append(items, billItem{Name: it.Name, PriceCents: it.PriceCents, Position: i})
	}

	b.Restaurant = parsed.Restaurant
	b.Currency = money.CurrencyOrDefault(parsed.Currency)
	b.TaxCents = parsed.TaxCents
	if b.TaxCents < 0 {
		b.TaxCents = 0
	}
	b.TipCents = parsed.TipCents
	if b.TipCents < 0 {
		b.TipCents = 0
	}
	applyParsedServiceCharge(&b, parsed.ServiceCharge)

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
		Restaurant             string `json:"restaurant"`
		Currency               string `json:"currency"`
		TaxCents               int    `json:"tax_cents"`
		TipCents               int    `json:"tip_cents"`
		ServiceChargeKind      string `json:"service_charge_kind"`
		ServiceChargeRateBps   int    `json:"service_charge_rate_bps"`
		ServiceChargeCents     int    `json:"service_charge_cents"`
		ServiceChargeHeadcount int    `json:"service_charge_headcount"`
		Status                 string `json:"status"`
		Items                  []struct {
			// ID is the existing item's id, sent back by the editor so an
			// edit updates that row in place — empty for a newly added item.
			ID         string `json:"id"`
			Name       string `json:"name"`
			PriceCents int    `json:"price_cents"`
		} `json:"items"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.TaxCents < 0 || req.TipCents < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tax and tip must be non-negative"})
		return
	}
	scKind, scRate, scCents, scHead, ok := normalizeServiceCharge(
		req.ServiceChargeKind, req.ServiceChargeRateBps, req.ServiceChargeCents, req.ServiceChargeHeadcount)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "service charge must be none, a non-negative percent, or a non-negative fixed amount",
		})
		return
	}
	if req.Status != "" && req.Status != "draft" && req.Status != "open" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid status"})
		return
	}
	// Currency is optional in the request; when present it must be a valid
	// ISO 4217 code, otherwise the bill keeps its existing currency.
	if req.Currency != "" {
		c, ok := money.NormalizeCurrency(req.Currency)
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "currency must be a 3-letter ISO 4217 code",
			})
			return
		}
		b.Currency = c
	}

	items := make([]billItem, 0, len(req.Items))
	for i, it := range req.Items {
		if it.PriceCents < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "item prices must be non-negative"})
			return
		}
		items = append(items, billItem{ID: it.ID, Name: it.Name, PriceCents: it.PriceCents, Position: i})
	}

	b.Restaurant = req.Restaurant
	b.TaxCents = req.TaxCents
	b.TipCents = req.TipCents
	b.ServiceChargeKind = scKind
	b.ServiceChargeRateBps = scRate
	b.ServiceChargeCents = scCents
	b.ServiceChargeHeadcount = scHead
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

func (s *Server) handleDeleteBill(w http.ResponseWriter, r *http.Request) {
	u := r.Context().Value(userCtxKey).(user)
	id := r.PathValue("id")

	b, err := s.loadBill(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "bill not found"})
		return
	}
	if err != nil {
		log.Printf("delete bill: load: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if b.hostUserID != u.ID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	if err := s.deleteBill(r.Context(), id); err != nil {
		log.Printf("delete bill: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteBill removes a bill and every row that references it — payments,
// claims, participants and items — in a single transaction. The deletes run
// child-table-first so a foreign key is never left dangling mid-transaction.
func (s *Server) deleteBill(ctx context.Context, id string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmts := []string{
		`DELETE FROM payments WHERE bill_id = ?`,
		`DELETE FROM claims WHERE participant_id IN (SELECT id FROM participants WHERE bill_id = ?)`,
		`DELETE FROM participants WHERE bill_id = ?`,
		`DELETE FROM items WHERE bill_id = ?`,
		`DELETE FROM bills WHERE id = ?`,
	}
	for _, q := range stmts {
		if _, err := tx.ExecContext(ctx, q, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// loadBill fetches a bill row by id without its items.
func (s *Server) loadBill(ctx context.Context, id string) (bill, error) {
	var b bill
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, host_user_id, restaurant, currency, tax_cents, tip_cents,
		        service_charge_kind, service_charge_rate_bps, service_charge_cents, service_charge_headcount,
		        status, split_mode, friend_token, created_at
		 FROM bills WHERE id = ?`, id).
		Scan(&b.ID, &b.hostUserID, &b.Restaurant, &b.Currency, &b.TaxCents, &b.TipCents,
			&b.ServiceChargeKind, &b.ServiceChargeRateBps, &b.ServiceChargeCents, &b.ServiceChargeHeadcount,
			&b.Status, &b.SplitMode, &b.friendToken, &b.CreatedAt)
	return b, err
}

// loadItems returns a bill's items ordered by position.
func (s *Server) loadItems(ctx context.Context, billID string) ([]billItem, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, name, price_cents, position FROM items
		 WHERE bill_id = ? ORDER BY position`, billID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []billItem{}
	for rows.Next() {
		var it billItem
		if err := rows.Scan(&it.ID, &it.Name, &it.PriceCents, &it.Position); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// saveBillAndItems updates the bill fields and reconciles its items within a
// single transaction. Items are matched by id and updated in place rather than
// deleted and recreated, so any claims referencing a kept item survive the
// edit — a blanket delete would violate the claims.item_id foreign key. An
// incoming item with no (or unknown) id is inserted as new; an existing item
// the caller dropped is deleted along with any claims that referenced it.
func (s *Server) saveBillAndItems(ctx context.Context, b bill, items []billItem) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`UPDATE bills SET restaurant = ?, currency = ?, tax_cents = ?, tip_cents = ?,
		        service_charge_kind = ?, service_charge_rate_bps = ?, service_charge_cents = ?,
		        service_charge_headcount = ?, status = ? WHERE id = ?`,
		b.Restaurant, b.Currency, b.TaxCents, b.TipCents,
		b.ServiceChargeKind, b.ServiceChargeRateBps, b.ServiceChargeCents, b.ServiceChargeHeadcount,
		b.Status, b.ID); err != nil {
		return err
	}

	// existing holds the ids of the bill's current items, so an incoming id
	// can be told apart from a stale or forged one.
	existing := map[string]bool{}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM items WHERE bill_id = ?`, b.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		existing[id] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	// Upsert each incoming item, recording which existing rows are kept.
	kept := map[string]bool{}
	for i := range items {
		items[i].Position = i
		if items[i].ID != "" && existing[items[i].ID] {
			if _, err := tx.ExecContext(ctx,
				`UPDATE items SET name = ?, price_cents = ?, position = ? WHERE id = ? AND bill_id = ?`,
				items[i].Name, items[i].PriceCents, items[i].Position, items[i].ID, b.ID); err != nil {
				return err
			}
			kept[items[i].ID] = true
			continue
		}
		items[i].ID = uuid.NewString()
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO items (id, bill_id, name, price_cents, position) VALUES (?, ?, ?, ?, ?)`,
			items[i].ID, b.ID, items[i].Name, items[i].PriceCents, items[i].Position); err != nil {
			return err
		}
	}

	// Delete the items the caller dropped, clearing any claims on them first
	// so the claims.item_id foreign key is never left dangling.
	for id := range existing {
		if kept[id] {
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM claims WHERE item_id = ?`, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM items WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// maxServiceChargeBps caps a percent service charge at 1000% to reject
// obviously malformed input while leaving any realistic rate valid.
const maxServiceChargeBps = 100000

// normalizeServiceCharge validates host-supplied service charge fields and
// returns the cleaned values, zeroing fields irrelevant to the chosen kind.
// ok is false when the input is malformed. An empty kind normalizes to "none".
func normalizeServiceCharge(kind string, rateBps, cents, headcount int) (outKind string, outRate, outCents, outHead int, ok bool) {
	switch kind {
	case "", "none":
		return "none", 0, 0, 0, true
	case "percent":
		if rateBps < 0 || rateBps > maxServiceChargeBps {
			return "", 0, 0, 0, false
		}
		return "percent", rateBps, 0, 0, true
	case "fixed":
		if cents < 0 || headcount < 0 {
			return "", 0, 0, 0, false
		}
		return "fixed", 0, cents, headcount, true
	default:
		return "", 0, 0, 0, false
	}
}

// applyParsedServiceCharge copies a service charge read from a receipt onto a
// bill, converting a percent rate to basis points.
func applyParsedServiceCharge(b *bill, sc receipt.ParsedServiceCharge) {
	switch sc.Kind {
	case "percent":
		b.ServiceChargeKind = "percent"
		b.ServiceChargeRateBps = int(sc.Percent*100 + 0.5)
		b.ServiceChargeCents = 0
	case "fixed":
		b.ServiceChargeKind = "fixed"
		b.ServiceChargeRateBps = 0
		b.ServiceChargeCents = sc.AmountCents
		if b.ServiceChargeCents < 0 {
			b.ServiceChargeCents = 0
		}
	default:
		b.ServiceChargeKind = "none"
		b.ServiceChargeRateBps = 0
		b.ServiceChargeCents = 0
	}
	b.ServiceChargeHeadcount = 0
}
