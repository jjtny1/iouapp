package api

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/jjtny1/splitit/internal/auth"
	"github.com/jjtny1/splitit/internal/config"
	"github.com/jjtny1/splitit/internal/db"
)

// testEnv is a fully wired IOU server backed by a fresh temp SQLite DB.
type testEnv struct {
	t      *testing.T
	server *httptest.Server
	url    string
}

// newTestEnv builds an isolated app: a temp-file SQLite DB, DevMode config
// with no Anthropic key (so the StubParser is used), and an httptest server.
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	cfg := config.Config{
		Port:            "0",
		DBPath:          dbPath,
		BaseURL:         "http://iou.test",
		AnthropicKey:    "",
		DevMode:         true,
		PaymentProvider: "mock",
	}
	srv := httptest.NewServer(NewRouter(database, cfg, auth.LogSender{}))
	t.Cleanup(srv.Close)
	return &testEnv{t: t, server: srv, url: srv.URL}
}

// newClient returns an HTTP client with its own cookie jar, so each client
// represents a distinct browser/session.
func (e *testEnv) newClient() *http.Client {
	e.t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		e.t.Fatalf("cookie jar: %v", err)
	}
	return &http.Client{Jar: jar}
}

// signIn performs the DevMode magic-link flow and returns an authenticated
// client whose jar holds the session cookie.
func (e *testEnv) signIn(email string) *http.Client {
	e.t.Helper()
	client := e.newClient()

	var reqResp map[string]string
	e.doJSON(client, http.MethodPost, "/api/auth/request",
		map[string]string{"email": email}, http.StatusOK, &reqResp)

	link, ok := reqResp["link"]
	if !ok || link == "" {
		e.t.Fatalf("dev mode response missing magic link: %v", reqResp)
	}
	u, err := url.Parse(link)
	if err != nil {
		e.t.Fatalf("parse magic link %q: %v", link, err)
	}
	token := u.Query().Get("token")
	if token == "" {
		e.t.Fatalf("magic link missing token: %q", link)
	}

	e.doJSON(client, http.MethodPost, "/api/auth/verify",
		map[string]string{"token": token}, http.StatusOK, nil)
	return client
}

// do issues a request and returns the response and its (already read) body.
func (e *testEnv) do(client *http.Client, method, path string, body io.Reader, contentType string) (*http.Response, []byte) {
	e.t.Helper()
	req, err := http.NewRequest(method, e.url+path, body)
	if err != nil {
		e.t.Fatalf("build request %s %s: %v", method, path, err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := client.Do(req)
	if err != nil {
		e.t.Fatalf("do %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		e.t.Fatalf("read body %s %s: %v", method, path, err)
	}
	return resp, raw
}

// doJSON sends a JSON request body, asserts the status, and decodes the
// response into out (if non-nil).
func (e *testEnv) doJSON(client *http.Client, method, path string, in any, wantStatus int, out any) []byte {
	e.t.Helper()
	var body io.Reader
	if in != nil {
		buf, err := json.Marshal(in)
		if err != nil {
			e.t.Fatalf("marshal request: %v", err)
		}
		body = bytes.NewReader(buf)
	}
	resp, raw := e.do(client, method, path, body, "application/json")
	if resp.StatusCode != wantStatus {
		e.t.Fatalf("%s %s: status = %d, want %d; body=%s",
			method, path, resp.StatusCode, wantStatus, raw)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			e.t.Fatalf("decode %s %s response: %v; body=%s", method, path, err, raw)
		}
	}
	return raw
}

// uploadReceipt posts a dummy multipart receipt file to a bill.
func (e *testEnv) uploadReceipt(client *http.Client, billID string, wantStatus int, out any) []byte {
	e.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("receipt", "receipt.jpg")
	if err != nil {
		e.t.Fatalf("create form file: %v", err)
	}
	// A JPEG SOI marker so the server's media-type check accepts the upload;
	// the stub parser ignores the bytes themselves.
	if _, err := fw.Write([]byte("\xff\xd8\xff\xe0dummy-image-bytes-the-stub-ignores")); err != nil {
		e.t.Fatalf("write form file: %v", err)
	}
	if err := mw.Close(); err != nil {
		e.t.Fatalf("close multipart writer: %v", err)
	}
	resp, raw := e.do(client, http.MethodPost,
		"/api/bills/"+billID+"/receipt", &buf, mw.FormDataContentType())
	if resp.StatusCode != wantStatus {
		e.t.Fatalf("upload receipt: status = %d, want %d; body=%s",
			resp.StatusCode, wantStatus, raw)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			e.t.Fatalf("decode receipt response: %v; body=%s", err, raw)
		}
	}
	return raw
}

// createBill creates a bill as the given host client and returns the parsed
// bill JSON (including host-only friend_token).
func (e *testEnv) createBill(host *http.Client) map[string]any {
	e.t.Helper()
	var b map[string]any
	e.doJSON(host, http.MethodPost, "/api/bills", map[string]any{},
		http.StatusCreated, &b)
	return b
}

func TestHealth(t *testing.T) {
	e := newTestEnv(t)
	var resp map[string]string
	e.doJSON(e.newClient(), http.MethodGet, "/api/health", nil, http.StatusOK, &resp)
	if resp["status"] != "ok" {
		t.Errorf("health status = %q, want %q", resp["status"], "ok")
	}
}

func TestHappyPathHostFlow(t *testing.T) {
	e := newTestEnv(t)
	host := e.signIn("host@example.com")

	bill := e.createBill(host)
	billID, _ := bill["id"].(string)
	if billID == "" {
		t.Fatal("created bill missing id")
	}
	if _, ok := bill["friend_token"]; !ok {
		t.Error("host view of created bill should include friend_token")
	}

	// Upload a receipt: StubParser fills in restaurant, items, tax, tip.
	var afterUpload map[string]any
	e.uploadReceipt(host, billID, http.StatusOK, &afterUpload)
	if afterUpload["restaurant"] != "Sample Diner" {
		t.Errorf("restaurant = %v, want Sample Diner", afterUpload["restaurant"])
	}
	items, _ := afterUpload["items"].([]any)
	if len(items) == 0 {
		t.Error("expected items after receipt upload")
	}
	if tax, _ := afterUpload["tax_cents"].(float64); tax <= 0 {
		t.Errorf("tax_cents = %v, want > 0", afterUpload["tax_cents"])
	}
	if tip, _ := afterUpload["tip_cents"].(float64); tip <= 0 {
		t.Errorf("tip_cents = %v, want > 0", afterUpload["tip_cents"])
	}
	if afterUpload["currency"] != "USD" {
		t.Errorf("currency = %v, want USD", afterUpload["currency"])
	}
	// The StubParser receipt carries a 10% percent service charge.
	if afterUpload["service_charge_kind"] != "percent" {
		t.Errorf("service_charge_kind = %v, want percent", afterUpload["service_charge_kind"])
	}
	if bps, _ := afterUpload["service_charge_rate_bps"].(float64); bps != 1000 {
		t.Errorf("service_charge_rate_bps = %v, want 1000", afterUpload["service_charge_rate_bps"])
	}

	// GET the bill as host: full detail with host-only fields.
	var detail map[string]any
	e.doJSON(host, http.MethodGet, "/api/bills/"+billID, nil, http.StatusOK, &detail)
	if _, ok := detail["friend_token"]; !ok {
		t.Error("host GET should include friend_token")
	}
	if _, ok := detail["share_url"]; !ok {
		t.Error("host GET should include share_url")
	}
	gotItems, _ := detail["items"].([]any)
	if len(gotItems) != len(items) {
		t.Errorf("host GET item count = %d, want %d", len(gotItems), len(items))
	}
}

func TestFriendFlowAndSummaryInvariant(t *testing.T) {
	e := newTestEnv(t)
	host := e.signIn("host@example.com")

	bill := e.createBill(host)
	billID := bill["id"].(string)
	friendToken := bill["friend_token"].(string)

	e.uploadReceipt(host, billID, http.StatusOK, nil)

	// Friend view by token must omit host-only fields.
	var friendBill map[string]any
	e.doJSON(e.newClient(), http.MethodGet, "/api/by-token/"+friendToken,
		nil, http.StatusOK, &friendBill)
	if _, ok := friendBill["friend_token"]; ok {
		t.Error("friend view should not expose friend_token")
	}
	if _, ok := friendBill["share_url"]; ok {
		t.Error("friend view should not expose share_url")
	}

	itemsRaw, _ := friendBill["items"].([]any)
	if len(itemsRaw) < 2 {
		t.Fatalf("need >= 2 items for split test, got %d", len(itemsRaw))
	}
	var itemIDs []string
	itemTotal := 0
	for _, it := range itemsRaw {
		m := it.(map[string]any)
		itemIDs = append(itemIDs, m["id"].(string))
		itemTotal += int(m["price_cents"].(float64))
	}
	taxCents := int(friendBill["tax_cents"].(float64))
	tipCents := int(friendBill["tip_cents"].(float64))

	// Two friends join the bill.
	joinFriend := func(name string) string {
		var resp struct {
			ParticipantToken string `json:"participant_token"`
		}
		e.doJSON(e.newClient(), http.MethodPost, "/api/bills/"+billID+"/participants",
			map[string]string{"display_name": name, "t": friendToken},
			http.StatusCreated, &resp)
		if resp.ParticipantToken == "" {
			t.Fatalf("join %s: missing participant_token", name)
		}
		return resp.ParticipantToken
	}
	tokenA := joinFriend("Alice")
	tokenB := joinFriend("Bob")

	// Alice claims item 0, Bob claims item 1; any further items stay unclaimed.
	e.doJSON(e.newClient(), http.MethodPut, "/api/bills/"+billID+"/claims",
		map[string]any{"participant_token": tokenA, "item_ids": itemIDs[:1]},
		http.StatusOK, nil)
	e.doJSON(e.newClient(), http.MethodPut, "/api/bills/"+billID+"/claims",
		map[string]any{"participant_token": tokenB, "item_ids": itemIDs[1:2]},
		http.StatusOK, nil)

	// GET the summary and assert the split invariant.
	var summary struct {
		Split struct {
			Participants []struct {
				TotalCents int `json:"total_cents"`
			} `json:"participants"`
			UnclaimedCents     int `json:"unclaimed_cents"`
			ServiceChargeCents int `json:"service_charge_cents"`
			GrandTotalCents    int `json:"grand_total_cents"`
		} `json:"split"`
	}
	e.doJSON(e.newClient(), http.MethodGet,
		"/api/bills/"+billID+"/summary?t="+friendToken, nil, http.StatusOK, &summary)

	sumParticipants := 0
	for _, p := range summary.Split.Participants {
		sumParticipants += p.TotalCents
	}
	// The StubParser receipt carries a 10% percent service charge, so the
	// reconciliation must include it alongside items, tax and tip.
	if summary.Split.ServiceChargeCents <= 0 {
		t.Errorf("service_charge_cents = %d, want > 0", summary.Split.ServiceChargeCents)
	}
	wantGrand := itemTotal + taxCents + tipCents + summary.Split.ServiceChargeCents
	if got := sumParticipants + summary.Split.UnclaimedCents; got != wantGrand {
		t.Errorf("invariant broken: sum(participant totals)=%d + unclaimed=%d = %d, want %d",
			sumParticipants, summary.Split.UnclaimedCents, got, wantGrand)
	}
	if summary.Split.GrandTotalCents != wantGrand {
		t.Errorf("grand_total_cents = %d, want %d", summary.Split.GrandTotalCents, wantGrand)
	}
	if summary.Split.UnclaimedCents <= 0 {
		t.Errorf("expected unclaimed_cents > 0 (some items left unclaimed), got %d",
			summary.Split.UnclaimedCents)
	}
}

func TestAuthNegative(t *testing.T) {
	e := newTestEnv(t)

	t.Run("me without cookie is 401", func(t *testing.T) {
		resp, _ := e.do(e.newClient(), http.MethodGet, "/api/auth/me", nil, "")
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", resp.StatusCode)
		}
	})

	t.Run("magic link token cannot be reused", func(t *testing.T) {
		client := e.newClient()
		var reqResp map[string]string
		e.doJSON(client, http.MethodPost, "/api/auth/request",
			map[string]string{"email": "reuse@example.com"}, http.StatusOK, &reqResp)
		u, err := url.Parse(reqResp["link"])
		if err != nil {
			t.Fatalf("parse link: %v", err)
		}
		token := u.Query().Get("token")

		// First verify succeeds.
		e.doJSON(client, http.MethodPost, "/api/auth/verify",
			map[string]string{"token": token}, http.StatusOK, nil)
		// Second verify with the same token must fail.
		resp, _ := e.do(e.newClient(), http.MethodPost, "/api/auth/verify",
			bytes.NewReader(mustJSON(t, map[string]string{"token": token})),
			"application/json")
		if resp.StatusCode == http.StatusOK {
			t.Error("reusing a magic-link token should fail")
		}
	})
}

func TestAccessControl(t *testing.T) {
	e := newTestEnv(t)
	host := e.signIn("owner@example.com")
	bill := e.createBill(host)
	billID := bill["id"].(string)

	t.Run("get bill without token and not host is 404", func(t *testing.T) {
		resp, _ := e.do(e.newClient(), http.MethodGet, "/api/bills/"+billID, nil, "")
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("status = %d, want 404", resp.StatusCode)
		}
	})

	t.Run("get bill with wrong token is 404", func(t *testing.T) {
		resp, _ := e.do(e.newClient(), http.MethodGet,
			"/api/bills/"+billID+"?t=not-the-real-token", nil, "")
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("status = %d, want 404", resp.StatusCode)
		}
	})

	t.Run("patch bill by different user is 403", func(t *testing.T) {
		other := e.signIn("intruder@example.com")
		resp, raw := e.do(other, http.MethodPatch, "/api/bills/"+billID,
			bytes.NewReader(mustJSON(t, map[string]any{
				"restaurant": "Hijacked", "tax_cents": 0, "tip_cents": 0,
			})), "application/json")
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("status = %d, want 403; body=%s", resp.StatusCode, raw)
		}
	})
}

func TestUpdateBillCurrency(t *testing.T) {
	e := newTestEnv(t)
	host := e.signIn("host@example.com")
	bill := e.createBill(host)
	billID := bill["id"].(string)
	if bill["currency"] != "USD" {
		t.Errorf("new bill currency = %v, want USD", bill["currency"])
	}

	// The host switches the bill to PLN; a lowercase code is normalized.
	var updated map[string]any
	e.doJSON(host, http.MethodPatch, "/api/bills/"+billID, map[string]any{
		"restaurant": "Bar Mleczny", "currency": "pln",
		"tax_cents": 0, "tip_cents": 0,
	}, http.StatusOK, &updated)
	if updated["currency"] != "PLN" {
		t.Errorf("currency = %v, want PLN", updated["currency"])
	}

	// It persists across a fresh GET.
	var reloaded map[string]any
	e.doJSON(host, http.MethodGet, "/api/bills/"+billID, nil, http.StatusOK, &reloaded)
	if reloaded["currency"] != "PLN" {
		t.Errorf("reloaded currency = %v, want PLN", reloaded["currency"])
	}

	// A malformed currency code is rejected.
	e.doJSON(host, http.MethodPatch, "/api/bills/"+billID, map[string]any{
		"restaurant": "Bar Mleczny", "currency": "zloty",
		"tax_cents": 0, "tip_cents": 0,
	}, http.StatusBadRequest, nil)
}

func TestUpdateBillServiceCharge(t *testing.T) {
	e := newTestEnv(t)
	host := e.signIn("host@example.com")
	bill := e.createBill(host)
	billID := bill["id"].(string)
	if bill["service_charge_kind"] != "none" {
		t.Errorf("new bill service_charge_kind = %v, want none", bill["service_charge_kind"])
	}

	// Host sets a 12.5% percent service charge.
	var updated map[string]any
	e.doJSON(host, http.MethodPatch, "/api/bills/"+billID, map[string]any{
		"restaurant": "Gaia", "tax_cents": 0, "tip_cents": 0,
		"service_charge_kind": "percent", "service_charge_rate_bps": 1250,
	}, http.StatusOK, &updated)
	if updated["service_charge_kind"] != "percent" {
		t.Errorf("service_charge_kind = %v, want percent", updated["service_charge_kind"])
	}
	if bps, _ := updated["service_charge_rate_bps"].(float64); bps != 1250 {
		t.Errorf("service_charge_rate_bps = %v, want 1250", updated["service_charge_rate_bps"])
	}

	// Switching to a fixed charge with a headcount persists across a GET, and
	// the percent-only rate field is cleared.
	e.doJSON(host, http.MethodPatch, "/api/bills/"+billID, map[string]any{
		"restaurant": "Gaia", "tax_cents": 0, "tip_cents": 0,
		"service_charge_kind": "fixed", "service_charge_cents": 1200,
		"service_charge_headcount": 4,
	}, http.StatusOK, nil)
	var reloaded map[string]any
	e.doJSON(host, http.MethodGet, "/api/bills/"+billID, nil, http.StatusOK, &reloaded)
	if reloaded["service_charge_kind"] != "fixed" {
		t.Errorf("reloaded kind = %v, want fixed", reloaded["service_charge_kind"])
	}
	if c, _ := reloaded["service_charge_cents"].(float64); c != 1200 {
		t.Errorf("reloaded cents = %v, want 1200", reloaded["service_charge_cents"])
	}
	if h, _ := reloaded["service_charge_headcount"].(float64); h != 4 {
		t.Errorf("reloaded headcount = %v, want 4", reloaded["service_charge_headcount"])
	}
	if bps, _ := reloaded["service_charge_rate_bps"].(float64); bps != 0 {
		t.Errorf("reloaded rate_bps = %v, want 0", reloaded["service_charge_rate_bps"])
	}

	// An unknown service charge kind is rejected.
	e.doJSON(host, http.MethodPatch, "/api/bills/"+billID, map[string]any{
		"restaurant": "Gaia", "tax_cents": 0, "tip_cents": 0,
		"service_charge_kind": "gratuity",
	}, http.StatusBadRequest, nil)
}

func TestPayments(t *testing.T) {
	e := newTestEnv(t)
	host := e.signIn("payee@example.com")
	bill := e.createBill(host)
	billID := bill["id"].(string)
	friendToken := bill["friend_token"].(string)
	e.uploadReceipt(host, billID, http.StatusOK, nil)

	// A friend joins and claims the first item.
	var friendBill map[string]any
	e.doJSON(e.newClient(), http.MethodGet, "/api/by-token/"+friendToken,
		nil, http.StatusOK, &friendBill)
	firstItemID := friendBill["items"].([]any)[0].(map[string]any)["id"].(string)

	var joinResp struct {
		Participant struct {
			ID string `json:"id"`
		} `json:"participant"`
		ParticipantToken string `json:"participant_token"`
	}
	e.doJSON(e.newClient(), http.MethodPost, "/api/bills/"+billID+"/participants",
		map[string]string{"display_name": "Carol", "t": friendToken},
		http.StatusCreated, &joinResp)
	partToken := joinResp.ParticipantToken
	partID := joinResp.Participant.ID

	e.doJSON(e.newClient(), http.MethodPut, "/api/bills/"+billID+"/claims",
		map[string]any{"participant_token": partToken, "item_ids": []string{firstItemID}},
		http.StatusOK, nil)

	t.Run("pay fails with 409 when host has no wallet", func(t *testing.T) {
		resp, raw := e.do(e.newClient(), http.MethodPost, "/api/bills/"+billID+"/pay",
			bytes.NewReader(mustJSON(t, map[string]string{"participant_token": partToken})),
			"application/json")
		if resp.StatusCode != http.StatusConflict {
			t.Errorf("status = %d, want 409; body=%s", resp.StatusCode, raw)
		}
	})

	// Host sets a payout wallet.
	const hostWallet = "0xHostWalletAddress"
	e.doJSON(host, http.MethodPatch, "/api/users/me",
		map[string]string{"wallet_address": hostWallet}, http.StatusOK, nil)

	// Look up the participant's expected total from the summary.
	var summary struct {
		Split struct {
			Participants []struct {
				ParticipantID string `json:"participant_id"`
				TotalCents    int    `json:"total_cents"`
			} `json:"participants"`
		} `json:"split"`
	}
	e.doJSON(e.newClient(), http.MethodGet,
		"/api/bills/"+billID+"/summary?t="+friendToken, nil, http.StatusOK, &summary)
	wantAmount := -1
	for _, p := range summary.Split.Participants {
		if p.ParticipantID == partID {
			wantAmount = p.TotalCents
		}
	}
	if wantAmount < 0 {
		t.Fatalf("participant %s not found in summary", partID)
	}

	t.Run("pay returns 402 challenge with host wallet and correct amount", func(t *testing.T) {
		resp, raw := e.do(e.newClient(), http.MethodPost, "/api/bills/"+billID+"/pay",
			bytes.NewReader(mustJSON(t, map[string]string{"participant_token": partToken})),
			"application/json")
		if resp.StatusCode != http.StatusPaymentRequired {
			t.Fatalf("status = %d, want 402; body=%s", resp.StatusCode, raw)
		}
		var ch struct {
			PaymentID   string `json:"payment_id"`
			AmountCents int    `json:"amount_cents"`
			Recipient   string `json:"recipient"`
		}
		if err := json.Unmarshal(raw, &ch); err != nil {
			t.Fatalf("decode challenge: %v", err)
		}
		if ch.Recipient != hostWallet {
			t.Errorf("recipient = %q, want %q", ch.Recipient, hostWallet)
		}
		if ch.AmountCents != wantAmount {
			t.Errorf("amount_cents = %d, want %d", ch.AmountCents, wantAmount)
		}
		if ch.PaymentID == "" {
			t.Error("challenge missing payment_id")
		}

		// Confirm the payment; status becomes paid with a tx_ref.
		var confirmed struct {
			Status string `json:"status"`
			TxRef  string `json:"tx_ref"`
		}
		e.doJSON(e.newClient(), http.MethodPost, "/api/bills/"+billID+"/pay/confirm",
			map[string]string{
				"participant_token": partToken,
				"payment_id":        ch.PaymentID,
				"proof":             "any-proof-the-mock-ignores",
			}, http.StatusOK, &confirmed)
		if confirmed.Status != "paid" {
			t.Errorf("status = %q, want paid", confirmed.Status)
		}
		if confirmed.TxRef == "" {
			t.Error("confirmed payment missing tx_ref")
		}
	})
}

func TestDeleteBill(t *testing.T) {
	e := newTestEnv(t)
	host := e.signIn("deleter@example.com")

	bill := e.createBill(host)
	billID := bill["id"].(string)
	friendToken := bill["friend_token"].(string)
	e.uploadReceipt(host, billID, http.StatusOK, nil)

	// A friend joins, claims an item, and a payment is initiated, so the bill
	// has rows in items, participants, claims and payments. The delete must
	// cascade through all of them without violating a foreign key.
	var friendBill map[string]any
	e.doJSON(e.newClient(), http.MethodGet, "/api/by-token/"+friendToken,
		nil, http.StatusOK, &friendBill)
	firstItemID := friendBill["items"].([]any)[0].(map[string]any)["id"].(string)

	var joinResp struct {
		ParticipantToken string `json:"participant_token"`
	}
	e.doJSON(e.newClient(), http.MethodPost, "/api/bills/"+billID+"/participants",
		map[string]string{"display_name": "Dana", "t": friendToken},
		http.StatusCreated, &joinResp)
	e.doJSON(e.newClient(), http.MethodPut, "/api/bills/"+billID+"/claims",
		map[string]any{"participant_token": joinResp.ParticipantToken,
			"item_ids": []string{firstItemID}}, http.StatusOK, nil)

	e.doJSON(host, http.MethodPatch, "/api/users/me",
		map[string]string{"wallet_address": "0xDeleteTestWallet"}, http.StatusOK, nil)
	// The 402 challenge is the success path of /pay; it inserts a payment row.
	if resp, raw := e.do(e.newClient(), http.MethodPost, "/api/bills/"+billID+"/pay",
		bytes.NewReader(mustJSON(t, map[string]string{
			"participant_token": joinResp.ParticipantToken})),
		"application/json"); resp.StatusCode != http.StatusPaymentRequired {
		t.Fatalf("pay: status = %d, want 402; body=%s", resp.StatusCode, raw)
	}

	t.Run("unauthenticated delete is 401", func(t *testing.T) {
		resp, _ := e.do(e.newClient(), http.MethodDelete, "/api/bills/"+billID, nil, "")
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", resp.StatusCode)
		}
	})

	t.Run("delete by a different user is 403", func(t *testing.T) {
		intruder := e.signIn("intruder@example.com")
		resp, _ := e.do(intruder, http.MethodDelete, "/api/bills/"+billID, nil, "")
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("status = %d, want 403", resp.StatusCode)
		}
	})

	t.Run("delete of an unknown bill is 404", func(t *testing.T) {
		resp, _ := e.do(host, http.MethodDelete, "/api/bills/does-not-exist", nil, "")
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("status = %d, want 404", resp.StatusCode)
		}
	})

	t.Run("host deletes the bill and every dependent row", func(t *testing.T) {
		resp, raw := e.do(host, http.MethodDelete, "/api/bills/"+billID, nil, "")
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("status = %d, want 204; body=%s", resp.StatusCode, raw)
		}

		// The bill is gone: a host GET 404s and it leaves the host's list.
		getResp, _ := e.do(host, http.MethodGet, "/api/bills/"+billID, nil, "")
		if getResp.StatusCode != http.StatusNotFound {
			t.Errorf("get after delete: status = %d, want 404", getResp.StatusCode)
		}
		var list []map[string]any
		e.doJSON(host, http.MethodGet, "/api/bills", nil, http.StatusOK, &list)
		for _, b := range list {
			if b["id"] == billID {
				t.Errorf("deleted bill %s still in the host's list", billID)
			}
		}
	})

	t.Run("deleting an already-deleted bill is 404", func(t *testing.T) {
		resp, _ := e.do(host, http.MethodDelete, "/api/bills/"+billID, nil, "")
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("status = %d, want 404", resp.StatusCode)
		}
	})
}

// mustJSON marshals v or fails the test.
func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}
